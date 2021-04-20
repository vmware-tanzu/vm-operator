// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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

	proberManager, err := prober.AddToVirtualMachineController(mgr)
	if err != nil {
		return err
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctx.MaxConcurrentReconciles,
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VmProvider,
		proberManager,
	)

	reqMapper := requestMapper{
		ctx: &requestMapperCtx{
			ControllerManagerContext: ctx,
			Client:                   r.Client,
			Logger:                   r.Logger,
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &vmopv1alpha1.VirtualMachineClassBinding{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: reqMapper}).
		Watches(&source.Kind{Type: &vmopv1alpha1.ContentSourceBinding{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: reqMapper}).
		Complete(r)
}

type requestMapper struct {
	ctx *requestMapperCtx
}

type requestMapperCtx struct {
	*context.ControllerManagerContext
	client.Client
	Logger logr.Logger
}

func (m requestMapper) Map(o handler.MapObject) []reconcile.Request {
	if csBinding, ok := o.Object.(*vmopv1alpha1.ContentSourceBinding); ok {
		return allVMsUsingContentSourceBinding(m.ctx, csBinding)
	}

	if vmClassBinding, ok := o.Object.(*vmopv1alpha1.VirtualMachineClassBinding); ok {
		return allVMsUsingVirtualMachineClassBinding(m.ctx, vmClassBinding)
	}

	return nil
}

// allVMsUsingContentSourceBinding is a function that maps a ContentSourceBinding to a list of reconcile request
// for all the VirtualMachines using VirtualMachineImages from the content source pointed by the ContentSourceBinding.
// Assume that only supported type is ContentLibraryProvider.
func allVMsUsingContentSourceBinding(ctx *requestMapperCtx, binding *vmopv1alpha1.ContentSourceBinding) []reconcile.Request {
	logger := ctx.Logger.WithValues("name", binding.Name, "namespace", binding.Namespace)

	logger.V(4).Info("Reconciling all VMs using images from a ContentSource because of a ContentSourceBinding watch")

	contentSource := &vmopv1alpha1.ContentSource{}
	if err := ctx.Client.Get(ctx, client.ObjectKey{Name: binding.ContentSourceRef.Name}, contentSource); err != nil {
		logger.Error(err, "Failed to get ContentSource for VM reconciliation due to ContentSourceBinding watch")
		return nil
	}

	providerRef := contentSource.Spec.ProviderRef
	clProviderFromBinding := vmopv1alpha1.ContentLibraryProvider{}
	if err := ctx.Client.Get(ctx, client.ObjectKey{Name: providerRef.Name}, &clProviderFromBinding); err != nil {
		logger.Error(err, "Failed to get ContentLibraryProvider for VM reconciliation due to ContentSourceBinding watch")
		return nil
	}

	// Filter images that have an OwnerReference to this ContentLibraryProvider.
	imageList := &vmopv1alpha1.VirtualMachineImageList{}
	if err := ctx.Client.List(ctx, imageList); err != nil {
		logger.Error(err, "Failed to get ContentLibraryProvider for VM reconciliation due to ContentSourceBinding watch")
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
	if err := ctx.Client.List(ctx, vmList, client.InNamespace(binding.Namespace)); err != nil {
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

// For a given VirtualMachineClassBinding, return reconcile requests
// for those VirtualMachines with corresponding VirtualMachinesClasses referenced
func allVMsUsingVirtualMachineClassBinding(ctx *requestMapperCtx, classBinding *vmopv1alpha1.VirtualMachineClassBinding) []reconcile.Request {
	logger := ctx.Logger.WithValues("name", classBinding.Name, "namespace", classBinding.Namespace)

	logger.V(4).Info("Reconciling all VMs referencing a VM class because of a VirtualMachineClassBinding watch")

	// Find all vms that match this vmclassbinding
	vmList := &vmopv1alpha1.VirtualMachineList{}
	if err := ctx.Client.List(ctx, vmList, client.InNamespace(classBinding.Namespace)); err != nil {
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

func NewReconciler(
	client client.Client,
	numReconcilers int,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterface,
	prober prober.Manager) *VirtualMachineReconciler {

	// Limit the maximum number of VirtualMachine creates by the provider. Calculated as MAX_CREATE_VMS_ON_PROVIDER
	// (default 80) percent of the total number of reconciler threads.
	maxConcurrentCreateVMsOnProvider := int(math.Ceil((float64(numReconcilers) * float64(lib.MaxConcurrentCreateVMsOnProvider())) / float64(100)))

	return &VirtualMachineReconciler{
		Client:                           client,
		Logger:                           logger,
		Recorder:                         recorder,
		VmProvider:                       vmProvider,
		Prober:                           prober,
		MaxConcurrentCreateVMsOnProvider: maxConcurrentCreateVMsOnProvider,
	}
}

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineReconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VmProvider vmprovider.VirtualMachineProviderInterface
	Prober     prober.Manager

	// Hack to limit concurrent create operations because they block and can take a long time.
	mutex                            sync.Mutex
	NumVMsBeingCreatedOnProvider     int
	MaxConcurrentCreateVMsOnProvider int
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

func (r *VirtualMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := goctx.Background()

	vm := &vmopv1alpha1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmCtx := &context.VirtualMachineContext{
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

func (r *VirtualMachineReconciler) deleteVm(ctx *context.VirtualMachineContext) (err error) {
	defer func() {
		r.Recorder.EmitEvent(ctx.VM, "Delete", err, false)
	}()

	err = r.VmProvider.DeleteVirtualMachine(ctx, ctx.VM)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			ctx.Logger.Info("To be deleted VirtualMachine was not found")
			return nil
		}
		ctx.Logger.Error(err, "Failed to delete VirtualMachine")
		return err
	}

	ctx.Logger.V(4).Info("Deleted VirtualMachine")
	return nil
}

func (r *VirtualMachineReconciler) ReconcileDelete(ctx *context.VirtualMachineContext) error {
	vm := ctx.VM

	ctx.Logger.Info("Reconciling VirtualMachine Deletion")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachine Deletion")
	}()

	if controllerutil.ContainsFinalizer(vm, finalizerName) {
		vm.Status.Phase = vmopv1alpha1.Deleting

		if err := r.deleteVm(ctx); err != nil {
			return err
		}

		vm.Status.Phase = vmopv1alpha1.Deleted
		controllerutil.RemoveFinalizer(vm, finalizerName)
	}

	// Remove the VM from prober manager if ReconcileDelete succeeds.
	r.Prober.RemoveFromProberManager(vm)

	return nil
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *VirtualMachineReconciler) ReconcileNormal(ctx *context.VirtualMachineContext) error {
	if !controllerutil.ContainsFinalizer(ctx.VM, finalizerName) {
		// The finalizer must be present before proceeding in order to ensure that the VM will
		// be cleaned up. Return immediately after here to let the patcher helper update the
		// object, and then we'll proceed on the next reconciliation.
		controllerutil.AddFinalizer(ctx.VM, finalizerName)
		return nil
	}

	ctx.Logger.Info("Reconciling VirtualMachine")
	defer func() {
		ctx.Logger.Info("Finished Reconciling VirtualMachine")
	}()

	if err := r.createOrUpdateVm(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile VirtualMachine")
		return err
	}

	// Add this VM to prober manager if ReconcileNormal succeeds.
	r.Prober.AddToProberManager(ctx.VM)

	return nil
}

func (r *VirtualMachineReconciler) getStoragePolicyID(ctx *context.VirtualMachineContext) (string, error) {
	scName := ctx.VM.Spec.StorageClass
	if scName == "" {
		return "", nil
	}

	sc := &storagev1.StorageClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		ctx.Logger.Error(err, "Failed to get StorageClass", "storageClass", scName)
		return "", err
	}

	return sc.Parameters["storagePolicyID"], nil
}

func (r *VirtualMachineReconciler) getContentLibraryProviderFromImage(ctx *context.VirtualMachineContext, image *vmopv1alpha1.VirtualMachineImage) (*vmopv1alpha1.ContentLibraryProvider, error) {
	for _, ownerRef := range image.OwnerReferences {
		if ownerRef.Kind == "ContentLibraryProvider" {
			clProvider := &vmopv1alpha1.ContentLibraryProvider{}
			if err := r.Get(ctx, client.ObjectKey{Name: ownerRef.Name}, clProvider); err != nil {
				ctx.Logger.Error(err, "error retrieving the ContentLibraryProvider from the API server", "clProviderName", ownerRef.Name)
				return nil, err
			}

			return clProvider, nil
		}
	}

	return nil, fmt.Errorf("VirtualMachineImage does not have an OwnerReference to the ContentLibraryProvider. imageName: %v", image.Name)
}

func (r *VirtualMachineReconciler) getContentSourceFromCLProvider(ctx *context.VirtualMachineContext, clProvider *vmopv1alpha1.ContentLibraryProvider) (*vmopv1alpha1.ContentSource, error) {
	for _, ownerRef := range clProvider.OwnerReferences {
		if ownerRef.Kind == "ContentSource" {
			cs := &vmopv1alpha1.ContentSource{}
			if err := r.Get(ctx, client.ObjectKey{Name: ownerRef.Name}, cs); err != nil {
				ctx.Logger.Error(err, "error retrieving the ContentSource from the API server", "contentSource", ownerRef.Name)
				return nil, err
			}

			return cs, nil
		}
	}

	return nil, fmt.Errorf("ContentLibraryProvider does not have an OwnerReference to the ContentSource. clProviderName: %v", clProvider.Name)
}

// getImageAndContentLibraryUUID fetches the VMImage content library UUID from the VM's image.
// This is done by checking the OwnerReference of the VirtualMachineImage resource. As a side effect, with VM service FSS,
// we also check if the VM's namespace has access to the VirtualMachineImage specified in the Spec. This is done by checking
// if a ContentSourceBinding existing in the namespace that points to the ContentSource corresponding to the specified image.
func (r *VirtualMachineReconciler) getImageAndContentLibraryUUID(ctx *context.VirtualMachineContext) (*vmopv1alpha1.VirtualMachineImage, string, error) {
	imageName := ctx.VM.Spec.ImageName

	vmImage := &vmopv1alpha1.VirtualMachineImage{}
	if err := r.Get(ctx, client.ObjectKey{Name: imageName}, vmImage); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineImage %s: %s", ctx.VM.Spec.ImageName, err)
		conditions.MarkFalse(ctx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineImageNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)

		ctx.Logger.Error(err, "Failed to get VirtualMachineImage", "imageName", imageName)
		return nil, "", err
	}

	clProvider, err := r.getContentLibraryProviderFromImage(ctx, vmImage)
	if err != nil {
		return nil, "", err
	}

	clUUID := clProvider.Spec.UUID

	// With VM Service, we only allow deploying a VM from an image that a developer's namespace has access to.
	if lib.IsVMServiceFSSEnabled() {
		contentSource, err := r.getContentSourceFromCLProvider(ctx, clProvider)
		if err != nil {
			return nil, "", err
		}

		csBindingList := &vmopv1alpha1.ContentSourceBindingList{}
		if err := r.List(ctx, csBindingList, client.InNamespace(ctx.VM.Namespace)); err != nil {
			msg := fmt.Sprintf("Failed to list ContentSourceBindings in namespace: %s", ctx.VM.Namespace)
			conditions.MarkFalse(ctx.VM,
				vmopv1alpha1.VirtualMachinePrereqReadyCondition,
				vmopv1alpha1.ContentSourceBindingNotFoundReason,
				vmopv1alpha1.ConditionSeverityError,
				msg)
			ctx.Logger.Error(err, msg)
			return nil, "", errors.Wrap(err, msg)
		}

		// Filter the bindings for the specified VM Image.
		matchingContentSourceBinding := false
		for _, csBinding := range csBindingList.Items {
			if csBinding.ContentSourceRef.Kind == "ContentSource" && csBinding.ContentSourceRef.Name == contentSource.Name {
				matchingContentSourceBinding = true
				break
			}
		}

		if !matchingContentSourceBinding {
			msg := fmt.Sprintf("Namespace does not have access to VirtualMachineImage. imageName: %v, contentLibraryUUID: %v, namespace: %v",
				ctx.VM.Spec.ImageName, clUUID, ctx.VM.Namespace)
			conditions.MarkFalse(ctx.VM,
				vmopv1alpha1.VirtualMachinePrereqReadyCondition,
				vmopv1alpha1.ContentSourceBindingNotFoundReason,
				vmopv1alpha1.ConditionSeverityError,
				msg)
			ctx.Logger.Error(nil, msg)
			return nil, "", fmt.Errorf(msg)
		}
	}

	return vmImage, clUUID, nil
}

// getVMClass checks if a VM class specified by a VM spec is valid. When the VMServiceFSSEnabled is enabled,
// a valid VM Class binding for the class in the VM's namespace must exist.
func (r *VirtualMachineReconciler) getVMClass(ctx *context.VirtualMachineContext) (*vmopv1alpha1.VirtualMachineClass, error) {
	className := ctx.VM.Spec.ClassName

	vmClass := &vmopv1alpha1.VirtualMachineClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: className}, vmClass); err != nil {
		msg := fmt.Sprintf("Failed to get VirtualMachineClass %s: %s", ctx.VM.Spec.ClassName, err)
		conditions.MarkFalse(ctx.VM,
			vmopv1alpha1.VirtualMachinePrereqReadyCondition,
			vmopv1alpha1.VirtualMachineClassNotFoundReason,
			vmopv1alpha1.ConditionSeverityError,
			msg)
		ctx.Logger.Error(err, "Failed to get VirtualMachineClass", "className", className)
		return nil, err
	}

	if lib.IsVMServiceFSSEnabled() {
		classBindingList := &vmopv1alpha1.VirtualMachineClassBindingList{}
		if err := r.List(ctx, classBindingList, client.InNamespace(ctx.VM.Namespace)); err != nil {
			msg := fmt.Sprintf("Failed to list VirtualMachineClassBindings in namespace: %s", ctx.VM.Namespace)
			conditions.MarkFalse(ctx.VM,
				vmopv1alpha1.VirtualMachinePrereqReadyCondition,
				vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
				vmopv1alpha1.ConditionSeverityError,
				msg)

			return nil, errors.Wrap(err, msg)
		}

		// Filter the bindings for the specified VM class.
		matchingClassBinding := false
		for _, classBinding := range classBindingList.Items {
			if classBinding.ClassRef.Kind == "VirtualMachineClass" && classBinding.ClassRef.Name == className {
				matchingClassBinding = true
				break
			}
		}

		if !matchingClassBinding {
			msg := fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %v, namespace: %v", ctx.VM.Spec.ClassName, ctx.VM.Namespace)
			conditions.MarkFalse(ctx.VM,
				vmopv1alpha1.VirtualMachinePrereqReadyCondition,
				vmopv1alpha1.VirtualMachineClassBindingNotFoundReason,
				vmopv1alpha1.ConditionSeverityError,
				msg)

			return nil, fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", className, ctx.VM.Namespace)
		}
	}

	return vmClass, nil
}

func (r *VirtualMachineReconciler) getVMMetadata(ctx *context.VirtualMachineContext) (*vmprovider.VmMetadata, error) {
	inMetadata := ctx.VM.Spec.VmMetadata
	if inMetadata == nil {
		return nil, nil
	}

	vmMetadataConfigMap := &v1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: inMetadata.ConfigMapName, Namespace: ctx.VM.Namespace}, vmMetadataConfigMap)
	if err != nil {
		return nil, err
	}

	outMetadata := &vmprovider.VmMetadata{
		Transport: inMetadata.Transport,
		Data:      vmMetadataConfigMap.Data,
	}

	return outMetadata, nil
}

