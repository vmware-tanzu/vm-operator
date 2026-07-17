// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// NICDeviceMatcher returns the hardware device that corresponds to a spec NIC
// entry. Returns nil when no match exists (device not yet provisioned, or
// excluded type). Implementations may be stateful (e.g. positional cursor
// advancing per call).
type NICDeviceMatcher func(iface vmopv1.VirtualMachineNetworkInterfaceSpec, specIdx int) vimtypes.BaseVirtualDevice

// DefaultNICMatcher returns a NICDeviceMatcher that positionally zips each
// managed ethernet spec interface to the corresponding ethernet device in the
// hardware list. SR-IOV and other non-VMX-namespace adapters are excluded from
// both sides.
//
// The device list is filtered once (O(n) per Reconcile call), not on every match.
//
// TODO: Replace positional zip with identity-based matching once device-order
// semantics are fully understood.
func DefaultNICMatcher(hwDevs []vimtypes.BaseVirtualDevice) NICDeviceMatcher {
	managed := collectManagedEthernetDevices(hwDevs)

	pos := 0
	return func(iface vmopv1.VirtualMachineNetworkInterfaceSpec, _ int) vimtypes.BaseVirtualDevice {
		if !isEthernetInterfaceType(iface.Type) {
			return nil
		}
		if pos >= len(managed) {
			return nil
		}
		dev := managed[pos]
		pos++
		return dev
	}
}

// EthernetDeviceIndex returns the ethernetN namespace index for an ethernet
// device (N = deviceKey - EthernetDeviceKeyBase). Returns (0, false) for
// non-ethernet devices.
func EthernetDeviceIndex(dev vimtypes.BaseVirtualDevice) (int32, bool) {
	if !isEthernetDevice(dev) {
		return 0, false
	}
	return dev.GetVirtualDevice().Key - vmopv1util.EthernetDeviceKeyBase, true
}

// collectManagedEthernetDevices returns all managed ethernet devices from the
// hardware list — those where isEthernetDevice returns true.
func collectManagedEthernetDevices(hwDevs []vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDevice {
	var out []vimtypes.BaseVirtualDevice
	for _, dev := range hwDevs {
		if isEthernetDevice(dev) {
			out = append(out, dev)
		}
	}
	return out
}

// isEthernetDevice reports whether dev is a VirtualEthernetCard that
// participates in the ethernetX VMX namespace. SR-IOV adapters implement
// BaseVirtualEthernetCard but use a separate vSphere mechanism and are excluded.
func isEthernetDevice(dev vimtypes.BaseVirtualDevice) bool {
	if dev == nil {
		return false
	}
	if _, ok := dev.(vimtypes.BaseVirtualEthernetCard); !ok {
		return false
	}
	_, isSRIOV := dev.(*vimtypes.VirtualSriovEthernetCard)
	return !isSRIOV
}

// isEthernetInterfaceType reports whether a spec interface type participates
// in the ethernetX VMX namespace. SR-IOV interfaces use a separate mechanism
// and are excluded.
func isEthernetInterfaceType(t vmopv1.VirtualMachineNetworkInterfaceType) bool {
	switch t {
	case vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV:
		return false
	default:
		return true
	}
}
