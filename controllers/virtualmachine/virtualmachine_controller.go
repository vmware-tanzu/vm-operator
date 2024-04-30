// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vspherevm "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

const (
	finalizerName = "virtualmachine.vmoperator.vmware.com"

	// vmClassControllerName is the name of the controller specified in a
	// VirtualMachineClass resource's field spec.controllerName field that
	// indicates a VM pointing to that VM Class should be reconciled by this
	// controller.
	vmClassControllerName = "vmoperator.vmware.com/vsphere"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachine{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	proberManager, err := prober.AddToManager(mgr, ctx.VMProvider)
	if err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
		proberManager)

	// isDefaultVMClassController is used to configure watches and predicates
	// for the controller. If a VirtualMachineClass resource's
	// spec.controllerName field is missing or empty, this controller will
	// consider that VM Class as long as isDefaultVMClassController is true.
	isDefaultVMClassController := getIsDefaultVMClassController(ctx)

	builder := ctrl.NewControllerManagedBy(mgr).
		// Filter any VMs that reference a VM Class with a spec.controllerName set
		// to a non-empty value other than "vmoperator.vmware.com/vsphere". Please
		// note that a VM will *not* be filtered if the VM Class does not exist. A
		// VM is also not filtered if the field spec.controllerName is missing or
		// empty as long as the default VM Class controller is
		// "vmoperator.vmware.com/vsphere".
		For(controlledType, ctrlbuilder.WithPredicates(kubeutil.VMForControllerPredicate(
			r.Client,
			// These events can be very verbose, so be careful to not log them
			// at too high of a log level.
			r.Logger.WithName("VMForControllerPredicate").V(8),
			vmClassControllerName,
			kubeutil.VMForControllerPredicateOptions{
				MatchIfVMClassNotFound:            true,
				MatchIfControllerNameFieldEmpty:   isDefaultVMClassController,
				MatchIfControllerNameFieldMissing: isDefaultVMClassController,
			}))).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles})

	builder = builder.Watches(&vmopv1.VirtualMachineClass{},
		handler.EnqueueRequestsFromMapFunc(classToVMMapperFn(ctx, r.Client, isDefaultVMClassController)))

	return builder.Complete(r)
}

// classToVMMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachines in response to an event on the VirtualMachineClass resource when
// WCP_Namespaced_VM_Class FSS is enabled.
func classToVMMapperFn(ctx *pkgctx.ControllerManagerContext, c client.Client, isDefaultVMClassController bool) func(_ context.Context, o client.Object) []reconcile.Request {
	// For a given VirtualMachineClass, return reconcile requests
	// for those VirtualMachines with corresponding VirtualMachinesClasses referenced
	return func(_ context.Context, o client.Object) []reconcile.Request {
		class := o.(*vmopv1.VirtualMachineClass)
		logger := ctx.Logger.WithValues("name", class.Name, "namespace", class.Namespace)

		// Only watch resources that reference a VirtualMachineClass with its
		// field spec.controllerName equal to controllerName or if the field is
		// empty and isDefaultVMClassController is true.
		controllerName := class.Spec.ControllerName
		if controllerName == "" && !isDefaultVMClassController {
			// Log at a high-level so the logs are not over-run.
			logger.V(8).Info(
				"Skipping class with empty controller name & not default VM Class controller",
				"defaultVMClassController", pkgcfg.FromContext(ctx).DefaultVMClassControllerName,
				"expectedControllerName", vmClassControllerName)
			return nil
		}
		if controllerName != vmClassControllerName {
			// Log at a high-level so the logs are not over-run.
			logger.V(8).Info(
				"Skipping class with mismatched controller name",
				"actualControllerName", controllerName,
				"expectedControllerName", vmClassControllerName)
			return nil
		}

		logger.V(4).Info("Reconciling all VMs referencing a VM class because of a VirtualMachineClass watch")

		// Find all VM resources that reference this VM Class.
		vmList := &vmopv1.VirtualMachineList{}
		if err := c.List(ctx, vmList, client.InNamespace(class.Namespace)); err != nil {
			logger.Error(err, "Failed to list VirtualMachines for reconciliation due to VirtualMachineClass watch")
			return nil
		}

		// Populate reconcile requests for VMs that reference this VM Class.
		var reconcileRequests []reconcile.Request
		for _, vm := range vmList.Items {
			if vm.Spec.ClassName == class.Name {
				key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
				reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
			}
		}

		logger.Info("Returning VM reconcile requests due to VirtualMachineClass watch", "requests", reconcileRequests)
		return reconcileRequests
	}
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface,
	prober prober.Manager) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
		Prober:     prober,
		vmMetrics:  metrics.NewVMMetrics(),
	}
}

// Reconciler reconciles a VirtualMachine object.
type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
	Prober     prober.Manager
	vmMetrics  *metrics.VMMetrics
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworkinterfaces;virtualnetworkinterfaces/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=netoperator.vmware.com,resources=networkinterfaces;vmxnet3networkinterfaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas,verbs=get;list;watch
// +kubebuilder:rbac:groups=nsx.vmware.com,resources=subnetports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nsx.vmware.com,resources=subnetports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmCtx := &pkgctx.VirtualMachineContext{
		Context: ctx,
		Logger:  ctrl.Log.WithName("VirtualMachine").WithValues("name", vm.NamespacedName()),
		VM:      vm,
	}

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper for %s", vmCtx.String())
	}

	defer func() {
		if err := patchHelper.Patch(ctx, vm); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vm.DeletionTimestamp.IsZero() {
		err = r.ReconcileDelete(vmCtx)
		return ctrl.Result{}, err
	}

	if err := r.ReconcileNormal(vmCtx); err != nil {
		vmCtx.Logger.Error(err, "Failed to reconcile VirtualMachine")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueDelay(vmCtx)}, nil
}

