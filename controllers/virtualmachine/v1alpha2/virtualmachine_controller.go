// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
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
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	metrics "github.com/vmware-tanzu/vm-operator/pkg/metrics2"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
	prober "github.com/vmware-tanzu/vm-operator/pkg/prober2"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName = "virtualmachine.vmoperator.vmware.com"

	// vmClassControllerName is the name of the controller specified in a
	// VirtualMachineClass resource's field spec.controllerName field that
	// indicates a VM pointing to that VM Class should be reconciled by this
	// controller.
	vmClassControllerName = "vmoperator.vmware.com/vsphere"
)

var (
	// isDefaultVMClassController is used to configure watches and predicates
	// for the controller. If a VirtualMachineClass resource's
	// spec.controllerName field is missing or empty, this controller will
	// consider that VM Class as long as isDefaultVMClassController is true.
	isDefaultVMClassController = vmClassControllerName ==
		lib.GetDefaultVirtualMachineClassControllerName()
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachine{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	proberManager, err := prober.AddToManager(mgr, ctx.VMProviderA2)
	if err != nil {
		return err
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProviderA2,
		proberManager,
		ctx.MaxConcurrentReconciles/(100/lib.MaxConcurrentCreateVMsOnProvider()),
	)

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

	builder = builder.Watches(&source.Kind{Type: &vmopv1.VirtualMachineClass{}},
		handler.EnqueueRequestsFromMapFunc(classToVMMapperFn(ctx, r.Client)))

	return builder.Complete(r)
}

// classToVMMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachines in response to an event on the VirtualMachineClass resource when
// WCP_Namespaced_VM_Class FSS is enabled.
func classToVMMapperFn(ctx *context.ControllerManagerContext, c client.Client) func(o client.Object) []reconcile.Request {
	// For a given VirtualMachineClass, return reconcile requests
	// for those VirtualMachines with corresponding VirtualMachinesClasses referenced
	return func(o client.Object) []reconcile.Request {
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
				"defaultVMClassController", lib.GetDefaultVirtualMachineClassControllerName(),
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
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterfaceA2,
	prober prober.Manager,
	maxDeployThreads int) *Reconciler {

	return &Reconciler{
		Client:           client,
		Logger:           logger,
		Recorder:         recorder,
		VMProvider:       vmProvider,
		Prober:           prober,
		vmMetrics:        metrics.NewVMMetrics(),
		maxDeployThreads: maxDeployThreads,
	}
}

// Reconciler reconciles a VirtualMachine object.
type Reconciler struct {
	client.Client
	Logger           logr.Logger
	Recorder         record.Recorder
	VMProvider       vmprovider.VirtualMachineProviderInterfaceA2
	Prober           prober.Manager
	vmMetrics        *metrics.VMMetrics
	maxDeployThreads int
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworkinterfaces;virtualnetworkinterfaces/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmCtx := &context.VirtualMachineContextA2{
		Context: goctx.WithValue(ctx, context.MaxDeployThreadsContextKey, r.maxDeployThreads),
		Logger:  ctrl.Log.WithName("VirtualMachine").WithValues("name", vm.NamespacedName()),
		VM:      vm,
	}

	// If the VM has a pause reconcile annotation, it is being restored on vCenter. Return here so our reconcile
	// does not replace the VM being restored on the vCenter inventory.
	//
	// Do not requeue the reconcile here since removing the pause annotation will trigger a reconcile anyway.
	if _, ok := vm.Annotations[vmopv1.PauseAnnotation]; ok {
		vmCtx.Logger.Info("Skipping reconcile since Pause annotation is set on the VM")
		return ctrl.Result{}, nil
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
func requeueDelay(ctx *context.VirtualMachineContextA2) time.Duration {
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

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachineContextA2) (reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachine Deletion")

	if controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		defer func() {
			r.Recorder.EmitEvent(ctx.VM, "Delete", reterr, false)
		}()

		if err := r.VMProvider.DeleteVirtualMachine(ctx, ctx.VM); err != nil {
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
func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachineContextA2) (reterr error) {
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
		ctx.Logger.Error(err, "Failed to reconcile VirtualMachine")
		r.Recorder.EmitEvent(ctx.VM, "CreateOrUpdate", err, false)
		return err
	}

	// Add this VM to prober manager if ReconcileNormal succeeds.
	r.Prober.AddToProberManager(ctx.VM)

	// Back up this VM if ReconcileNormal succeeds and the FSS is enabled.
	if lib.IsVMServiceBackupRestoreFSSEnabled() {
		if err := r.VMProvider.BackupVirtualMachine(ctx, ctx.VM); err != nil {
			ctx.Logger.Error(err, "Failed to backup VirtualMachine")
			r.Recorder.EmitEvent(ctx.VM, "Backup", err, false)
			return err
		}
	}

	ctx.Logger.Info("Finished Reconciling VirtualMachine")
	return nil
}
