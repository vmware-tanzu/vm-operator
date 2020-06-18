// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	storagetypev1 "k8s.io/api/storage/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	return add(ctx, mgr, newReconciler(ctx, mgr))
}

func newReconciler(ctx *context.ControllerManagerContext, mgr manager.Manager) reconcile.Reconciler {
	return &VirtualMachineReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("VirtualMachine"),
		recorder:   record.New(mgr.GetEventRecorderFor("virtualmachine")),
		vmProvider: ctx.VmProvider,
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
	Log        logr.Logger
	recorder   record.Recorder
	vmProvider vmprovider.VirtualMachineProviderInterface
}

const (
	finalizerName                  = "virtualmachine.vmoperator.vmware.com"
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
)

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

	// Use whatever clusterID is returned, even the empty string.
	clusterID, _ := r.vmProvider.GetClusterID(ctx, req.Namespace)
	ctx = goctx.WithValue(ctx, vimtypes.ID{}, fmt.Sprintf("vmoperator-vmctrl-%s-%s", clusterID, instance.Name))

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		err = r.reconcileDelete(ctx, instance)
		return ctrl.Result{}, err
	}

	if err := r.reconcileNormal(ctx, instance); err != nil {
		r.Log.Error(err, "Failed to reconcile VirtualMachine", "name", instance.NamespacedName())
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

func (r *VirtualMachineReconciler) deleteVm(ctx goctx.Context, vm *vmoperatorv1alpha1.VirtualMachine) (err error) {
	defer func() {
		r.recorder.EmitEvent(vm, "Delete", err, false)
	}()

	err = r.vmProvider.DeleteVirtualMachine(ctx, vm)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			r.Log.Info("To be deleted VirtualMachine was not found", "name", vm.NamespacedName())
			return nil
		}
		r.Log.Error(err, "Failed to delete VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	vm.Status.Phase = vmoperatorv1alpha1.Deleted
	r.Log.V(4).Info("Deleted VirtualMachine", "name", vm.NamespacedName())

	return nil
}

func (r *VirtualMachineReconciler) reconcileDelete(ctx goctx.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	r.Log.Info("Reconciling VirtualMachine Deletion", "name", vm.NamespacedName())
	defer func() {
		r.Log.Info("Finished Reconciling VirtualMachine Deletion", "name", vm.NamespacedName())
	}()

	if lib.ContainsString(vm.ObjectMeta.Finalizers, finalizerName) {
		if err := r.deleteVm(ctx, vm); err != nil {
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
func (r *VirtualMachineReconciler) reconcileNormal(ctx goctx.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	r.Log.Info("Reconciling VirtualMachine", "name", vm.NamespacedName())
	r.Log.V(4).Info("Original VM Status", "name", vm.NamespacedName(), "status", vm.Status)
	defer func() {
		r.Log.Info("Finished Reconciling VirtualMachine", "name", vm.NamespacedName())
	}()

	if !lib.ContainsString(vm.ObjectMeta.Finalizers, finalizerName) {
		vm.ObjectMeta.Finalizers = append(vm.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, vm); err != nil {
			return err
		}
	}

	if err := r.createOrUpdateVm(ctx, vm); err != nil {
		r.Log.Error(err, "Failed to reconcile VirtualMachine")
		return err
	}

	if r.Log.V(4).Enabled() {
		// Before we update the status, get the current resource and log it
		latestVm := &vmoperatorv1alpha1.VirtualMachine{}
		err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name}, latestVm)
		if err == nil {
			r.Log.V(4).Info("Latest resource before update", "name", vm.NamespacedName(), "resource", latestVm)
			r.Log.V(4).Info("Resource to update", "name", vm.NamespacedName(), "resource", vm)
		}
	}

	r.Log.V(4).Info("Updated VM Status", "name", vm.NamespacedName(), "status", vm.Status)
	if err := r.Status().Update(ctx, vm); err != nil {
		r.Log.Error(err, "Failed to update VirtualMachine status", "name", vm.NamespacedName())
		return err
	}

	return nil
}

func (r *VirtualMachineReconciler) validateStorageClass(ctx goctx.Context, namespace, scName string) error {
	resourceQuotas := &v1.ResourceQuotaList{}
	err := r.List(ctx, resourceQuotas, client.InNamespace(namespace))
	if err != nil {
		r.Log.Error(err, "Failed to list ResourceQuotas", "namespace", namespace)
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

func (r *VirtualMachineReconciler) lookupStoragePolicyID(ctx goctx.Context, namespace, storageClassName string) (string, error) {
	err := r.validateStorageClass(ctx, namespace, storageClassName)
	if err != nil {
		return "", err
	}

	sc := &storagetypev1.StorageClass{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: storageClassName}, sc)
	if err != nil {
		r.Log.Error(err, "Failed to get StorageClass", "storageClassName", storageClassName)
		return "", err
	}

	return sc.Parameters["storagePolicyID"], nil
}

// createOrUpdateVm calls into the VM provider to reconcile a VirtualMachine
func (r *VirtualMachineReconciler) createOrUpdateVm(ctx goctx.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	vmClass := &vmoperatorv1alpha1.VirtualMachineClass{}
	err := r.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)
	if err != nil {
		r.Log.Error(err, "Failed to get VirtualMachineClass for VirtualMachine",
			"vmName", vm.NamespacedName(), "class", vm.Spec.ClassName)
		return err
	}

	var vmMetadata vmprovider.VirtualMachineMetadata
	if metadata := vm.Spec.VmMetadata; metadata != nil {
		configMap := &v1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{Name: metadata.ConfigMapName, Namespace: vm.Namespace}, configMap)
		if err != nil {
			r.Log.Error(err, "Failed to get VirtualMachineMetadata ConfigMap",
				"vmName", vm.NamespacedName(), "configMapName", metadata.ConfigMapName)
			return err
		}

		vmMetadata = configMap.Data
	}

	var resourcePolicy *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy
	if policyName := vm.Spec.ResourcePolicyName; policyName != "" {
		resourcePolicy = &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{}
		err = r.Get(ctx, client.ObjectKey{Name: policyName, Namespace: vm.Namespace}, resourcePolicy)
		if err != nil {
			r.Log.Error(err, "Failed to get VirtualMachineSetResourcePolicy",
				"vmName", vm.NamespacedName(), "resourcePolicyName", policyName)
			return err
		}

		// Make sure that the corresponding entities (RP and Folder) are created on the infra provider before
		// reconciling the VM. Requeue if the ResourcePool and Folders are not yet created for this ResourcePolicy.
		rpReady, err := r.vmProvider.DoesVirtualMachineSetResourcePolicyExist(ctx, resourcePolicy)
		if err != nil {
			r.Log.Error(err, "Failed to check if VirtualMachineSetResourcePolicy exists")
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

	exists, err := r.vmProvider.DoesVirtualMachineExist(ctx, vm)
	if err != nil {
		r.Log.Error(err, "Failed to check if VirtualMachine exists from provider", "name", vm.NamespacedName())
		return err
	}

	if !exists {
		err = r.vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
		if err != nil {
			r.Log.Error(err, "Provider failed to create VirtualMachine", "name", vm.NamespacedName())
			return err
		}
	}

	// At this point, the VirtualMachine is either created, or it already exists. Call into the provider for update.

	pkg.AddAnnotations(&vm.ObjectMeta)

	err = r.vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
	if err != nil {
		r.Log.Error(err, "Provider failed to update VirtualMachine", "name", vm.NamespacedName())
		return err
	}

	return nil
}
