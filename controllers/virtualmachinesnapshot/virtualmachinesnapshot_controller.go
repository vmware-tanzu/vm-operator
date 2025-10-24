package virtualmachinesnapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	Finalizer = "vmoperator.vmware.com/virtualmachinesnapshot"
)

var (
	errVMNameEmpty = pkgerr.NoRequeueNoErr("VirtualMachineSnapshot VMName is empty")
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
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
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
		Logger:                 pkglog.FromContextOrDefault(ctx),
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

		reconcileSnapshotReadyCondition(vmSnapshot)

		vmSnapshotCtx.Logger.V(4).Info("Patching VirtualMachineSnapshot",
			"snapshot", vmSnapshot)
		if err := patchHelper.Patch(ctx, vmSnapshot); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	if !vmSnapshot.DeletionTimestamp.IsZero() {
		if err := r.ReconcileDelete(vmSnapshotCtx); err != nil {
			return ctrl.Result{},
				fmt.Errorf("failed to delete VirtualMachineSnapshot: %w", err)
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

	if vmSnapshot.Spec.VMName == "" {
		return ctrl.Result{}, errors.New("vmName is required")
	}

	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmSnapshot.Spec.VMName, Namespace: vmSnapshot.Namespace}
	if err := r.Get(ctx, objKey, vm); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get VirtualMachine %q: %w", objKey, err)
	}

	ctx.VM = vm

	// The snapshot must be owned by a VM.  Set an owner reference to the VM.
	if err := controllerutil.SetOwnerReference(ctx.VM, ctx.VirtualMachineSnapshot, r.Scheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference to snapshot: %w", err)
	}

	if vm.Status.UniqueID == "" {
		return ctrl.Result{}, errors.New("VM hasn't been created and has no uniqueID")
	}

	if vmSnapshot.Status.Storage == nil {
		vmSnapshot.Status.Storage = &vmopv1.VirtualMachineSnapshotStorageStatus{}
	}

	// Calculate the requested capacity of the snapshot at the beginning only once.
	if vmSnapshot.Status.Storage.Requested == nil {
		if err := r.calculateRequestedCapacity(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Only start calculating the used capacity and sync CSI volume
	// after the snapshot is created.
	if !pkgcnd.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition) {
		return ctrl.Result{}, nil
	}

	ensureCSIVolumeSyncAnnotation(vmSnapshot)

	pkgcnd.MarkFalse(
		vmSnapshot,
		vmopv1.VirtualMachineSnapshotCSIVolumeSyncedCondition,
		vmopv1.VirtualMachineSnapshotCSIVolumeSyncInProgressReason,
		"Sync CSI volume requested",
	)

	if err := r.calculateUsedCapacity(ctx); err != nil {
		return ctrl.Result{}, err
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

	if vmSnapshot.Spec.VMName == "" {
		return errVMNameEmpty
	}

	// We should notify all impacted SPUs to update their requested capacity and
	// used capacity of the SPU that shares the same SC as the owner VM.
	if vmSnapshot.Status.Storage != nil &&
		vmSnapshot.Status.Storage.Requested != nil {

		for _, requested := range vmSnapshot.Status.Storage.Requested {
			ctx.StorageClassesToSync.Insert(requested.StorageClass)
		}
	}

	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmSnapshot.Spec.VMName, Namespace: vmSnapshot.Namespace}
	if err := r.Get(ctx, objKey, vm); err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Logger.V(5).Info("VirtualMachine not found, assuming the " +
				"snapshot is deleted along with moVM, remove finalizer")
			controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
			// No need to explicitly enqueue SPU update here since VM deletion
			// will trigger SPU sync for usage update.
			return nil
		}
		return fmt.Errorf("failed to get VirtualMachine %q: %w", objKey, err)
	}

	ctx.VM = vm

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
	if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] !=
		constants.CSIVSphereVolumeSyncAnnotationValueCompleted {

		vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] =
			constants.CSIVSphereVolumeSyncAnnotationValueRequested
	}
}

