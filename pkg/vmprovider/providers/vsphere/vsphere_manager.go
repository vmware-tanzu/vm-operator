/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import (
	"context"
	"github.com/vmware/govmomi"
	//"github.com/vmware/govmomi/object"
	//vimTypes "github.com/vmware/govmomi/vim25/types"
	"log"
	//vmv1 "vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
)

type VSphereManager struct {
	Config 			VSphereVmProviderConfig
	ResourceContext *ResourceContext
}

func NewVSphereManager() *VSphereManager {
	return &VSphereManager{Config: *NewVsphereVmProviderConfig()}
}

func (v *VSphereManager) resolveResources(ctx context.Context, client *govmomi.Client) (*ResourceContext, error) {
	if v.ResourceContext != nil {
		return v.ResourceContext, nil
	}

	dc, err := NewDatacenter(*client, v.Config.Datacenter)
	if err != nil {
		return nil, err
	}

	err = dc.Lookup()
	if err != nil {
		return nil, err
	}

	folder, err := NewFolder(*client, dc.Datacenter, v.Config.Folder)
	if err != nil {
		return nil, err
	}

	err = folder.Lookup()
	if err != nil {
		return nil, err
	}

	rp, err := NewResourcePool(*client, dc.Datacenter, v.Config.ResourcePool)
	if err != nil {
		return nil, err
	}

	err = rp.Lookup()
	if err != nil {
		return nil, err
	}

	ds, err := NewDatastore(*client, dc.Datacenter, v.Config.Datastore)
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

func (v *VSphereManager) ListVms(ctx context.Context, vClient *govmomi.Client, vmFolder string) ([]*VM, error) {
	vms := []*VM{}
	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		log.Printf("Failed to resolve resources Vms: %d", err)
		return nil, err
	}

	list, err := rc.datacenter.ListVms(ctx, "*")
	if err != nil {
		log.Printf("Failed to list Vms: %d", err)
		return nil, err
	}

	for _, vmiter := range list {
		log.Printf("Found VM: %s %s %s", vmiter.Name(), vmiter.Reference().Type, vmiter.Reference().Value)
		vm, err := v.LookupVm(ctx, vClient, vmiter.Name())
		if err == nil {
			log.Printf("Append VM: %s", vm.name)
			vms = append(vms, vm)
		}
	}

	return vms, nil
}

func (v *VSphereManager) LookupVm(ctx context.Context, vClient *govmomi.Client, vmName string) (*VM, error) {
	log.Printf("Lookup VM")
	rc, err := v.resolveResources(ctx, vClient)
	if err != nil {
		return nil, err
	}
	log.Printf("New VM")

	vm, err := NewVM(*vClient, rc.datacenter, vmName)
	if err != nil {
		return nil, err
	}
	log.Printf("VM.lookup")

	err = vm.Lookup()
	if err != nil {
		return nil, err
	}

	log.Printf("vm: %s path: %s", vm.name, vm.VirtualMachine.InventoryPath)

	return vm, nil
}

