/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"io"
	"log"
	"vmware.com/kubevsphere/pkg/vmprovider"
)

var _ = &VSphereVmProvider{}

func InitProvider() {
	vmprovider.RegisterVmProvider("vsphere", func(config io.Reader) (vmprovider.VirtualMachineProviderInterface, error) {
		providerConfig := NewVsphereVmProviderConfig()
		return newVSphereVmProvider(providerConfig)
	})
}

type VSphereVmProvider struct {
	Config VSphereVmProviderConfig
	manager *VSphereManager
}

// Creates new Controller node interface and returns
func newVSphereVmProvider(providerConfig *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	//vs, err := buildVSphereFromConfig(cfg)
	//if err != nil {
	//	return nil, err
	//}
	vs := NewVSphereManager()
	vmProvider := &VSphereVmProvider{*providerConfig, vs}
	return vmProvider, nil
}

func (vs *VSphereVmProvider) VirtualMachines() (vmprovider.VirtualMachines, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) VirtualMachineImages() (vmprovider.VirtualMachineImages, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) Initialize(clientBuilder vmprovider.ClientBuilder, stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]vmprovider.VirtualMachineImage, error) {
	log.Print("Listing VM images")

	vMan := vs.manager

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}

	defer vClient.Logout(ctx)

	vms, err := vMan.ListVms(ctx, vClient, "")
	if err != nil {
		return nil, err
	}

	newImages := []vmprovider.VirtualMachineImage{}
	for _, vm := range vms {
		// include only vm templates
		newImages = append(newImages, vmprovider.VirtualMachineImage{Name: vm.VirtualMachine.Name()})
	}

	return newImages, nil

	//return NewVirtualMachineImageListFake(), nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (vmprovider.VirtualMachineImage, error) {
	log.Print("Getting VM images")
	return NewVirtualMachineImageFake(name), nil
}

func NewVirtualMachineImageFake(name string) vmprovider.VirtualMachineImage {
	return vmprovider.VirtualMachineImage{Name: name}
}

func NewVirtualMachineImageListFake() []vmprovider.VirtualMachineImage {
	//func NewVirtualMachineImageFake() runtime.Object {
	images := []vmprovider.VirtualMachineImage{}

	fake1 := NewVirtualMachineImageFake("Fake")
	fake2 := NewVirtualMachineImageFake("Fake2")
	images = append(images, fake1)
	images = append(images, fake2)
	return images
}
func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]vmprovider.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (vmprovider.VirtualMachine, error) {
	return vmprovider.VirtualMachine{}, nil
}


