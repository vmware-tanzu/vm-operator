// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"math"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

type VMContext struct {
	context.Context
	Logger logr.Logger
	VM     *vmopv1alpha1.VirtualMachine
}

type VMCloneContext struct {
	VMContext
	ResourcePool        *object.ResourcePool
	Folder              *object.Folder
	StorageProvisioning string
}

type VMUpdateContext struct {
	VMContext
	IsOff bool
}

func memoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func CpuQuantityToMhz(q resource.Quantity, cpuFreqMhz uint64) int64 {
	return int64(math.Ceil(float64(q.MilliValue()) * float64(cpuFreqMhz) / float64(1000)))
}

// Prepare a vApp VmConfigSpec which will set the vmMetadata supplied key/value fields. Only
// fields marked userConfigurable and pre-existing on the VM (ie. originated from the OVF Image)
// will be set, and all others will be ignored.
func GetMergedvAppConfigSpec(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo) *vimTypes.VmConfigSpec {
	var outProps []vimTypes.VAppPropertySpec

	for _, vmProp := range vmProps {
		if vmProp.UserConfigurable == nil || !*vmProp.UserConfigurable {
			continue
		}

		inPropValue, found := inProps[vmProp.Id]
		if !found || vmProp.Value == inPropValue {
			continue
		}

		vmPropCopy := vmProp
		vmPropCopy.Value = inPropValue
		outProp := vimTypes.VAppPropertySpec{
			ArrayUpdateSpec: vimTypes.ArrayUpdateSpec{
				Operation: vimTypes.ArrayUpdateOperationEdit,
			},
			Info: &vmPropCopy,
		}
		outProps = append(outProps, outProp)
	}

	if len(outProps) == 0 {
		return nil
	}

	return &vimTypes.VmConfigSpec{Property: outProps}
}

func resizeVirtualDisksDeviceChanges(
	vmCtx VMContext,
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

// CreateCustomizationSpec creates the customization spec for the vm
func (s *Session) CreateCustomizationSpec(
	vmCtx VMContext,
	resVM *res.VirtualMachine,
	dnsServers []string,
	nicCustomizations []vimTypes.CustomizationAdapterMapping) (*vimTypes.CustomizationSpec, error) {

	customSpec := &vimTypes.CustomizationSpec{
		// This spec is for Linux guest OS. Need to change if other guest OS needs to be supported.
		Identity: &vimTypes.CustomizationLinuxPrep{
			HostName: &vimTypes.CustomizationFixedName{
				Name: vmCtx.VM.Name,
			},
			HwClockUTC: vimTypes.NewBool(true),
		},
		GlobalIPSettings: vimTypes.CustomizationGlobalIPSettings{
			DnsServerList: dnsServers,
		},
	}

	if len(vmCtx.VM.Spec.NetworkInterfaces) == 0 {
		// In the corresponding code in cloneVMNicDeviceChanges(), none of the existing interfaces were removed,
		// so GetNetworkDevices() will give us all the original interfaces. Assume they should be
		// configured for DHCP since that was the behavior of the prior code. The config is currently
		// really only used in the test environments.
		netDevices, err := resVM.GetNetworkDevices(vmCtx)
		if err != nil {
			return nil, err
		}

		var interfaceCustomizations []vimTypes.CustomizationAdapterMapping
		for _, dev := range netDevices {
			card, ok := dev.(vimTypes.BaseVirtualEthernetCard)
			if !ok {
				continue
			}

			interfaceCustomizations = append(interfaceCustomizations, vimTypes.CustomizationAdapterMapping{
				MacAddress: card.GetVirtualEthernetCard().MacAddress,
				Adapter: vimTypes.CustomizationIPSettings{
					Ip: &vimTypes.CustomizationDhcpIpGenerator{},
				},
			})
		}
		customSpec.NicSettingMap = interfaceCustomizations
	} else {
		customSpec.NicSettingMap = nicCustomizations
	}

	return customSpec, nil
}
