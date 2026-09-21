// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package intent

import (
	"fmt"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
)

// reportUnsupported records every portable field this provider does not act
// on, so nothing a user set is dropped in silence.
//
// The list is long because the portable API is larger than this first feature.
// That is the point: a field this provider ignores is not a field the user
// stopped caring about, and the difference between "not implemented" and
// "quietly discarded" is the difference between a user who can plan and a user
// who is surprised.
//
// Each entry is phrased so a reader can tell WHAT was dropped and WHY, because
// "unsupported" on its own tells them nothing they can act on.
func (i *Intent) reportUnsupported(vm *kubevmv1a1.VirtualMachine) {
	i.reportSizing(vm)
	i.reportDisks(vm)
	i.reportNetworking(vm)
	i.reportGuest(vm)
	i.reportPlacement(vm)
	i.reportPowerOffMode(vm)
	i.reportSuspended(vm)
}

// reportSuspended notes a request for Suspended, which EC2 cannot satisfy.
func (i *Intent) reportSuspended(vm *kubevmv1a1.VirtualMachine) {
	// Reported rather than approximated: stopping the machine and calling it
	// suspended would be a lie that looks like success.
	if vm.Spec.PowerState == kubevmv1a1.PowerStateSuspended {
		// No pointer into this repository's finding register: whoever
		// reads this is operating a machine, not reviewing the provider.
		i.note("spec.powerState Suspended is not available: EC2 has no " +
			"suspended state. Hibernation lands in stopped and must be " +
			"enabled when the instance is launched")
	}
}

// note appends one unsupported-field report.
func (i *Intent) note(format string, args ...any) {
	i.Unsupported = append(i.Unsupported, fmt.Sprintf(format, args...))
}

// reportSizing covers inline CPU and memory.
func (i *Intent) reportSizing(vm *kubevmv1a1.VirtualMachine) {
	it := vm.Spec.InstanceType
	if it == nil || it.Resources == nil {
		return
	}
	i.note("spec.instanceType.resources is not applied: EC2 has no custom " +
		"sizing, only a fixed catalogue of instance types. Name one through " +
		"spec.instanceType.name instead — rounding a request up to the " +
		"nearest type is a decision this provider will not make for you")
}

// reportDisks covers the boot disk's unconsumed fields and data disks.
func (i *Intent) reportDisks(vm *kubevmv1a1.VirtualMachine) {
	if bd := vm.Spec.BootDisk; bd != nil {
		if bd.Source.Snapshot != nil {
			i.note("spec.bootDisk.source.snapshot is not applied: this " +
				"provider boots from an image only")
		}
		if bd.Source.Blank {
			i.note("spec.bootDisk.source.blank is not applied: a machine " +
				"needs something to boot")
		}
		if bd.SizeGiB != nil {
			i.note("spec.bootDisk.sizeGiB is not applied: the boot volume "+
				"takes the image's own size (%d GiB was requested)",
				*bd.SizeGiB)
		}
		if bd.StorageClassName != "" {
			i.note("spec.bootDisk.storageClassName %q is not applied: the "+
				"boot volume takes the account's default EBS type",
				bd.StorageClassName)
		}
	}
	if n := len(vm.Spec.Disks); n > 0 {
		i.note("spec.disks is not applied: %d data disk(s) were requested "+
			"and this provider attaches none", n)
	}
}

// reportNetworking covers everything on the network spec beyond the first
// interface's network reference and public-address preference.
func (i *Intent) reportNetworking(vm *kubevmv1a1.VirtualMachine) {
	n := vm.Spec.Network
	if n == nil {
		return
	}

	if n.HostName != nil {
		i.note("spec.network.hostName %q is not applied: the guest takes "+
			"EC2's own private DNS name", *n.HostName)
	}
	if len(n.Nameservers) > 0 {
		i.note("spec.network.nameservers is not applied: resolvers come " +
			"from the VPC over DHCP")
	}
	if len(n.SearchDomains) > 0 {
		i.note("spec.network.searchDomains is not applied: search domains " +
			"come from the VPC over DHCP")
	}

	for idx, iface := range n.Interfaces {
		if idx > 0 {
			i.note("spec.network.interfaces[%d] (%q) is not applied: this "+
				"provider attaches one interface", idx, iface.Name)
			continue
		}
		if len(iface.Addresses) > 0 {
			i.note("spec.network.interfaces[%q].addresses is not applied: "+
				"the address is assigned from the subnet", iface.Name)
		}
		if iface.DHCP4 != nil && !*iface.DHCP4 {
			i.note("spec.network.interfaces[%q].dhcp4 false is not "+
				"available: addresses in a VPC are always assigned by DHCP",
				iface.Name)
		}
		if iface.DHCP6 != nil && !*iface.DHCP6 {
			i.note("spec.network.interfaces[%q].dhcp6 false is not "+
				"available in a dual-stack subnet", iface.Name)
		}
	}
}

// reportGuest covers bootstrap data and SSH keys.
func (i *Intent) reportGuest(vm *kubevmv1a1.VirtualMachine) {
	if vm.Spec.Bootstrap != nil {
		i.note("spec.bootstrap is not applied: this provider sends no " +
			"user data, so the guest boots unconfigured")
	}
	if n := len(vm.Spec.SSHPublicKeys); n > 0 {
		i.note("spec.sshPublicKeys is not applied: %d key(s) were given "+
			"and none is injected, so the machine may be unreachable", n)
	}
}

// reportPlacement covers zone pinning, spot and tags.
func (i *Intent) reportPlacement(vm *kubevmv1a1.VirtualMachine) {
	if vm.Spec.FailureDomain != nil {
		i.note("spec.failureDomain %q is not applied: placement follows "+
			"the subnet, which pins the availability zone on its own",
			*vm.Spec.FailureDomain)
	}
	if s := vm.Spec.Scheduling; s != nil && s.Spot != nil && *s.Spot {
		i.note("spec.scheduling.spot is not applied: this provider " +
			"launches on-demand capacity only")
	}
	if n := len(vm.Spec.Tags); n > 0 {
		i.note("spec.tags is not applied: %d tag(s) were given and this "+
			"provider sets only its own ownership tags", n)
	}
}

// reportPowerOffMode covers the guest-involvement preference.
func (i *Intent) reportPowerOffMode(vm *kubevmv1a1.VirtualMachine) {
	// Suspended is reported separately, by reportSuspended.
	if m := vm.Spec.PowerOffMode; m != "" &&
		m != kubevmv1a1.PowerOpModeTrySoft {
		i.note("spec.powerOffMode %q is not applied: StopInstances asks "+
			"the guest to shut down and falls back on its own, which is "+
			"TrySoft behaviour and not configurable", m)
	}
}
