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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
	recorder record.Recorder) *Reconciler {
	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a VirtualMachineSnapShot object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
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
		// TODO: Handle snapshot deletion here.
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

	objRef := &vmopv1common.LocalObjectRef{
		APIVersion: vmSnapshot.APIVersion,
		Kind:       vmSnapshot.Kind,
		Name:       ctx.VirtualMachineSnapshot.Name,
	}

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
