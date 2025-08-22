package virtualmachinesnapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	Finalizer = "vmoperator.vmware.com/virtualmachinesnapshot"
)

var (
	errVMRefNil = pkgerr.NoRequeueNoErr("VirtualMachineSnapshot VMRef is nil")
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineSnapshot{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			SkipNameValidation:      SkipNameValidation,
			LogConstructor:          pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		VMProvider: vmProvider,
		Recorder:   recorder,
	}
}

// Reconciler reconciles a VirtualMachineSnapShot object.
type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = cource.JoinContext(ctx, r.Context)
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, vmSnapshot); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmSnapshotCtx := &pkgctx.VirtualMachineSnapshotContext{
		Context:                ctx,
		Logger:                 pkgutil.FromContextOrDefault(ctx),
		VirtualMachineSnapshot: vmSnapshot,
		StorageClassesToSync:   sets.New[string](),
	}

	patchHelper, err := patch.NewHelper(vmSnapshot, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmSnapshotCtx, err)
	}

	defer func() {
		for _, sc := range vmSnapshotCtx.StorageClassesToSync.UnsortedList() {
			vmopv1util.SyncStorageUsageForNamespace(
				ctx,
				vmSnapshot.Namespace,
				sc)
		}
		vmSnapshotCtx.Logger.Info("Patching VirtualMachineSnapShot", "snap", vmSnapshot)
		if err := patchHelper.Patch(ctx, vmSnapshot); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmSnapshotCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmSnapshot.DeletionTimestamp.IsZero() {
		if err := r.ReconcileDelete(vmSnapshotCtx); err != nil {
			vmSnapshotCtx.Logger.Error(err, "Failed to delete VirtualMachineSnapshot")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return r.ReconcileNormal(vmSnapshotCtx)
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineSnapshotContext) (ctrl.Result, error) {
	ctx.Logger.Info("Reconciling VirtualMachineSnapshot")

	// If the finalizer is not present, add it.  Return so the object is patched immediately.
	if controllerutil.AddFinalizer(ctx.VirtualMachineSnapshot, Finalizer) {
		return ctrl.Result{}, nil
	}

	vmSnapshot := ctx.VirtualMachineSnapshot
	ctx.Logger.Info("Fetching VirtualMachine from snapshot object")

	if vmSnapshot.Spec.VMRef == nil {
		return ctrl.Result{}, errVMRefNil
	}

	if conditions.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition) {
		ensureCSIVolumeSyncAnnotation(vmSnapshot)
	}

	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmSnapshot.Spec.VMRef.Name, Namespace: vmSnapshot.Namespace}
	if err := r.Get(ctx, objKey, vm); err != nil {
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		return ctrl.Result{}, err
	}

	ctx.VM = vm

	// The snapshot must be owned by a VM.  Set an owner reference to the VM.
	if err := controllerutil.SetOwnerReference(ctx.VM, ctx.VirtualMachineSnapshot, r.Scheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference to snapshot: %w", err)
	}

	if vm.Status.UniqueID == "" {
		return ctrl.Result{}, errors.New("VM hasn't been created and has no uniqueID")
	}

	// Calculate the requested capacity of the snapshot at the beginning only once.
	if vmSnapshot.Status.Storage == nil {
		vmSnapshot.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{}
	}

	if vmSnapshot.Status.Storage.Requested == nil {
		ctx.Logger.V(4).Info("Updating snapshot's status requested capacity")
		vmSnapshot.Status.Storage.Requested = []vmopv1.VirtualMachineSnapshotStorageStatusRequested{}
		requested, err := kubeutil.CalculateReservedForSnapshotPerStorageClass(ctx, r.Client, r.Logger, *vmSnapshot)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to calculate requested capacity for snapshot: %w", err)
		}
		vmSnapshot.Status.Storage.Requested = requested
		ctx.Logger.V(5).Info("Updated vmSnapshot requested capacity",
			"requested", vmSnapshot.Status.Storage.Requested)
		// Enqueue the storage classes to sync corresponding SPU.
		for _, requested := range vmSnapshot.Status.Storage.Requested {
			ctx.StorageClassesToSync.Insert(requested.StorageClass)
		}
	}

	ctx.Logger.V(5).Info("Updating snapshot's status used capacity")
	size, err := r.VMProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get snapshot size: %w", err)
	}
	// TODO(lubron): Now the format will very likely to fall back to decimalSI
	// since the size is int64 and it's not a multiple of 1024.
	// We can think of refactoring to show a more humain readble value without loosing
	// the precision in the future.
	total := kubeutil.BytesToResource(size)

	if vmSnapshot.Status.Storage.Used == nil {
		vmSnapshot.Status.Storage.Used = resource.NewQuantity(0, resource.BinarySI)
	}
	vmSnapshot.Status.Storage.Used = total
	ctx.Logger.V(5).Info("Updated vmSnapshot's status used capacity", "used", total)

	// Enqueue the storage class to sync corresponding SPU only after CSI has completed the sync.
	if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] ==
		constants.CSIVSphereVolumeSyncAnnotationValueCompleted {
		ctx.StorageClassesToSync.Insert(vm.Spec.StorageClass)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineSnapshot deletion")
	vmSnapshot := ctx.VirtualMachineSnapshot

	if !controllerutil.ContainsFinalizer(vmSnapshot, Finalizer) {
		ctx.Logger.V(4).Info("VirtualMachineSnapshot finalizer not found, skipping deletion")
		return nil
	}

	if vmSnapshot.Spec.VMRef == nil {
		return errVMRefNil
	}

	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmSnapshot.Spec.VMRef.Name, Namespace: vmSnapshot.Namespace}
	if err := r.Get(ctx, objKey, vm); err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Logger.V(5).Info("VirtualMachine not found, assuming the " +
				"snapshot is deleted along with moVM, remove finalizer")
			controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
			// We don't sync SPU since we don't know which StorageClass of the VM.
			return nil
		}
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		return err
	}

	// Enqueue the storage class to sync corresponding SPU.
	ctx.StorageClassesToSync.Insert(vm.Spec.StorageClass)

	ctx.VM = vm

	// Fetch and update the parent snapshot first then delete the snapshot from the VM.
	// Since we find the parent snapshot from VC.
	if err := r.updateParentSnapshot(ctx); err != nil {
		return err
	}

	// delete snapshot from the VM
	vmNotFound, err := r.deleteSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	if vmNotFound {
		ctx.Logger.V(5).Info("VirtualMachine not found, assuming the" +
			" snapshot is deleted along with moVM, remove finalizer")
		controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
		return nil
	}

	if err := r.syncVMSSnapshotTreeStatus(ctx); err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
	return nil
}

