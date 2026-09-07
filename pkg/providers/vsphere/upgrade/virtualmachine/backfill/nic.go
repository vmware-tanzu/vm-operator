// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"
	"reflect"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	pkgrecord "github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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

// NICUnitNumberBackfillAmbiguous is the warning-event reason emitted when an
// interface's observed unit number was recorded by positional matching rather
// than a unique match.
const NICUnitNumberBackfillAmbiguous = "NICUnitNumberBackfillAmbiguous"

// NICUnitNumbersFromMoVM records each spec.network.interfaces entry's observed
// PCI unit number from the VM's live vSphere ethernet devices during schema
// upgrade. Once recorded, the unit number is the interface's identifier for
// its hardware, so the value recorded here must be the one vSphere actually
// observed. Spec wins: an interface that already carries a unit number is
// never overwritten, including when the observed device sits at a different
// slot — that disagreement is reported by the steady-state hardware condition,
// not by this one-shot backfill.
//
// Interfaces are matched to devices in two passes:
//
//  1. Hodgepodge matching via the network package's provider-dispatched
//     matcher (network.FindMatchingEthCardForInterfaceSpec), the same
//     MAC / ExternalID / backing criteria the reconcile-time matching uses
//     (FindMatchingEthCard / MapEthernetDevicesToSpecIdx). A returned index
//     is a unique, provider-approved match; the device is claimed even when
//     the interface's spec unit number is already set, so the zip below
//     cannot hand that device to a different interface. When the client is
//     nil, pass 1 is skipped entirely.
//
//  2. A positional zip of the remaining unmatched spec interfaces against
//     the remaining unclaimed devices (with an observed unit number), as a
//     strictly last resort so every interface receives its observed unit
//     number. A single warning event (NICUnitNumberBackfillAmbiguous) names
//     every interface whose value came from the zip: nothing in the
//     resulting spec distinguishes a zipped value from a matched one, yet a
//     wrong zip assignment is one-shot and feeds unit-number-first reconcile
//     matching, status, and boot-order device selection. No other case
//     emits an event here.
//
// Per interface, the write is skipped (the unit number stays nil) when the
// observed value would make the spec inadmissible (G7): already claimed by
// another interface's spec value or by a value recorded earlier in this same
// pass, or — defensively, since ethernet cards are allocated units 7-16 by
// the platform — outside that range. Note what the skip does and does not
// buy: it keeps the spec admissible, but it does not leave the interface
// un-numbered — the mutation webhook runs on this very patch and assigns the
// interface a free slot, exactly as AddControllersForVolumes numbers a volume
// the disk backfill skipped (and that invented slot is acted on under the
// unit-number identity model). This is accepted; see the spec plan's
// "Consistency with the disk placement model".
//
// The backfill is driven by the spec's interface list and runs even when
// spec.network.disabled is true (the devices may still exist in vSphere); a
// VM with no interfaces records nothing.
//
// Returns true if any spec field was mutated.
func NICUnitNumbersFromMoVM(
	ctx context.Context,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) bool {

	if moVM.Config == nil {
		return false
	}
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false
	}

	// The same ethernet device list the provider-dispatched matcher consumes;
	// its indices are the ones claimedDevs tracks. collectEthernetDevicesFromMoVM
	// selects the identical set (IsEthernetCard and SelectByType agree), but a
	// single list guarantees index alignment between pass 1 and the zip.
	ethDevs := object.VirtualDeviceList(moVM.Config.Hardware.Device).
		SelectByType((*vimtypes.VirtualEthernetCard)(nil))

	// G7 admissibility guard: unit numbers already claimed in the spec, plus
	// any recorded by this same pass, are unavailable for recording.
	occupiedUnits := sets.New[int32]()
	for i := range vm.Spec.Network.Interfaces {
		if u := vm.Spec.Network.Interfaces[i].UnitNumber; u != nil {
			occupiedUnits.Insert(*u)
		}
	}

	var (
		claimedDevs   = make([]bool, len(ethDevs))
		hodgepodgeIdx = make([]bool, len(vm.Spec.Network.Interfaces))
		mutated       bool
		zipMatched    []string
	)

	// Pass 1: provider-dispatched hodgepodge matching. A nil client cannot
	// resolve provider CRs; skip the pass rather than panic (the zip below
	// still records observed slots, with its ambiguity event).
	if client != nil {
		vmCtx := pkgctx.NewVirtualMachineContext(ctx, vm)

		// Match against only the devices not yet claimed by an earlier
		// interface, mirroring MapEthernetDevicesToSpecIdx's consumption of
		// each matched card before the next lookup: without this, two
		// interfaces whose criteria both match the same card (e.g. two
		// same-network Named interfaces without MACs) would both match the
		// first device. unclaimedIdx maps positions in unclaimedDevs back to
		// their indices in ethDevs.
		unclaimedIdx := make([]int, 0, len(ethDevs))
		unclaimedDevs := make([]vimtypes.BaseVirtualDevice, 0, len(ethDevs))
		rebuildUnclaimed := func() {
			unclaimedIdx = unclaimedIdx[:0]
			unclaimedDevs = unclaimedDevs[:0]
			for j := range ethDevs {
				if !claimedDevs[j] {
					unclaimedIdx = append(unclaimedIdx, j)
					unclaimedDevs = append(unclaimedDevs, ethDevs[j])
				}
			}
		}
		rebuildUnclaimed()

		for i := range vm.Spec.Network.Interfaces {
			iface := &vm.Spec.Network.Interfaces[i]

			if len(unclaimedDevs) == 0 {
				break
			}

			matchingIdx := network.FindMatchingEthCardForInterfaceSpec(
				vmCtx, client, *iface, unclaimedDevs)
			if matchingIdx < 0 {
				continue
			}

			devIdx := unclaimedIdx[matchingIdx]
			claimedDevs[devIdx] = true
			hodgepodgeIdx[i] = true
			rebuildUnclaimed()

			if iface.UnitNumber != nil {
				continue // spec wins
			}

			if recordEthDeviceUnitNumber(
				ctx, iface, ethDevs[devIdx], occupiedUnits) {
				mutated = true
			}
		}
	} else {
		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"Skipping NIC unit number hodgepodge matching: no client")
	}

	// Reserve every unclaimed device whose observed slot is already declared
	// by a numbered spec interface (or recorded this pass): the zip must only
	// pair leftover interfaces with devices that are not already some other
	// interface's identity, otherwise it G7-skips the write and consumes the
	// pairing, costing a later valid device its record. Note pass-1 claim is
	// still by match, not by unit: a uniquely matched physical NIC must be
	// reserved even when the spec disagrees with it.
	for j := range ethDevs {
		if claimedDevs[j] {
			continue
		}
		if u := ethDevs[j].GetVirtualDevice().UnitNumber; u != nil && occupiedUnits.Has(*u) {
			claimedDevs[j] = true
		}
	}

	// Pass 2: positional zip of the remaining interfaces against the
	// remaining unclaimed devices with an observed unit number. Devices
	// without an observed unit number have nothing to record and are not
	// zip candidates.
	for i := range vm.Spec.Network.Interfaces {
		iface := &vm.Spec.Network.Interfaces[i]
		if hodgepodgeIdx[i] || iface.UnitNumber != nil {
			continue
		}

		for j := range ethDevs {
			dev := ethDevs[j]
			if claimedDevs[j] || dev.GetVirtualDevice().UnitNumber == nil {
				continue
			}

			claimedDevs[j] = true
			if recordEthDeviceUnitNumber(ctx, iface, dev, occupiedUnits) {
				mutated = true
				zipMatched = append(zipMatched, iface.Name)
			}
			break
		}
	}

	if len(zipMatched) > 0 {
		// G8.2: this is the only case a one-shot event is the right mechanism
		// for — nothing in the resulting spec records that the value came from
		// the zip. The other divergence cases (an explicit value disagreeing
		// with the observed slot, a G7-skipped interface) are steady-state
		// observable and belong to the hardware condition, not here.
		pkgrecord.FromContext(ctx).Warnf(
			vm,
			NICUnitNumberBackfillAmbiguous,
			"Observed unit numbers were recorded for the following network interfaces by positional matching rather than a unique match, and may be assigned incorrectly: %s",
			strings.Join(zipMatched, ", "))
	}

	return mutated
}

