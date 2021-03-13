// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VirtualMachine struct {
	Name             string
	vcVirtualMachine *object.VirtualMachine
	logger           logr.Logger
}

var log = logf.Log.WithName("vmresource")

// NewVMForCreate returns a VirtualMachine that Create() can be called on
// to create the VM and set the VirtualMachine object reference.
func NewVMForCreate(name string) *VirtualMachine {
	return &VirtualMachine{
		Name:   name,
		logger: log.WithValues("name", name),
	}
}

func NewVMFromObject(objVm *object.VirtualMachine) (*VirtualMachine, error) {
	return &VirtualMachine{
		Name:             objVm.Name(),
		vcVirtualMachine: objVm,
		logger:           log.WithValues("name", objVm.Name()),
	}, nil
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

func (vm *VirtualMachine) Delete(ctx context.Context) error {
	vm.logger.V(5).Info("Delete VM")

	if vm.vcVirtualMachine == nil {
		return fmt.Errorf("failed to delete VM because the VM object is not set")
	}

	destroyTask, err := vm.vcVirtualMachine.Destroy(ctx)
	if err != nil {
		return err
	}

	_, err = destroyTask.WaitForResult(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "delete VM task failed")
	}

	return nil
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

// IpAddress returns the IpAddress of the VM if powered on, error otherwise
func (vm *VirtualMachine) IpAddress(ctx context.Context) (string, error) {
	vm.logger.V(5).Info("Get IpAddress")

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil || ps == types.VirtualMachinePowerStatePoweredOff {
		return "", err
	}

	// Just get some IP from guest
	var o mo.VirtualMachine
	err = vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"guest.ipAddress"}, &o)
	if err != nil {
		return "", err
	}

	if o.Guest == nil {
		vm.logger.Info("VM GuestInfo is empty")
		return "", &find.NotFoundError{}
	}

	return o.Guest.IpAddress, nil
}

// IsGuestCustomizationPending checks if a VM has a pending guest customization.
func (vm *VirtualMachine) IsGuestCustomizationPending(ctx context.Context) (bool, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.extraConfig"}, &o)
	if err != nil {
		vm.logger.Error(err, "Error getting VM config.ExtraConfig properties")
		return false, err
	}

	for _, opt := range o.Config.ExtraConfig {
		if optValue := opt.GetOptionValue(); optValue.Key == "tools.deployPkg.fileName" {
			fileName := optValue.Value.(string)
			if fileName != "" {
				return true, nil
			}
		}
	}

	return false, nil
}

func (vm *VirtualMachine) GetConfig(ctx context.Context) (*types.VirtualMachineConfigInfo, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config"}, &o)
	if err != nil {
		vm.logger.Error(err, "Error getting VM config properties")
		return nil, err
	}

	return o.Config, nil
}

func (vm *VirtualMachine) InstanceUUID(ctx context.Context) (string, error) {
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.instanceUuid"}, &o)
	if err != nil {
		return "", err
	}

	return o.Config.InstanceUuid, nil
}

func (vm *VirtualMachine) BiosUUID(ctx context.Context) (string, error) {
	return vm.vcVirtualMachine.UUID(ctx), nil
}

func (vm *VirtualMachine) ResourcePool(ctx context.Context) (string, error) {
	rp, err := vm.vcVirtualMachine.ResourcePool(ctx)
	if err != nil {
		return "", err
	}
	return rp.Reference().Value, nil
}

func (vm *VirtualMachine) ChangeTrackingEnabled(ctx context.Context) (*bool, error) {
	configInfo, err := vm.GetConfigInfo(ctx)
	if err != nil || configInfo == nil {
		return nil, err
	}
	return configInfo.ChangeTrackingEnabled, nil
}

func (vm *VirtualMachine) ReferenceValue() string {
	vm.logger.V(5).Info("Get ReferenceValue")
	return vm.vcVirtualMachine.Reference().Value
}

