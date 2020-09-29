// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	v1 "k8s.io/api/core/v1"
	storagetypev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

const (
	finalizerName                  = "virtualmachine.vmoperator.vmware.com"
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
	controllerName                 = "virtualmachine-controller"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(ctx, mgr, NewReconciler(ctx, mgr))
}

func NewReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	var controllerNameLong = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerName)
	return &VirtualMachineReconciler{
		Client:     mgr.GetClient(),
		Logger:     ctrllog.Log.WithName("controllers").WithName(controllerName),
		Recorder:   record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		VmProvider: ctx.VmProvider,
	}
}

func add(ctx *context.ControllerManagerContext, mgr manager.Manager, r reconcile.Reconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vmoperatorv1alpha1.VirtualMachine{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Owns(&cnsv1alpha1.CnsNodeVmAttachment{}).
		Complete(r)
}

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineReconciler struct {
	client.Client
	Logger     logr.Logger
	Recorder   record.Recorder
	VmProvider vmprovider.VirtualMachineProviderInterface
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

func (r *VirtualMachineReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := goctx.Background()

	instance := &vmoperatorv1alpha1.VirtualMachine{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	vmContext := &context.VirtualMachineContext{
		Context: ctx,
		VMObjectKey: client.ObjectKey{
			Namespace: req.Namespace,
			Name:      req.Name,
		},
		VM: instance,
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		err = r.ReconcileDelete(vmContext)
		return ctrl.Result{}, err
	}

	if err := r.ReconcileNormal(vmContext); err != nil {
		r.Logger.Error(err, "Failed to reconcile VirtualMachine", "name", instance.NamespacedName())
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueDelay(ctx, instance)}, nil
}

// Determine if we should request a non-zero requeue delay in order to trigger a non-rate limited reconcile
// at some point in the future.  Use this delay-based reconcile to trigger a specific reconcile to discovery the VM IP
// address rather than relying on the resync period to do.
//
// TODO: It would be much preferable to determine that a non-error resync is required at the source of the determination that
// TODO: the VM IP isn't available rather than up here in the reconcile loop.  However, in the interest of time, we are making
// TODO: this determination here and will have to refactor at some later date.
func requeueDelay(ctx goctx.Context, vm *vmoperatorv1alpha1.VirtualMachine) time.Duration {
	if vm.Status.VmIp == "" && vm.Status.PowerState == vmoperatorv1alpha1.VirtualMachinePoweredOn {
		return 10 * time.Second
	}

	return 0
}

func (r *VirtualMachineReconciler) deleteVm(ctx *context.VirtualMachineContext) (err error) {
	vm := ctx.VM
	defer func() {
		r.Recorder.EmitEvent(vm, "Delete", err, false)
	}()

	err = r.VmProvider.DeleteVirtualMachine(ctx, vm)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			r.Logger.Info("To be deleted VirtualMachine was not found", "name", vm.NamespacedName())
			return nil
		}
		r.Logger.Error(err, "Failed to delete VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	vm.Status.Phase = vmoperatorv1alpha1.Deleted
	r.Logger.V(4).Info("Deleted VirtualMachine", "name", vm.NamespacedName())

	return nil
}

func (r *VirtualMachineReconciler) ReconcileDelete(ctx *context.VirtualMachineContext) error {
	vm := ctx.VM
	r.Logger.Info("Reconciling VirtualMachine Deletion", "name", vm.NamespacedName())
	defer func() {
		r.Logger.Info("Finished Reconciling VirtualMachine Deletion", "name", vm.NamespacedName())
	}()

	if lib.ContainsString(vm.ObjectMeta.Finalizers, finalizerName) {
		if vm.Status.Phase != vmoperatorv1alpha1.Deleting {
			vm.Status.Phase = vmoperatorv1alpha1.Deleting
			if err := r.Update(ctx, vm); err != nil {
				r.Logger.Error(err, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
				return err
			}
		}

		if err := r.deleteVm(ctx); err != nil {
			return err
		}

		vm.ObjectMeta.Finalizers = lib.RemoveString(vm.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, vm); err != nil {
			return err
		}
	}

	return nil
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (r *VirtualMachineReconciler) ReconcileNormal(ctx *context.VirtualMachineContext) error {
	vm := ctx.VM
	r.Logger.Info("Reconciling VirtualMachine", "name", vm.NamespacedName())
	r.Logger.V(4).Info("Original VM Status", "name", vm.NamespacedName(), "status", vm.Status)
	defer func() {
		r.Logger.Info("Finished Reconciling VirtualMachine", "name", vm.NamespacedName())
	}()

	if !lib.ContainsString(vm.ObjectMeta.Finalizers, finalizerName) {
		vm.ObjectMeta.Finalizers = append(vm.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, vm); err != nil {
			return err
		}
	}

	vmCopy := vm.DeepCopy()

	if err := r.createOrUpdateVm(ctx, vm); err != nil {
		r.Logger.Error(err, "Failed to reconcile VirtualMachine")
		return err
	}

	if r.Logger.V(4).Enabled() {
		// Before we update the status, get the current resource and log it
		latestVm := &vmoperatorv1alpha1.VirtualMachine{}
		err := r.Get(ctx, ctx.VMObjectKey, latestVm)
		if err == nil {
			r.Logger.V(4).Info("Latest resource before update", "name", vm.NamespacedName(), "resource", latestVm)
			r.Logger.V(4).Info("Resource to update", "name", vm.NamespacedName(), "resource", vm)
		}
	}

	r.Logger.V(4).Info("Updated VM Status", "name", vm.NamespacedName(), "status", vm.Status)
	if !apiequality.Semantic.DeepEqual(vmCopy.Status, vm.Status) {
		if err := r.Status().Update(ctx, vm); err != nil {
			r.Logger.Error(err, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
			return err
		}
	}

	return nil
}

func (r *VirtualMachineReconciler) validateStorageClass(ctx *context.VirtualMachineContext, namespace, scName string) error {
	resourceQuotas := &v1.ResourceQuotaList{}
	err := r.List(ctx, resourceQuotas, client.InNamespace(namespace))
	if err != nil {
		r.Logger.Error(err, "Failed to list ResourceQuotas", "namespace", namespace)
		return err
	}

	if len(resourceQuotas.Items) == 0 {
		return fmt.Errorf("no ResourceQuotas assigned to namespace '%s'", namespace)
	}

	for _, resourceQuota := range resourceQuotas.Items {
		for resourceName := range resourceQuota.Spec.Hard {
			resourceNameStr := resourceName.String()
			if !strings.Contains(resourceNameStr, storageResourceQuotaStrPattern) {
				continue
			}
			// BMV: Match prefix 'scName + storageResourceQuotaStrPattern'?
			scNameFromRQ := strings.Split(resourceNameStr, storageResourceQuotaStrPattern)[0]
			if scName == scNameFromRQ {
				return nil
			}
		}
	}

	return fmt.Errorf("StorageClass '%s' is not assigned to any ResourceQuotas in namespace '%s'", scName, namespace)
}

func (r *VirtualMachineReconciler) lookupStoragePolicyID(ctx *context.VirtualMachineContext, namespace, storageClassName string) (string, error) {
	err := r.validateStorageClass(ctx, namespace, storageClassName)
	if err != nil {
		return "", err
	}

	sc := &storagetypev1.StorageClass{}
	err = r.Get(ctx, client.ObjectKey{Name: storageClassName}, sc)
	if err != nil {
		r.Logger.Error(err, "Failed to get StorageClass", "storageClassName", storageClassName)
		return "", err
	}

	return sc.Parameters["storagePolicyID"], nil
}

// createOrUpdateVm calls into the VM provider to reconcile a VirtualMachine
func (r *VirtualMachineReconciler) createOrUpdateVm(ctx *context.VirtualMachineContext, vm *vmoperatorv1alpha1.VirtualMachine) error {
	vmClass := &vmoperatorv1alpha1.VirtualMachineClass{}
	err := r.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)
	if err != nil {
		r.Logger.Error(err, "Failed to get VirtualMachineClass for VirtualMachine",
			"vmName", vm.NamespacedName(), "class", vm.Spec.ClassName)
		return err
	}

	vmMetadata, err := r.getVmMetadata(ctx)
	if err != nil {
		return err
	}

	var resourcePolicy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
	if policyName := vm.Spec.ResourcePolicyName; policyName != "" {
		resourcePolicy = &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
		err = r.Get(ctx, client.ObjectKey{Name: policyName, Namespace: vm.Namespace}, resourcePolicy)
		if err != nil {
			r.Logger.Error(err, "Failed to get VirtualMachineSetResourcePolicy",
				"vmName", vm.NamespacedName(), "resourcePolicyName", policyName)
			return err
		}

		// Make sure that the corresponding entities (RP and Folder) are created on the infra provider before
		// reconciling the VM. Requeue if the ResourcePool and Folders are not yet created for this ResourcePolicy.
		rpReady, err := r.VmProvider.DoesVirtualMachineSetResourcePolicyExist(ctx, resourcePolicy)
		if err != nil {
			r.Logger.Error(err, "Failed to check if VirtualMachineSetResourcePolicy exists")
			return err
		}
		if !rpReady {
			return fmt.Errorf("VirtualMachineSetResourcePolicy not actualized yet. Failing VirtualMachine reconcile")
		}
	}

	var storagePolicyID string
	if storageClass := vm.Spec.StorageClass; storageClass != "" {
		storagePolicyID, err = r.lookupStoragePolicyID(ctx, vm.Namespace, storageClass)
		if err != nil {
			return err
		}
	}

	vmConfigArgs := vmprovider.VmConfigArgs{
		VmClass:          *vmClass,
		VmMetadata:       vmMetadata,
		ResourcePolicy:   resourcePolicy,
		StorageProfileID: storagePolicyID,
	}
	exists, err := r.VmProvider.DoesVirtualMachineExist(ctx, vm)
	if err != nil {
		r.Logger.Error(err, "Failed to check if VirtualMachine exists from provider", "name", vm.NamespacedName())
		return err
	}

	if !exists {
		if vm.Status.Phase != vmoperatorv1alpha1.Creating {
			vm.Status.Phase = vmoperatorv1alpha1.Creating
			if err = r.Update(ctx, vm); err != nil {
				r.Logger.Error(err, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
				return err
			}
		}

		err = r.VmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
		if err != nil {
			r.Logger.Error(err, "Provider failed to create VirtualMachine", "name", vm.NamespacedName())
			r.Recorder.EmitEvent(vm, "Create", err, false)
			return err
		}
	}

	// At this point, the VirtualMachine is either created, or it already exists. Call into the provider for update.

	pkg.AddAnnotations(&vm.ObjectMeta)

	err = r.VmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		r.Logger.Error(err, "Provider failed to update VirtualMachine", "name", vm.NamespacedName())
		r.Recorder.EmitEvent(vm, "Update", err, false)
		return err
	}

	return nil
}

func (r *VirtualMachineReconciler) getVmMetadata(ctx *context.VirtualMachineContext) (*vmprovider.VmMetadata, error) {
	vm := ctx.VM
	inMetadata := vm.Spec.VmMetadata
	if inMetadata == nil {
		return nil, nil
	}

	outMetadata := vmprovider.VmMetadata{
		Transport: inMetadata.Transport,
	}

	vmMetadataConfigMap := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: inMetadata.ConfigMapName, Namespace: vm.Namespace}, vmMetadataConfigMap)
	if err != nil {
		return nil, err
	}

	outMetadata.Data = make(map[string]string)
	for k, v := range vmMetadataConfigMap.Data {
		outMetadata.Data[k] = v
	}

	return &outMetadata, nil
}
