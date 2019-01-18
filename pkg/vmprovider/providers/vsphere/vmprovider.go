/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"github.com/golang/glog"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"vmware.com/kubevsphere/pkg"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	"vmware.com/kubevsphere/pkg/vmprovider"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/sequence"
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

func (vs *VSphereVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1beta1.VirtualMachineImage, error) {
	glog.Info("Listing VM images")

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vms, err := vs.manager.ListVms(ctx, vClient, "")
	if err != nil {
		return nil, err
	}

	newImages := []*v1beta1.VirtualMachineImage{}
	for _, vm := range vms {
		powerState, _ := vm.VirtualMachine.PowerState(ctx)
		ps := string(powerState)
		newImages = append(newImages,
			&v1beta1.VirtualMachineImage{
				ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
				Status: v1beta1.VirtualMachineImageStatus{
					Uuid: vm.VirtualMachine.UUID(ctx),
					PowerState: ps,
					InternalId: vm.VirtualMachine.Reference().Value,
				},
			},
		)
	}

	return newImages, nil
}

func (vs *VSphereVmProvider) GetVirtualMachineImage(ctx context.Context, name string) (*v1beta1.VirtualMachineImage, error) {
	glog.Info("Getting VM images")

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}
	glog.Info("Getting VM image 1")

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vm, err := vs.manager.LookupVm(ctx, vClient, name)
	if err != nil {
		return nil, err
	}

	glog.Info("Getting VM image 2")

	powerState, _ := vm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	return &v1beta1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: vm.VirtualMachine.Name()},
		Status: v1beta1.VirtualMachineImageStatus{
			Uuid: vm.VirtualMachine.UUID(ctx),
			PowerState: ps,
			InternalId: vm.VirtualMachine.Reference().Value,
		},
	}, nil
}

func NewVirtualMachineImageFake(name string) *v1beta1.VirtualMachineImage {
	return &v1beta1.VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{Name: name},
	}
}

func NewVirtualMachineImageListFake() []*v1beta1.VirtualMachineImage {
	images := []*v1beta1.VirtualMachineImage{}

	fake1 := NewVirtualMachineImageFake("Fake")
	fake2 := NewVirtualMachineImageFake("Fake2")
	images = append(images, fake1)
	images = append(images, fake2)
	return images
}

func (vs *VSphereVmProvider) generateVmStatus(ctx context.Context, actualVm *resources.VM) (*v1beta1.VirtualMachine, error) {
	powerState, _ := actualVm.VirtualMachine.PowerState(ctx)
	ps := string(powerState)

	host, err := actualVm.VirtualMachine.HostSystem(ctx)
	if err != nil {
		glog.Infof("Failed to acquire host system for VM %s: %s", err, actualVm.Name)
		return nil, err
	}

	vmIp, err := actualVm.IpAddress(ctx)
	if err != nil {
		glog.Infof("Failed to acquire host system for VM %s: %s", err, actualVm.Name)
		return nil, err
	}

	glog.Infof("VM Host/IP is %s/%s", host.Name(), vmIp)

	vm := &v1beta1.VirtualMachine{
		Status: v1beta1.VirtualMachineStatus{
			Phase: "",
			PowerState: ps,
			//Host: host.Name(),
			Host: vmIp,
			VmIp: vmIp,
			/*
			ConfigStatus: v1beta1.VirtualMachineConfigStatus{
				Uuid: actualVm.VirtualMachine.UUID(ctx),
				InternalId: actualVm.VirtualMachine.Reference().Value,
			},
			*/
				//Host: actualVm.VirtualMachine.HostSystem(ctx),
		},
	}

	glog.Infof("Generated VM status: %+v", vm.Status)

	return vm, nil
}

func (vs *VSphereVmProvider) mergeVmStatus(ctx context.Context, desiredVm *v1beta1.VirtualMachine, actualVm *resources.VM) (*v1beta1.VirtualMachine, error) {

	statusVm, err := vs.generateVmStatus(ctx, actualVm)
	if err != nil {
		return nil, err
	}

	return &v1beta1.VirtualMachine{
		TypeMeta: desiredVm.TypeMeta,
		ObjectMeta: desiredVm.ObjectMeta,
		Spec: desiredVm.Spec,
		Status: *statusVm.Status.DeepCopy(),
	}, nil
}

func (vs *VSphereVmProvider) ListVirtualMachines(ctx context.Context, namespace string) ([]*v1beta1.VirtualMachine, error) {
	return nil, nil
}

func (vs *VSphereVmProvider) GetVirtualMachine(ctx context.Context, name string) (*v1beta1.VirtualMachine, error) {
	glog.Info("Getting VMs")

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	vm, err := vs.manager.LookupVm(ctx, vClient, name)
	if err != nil {
		return nil, err
	}

	return vs.generateVmStatus(ctx, vm)
}