func (r *VirtualMachineReconciler) getResourcePolicy(ctx *context.VirtualMachineContext) (*vmopv1alpha1.VirtualMachineSetResourcePolicy, error) {
	rpName := ctx.VM.Spec.ResourcePolicyName
	if rpName == "" {
		return nil, nil
	}

	resourcePolicy := &vmopv1alpha1.VirtualMachineSetResourcePolicy{}
	err := r.Get(ctx, client.ObjectKey{Name: rpName, Namespace: ctx.VM.Namespace}, resourcePolicy)
	if err != nil {
		ctx.Logger.Error(err, "Failed to get VirtualMachineSetResourcePolicy", "resourcePolicyName", rpName)
		return nil, err
	}

	// Make sure that the corresponding entities (RP and Folder) are created on the infra provider before
	// reconciling the VM. Requeue if the ResourcePool and Folders are not yet created for this ResourcePolicy.
	rpReady, err := r.VmProvider.DoesVirtualMachineSetResourcePolicyExist(ctx, resourcePolicy)
	if err != nil {
		ctx.Logger.Error(err, "Failed to check if VirtualMachineSetResourcePolicy exists")
		return nil, err
	}
	if !rpReady {
		return nil, fmt.Errorf("VirtualMachineSetResourcePolicy is not yet ready")
	}

	return resourcePolicy, nil
}