/*
func (v *VSphereManager) deleteVmInvoke(ctx context.Context, client *govmomi.Client, name string) (*object.Task, error) {
	rc, err := v.refreshResources(ctx, client)
	if err != nil {
		return nil, err
	}

	vm, err := NewVM(*client, rc.datacenter, name)
	if err != nil {
		return nil, err
	}

	err = vm.Lookup()
	if err != nil {
		return nil, err
	}

	return vm.Delete(ctx)
}

func (v *VSphereManager) DeleteVm(ctx context.Context, kClient client.Client, vClient *govmomi.Client, request reconcile.Request) error {
	task, err := v.deleteVmInvoke(ctx, vClient, request.Name)
	if err != nil {
		log.Printf("Failed to delete VM: %s", err.Error())
		return err
	}

	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		log.Printf("VM delete task failed %s", err.Error())
		return err
	}

	return nil
}

func (v *VSphereManager) createVmInvoke(ctx context.Context, client *govmomi.Client, rc *ResourceContext, vmSpec vimTypes.VirtualMachineConfigSpec) (*object.Task, error) {

	vm, err := NewVM(*client, rc.datacenter, vmSpec.Name)
	if err != nil {
		return nil, err
	}

	vmSpec.Files = &vimTypes.VirtualMachineFileInfo{
		VmPathName: fmt.Sprintf("[%s]", rc.datastore.Datastore.Name()),
	}

	return vm.Create(ctx, rc.folder.Folder, rc.resourcePool.ResourcePool, vmSpec)
}

func (v *VSphereManager) updateVmStatus(ctx context.Context, kClient client.Client, instance *vmv1.VM, vm *VM) error {
	instance.Status.State = "Created"
	ps, err := vm.VirtualMachine.PowerState(ctx)
	if err != nil {
		return err
	}

	instance.Status.RuntimeStatus.PowerState = string(ps)
	//instance.Status.PowerStatus = string(ps)
	//instance.Status.Created = vm.VirtualMachine.

	err = kClient.Status().Update(context.Background(), instance)
	if err != nil {
		log.Printf("Update failed: %s", err.Error())
		return err
	}
	return nil
}

func (v *VSphereManager) CreateVm(ctx context.Context, kClient client.Client, vClient *govmomi.Client, request reconcile.Request, instance *vmv1.VM) (*VM, error) {
	rc, err := v.refreshResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	vmSpec := vimTypes.VirtualMachineConfigSpec{
		Name:     request.Name,
		NumCPUs:  int32(instance.Spec.CpuReqs.CpuCount),
		MemoryMB: int64(instance.Spec.MemoryReqs.MemoryCapacity),
	}

	task, err := v.createVmInvoke(ctx, vClient, rc, vmSpec)
	if err != nil {
		log.Printf("Failed to create VM: %s", err.Error())
		return nil, err
	}

	//info, err := task.WaitForResult(ctx, nil)
	_, err = task.WaitForResult(ctx, nil)
	if err != nil {
		log.Printf("VM Create task failed %s", err.Error())
		return nil, err
	}

	vm, err := NewVM(*vClient, rc.datacenter, vmSpec.Name)
	if err != nil {
		return nil, err
	}

	// DWB: Need resolve from info rather than lookup
	err = vm.Lookup()
	if err != nil {
		return nil, err
	}

	err = v.updateVmStatus(ctx, kClient, instance, vm)
	if err != nil {
		return nil, err
	}

	log.Printf("Created VM %s!", vmSpec.Name)
	return vm, nil

	//return object.NewVirtualMachine(client.Client, info.Result.(vimTypes.ManagedObjectReference)), nil
}

func (v *VSphereManager) updatePowerState(ctx context.Context, instance *vmv1.VM, vm *VM) error {


	ps, err := vm.VirtualMachine.PowerState(ctx)
	if err != nil {
		log.Printf("Failed to acquire power state: %s", err.Error())
		return err
	}

	log.Printf("Current power state: %s", ps)

	if string(ps) != string(instance.Spec.PowerState) {
		// Bring PowerState into conformance
		var task *object.Task
		switch instance.Spec.PowerState {
		case vmv1.PoweredOff:
			task, err = vm.VirtualMachine.PowerOff(ctx)
		case vmv1.PoweredOn:
			task, err = vm.VirtualMachine.PowerOn(ctx)
		}

		if err != nil {
			log.Printf("Failed to change power state to %s", instance.Spec.PowerState)
			return err
		}

		_, err = task.WaitForResult(ctx, nil)
		if err != nil {
			log.Printf("VM Power State change task failed %s", err.Error())
			return err
		}
	} else {
		log.Printf("Power state already at desired state of %s", ps)
	}

	return nil
}

func (v *VSphereManager) UpdateVm(ctx context.Context, kClient client.Client, vClient *govmomi.Client, request reconcile.Request, instance *vmv1.VM, vm *VM) (*VM, error) {
	// Diff instance with VM config on backend
	// DWB: Make this a table of prop actors
	// Update VM Config first
	// Perform Power Ops second

	_, err := v.refreshResources(ctx, vClient)
	if err != nil {
		return nil, err
	}

	//vmSpec := vimTypes.VirtualMachineConfigSpec{
	//}
	err = v.updatePowerState(ctx, instance, vm)
	if err != nil {
		return nil, err
	}

	err = v.updateVmStatus(ctx, kClient, instance, vm)
	if err != nil {
		return nil, err
	}

	log.Printf("Udpated VM %s!", request.Name)
	return vm, nil
}
*/
