/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine

import (
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"vmware.com/kubevsphere/pkg"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	clientSet "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	"vmware.com/kubevsphere/pkg/lib"
	vmprov "vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"
)

// +controller:group=vmoperator,version=v1alpha1,kind=VirtualMachine,resource=virtualmachines
type VirtualMachineControllerImpl struct {
	builders.DefaultControllerFns

	clientSet clientSet.Interface
	informers *sharedinformers.SharedInformers

	vmLister        listers.VirtualMachineLister
	vmClassesLister listers.VirtualMachineClassLister
	vmServiceLister listers.VirtualMachineServiceLister

	vmProvider iface.VirtualMachineProviderInterface
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {

	c.informers = arguments.GetSharedInformers()

	vmOperator := arguments.GetSharedInformers().Factory.Vmoperator().V1alpha1()

	// Use the lister for indexing VirtualMachines labels
	c.vmLister = vmOperator.VirtualMachines().Lister()
	c.vmClassesLister = vmOperator.VirtualMachineClasses().Lister()
	c.vmServiceLister = vmOperator.VirtualMachineServices().Lister()

	clientSet, err := clientSet.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("Failed to create the virtual machine client: %v", err)
	}
	c.clientSet = clientSet

	if err := vsphere.InitProvider(arguments.GetSharedInformers().KubernetesClientSet); err != nil {
		glog.Fatalf("Failed to initialize vSphere provider: %s", err)
	}
	vmProvider, err := vmprov.NewVmProvider()
	if err != nil {
		glog.Fatalf("Failed to find vmprovider: %s", err)
	}
	c.vmProvider = vmProvider
}

// Used to reconcile VM status periodically to discover async changes from the VM provider backend
func (c *VirtualMachineControllerImpl) postVmEventsToWorkqueue(vm *v1alpha1.VirtualMachine) error {

	if key, err := cache.MetaNamespaceKeyFunc(vm); err == nil {
		c.informers.WorkerQueues["VirtualMachine"].Queue.AddAfter(key, time.Second)
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
	vmName := vmToReconcile.Name
	vmClientSet := c.clientSet.VmoperatorV1alpha1().VirtualMachines(vmToReconcile.Namespace)

	startTime := time.Now()
	defer func() {
		_ = c.postVmEventsToWorkqueue(vmToReconcile)
		glog.V(0).Infof("Finished reconciling VirtualMachine %q duration: %s err: %v",
			vmName, time.Since(startTime), err)
	}()

	glog.V(0).Infof("Reconciling VirtualMachine %q", vmName)

	// Trigger VirtualMachineService evaluation
	if err := c.postVmServiceEventsToWorkqueue(vmToReconcile); err != nil {
		// Keep going in case of an error.
		glog.Errorf("Error posting service event to workqueue for VirtualMachine %q: %v", vmName, err)
	}

	if !vmToReconcile.ObjectMeta.DeletionTimestamp.IsZero() {
		// This VM has been deleted, sync with backend.
		finalizer := v1alpha1.VirtualMachineFinalizer

		if !lib.Contains(vmToReconcile.ObjectMeta.Finalizers, finalizer) {
			glog.Infof("Reconciling deleted VirtualMachine %q is a no-op since there is no finalizer", vmName)
			return nil
		}

		glog.Infof("Reconciling VirtualMachine %q marked for deletion", vmName)

		if err := c.processVmDeletion(vmToReconcile); err != nil {
			glog.Errorf("Error deleting VirtualMachine %q: %v", vmName, err)
			return err
		}

		vmToReconcile.ObjectMeta.Finalizers = lib.Filter(vmToReconcile.ObjectMeta.Finalizers, finalizer)
		if _, err := vmClientSet.Update(vmToReconcile); err != nil {
			glog.Errorf("Error updating VirtualMachine %q after removing finalizer: %v", vmName, err)
			return err
		}

		glog.Infof("VirtualMachine %q was deleted", vmName)
		return nil
	}

	// vm holds the latest VirtualMachine object from apiserver
	vm, err := c.vmLister.VirtualMachines(vmToReconcile.Namespace).Get(vmName)
	if err != nil {
		glog.Infof("Unable to get latest VirtualMachine %q from Lister: %v", vmName, err)
		return err
	}

	_, err = c.processVmCreateOrUpdate(vm)
	if err != nil {
		glog.Infof("Failed to process VirtualMachine %q CreateOrUpdate: %v", vmName, err)
		return err
	}

	return err
}

func (c *VirtualMachineControllerImpl) processVmDeletion(vmToDelete *v1alpha1.VirtualMachine) error {
	vmName := vmToDelete.Name

	glog.Infof("Processing VirtualMachine %q delete", vmName)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return errors.NewMethodNotSupported(schema.GroupResource{Group: "vmoperator", Resource: "VirtualMachines"}, "list")
	}

	ctx := context.TODO()
	err := vmsProvider.DeleteVirtualMachine(ctx, vmToDelete)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Infof("Failed to delete VirtualMachine %q because it was not found", vmName)
			err = nil
		} else {
			glog.Errorf("Failed to delete VirtualMachine %q: %v", vmName, err)
		}
	} else {
		glog.V(4).Infof("Deleted VirtualMachine %q", vmName)
	}

	return err
}

