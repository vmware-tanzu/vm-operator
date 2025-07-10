package virtualmachinesnapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	Finalizer = "vmoperator.vmware.com/virtualmachinesnapshot"
)

var (
	errParentVMSnapshotNotFound = errors.New("parent snapshot not found")
	errVMRefNil                 = errors.New("VirtualMachineSnapshot VMRef is nil")
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

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, vmSnapshot); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmSnapshotCtx := &pkgctx.VirtualMachineSnapshotContext{
		Context:                ctx,
		Logger:                 ctrl.Log.WithName("VirtualMachineSnapShot").WithValues("name", req.Name),
		VirtualMachineSnapshot: vmSnapshot,
	}

	patchHelper, err := patch.NewHelper(vmSnapshot, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmSnapshotCtx, err)
	}
	defer func() {
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

	if err := r.ReconcileNormal(vmSnapshotCtx); err != nil {
		vmSnapshotCtx.Logger.Error(err, "Failed to reconcile VirtualMachineSnapShot")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineSnapshot")

	if !controllerutil.ContainsFinalizer(ctx.VirtualMachineSnapshot, Finalizer) {
		// Set the finalizer and return so the object is patched immediately.
		controllerutil.AddFinalizer(ctx.VirtualMachineSnapshot, Finalizer)
		return nil
	}
	// return early if snapshot is ready; nothing to do
	if conditions.IsTrue(ctx.VirtualMachineSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition) {
		return nil
	}

	vmSnapshot := ctx.VirtualMachineSnapshot
	ctx.Logger.Info("Fetching VirtualMachine from snapshot object", "vmSnapshot", vmSnapshot.Name)
	if vmSnapshot.Spec.VMRef == nil {
		return errVMRefNil
	}
	vm := &vmopv1.VirtualMachine{}
	objKey := client.ObjectKey{Name: vmSnapshot.Spec.VMRef.Name, Namespace: vmSnapshot.Namespace}
	if err := r.Get(ctx, objKey, vm); err != nil {
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		return err
	}

	ctx.VM = vm
	if vm.Status.UniqueID == "" {
		return errors.New("VM hasn't been created and has no uniqueID")
	}

	// vm object already set with snapshot reference
	if vm.Status.CurrentSnapshot != nil && vm.Status.CurrentSnapshot.Name == ctx.VirtualMachineSnapshot.Name {
		ctx.Logger.Info("VirtualMachine current snapshot already up to date", "status.currentSnapshot", vm.Status.CurrentSnapshot.Name)
		return nil
	}

	objRef := vmSnapshotCRToLocalObjectRef(ctx.VirtualMachineSnapshot)

	// patch vm resource with the status.currentSnapshot
	vmPatch := client.MergeFrom(vm.DeepCopy())
	vm.Status.CurrentSnapshot = objRef
	if err := r.Status().Patch(ctx, vm, vmPatch); err != nil {
		return fmt.Errorf(
			"failed to patch VM resource %s with current snapshot %s: %w", objKey,
			ctx.VirtualMachineSnapshot.Name, err)
	}

	ctx.Logger.Info("Successfully patched VirtualMachine status with current snapshot reference", "vm.Name", vm.Name, "status.currentSnapshot", vm.Status.CurrentSnapshot.Name)
	return nil
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
			ctx.Logger.V(4).Info("VirtualMachine not found, assuming the snapshot is deleted along with moVM, remove finalizer")
			controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
			return nil
		}
		ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
		return err
	}

	ctx.VM = vm
	vmNotFound, err := r.deleteSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	if vmNotFound {
		ctx.Logger.V(4).Info("VirtualMachine not found, assuming the snapshot is deleted along with moVM, remove finalizer")
		controllerutil.RemoveFinalizer(vmSnapshot, Finalizer)
		return nil
	}

	parent, err := r.updateParentSnapshot(ctx)
	if err != nil {
		return err
	}

	if err := r.updateVMStatus(ctx, parent); err != nil {
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

func (r *Reconciler) updateParentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext) (*vmopv1.VirtualMachineSnapshot, error) {
	ctx.Logger.V(3).Info("Updating parent snapshot")
	vmSnapshot := ctx.VirtualMachineSnapshot
	parent, err := r.getParentSnapshot(ctx, vmSnapshot)
	if err != nil {
		if errors.Is(err, errParentVMSnapshotNotFound) {
			ctx.Logger.V(5).Info("parent snapshot not found")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get parent snapshot: %w", err)
	}
	ctx.Logger.V(5).Info("parent snapshot found", "parent", parent.Name)

	// remove current snapshot from parent's children
	parentPatch := client.MergeFrom(parent.DeepCopy())
	for i, child := range parent.Status.Children {
		if child.Name == vmSnapshot.Name {
			parent.Status.Children = slices.Delete(parent.Status.Children, i, i+1)
			break
		}
	}
	children := vmSnapshot.Status.Children
	if children != nil {
		ctx.Logger.V(5).Info("Add children snapshots of current snapshot to parent's children")
		// merge current's children to parent's children.
		// make sure no duplicates are added.
		for _, child := range children {
			if !slices.Contains(parent.Status.Children, child) {
				parent.Status.Children = append(parent.Status.Children, child)
			}
		}
	}
	if err := r.Status().Patch(ctx, parent, parentPatch); err != nil {
		return nil, fmt.Errorf("failed to patch parent snapshot %s with children: %w", parent.Name, err)
	}
	return parent, nil
}

// TODO: use a func in vmprovider to find the parent snapshot.
// So that we could recursively check VirtualMachineSnapshotTree
// to avoid the overhead of recursively requesting the apiserver.
// getParentSnapshotName returns the parent snapshot name of the given snapshot.
// It first checks if the snapshot is a root snapshot, if not, it will traverse the root snapshots to find the parent snapshot.
// The upper limit of snapshot count of a VM is 128. The overhead here could be considered acceptable.
func (r *Reconciler) getParentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext, vmSnapshot *vmopv1.VirtualMachineSnapshot) (*vmopv1.VirtualMachineSnapshot, error) {
	rootSnapshots := ctx.VM.Status.RootSnapshots
	if len(rootSnapshots) == 0 {
		ctx.Logger.V(5).Info("no root snapshots found for VM")
		return nil, errParentVMSnapshotNotFound
	}

	for _, rootSnapshot := range rootSnapshots {
		if rootSnapshot.Name == vmSnapshot.Name {
			return nil, errParentVMSnapshotNotFound
		}
		parent, err := r.findParentSnapshotHelper(ctx, rootSnapshot.Name, vmSnapshot.Name, vmSnapshot.Namespace)
		if err != nil && !errors.Is(err, errParentVMSnapshotNotFound) {
			return nil, err
		}
		if parent != nil {
			return parent, nil
		}
	}

	return nil, errParentVMSnapshotNotFound
}

