package vsphere

import (
	"context"
	"errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

type VM struct {
	client         govmomi.Client
	name           string
	datacenter     *Datacenter
	VirtualMachine *object.VirtualMachine
	finder         *find.Finder
}

func NewVM(client govmomi.Client, datacenter *Datacenter, name string) (*VM, error) {
	return &VM{client: client, datacenter: datacenter, name: name}, nil
}

func (vm *VM) Lookup() error {

	if vm.finder == nil {
		vm.finder = find.NewFinder(vm.client.Client, false)
	}

	vm.finder.SetDatacenter(vm.datacenter.Datacenter)

	virtualMachine, err := vm.finder.VirtualMachine(context.TODO(), vm.name)
	if err != nil {
		return err
	}

	vm.VirtualMachine = virtualMachine

	return nil
}

func (vm *VM) Create(ctx context.Context, folder *object.Folder, rp *object.ResourcePool, vmSpec types.VirtualMachineConfigSpec) (*object.Task, error) {
	return folder.CreateVM(ctx, vmSpec, rp, nil)
}

func (vm *VM) Clone(ctx context.Context, sourceVm *object.VirtualMachine, folder *object.Folder, cloneSpec types.VirtualMachineCloneSpec) (*object.Task, error) {
	return sourceVm.Clone(ctx, folder, cloneSpec.Config.Name, cloneSpec)
}

func (vm *VM) Delete(ctx context.Context) (*object.Task, error) {
	if vm.VirtualMachine == nil {
		return nil, errors.New("VM is not set")
	}
	return vm.VirtualMachine.Destroy(ctx)
}
