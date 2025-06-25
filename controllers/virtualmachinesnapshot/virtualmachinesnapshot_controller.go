package virtualmachinesnapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
)

const (
	Finalizer = "vmoperator.vmware.com/virtualmachinesnapshot"
)

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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	VMProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		VMProvider: VMProvider,
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

	vmSnapshot := ctx.VirtualMachineSnapshot
	if !controllerutil.ContainsFinalizer(ctx.VirtualMachineSnapshot, Finalizer) {
		// Set the finalizer and return so the object is patched immediately.
		controllerutil.AddFinalizer(ctx.VirtualMachineSnapshot, Finalizer)
		return nil
	}
	// return early if snapshot is ready; nothing to do
	if conditions.IsTrue(ctx.VirtualMachineSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition) {
		return nil
	}

	ctx.Logger.Info("Fetching VirtualMachine from snapshot object", "vmSnapshot", vmSnapshot.Name)
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
	if vm.Spec.CurrentSnapshot != nil && vm.Spec.CurrentSnapshot.Name == ctx.VirtualMachineSnapshot.Name {
		ctx.Logger.Info("VirtualMachine current snapshot already up to date", "spec.currentSnapshot", vm.Spec.CurrentSnapshot.Name)
		return nil
	}

	objRef := vmSnapshotCRToLocalObjectRef(ctx.VirtualMachineSnapshot)

	// patch vm resource with the spec.currentSnapshot
	vmPatch := client.MergeFrom(vm.DeepCopy())
	vm.Spec.CurrentSnapshot = objRef
	if err := r.Patch(ctx, vm, vmPatch); err != nil {
		return fmt.Errorf(
			"failed to patch VM resource %s with current snapshot %s: %w", objKey,
			ctx.VirtualMachineSnapshot.Name, err)
	}

	ctx.Logger.Info("Successfully patched VirtualMachine's current snapshot reference", "vm.Name", vm.Name, "spec.currentSnapshot", vm.Spec.CurrentSnapshot.Name)
	return nil
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	ctx.Logger.Info("Reconciling VirtualMachineSnapshot deletion")

	snapshot := ctx.VirtualMachineSnapshot
	if controllerutil.ContainsFinalizer(snapshot, Finalizer) {
		vm := &vmopv1.VirtualMachine{}
		objKey := client.ObjectKey{Name: snapshot.Spec.VMRef.Name, Namespace: snapshot.Namespace}
		if err := r.Get(ctx, objKey, vm); err != nil {
			ctx.Logger.Error(err, "failed to get VirtualMachine", "vm", objKey)
			return err
		}
		ctx.VM = vm
		if err := r.deleteSnapshotFromVSphere(ctx); err != nil {
			return err
		}
		parent, err := r.updateParentSnapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.updateVMCurrentSnapshot(ctx, parent); err != nil {
			return err
		}

		controllerutil.RemoveFinalizer(snapshot, Finalizer)
	}

	return nil
}

func (r *Reconciler) deleteSnapshotFromVSphere(ctx *pkgctx.VirtualMachineSnapshotContext) error {
	snapshot := ctx.VirtualMachineSnapshot
	// TODO: set removeChildren and consolidate to false by default for now.
	if err := r.VMProvider.DeleteSnapshot(ctx, snapshot, ctx.VM, false, nil); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	return nil
}

func (r *Reconciler) updateParentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext) (*vmopv1.VirtualMachineSnapshot, error) {
	vmSnapshot := ctx.VirtualMachineSnapshot
	parent, err := r.getParentSnapshot(ctx, vmSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent snapshot: %w", err)
	}
	if parent == nil {
		ctx.Logger.V(5).Info("parent snapshot not found")
		return nil, nil
	}
	ctx.Logger.V(5).Info("parent snapshot found", "parent", parent.Name)

	// remove current snapshot from parent's children
	parentPatch := client.MergeFrom(parent.DeepCopy())
	for i, child := range parent.Status.Children {
		if child.Name == vmSnapshot.Name {
			parent.Status.Children = append(parent.Status.Children[:i], parent.Status.Children[i+1:]...)
			break
		}
	}
	children := vmSnapshot.Status.Children
	if children != nil {
		ctx.Logger.V(5).Info("Add children snapshots of current snapshot to parent's children")
		// merge current's children to parent's children.
		// no duplicates check here since we assume each snapshot can only have one parent.
		parent.Status.Children = append(parent.Status.Children, children...)
	}
	if err := r.Status().Patch(ctx, parent, parentPatch); err != nil {
		return nil, fmt.Errorf("failed to patch parent snapshot %s with children: %w", parent.Name, err)
	}
	return parent, nil
}

func (r *Reconciler) getParentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext, snapshot *vmopv1.VirtualMachineSnapshot) (*vmopv1.VirtualMachineSnapshot, error) {
	allSnapshots := &vmopv1.VirtualMachineSnapshotList{}
	if err := r.List(ctx, allSnapshots, &client.ListOptions{
		Namespace: snapshot.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list all snapshots under namespace %s: %w", snapshot.Namespace, err)
	}

	for _, s := range allSnapshots.Items {
		for _, c := range s.Status.Children {
			if c.Name == snapshot.Name {
				return &s, nil
			}
		}
	}
	return nil, nil
}

func (r *Reconciler) updateVMCurrentSnapshot(ctx *pkgctx.VirtualMachineSnapshotContext, vmSnapshot *vmopv1.VirtualMachineSnapshot) error {
	vm := ctx.VM
	if vm.Spec.CurrentSnapshot != nil && vm.Spec.CurrentSnapshot.Name != ctx.VirtualMachineSnapshot.Name {
		ctx.Logger.Info("VM current snapshot is not the same as the snapshot being deleted, skipping update")
		return nil
	}
	vmPatch := client.MergeFrom(vm.DeepCopy())
	if vmSnapshot != nil {
		ctx.Logger.Info("Updating VM current snapshot", "vm", vm.Name, "new current snapshot", vmSnapshot.Name)
		vm.Spec.CurrentSnapshot = vmSnapshotCRToLocalObjectRef(vmSnapshot)
	} else {
		ctx.Logger.Info("Updating VM current snapshot", "vm", vm.Name, "new current snapshot", "nil")
		vm.Spec.CurrentSnapshot = nil
	}
	if err := r.Patch(ctx, vm, vmPatch); err != nil {
		return fmt.Errorf("failed to patch VM %s with current snapshot: %w", vm.Name, err)
	}

	return nil
}

func vmSnapshotCRToLocalObjectRef(snapshot *vmopv1.VirtualMachineSnapshot) *vmopv1common.LocalObjectRef {
	return &vmopv1common.LocalObjectRef{
		APIVersion: snapshot.APIVersion,
		Kind:       snapshot.Kind,
		Name:       snapshot.Name,
	}
}
