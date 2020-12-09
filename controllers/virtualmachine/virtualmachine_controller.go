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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName                  = "virtualmachine.vmoperator.vmware.com"
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachine{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		mgr.GetClient(),
		ctx.MaxConcurrentReconciles,
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VmProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	numReconcilers int,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider vmprovider.VirtualMachineProviderInterface) *VirtualMachineReconciler {

	// Limit the maximum number of VirtualMachine creates by the provider. Calculated as MAX_CREATE_VMS_ON_PROVIDER
	// (default 80) percent of the total number of reconciler threads.
	maxConcurrentCreateVMsOnProvider := int(math.Ceil((float64(numReconcilers) * float64(lib.MaxConcurrentCreateVMsOnProvider())) / float64(100)))

	return &VirtualMachineReconciler{
		Client:                           client,
		Logger:                           logger,
		Recorder:                         recorder,
		VmProvider:                       vmProvider,
		MaxConcurrentCreateVMsOnProvider: maxConcurrentCreateVMsOnProvider,
	}
}

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineReconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VmProvider vmprovider.VirtualMachineProviderInterface
	// To manipulate the number of VMs being created on provider.
	mutex sync.Mutex
	// Number of VMs being created on the provider.
	NumVMsBeingCreatedOnProvider int
	// Max number of concurrent VM create operations allowed on the provider.
	// TODO: Remove once we have a tuned number from scale testing.
	MaxConcurrentCreateVMsOnProvider int
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=cns.vmware.com,resources=cnsnodevmattachments,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworkinterfaces;virtualnetworkinterfaces/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclassbindings,verbs=get;list;watch

func (r *VirtualMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := goctx.Background()

	vm := &vmopv1alpha1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
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

	return nil
}

func (r *VirtualMachineReconciler) validateStorageClass(ctx *context.VirtualMachineContext, namespace, scName string) error {
	resourceQuotas := &v1.ResourceQuotaList{}
	if err := r.List(ctx, resourceQuotas, client.InNamespace(namespace)); err != nil {
		ctx.Logger.Error(err, "Failed to list ResourceQuotas in namespace", "namespace", namespace)
		return err
	}

	if len(resourceQuotas.Items) == 0 {
		return fmt.Errorf("no ResourceQuotas assigned to namespace %s", namespace)
	}

	prefix := scName + storageResourceQuotaStrPattern
	for _, resourceQuota := range resourceQuotas.Items {
		for resourceName := range resourceQuota.Spec.Hard {
			if strings.HasPrefix(resourceName.String(), prefix) {
				return nil
			}
		}
	}

	return fmt.Errorf("StorageClass %s is not assigned to any ResourceQuotas in namespace %s", scName, namespace)
}

func (r *VirtualMachineReconciler) getStoragePolicyID(ctx *context.VirtualMachineContext) (string, error) {
	scName := ctx.VM.Spec.StorageClass
	if scName == "" {
		return "", nil
	}

	err := r.validateStorageClass(ctx, ctx.VM.Namespace, scName)
	if err != nil {
		return "", err
	}

	sc := &storagev1.StorageClass{}
	err = r.Get(ctx, client.ObjectKey{Name: scName}, sc)
	if err != nil {
		ctx.Logger.Error(err, "Failed to get StorageClass", "storageClass", scName)
		return "", err
	}

	return sc.Parameters["storagePolicyID"], nil
}

func (r *VirtualMachineReconciler) GetCLUUID(ctx *context.VirtualMachineContext) (string, error) {
	vmImage := &vmopv1alpha1.VirtualMachineImage{}
	imageName := ctx.VM.Spec.ImageName

	if err := r.Get(ctx, client.ObjectKey{Name: imageName}, vmImage); err != nil {
		ctx.Logger.Error(err, "Failed to get VirtualMachineImage", "imageName", imageName)
		return "", err
	}

	for _, ownerRef := range vmImage.OwnerReferences {
		if ownerRef.Kind == "ContentLibraryProvider" {
			clProvider := &vmopv1alpha1.ContentLibraryProvider{}
			if err := r.Get(ctx, client.ObjectKey{Name: ownerRef.Name}, clProvider); err != nil {
				r.Logger.Error(err, "error retrieving the ContentLibraryProvider from the API server", "clProviderName", ownerRef.Name)
				return "", err
			}

			return clProvider.Spec.UUID, nil
		}
	}

	return "", nil
}

// getVMClass checks if a VM class specified by a VM spec is valid. When the VMServiceFSSEnabled is enabled,
// a valid VM Class binding for the class in the VM's namespace must exist.
func (r *VirtualMachineReconciler) getVMClass(ctx *context.VirtualMachineContext) (*vmopv1alpha1.VirtualMachineClass, error) {
	className := ctx.VM.Spec.ClassName

	if lib.IsVMServiceFSSEnabled() {
		classBindingList := &vmopv1alpha1.VirtualMachineClassBindingList{}
		if err := r.List(ctx, classBindingList, client.InNamespace(ctx.VM.Namespace)); err != nil {
			ctx.Logger.Error(err, "Failed to list VirtualMachineClassBindings")
			return nil, err
		}

		if len(classBindingList.Items) == 0 {
			return nil, fmt.Errorf("no VirtualMachineClassBindings exist in namespace %s", ctx.VM.Namespace)
		}

		// Filter the bindings for the specified VM class.
		var matchingClassBinding bool
		for _, classBinding := range classBindingList.Items {
			if classBinding.ClassRef.Kind == "VirtualMachineClass" && classBinding.ClassRef.Name == className {
				matchingClassBinding = true
				break
			}
		}

		if !matchingClassBinding {
			return nil, fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s",
				className, ctx.VM.Namespace)
		}
	}

	vmClass := &vmopv1alpha1.VirtualMachineClass{}
	err := r.Get(ctx, client.ObjectKey{Name: className}, vmClass)
	if err != nil {
		ctx.Logger.Error(err, "Failed to get VirtualMachineClass for VirtualMachine",
			"className", className)
		return nil, err
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

	clUUID, err := r.GetCLUUID(ctx)
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

	vm := ctx.VM
	vmConfigArgs := vmprovider.VmConfigArgs{
		VmClass:            *vmClass,
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

	// At this point, the VirtualMachine is either created, or it already existed. Call into the provider for update.
	vm.Status.Phase = vmopv1alpha1.Created
	//pkg.AddAnnotations(&vm.ObjectMeta) // BMV: Not actually saved.

	err = r.VmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		ctx.Logger.Error(err, "Provider failed to update VirtualMachine")
		r.Recorder.EmitEvent(vm, "Update", err, false)
		return err
	}

	return nil
}
