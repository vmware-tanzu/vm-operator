// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// BackfillNICDevicePropertiesFromMoVM populates device-level NIC properties
// that are NOT stored in ExtraConfig:
//
//   - spec.network.interfaces[i].vNUMANodeID from VirtualDevice.NumaNode
//     (positive values only; 0 is indistinguishable from "not set" via XML
//     omitempty, so it is skipped).
//
//   - spec.network.interfaces[i].vmxnet3.UPTv2Enabled from
//     VirtualVmxnet3.Uptv2Enabled (VMXNet3 or type-unset interfaces only).
//
// Index alignment: spec interface[i] corresponds to the i-th ethernet device
// in moVM.Config.Hardware.Device (same convention as
// FillEmptyNetworkInterfaceTypesFromMoVM). Devices beyond the spec interface
// count are ignored; spec interfaces beyond the device count are untouched.
//
// Existing (non-nil) spec values are never overwritten (spec wins).
// Returns true if any field was mutated.
func BackfillNICDevicePropertiesFromMoVM(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (bool, error) {

	if moVM.Config == nil {
		return false, nil
	}
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	ethDevs := collectEthernetDevicesFromMoVM(moVM)
	mutated := false

	for i := range vm.Spec.Network.Interfaces {
		if i >= len(ethDevs) {
			break
		}
		iface := &vm.Spec.Network.Interfaces[i]
		dev := ethDevs[i]

		if m := backfillVNUMANodeID(iface, dev); m {
			mutated = true
		}
		if m := backfillUPTv2Enabled(iface, dev); m {
			mutated = true
		}
	}

	return mutated, nil
}

// backfillVNUMANodeID populates iface.VNUMANodeID from dev.NumaNode when the
// device reports a positive NUMA node assignment and the spec field is nil.
func backfillVNUMANodeID(
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	dev vimtypes.BaseVirtualDevice) bool {

	if iface.VNUMANodeID != nil {
		return false // spec wins
	}

	numaNode := dev.GetVirtualDevice().NumaNode
	if numaNode <= 0 {
		// 0 → indistinguishable from "not set" due to XML omitempty.
		// Negative → explicitly no affinity.
		// TODO: Need a fix from govmomi.
		return false
	}

	iface.VNUMANodeID = &numaNode
	return true
}

// backfillUPTv2Enabled populates iface.vmxnet3.UPTv2Enabled from
// VirtualVmxnet3.Uptv2Enabled when the device has it set and the spec
// field is nil. Only applies to VMXNet3 (or type-unset) interfaces.
func backfillUPTv2Enabled(
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	dev vimtypes.BaseVirtualDevice) bool {

	vmxnet3Dev, ok := dev.(*vimtypes.VirtualVmxnet3)
	if !ok || vmxnet3Dev.Uptv2Enabled == nil {
		return false
	}

	// UPTv2 is a VMXNet3-only feature.
	if iface.Type != "" &&
		iface.Type != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		return false
	}

	if iface.VMXNet3 != nil && iface.VMXNet3.UPTv2Enabled != nil {
		return false // spec wins
	}

	if iface.VMXNet3 == nil {
		iface.VMXNet3 = &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{}
	}

	v := *vmxnet3Dev.Uptv2Enabled
	iface.VMXNet3.UPTv2Enabled = &v
	return true
}

// collectEthernetDevicesFromMoVM returns the ethernet devices from moVM in
// the order they appear in Config.Hardware.Device.
func collectEthernetDevicesFromMoVM(
	moVM mo.VirtualMachine) []vimtypes.BaseVirtualDevice {

	if moVM.Config == nil {
		return nil
	}

	devs := make([]vimtypes.BaseVirtualDevice, 0,
		len(moVM.Config.Hardware.Device))

	for _, dev := range moVM.Config.Hardware.Device {
		if pkgutil.IsEthernetCard(dev) {
			devs = append(devs, dev)
		}
	}
	return devs
}
