// Copyright (c) 2018-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

type VirtualMachine struct {
	Name             string
	vcVirtualMachine *object.VirtualMachine
	logger           logr.Logger
}

var log = logf.Log.WithName("vmresource")

func NewVMFromObject(objVM *object.VirtualMachine) *VirtualMachine {
	return &VirtualMachine{
		Name:             objVM.Name(),
		vcVirtualMachine: objVM,
		logger:           log.WithValues("name", objVM.Name()),
	}
}

func (vm *VirtualMachine) VcVM() *object.VirtualMachine {
	return vm.vcVirtualMachine
}

func (vm *VirtualMachine) Create(ctx context.Context, folder *object.Folder, pool *object.ResourcePool, vmSpec *types.VirtualMachineConfigSpec) error {
	vm.logger.V(5).Info("Create VM")

	if vm.vcVirtualMachine != nil {
		return fmt.Errorf("failed to create VM %q because the VM object is already set", vm.Name)
	}

	createTask, err := folder.CreateVM(ctx, *vmSpec, pool, nil)
	if err != nil {
		return err
	}

	result, err := createTask.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "create VM %q task failed", vm.Name)
	}

	vm.vcVirtualMachine = object.NewVirtualMachine(folder.Client(), result.Result.(types.ManagedObjectReference))
	return nil
}

func (vm *VirtualMachine) Clone(ctx context.Context, folder *object.Folder, cloneSpec *types.VirtualMachineCloneSpec) (*types.ManagedObjectReference, error) {
	vm.logger.V(5).Info("Clone VM")

	cloneTask, err := vm.vcVirtualMachine.Clone(ctx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := cloneTask.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "clone VM task failed")
	}

	ref := result.Result.(types.ManagedObjectReference)
	return &ref, nil
}

func (vm *VirtualMachine) Reconfigure(ctx context.Context, configSpec *types.VirtualMachineConfigSpec) error {
	vm.logger.V(5).Info("Reconfiguring VM", "configSpec", configSpec)

	reconfigureTask, err := vm.vcVirtualMachine.Reconfigure(ctx, *configSpec)
	if err != nil {
		return err
	}

	_, err = reconfigureTask.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "reconfigure VM task failed")
	}

	return nil
}

func (vm *VirtualMachine) GetProperties(ctx context.Context, properties []string) (*mo.VirtualMachine, error) {
	var o mo.VirtualMachine
	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), properties, &o)
	if err != nil {
		vm.logger.Error(err, "Error getting VM properties")
		return nil, err
	}

	return &o, nil
}

func (vm *VirtualMachine) ReferenceValue() string {
	vm.logger.V(5).Info("Get ReferenceValue")
	return vm.vcVirtualMachine.Reference().Value
}

func (vm *VirtualMachine) MoRef() types.ManagedObjectReference {
	vm.logger.V(5).Info("Get MoRef")
	return vm.vcVirtualMachine.Reference()
}

func (vm *VirtualMachine) UniqueID(ctx context.Context) (string, error) {
	// Notes from Alkesh Shah regarding MoIDs in VC as of 7.0
	//
	// MoRef IDs are unique within the scope of a single VC. Since Clusters are entities in VCs, the MoRef IDs will be unique across clusters
	//
	// Identity in VC is derived from a sequence. This ID is used in generating the MoId (or MoRef ID) for the entity in VC. Sequence is monotonically
	// increasing and so during regular operation there are no dupes
	//
	// Backup-Restore: We now make sure that our sequence does not go back in time when restoring from a backup
	// ( ) So this
	// ensures that after restore we get new MoIds which are never used before… (we advance the sequence counter based on time)
	//
	// Discovery of VMs: We only use moids from the VM store during restore from a backup. In the unlikely event
	// that there are two VMs which are presenting the same MoId, we will regenerate a new MoId based on the current
	// sequence. Keep in mind, Ideally the unlikely scenario should not occur as we attempt to tamper proof the MoId
	// stored in the VM store ( )
	// so two VMs having the same MoId should not happen because they cannot have the same VMX path and we use VMX path
	// for ensuring this tamper proof behavior.
	//
	// Removing from VC and Re-adding the VM to same VC: VM will be given a new MoId (even if the VM is added using
	// RegisterVM operation from VC)
	// Basically, lifetime of the identity is tied to VC’s knowledge of it’s existence in it’s inventory
	return vm.ReferenceValue(), nil
}

func (vm *VirtualMachine) SetPowerState(
	ctx context.Context,
	currentPowerState,
	desiredPowerState vmopv1.VirtualMachinePowerState,
	desiredPowerOpMode vmopv1.VirtualMachinePowerOpMode) error {

	_, err := vmutil.SetAndWaitOnPowerState(
		ctx,
		vm.VcVM().Client(),
		mo.VirtualMachine{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: vm.VcVM().Reference(),
				},
			},
			Summary: types.VirtualMachineSummary{
				Runtime: types.VirtualMachineRuntimeInfo{
					PowerState: vmutil.ParsePowerState(string(currentPowerState)),
				},
			},
		},
		false,
		vmutil.ParsePowerState(string(desiredPowerState)),
		vmutil.ParsePowerOpMode(string(desiredPowerOpMode)))

	return err
}

// GetVirtualDevices returns the VMs VirtualDeviceList.
func (vm *VirtualMachine) GetVirtualDevices(ctx context.Context) (object.VirtualDeviceList, error) {
	vm.logger.V(5).Info("GetVirtualDevices")
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		vm.logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return deviceList, err
}

// GetVirtualDisks returns the list of VMs vmdks.
func (vm *VirtualMachine) GetVirtualDisks(ctx context.Context) (object.VirtualDeviceList, error) {
	vm.logger.V(5).Info("GetVirtualDisks")
	deviceList, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		vm.logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return deviceList.SelectByType((*types.VirtualDisk)(nil)), nil
}

func (vm *VirtualMachine) GetNetworkDevices(ctx context.Context) (object.VirtualDeviceList, error) {
	vm.logger.V(4).Info("GetNetworkDevices")
	devices, err := vm.vcVirtualMachine.Device(ctx)
	if err != nil {
		vm.logger.Error(err, "Failed to get devices for VM")
		return nil, err
	}

	return devices.SelectByType((*types.VirtualEthernetCard)(nil)), nil
}

func (vm *VirtualMachine) Customize(ctx context.Context, spec types.CustomizationSpec) error {
	vm.logger.V(5).Info("Customize", "spec", spec)

	customizeTask, err := vm.vcVirtualMachine.Customize(ctx, spec)
	if err != nil {
		vm.logger.Error(err, "Failed to customize VM")
		return err
	}

	taskInfo, err := customizeTask.WaitForResult(ctx, nil)
	if err != nil {
		vm.logger.Error(err, "Failed to wait for the result of Customize VM")
		return err
	}

	if taskErr := taskInfo.Error; taskErr != nil {
		// Fetch fault messages for task.Error
		fault := taskErr.Fault.GetMethodFault()
		if fault != nil {
			err = errors.Wrapf(err, "Fault messages: %v", fault.FaultMessage)
		}

		return errors.Wrap(err, "Customization task failed")
	}

	return nil
}
