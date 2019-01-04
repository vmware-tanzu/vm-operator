
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package virtualmachineimage

import (
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
)

// +controller:group=vmoperator,version=v1beta1,kind=VirtualMachineImage,resource=virtualmachineimages
type VirtualMachineImageControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about VirtualMachineImage
	lister listers.VirtualMachineImageLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineImageControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing virtualmachineimages labels
	c.lister = arguments.GetSharedInformers().Factory.Vmoperator().V1beta1().VirtualMachineImages().Lister()
}

// Reconcile handles enqueued messages
func (c *VirtualMachineImageControllerImpl) Reconcile(u *v1beta1.VirtualMachineImage) error {
	// Implement controller logic here
	glog.Infof("Running reconcile VirtualMachineImage for %s\n", u.Name)
	return nil
}

func (c *VirtualMachineImageControllerImpl) Get(namespace, name string) (*v1beta1.VirtualMachineImage, error) {
	return c.lister.VirtualMachineImages(namespace).Get(name)
}