func (vm *VirtualMachine) ManagedObject(ctx context.Context) (*mo.VirtualMachine, error) {
	vm.logger.V(5).Info("Get ManagedObject")
	var props mo.VirtualMachine
	if err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), nil, &props); err != nil {
		return nil, err
	}
	return &props, nil
}

func (vm *VirtualMachine) ImageFields(ctx context.Context) (powerState, uuid, reference string) {
	vm.logger.V(5).Info("ImageFields")
	ps, _ := vm.vcVirtualMachine.PowerState(ctx)

	powerState = string(ps)
	uuid = vm.vcVirtualMachine.UUID(ctx)
	reference = vm.ReferenceValue()
	return
}

// GetCreationTime returns the creation time of the VM
func (vm *VirtualMachine) GetCreationTime(ctx context.Context) (*time.Time, error) {
	vm.logger.V(5).Info("GetCreationTime")
	var o mo.VirtualMachine

	err := vm.vcVirtualMachine.Properties(ctx, vm.vcVirtualMachine.Reference(), []string{"config.createDate"}, &o)
	if err != nil {
		return nil, err
	}

	if o.Config == nil {
		return &time.Time{}, nil
	}

	return o.Config.CreateDate, nil
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
	// (https://confluence.eng.vmware.com/display/VCB/StableMOID+Architecture?src=contextnavpagetreemode ) So this
	// ensures that after restore we get new MoIds which are never used before… (we advance the sequence counter based on time)
	//
	// Discovery of VMs: We only use moids from the VM store during restore from a backup. In the unlikely event
	// that there are two VMs which are presenting the same MoId, we will regenerate a new MoId based on the current
	// sequence. Keep in mind, Ideally the unlikely scenario should not occur as we attempt to tamper proof the MoId
	// stored in the VM store (https://confluence.eng.vmware.com/display/VCB/Bauhaus+VM+Store+security+and+VM+config+file+access?src=contextnavpagetreemode )
	// so two VMs having the same MoId should not happen because they cannot have the same VMX path and we use VMX path
	// for ensuring this tamper proof behavior.
	//
	// Removing from VC and Re-adding the VM to same VC: VM will be given a new MoId (even if the VM is added using
	// RegisterVM operation from VC)
	// Basically, lifetime of the identity is tied to VC’s knowledge of it’s existence in it’s inventory
	return vm.ReferenceValue(), nil
}

// GetStatus returns a VirtualMachine's Status
func (vm *VirtualMachine) GetStatus(ctx context.Context) (*v1alpha1.VirtualMachineStatus, error) {
	vm.logger.V(5).Info("GetStatus")
	// TODO(bryanv) We should get all the needed fields in one call to VC.

	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PowerState for VirtualMachine: %s", vm.Name)
	}

	host, err := vm.vcVirtualMachine.HostSystem(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM HostSystem for VirtualMachine: %s", vm.Name)
	}

	// Use ObjectName instead of Name to fetch hostname because ... ???
	hostname, err := host.ObjectName(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get VM hostname for VirtualMachine: %s", vm.Name)
	}

	uniqueId, err := vm.UniqueID(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate a unique id for VirtualMachine: %s", vm.Name)
	}

	ip, err := vm.IpAddress(ctx)
	if err != nil {
		vm.logger.Error(err, "failed to get IP address for VirtualMachine")
		ip = ""
	}

	biosUUID, err := vm.BiosUUID(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get BiosUUID for VirtualMachine %s", vm.Name)
	}

	instanceUUID, err := vm.InstanceUUID(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get InstanceUUID for VirtualMachine %s", vm.Name)
	}

	cbt, err := vm.ChangeTrackingEnabled(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get ChangeTrackingEnabled for VirtualMachine %s", vm.Name)
	}

	return &v1alpha1.VirtualMachineStatus{
		Host:                hostname,
		Phase:               v1alpha1.Created,
		PowerState:          v1alpha1.VirtualMachinePowerState(ps),
		VmIp:                ip,
		UniqueID:            uniqueId,
		BiosUUID:            biosUUID,
		InstanceUUID:        instanceUUID,
		ChangeBlockTracking: cbt,
	}, nil
}

