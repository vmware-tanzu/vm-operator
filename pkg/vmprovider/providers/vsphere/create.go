/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"github.com/golang/glog"

	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

func (v *VSphereManager) createVmInvoke(ctx context.Context, sc *session.SessionContext, vmSpec vimTypes.VirtualMachineConfigSpec) (*object.Task, error) {
	rc := v.ResourceContext
	vm := &resources.VirtualMachine{Name: vmSpec.Name}
	vmSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", rc.Datastore.Datastore.Name()),
	}

	return vm.Create(ctx, rc.Folder.Folder, rc.ResourcePool.ResourcePool, vmSpec)
}

func (v *VSphereManager) CreateVm(ctx context.Context, sc *session.SessionContext, vmToCreate *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass) (*resources.VirtualMachine, error) {
	vmName := vmToCreate.Name

	glog.Infof("Creating VirtualMachine %q", vmName)

	configSpec := createVMConfigSpec(vmToCreate.Name, &vmClass.Spec)
	task, err := v.createVmInvoke(ctx, sc, configSpec)
	if err != nil {
		glog.Errorf("Failed to create VirtualMachine %q: %v", vmName, err)
		return nil, err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VirtualMachine %q create task failed: %v", vmName, err)
		return nil, err
	}

	// TODO(parunesh): This is just terrible. Basically the VM is already created but since we dont have any way of doing task.result, we create another object and perform a lookup.
	// Need something like this- https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/pkg/cloud/vsphere/provisioner/govmomi/create.go#L55
	newVm, err := resources.NewVM(ctx, sc.Finder, configSpec.Name)
	if err != nil {
		return nil, err
	}

	glog.Infof("Created VM %s!", configSpec.Name)

	return newVm, nil
}

func createVMConfigSpec(vmName string, vmClassSpec *v1alpha1.VirtualMachineClassSpec) vimTypes.VirtualMachineConfigSpec {

	configSpec := vimTypes.VirtualMachineConfigSpec{
		Name:     vmName,
		NumCPUs:  int32(vmClassSpec.Hardware.Cpus),
		MemoryMB: int64(vmClassSpec.Hardware.Memory),
	}

	configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{
		Reservation: &vmClassSpec.Policies.Resources.Requests.Cpu,
		Limit:       &vmClassSpec.Policies.Resources.Limits.Cpu,
	}

	configSpec.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
		Reservation: &vmClassSpec.Policies.Resources.Requests.Memory,
		Limit:       &vmClassSpec.Policies.Resources.Limits.Memory,
	}

	return configSpec
}

func (v *VSphereManager) cloneVmInvoke(ctx context.Context, sc *session.SessionContext,
	sourceVm *resources.VirtualMachine, cloneSpec vimTypes.VirtualMachineCloneSpec) (*object.Task, error) {

	vm := &resources.VirtualMachine{Name: cloneSpec.Config.Name}
	rc := v.ResourceContext

	return vm.Clone(ctx, sourceVm.VirtualMachine, rc.Folder.Folder, cloneSpec)
}

func (v *VSphereManager) CloneVm(ctx context.Context, sc *session.SessionContext,
	newVm *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass) (*resources.VirtualMachine, error) {

	cloneSrc := newVm.Spec.Image

	glog.Infof("Cloning VirtualMachine %q from source %q", newVm.Name, cloneSrc)

	// Lookup the source from an existing VM or template.
	cloneSrcVm, err := v.LookupVm(ctx, sc, cloneSrc)
	if err != nil {
		glog.Errorf("Failed to lookup the VM clone source %q: %v", cloneSrc, err)
		return nil, err
	}

	configSpec := createVMConfigSpec(newVm.Name, &vmClass.Spec)
	powerOn := true // TODO(bryanv) Honor newVm.Spec.PowerState
	memory := false // No full memory clones

	cloneSpec := vimTypes.VirtualMachineCloneSpec{
		Config:  &configSpec,
		PowerOn: powerOn,
		Memory:  &memory,
	}

	task, err := v.cloneVmInvoke(ctx, sc, cloneSrcVm, cloneSpec)
	if err != nil {
		glog.Errorf("Failed to start clone for VirtualMachine %q from source %q: %v", newVm.Name, cloneSrc, err)
		return nil, err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VirtualMachine %q clone from source %q task failed: %v", newVm.Name, cloneSrc, err)
		return nil, err
	}

	// TODO(parunesh): Again, horrible. Shouldn't have to do extra lookups.
	clonedVm, err := resources.NewVM(ctx, sc.Finder, cloneSpec.Config.Name)
	if err != nil {
		return nil, err
	}

	glog.Infof("Cloned VirtualMachine %q from %q", clonedVm.Name, cloneSrc)

	return clonedVm, nil
}
