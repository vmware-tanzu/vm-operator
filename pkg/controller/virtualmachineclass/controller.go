/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass

import (
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/controller/sharedinformers"
	"log"

	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"

	listers "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/listers_generated/vmoperator/v1alpha1"
)

// +controller:group=vmoperator,version=v1alpha1,kind=VirtualMachineClass,resource=virtualmachineclasses
type VirtualMachineClassControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about VirtualMachineClass
	lister listers.VirtualMachineClassLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineClassControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing virtualmachineclasses labels
	c.lister = arguments.GetSharedInformers().Factory.Vmoperator().V1alpha1().VirtualMachineClasses().Lister()
}

// Reconcile handles enqueued messages
func (c *VirtualMachineClassControllerImpl) Reconcile(u *v1alpha1.VirtualMachineClass) error {
	// Implement controller logic here
	log.Printf("Running reconcile VirtualMachineClass for %s\n", u.Name)
	return nil
}

func (c *VirtualMachineClassControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachineClass, error) {
	return c.lister.VirtualMachineClasses(namespace).Get(name)
}
