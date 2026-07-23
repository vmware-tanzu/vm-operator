// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/networkextraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func newVMXNet3Dev(key int32) *vimtypes.VirtualVmxnet3 {
	d := &vimtypes.VirtualVmxnet3{}
	d.Key = key
	return d
}

func getVal(ov pkgutil.OptionValues, key string) (string, bool) {
	return ov.GetString(key)
}

var _ = Describe("DefaultNICMatcher / EthernetDeviceIndex", func() {
	It("positionally zips ethernet spec interfaces to ethernet hardware devices", func() {
		matcher := networkextraconfig.DefaultNICMatcher([]vimtypes.BaseVirtualDevice{
			newVMXNet3Dev(4000),
			newVMXNet3Dev(4001),
		})

		dev0 := matcher(vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth0"}, 0)
		Expect(dev0).ToNot(BeNil())
		idx0, ok := networkextraconfig.EthernetDeviceIndex(dev0)
		Expect(ok).To(BeTrue())
		Expect(idx0).To(Equal(int32(0)))

		dev1 := matcher(vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth1"}, 1)
		Expect(dev1).ToNot(BeNil())
		idx1, ok := networkextraconfig.EthernetDeviceIndex(dev1)
		Expect(ok).To(BeTrue())
		Expect(idx1).To(Equal(int32(1)))
	})

	It("returns nil for SR-IOV interface types", func() {
		matcher := networkextraconfig.DefaultNICMatcher([]vimtypes.BaseVirtualDevice{newVMXNet3Dev(4000)})
		dev := matcher(vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name: "eth0",
			Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV,
		}, 0)
		Expect(dev).To(BeNil())
	})

	It("returns nil when no more hardware devices are available", func() {
		matcher := networkextraconfig.DefaultNICMatcher(nil)
		Expect(matcher(vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth0"}, 0)).To(BeNil())
	})
})

var _ = Describe("DesiredNICExtraConfig", func() {
	ctx := context.Background()

	It("translates first-class VMXNet3 fields with the device prefix", func() {
		iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
			VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{RSSOffloadEnabled: ptr.To(true)},
		}
		got := networkextraconfig.DesiredNICExtraConfig(ctx, iface, 4000, nil)
		v, ok := getVal(got, "ethernet0.rssoffload")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("TRUE"))
	})

	It("writes bag keys with the device prefix", func() {
		iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
			AdvancedProperties: []vmopv1common.KeyValuePair{{Key: "latencySensitivity.level", Value: "high"}},
		}
		got := networkextraconfig.DesiredNICExtraConfig(ctx, iface, 4000, nil)
		v, ok := getVal(got, "ethernet0.latencySensitivity.level")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("high"))
	})

	It("skips a first-class key if present in advancedProperties", func() {
		iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
			AdvancedProperties: []vmopv1common.KeyValuePair{{Key: "ethernet0.rssoffload", Value: "bogus"}},
		}
		got := networkextraconfig.DesiredNICExtraConfig(ctx, iface, 4000, nil)
		Expect(got).To(BeEmpty())
	})

	It("clears a previously-managed bag key no longer requested", func() {
		iface := vmopv1.VirtualMachineNetworkInterfaceSpec{}
		got := networkextraconfig.DesiredNICExtraConfig(ctx, iface, 4000, []string{"foo"})
		v, ok := getVal(got, "ethernet0.foo")
		Expect(ok).To(BeTrue())
		Expect(v).To(BeEmpty())
	})

	It("does not clear a managed key that is still requested", func() {
		iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
			AdvancedProperties: []vmopv1common.KeyValuePair{{Key: "foo", Value: "bar"}},
		}
		got := networkextraconfig.DesiredNICExtraConfig(ctx, iface, 4000, []string{"foo"})
		v, ok := getVal(got, "ethernet0.foo")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("bar"))
	})
})

