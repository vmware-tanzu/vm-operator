// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

type VirtualMachine struct {
	Name             string
	vcVirtualMachine *object.VirtualMachine
}

func NewVMFromObject(objVM *object.VirtualMachine) *VirtualMachine {
	return &VirtualMachine{
		Name:             objVM.Name(),
		vcVirtualMachine: objVM,
	}
}

func (vm *VirtualMachine) VcVM() *object.VirtualMachine {
	return vm.vcVirtualMachine
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, pool *object.ResourcePool, vmSpec *vimtypes.VirtualMachineConfigSpec) error {
	pkglog.FromContextOrDefault(ctx).V(5).Info("Create VM")

	if vm.vcVirtualMachine != nil {
		return fmt.Errorf("failed to create VM %q because the VM object is already set", vm.Name)
	}

	createTask, err := folder.CreateVM(ctx, *vmSpec, pool, nil)
	if err != nil {
		return err
	}

	result, err := createTask.WaitForResult(ctx, nil)
	if err != nil {
		return fmt.Errorf("create VM %q task failed: %w", vm.Name, err)
	}

	vm.vcVirtualMachine = object.NewVirtualMachine(folder.Client(), result.Result.(vimtypes.ManagedObjectReference))
	return nil
}

func (vm *VirtualMachine) Clone(ctx context.Context, folder *object.Folder, cloneSpec *vimtypes.VirtualMachineCloneSpec) (*vimtypes.ManagedObjectReference, error) {
	pkglog.FromContextOrDefault(ctx).V(5).Info("Clone VM")

	cloneTask, err := vm.vcVirtualMachine.Clone(ctx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := cloneTask.WaitForResult(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("clone VM task failed: %w", err)
	}

	ref := result.Result.(vimtypes.ManagedObjectReference)
	return &ref, nil
}

func (vm *VirtualMachine) Reconfigure(
	ctx context.Context,
	configSpec *vimtypes.VirtualMachineConfigSpec) (*vimtypes.TaskInfo, error) {

	ctxop.MarkUpdate(ctx)

	logger := pkglog.FromContextOrDefault(ctx)
	logger.Info("Reconfiguring VM", "configSpec", pkgutil.SafeConfigSpecToString(configSpec))

	reconfigureTask, err := vm.vcVirtualMachine.Reconfigure(ctx, *configSpec)
	if err != nil {
		return nil, err
	}

	taskInfo, err := reconfigureTask.WaitForResult(ctx, nil)
	if err != nil {
		return taskInfo, fmt.Errorf("reconfigure VM task failed: %w", err)
	}

	return taskInfo, nil
}

func (vm *VirtualMachine) GetProperties(ctx context.Context, properties []string) (*mo.VirtualMachine, error) {
	var o mo.VirtualMachine
	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), properties, &o)
	if err != nil {
		pkglog.FromContextOrDefault(ctx).Error(err, "Error getting VM properties")
		return nil, err
	}

	return &o, nil
}

var ErrSetPowerState = pkgerr.NoRequeueNoErr("updated power state")

func (vm *VirtualMachine) SetPowerState(
	ctx context.Context,
	currentPowerState,
	desiredPowerState vmopv1.VirtualMachinePowerState,
	desiredPowerOpMode vmopv1.VirtualMachinePowerOpMode) error {

	result, err := vmutil.SetAndWaitOnPowerState(
		ctx,
		vm.VcVM().Client(),
		mo.VirtualMachine{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: vm.VcVM().Reference(),
				},
			},
			Summary: vimtypes.VirtualMachineSummary{
				Runtime: vimtypes.VirtualMachineRuntimeInfo{
					PowerState: vmutil.ParsePowerState(string(currentPowerState)),
				},
			},
		},
		false,
		vmutil.ParsePowerState(string(desiredPowerState)),
		vmutil.ParsePowerOpMode(string(desiredPowerOpMode)))

	if err != nil {
		return err
	}

	if result.AnyChange() {
		ctxop.MarkUpdate(ctx)
		return ErrSetPowerState
	}

	return nil
}

// GetVirtualDevices returns the VMs VirtualDeviceList.
func (vm *VirtualMachine) GetVirtualDevices(ctx context.Context) (object.VirtualDeviceList, error) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("GetVirtualDevices")
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return deviceList, err
}

// GetVirtualDisks returns the list of VMs vmdks.
func (vm *VirtualMachine) GetVirtualDisks(ctx context.Context) (object.VirtualDeviceList, error) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("GetVirtualDisks")
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return deviceList.SelectByType((*vimtypes.VirtualDisk)(nil)), nil
}

func (vm *VirtualMachine) GetNetworkDevices(ctx context.Context) (object.VirtualDeviceList, error) {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("GetNetworkDevices")
	devices, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil)), nil
}

func (vm *VirtualMachine) Customize(ctx context.Context, spec vimtypes.CustomizationSpec) error {
	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(5).Info("Customize", "spec", spec)

	ctxop.MarkUpdate(ctx)

	customizeTask, err := vm.vcVirtualMachine.Customize(ctx, spec)
	if err != nil {
		logger.Error(err, "Failed to customize VM")
		return err
	}

	taskInfo, err := customizeTask.WaitForResult(ctx, nil)
	if err != nil {
		logger.Error(err, "Failed to wait for the result of Customize VM")
		return err
	}

	if taskErr := taskInfo.Error; taskErr != nil {
		// Fetch fault messages for task.Error
		fault := taskErr.Fault.GetMethodFault()
		if fault != nil {
			err = fmt.Errorf("fault messages: %v: %w", fault.FaultMessage, err)
		}

		return fmt.Errorf("customization task failed: %w", err)
	}

	return nil
}
