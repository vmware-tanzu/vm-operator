// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

func (s *Session) DeleteVirtualMachine(vmCtx context.VirtualMachineContext) error {
	resVM, err := s.GetVirtualMachine(vmCtx)
	if err != nil {
		return transformVMError(vmCtx.VM.NamespacedName(), err)
	}

	moVM, err := resVM.GetProperties(vmCtx, []string{"summary.runtime"})
	if err != nil {
		return err
	}

	if moVM.Summary.Runtime.PowerState != vimTypes.VirtualMachinePowerStatePoweredOff {
		if err := resVM.SetPowerState(vmCtx, vmopv1alpha1.VirtualMachinePoweredOff); err != nil {
			return err
		}
	}

	return resVM.Delete(vmCtx)
}

func (s *Session) GetVirtualMachineGuestHeartbeat(vmCtx context.VirtualMachineContext) (vmopv1alpha1.GuestHeartbeatStatus, error) {
	resVM, err := s.GetVirtualMachine(vmCtx)
	if err != nil {
		return "", transformVMError(vmCtx.VM.NamespacedName(), err)
	}

	moVM, err := resVM.GetProperties(vmCtx, []string{"guestHeartbeatStatus"})
	if err != nil {
		return "", err
	}

	return vmopv1alpha1.GuestHeartbeatStatus(moVM.GuestHeartbeatStatus), nil
}

func (s *Session) GetVirtualMachineWebMKSTicket(vmCtx context.VirtualMachineContext, pubKey string) (string, error) {
	resVM, err := s.GetVirtualMachine(vmCtx)
	if err != nil {
		return "", transformVMError(vmCtx.VM.NamespacedName(), err)
	}

	ticket, err := resVM.GetWebMKSTicket(vmCtx)
	if err != nil {
		return "", err
	}

	return EncryptWebMKS(pubKey, ticket)
}

func updateVirtualDiskDeviceChanges(
	vmCtx context.VirtualMachineContext,
	virtualDisks object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	// XXX (dramdass): Right now, we only resize disks that exist in the VM template. The disks
	// are keyed by deviceKey and the desired new size must be larger than the original size.
	// The number of disks is expected to be O(1) so we the nested loop is ok here.
	var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	for _, volume := range vmCtx.VM.Spec.Volumes {
		if volume.VsphereVolume == nil || volume.VsphereVolume.DeviceKey == nil {
			continue
		}

		deviceKey := int32(*volume.VsphereVolume.DeviceKey)
		found := false

		for _, vmDevice := range virtualDisks {
			vmDisk, ok := vmDevice.(*vimTypes.VirtualDisk)
			if !ok || vmDisk.GetVirtualDevice().Key != deviceKey {
				continue
			}

			newCapacityInBytes := volume.VsphereVolume.Capacity.StorageEphemeral().Value()
			if newCapacityInBytes < vmDisk.CapacityInBytes {
				// TODO Could be nice if the validating webhook would check this, but we
				// have a long ways before the provider can be used from there, if even a good idea.
				err := errors.Errorf("cannot shrink disk with device key %d from %d bytes to %d bytes",
					deviceKey, vmDisk.CapacityInBytes, newCapacityInBytes)
				return nil, err
			}

			if vmDisk.CapacityInBytes < newCapacityInBytes {
				vmDisk.CapacityInBytes = newCapacityInBytes
				deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationEdit,
					Device:    vmDisk,
				})
			}

			found = true
			break
		}

		if !found {
			return nil, errors.Errorf("could not find volume with device key %d", deviceKey)
		}
	}

	return deviceChanges, nil
}