var _ = Describe("NICExtraConfigDiff", func() {
	ctx := context.Background()

	It("returns nothing when overlay matches observed", func() {
		observed := pkgutil.OptionValues{&vimtypes.OptionValue{Key: "ethernet0.rssoffload", Value: "TRUE"}}
		overlay := pkgutil.OptionValues{&vimtypes.OptionValue{Key: "ethernet0.rssoffload", Value: "TRUE"}}
		applied, deferred, powerCyclePending := networkextraconfig.NICExtraConfigDiff(ctx, observed, overlay, nil, false)
		Expect(applied).To(BeEmpty())
		Expect(deferred).To(BeEmpty())
		Expect(powerCyclePending).To(BeFalse())
	})

	It("scopes the diff to overlay's own keys when existingEC is nil", func() {
		// An unrelated reconciler's pending change (in existingEC, here omitted)
		// must not surface as this NIC's mismatch.
		observed := pkgutil.OptionValues{}
		overlay := pkgutil.OptionValues{&vimtypes.OptionValue{Key: "ethernet0.foo", Value: "bar"}}
		applied, _, _ := networkextraconfig.NICExtraConfigDiff(ctx, observed, overlay, nil, false)
		Expect(applied).To(HaveLen(1))
	})

	It("merges overlay onto existingEC before diffing", func() {
		observed := pkgutil.OptionValues{}
		existingEC := pkgutil.OptionValues{&vimtypes.OptionValue{Key: "ethernet0.other", Value: "baz"}}
		overlay := pkgutil.OptionValues{&vimtypes.OptionValue{Key: "ethernet0.foo", Value: "bar"}}
		applied, _, _ := networkextraconfig.NICExtraConfigDiff(ctx, observed, overlay, existingEC, false)
		Expect(applied).To(HaveLen(2))
	})
})

var _ = Describe("ReconcileNICFields", func() {
	var (
		vm  vmopv1.VirtualMachine
		ci  vimtypes.VirtualMachineConfigInfo
		dev *vimtypes.VirtualVmxnet3
	)

	BeforeEach(func() {
		vm = vmopv1.VirtualMachine{}
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
		dev = newVMXNet3Dev(4000)
		ci = vimtypes.VirtualMachineConfigInfo{Version: "vmx-21"}
	})

	Context("with dryRun=true", func() {
		It("reports the field as needing a change but never mutates dev", func() {
			iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
				VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{UPTv2Enabled: ptr.To(true)},
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{ReservationLockedToMax: ptr.To(true)}

			cs := &vimtypes.VirtualMachineConfigSpec{}
			blocked, blockedPowerOff := networkextraconfig.ReconcileNICFields(vm, iface, dev, ci, cs, true)

			Expect(blocked).To(BeEmpty())
			Expect(blockedPowerOff).To(BeEmpty())
			Expect(cs.DeviceChange).To(BeEmpty(), "dryRun must not add DeviceChange entries")
			Expect(dev.Uptv2Enabled).To(BeNil(), "dryRun must never mutate the source device")
		})

		It("still reports blocked (prerequisite) fields", func() {
			iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
				VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{UPTv2Enabled: ptr.To(true)},
			}
			// No memoryAdvanced set → prerequisite blocked.
			cs := &vimtypes.VirtualMachineConfigSpec{}
			blocked, _ := networkextraconfig.ReconcileNICFields(vm, iface, dev, ci, cs, true)
			Expect(blocked).To(HaveLen(1))
			Expect(dev.Uptv2Enabled).To(BeNil())
		})
	})

	Context("with dryRun=false", func() {
		It("mutates dev and adds a DeviceChange entry", func() {
			iface := vmopv1.VirtualMachineNetworkInterfaceSpec{
				VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{UPTv2Enabled: ptr.To(true)},
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{ReservationLockedToMax: ptr.To(true)}

			cs := &vimtypes.VirtualMachineConfigSpec{}
			blocked, blockedPowerOff := networkextraconfig.ReconcileNICFields(vm, iface, dev, ci, cs, false)

			Expect(blocked).To(BeEmpty())
			Expect(blockedPowerOff).To(BeEmpty())
			Expect(cs.DeviceChange).To(HaveLen(1))
			Expect(dev.Uptv2Enabled).To(Equal(ptr.To(true)))
		})
	})
})
