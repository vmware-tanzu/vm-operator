/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type VirtualMachine struct {
	Name           string
	virtualMachine *object.VirtualMachine
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
		Name:           objVm.Name(),
		virtualMachine: objVm,
	}, nil
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, pool *object.ResourcePool, vmSpec *types.VirtualMachineConfigSpec) error {

	if vm.virtualMachine != nil {
		glog.Errorf("Failed to create VM %q because the VM object is already set", vm.Name)
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

	vm.virtualMachine = object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))

	return nil
}

func (vm *VirtualMachine) Clone(ctx context.Context, folder *object.Folder, cloneSpec *types.VirtualMachineCloneSpec) (*VirtualMachine, error) {
	task, err := vm.virtualMachine.Clone(ctx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "clone VM %q task failed", vm.Name)
	}

	clonedObjVm := object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))
	clonedResVm := VirtualMachine{Name: clonedObjVm.Name(), virtualMachine: clonedObjVm}

	return &clonedResVm, nil
}

func (vm *VirtualMachine) Delete(ctx context.Context) error {

	if vm.virtualMachine == nil {
		return fmt.Errorf("failed to delete VM because the VM object is not set")
	}

	// TODO(bryanv) Move power off if needed call here?

	task, err := vm.virtualMachine.Destroy(ctx)
	if err != nil {
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "delete VM task failed")
	}

	return nil
}

func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	// Just get some IP from guest
	err := vm.virtualMachine.Properties(ctx, vm.virtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
	if err != nil {
		return "", err
	}

	if o.Guest == nil {
		glog.Infof("VM %q Guest info is empty", vm.Name)
		return "", &find.NotFoundError{}
	}

	return o.Guest.IpAddress, nil
}

// CpuAllocation returns the current cpu resource settings from the VM
func (vm *VirtualMachine) CpuAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.virtualMachine.Properties(ctx, vm.virtualMachine.Reference(), []string{"config.cpuAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.CpuAllocation, nil
}

// MemoryAllocation returns the current memory resource settings from the VM
func (vm *VirtualMachine) MemoryAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.virtualMachine.Properties(ctx, vm.virtualMachine.Reference(), []string{"config.memoryAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.MemoryAllocation, nil
}

func (vm *VirtualMachine) ReferenceValue() string {
	return vm.virtualMachine.Reference().Value
}

func (vm *VirtualMachine) ImageFields(ctx context.Context) (powerState, uuid, reference string) {
	ps, _ := vm.virtualMachine.PowerState(ctx)

	powerState = string(ps)
	uuid = vm.virtualMachine.UUID(ctx)
	reference = vm.ReferenceValue()

	return
}

func (vm *VirtualMachine) StatusFields(ctx context.Context) (powerState, hostSystemName, ip string) {

	ps, _ := vm.virtualMachine.PowerState(ctx)
	powerState = string(ps)

	if host, err := vm.virtualMachine.HostSystem(ctx); err == nil {
		hostSystemName = host.Name()
	}

	if ps == types.VirtualMachinePowerStatePoweredOn {
		ip, _ = vm.IpAddress(ctx)
	}

	return
}

func (vm *VirtualMachine) SetPowerState(ctx context.Context, desiredPowerState string) error {

	ps, err := vm.virtualMachine.PowerState(ctx)
	if err != nil {
		glog.Errorf("Failed to get VM %q power state: %v", vm.Name, err)
		return err
	}

	glog.Infof("VM %q current power state: %s, desired: %s", vm.Name, ps, desiredPowerState)

	if string(ps) == desiredPowerState {
		return nil
	}

	var task *object.Task

	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		task, err = vm.virtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.virtualMachine.PowerOff(ctx)
	default:
		// TODO(bryanv) Suspend? How would we handle reset?
		err = fmt.Errorf("invalid desired power state %s", desiredPowerState)
	}

	if err != nil {
		glog.Errorf("Failed to change VM %q to power state %s: %v", vm.Name, desiredPowerState, err)
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM %q change power state task failed: %v", vm.Name, err)
		return err
	}

	return nil
}
