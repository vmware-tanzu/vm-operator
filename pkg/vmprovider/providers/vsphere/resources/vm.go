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

// IpAddress returns the IpAddress of the VM if powered on, error otherwise
func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	ps, err := vm.virtualMachine.PowerState(ctx)
	if err != nil || ps == types.VirtualMachinePowerStatePoweredOff {
		return "", err
	}

	// Just get some IP from guest
	err = vm.virtualMachine.Properties(ctx, vm.virtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
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

func (vm *VirtualMachine) ManagedObject(ctx context.Context) (*mo.VirtualMachine, error) {
	var props mo.VirtualMachine
	if err := vm.virtualMachine.Properties(ctx, vm.virtualMachine.Reference(), nil, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

func (vm *VirtualMachine) ImageFields(ctx context.Context) (powerState, uuid, reference string) {
	ps, _ := vm.virtualMachine.PowerState(ctx)

	powerState = string(ps)
	uuid = vm.virtualMachine.UUID(ctx)
	reference = vm.ReferenceValue()

	return
}

// GetStatus returns a VirtualMachine's Status
func (vm *VirtualMachine) GetStatus(ctx context.Context) (*v1alpha1.VirtualMachineStatus, error) {
	// TODO(bryanv) We should get all the needed fields in one call to VC.

	ps, err := vm.virtualMachine.PowerState(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM %v PowerState VM", vm.Name)
	}

	host, err := vm.virtualMachine.HostSystem(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM %v HostSystem", vm.Name)
	}

	// Since guest info is empty, IpAddress returns not found error which fails the operation.
	// Comment this until we figure out what to do with the IP address.
	// TODO (parunesh): Uncomment when we have a way to get the IP address of a Vm.

	// ip, err := vm.IpAddress(ctx)
	// if err != nil {
	// 	return nil, errors.Wrapf(err, "failed to get VM %v IP", vm.Name)
	//}

	return &v1alpha1.VirtualMachineStatus{
		PowerState: string(ps),
		Host:       host.Name(),
		Phase:      v1alpha1.Created,
	}, nil
}

func (vm *VirtualMachine) SetPowerState(ctx context.Context, desiredPowerState string) error {

	ps, err := vm.virtualMachine.PowerState(ctx)
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
		task, err = vm.virtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		task, err = vm.virtualMachine.PowerOff(ctx)
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
