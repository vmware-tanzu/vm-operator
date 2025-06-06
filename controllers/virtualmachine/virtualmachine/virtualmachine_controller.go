// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/metrics"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/diskpromo"
)

const (
	deprecatedFinalizerName = "virtualmachine.vmoperator.vmware.com"
	finalizerName           = "vmoperator.vmware.com/virtualmachine"

	// vmClassControllerName is the name of the controller specified in a
	// VirtualMachineClass resource's field spec.controllerName field that
	// indicates a VM pointing to that VM Class should be reconciled by this
	// controller.
	vmClassControllerName = "vmoperator.vmware.com/vsphere"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.MaxConcurrentReconciles,
			SkipNameValidation:      SkipNameValidation,
		})

	builder = builder.Watches(&vmopv1.VirtualMachineClass{},
		handler.EnqueueRequestsFromMapFunc(classToVMMapperFn(ctx, r.Client, isDefaultVMClassController)))

	if pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
		builder = builder.Watches(
			&byokv1.EncryptionClass{},
			handler.EnqueueRequestsFromMapFunc(
				vmopv1util.EncryptionClassToVirtualMachineMapper(ctx, r.Client),
			))
	}

	if pkgcfg.FromContext(ctx).AsyncSignalEnabled {
		builder = builder.WatchesRawSource(source.Channel(
			cource.FromContextWithBuffer(ctx, "VirtualMachine", 100),
			&handler.EnqueueRequestForObject{}))
	}

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		builder = builder.Watches(
			&vmopv1.VirtualMachineImageCache{},
			handler.EnqueueRequestsFromMapFunc(
				vmopv1util.VirtualMachineImageCacheToItemMapper(
					ctx,
					r.Logger.WithName("VirtualMachineImageCacheToItemMapper"),
					r.Client,
					vmopv1.GroupVersion,
					controlledTypeName,
				),
			),
		)
	}

	return builder.Complete(r)
}

// classToVMMapperFn returns a mapper function that can be used to queue reconcile request
// for the VirtualMachines in response to an event on the VirtualMachineClass resource when
// WCP_Namespaced_VM_Class FSS is enabled.
func classToVMMapperFn(
	ctx *pkgctx.ControllerManagerContext,
	c client.Client,
	isDefaultVMClassController bool) func(_ context.Context, o client.Object) []reconcile.Request {

	// For a given VirtualMachineClass, return reconcile requests
	// for those VirtualMachines with corresponding VirtualMachinesClasses referenced
	return func(_ context.Context, o client.Object) []reconcile.Request {
		class := o.(*vmopv1.VirtualMachineClass)
		logger := ctx.Logger.WithValues("name", class.Name, "namespace", class.Namespace)

		// Only watch resources that reference a VirtualMachineClass with its
		// field spec.controllerName equal to controllerName or if the field is
		// empty and isDefaultVMClassController is true.
		if controllerName := class.Spec.ControllerName; controllerName == "" {
			if !isDefaultVMClassController {
				// Log at a high-level so the logs are not over-run.
				logger.V(8).Info(
					"Skipping class with empty controller name & not default VM Class controller",
					"defaultVMClassController", pkgcfg.FromContext(ctx).DefaultVMClassControllerName,
					"expectedControllerName", vmClassControllerName)
				return nil
			}
		} else if controllerName != vmClassControllerName {
			// Log at a high-level so the logs are not over-run.
			logger.V(8).Info(
				"Skipping class with mismatched controller name",
				"actualControllerName", controllerName,
				"expectedControllerName", vmClassControllerName)
			return nil
		}

		logger.V(4).Info("Reconciling all VMs referencing a VM class because of a VirtualMachineClass watch")

		// Find all VM resources that reference this VM Class.
		// TODO: May need an index or to use pagination.
		vmList := &vmopv1.VirtualMachineList{}
		if err := c.List(ctx, vmList, client.InNamespace(class.Namespace)); err != nil {
			logger.Error(err, "Failed to list VirtualMachines for reconciliation due to VirtualMachineClass watch")
			return nil
		}

		// Populate reconcile requests for VMs that reference this VM Class.
		var reconcileRequests []reconcile.Request
		for _, vm := range vmList.Items {
			if vm.Spec.ClassName == class.Name {
				// TODO: We could be smarter here on what VMs actually need to be reconciled.
				key := client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}
				reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
			}
		}

		if len(reconcileRequests) > 0 {
			logger.V(4).Info("Returning VM reconcile requests due to VirtualMachineClass watch", "requests", reconcileRequests)
		}
		return reconcileRequests
	}
}

