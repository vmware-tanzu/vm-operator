// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"
	"fmt"
	"reflect"
	"strings"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const finalizerName = "virtualmachine.vmoperator.vmware.com"

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachine{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	proberManager, err := prober.AddToManager(mgr, ctx.VMProvider)
	if err != nil {
		return err
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
		proberManager,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &vmopv1alpha1.VirtualMachineClassBinding{}},
			handler.EnqueueRequestsFromMapFunc(classBindingToVMMapperFn(ctx, r.Client))).
		Watches(&source.Kind{Type: &vmopv1alpha1.ContentSourceBinding{}},
			handler.EnqueueRequestsFromMapFunc(csBindingToVMMapperFn(ctx, r.Client))).
		Complete(r)
}

// csBindingToVMMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachines in response to an event on the ContentSourceBinding resource.
func csBindingToVMMapperFn(ctx *context.ControllerManagerContext, c client.Reader) func(o client.Object) []reconcile.Request {
	return func(o client.Object) []reconcile.Request {
		binding := o.(*vmopv1alpha1.ContentSourceBinding)
		logger := ctx.Logger.WithValues("name", binding.Name, "namespace", binding.Namespace)

		logger.V(4).Info("Reconciling all VMs using images from a ContentSource because of a ContentSourceBinding watch")

		contentSource := &vmopv1alpha1.ContentSource{}
		if err := c.Get(ctx, client.ObjectKey{Name: binding.ContentSourceRef.Name}, contentSource); err != nil {
			logger.Error(err, "Failed to get ContentSource for VM reconciliation due to ContentSourceBinding watch")
			return nil
		}

		providerRef := contentSource.Spec.ProviderRef
		// Assume that only supported type is ContentLibraryProvider.
		clProviderFromBinding := vmopv1alpha1.ContentLibraryProvider{}
		if err := c.Get(ctx, client.ObjectKey{Name: providerRef.Name}, &clProviderFromBinding); err != nil {
			logger.Error(err, "Failed to get ContentLibraryProvider for VM reconciliation due to ContentSourceBinding watch")
			return nil
		}

		// Filter images that have an OwnerReference to this ContentLibraryProvider.
		imageList := &vmopv1alpha1.VirtualMachineImageList{}
		if err := c.List(ctx, imageList); err != nil {
			logger.Error(err, "Failed to list VirtualMachineImages for VM reconciliation due to ContentSourceBinding watch")
			return nil
		}

		imagesToReconcile := make(map[string]struct{})
		for _, img := range imageList.Items {
			for _, ownerRef := range img.OwnerReferences {
				if ownerRef.Kind == "ContentLibraryProvider" && ownerRef.UID == clProviderFromBinding.UID {
					imagesToReconcile[img.Name] = struct{}{}
				}
			}
		}

		// Filter VMs that reference the images from the content source.
		vmList := &vmopv1alpha1.VirtualMachineList{}
		if err := c.List(ctx, vmList, client.InNamespace(binding.Namespace)); err != nil {
			logger.Error(err, "Failed to list VirtualMachines for reconciliation due to ContentSourceBinding watch")
			return nil
		}

		var reconcileRequests []reconcile.Request
		for _, vm := range vmList.Items {
			if _, ok := imagesToReconcile[vm.Spec.ImageName]; ok {
				key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
				reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
			}
		}

		logger.V(4).Info("Returning VM reconcile requests due to ContentSourceBinding watch", "requests", reconcileRequests)
		return reconcileRequests
	}
}

// classBindingToVMMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachines in response to an event on the VirtualMachineClassBinding resource.
func classBindingToVMMapperFn(ctx *context.ControllerManagerContext, c client.Client) func(o client.Object) []reconcile.Request {
	// For a given VirtualMachineClassBinding, return reconcile requests
	// for those VirtualMachines with corresponding VirtualMachinesClasses referenced
	return func(o client.Object) []reconcile.Request {
		classBinding := o.(*vmopv1alpha1.VirtualMachineClassBinding)
		logger := ctx.Logger.WithValues("name", classBinding.Name, "namespace", classBinding.Namespace)

		logger.V(4).Info("Reconciling all VMs referencing a VM class because of a VirtualMachineClassBinding watch")

		// Find all vms that match this vmclassbinding
		vmList := &vmopv1alpha1.VirtualMachineList{}
		if err := c.List(ctx, vmList, client.InNamespace(classBinding.Namespace)); err != nil {
			logger.Error(err, "Failed to list VirtualMachines for reconciliation due to VirtualMachineClassBinding watch")
			return nil
		}

		// Populate reconcile requests for vms matching the classbinding reference
		var reconcileRequests []reconcile.Request
		for _, vm := range vmList.Items {
			if vm.Spec.ClassName == classBinding.ClassRef.Name {
				key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
				reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
			}
		}

		logger.V(4).Info("Returning VM reconcile requests due to VirtualMachineClassBinding watch", "requests", reconcileRequests)
		return reconcileRequests
	}
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterface,
	prober prober.Manager) *Reconciler {

	return &Reconciler{
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
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider vmprovider.VirtualMachineProviderInterface
	Prober     prober.Manager
	vmMetrics  *metrics.VMMetrics
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworkinterfaces;virtualnetworkinterfaces/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclassbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsources,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentlibraryproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=contentsourcebindings,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx goctx.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	vm := &vmopv1alpha1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmCtx := &context.VirtualMachineContext{
		Context: ctx,
		Logger:  ctrl.Log.WithName("VirtualMachine").WithValues("name", vm.NamespacedName()),
		VM:      vm,
	}

	// If the VM has a pause reconcile annotation, it is being restored on vCenter. Return here so our reconcile
	// does not replace the VM being restored on the vCenter inventory.
	//
	// Do not requeue the reconcile here since removing the pause annotation will trigger a reconcile anyway.
	if _, ok := vm.Annotations[vmopv1alpha1.PauseAnnotation]; ok {
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
func requeueDelay(ctx *context.VirtualMachineContext) time.Duration {
	// If the VM is in Creating phase, the reconciler has run out of threads to Create VMs on the provider. Do not queue
	// immediately to avoid exponential backoff.
	if ctx.VM.Status.Phase == vmopv1alpha1.Creating {
		return 10 * time.Second
	}

	if ctx.VM.Status.VmIp == "" && ctx.VM.Status.PowerState == vmopv1alpha1.VirtualMachinePoweredOn {
		return 10 * time.Second
	}

	return 0
}

func (r *Reconciler) ReconcileDelete(ctx *context.VirtualMachineContext) (reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachine Deletion")

	if controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		ctx.VM.Status.Phase = vmopv1alpha1.Deleting

		defer func() {
			r.Recorder.EmitEvent(ctx.VM, "Delete", reterr, false)
		}()

		if err := r.VMProvider.DeleteVirtualMachine(ctx, ctx.VM); client.IgnoreNotFound(err) != nil {
			ctx.Logger.Error(err, "Failed to delete VirtualMachine")
			return err
		}

		ctx.VM.Status.Phase = vmopv1alpha1.Deleted
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
func (r *Reconciler) ReconcileNormal(ctx *context.VirtualMachineContext) (reterr error) {
	if !controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		// The finalizer must be present before proceeding in order to ensure that the VM will
		// be cleaned up. Return immediately after here to let the patcher helper update the
		// object, and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.VM, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachine")

	defer func(initialVMStatus *vmopv1alpha1.VirtualMachineStatus) {
		// Log the reconcile time using the CR creation time and the time the VM reached the desired state
		if reterr == nil && !apiequality.Semantic.DeepEqual(initialVMStatus, &ctx.VM.Status) {
			ctx.Logger.Info("Finished Reconciling VirtualMachine with updates to the CR",
				"createdTime", ctx.VM.CreationTimestamp, "currentTime", time.Now().Format(time.RFC3339),
				"spec.PowerState", ctx.VM.Spec.PowerState, "status.PowerState", ctx.VM.Status.PowerState)
		} else {
			ctx.Logger.Info("Finished Reconciling VirtualMachine")
		}

		if ip := initialVMStatus.VmIp; ip == "" && ip != ctx.VM.Status.VmIp {
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

	ctx.VM.Status.Phase = vmopv1alpha1.Created
	// Add this VM to prober manager if ReconcileNormal succeeds.
	r.Prober.AddToProberManager(ctx.VM)

	ctx.Logger.Info("Finished Reconciling VirtualMachine")
	return nil
}
