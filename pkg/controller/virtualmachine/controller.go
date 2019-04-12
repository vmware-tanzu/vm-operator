/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"
	"time"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/sharedinformers"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/iface"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"
	clientSet "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset"
	listers "github.com/vmware-tanzu/vm-operator/pkg/client/listers_generated/vmoperator/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
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
func (c *VirtualMachineControllerImpl) Reconcile(vmToReconcile *v1alpha1.VirtualMachine) error {
	var err error
	vmName := vmToReconcile.GetFullName()

	startTime := time.Now()
	defer func() {
		_ = c.postVmEventsToWorkqueue(vmToReconcile)
		glog.V(0).Infof("Finished reconciling VirtualMachine %v duration: %s err: %v",
			vmName, time.Since(startTime), err)
	}()

	glog.V(0).Infof("Reconciling VirtualMachine %v", vmName)

	// Trigger VirtualMachineService evaluation
	if err := c.postVmServiceEventsToWorkqueue(vmToReconcile); err != nil {
		// Keep going in case of an error.
		glog.Errorf("Error posting service event to workqueue for VirtualMachine %v: %v", vmName, err)
	}

	if !vmToReconcile.ObjectMeta.DeletionTimestamp.IsZero() {
		// This VM has been deleted, sync with backend.
		finalizer := v1alpha1.VirtualMachineFinalizer

		if !lib.Contains(vmToReconcile.ObjectMeta.Finalizers, finalizer) {
			glog.Infof("Reconciling deleted VirtualMachine %v is a no-op since there is no finalizer", vmName)
			return nil
		}

		glog.Infof("Reconciling VirtualMachine %v marked for deletion", vmName)

		if err := c.processVmDeletion(vmToReconcile); err != nil {
			glog.Errorf("Failed deleting VirtualMachine %v: %v", vmName, err)
			return err
		}

		vmToReconcile.ObjectMeta.Finalizers = lib.Filter(vmToReconcile.ObjectMeta.Finalizers, finalizer)

		vmClientSet := c.clientSet.VmoperatorV1alpha1().VirtualMachines(vmToReconcile.Namespace)
		if _, err := vmClientSet.Update(vmToReconcile); err != nil {
			glog.Errorf("Failed updating VirtualMachine %v after removing finalizer: %v", vmName, err)
			return err
		}

		return nil
	}

	// vm holds the latest VirtualMachine object from apiserver
	vm, err := c.vmLister.VirtualMachines(vmToReconcile.Namespace).Get(vmToReconcile.Name)
	if err != nil {
		glog.Infof("Failed to get latest VirtualMachine %v from Lister: %v", vmName, err)
		return err
	}

	_, err = c.processVmCreateOrUpdate(vm)
	if err != nil {
		glog.Infof("Failed to process VirtualMachine %v CreateOrUpdate: %v", vmName, err)
		return err
	}

	return nil
}

func (c *VirtualMachineControllerImpl) processVmDeletion(vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.GetFullName()

	err := c.vmProvider.DeleteVirtualMachine(context.TODO(), vmToDelete)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Infof("Failed to delete VirtualMachine %v because it was not found", vmName)
			return nil
		}
		glog.Errorf("Failed to delete VirtualMachine %v: %v", vmName, err)
		return err
	}

	glog.V(4).Infof("Deleted VirtualMachine %v", vmName)

	return nil
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (c *VirtualMachineControllerImpl) processVmCreateOrUpdate(vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmName := vmToUpdate.GetFullName()

	glog.Infof("Process VirtualMachine %v CreateOrUpdate", vmName)

	var vm *v1alpha1.VirtualMachine
	ctx := context.TODO()
	_, err := c.vmProvider.GetVirtualMachine(ctx, vmToUpdate.Namespace, vmToUpdate.Name)
	switch {
	case errors.IsNotFound(err):
		vm, err = c.processVmCreate(ctx, vmToUpdate)
	case err != nil:
		glog.Infof("Failed to get VirtualMachine %v from provider: %v", vmName, err)
	default:
		vm, err = c.processVmUpdate(ctx, vmToUpdate)
	}

	if err != nil {
		return nil, err
	}

	vmClientSet := c.clientSet.VmoperatorV1alpha1().VirtualMachines(vmToUpdate.Namespace)
	if _, err = vmClientSet.UpdateStatus(vm); err != nil {
		glog.Errorf("Failed to update VirtualMachine %v Object: %v", vmName, err)
		//return nil, err ???
	}

	return vm, nil
}

// Process a create event for a new VM.
func (c *VirtualMachineControllerImpl) processVmCreate(ctx context.Context, vmToCreate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {

	vmClass, err := c.vmClassLister.VirtualMachineClasses(vmToCreate.Namespace).Get(vmToCreate.Spec.ClassName)
	if err != nil {
		glog.Errorf("Failed to get VirtualMachineClass %q: %v", vmToCreate.Spec.ClassName, err)
		return nil, err
	}

	newVm, err := c.vmProvider.CreateVirtualMachine(ctx, vmToCreate, vmClass)
	if err != nil {
		glog.Errorf("Provider failed to create VirtualMachine %v: %v", vmToCreate.GetFullName(), err)
		return nil, err
	}

	pkg.AddAnnotations(&newVm.ObjectMeta)

	return newVm, nil
}

// Process an update event for an existing VM.
func (c *VirtualMachineControllerImpl) processVmUpdate(ctx context.Context, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {

	newVm, err := c.vmProvider.UpdateVirtualMachine(ctx, vmToUpdate)
	if err != nil {
		glog.Errorf("Provider failed to update VirtualMachine %v: %v", vmToUpdate.GetFullName(), err)
		return nil, err
	}

	return newVm, nil
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachine, error) {
	return c.vmLister.VirtualMachines(namespace).Get(name)
}