func upgradeSchema(ctx *pkgctx.VirtualMachineContext) {
	// If empty, this VM was created before v1alpha3 added the spec.instanceUUID field.
	if ctx.VM.Spec.InstanceUUID == "" && ctx.VM.Status.InstanceUUID != "" {
		ctx.VM.Spec.InstanceUUID = ctx.VM.Status.InstanceUUID
		ctx.Logger.Info("Upgrade VirtualMachine spec",
			"instanceUUID", ctx.VM.Spec.InstanceUUID)
	}

	// If empty, this VM was created before v1alpha3 added the spec.biosUUID field.
	if ctx.VM.Spec.BiosUUID == "" && ctx.VM.Status.BiosUUID != "" {
		ctx.VM.Spec.BiosUUID = ctx.VM.Status.BiosUUID
		ctx.Logger.Info("Upgrade VirtualMachine spec",
			"biosUUID", ctx.VM.Spec.BiosUUID)
	}

	// If empty, this VM was created before v1alpha3 added the instanceID field
	if bs := ctx.VM.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			if ci.InstanceID == "" {
				iid := vmlifecycle.BootStrapCloudInitInstanceID(*ctx, ci)
				ctx.Logger.Info("Upgrade VirtualMachine spec",
					"bootstrap.cloudInit.instanceID", iid)
			}
		}
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
// +kubebuilder:rbac:groups=netoperator.vmware.com,resources=networkinterfaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyquotas,verbs=get;list;watch
// +kubebuilder:rbac:groups=crd.nsx.vmware.com,resources=subnetports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.nsx.vmware.com,resources=subnetports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=encryption.vmware.com,resources=encryptionclasses,verbs=get;list;watch

// Reconcile the object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = cource.JoinContext(ctx, r.Context)
	ctx = vmconfig.WithContext(ctx)

	if pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
		ctx = vmconfig.Register(ctx, crypto.New())
	}

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		ctx = vmconfig.Register(ctx, diskpromo.New())
	}

	ctx = ctxop.WithContext(ctx)
	ctx = ovfcache.JoinContext(ctx, r.Context)
	ctx = record.WithContext(ctx, r.Recorder)

	vm := &vmopv1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger := ctrl.Log.WithName("VirtualMachine").WithValues("name", vm.NamespacedName())

	if pkgcfg.FromContext(ctx).Features.FastDeploy {

		// Check to see if the VM's annotations specify a fast-deploy mode.
		mode := vm.Annotations[pkgconst.FastDeployAnnotationKey]

		if mode == "" {
			// If the VM's annotation does not specify a fast-deploy mode then
			// get the mode from the global config.
			mode = pkgcfg.FromContext(ctx).FastDeployMode
		}

		switch strings.ToLower(mode) {
		case pkgconst.FastDeployModeDirect, pkgconst.FastDeployModeLinked:
			logger.V(4).Info("Using fast-deploy for this VM", "mode", mode)
			// No-op
		default:
			// Create a copy of the config where the Fast Deploy feature is
			// disabled for this call-stack.
			cfg := pkgcfg.FromContext(ctx)
			cfg.Features.FastDeploy = false
			ctx = pkgcfg.WithContext(ctx, cfg)
			logger.Info("Disabled fast-deploy for this VM")
		}
	}

	// Update the type of placement logic based on whether or not the VM is a
	// Kubernetes node and the WorkloadDomainIsolation capability.
	ctx = vmopv1util.GetContextWithWorkloadDomainIsolation(ctx, *vm)
	if !pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		logger.Info("Disabled WorkloadDomainIsolation capability for this VM")
	}

	vmCtx := &pkgctx.VirtualMachineContext{
		Context: ctx,
		Logger:  logger,
		VM:      vm,
	}

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmCtx, err)
	}

	defer func() {
		vmopv1util.SyncStorageUsageForNamespace(
			ctx,
			vm.Namespace,
			vm.Spec.StorageClass)
		if err := patchHelper.Patch(ctx, vm); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vm.DeletionTimestamp.IsZero() {
		return pkgerr.ResultFromError(r.ReconcileDelete(vmCtx))
	}

	if err := r.ReconcileNormal(vmCtx); err != nil && !ignoredCreateErr(err) {
		return pkgerr.ResultFromError(err)
	}

	// Requeue after N amount of time according to the state of the VM.
	return ctrl.Result{RequeueAfter: requeueDelay(vmCtx, err)}, nil
}