// recordEthDeviceUnitNumber records the device's observed unit number onto
// iface unless doing so would make the spec inadmissible (G7): the value is
// outside the valid ethernet-card range, or already claimed by another
// interface's spec value or by a value recorded earlier in this same pass
// (occupiedUnits). The device's UnitNumber is already *int32 — copy the
// pointed-to value with a nil guard; never re-take its address, which would
// be a **int32 and would alias a field inside moVM.
func recordEthDeviceUnitNumber(
	ctx context.Context,
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	dev vimtypes.BaseVirtualDevice,
	occupiedUnits sets.Set[int32]) bool {

	observed := dev.GetVirtualDevice().UnitNumber
	if observed == nil {
		// The device reports no unit number: nothing to record.
		return false
	}

	unit := *observed

	if unit < vmopv1util.NICUnitNumberFirst || unit > vmopv1util.NICUnitNumberMax {
		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"Skipping NIC unit number backfill: observed value is out of the valid range",
			"interface", iface.Name,
			"unitNumber", unit)
		return false
	}

	if occupiedUnits.Has(unit) {
		pkglog.FromContextOrDefault(ctx).V(4).Info(
			"Skipping NIC unit number backfill: observed value is already claimed by another interface",
			"interface", iface.Name,
			"unitNumber", unit)
		return false
	}

	occupiedUnits.Insert(unit)
	iface.UnitNumber = ptr.To(unit)
	return true
}
