/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage

import (
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"

	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/controller/sharedinformers"
	listers "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/listers_generated/vmoperator/v1alpha1"
)

// +controller:group=vmoperator,version=v1alpha1,kind=VirtualMachineImage,resource=virtualmachineimages
type VirtualMachineImageControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about VirtualMachineImage
	lister listers.VirtualMachineImageLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *VirtualMachineImageControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing virtualmachineimages labels
	c.lister = arguments.GetSharedInformers().Factory.Vmoperator().V1alpha1().VirtualMachineImages().Lister()
}

// Reconcile handles enqueued messages
func (c *VirtualMachineImageControllerImpl) Reconcile(u *v1alpha1.VirtualMachineImage) error {
	// Implement controller logic here
	glog.Infof("Running reconcile VirtualMachineImage for %s\n", u.Name)
	return nil
}

func (c *VirtualMachineImageControllerImpl) Get(namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	return c.lister.VirtualMachineImages(namespace).Get(name)
}