// createOrUpdateVm calls into the VM provider to reconcile a VirtualMachine
func (r *VirtualMachineReconciler) createOrUpdateVm(ctx *context.VirtualMachineContext) error {
	vmClass, err := r.getVMClass(ctx)
	if err != nil {
		return err
	}

	vmImage, clUUID, err := r.getImageAndContentLibraryUUID(ctx)
	if err != nil {
		return err
	}

	vmMetadata, err := r.getVMMetadata(ctx)
	if err != nil {
		return err
	}

	resourcePolicy, err := r.getResourcePolicy(ctx)
	if err != nil {
		return err
	}

	storagePolicyID, err := r.getStoragePolicyID(ctx)
	if err != nil {
		return err
	}

	// Update VirtualMachine conditions to indicate all prereqs have been met.
	conditions.MarkTrue(ctx.VM, vmopv1alpha1.VirtualMachinePrereqReadyCondition)

	vm := ctx.VM
	vmConfigArgs := vmprovider.VmConfigArgs{
		VmClass:            *vmClass,
		VmImage:            vmImage,
		VmMetadata:         vmMetadata,
		ResourcePolicy:     resourcePolicy,
		StorageProfileID:   storagePolicyID,
		ContentLibraryUUID: clUUID,
	}

	exists, err := r.VmProvider.DoesVirtualMachineExist(ctx, vm)
	if err != nil {
		ctx.Logger.Error(err, "Failed to check if VirtualMachine exists from provider")
		return err
	}

	if !exists {
		// Set the phase to Creating first so we do not queue the reconcile immediately if we do not have threads available.
		vm.Status.Phase = vmopv1alpha1.Creating

		// Return and requeue the reconcile request so the provider has reconciler threads available to update the Status of
		// existing VirtualMachines.
		// Ignore overflow since we never expect this to go beyond 32 bits.
		r.mutex.Lock()

		if r.NumVMsBeingCreatedOnProvider >= r.MaxConcurrentCreateVMsOnProvider {
			ctx.Logger.Info("Not enough workers to update VirtualMachine status. Re-queueing the reconcile request")
			// Return nil here so we don't requeue immediately and cause an exponential backoff.
			r.mutex.Unlock()
			return nil
		}

		r.NumVMsBeingCreatedOnProvider += 1
		r.mutex.Unlock()

		defer func() {
			r.mutex.Lock()
			r.NumVMsBeingCreatedOnProvider -= 1
			r.mutex.Unlock()
		}()

		err = r.VmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
		if err != nil {
			ctx.Logger.Error(err, "Provider failed to create VirtualMachine")
			r.Recorder.EmitEvent(vm, "Create", err, false)
			return err
		}
	}

	vm.Status.Phase = vmopv1alpha1.Created

	err = r.VmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		ctx.Logger.Error(err, "Provider failed to update VirtualMachine")
		r.Recorder.EmitEvent(vm, "Update", err, false)
		return err
	}

	return nil
}
