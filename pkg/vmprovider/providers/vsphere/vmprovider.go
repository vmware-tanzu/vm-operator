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
	log.Print("Listing VM images 1")

	defer vClient.Logout(ctx)

	vms, err := vMan.ListVms(ctx, vClient, "")
	if err != nil {
		return nil, err
	}

	log.Print("Listing VM images 2")

	newImages := []vmprovider.VirtualMachineImage{}
	for _, vm := range vms {
		powerState, _ := vm.VirtualMachine.PowerState(ctx)
		ps := string(powerState)
		newImages = append(newImages,
			vmprovider.VirtualMachineImage{
				Name: vm.VirtualMachine.Name(),
				PowerState: ps,
				Uuid: vm.VirtualMachine.UUID(ctx),
				InternalId: vm.VirtualMachine.Reference().Value,
			},
		)
	}
	log.Print("Listing VM images 3")

	return newImages, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (vmprovider.VirtualMachineImage, error) {
	log.Print("Getting VM images")
	//return NewVirtualMachineImageFake(name), nil
	vMan := vs.manager

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return vmprovider.VirtualMachineImage{}, err
	}
	log.Print("Getting VM image 1")

	defer vClient.Logout(ctx)

	vm, err := vMan.LookupVm(ctx, vClient, name)
	if err != nil {
		return vmprovider.VirtualMachineImage{}, err
	}

	log.Print("Getting VM image 2")

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return vmprovider.VirtualMachineImage{
		Name: vm.VirtualMachine.Name(),
		PowerState: ps,
		Uuid: vm.VirtualMachine.UUID(ctx),
		InternalId: vm.VirtualMachine.Reference().Value,
	}, nil
}

func NewVirtualMachineImageFake(name string) vmprovider.VirtualMachineImage {
	return vmprovider.VirtualMachineImage{Name: name}
}

func NewVirtualMachineImageListFake() []vmprovider.VirtualMachineImage {
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
	log.Print("Getting VMs")
	vMan := vs.manager

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return vmprovider.VirtualMachine{}, err
	}
	log.Print("Getting VM 1")

	defer vClient.Logout(ctx)

	vm, err := vMan.LookupVm(ctx, vClient, name)
	if err != nil {
		return vmprovider.VirtualMachine{}, err
	}

	log.Print("Getting VM image 2")

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return vmprovider.VirtualMachine{
		Name: vm.VirtualMachine.Name(),
		PowerState: ps,
		Uuid: vm.VirtualMachine.UUID(ctx),
		InternalId: vm.VirtualMachine.Reference().Value,
	}, nil
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm vmprovider.VirtualMachine) (error) {
	log.Printf("Creating Vm: %s", vm.Name)
	/*
	vMan := vs.manager


	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return err
	}
	log.Print("Creating VM 1")

	defer vClient.Logout(ctx)

	_, err = vMan.CreateVm(ctx, r.Client, vClient, request, instance)
	if err != nil {
		log.Printf("Create VM failed %s!", err)
		return reconcile.Result{}, err
	}
	*/
	return nil
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, name string) (error) {
	return nil
}


