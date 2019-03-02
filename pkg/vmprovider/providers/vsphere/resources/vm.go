/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"
	"errors"

	"github.com/golang/glog"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type VirtualMachine struct {
	Name           string
	VirtualMachine *object.VirtualMachine
}

// Lookup a VM with a given name. If success, return a resources.VirtualMachine, error otherwise.
func NewVM(ctx context.Context, finder *find.Finder, name string) (*VirtualMachine, error) {
	vm, err := finder.VirtualMachine(ctx, name)
	if err != nil {
		glog.Errorf("Error while creating VM: %s [%s]", name, err)
		return nil, err
	}

	return &VirtualMachine{
		Name:           name,
		VirtualMachine: vm,
	}, nil
}

func NewVMFromReference(client govmomi.Client, reference types.ManagedObjectReference) (*VirtualMachine, error) {
	vm := object.NewVirtualMachine(client.Client, reference)
	return &VirtualMachine{Name: vm.Name(), VirtualMachine: vm}, nil
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, rp *object.ResourcePool, vmSpec types.VirtualMachineConfigSpec) (*object.Task, error) {
	return folder.CreateVM(ctx, vmSpec, rp, nil)
}

func (vm *VirtualMachine) Clone(ctx context.Context, sourceVm *object.VirtualMachine, folder *object.Folder, cloneSpec types.VirtualMachineCloneSpec) (*object.Task, error) {
	return sourceVm.Clone(ctx, folder, cloneSpec.Config.Name, cloneSpec)
}

func (vm *VirtualMachine) Delete(ctx context.Context) (*object.Task, error) {
	if vm.VirtualMachine == nil {
		return nil, errors.New("VM is not set")
	}
	return vm.VirtualMachine.Destroy(ctx)
}

// Just get some IP from guest
func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	err := vm.VirtualMachine.Properties(ctx, vm.VirtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
	if err != nil {
		return "", err
	}

	if o.Guest == nil {
		glog.Infof("Guest info is empty")
		return "", &find.NotFoundError{}
	}

	return o.Guest.IpAddress, nil
}

// Acquire the current cpu resource settings from the VM
func (vm *VirtualMachine) CpuAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.VirtualMachine.Properties(ctx, vm.VirtualMachine.Reference(), []string{"config.cpuAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.CpuAllocation, nil
}

// Acquire the current memory resource settings from the VM
func (vm *VirtualMachine) MemoryAllocation(ctx context.Context) (*types.ResourceAllocationInfo, error) {
	var o mo.VirtualMachine

	err := vm.VirtualMachine.Properties(ctx, vm.VirtualMachine.Reference(), []string{"config.memoryAllocation"}, &o)
	if err != nil {
		return nil, err
	}

	return o.Config.MemoryAllocation, nil
}