// Determine if we should request a non-zero requeue delay in order to trigger a non-rate limited reconcile
// at some point in the future.  Use this delay-based reconcile to trigger a specific reconcile to discovery the VM IP
// address rather than relying on the resync period to do.
//
// TODO: It would be much preferable to determine that a non-error resync is required at the source of the determination that
// TODO: the VM IP isn't available rather than up here in the reconcile loop.  However, in the interest of time, we are making
// TODO: this determination here and will have to refactor at some later date.
func requeueDelay(ctx *pkgctx.VirtualMachineContext) time.Duration {
	// If the VM is in Creating phase, the reconciler has run out of threads to Create VMs on the provider. Do not queue
	// immediately to avoid exponential backoff.
	if conditions.IsFalse(ctx.VM, vmopv1.VirtualMachineConditionCreated) {
		return 10 * time.Second
	}

	if ctx.VM.Status.PowerState == vmopv1.VirtualMachinePowerStateOn {
		network := ctx.VM.Status.Network
		if network == nil || (network.PrimaryIP4 == "" && network.PrimaryIP6 == "") {
			return 10 * time.Second
		}
	}

	return 0
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineContext) (reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachine Deletion")

	// If the VM reconciliation has been paused by the developer,
	// skip deletion and return.
	if _, exists := ctx.VM.Annotations[vmopv1.PauseAnnotation]; exists {
		ctx.Logger.Info("VirtualMachine is not deleted because devops paused this VM")
		return nil
	}

	if controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		defer func() {
			r.Recorder.EmitEvent(ctx.VM, "Delete", reterr, false)
		}()

		if err := r.VMProvider.DeleteVirtualMachine(ctx, ctx.VM); err != nil {
			// If VM can not be deleted due to reconciliation being paused, ignore that.
			if errors.Is(err, vspherevm.ErrorVMPausedByAdmin()) {
				ctx.Logger.Info("VM could not be deleted since it contains the pause reconcile ExtraConfig key")
				return nil
			}
			ctx.Logger.Error(err, "Failed to delete VirtualMachine")
			return err
		}

		controllerutil.RemoveFinalizer(ctx.VM, finalizerName)
		ctx.Logger.Info("Provider Completed deleting Virtual Machine", "time", time.Now().Format(time.RFC3339))
	}

	// BMV: Shouldn't these be in the ContainsFinalizer block?
	r.vmMetrics.DeleteMetrics(ctx)
	r.Prober.RemoveFromProberManager(ctx.VM)

	ctx.Logger.Info("Finished Reconciling VirtualMachine Deletion")
	return nil
}

// ReconcileNormal processes a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineContext) (reterr error) {
	if !controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		// The finalizer must be present before proceeding in order to ensure that the VM will
		// be cleaned up. Return immediately after here to let the patcher helper update the
		// object, and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.VM, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachine")

	defer func(beforeVMStatus *vmopv1.VirtualMachineStatus) {
		// Log the reconcile time using the CR creation time and the time the VM reached the desired state
		if reterr == nil && !apiequality.Semantic.DeepEqual(beforeVMStatus, &ctx.VM.Status) {
			ctx.Logger.Info("Finished Reconciling VirtualMachine with updates to the CR",
				"createdTime", ctx.VM.CreationTimestamp, "currentTime", time.Now().Format(time.RFC3339),
				"spec.PowerState", ctx.VM.Spec.PowerState, "status.PowerState", ctx.VM.Status.PowerState)
		} else {
			ctx.Logger.Info("Finished Reconciling VirtualMachine")
		}

		beforeNoIP := beforeVMStatus.Network == nil || (beforeVMStatus.Network.PrimaryIP4 == "" && beforeVMStatus.Network.PrimaryIP6 == "")
		nowWithIP := ctx.VM.Status.Network != nil && (ctx.VM.Status.Network.PrimaryIP4 != "" || ctx.VM.Status.Network.PrimaryIP6 != "")
		if beforeNoIP && nowWithIP {
			ctx.Logger.Info("VM successfully got assigned with an IP address", "time", time.Now().Format(time.RFC3339))
		}
	}(ctx.VM.Status.DeepCopy())

	defer func() {
		r.vmMetrics.RegisterVMCreateOrUpdateMetrics(ctx)
	}()

	if err := r.VMProvider.CreateOrUpdateVirtualMachine(ctx, ctx.VM); err != nil {
		r.Recorder.EmitEvent(ctx.VM, "CreateOrUpdate", err, false)
		return err
	}

	// Add this VM to prober manager if ReconcileNormal succeeds.
	r.Prober.AddToProberManager(ctx.VM)

	ctx.Logger.Info("Finished Reconciling VirtualMachine")
	return nil
}

func getIsDefaultVMClassController(ctx context.Context) bool {
	if v := pkgcfg.FromContext(ctx).DefaultVMClassControllerName; v == "" || v == vmClassControllerName {
		return true
	}
	return false
}
