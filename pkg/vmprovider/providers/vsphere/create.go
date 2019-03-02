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

func (v *VSphereManager) CreateVm(ctx context.Context, sc *session.SessionContext, vmToCreate *v1alpha1.VirtualMachine) (*resources.VirtualMachine, error) {
	glog.Infof("CreateVm %s", vmToCreate.Name)

	vmSpec := vimTypes.VirtualMachineConfigSpec{
		Name:     vmToCreate.Name,
		NumCPUs:  int32(vmToCreate.Spec.Resources.Capacity.Cpu),
		MemoryMB: int64(vmToCreate.Spec.Resources.Capacity.Memory),
	}

	task, err := v.createVmInvoke(ctx, sc, vmSpec)
	if err != nil {
		glog.Errorf("Failed to create VM: %s", err.Error())
		return nil, err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Create task failed %s", err.Error())
		return nil, err
	}

	// TODO(parunesh): This is just terrible. Basically the VM is already created but since we dont have any way of doing task.result, we create another object and perform a lookup.
	// Need something like this- https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/pkg/cloud/vsphere/provisioner/govmomi/create.go#L55
	newVm, err := resources.NewVM(ctx, sc.Finder, vmSpec.Name)
	if err != nil {
		return nil, err
	}

	glog.Infof("Created VM %s!", vmSpec.Name)
	return newVm, nil
}

func (v *VSphereManager) cloneVmInvoke(ctx context.Context, sc *session.SessionContext, sourceVm *resources.VirtualMachine, cloneSpec vimTypes.VirtualMachineCloneSpec) (*object.Task, error) {
	vm := &resources.VirtualMachine{Name: cloneSpec.Config.Name}
	rc := v.ResourceContext

	return vm.Clone(ctx, sourceVm.VirtualMachine, rc.Folder.Folder, cloneSpec)
}

func (v *VSphereManager) CloneVm(ctx context.Context, sc *session.SessionContext, vmToClone *v1alpha1.VirtualMachine) (*resources.VirtualMachine, error) {
	glog.Infof("CloneVm %s", vmToClone.Name)

	// Find existing VM or template matching the image name
	sourceVm, err := v.LookupVm(ctx, sc, vmToClone.Spec.Image)
	if err != nil {
		glog.Errorf("Failed to find source VM: %s, error: %s", vmToClone.Spec.Image, err)
		return nil, err
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{
		Name:     vmToClone.Name,
		NumCPUs:  int32(vmToClone.Spec.Resources.Capacity.Cpu),
		MemoryMB: int64(vmToClone.Spec.Resources.Capacity.Memory),
	}

	// No mem-full clones
	memory := false

	cloneSpec := vimTypes.VirtualMachineCloneSpec{
		Config:  configSpec,
		PowerOn: true, // hack to on for now
		Memory:  &memory,
	}

	task, err := v.cloneVmInvoke(ctx, sc, sourceVm, cloneSpec)
	if err != nil {
		glog.Errorf("Failed to create VM: %s", err.Error())
		return nil, err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Create task failed %s", err.Error())
		return nil, err
	}

	// TODO(parunesh): Again, horrible. Shouldnt have to do extra lookups.
	clonedVm, err := resources.NewVM(ctx, sc.Finder, cloneSpec.Config.Name)
	if err != nil {
		return nil, err
	}

	glog.Infof("Clone VM %s from %s!", vmToClone.Name, cloneSpec.Config.Name)

	return clonedVm, nil
}
