
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package virtualmachine

import (
	"log"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1beta1"
)

// +controller:group=vmoperator,version=v1beta1,kind=VirtualMachine,resource=virtualmachines
type VirtualMachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about VirtualMachine
	lister listers.VirtualMachineLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing virtualmachines labels
	c.lister = arguments.GetSharedInformers().Factory.Vmoperator().V1beta1().VirtualMachines().Lister()
}

// Reconcile handles enqueued messages
func (c *VirtualMachineControllerImpl) Reconcile(u *v1beta1.VirtualMachine) error {
	// Implement controller logic here
	log.Printf("Running reconcile VirtualMachine for %s\n", u.Name)
	return nil
}

func (c *VirtualMachineControllerImpl) Get(namespace, name string) (*v1beta1.VirtualMachine, error) {
	return c.lister.VirtualMachines(namespace).Get(name)
}
