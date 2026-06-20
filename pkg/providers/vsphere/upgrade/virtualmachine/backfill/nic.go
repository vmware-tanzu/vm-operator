// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"
	"reflect"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// NICConfigFromMoVM populates per-NIC spec fields from the live vSphere
// VM configuration during schema upgrade. For each zipped (spec interface,
// ethernet device) pair:
//
//   - spec.network.interfaces[i].type: set from the device type when empty,
//     defaulting to VMXNet3 for unrecognised device types.
//   - spec.network.interfaces[i].vmxnet3.*: backfilled from moVM.Config.ExtraConfig
//     using the device-key-derived ethernetX prefix.
//   - spec.network.interfaces[i].vnumaNodeID: filled from VirtualDevice.NumaNode.
//   - spec.network.interfaces[i].vmxnet3.UPTv2Enabled: filled from
//     VirtualVmxnet3.Uptv2Enabled.
//
// Spec interfaces are zipped by position to ethernet devices. Spec interfaces
// beyond the device count are not modified.
//
// TODO(BV): Do actual matching between the spec and devices, but for
// now just zip together.
//
// Spec wins: only nil/zero fields are written.
// Returns true if any field was mutated.
func NICConfigFromMoVM(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) bool {

	if moVM.Config == nil {
		return false
	}
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false
	}

	ethDevs := collectEthernetDevicesFromMoVM(moVM)
	mutated := false

	for i := range vm.Spec.Network.Interfaces {
		iface := &vm.Spec.Network.Interfaces[i]

		if i >= len(ethDevs) {
			// No matching hardware device: default Type to VMXNet3 if unset.
			if iface.Type == "" {
				iface.Type = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
				mutated = true
			}
			continue
		}

		dev := ethDevs[i]

		if iface.Type == "" {
			t := mapVimEthernetToNetworkInterfaceType(dev)
			if t == "" {
				t = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
			}
			iface.Type = t
			mutated = true
		}

		if backfillNICSpec(
			ctx,
			vmopv1util.EthernetExtraConfigPrefix(dev.GetVirtualDevice().Key),
			iface,
			moVM.Config.ExtraConfig) {
			mutated = true
		}

		if backfillVNUMANodeID(iface, dev) {
			mutated = true
		}
		if backfillUPTv2Enabled(iface, dev) {
			mutated = true
		}
	}

	return mutated
}

func mapVimEthernetToNetworkInterfaceType(
	dev vimtypes.BaseVirtualDevice) vmopv1.VirtualMachineNetworkInterfaceType {

	switch dev.(type) {
	case *vimtypes.VirtualVmxnet3:
		return vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
	case *vimtypes.VirtualSriovEthernetCard:
		return vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
	case *vimtypes.VirtualE1000:
		return vmopv1.VirtualMachineNetworkInterfaceTypeE1000
	case *vimtypes.VirtualE1000e:
		return vmopv1.VirtualMachineNetworkInterfaceTypeE1000e
	case *vimtypes.VirtualVmxnet2:
		return vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet2
	case *vimtypes.VirtualPCNet32:
		return vmopv1.VirtualMachineNetworkInterfaceTypePCNet32
	default:
		return ""
	}
}

// backfillNICSpec backfills vmxnet3.* spec fields from ExtraConfig using the
// given prefix (e.g. "ethernet0."). Only applies to VMXNet3 NICs.
func backfillNICSpec(
	ctx context.Context,
	prefix string,
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	extraConfig []vimtypes.BaseOptionValue) bool {

	// Only backfill vmxnet3 fields for VMXNet3 NICs.
	if iface.Type != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		return false
	}
	log := pkglog.FromContextOrDefault(ctx)
	mutated := false

	for _, bov := range extraConfig {
		ov, ok := bov.(*vimtypes.OptionValue)
		if !ok {
			continue
		}

		if !strings.HasPrefix(ov.Key, prefix) {
			continue
		}

		raw, ok := ov.Value.(string)
		if !ok {
			continue
		}

		fieldIdx, exists := vmopv1util.VMXNet3NICKeyMap()[vmopv1util.NormalizeEthernetDeviceKey(ov.Key)]
		if !exists {
			continue
		}

		// Spec wins: skip if the field is already non-zero.
		if iface.VMXNet3 != nil {
			rv := reflect.ValueOf(iface.VMXNet3).Elem().Field(fieldIdx)
			if !rv.IsZero() {
				continue
			}
		}

		// Decode into a nicSpec struct to avoid initialising iface.VMXNet3
		// prematurely. If the raw value is a host-default sentinel (auto,
		// default, dontcare) the decoded field stays zero and we skip without
		// touching spec — preserving the nil=auto convention for *bool fields.
		var nicSpec vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec
		nicSpecFieldValue := reflect.ValueOf(&nicSpec).Elem().Field(fieldIdx)
		if err := vmopv1util.DecodeVMXFieldValue(ctx, nicSpecFieldValue, raw); err != nil {
			log.V(1).Error(err, "cannot decode vmx nic field; skipping", "key", ov.Key)
			continue
		}
		if nicSpecFieldValue.IsZero() {
			continue
		}

		if iface.VMXNet3 == nil {
			iface.VMXNet3 = &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{}
		}
		reflect.ValueOf(iface.VMXNet3).Elem().Field(fieldIdx).Set(nicSpecFieldValue)
		mutated = true
	}
	return mutated
}

// backfillVNUMANodeID populates iface.VNUMANodeID from dev.NumaNode when the
// device reports a NUMA node assignment (>= 0) and the spec field is nil.
func backfillVNUMANodeID(
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	dev vimtypes.BaseVirtualDevice) bool {

	if iface.VNUMANodeID != nil {
		return false // spec wins
	}

	numaNode := dev.GetVirtualDevice().NumaNode
	if numaNode == nil || *numaNode < 0 {
		// nil → unset (no NUMA assignment, or cleared and read back as absent).
		// Negative → explicitly no affinity.
		return false
	}

	iface.VNUMANodeID = numaNode
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