// Determine if we should request a non-zero requeue delay in order to trigger a
// non-rate limited reconcile at some point in the future.
//
// When async signal is disabled, this is used to trigger a specific reconcile
// to discovery the VM IP address rather than relying on the resync period to
// do.
//
// TODO
// It would be much preferable to determine that a non-error resync is required
// at the source of the determination that the VM IP is not available rather
// than up here in the reconcile loop. However, in the interest of time, we are
// making this determination here and will have to refactor at some later date.
func requeueDelay(
	ctx *pkgctx.VirtualMachineContext,
	err error) time.Duration {

	// Do not requeue if the VMI cache object is not ready. Instead allow the
	// watcher to trigger the reconcile when the cache object is updated.
	if ctx.VM.Labels[pkgconst.VMICacheLabelKey] != "" {
		return 0
	}

	// If there were too many concurrent create operations or if the VM is in
	// Creating phase, the reconciler has run out of threads or goroutines to
	// Create VMs on the provider. Do not queue immediately to avoid exponential
	// backoff.
	if ignoredCreateErr(err) ||
		!conditions.IsTrue(ctx.VM, vmopv1.VirtualMachineConditionCreated) {

		return pkgcfg.FromContext(ctx).CreateVMRequeueDelay
	}

	// Do not requeue for the IP address if async signal is enabled.
	if pkgcfg.FromContext(ctx).AsyncSignalEnabled {
		return 0
	}

	if ctx.VM.Status.PowerState == vmopv1.VirtualMachinePowerStateOn {
		networkSpec := ctx.VM.Spec.Network
		if networkSpec != nil && !networkSpec.Disabled {
			networkStatus := ctx.VM.Status.Network
			if networkStatus == nil || (networkStatus.PrimaryIP4 == "" && networkStatus.PrimaryIP6 == "") {
				return pkgcfg.FromContext(ctx).PoweredOnVMHasIPRequeueDelay
			}
		}
	}

	return 0
}

func (r *Reconciler) ReconcileDelete(ctx *pkgctx.VirtualMachineContext) (reterr error) {
	ctx.Logger.Info("Reconciling VirtualMachine Deletion")

	skipDelete := false
	for k, v := range ctx.VM.Annotations {
		switch {
		case k == vmopv1.PauseAnnotation,
			strings.HasPrefix(k, vmopv1.CheckAnnotationDelete+"/"):

			skipDelete = true
			ctx.Logger.Info(
				"Skipping delete due to annotation",
				"annotationKey", k, "annotationValue", v)
		}
	}
	if skipDelete {
		return nil
	}

	if controllerutil.ContainsFinalizer(ctx.VM, finalizerName) ||
		controllerutil.ContainsFinalizer(ctx.VM, deprecatedFinalizerName) {

		_, skip := ctx.VM.Annotations[pkgconst.SkipDeletePlatformResourceKey]
		if skip {
			ctx.Logger.Info("Skipping deletion of underlying VM",
				"reason", pkgconst.SkipDeletePlatformResourceKey,
				"managedObjectID", ctx.VM.Status.UniqueID,
				"biosUUID", ctx.VM.Status.BiosUUID,
				"instanceUUID", ctx.VM.Status.InstanceUUID)
		} else {
			defer func() {
				r.Recorder.EmitEvent(ctx.VM, "Delete", reterr, false)
			}()

			if err := r.VMProvider.DeleteVirtualMachine(ctx, ctx.VM); err != nil {
				return err
			}
		}

		controllerutil.RemoveFinalizer(ctx.VM, finalizerName)
		controllerutil.RemoveFinalizer(ctx.VM, deprecatedFinalizerName)
	}

	// BMV: Shouldn't these be in the ContainsFinalizer block?
	r.vmMetrics.DeleteMetrics(ctx)
	r.Prober.RemoveFromProberManager(ctx.VM)

	ctx.Logger.Info("Finished Reconciling VirtualMachine Deletion")
	return nil
}

