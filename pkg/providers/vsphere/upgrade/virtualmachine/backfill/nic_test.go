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
	"k8s.io/client-go/tools/events"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine/backfill"
	pkgrecord "github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
					Expect(bool(*s.UDPRSSEnabled)).To(BeFalse())
				}),
			Entry("UDPRSSEnabled 1 (enabled)",
				"ethernet0.udpRSS", "1",
				func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
					Expect(s.UDPRSSEnabled).ToNot(BeNil())
					Expect(bool(*s.UDPRSSEnabled)).To(BeTrue())
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

var _ = Describe("NICUnitNumbersFromMoVM", func() {

	// ------------------------------------------------------------------ //
	// Helpers
	// ------------------------------------------------------------------ //

	// moVMWithEthDevs builds a moVM with only hardware devices (no
	// ExtraConfig). Scoped here because moVMWithEthernet belongs to the
	// BackfillNICConfigFromMoVM Describe's closure above.
	moVMWithEthDevs := func(devs ...vimtypes.BaseVirtualDevice) mo.VirtualMachine {
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{Device: devs},
			},
		}
	}

	// ethCardWithUnit creates a VirtualVmxnet3 with the given key, MAC
	// address, named network backing, and observed unit number. The Named
	// network provider's matcher only considers devices whose backing
	// DeviceName equals the interface's Network.Name, so fixtures that
	// exercise the hodgepodge pass carry that backing; devices without a
	// backing match are reachable only through the positional zip.
	ethCardWithUnit := func(key int32, mac, netName string, unit *int32) vimtypes.BaseVirtualDevice {
		dev := &vimtypes.VirtualVmxnet3{}
		dev.Key = key
		dev.MacAddress = mac
		dev.UnitNumber = unit
		if netName != "" {
			dev.Backing = &vimtypes.VirtualEthernetCardNetworkBackingInfo{
				VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
					DeviceName: netName,
				},
			}
		}
		return dev
	}

	// ifaceWith builds a spec interface with the given name, MAC address,
	// and unit number, and no Network reference: under the Named provider,
	// findMatchingEthCardNamed returns -1 for such interfaces, so they fall
	// through to the positional zip (unless numbered, which the zip skips).
	ifaceWith := func(name, mac string, unit *int32) vmopv1.VirtualMachineNetworkInterfaceSpec {
		return vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name:       name,
			MACAddr:    mac,
			UnitNumber: unit,
		}
	}

	// ifaceWithNet builds a spec interface like ifaceWith but carrying a
	// Network reference matching the fixtures' device backing, so the Named
	// provider's hodgepodge matcher can resolve it (comparing the MAC only
	// when one is specified).
	ifaceWithNet := func(name, mac, netName string, unit *int32) vmopv1.VirtualMachineNetworkInterfaceSpec {
		iface := ifaceWith(name, mac, unit)
		iface.Network = &vmopv1common.PartialObjectRef{Name: netName}
		return iface
	}

	// newRecorderCtx returns a context with a fake recorder injected, plus
	// the fake so tests can assert on emitted events. pkgrecord.FromContext
	// panics when no recorder is present (mirroring the production reconcile
	// context, which always carries one), so every test injects one; the
	// no-event cases assert the fake stayed empty. The context also carries
	// the Named network provider type so the pass-1 matcher dispatches; the
	// default (empty) provider type would match nothing.
	newRecorderCtx := func() (context.Context, *events.FakeRecorder) {
		fakeRecorder := events.NewFakeRecorder(100)
		ctx := pkgrecord.WithContext(
			context.Background(),
			pkgrecord.New(fakeRecorder),
		)
		ctx = pkgcfg.WithContext(
			ctx,
			pkgcfg.Config{
				NetworkProviderType: pkgcfg.NetworkProviderTypeNamed,
			},
		)
		return ctx, fakeRecorder
	}

	// newClient returns a fresh fake controller-runtime client for the pass-1
	// matcher. The Named provider's matcher never reads provider CRs, so the
	// client's contents are irrelevant here; it only must not be nil.
	newClient := func() ctrlclient.Client {
		return builder.NewFakeClient()
	}

	const ambiguousEventMsgPrefix = "Warning NICUnitNumberBackfillAmbiguous Observed unit numbers were recorded for the following network interfaces by positional matching rather than a unique match, and may be assigned incorrectly: "

	const (
		mac1 = "aa:bb:cc:dd:ee:01"
		mac2 = "aa:bb:cc:dd:ee:02"
		mac3 = "aa:bb:cc:dd:ee:03"

		netName  = "net-1"
		bogusNet = "bogus"
	)

	var (
		vm     *vmopv1.VirtualMachine
		moVM   mo.VirtualMachine
		ctx    context.Context
		client ctrlclient.Client
		rec    *events.FakeRecorder
	)

	BeforeEach(func() {
		ctx, rec = newRecorderCtx()
		client = newClient()
	})

	AfterEach(func() {
		vm = nil
		moVM = mo.VirtualMachine{}
		client = nil
	})

	// ------------------------------------------------------------------ //
	// Guard conditions
	// ------------------------------------------------------------------ //

	When("moVM.Config is nil", func() {
		It("returns false", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWith("eth0", "", nil),
						},
					},
				},
			}
			moVM = mo.VirtualMachine{}

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(rec.Events).To(BeEmpty())
		})
	})

	When("the VM has no spec network interfaces", func() {
		It("returns false and records nothing", func() {
			vm = &vmopv1.VirtualMachine{Spec: vmopv1.VirtualMachineSpec{}}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(rec.Events).To(BeEmpty())
		})
	})

	// ------------------------------------------------------------------ //
	// Hodgepodge (provider-dispatched) matching
	// ------------------------------------------------------------------ //

	When("an interface's MAC uniquely matches a device", func() {
		It("records the observed unit number", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("matches the MAC case-insensitively, as findMatchingEthCardNamed does", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", "AA:BB:CC:DD:EE:01", netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("backing match alone suffices when no MAC is specified", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", "", netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("two same-network interfaces without MACs match distinct devices", func() {
			// Pass 1 must consume a matched device before the next interface's
			// lookup (as MapEthernetDevicesToSpecIdx does): with backing-only
			// matching, two same-network Named interfaces would otherwise both
			// match the first device, marking the second interface
			// hodgepodge-matched and G7-skipping its duplicate unit instead of
			// leaving it to zip with the second device.
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", "", netName, nil),
							ifaceWithNet("eth1", "", netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("spec wins when a unit number is already set", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, ptr.To(int32(8))),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("spec wins when the observed slot disagrees; no event (steady-state condition)", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, ptr.To(int32(8))),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(9))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(BeEmpty())
		})

		It("records nothing when the device has no observed unit number", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(ethCardWithUnit(4000, mac1, netName, nil))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).To(BeNil())
			Expect(rec.Events).To(BeEmpty())
		})

		// The provider matcher claims the first device satisfying its
		// criteria; unlike a count-based unique check it does not reject
		// duplicates. This pins that first-match semantics so a future
		// change is deliberate.
		It("a MAC shared by several devices claims the first match", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac1, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})
	})

	// ------------------------------------------------------------------ //
	// G7 admissibility guard
	// ------------------------------------------------------------------ //

	When("the observed value is already claimed by another interface", func() {
		It("the write is skipped and the spec stays admissible; no event", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWith("eth0", "", ptr.To(int32(9))),
							ifaceWithNet("eth1", mac2, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(9))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(9))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).To(BeNil())
			Expect(rec.Events).To(BeEmpty())
		})
	})

	When("the observed value is out of the valid range", func() {
		It("the write is skipped (defensive; real vSphere never reports this); no event", func() {
			// Below the band (3 sits in the unoccupied SCSI HBA band).
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(3))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).To(BeNil())
			Expect(rec.Events).To(BeEmpty())

			// Above the band.
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(17))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).To(BeNil())
			Expect(rec.Events).To(BeEmpty())
		})
	})

	// ------------------------------------------------------------------ //
	// Positional zip
	// ------------------------------------------------------------------ //

	When("interfaces the hodgepodge could not claim are zip-matched", func() {
		It("records the observed unit numbers and emits one NICUnitNumberBackfillAmbiguous event", func() {
			// Both interfaces reference a network no device backs, so pass 1
			// cannot claim anything and the zip carries the whole load.
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", "", bogusNet, nil),
							ifaceWithNet("eth1", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(HaveLen(1))
			Expect(<-rec.Events).
				To(Equal(ambiguousEventMsgPrefix + "eth0, eth1"))
		})

		It("mixed unique+zip: only the zipped interface is named in the event", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
							ifaceWithNet("eth1", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(HaveLen(1))
			Expect(<-rec.Events).
				To(Equal(ambiguousEventMsgPrefix + "eth1"))
		})

		It("a uniquely-matched interface with a spec unit number claims its device, keeping the zip off it", func() {
			// eth0's device is identified by the provider matcher; the zip
			// must not hand it to the unnumbered eth1.
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, ptr.To(int32(7))),
							ifaceWithNet("eth1", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeFalse())
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).To(BeNil())
			Expect(rec.Events).To(BeEmpty())
		})

		It("devices with no observed unit number are not zip candidates", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, nil),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(HaveLen(1))
		})

		It("a device already at a numbered interface's declared slot is reserved from the zip", func() {
			// eth0 declares unit 9 but has no device identified by the
			// hodgepodge; the device actually sitting at slot 9 is not eth0's
			// (nothing matched it). The reservation must keep that device out
			// of the zip, so the leftover interfaces pair against the free
			// slots 7 and 8 and nothing is G7-skipped.
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWith("eth0", "", ptr.To(int32(9))),
							ifaceWithNet("eth1", "", bogusNet, nil),
							ifaceWithNet("eth2", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(9))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(7))),
				ethCardWithUnit(4002, mac3, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(9))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(vm.Spec.Network.Interfaces[2].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(rec.Events).To(HaveLen(1))
			Expect(<-rec.Events).
				To(Equal(ambiguousEventMsgPrefix + "eth1, eth2"))
		})
	})

	// ------------------------------------------------------------------ //
	// Interface/device count mismatch
	// ------------------------------------------------------------------ //

	When("there are more spec interfaces than devices", func() {
		It("records what it can; the remainder stay nil", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
							ifaceWithNet("eth1", "", bogusNet, nil),
							ifaceWithNet("eth2", "", bogusNet, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(vm.Spec.Network.Interfaces[1].UnitNumber).
				To(Equal(ptr.To(int32(8))))
			Expect(vm.Spec.Network.Interfaces[2].UnitNumber).To(BeNil())
			Expect(rec.Events).To(HaveLen(1))
			Expect(<-rec.Events).
				To(Equal(ambiguousEventMsgPrefix + "eth1"))
		})
	})

	When("there are more devices than spec interfaces", func() {
		It("records the matched interface; the extra device is unclaimed", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))),
				ethCardWithUnit(4001, mac2, netName, ptr.To(int32(8))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})
	})

	When("spec.network.disabled is true", func() {
		It("the backfill still runs: it is driven by the spec's interface list", func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Disabled: true,
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							ifaceWithNet("eth0", mac1, netName, nil),
						},
					},
				},
			}
			moVM = moVMWithEthDevs(
				ethCardWithUnit(4000, mac1, netName, ptr.To(int32(7))))

			Expect(backfill.NICUnitNumbersFromMoVM(ctx, client, vm, moVM)).To(BeTrue())
			Expect(vm.Spec.Network.Interfaces[0].UnitNumber).
				To(Equal(ptr.To(int32(7))))
			Expect(rec.Events).To(BeEmpty())
		})
	})
})
