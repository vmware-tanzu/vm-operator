/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"
	"fmt"

	"k8s.io/klog"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

	"github.com/pkg/errors"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type VirtualMachine struct {
	Name             string
	vcVirtualMachine *object.VirtualMachine
}

// NewVMForCreate returns a VirtualMachine that Create() can be called on
// to create the VM and set the VirtualMachine object reference.
func NewVMForCreate(name string) *VirtualMachine {
	return &VirtualMachine{
		Name: name,
	}
}

func NewVMFromObject(objVm *object.VirtualMachine) (*VirtualMachine, error) {
	return &VirtualMachine{
		Name:             objVm.Name(),
		vcVirtualMachine: objVm,
	}, nil
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, pool *object.ResourcePool, vmSpec *types.VirtualMachineConfigSpec) error {
	if vm.vcVirtualMachine != nil {
		klog.Errorf("Failed to create VM %q because the VM object is already set", vm.Name)
		return fmt.Errorf("failed to create VM %q because the VM object is already set", vm.Name)
	}

	task, err := folder.CreateVM(ctx, *vmSpec, pool, nil)
	if err != nil {
		return err
	}

	result, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "create VM %q task failed", vm.Name)
	}

	vm.vcVirtualMachine = object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))

	return nil
}

func (vm *VirtualMachine) Clone(ctx context.Context, folder *object.Folder, cloneSpec *types.VirtualMachineCloneSpec) (*VirtualMachine, error) {
	task, err := vm.vcVirtualMachine.Clone(ctx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "clone VM %q task failed", vm.Name)
	}

	clonedObjVm := object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))
	clonedResVm := VirtualMachine{Name: clonedObjVm.Name(), vcVirtualMachine: clonedObjVm}

	return &clonedResVm, nil
}

func (vm *VirtualMachine) Delete(ctx context.Context) error {

	if vm.vcVirtualMachine == nil {
		return fmt.Errorf("failed to delete VM because the VM object is not set")
	}

	// TODO(bryanv) Move power off if needed call here?

	task, err := vm.vcVirtualMachine.Destroy(ctx)
	if err != nil {
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "delete VM task failed")
	}

	return nil
}

func (vm *VirtualMachine) Reconfigure(ctx context.Context, configSpec *types.VirtualMachineConfigSpec) error {
	task, err := vm.vcVirtualMachine.Reconfigure(ctx, *configSpec)
	if err != nil {
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "reconfigure VM %q task failed", vm.Name)
	}

	return nil
}

// IpAddress returns the IpAddress of the VM if powered on, error otherwise
func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil || ps == types.VirtualMachinePowerStatePoweredOff {
		return "", err
	}

	// Just get some IP from guest
	err = vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
	if err != nil {
		return "", err
	}

	if o.Guest == nil {
		klog.Infof("VM %q Guest info is empty", vm.Name)
		return "", &find.NotFoundError{}
	}

	return o.Guest.IpAddress, nil
}

// CpuAllocation returns the current cpu resource settings from the VM
func (vm *VirtualMachine) CpuAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.cpuAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.CpuAllocation, nil
}

// MemoryAllocation returns the current memory resource settings from the VM
func (vm *VirtualMachine) MemoryAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.memoryAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.MemoryAllocation, nil
}

func (vm *VirtualMachine) ReferenceValue() string {
	return vm.vcVirtualMachine.Reference().Value
}

func (vm *VirtualMachine) ManagedObject(ctx context.Context) (*mo.VirtualMachine, error) {
	var props mo.VirtualMachine
	if err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), nil, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

func (vm *VirtualMachine) ImageFields(ctx context.Context) (powerState, uuid, reference string) {
	ps, _ := vm.vcVirtualMachine.PowerState(ctx)

	powerState = string(ps)
	uuid = vm.vcVirtualMachine.UUID(ctx)
	reference = vm.ReferenceValue()

	return
}

// GetStatus returns a VirtualMachine's Status
func (vm *VirtualMachine) GetStatus(ctx context.Context) (*v1alpha1.VirtualMachineStatus, error) {
	// TODO(bryanv) We should get all the needed fields in one call to VC.

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM %v PowerState VM", vm.Name)
	}

	host, err := vm.vcVirtualMachine.HostSystem(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM %v HostSystem", vm.Name)
	}

	ip, err := vm.IpAddress(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM %v IP address", vm.Name)
	}

	return &v1alpha1.VirtualMachineStatus{
		Host:       host.Name(),
		Phase:      v1alpha1.Created,
		PowerState: string(ps),
		VmIp:       ip,
	}, nil
}

func (vm *VirtualMachine) SetPowerState(ctx context.Context, desiredPowerState string) error {

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		klog.Errorf("Failed to get VM %q power state: %v", vm.Name, err)
		return err
	}

	klog.Infof("VM %q current power state: %s, desired: %s", vm.Name, ps, desiredPowerState)

	if string(ps) == desiredPowerState {
		return nil
	}

	var task *object.Task

	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		task, err = vm.vcVirtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.vcVirtualMachine.PowerOff(ctx)
	default:
		// TODO(bryanv) Suspend? How would we handle reset?
		err = fmt.Errorf("invalid desired power state %s", desiredPowerState)
	}

	if err != nil {
		klog.Errorf("Failed to change VM %q to power state %s: %v", vm.Name, desiredPowerState, err)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		klog.Errorf("VM %q change power state task failed: %v", vm.Name, err)
		return err
	}

	return nil
}

// GetVirtualDisks returns the list of VMs vmdks
func (vm *VirtualMachine) GetVirtualDisks(ctx context.Context) (object.VirtualDeviceList, error) {
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		return nil, err
	}

	return deviceList.SelectByType((*types.VirtualDisk)(nil)), nil
}

func (vm *VirtualMachine) GetNetworkDevices(ctx context.Context) ([]types.BaseVirtualDevice, error) {
	devices, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		klog.Errorf("Failed to get devices for VM %q: %v", vm.Name, err)
		return nil, err
	}

	return devices.SelectByType((*types.VirtualEthernetCard)(nil)), nil
}
