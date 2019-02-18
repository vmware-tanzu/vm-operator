/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
)

// TODO: Merge this code with vsphere vmprovider, and/or decouple it into smaller modules to avoid this monolith.
// TODO: Make task tracking funcs async via goroutines

type VSphereManager struct {
	Config          VSphereVmProviderConfig
	ResourceContext *ResourceContext
}

func NewVSphereManager() *VSphereManager {
	return &VSphereManager{Config: *GetVsphereVmProviderConfig()}
}

func (v *VSphereManager) resolveResources(ctx context.Context, client *govmomi.Client) (*ResourceContext, error) {
	if v.ResourceContext != nil {
		return v.ResourceContext, nil
	}

	dc, err := resources.NewDatacenter(*client, v.Config.Datacenter)
	if err != nil {
		return nil, err
	}

	err = dc.Lookup()
	if err != nil {
		return nil, err
	}

	folder, err := resources.NewFolder(*client, dc.Datacenter, v.Config.Folder)
	if err != nil {
		return nil, err
	}

	err = folder.Lookup()
	if err != nil {
		return nil, err
	}

	rp, err := resources.NewResourcePool(*client, dc.Datacenter, v.Config.ResourcePool)
	if err != nil {
		return nil, err
	}

	err = rp.Lookup()
	if err != nil {
		return nil, err
	}

	ds, err := resources.NewDatastore(*client, dc.Datacenter, v.Config.Datastore)
	if err != nil {
		return nil, err
	}

	err = ds.Lookup()
	if err != nil {
		return nil, err
	}

	rc := ResourceContext{
		datacenter:   dc,
		folder:       folder,
		resourcePool: rp,
		datastore:    ds,
	}

	v.ResourceContext = &rc

	return v.ResourceContext, nil
}

func (v *VSphereManager) ListVms(ctx context.Context, vClient *govmomi.Client, vmFolder string) ([]*resources.VM, error) {
	glog.Info("Listing VMs")
	vms := []*resources.VM{}
	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		glog.Infof("Failed to resolve resources Vms: %s", err)
		return nil, err
	}

	list, err := rc.datacenter.ListVms(ctx, "*")
	if err != nil {
		glog.Infof("Failed to list Vms: %s", err)
		return nil, err
	}

	for _, vmiter := range list {
		glog.Infof("Found VM: %s %s %s", vmiter.Name(), vmiter.Reference().Type, vmiter.Reference().Value)
		vm, err := v.LookupVm(ctx, vClient, vmiter.Name())
		if err == nil {
			glog.Infof("Append VM: %s", vm.Name)
			vms = append(vms, vm)
		}
	}

	return vms, nil
}

func (v *VSphereManager) LookupVm(ctx context.Context, vClient *govmomi.Client, vmName string) (*resources.VM, error) {
	glog.Info("Lookup VM")
	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	vm, err := resources.NewVM(*vClient, rc.datacenter, vmName)
	if err != nil {
		return nil, err
	}

	err = vm.Lookup()
	if err != nil {
		return nil, err
	}

	glog.Infof("vm: %s path: %s", vm.Name, vm.VirtualMachine.InventoryPath)

	return vm, nil
}

func (v *VSphereManager) deleteVmInvoke(ctx context.Context, client *govmomi.Client, name string) (*object.Task, error) {
	rc, err := v.resolveResources(ctx, client)
	if err != nil {
		return nil, err
	}

	vm, err := resources.NewVM(*client, rc.datacenter, name)
	if err != nil {
		return nil, err
	}

	err = vm.Lookup()
	if err != nil {
		return nil, err
	}

	return vm.Delete(ctx)
}

func (v *VSphereManager) DeleteVm(ctx context.Context, vClient *govmomi.Client, vm v1alpha1.VirtualMachine) error {
	glog.Infof("DeleteVm %s", vm.Name)

	task, err := v.deleteVmInvoke(ctx, vClient, vm.Name)
	if err != nil {
		glog.Infof("Failed to delete VM: %s", err.Error())
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Infof("VM delete task failed %s", err.Error())
		return err
	}

	return nil
}

