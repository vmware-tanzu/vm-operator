/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage

import (
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder-alpha/pkg/builders"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	listers "vmware.com/kubevsphere/pkg/client/listers_generated/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/controller/sharedinformers"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere"
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

	vsphere.InitProvider(arguments.GetSharedInformers().KubernetesClientSet)
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
