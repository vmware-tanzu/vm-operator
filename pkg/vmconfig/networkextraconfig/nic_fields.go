// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig

import (
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// nicFieldDef describes how to observe and reconcile a single NIC-level
// device-spec field (e.g. Uptv2Enabled, NumaNode) via ConfigSpec.DeviceChange.
type nicFieldDef struct {
	// fieldName is the spec path shown in condition messages.
	fieldName string
	// minHWVer is the minimum hardware version required; 0 means no gate.
	minHWVer vimtypes.HardwareVersion
	// prerequisite, if non-nil, checks runtime conditions (EFI firmware,
	// vNUMA topology) beyond the hardware version. Called after differs=true
	// and after the hwVer gate passes. Returns (true, reason) when blocked.
	prerequisite func(
		vm vmopv1.VirtualMachine,
		iface vmopv1.VirtualMachineNetworkInterfaceSpec,
		dev vimtypes.BaseVirtualDevice,
		ci vimtypes.VirtualMachineConfigInfo,
	) (blocked bool, reason string)
	// hotPluggable reports whether the field can be applied while powered on.
	hotPluggable func(
		vm vmopv1.VirtualMachine,
		iface vmopv1.VirtualMachineNetworkInterfaceSpec,
		dev vimtypes.BaseVirtualDevice,
		ci vimtypes.VirtualMachineConfigInfo,
	) bool
	// differs returns true when a DeviceChange Edit is needed for this field.
	differs func(
		vm vmopv1.VirtualMachine,
		iface vmopv1.VirtualMachineNetworkInterfaceSpec,
		dev vimtypes.BaseVirtualDevice,
		ci vimtypes.VirtualMachineConfigInfo,
		cs *vimtypes.VirtualMachineConfigSpec,
	) bool
	// apply writes the desired value to cs.DeviceChange.
	apply func(
		vm vmopv1.VirtualMachine,
		iface vmopv1.VirtualMachineNetworkInterfaceSpec,
		dev vimtypes.BaseVirtualDevice,
		ci vimtypes.VirtualMachineConfigInfo,
		cs *vimtypes.VirtualMachineConfigSpec,
	)
}

// nicFields is the ordered list of NIC device-spec fields managed by
// this reconciler. Adding a new field requires only a new entry here.
var nicFields = []nicFieldDef{
	{
		fieldName: "interfaces[i].vmxnet3.uptv2Enabled",
		minHWVer:  vimtypes.VMX20,
		// UPTv2 requires full memory reservation (reservationLockedToMax or
		// explicit requests.memory == size.memory).  Only block when the
		// desired value is true — disabling UPTv2 never requires reservation.
		prerequisite: func(vm vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, _ vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo) (bool, string) {
			if !desiredUPTv2Enabled(iface) {
				return false, ""
			}
			if vmopv1util.FullMemReservationSpecMet(&vm) {
				return false, ""
			}
			return true, "full memory reservation required: " +
				"set spec.memoryAdvanced.reservationLockedToMax=true, " +
				"or set spec.resources.requests.memory equal to spec.resources.size.memory"
		},
		hotPluggable: func(_ vmopv1.VirtualMachine, _ vmopv1.VirtualMachineNetworkInterfaceSpec, _ vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo) bool {
			return true
		},
		differs: func(_ vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, dev vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo, _ *vimtypes.VirtualMachineConfigSpec) bool {
			vmxnet3, ok := dev.(*vimtypes.VirtualVmxnet3)
			if !ok {
				return false
			}
			return ptr.Deref(vmxnet3.Uptv2Enabled) != desiredUPTv2Enabled(iface)
		},
		apply: func(_ vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, dev vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			mutable := findOrCreateDeviceEdit(cs, dev)
			v := desiredUPTv2Enabled(iface)
			mutable.(*vimtypes.VirtualVmxnet3).Uptv2Enabled = &v
		},
	},
	{
		fieldName: "interfaces[i].vnumaNodeID",
		minHWVer:  vimtypes.VMX20,
		hotPluggable: func(_ vmopv1.VirtualMachine, _ vmopv1.VirtualMachineNetworkInterfaceSpec, _ vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo) bool {
			return false
		},
		prerequisite: func(vm vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, _ vimtypes.BaseVirtualDevice, ci vimtypes.VirtualMachineConfigInfo) (bool, string) {
			if iface.VNUMANodeID == nil {
				return false, ""
			}
			if !vmopv1util.FirmwareIsEFI(&vm, ci) {
				return true, "EFI firmware required"
			}
			if vmopv1util.GetVNUMANodeCount(&vm) == 0 {
				return true, "vNUMA topology required (spec.cpuAdvanced.topology.vnumaNodeCount must be > 0)"
			}
			return false, ""
		},
		differs: func(_ vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, dev vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo, _ *vimtypes.VirtualMachineConfigSpec) bool {
			desired := desiredVNUMANodeID(iface)
			current := dev.GetVirtualDevice().NumaNode
			if *desired < 0 && (current == nil || *current < 0) {
				return false
			}
			if current == nil {
				return true
			}
			return *current != *desired
		},
		apply: func(_ vmopv1.VirtualMachine, iface vmopv1.VirtualMachineNetworkInterfaceSpec, dev vimtypes.BaseVirtualDevice, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			mutable := findOrCreateDeviceEdit(cs, dev)
			mutable.GetVirtualDevice().NumaNode = desiredVNUMANodeID(iface)
		},
	},
}

// reconcileNICFields evaluates all nicFields entries for a single
// spec interface / hardware device pair and applies changes to cs.
// Returns the names of blocked (prerequisite failure) and blockedPowerOff
// (hot-plug not possible) fields.
func reconcileNICFields(
	vm vmopv1.VirtualMachine,
	iface vmopv1.VirtualMachineNetworkInterfaceSpec,
	dev vimtypes.BaseVirtualDevice,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec,
) (blocked, blockedPowerOff []string) {
	hwVer, _ := vimtypes.ParseHardwareVersion(ci.Version)
	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn

	for _, f := range nicFields {
		// Skip early if current state already matches desired.
		if !f.differs(vm, iface, dev, ci, cs) {
			continue
		}

		// Hardware version gate.
		if f.minHWVer > 0 && hwVer < f.minHWVer {
			blocked = append(blocked,
				fmt.Sprintf("%s (requires hwVer >= %d)", f.fieldName, f.minHWVer))
			continue
		}

		// Runtime prerequisite check (EFI, vNUMA topology, etc.).
		if f.prerequisite != nil {
			if prereqBlocked, reason := f.prerequisite(vm, iface, dev, ci); prereqBlocked {
				blocked = append(blocked, fmt.Sprintf("%s (%s)", f.fieldName, reason))
				continue
			}
		}

		if poweredOn && !f.hotPluggable(vm, iface, dev, ci) {
			blockedPowerOff = append(blockedPowerOff, f.fieldName)
		} else {
			f.apply(vm, iface, dev, ci, cs)
		}
	}
	return blocked, blockedPowerOff
}

// desiredUPTv2Enabled returns the desired UPTv2 enablement: nil spec → false.
func desiredUPTv2Enabled(iface vmopv1.VirtualMachineNetworkInterfaceSpec) bool {
	if iface.VMXNet3 == nil || iface.VMXNet3.UPTv2Enabled == nil {
		return false
	}
	return *iface.VMXNet3.UPTv2Enabled
}

// desiredVNUMANodeID maps the spec pointer to the vSphere wire value.
//
// VirtualDevice.NumaNode wire semantics (*int32):
//
//	nil      → field absent from wire → vSphere status-quo (no change)
//	ptr(-1)  → clears affinity        → ConfigInfo omits field → Go reads nil
//	ptr(0)   → assigns NUMA node 0
//	ptr(N>0) → assigns NUMA node N
//
// To clear an existing NUMA assignment send ptr(-1). VNUMANodeID == nil means
// no assignment desired, so we send ptr(-1) to clear. Values >= 0 are sent
// as-is.
func desiredVNUMANodeID(iface vmopv1.VirtualMachineNetworkInterfaceSpec) *int32 {
	if iface.VNUMANodeID == nil {
		return ptr.To(int32(-1))
	}
	return iface.VNUMANodeID
}

// findOrCreateDeviceEdit finds the existing Edit DeviceChange entry in cs for
// the device matching dev's key. If none is found, dev is appended directly as
// a new Edit entry. Returns the mutable device in the (found or new) entry,
// ensuring at most one Edit entry per device key regardless of how many fields
// need updating or whether another reconciler already added an entry.
func findOrCreateDeviceEdit(cs *vimtypes.VirtualMachineConfigSpec, dev vimtypes.BaseVirtualDevice) vimtypes.BaseVirtualDevice {
	key := dev.GetVirtualDevice().Key
	for _, dc := range cs.DeviceChange {
		if spec, ok := dc.(*vimtypes.VirtualDeviceConfigSpec); ok {
			if spec.Operation == vimtypes.VirtualDeviceConfigSpecOperationEdit &&
				spec.Device != nil && spec.Device.GetVirtualDevice().Key == key {
				return spec.Device
			}
		}
	}
	cs.DeviceChange = append(cs.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
		Device:    dev,
		Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
	})
	return dev
}