func (r *Reconciler) findParentSnapshotHelper(ctx *pkgctx.VirtualMachineSnapshotContext, parentName, target, namespace string) (*vmopv1.VirtualMachineSnapshot, error) {
	parentVMSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := r.Get(ctx, client.ObjectKey{Name: parentName, Namespace: namespace}, parentVMSnapshot); err != nil {
		return nil, fmt.Errorf("failed to get parent snapshot %s: %w", parentName, err)
	}

	for _, child := range parentVMSnapshot.Status.Children {
		if child.Name == target {
			ctx.Logger.V(5).Info("found parent snapshot", "parent", parentName, "target", target)
			return parentVMSnapshot, nil
		}
		parent, err := r.findParentSnapshotHelper(ctx, child.Name, target, namespace)
		if err != nil && !errors.Is(err, errParentVMSnapshotNotFound) {
			return nil, err
		}
		if parent != nil {
			return parent, nil
		}
	}
	return nil, errParentVMSnapshotNotFound
}

// Update root snapshots and current snapshot of VM.
func (r *Reconciler) updateVMStatus(ctx *pkgctx.VirtualMachineSnapshotContext, parentVMSnapshot *vmopv1.VirtualMachineSnapshot) error {
	ctx.Logger.Info("Updating VM status")
	if err := r.updateVMCurrentSnapshot(ctx, parentVMSnapshot); err != nil {
		return err
	}
	if err := r.updateVMRootSnapshots(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) updateVMCurrentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext, parentVMSnapshot *vmopv1.VirtualMachineSnapshot) error {
	vm := ctx.VM
	if vm.Status.CurrentSnapshot != nil && vm.Status.CurrentSnapshot.Name != ctx.VirtualMachineSnapshot.Name {
		ctx.Logger.V(5).Info("VM status current snapshot is not the same as the snapshot being deleted, skipping update")
		return nil
	}
	vmPatch := client.MergeFrom(vm.DeepCopy())
	if parentVMSnapshot != nil {
		vm.Status.CurrentSnapshot = vmSnapshotCRToLocalObjectRef(parentVMSnapshot)
		ctx.Logger.V(5).Info("Updating VM status current snapshot", "vm", vm.Name, "new current snapshot", parentVMSnapshot.Name)
	} else {
		ctx.Logger.V(5).Info("Updating VM status current snapshot", "vm", vm.Name, "new current snapshot", "nil")
		vm.Status.CurrentSnapshot = nil
	}
	if err := r.Status().Patch(ctx, vm, vmPatch); err != nil {
		return fmt.Errorf("failed to patch VM %s status with current snapshot: %w", vm.Name, err)
	}
	return nil
}

func (r *Reconciler) updateVMRootSnapshots(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.Info("Updating VM root snapshots")
	vm := ctx.VM
	vmSnapshot := ctx.VirtualMachineSnapshot
	if len(vm.Status.RootSnapshots) == 0 {
		ctx.Logger.V(5).Info("No root snapshots found for VM, skipping update")
		return nil
	}
	// Check if the deleted snapshot is a root snapshot by only comparing the name,
	// in case we bump the new api version
	if !slices.ContainsFunc(vm.Status.RootSnapshots, func(e vmopv1common.LocalObjectRef) bool {
		return e.Name == vmSnapshot.Name
	}) {
		ctx.Logger.V(5).Info("Deleted snapshot is not a root snapshot, skipping update")
		return nil
	}

	vmPatch := client.MergeFrom(vm.DeepCopy())
	vm.Status.RootSnapshots = slices.DeleteFunc(vm.Status.RootSnapshots, func(e vmopv1common.LocalObjectRef) bool {
		return e.Name == vmSnapshot.Name
	})
	for _, child := range vmSnapshot.Status.Children {
		if !slices.Contains(vm.Status.RootSnapshots, child) {
			vm.Status.RootSnapshots = append(vm.Status.RootSnapshots, child)
		}
	}
	if err := r.Status().Patch(ctx, vm, vmPatch); err != nil {
		return fmt.Errorf("failed to patch VM %s with root snapshots: %w", vm.Name, err)
	}
	return nil
}

func vmSnapshotCRToLocalObjectRef(vmSnapshot *vmopv1.VirtualMachineSnapshot) *vmopv1common.LocalObjectRef {
	return &vmopv1common.LocalObjectRef{
		APIVersion: vmSnapshot.APIVersion,
		Kind:       vmSnapshot.Kind,
		Name:       vmSnapshot.Name,
	}
}
