/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"github.com/golang/glog"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"vmware.com/kubevsphere/pkg"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
)

var _ = &VSphereVmProvider{}

const VsphereVmProviderName string = "vsphere"

func InitProvider() {
	vmprovider.RegisterVmProvider(VsphereVmProviderName, func(config io.Reader) (iface.VirtualMachineProviderInterface, error) {
		providerConfig := NewVsphereVmProviderConfig()
		return newVSphereVmProvider(providerConfig)
	})
}

type VSphereVmProvider struct {
	Config VSphereVmProviderConfig
	manager *VSphereManager
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, moRef string) {
	// Add vSphere provider annotations to the object meta
	annotations := objectMeta.GetAnnotations()
	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	annotations[pkg.VmOperatorVcUuidKey] = vs.Config.VcUrl
	annotations[pkg.VmOperatorMorefKey] = moRef
	objectMeta.SetAnnotations(annotations)
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

func (vs *VSphereVmProvider) VirtualMachines() (iface.VirtualMachines, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) VirtualMachineImages() (iface.VirtualMachineImages, bool) {
	return vs, true
}

func (vs *VSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]v1beta1.VirtualMachineImage, error) {
	glog.Info("Listing VM images")

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}
	glog.Info("Listing VM images 1")

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vms, err := vs.manager.ListVms(ctx, vClient, "")
	if err != nil {
		return nil, err
	}

	glog.Info("Listing VM images 2")

	newImages := []v1beta1.VirtualMachineImage{}
	for _, vm := range vms {
		powerState, _ := vm.VirtualMachine.PowerState(ctx)
		ps := string(powerState)
		newImages = append(newImages,
			v1beta1.VirtualMachineImage{
				ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
				Status: v1beta1.VirtualMachineImageStatus{
					Uuid: vm.VirtualMachine.UUID(ctx),
					PowerState: ps,
					InternalId: vm.VirtualMachine.Reference().Value,
				},
			},
		)
	}
	glog.Info("Listing VM images 3")

	return newImages, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (v1beta1.VirtualMachineImage, error) {
	glog.Info("Getting VM images")
	//return NewVirtualMachineImageFake(name), nil

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return v1beta1.VirtualMachineImage{}, err
	}
	glog.Info("Getting VM image 1")

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vm, err := vs.manager.LookupVm(ctx, vClient, name)
	if err != nil {
		return v1beta1.VirtualMachineImage{}, err
	}

	glog.Info("Getting VM image 2")

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return v1beta1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
		Status: v1beta1.VirtualMachineImageStatus{
			Uuid: vm.VirtualMachine.UUID(ctx),
			PowerState: ps,
			InternalId: vm.VirtualMachine.Reference().Value,
		},
	}, nil
}

func NewVirtualMachineImageFake(name string) v1beta1.VirtualMachineImage {
	return v1beta1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: name},
	}
}

func NewVirtualMachineImageListFake() []v1beta1.VirtualMachineImage {
	images := []v1beta1.VirtualMachineImage{}

	fake1 := NewVirtualMachineImageFake("Fake")
	fake2 := NewVirtualMachineImageFake("Fake2")
	images = append(images, fake1)
	images = append(images, fake2)
	return images
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]v1beta1.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (v1beta1.VirtualMachine, error) {
	glog.Info("Getting VMs")

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return v1beta1.VirtualMachine{}, err
	}
	glog.Info("Getting VM 1")

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vm, err := vs.manager.LookupVm(ctx, vClient, name)
	if err != nil {
		return v1beta1.VirtualMachine{}, err
	}

	glog.Info("Getting VM image 2")

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return v1beta1.VirtualMachine{
		Spec: v1beta1.VirtualMachineSpec{},
		Status: v1beta1.VirtualMachineStatus{
			ConfigStatus: v1beta1.VirtualMachineConfigStatus{
				Uuid: vm.VirtualMachine.UUID(ctx),
				InternalId: vm.VirtualMachine.Reference().Value,
			},
			RuntimeStatus: v1beta1.VirtualMachineRuntimeStatus{
				PowerState: ps,
			},
		},
	}, nil
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vm v1beta1.VirtualMachine) (v1beta1.VirtualMachine, error) {
	glog.Infof("Creating Vm: %s", vm.Name)

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return v1beta1.VirtualMachine{}, err
	}
	glog.Info("Creating VM 1")

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	// Determine if we should clone from an existing image or create from scratch.  Create from scratch is really
	// only useful for dummy VMs at the moment.
	var newVm *VM
	switch {
	case vm.Spec.Image != "":
		glog.Infof("Cloning VM from %s", vm.Spec.Image)
		newVm, err = vs.manager.CloneVm(ctx, vClient, vm)
	default:
		glog.Info("Creating new VM")
		newVm, err = vs.manager.CreateVm(ctx, vClient, vm)
	}

	if err != nil {
		glog.Infof("Create VM failed %s!", err)
		return v1beta1.VirtualMachine{}, err
	}

	vs.addProviderAnnotations(&vm.ObjectMeta, newVm.VirtualMachine.Reference().Value)

	return vm, nil
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vm v1beta1.VirtualMachine) (error) {
	glog.Infof("Deleting Vm: %s", vm.Name)

	vClient, err := NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return err
	}
	glog.Info("Deleting VM 1")

	err = vs.manager.DeleteVm(ctx, vClient, vm)
	if err != nil {
		glog.Infof("Delete VM failed %s!", err)
		return err
	}
	return nil
}