func (vs *VSphereVmProvider) addProviderAnnotations(objectMeta *v1.ObjectMeta, moRef string) {
	// Add vSphere provider annotations to the object meta
	annotations := objectMeta.GetAnnotations()

	annotations[pkg.VmOperatorVmProviderKey] = VsphereVmProviderName
	//annotations[pkg.VmOperatorVcUuidKey] = vs.Config.VcUrl
	annotations[pkg.VmOperatorMorefKey] = moRef

	objectMeta.SetAnnotations(annotations)
}

func (vs *VSphereVmProvider) CreateVirtualMachine(ctx context.Context, vmToCreate *v1beta1.VirtualMachine) (*v1beta1.VirtualMachine, error) {
	glog.Infof("Creating Vm: %s", vmToCreate.Name)

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}

	// DWB: Reason about how to handle client management and logout
	//defer vClient.Logout(ctx)

	// Determine if we should clone from an existing image or create from scratch.  Create from scratch is really
	// only useful for dummy VMs at the moment.
	var newVm *resources.VM
	switch {
	case vmToCreate.Spec.Image != "":
		glog.Infof("Cloning VM from %s", vmToCreate.Spec.Image)
		newVm, err = vs.manager.CloneVm(ctx, vClient, vmToCreate)
	default:
		glog.Info("Creating new VM")
		newVm, err = vs.manager.CreateVm(ctx, vClient, vmToCreate)
	}

	if err != nil {
		glog.Infof("Create VM failed %s!", err)
		return nil, err
	}

	vs.addProviderAnnotations(&vmToCreate.ObjectMeta, newVm.VirtualMachine.Reference().Value)
	return vs.mergeVmStatus(ctx, vmToCreate, newVm)
}

func (vs *VSphereVmProvider) updatePowerState(ctx context.Context, vclient *govmomi.Client, vmToUpdate *v1beta1.VirtualMachine, vm *resources.VM) (*resources.VM, error) {

	ps, err := vm.VirtualMachine.PowerState(ctx)
	if err != nil {
		glog.Errorf("Failed to acquire power state: %s", err.Error())
		return nil, err
	}

	glog.Infof("Current power state: %s, desired power state: %s", ps, vmToUpdate.Spec.PowerState)

	if string(ps) == string(vmToUpdate.Spec.PowerState) {
		glog.Infof("Power state already at desired state of %s", ps)
		return vm, nil
	}

	// Bring PowerState into conformance
	var task *object.Task
	switch vmToUpdate.Spec.PowerState {
	case v1beta1.VirtualMachinePoweredOff:
		task, err = vm.VirtualMachine.PowerOff(ctx)
	case v1beta1.VirtualMachinePoweredOn:
		task, err = vm.VirtualMachine.PowerOn(ctx)
	}

	if err != nil {
		glog.Errorf("Failed to change power state to %s", vmToUpdate.Spec.PowerState)
		return nil, err
	}

	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Power State change task failed %s", err.Error())
		return nil, err
	}

	return resources.NewVMFromReference(*vclient, vm.Datacenter, taskInfo.Result.(types.ManagedObjectReference))
}

func (vs *VSphereVmProvider) UpdateVirtualMachine(ctx context.Context, vmToUpdate *v1beta1.VirtualMachine) (*v1beta1.VirtualMachine, error) {
	glog.Infof("Updating Vm: %s", vmToUpdate.Name)

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return nil, err
	}

	vm, err := vs.manager.LookupVm(ctx, vClient, vmToUpdate.Name)
	if err != nil {
		return nil, err
	}

	/*
	err = vs.updateCapacity(ctx, vmToUpdate, vm)
	if err != nil {
		return v1beta1.VirtualMachine{}, err
	}
	*/

	newVm, err := vs.updatePowerState(ctx, vClient, vmToUpdate, vm)
	if err != nil {
		return nil, err
	}

	// Update spec
	return vs.mergeVmStatus(ctx, vmToUpdate, newVm)
}

func (vs *VSphereVmProvider) DeleteVirtualMachine(ctx context.Context, vmToDelete *v1beta1.VirtualMachine) (error) {
	glog.Infof("Deleting Vm: %s", vmToDelete.Name)

	vClient, err := resources.NewClient(ctx, vs.Config.VcUrl)
	if err != nil {
		return err
	}

	vm, err := vs.manager.LookupVm(ctx, vClient, vmToDelete.Name)
	if err != nil {
		return err
	}

	deleteSequence := sequence.NewVirtualMachineDeleteSequence(vmToDelete, vm)
	err = deleteSequence.Execute(ctx)

	glog.Infof("Delete sequence completed: %s", err)
	return err
}
