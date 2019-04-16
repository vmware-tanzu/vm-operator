/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	clientSet "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	listers "github.com/vmware-tanzu/vm-operator/pkg/client/listers_generated/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/sharedinformers"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/iface"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// +controller:group=vmoperator,version=v1alpha1,kind=VirtualMachine,resource=virtualmachines
type VirtualMachineControllerImpl struct {
	builders.DefaultControllerFns

	clientSet clientSet.Interface
	informers *sharedinformers.SharedInformers

	vmLister        listers.VirtualMachineLister
	vmClassLister   listers.VirtualMachineClassLister
	vmServiceLister listers.VirtualMachineServiceLister

	vmProvider iface.VirtualMachineProviderInterface
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {

	c.clientSet = clientSet.NewForConfigOrDie(arguments.GetRestConfig())
	c.vmProvider = vmprovider.GetVmProviderOrDie()
	c.informers = arguments.GetSharedInformers()

	vmOperator := arguments.GetSharedInformers().Factory.Vmoperator().V1alpha1()
	c.vmLister = vmOperator.VirtualMachines().Lister()
	c.vmClassLister = vmOperator.VirtualMachineClasses().Lister()
	c.vmServiceLister = vmOperator.VirtualMachineServices().Lister()
}

// Used to reconcile VM status periodically to discover async changes from the VM provider backend
func (c *VirtualMachineControllerImpl) postVmEventsToWorkqueue(vm *v1alpha1.VirtualMachine) error {
	if key, err := cache.MetaNamespaceKeyFunc(vm); err == nil {
		c.informers.WorkerQueues["VirtualMachine"].Queue.AddAfter(key, 10*time.Second)
	}

	return nil
}

func (c *VirtualMachineControllerImpl) postVmServiceEventsToWorkqueue(vm *v1alpha1.VirtualMachine) error {
	vmServices, err := c.vmServiceLister.VirtualMachineServices(vm.Namespace).List(labels.Everything())
	if err != nil {
		return err
	}

	for _, vmService := range vmServices {
		if key, err := cache.MetaNamespaceKeyFunc(vmService); err == nil {
			c.informers.WorkerQueues["VirtualMachineService"].Queue.AddRateLimited(key)
		}
	}

	return nil
}

// Reconcile handles enqueued messages
func (c *VirtualMachineControllerImpl) Reconcile(originalVm *v1alpha1.VirtualMachine) error {
	var err error

	vm := originalVm.DeepCopy()
	vmName := vm.NamespacedName()

	startTime := time.Now()
	defer func() {
		_ = c.postVmEventsToWorkqueue(vm)
		glog.V(0).Infof("Finished reconciling VirtualMachine %v duration: %s err: %v",
			vmName, time.Since(startTime), err)
	}()

	glog.V(0).Infof("Reconciling VirtualMachine %v", vmName)

	// Trigger VirtualMachineService evaluation
	if err := c.postVmServiceEventsToWorkqueue(vm); err != nil {
		// Keep going in case of an error.
		glog.Errorf("Error posting service event to workqueue for VirtualMachine %v: %v", vmName, err)
	}

	ctx := context.Background()

	// VM is being deleted, remove the finalizer and update the status
	if !vm.ObjectMeta.DeletionTimestamp.IsZero() {
		const finalizer = v1alpha1.VirtualMachineFinalizer

		if !lib.Contains(vm.ObjectMeta.Finalizers, finalizer) {
			glog.Infof("Reconciling deleted VirtualMachine %v is a no-op since there is no finalizer", vmName)
			return nil
		}

		glog.Infof("Reconciling VirtualMachine %v marked for deletion", vmName)
		if err := c.deleteVm(ctx, vm); err != nil {
			glog.Infof("Failed to delete VirtualMachine %v: %v", vmName, err)
			return err
		}

		// Remove the finalizer and update the VM
		vm.ObjectMeta.Finalizers = lib.Filter(vm.ObjectMeta.Finalizers, finalizer)
		vm.Status.Phase = v1alpha1.Deleted

		// TODO(bryanv) Shouldn't ignore these errors
		_ = c.updateVirtualMachineStatus(vm)

		return nil
	}

	if _, err := c.reconcileVm(ctx, vm); err != nil {
		glog.Errorf("Error reconciling VirtualMachine %v: %v", vmName, err)
		return err
	}

	return nil
}

func (c *VirtualMachineControllerImpl) deleteVm(ctx context.Context, vm *v1alpha1.VirtualMachine) error {

	err := c.vmProvider.DeleteVirtualMachine(ctx, vm)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Infof("Failed to delete VirtualMachine %v because it was not found", vm.NamespacedName())
			return nil
		}

		glog.Errorf("Failed to delete VirtualMachine %v: %v", vm.NamespacedName(), err)
		return err
	}

	glog.V(4).Infof("Deleted VirtualMachine %v", vm.NamespacedName())

	return nil
}

