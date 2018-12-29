/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"io"
	"vmware.com/kubevsphere/pkg/vmprovider"
)

func init() {
	vmprovider.RegisterVmProvider("vsphere", func(config io.Reader) (vmprovider.VirtualMachineProviderInterface, error) {
		providerConfig := NewVsphereVmProviderConfig()
		return newVSphereVmProvider(providerConfig)
	})
}

// Creates new Controller node interface and returns
func newVSphereVmProvider(providerConfig *VSphereVmProviderConfig) (*VSphereVmProvider, error) {
	//vs, err := buildVSphereFromConfig(cfg)
	//if err != nil {
	//	return nil, err
	//}
	vmProvider := &VSphereVmProvider{*providerConfig}
	return vmProvider, nil
}

type VSphereVmProvider struct {
	Config VSphereVmProviderConfig
}

func (vs *VSphereVmProvider) VirtualMachines() (vmprovider.VirtualMachines, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) VirtualMachineImages() (vmprovider.VirtualMachineImages, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) Initialize(clientBuilder vmprovider.ClientBuilder, stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*vmprovider.VirtualMachineImage, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (*vmprovider.VirtualMachineImage, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]*vmprovider.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (*vmprovider.VirtualMachine, error) {
	return nil, nil
}
