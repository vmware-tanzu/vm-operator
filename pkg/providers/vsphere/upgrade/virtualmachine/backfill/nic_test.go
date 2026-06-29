// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine/backfill"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("BackfillNICConfigFromMoVM", func() {

	ctx := context.Background()

	// ------------------------------------------------------------------ //
	// Helpers
	// ------------------------------------------------------------------ //

	// moVMWithEthernet builds a moVM with only hardware devices (no ExtraConfig).
	moVMWithEthernet := func(devs ...vimtypes.BaseVirtualDevice) mo.VirtualMachine {
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{Device: devs},
			},
		}
	}

	// moVMWithNICsAndExtraConfig builds a moVM with hardware ethernet devices
	// and ExtraConfig — required for ExtraConfig NIC tests so the ethernetX
	// index can be derived from the device key.
	moVMWithNICsAndExtraConfig := func(
		devs []vimtypes.BaseVirtualDevice,
		kvs ...*vimtypes.OptionValue) mo.VirtualMachine {

		ec := make([]vimtypes.BaseOptionValue, len(kvs))
		for i, kv := range kvs {
			ec[i] = kv
		}
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware:    vimtypes.VirtualHardware{Device: devs},
				ExtraConfig: ec,
			},
		}
	}

	// vmxnet3Dev creates a VirtualVmxnet3 device with the given key.
	// ethernetX index = key - 4000 (vSphere convention).
	vmxnet3Dev := func(key int32) vimtypes.BaseVirtualDevice {
		return &vimtypes.VirtualVmxnet3{
			VirtualVmxnet: vimtypes.VirtualVmxnet{
				VirtualEthernetCard: vimtypes.VirtualEthernetCard{
					VirtualDevice: vimtypes.VirtualDevice{Key: key},
				},
			},
		}
	}

	// vmxnet3WithProps creates a VirtualVmxnet3 with NumaNode and Uptv2Enabled set.
	vmxnet3WithProps := func(numaNode *int32, uptv2Enabled *bool) *vimtypes.VirtualVmxnet3 {
		dev := &vimtypes.VirtualVmxnet3{Uptv2Enabled: uptv2Enabled}
		dev.NumaNode = numaNode
		return dev
	}

	sriovDev := func(numaNode *int32) *vimtypes.VirtualSriovEthernetCard {
		dev := &vimtypes.VirtualSriovEthernetCard{}
		dev.NumaNode = numaNode
		return dev
	}

	ov := func(k, v string) *vimtypes.OptionValue {
		return &vimtypes.OptionValue{Key: k, Value: v}
	}

	var (
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
							Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
						},
					},
				},
			},
		}
	})

	// ------------------------------------------------------------------ //
	// Guard conditions
	// ------------------------------------------------------------------ //

	When("moVM.Config is nil", func() {
		BeforeEach(func() {
			moVM = mo.VirtualMachine{Config: nil}
		})

		It("returns no mutation", func() {
			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeFalse())
		})
	})

	When("VM has no network spec", func() {
		BeforeEach(func() {
			vm.Spec.Network = nil
			moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{})
		})

		It("returns no mutation", func() {
			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeFalse())
		})
	})

	When("VM has no interfaces", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces = nil
			moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{})
		})

		It("returns no mutation", func() {
			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeFalse())
		})
	})

	When("moVM has no ethernet devices", func() {
		BeforeEach(func() {
			moVM = moVMWithEthernet(&vimtypes.VirtualPCIPassthrough{})
		})

		It("returns no mutation", func() {
			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeFalse())
		})
	})

	// ------------------------------------------------------------------ //
	// Type backfill
	// ------------------------------------------------------------------ //

	Context("NIC Type from device", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces[0].Type = ""
		})

		When("moVM has VMXNet3 device", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{})
			})

			It("sets type to VMXNet3", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		When("moVM has SRIOV device", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(&vimtypes.VirtualSriovEthernetCard{})
			})

			It("sets type to SRIOV", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})

		DescribeTable("legacy NIC types",
			func(dev vimtypes.BaseVirtualDevice, expectedType vmopv1.VirtualMachineNetworkInterfaceType) {
				moVM = moVMWithEthernet(dev)
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(expectedType))
			},
			Entry("E1000", &vimtypes.VirtualE1000{}, vmopv1.VirtualMachineNetworkInterfaceTypeE1000),
			Entry("E1000e", &vimtypes.VirtualE1000e{}, vmopv1.VirtualMachineNetworkInterfaceTypeE1000e),
			Entry("VMXNet2", &vimtypes.VirtualVmxnet2{}, vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet2),
			Entry("PCNet32", &vimtypes.VirtualPCNet32{}, vmopv1.VirtualMachineNetworkInterfaceTypePCNet32),
		)

		When("moVM has mixed NIC types", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0"},
					{Name: "eth1"},
				}
				moVM = moVMWithEthernet(&vimtypes.VirtualE1000{}, &vimtypes.VirtualSriovEthernetCard{})
			})

			It("sets correct type for each interface", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeE1000))
				Expect(vm.Spec.Network.Interfaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})

		When("type already set — spec wins", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeE1000
				moVM = moVMWithEthernet(&vimtypes.VirtualSriovEthernetCard{})
			})

			It("does not overwrite existing type", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeE1000))
			})
		})

		When("VM has more interfaces than moVM NICs", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0"},
					{Name: "eth1"},
				}
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{})
			})

			It("sets type from device for matched interface; defaults extra to VMXNet3", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
				Expect(vm.Spec.Network.Interfaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})
	})

	// ------------------------------------------------------------------ //
	// vmxnet3.* ExtraConfig backfill
	// ------------------------------------------------------------------ //

	Context("vmxnet3.* from ExtraConfig", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
		})

		DescribeTable("each vmxnet3 vmx-tagged field round-trips",
			func(key, raw string, check func(*vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec)) {
				// Device key 4000 → ethernetIdx 0 → prefix "ethernet0."
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov(key, raw))

				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue(), "expected mutation for key %q", key)
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
				check(vm.Spec.Network.Interfaces[0].VMXNet3)
			},
			Entry("CtxPerDev PerDevice",
				"ethernet0.ctxPerDev", "1",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.CtxPerDev).ToNot(BeNil())
					Expect(*s.CtxPerDev).To(Equal(vmopv1.TxContextThreadingModePerDevice))
				}),
			Entry("RSSOffloadEnabled TRUE",
				"ethernet0.rssoffload", "TRUE",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.RSSOffloadEnabled).ToNot(BeNil())
					Expect(*s.RSSOffloadEnabled).To(BeTrue())
				}),
			Entry("UDPRSSEnabled 2 (disabled)",
				"ethernet0.udpRSS", "2",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.UDPRSSEnabled).ToNot(BeNil())
					Expect(*s.UDPRSSEnabled).To(Equal(vmopv1.UDPRSSModeDisabled))
				}),
			Entry("UDPRSSEnabled 1 (enabled)",
				"ethernet0.udpRSS", "1",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.UDPRSSEnabled).ToNot(BeNil())
					Expect(*s.UDPRSSEnabled).To(Equal(vmopv1.UDPRSSModeEnabled))
				}),
			Entry("CoalescingScheme Disabled",
				"ethernet0.coalescingScheme", "Disabled",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.CoalescingScheme).ToNot(BeNil())
					Expect(*s.CoalescingScheme).To(Equal(vmopv1.CoalescingSchemeDisabled))
				}),
			Entry("CoalescingParams 4000",
				"ethernet0.coalescingParams", "4000",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.CoalescingParams).ToNot(BeNil())
					Expect(*s.CoalescingParams).To(Equal("4000"))
				}),
			Entry("PNICFeatures bitmask 3 → LRO+bit1",
				"ethernet0.pnicFeatures", "3",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.PNICFeatures).To(ConsistOf(
						vmopv1.PNICQueueFeatureLargeReceiveOffload,
						vmopv1.PNICQueueFeature("2"),
					))
				}),
		)

		DescribeTable("auto/default/dontcare sentinels leave spec field nil and report no mutation",
			func(key, raw string) {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov(key, raw))

				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse(),
					"auto sentinel %q for key %q should not mutate spec", raw, key)
				// VMXNet3 must stay nil — no empty struct created.
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			},
			Entry("RSSOffloadEnabled auto", "ethernet0.rssoffload", "auto"),
			Entry("RSSOffloadEnabled AUTO", "ethernet0.rssoffload", "AUTO"),
			Entry("RSSOffloadEnabled DEFAULT", "ethernet0.rssoffload", "DEFAULT"),
			Entry("RSSOffloadEnabled dontcare", "ethernet0.rssoffload", "dontcare"),
			Entry("UDPRSSEnabled auto", "ethernet0.udpRSS", "auto"),
			Entry("UDPRSSEnabled DEFAULT", "ethernet0.udpRSS", "DEFAULT"),
		)

		When("all vmx-tagged NIC keys carry auto sentinels", func() {
			BeforeEach(func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.rssoffload", "auto"),
					ov("ethernet0.udpRSS", "DEFAULT"),
				)
			})

			It("leaves iface.VMXNet3 nil — no empty struct created", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})

		DescribeTable("extended bool forms are accepted for *bool NIC fields",
			func(key, raw string, wantTrue bool) {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov(key, raw))

				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue(), "key %q raw %q should mutate", key, raw)
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3.RSSOffloadEnabled).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.RSSOffloadEnabled).To(Equal(wantTrue))
			},
			Entry("yes → true", "ethernet0.rssoffload", "yes", true),
			Entry("no → false", "ethernet0.rssoffload", "no", false),
			Entry("on → true", "ethernet0.rssoffload", "on", true),
			Entry("off → false", "ethernet0.rssoffload", "off", false),
		)

		DescribeTable("vSphere-managed keys are silently dropped",
			func(key string) {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov(key, "value"))
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			},
			Entry("pciSlotNumber", "ethernet0.pciSlotNumber"),
			Entry("present", "ethernet0.present"),
			Entry("virtualDev", "ethernet0.virtualDev"),
			Entry("generatedAddress", "ethernet0.generatedAddress"),
			Entry("uptCompatibility", "ethernet0.uptCompatibility"),
			Entry("dvs.switchId", "ethernet0.dvs.switchId"),
			Entry("opaqueNetwork.id", "ethernet0.opaqueNetwork.id"),
		)

		It("unknown NIC ExtraConfig key is silently dropped", func() {
			moVM = moVMWithNICsAndExtraConfig(
				[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
				ov("ethernet0.foo", "value"))
			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
		})

		When("NIC type is SRIOV — vmxnet3 ExtraConfig skipped", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
			})

			It("skips all ethernet0.* keys", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.ctxPerDev", "1"))
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})

		When("NIC type is E1000 — vmxnet3 ExtraConfig skipped", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeE1000
			})

			It("skips all ethernet0.* keys", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.ctxPerDev", "1"))
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})

		When("NIC type is unset — type derived from device, then vmxnet3 keys applied", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = ""
			})

			It("sets type from device and backfills vmxnet3 fields", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.ctxPerDev", "2"))
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev).To(
					Equal(vmopv1.TxContextThreadingModePerVM))
			})
		})

		When("vmxnet3 field already set — spec wins", func() {
			BeforeEach(func() {
				mode := vmopv1.TxContextThreadingModePerQueue
				vm.Spec.Network.Interfaces[0].VMXNet3 =
					&vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{CtxPerDev: &mode}
			})

			It("does not overwrite existing value", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.ctxPerDev", "1"))
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev).To(
					Equal(vmopv1.TxContextThreadingModePerQueue))
			})
		})

		When("multiple NICs with mixed ExtraConfig keys", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					{Name: "eth2", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
				}
			})

			It("lands keys on the correct interfaces; orphan ethernet5 is dropped", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000), vmxnet3Dev(4001), vmxnet3Dev(4002)},
					ov("ethernet0.ctxPerDev", "1"),
					ov("ethernet2.ctxPerDev", "2"),
					ov("ethernet5.ctxPerDev", "3"), // no matching device → drop
				)
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())

				ifaces := vm.Spec.Network.Interfaces
				Expect(ifaces[0].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[0].VMXNet3.CtxPerDev).To(Equal(vmopv1.TxContextThreadingModePerDevice))
				Expect(ifaces[1].VMXNet3).To(BeNil())
				Expect(ifaces[2].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[2].VMXNet3.CtxPerDev).To(Equal(vmopv1.TxContextThreadingModePerVM))
			})
		})

		When("device keys are non-zero-based (deviceKey=4001 → ethernetIdx=1)", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
				}
			})

			It("uses device-key-derived ethernetX index, not spec array position", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4001), vmxnet3Dev(4002)},
					ov("ethernet1.ctxPerDev", "1"),
					ov("ethernet2.ctxPerDev", "2"),
				)
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())

				ifaces := vm.Spec.Network.Interfaces
				Expect(ifaces[0].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[0].VMXNet3.CtxPerDev).To(Equal(vmopv1.TxContextThreadingModePerDevice))
				Expect(ifaces[1].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[1].VMXNet3.CtxPerDev).To(Equal(vmopv1.TxContextThreadingModePerVM))
			})
		})
	})

	// ------------------------------------------------------------------ //
	// VNUMANodeID backfill
	// ------------------------------------------------------------------ //

	Context("VNUMANodeID from VirtualDevice.NumaNode", func() {
		When("NumaNode is positive", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(ptr.To(int32(2)), nil))
			})

			It("backfills VNUMANodeID", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(2)))
			})
		})

		When("NumaNode is zero (NUMA node 0 is a valid assignment)", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(ptr.To(int32(0)), nil))
			})

			It("backfills VNUMANodeID with node 0", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(0)))
			})
		})

		When("NumaNode is nil (unset or cleared)", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, nil))
			})

			It("does not backfill VNUMANodeID", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).To(BeNil())
			})
		})

		When("NumaNode is negative (explicitly no affinity)", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(ptr.To(int32(-1)), nil))
			})

			It("does not backfill VNUMANodeID", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).To(BeNil())
			})
		})

		When("VNUMANodeID already set — spec wins", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].VNUMANodeID = ptr.To(int32(5))
				moVM = moVMWithEthernet(vmxnet3WithProps(ptr.To(int32(7)), nil))
			})

			It("does not overwrite existing value", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(5)))
			})
		})

		When("NIC is SRIOV — VNUMANodeID still backfilled (on VirtualDevice base)", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
				moVM = moVMWithEthernet(sriovDev(ptr.To(int32(3))))
			})

			It("backfills VNUMANodeID regardless of NIC type", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(3)))
			})
		})
	})

	// ------------------------------------------------------------------ //
	// UPTv2Enabled backfill
	// ------------------------------------------------------------------ //

	Context("UPTv2Enabled from VirtualVmxnet3.Uptv2Enabled", func() {
		When("Uptv2Enabled is true", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, ptr.To(true)))
			})

			It("backfills vmxnet3.UPTv2Enabled = true", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeTrue())
			})
		})

		When("Uptv2Enabled is false", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, ptr.To(false)))
			})

			It("backfills vmxnet3.UPTv2Enabled = false", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).ToNot(BeNil())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeFalse())
			})
		})

		When("Uptv2Enabled is nil on device", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, nil))
			})

			It("does not mutate", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})

		When("UPTv2Enabled already set — spec wins", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].VMXNet3 =
					&vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
						UPTv2Enabled: ptr.To(false),
					}
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, ptr.To(true)))
			})

			It("does not overwrite existing value", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeFalse())
			})
		})

		When("NIC type is SRIOV — UPTv2Enabled skipped", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
				moVM = moVMWithEthernet(vmxnet3WithProps(nil, ptr.To(true)))
			})

			It("does not backfill UPTv2Enabled for SRIOV interface", func() {
				mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})
	})

	// ------------------------------------------------------------------ //
	// Multi-NIC index alignment
	// ------------------------------------------------------------------ //

	Context("multi-NIC index alignment", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
				{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV},
				{Name: "eth2", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
			}
		})

		It("aligns device[0]→interfaces[0], device[1]→interfaces[1], etc.", func() {
			moVM = moVMWithEthernet(
				vmxnet3WithProps(ptr.To(int32(1)), ptr.To(true)),  // interfaces[0]
				sriovDev(ptr.To(int32(2))),                        // interfaces[1] — only VNUMANodeID
				vmxnet3WithProps(ptr.To(int32(3)), ptr.To(false)), // interfaces[2]
				vmxnet3WithProps(ptr.To(int32(4)), ptr.To(true)),  // no spec interface → ignored
			)

			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeTrue())

			ifaces := vm.Spec.Network.Interfaces

			Expect(ifaces[0].VNUMANodeID).ToNot(BeNil())
			Expect(*ifaces[0].VNUMANodeID).To(Equal(int32(1)))
			Expect(ifaces[0].VMXNet3).ToNot(BeNil())
			Expect(*ifaces[0].VMXNet3.UPTv2Enabled).To(BeTrue())

			Expect(ifaces[1].VNUMANodeID).ToNot(BeNil())
			Expect(*ifaces[1].VNUMANodeID).To(Equal(int32(2)))
			Expect(ifaces[1].VMXNet3).To(BeNil())

			Expect(ifaces[2].VNUMANodeID).ToNot(BeNil())
			Expect(*ifaces[2].VNUMANodeID).To(Equal(int32(3)))
			Expect(ifaces[2].VMXNet3).ToNot(BeNil())
			Expect(*ifaces[2].VMXNet3.UPTv2Enabled).To(BeFalse())
		})

		It("extra spec interfaces beyond device count: Type preserved if set, no device properties", func() {
			moVM = moVMWithEthernet(vmxnet3WithProps(ptr.To(int32(1)), ptr.To(true))) // 1 device, 3 spec ifaces

			mutated := backfill.NICConfigFromMoVM(ctx, vm, moVM)
			Expect(mutated).To(BeTrue())

			ifaces := vm.Spec.Network.Interfaces
			// Matched interface gets full backfill.
			Expect(ifaces[0].VNUMANodeID).ToNot(BeNil())
			// Unmatched interfaces: Type already set is preserved; no device properties.
			Expect(ifaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			Expect(ifaces[1].VNUMANodeID).To(BeNil())
			Expect(ifaces[2].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			Expect(ifaces[2].VNUMANodeID).To(BeNil())
		})
	})
})