// deleteSnapshot deletes the snapshot from the VM.
// It returns true if the VM is not found, false otherwise.
func (r *Reconciler) deleteSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext) (bool, error) {
	vmSnapshot := ctx.VirtualMachineSnapshot
	// TODO: set removeChildren and consolidate to false by default for now.
	vmNotFound, err := r.VMProvider.DeleteSnapshot(ctx.Context, vmSnapshot, ctx.VM, false, nil)
	if err != nil {
		return false, fmt.Errorf("failed to delete snapshot: %w", err)
	}

	return vmNotFound, nil
}

func (r *Reconciler) updateParentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.V(3).Info("Updating parent snapshot")
	vmSnapshot := ctx.VirtualMachineSnapshot
	parent, err := r.VMProvider.GetParentSnapshot(ctx.Context, vmSnapshot.Name, ctx.VM)
	if err != nil {
		return fmt.Errorf("failed to get parent snapshot: %w", err)
	}
	if parent == nil {
		ctx.Logger.V(5).Info("parent snapshot not found")
		return nil
	}
	ctx.Logger.V(5).Info("parent snapshot found", "parent", parent.Name)

	parentVMSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := r.Get(ctx, client.ObjectKey{Name: parent.Name, Namespace: vmSnapshot.Namespace}, parentVMSnapshot); err != nil {
		return fmt.Errorf("failed to get parent snapshot %s: %w", parent.Name, err)
	}

	// remove current snapshot from parent's children
	parentPatch := client.MergeFrom(parentVMSnapshot.DeepCopy())
	for i, child := range parentVMSnapshot.Status.Children {
		if child.Name == vmSnapshot.Name {
			parentVMSnapshot.Status.Children = slices.Delete(parentVMSnapshot.Status.Children, i, i+1)
			break
		}
	}
	children := vmSnapshot.Status.Children
	if children != nil {
		ctx.Logger.V(5).Info("Add children snapshots of current snapshot to parent's children")
		// merge current's children to parent's children.
		// make sure no duplicates are added.
		for _, child := range children {
			if !slices.Contains(parentVMSnapshot.Status.Children, child) {
				parentVMSnapshot.Status.Children = append(parentVMSnapshot.Status.Children, child)
			}
		}
	}
	if err := r.Status().Patch(ctx, parentVMSnapshot, parentPatch); err != nil {
		return fmt.Errorf("failed to patch parent snapshot %s with children: %w", parentVMSnapshot.Name, err)
	}
	return nil
}

// SyncVMSnapshotTreeStatus syncs the VM's current and root snapshots status.
func (r *Reconciler) syncVMSSnapshotTreeStatus(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.Info("Syncing VM's current and root snapshots status")
	mergePatch := client.MergeFrom(ctx.VM.DeepCopy())
	if err := r.VMProvider.SyncVMSnapshotTreeStatus(ctx.Context, ctx.VM); err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, ctx.VM, mergePatch)
}

// ensureCSIVolumeSyncAnnotation ensures the annotation is set to request
// to notify CSI driver to update the usage of VolumeSnapshot.
func ensureCSIVolumeSyncAnnotation(vmSnapshot *vmopv1.VirtualMachineSnapshot) {
	if vmSnapshot.Annotations == nil {
		vmSnapshot.Annotations = make(map[string]string)
	}
	// As long as the value is not set to completed by CSI driver, we need to mark it as requested.
	if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] != constants.CSIVSphereVolumeSyncAnnotationValueCompleted {
		vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] = constants.CSIVSphereVolumeSyncAnnotationValueRequest
	}
}