// updateVirtualMachine updates the VirtualMachine status
func (c *VirtualMachineControllerImpl) updateVirtualMachineStatus(vm *v1alpha1.VirtualMachine) error {
	vmClientSet := c.clientSet.VmoperatorV1alpha1().VirtualMachines(vm.Namespace)
	if vm, err := vmClientSet.UpdateStatus(vm); err != nil {
		glog.Errorf("Failed to update VirtualMachine %v status: %v", vm.NamespacedName(), err)
		return err
	}

	return nil
}

// reconcileVm updates a VM if it exists, creates a new one if it doesn't. Update the status phase to reflect the transition
func (c *VirtualMachineControllerImpl) reconcileVm(ctx context.Context, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmName := vmToUpdate.NamespacedName()

	glog.Infof("Process VirtualMachine %v CreateOrUpdate", vmName)

	_, err := c.vmProvider.GetVirtualMachine(ctx, vmToUpdate.Namespace, vmToUpdate.Name)
	switch {
	case errors.IsNotFound(err):
		vmToUpdate.Status.Phase = v1alpha1.Creating
		vmToUpdate, err = c.createVm(ctx, vmToUpdate)
	case err != nil:
		glog.Infof("Failed to get VirtualMachine %v from provider: %v", vmName, err)
	default:
		vmToUpdate, err = c.updateVm(ctx, vmToUpdate)
	}

	// update the status even in case of failure to reflect which operation failed
	// TODO(bryanv) Shouldn't ignore these errors
	_ = c.updateVirtualMachineStatus(vmToUpdate)

	// return the original error
	return vmToUpdate, err
}

func (c *VirtualMachineControllerImpl) createVm(ctx context.Context, vm *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmClass, err := c.vmClassLister.VirtualMachineClasses(vm.Namespace).Get(vm.Spec.ClassName)
	if err != nil {
		glog.Errorf("Failed to get VirtualMachineClass %q: %v", vm.Spec.ClassName, err)
		return vm, err
	}

	var metadata map[string]string
	if vm.Spec.VmMetadata != nil && vm.Spec.VmMetadata.ConfigMapName != "" {
		cm, err := c.informers.KubernetesClientSet.CoreV1().ConfigMaps(vm.Namespace).Get(vm.Spec.VmMetadata.ConfigMapName, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed to get ConfigMap %s for VmMetadata %q: %v", vm.Spec.VmMetadata.ConfigMapName, vm.Spec.ClassName, err)
			return vm, err
		}
		metadata = cm.Data
	}

	newVm, err := c.vmProvider.CreateVirtualMachine(ctx, vm, vmClass, metadata)
	if err != nil {
		glog.Errorf("Provider failed to create VirtualMachine %v: %v", vm.NamespacedName(), err)
		return vm, err
	}

	pkg.AddAnnotations(&newVm.ObjectMeta)

	return newVm, nil
}

// Process an update event for an existing VM.
func (c *VirtualMachineControllerImpl) updateVm(ctx context.Context, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {

	newVm, err := c.vmProvider.UpdateVirtualMachine(ctx, vmToUpdate)
	if err != nil {
		glog.Errorf("Provider failed to update VirtualMachine %v: %v", vmToUpdate.NamespacedName(), err)
		return nil, err
	}

	return newVm, nil
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachine, error) {
	return c.vmLister.VirtualMachines(namespace).Get(name)
}