// ReconcileNormal processes a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *Reconciler) ReconcileNormal(ctx *pkgctx.VirtualMachineContext) (reterr error) {
	// Return early if the VM reconciliation is paused.
	if _, exists := ctx.VM.Annotations[vmopv1.PauseAnnotation]; exists {
		ctx.Logger.Info("Skipping reconciliation since VirtualMachine contains the pause annotation")
		return nil
	}

	if !controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {

		// If the object has the deprecated finalizer, remove it.
		if updated := controllerutil.RemoveFinalizer(ctx.VM, deprecatedFinalizerName); updated {
			ctx.Logger.V(5).Info("Removed deprecated finalizer", "finalizerName", deprecatedFinalizerName)
		}

		// The finalizer must be present before proceeding in order to ensure that the VM will
		// be cleaned up. Return immediately after here to let the patcher helper update the
		// object, and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.VM, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachine")

	defer func(beforeVMStatus *vmopv1.VirtualMachineStatus) {
		if pkgerr.IsNoRequeueError(reterr) {
			ctx.Logger.V(4).Info(
				"vm will be re-reconciled by async watcher",
				"result", reterr.Error())
		}
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

	// Upgrade schema fields where needed
	upgradeSchema(ctx)

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		// Do not proceed unless the VMI cache this VM needs is ready.
		if !r.isVMICacheReady(ctx) {
			return nil
		}
	}

	var (
		err     error
		chanErr <-chan error
	)

	if pkgcfg.FromContext(ctx).AsyncSignalEnabled &&
		pkgcfg.FromContext(ctx).AsyncCreateEnabled {
		//
		// Non-blocking create
		//
		chanErr, err = r.VMProvider.CreateOrUpdateVirtualMachineAsync(ctx, ctx.VM)
	} else {
		//
		// Blocking create
		//
		err = r.VMProvider.CreateOrUpdateVirtualMachine(ctx, ctx.VM)
	}

	switch {
	case ctxop.IsCreate(ctx) && !ignoredCreateErr(err):

		if chanErr == nil {
			//
			// Blocking create
			//
			err = r.handleBlockingCreateErr(ctx, err)
		} else {
			//
			// Non-blocking create
			//
			if err != nil {
				// Failed before goroutine, such as when getting the pre-reqs
				// to do a create, i.e. createArgs.
				err = r.handleBlockingCreateErr(ctx, err)
			} else {
				go r.handleNonBlockingCreateErr(ctx.VM.DeepCopy(), chanErr)
			}
		}
	case ctxop.IsUpdate(ctx):

		r.Recorder.EmitEvent(
			ctx.VM, "Update", filterNoRequeueErr(err), false)

	case err != nil && !ignoredCreateErr(err):

		// Catch all event for neither create nor update op.
		r.Recorder.EmitEvent(
			ctx.VM, "ReconcileNormal", filterNoRequeueErr(err), true)
	}

	if !pkgcfg.FromContext(ctx).AsyncSignalEnabled {

		// Add the VM to the probe manager. This is idempotent.
		r.Prober.AddToProberManager(ctx.VM)

	} else if p := ctx.VM.Spec.ReadinessProbe; p != nil && p.TCPSocket != nil {
		// TCP probes still use the probe manager.
		r.Prober.AddToProberManager(ctx.VM)
	} else {
		// Remove the probe in case it *was* a TCP probe but switched to one
		// of the other types.
		r.Prober.RemoveFromProberManager(ctx.VM)
	}

	return err
}

func getIsDefaultVMClassController(ctx context.Context) bool {
	if v := pkgcfg.FromContext(ctx).DefaultVMClassControllerName; v == "" || v == vmClassControllerName {
		return true
	}
	return false
}

