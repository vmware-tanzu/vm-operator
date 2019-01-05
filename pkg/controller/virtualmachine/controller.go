
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package virtualmachine

import (
	"context"
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"time"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	clientSet "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset"
	vmclientSet "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1beta1"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	vmprov "vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
)

// +controller:group=vmoperator,version=v1beta1,kind=VirtualMachine,resource=virtualmachines
type VirtualMachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about VirtualMachine
	lister listers.VirtualMachineLister

	clientSet clientSet.Interface
	vmClientSet vmclientSet.VirtualMachineInterface

	vmProvider iface.VirtualMachineProviderInterface
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing virtualmachines labels
	c.lister = arguments.GetSharedInformers().Factory.Vmoperator().V1beta1().VirtualMachines().Lister()

	clientSet, err := clientSet.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error creating virtual machine client: %v", err)
	}
	c.clientSet = clientSet

	c.vmClientSet = clientSet.VmoperatorV1beta1().VirtualMachines(corev1.NamespaceDefault)

	// Get a vmprovider instance
	vmProvider, err := vmprov.NewVmProvider()
	if err != nil {
		glog.Fatalf("Failed to find vmprovider: %s", err)
	}
	c.vmProvider = vmProvider
}

// Function to filter a string from a list. Returns the filtered list
func (c *VirtualMachineControllerImpl) filter(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}

// Function to determine if a list contains s specific string
func (c *VirtualMachineControllerImpl) contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}

// Reconcile handles enqueued messages
func (c *VirtualMachineControllerImpl) Reconcile(u *v1beta1.VirtualMachine) error {
	// Implement controller logic here
	glog.V(0).Infof("Running reconcile VirtualMachine for %s\n", u.Name)

	startTime := time.Now()
	defer func() {
		glog.V(0).Infof("Finished syncing vm %q (%v)", u.Name, time.Since(startTime))
	}()

	// We hold a Finalizer on the VM, so it must be present
	if !u.ObjectMeta.DeletionTimestamp.IsZero() {
		// This VM has been deleted, sync with backend
		glog.Infof("Deletion timestamp is non-zero")

		// Noop if our finalizer is not present
		//if u.ObjectMeta.Finalizers()
		if !c.contains(u.ObjectMeta.Finalizers, v1beta1.VirtualMachineFinalizer) {
			glog.Infof("reconciling virtual machine object %v causes a no-op as there is no finalizer.", u.Name)
			return nil
		}

		glog.Infof("reconciling virtual machine object %v triggers delete.", u.Name)
		if err := c.processVmDeletion(u); err != nil {
			glog.Errorf("Error deleting machine object %v; %v", u.Name, err)
			return err
		}

		// Remove finalizer on successful deletion.
		glog.Infof("virtual machine object %v deletion successful, removing finalizer.", u.Name)
		u.ObjectMeta.Finalizers = c.filter(u.ObjectMeta.Finalizers, v1beta1.VirtualMachineFinalizer)
		if _, err := c.vmClientSet.Update(u); err != nil {
			glog.Errorf("Error removing finalizer from machine object %v; %v", u.Name, err)
			return err
		}
		return nil
	}

	// vm holds the latest vm info from apiserver
	vm, err := c.lister.VirtualMachines(u.Namespace).Get(u.Name)
	switch {
	case err != nil:
		glog.Infof("Unable to retrieve vm %v from store: %v", u.Name, err)
	default:
		err = c.processVmCreateOrUpdate(vm)
	}

	return err
}

func (c *VirtualMachineControllerImpl) processVmDeletion(u *v1beta1.VirtualMachine) error {
	glog.Infof("Process VM Deletion for vm %s", u.Name)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return errors.NewMethodNotSupported(schema.GroupResource{"vmoperator", "VirtualMachines"}, "list")
	}

	ctx := context.TODO()

	err := vmsProvider.DeleteVirtualMachine(ctx, *u)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.Infof("Failed to delete vm %s, already deleted?", u.Name)
		} else {
			glog.Errorf("Failed to delete vm %s: %s", u.Name, err)
			return err
		}
	}

	glog.V(4).Infof("Deleted VM %s %s", u.Name)
	return nil
}

func (c *VirtualMachineControllerImpl) processVmCreateOrUpdate(u *v1beta1.VirtualMachine) error {
	glog.Infof("Process VM Create or Update for vm %s", u.Name)

	vmsProvider, supported := c.vmProvider.VirtualMachines()
	if !supported {
		glog.Errorf("Provider doesn't support vms func")
		return errors.NewMethodNotSupported(schema.GroupResource{"vmoperator", "VirtualMachines"}, "list")
	}

	ctx := context.TODO()
	vm, err := vmsProvider.GetVirtualMachine(ctx, u.Name)
	switch {
		// For now, treat any error as not found
	case err != nil:
	//case errors.IsNotFound(err):
		glog.Infof("VM doesn't exist in backend provider.  Creating now")
		err = c.processVmCreate(ctx, vmsProvider, u)
	//case err != nil:
	//	glog.Infof("Unable to retrieve vm %v from store: %v", u.Name, err)
	default:
		glog.V(4).Infof("Acquired VM %s %s", vm.Name, vm.Status.ConfigStatus.InternalId)
		glog.Infof("Updating Vm %s", vm.Name)
		err = c.processVmUpdate(ctx, vmsProvider, u)
	}

	return err
}

func (c *VirtualMachineControllerImpl) processVmCreate(ctx context.Context, vmsProvider iface.VirtualMachines, vm *v1beta1.VirtualMachine) error {
	glog.Infof("Creating VM: %s", vm.Name)
	//err := vmsProvider.CreateVirtualMachine(ctx, VirtualMachineApiToProvider(*vm))
	newVm, err := vmsProvider.CreateVirtualMachine(ctx, *vm)
	if err != nil {
		glog.Errorf("Provider Failed to Create VM %s: %s", vm.Name, err)
	}

	newVm.DeepCopyInto(vm)
	// Update status from returned object

	// TODO: Have provider pass back a set of annotations
	// TODO: Apply annotations to the object and update

	// Add an Annotation to indicate origin by the vmoperator
	//annotations := vm.GetAnnotations()
	//annotations[pkg.VmOperatorVcUuidKey] = "test"
	//vm.SetAnnotations(annotations)
	return err
}

func (c *VirtualMachineControllerImpl) processVmUpdate(ctx context.Context, vmsProvider iface.VirtualMachines, vm *v1beta1.VirtualMachine) error {
	return nil
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1beta1.VirtualMachine, error) {
	return c.lister.VirtualMachines(namespace).Get(name)
}