func (v *VSphereManager) createVmInvoke(ctx context.Context, client *govmomi.Client, rc *ResourceContext, vmSpec vimTypes.VirtualMachineConfigSpec) (*object.Task, error) {

	vm, err := resources.NewVM(*client, rc.datacenter, vmSpec.Name)
	if err != nil {
		return nil, err
	}

	vmSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", rc.datastore.Datastore.Name()),
	}

	return vm.Create(ctx, rc.folder.Folder, rc.resourcePool.ResourcePool, vmSpec)
}

func (v *VSphereManager) CreateVm(ctx context.Context, vClient *govmomi.Client, vmToCreate *v1alpha1.VirtualMachine) (*resources.VM, error) {
	glog.Infof("CreateVm %s", vmToCreate.Name)

	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	vmSpec := vimTypes.VirtualMachineConfigSpec{
		Name:     vmToCreate.Name,
		NumCPUs:  int32(vmToCreate.Spec.Resources.Capacity.Cpu),
		MemoryMB: int64(vmToCreate.Spec.Resources.Capacity.Memory),
	}

	task, err := v.createVmInvoke(ctx, vClient, rc, vmSpec)
	if err != nil {
		glog.Errorf("Failed to create VM: %s", err.Error())
		return nil, err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Create task failed %s", err.Error())
		return nil, err
	}

	newVm, err := resources.NewVM(*vClient, rc.datacenter, vmSpec.Name)
	if err != nil {
		return nil, err
	}

	// DWB: Need to resolve from info rather than lookup
	err = newVm.Lookup()
	if err != nil {
		return nil, err
	}

	glog.Infof("Created VM %s!", vmSpec.Name)
	return newVm, nil
}

func (v *VSphereManager) cloneVmInvoke(ctx context.Context, client *govmomi.Client, rc *ResourceContext, sourceVm *resources.VM, cloneSpec vimTypes.VirtualMachineCloneSpec) (*object.Task, error) {

	vm, err := resources.NewVM(*client, rc.datacenter, cloneSpec.Config.Name)
	if err != nil {
		return nil, err
	}

	return vm.Clone(ctx, sourceVm.VirtualMachine, rc.folder.Folder, cloneSpec)
}

func (v *VSphereManager) CloneVm(ctx context.Context, vClient *govmomi.Client, vmToClone *v1alpha1.VirtualMachine) (*resources.VM, error) {
	glog.Infof("CloneVm %s", vmToClone.Name)

	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	// Find existing VM or template matching the image name
	sourceVm, err := v.LookupVm(ctx, vClient, vmToClone.Spec.Image)
	if err != nil {
		glog.Errorf("Failed to find source VM %s: %s", vmToClone.Spec.Image, err)
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

	task, err := v.cloneVmInvoke(ctx, vClient, rc, sourceVm, cloneSpec)
	if err != nil {
		glog.Errorf("Failed to create VM: %s", err.Error())
		return nil, err
	}

	//info, err := task.WaitForResult(ctx, nil)
	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		glog.Errorf("VM Create task failed %s", err.Error())
		return nil, err
	}

	clonedVm, err := resources.NewVM(*vClient, rc.datacenter, cloneSpec.Config.Name)
	if err != nil {
		return nil, err
	}

	// DWB: Need to resolve from info rather than lookup
	err = clonedVm.Lookup()
	if err != nil {
		return nil, err
	}

	glog.Infof("Clone VM %s from %s!", vmToClone.Name, cloneSpec.Config.Name)

	return clonedVm, nil
}

/*
func (v *VSphereManager) UpdateVm(ctx context.Context, vClient *govmomi.Client, vmToUpdate *v1alpha1.VirtualMachine, vm *VM) (*VM, error) {
	// Diff instance with VM config on backend
	// DWB: Make this a table of prop actors
	// Update VM Config first
	// Perform Power Ops second

	_, err := v.resolveResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	//vmSpec := vimTypes.VirtualMachineConfigSpec{
	//}
	/*
	err = v.updatePowerState(ctx, instance, vm)
	if err != nil {
		return nil, err
	}

	glog.Infof("Udpated VM %s!", vmToUpdate.Name)
	return vm, nil
}
*/