// ignoredCreateErr is written this way in order to illustrate coverage more
// accurately.
func ignoredCreateErr(err error) bool {
	if errors.Is(err, providers.ErrReconcileInProgress) {
		return true
	}
	if errors.Is(err, providers.ErrTooManyCreates) {
		return true
	}
	return false
}

func (r *Reconciler) handleBlockingCreateErr(
	ctx *pkgctx.VirtualMachineContext, err error) error {

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		if pkgerr.WatchVMICacheIfNotReady(err, ctx.VM) {
			// If the cache is not yet ready then do not return an error,
			// but simultaneously, do not reflect a successful create. The
			// VM will be requeued once the cache object is ready.
			return nil
		}
	}

	r.Recorder.EmitEvent(ctx.VM, "Create", filterNoRequeueErr(err), false)
	return err
}

func (r *Reconciler) handleNonBlockingCreateErr(
	obj *vmopv1.VirtualMachine, chanErr <-chan error) {

	// Emit event once channel is closed.
	failed := false
	for err := range chanErr {
		if err != nil &&
			!errors.Is(err, vsphere.ErrCreate) &&
			!pkgerr.IsNoRequeueError(err) {

			failed = true
			r.Recorder.EmitEvent(obj, "Create", err, false)
		}
	}
	if !failed {
		// If no error the channel is just closed.
		r.Recorder.EmitEvent(obj, "Create", nil, false)
	}
}

func (r *Reconciler) isVMICacheReady(ctx *pkgctx.VirtualMachineContext) bool {
	vmicName := ctx.VM.Labels[pkgconst.VMICacheLabelKey]
	if vmicName == "" {
		// The object is not waiting on a VMI cache object and can proceed.
		return true
	}

	var (
		vmic    vmopv1.VirtualMachineImageCache
		vmicKey = client.ObjectKey{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      vmicName,
		}
	)

	if err := r.Client.Get(ctx, vmicKey, &vmic); err != nil {
		ctx.Logger.Error(
			err,
			"Skipping due to failure getting vmicache object")
		return false
	}

	// Assert the OVF is ready.
	if !conditions.IsTrue(
		vmic,
		vmopv1.VirtualMachineImageCacheConditionOVFReady) {

		ctx.Logger.V(4).Info(
			"Skipping due to missing true condition",
			"conditionType", vmopv1.VirtualMachineImageCacheConditionOVFReady)
		return false
	}

	// Check and see if the VM is waiting on the disks yet.
	location := ctx.VM.Annotations[pkgconst.VMICacheLocationAnnotationKey]
	if location == "" {
		// Remove the label the VM that indicate it is waiting on the VMI cache
		// resource to be ready with the OVF.
		delete(ctx.VM.Labels, pkgconst.VMICacheLabelKey)

		return true
	}

	// Find the matching location status.
	var locStatus *vmopv1.VirtualMachineImageCacheLocationStatus
	if locParts := strings.Split(location, ","); len(locParts) == 2 {
		dcID, dsID := locParts[0], locParts[1]
		for i := range vmic.Status.Locations {
			l := vmic.Status.Locations[i]
			if l.DatacenterID == dcID && l.DatastoreID == dsID {
				locStatus = &l
				break
			}
		}
	}

	// Assert the cached disks are ready.
	if locStatus == nil || !conditions.IsTrue(
		locStatus,
		vmopv1.ReadyConditionType) {

		ctx.Logger.V(4).Info(
			"Skipping due to missing true condition",
			"conditionType", vmopv1.VirtualMachineImageCacheConditionFilesReady,
			"locationStatus", locStatus)
		return false
	}

	// Remove the label and annotations from the VM that indicate it is waiting
	// on a VMI cache resource to be ready with the cached disks.
	delete(ctx.VM.Labels, pkgconst.VMICacheLabelKey)
	delete(ctx.VM.Annotations, pkgconst.VMICacheLocationAnnotationKey)

	return true
}

func filterNoRequeueErr(err error) error {
	if pkgerr.IsNoRequeueError(err) {
		return nil
	}
	return err
}