func reconcileSnapshotReadyCondition(vmSnapshot *vmopv1.VirtualMachineSnapshot) {
	// If the snapshot is being deleted, skip setting the condition.
	if !vmSnapshot.DeletionTimestamp.IsZero() {
		return
	}

	created := pkgcnd.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
	synced := pkgcnd.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCSIVolumeSyncedCondition)
	switch {
	case !created:
		pkgcnd.MarkFalse(
			vmSnapshot,
			vmopv1.VirtualMachineSnapshotReadyCondition,
			vmopv1.VirtualMachineSnapshotWaitingForCreationReason,
			"Snapshot is not ready because it doesn't have created condition",
		)
	case !synced:
		pkgcnd.MarkFalse(
			vmSnapshot,
			vmopv1.VirtualMachineSnapshotReadyCondition,
			vmopv1.VirtualMachineSnapshotWaitingForCSISyncReason,
			"Snapshot is not ready because CSI volume sync hasn't been completed",
		)
	default:
		// Only when the snapshot is created and the CSI volume sync is completed,
		// mark the snapshot as ready.
		pkgcnd.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
	}
}

func (r *Reconciler) calculateRequestedCapacity(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.V(4).Info("Updating snapshot's status requested capacity")
	vmSnapshot := ctx.VirtualMachineSnapshot
	requested, err := kubeutil.CalculateReservedForSnapshotPerStorageClass(ctx, r.Client, r.Logger, *vmSnapshot)
	if err != nil {
		return fmt.Errorf("failed to calculate requested capacity for snapshot: %w", err)
	}
	vmSnapshot.Status.Storage.Requested = requested

	ctx.Logger.V(5).Info("Updated vmSnapshot requested capacity",
		"requested", vmSnapshot.Status.Storage.Requested)

	// Add storageClasses referenced by the VM to the queue so their corresponding
	// SPUs could be synced to update their requested capacity.
	for _, requested := range vmSnapshot.Status.Storage.Requested {
		ctx.StorageClassesToSync.Insert(requested.StorageClass)
	}

	return nil
}

func (r *Reconciler) calculateUsedCapacity(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.V(4).Info("Updating snapshot's status used capacity")
	vmSnapshot := ctx.VirtualMachineSnapshot
	vm := ctx.VM
	size, err := r.VMProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
	if err != nil {
		return fmt.Errorf("failed to get snapshot size: %w", err)
	}
	// TODO(lubron): Now the format will very likely to fall back to decimalSI
	// since the size is int64 and it's not a multiple of 1024.
	// We can think of refactoring to show a more human readable value without losing
	// the precision in the future.
	total := kubeutil.BytesToResource(size)

	if vmSnapshot.Status.Storage.Used == nil {
		vmSnapshot.Status.Storage.Used = resource.NewQuantity(0, resource.BinarySI)
	}
	vmSnapshot.Status.Storage.Used = total
	ctx.Logger.V(5).Info("Updated vmSnapshot's status used capacity", "used", total)

	// Enqueue storage classes to sync corresponding SPUs with requested/used
	// capacity again after CSI has completed the sync.
	// Also mark the CSI volume sync condition as true.
	if vmSnapshot.Annotations[constants.CSIVSphereVolumeSyncAnnotationKey] ==
		constants.CSIVSphereVolumeSyncAnnotationValueCompleted {

		if !pkgcnd.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCSIVolumeSyncedCondition) {
			// Add storageClasses referenced by the VM to the queue so their
			// corresponding SPUs could be synced to update their requested/used
			// capacity.
			// Ideally it should always be synced but trying to save some
			// cycles to only sync all impacted SPUs after CSI sync is done.
			for _, requested := range vmSnapshot.Status.Storage.Requested {
				ctx.StorageClassesToSync.Insert(requested.StorageClass)
			}

			pkgcnd.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCSIVolumeSyncedCondition)
		} else {
			// In most of the cases, we just need to update the used capacity of
			// the SPU that has same StorageClass as VM.
			ctx.StorageClassesToSync.Insert(vm.Spec.StorageClass)
		}
	}

	return nil
}