func (vm *VirtualMachine) GetConfigInfo(ctx context.Context) (*types.VirtualMachineConfigInfo, error) {
	vm.logger.V(5).Info("GetConfigInfo")
	moVM, err := vm.ManagedObject(ctx)
	if err != nil {
		return nil, err
	}
	return moVM.Config, nil
}

func (vm *VirtualMachine) GetVAppVmConfigInfo(ctx context.Context) (*types.VmConfigInfo, error) {
	vm.logger.V(5).Info("GetVAppVmConfigInfo")
	moVM, err := vm.ManagedObject(ctx)
	if err != nil {
		return nil, err
	}
	if moVM.Config.VAppConfig == nil {
		return nil, nil
	}
	return moVM.Config.VAppConfig.GetVmConfigInfo(), nil
}

func (vm *VirtualMachine) GetOvfProperties(ctx context.Context) (map[string]string, error) {
	vm.logger.V(5).Info("GetOvfProperties")
	vAppConfig, err := vm.GetVAppVmConfigInfo(ctx)
	if err != nil {
		return nil, err
	}

	properties := make(map[string]string)

	if vAppConfig != nil {
		props := vAppConfig.Property
		for _, prop := range props {
			if strings.HasPrefix(prop.Id, "vmware-system") {
				if prop.Value != "" {
					properties[prop.Id] = prop.Value
				} else {
					properties[prop.Id] = prop.DefaultValue
				}
			}
		}
	}

	return properties, nil
}

func (vm *VirtualMachine) IsVMPoweredOff(ctx context.Context) (bool, error) {
	vm.logger.V(5).Info("IsVMPoweredOff")
	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		return false, err
	}

	return ps == types.VirtualMachinePowerStatePoweredOff, nil
}

func (vm *VirtualMachine) GetPowerState(ctx context.Context) (types.VirtualMachinePowerState, error) {
	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		return "", err
	}

	return ps, nil
}

func (vm *VirtualMachine) SetPowerState(ctx context.Context, desiredPowerState v1alpha1.VirtualMachinePowerState) error {
	vm.logger.V(5).Info("SetPowerState", "desiredState", desiredPowerState)
	ps, err := vm.vcVirtualMachine.PowerState(ctx)
	if err != nil {
		vm.logger.Error(err, "Failed to get VM power state")
		return err
	}

	vm.logger.V(4).Info("VM power state", "currentState", ps, "desiredState", desiredPowerState)
	if v1alpha1.VirtualMachinePowerState(ps) == desiredPowerState {
		return nil
	}

	var powerTask *object.Task
	switch desiredPowerState {
	case v1alpha1.VirtualMachinePoweredOn:
		powerTask, err = vm.vcVirtualMachine.PowerOn(ctx)
	case v1alpha1.VirtualMachinePoweredOff:
		powerTask, err = vm.vcVirtualMachine.PowerOff(ctx)
	default:
		err = fmt.Errorf("invalid desired power state %s", desiredPowerState)
	}

	if err != nil {
		vm.logger.Error(err, "Failed to change VM power state", "desiredState", desiredPowerState)
		return err
	}

	_, err = powerTask.WaitForResult(ctx, nil)
	if err != nil {
		vm.logger.Error(err, "VM change power state task failed")
		return err
	}

	return nil
}

// GetVirtualDisks returns the list of VMs vmdks
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

	_, err = customizeTask.WaitForResult(ctx, nil)
	if err != nil {
		// Fetch fault messages for task.Error
		if taskErr, ok := err.(task.Error); ok {
			fault := taskErr.Fault().GetMethodFault()
			if fault != nil {
				err = errors.Wrapf(err, "Fault messages: %v", fault.FaultMessage)
			}
		}

		vm.logger.Error(err, "Customization task failed")
		return err
	}

	return nil
}