// Process a level trigger for this VM: create if it doesn't exist otherwise update the existing VM.
func (c *VirtualMachineControllerImpl) processVmCreateOrUpdate(vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	vmName := vmToUpdate.Name

	glog.Infof("Process VirtualMachine %q CreateOrUpdate", vmName)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return nil, errors.NewMethodNotSupported(schema.GroupResource{Group: "vmoperator", Resource: "VirtualMachines"}, "list")
	}

	var vm *v1alpha1.VirtualMachine
	ctx := context.TODO()
	_, err := vmsProvider.GetVirtualMachine(ctx, vmName)
	switch {
	case errors.IsNotFound(err):
		glog.Infof("Creating VirtualMachine %q because it was not found", vmName)
		vm, err = c.processVmCreate(ctx, vmsProvider, vmToUpdate)
	case err != nil:
		glog.Infof("Unable to get VirtualMachine %q from provider: %v", vmName, err)
	default:
		glog.Infof("Updating VirtualMachine %q", vmName)
		vm, err = c.processVmUpdate(ctx, vmsProvider, vmToUpdate)
	}

	if err != nil {
		glog.Errorf("Failed to create or update VirtualMachine %q: %v", vmName, err)
		return nil, err
	}

	vmClientSet := c.clientSet.VmoperatorV1alpha1().VirtualMachines(vmToUpdate.Namespace)
	if _, err = vmClientSet.UpdateStatus(vm); err != nil {
		glog.Errorf("Failed to update VirtualMachine %q Object: %v", vmName, err)
		//return nil, err ???
	}

	return vm, err
}

// Process a create event for a new VM.
func (c *VirtualMachineControllerImpl) processVmCreate(ctx context.Context, vmsProvider iface.VirtualMachines, vmToCreate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Creating VirtualMachine %q -- %s", vmToCreate.Name, vmToCreate.Namespace)

	vmClass, err := c.vmClassesLister.VirtualMachineClasses(vmToCreate.Namespace).Get(vmToCreate.Spec.VirtualMachineClassName)
	if err != nil {
		glog.Errorf("Cannot find VirtualMachineClass %q: %v", vmToCreate.Spec.VirtualMachineClassName, err)
		return nil, err
	}

	newVm, err := vmsProvider.CreateVirtualMachine(ctx, vmToCreate, vmClass)
	if err != nil {
		glog.Errorf("Provider Failed to Create VirtualMachine %q: %v", vmToCreate.Name, err)
		return nil, err
	}

	pkg.AddAnnotations(&newVm.ObjectMeta)

	return newVm, err
}

// Process an update event for an existing VM.
func (c *VirtualMachineControllerImpl) processVmUpdate(ctx context.Context, vmsProvider iface.VirtualMachines, vmToUpdate *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error) {
	glog.Infof("Updating VirtualMachine: %q", vmToUpdate.Name)

	newVm, err := vmsProvider.UpdateVirtualMachine(ctx, vmToUpdate)
	if err != nil {
		glog.Errorf("Provider Failed to Update VirtualMachine %q: %v", vmToUpdate.Name, err)
	}

	return newVm, err
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachine, error) {
	return c.vmLister.VirtualMachines(namespace).Get(name)
}
