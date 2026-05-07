// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func backfillExtraConfigTests() {

	// moVMWithExtraConfig builds a moVM with only ExtraConfig — suitable for
	// VM-level (spec.advanced.*) backfill tests that need no NIC hardware.
	moVMWithExtraConfig := func(
		kvs ...*vimtypes.OptionValue) mo.VirtualMachine {

		ec := make([]vimtypes.BaseOptionValue, len(kvs))
		for i, kv := range kvs {
			ec[i] = kv
		}
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				ExtraConfig: ec,
			},
		}
	}

	// moVMWithNICsAndExtraConfig builds a moVM with both hardware ethernet
	// devices and ExtraConfig — required for NIC-level backfill tests so the
	// ethernetX index can be derived from the device key.
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

	ov := func(k, v string) *vimtypes.OptionValue {
		return &vimtypes.OptionValue{Key: k, Value: v}
	}

	Describe("BackfillExtraConfigFromMoVM", func() {
		var (
			vm   *vmopv1.VirtualMachine
			moVM mo.VirtualMachine
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
			moVM = moVMWithExtraConfig()
		})

		// ------------------------------------------------------------------ //
		// VM-level (spec.advanced.*) backfill
		// ------------------------------------------------------------------ //

		Context("VM-level backfill", func() {

			DescribeTable("each vmx-tagged field round-trips",
				func(key, raw string, check func(*vmopv1.VirtualMachineAdvancedSpec)) {
					moVM = moVMWithExtraConfig(ov(key, raw))

					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue(), "expected mutation for key %q", key)
					Expect(vm.Spec.Advanced).ToNot(BeNil())
					check(vm.Spec.Advanced)
				},
				Entry("PreferHTEnabled TRUE",
					"numa.vcpu.preferHT", "TRUE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.PreferHTEnabled).ToNot(BeNil())
						Expect(*a.PreferHTEnabled).To(BeTrue())
					}),
				Entry("PreferHTEnabled FALSE",
					"numa.vcpu.preferHT", "FALSE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.PreferHTEnabled).ToNot(BeNil())
						Expect(*a.PreferHTEnabled).To(BeFalse())
					}),
				Entry("HugePages1GEnabled TRUE",
					"sched.mem.lpage.enable1GPage", "TRUE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.HugePages1GEnabled).ToNot(BeNil())
						Expect(*a.HugePages1GEnabled).To(BeTrue())
					}),
				Entry("TimeTrackerLowLatencyEnabled FALSE",
					"timeTracker.lowLatency", "FALSE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.TimeTrackerLowLatencyEnabled).ToNot(BeNil())
						Expect(*a.TimeTrackerLowLatencyEnabled).To(BeFalse())
					}),
				Entry("CPUAffinityExclusiveNoStatsEnabled TRUE",
					"sched.cpu.affinity.exclusiveNoStats", "TRUE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.CPUAffinityExclusiveNoStatsEnabled).ToNot(BeNil())
						Expect(*a.CPUAffinityExclusiveNoStatsEnabled).To(BeTrue())
					}),
				Entry("VMXSwapEnabled FALSE",
					"sched.swap.vmxSwapEnabled", "FALSE",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.VMXSwapEnabled).ToNot(BeNil())
						Expect(*a.VMXSwapEnabled).To(BeFalse())
					}),
				Entry("PNUMANodeAffinity single node",
					"numa.nodeAffinity", "0",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.PNUMANodeAffinity).To(Equal([]int32{0}))
					}),
				Entry("PNUMANodeAffinity multiple nodes",
					"numa.nodeAffinity", "0,1,2",
					func(a *vmopv1.VirtualMachineAdvancedSpec) {
						Expect(a.PNUMANodeAffinity).To(Equal([]int32{0, 1, 2}))
					}),
			)

			When("moVM.Config is nil", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{Config: nil}
				})

				It("returns no mutation", func() {
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Advanced).To(BeNil())
				})
			})

			When("moVM ExtraConfig is empty", func() {
				It("returns no mutation", func() {
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Advanced).To(BeNil())
				})
			})

			When("spec field is already set (idempotency)", func() {
				BeforeEach(func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						PreferHTEnabled: ptr.To(true),
					}
					moVM = moVMWithExtraConfig(ov("numa.vcpu.preferHT", "FALSE"))
				})

				It("does not overwrite the existing value", func() {
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(*vm.Spec.Advanced.PreferHTEnabled).To(BeTrue())
				})
			})

			When("spec field differs from moVM (drift — spec wins)", func() {
				BeforeEach(func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						VMXSwapEnabled: ptr.To(false),
					}
					moVM = moVMWithExtraConfig(ov("sched.swap.vmxSwapEnabled", "TRUE"))
				})

				It("leaves the spec value unchanged", func() {
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(*vm.Spec.Advanced.VMXSwapEnabled).To(BeFalse())
				})
			})

			DescribeTable("unknown / bookkeeping keys are silently dropped",
				func(key string) {
					moVM = moVMWithExtraConfig(ov(key, "value"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Advanced).To(BeNil())
				},
				Entry("unknown user key", "customer.knob"),
				Entry("vmservice reserved", "vmservice.foo"),
				Entry("guestinfo reserved", "guestinfo.bar"),
				Entry("tools bookkeeping", "tools.guestlib.enableHostInfo"),
				Entry("migrate bookkeeping", "migrate.encryptionMode"),
				Entry("sched.swap derived", "sched.swap.derivedName"),
				Entry("scsi pciSlotNumber", "scsi0.pciSlotNumber"),
			)
		})

		// ------------------------------------------------------------------ //
		// NIC-level (spec.network.interfaces[i].vmxnet3.*) backfill
		// ------------------------------------------------------------------ //

		Context("NIC-level backfill", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
							Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
						},
					},
				}
			})

			DescribeTable("each vmxnet3 vmx-tagged field round-trips",
				func(key, raw string, check func(*vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec)) {
					// Device key 4000 → ethernetIdx 0 → prefix "ethernet0."
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov(key, raw))

					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue(), "expected mutation for key %q", key)
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
					check(vm.Spec.Network.Interfaces[0].VMXNet3)
				},
				Entry("CtxPerDev PerDevice",
					"ethernet0.ctxPerDev", "PerDevice",
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
				Entry("UDPRSSEnabled FALSE",
					"ethernet0.udpRSS", "FALSE",
					func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
						Expect(s.UDPRSSEnabled).ToNot(BeNil())
						Expect(*s.UDPRSSEnabled).To(BeFalse())
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
				Entry("PNICFeatures raw value stored",
					"ethernet0.pnicfeatures", "3",
					func(s *vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec) {
						Expect(s.PNICFeatures).To(Equal(
							[]vmopv1.PNICQueueFeature{"3"}))
					}),
			)

			DescribeTable("vSphere-managed keys are silently dropped",
				func(key string) {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov(key, "value"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
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

			It("unknown NIC key is silently dropped", func() {
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.foo", "value"))
				mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})

			When("NIC type is SRIOV", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type =
						vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
				})

				It("skips all ethernet0.* keys", func() {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov("ethernet0.ctxPerDev", "PerDevice"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
				})
			})

			When("NIC type is E1000", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type =
						vmopv1.VirtualMachineNetworkInterfaceTypeE1000
				})

				It("skips all ethernet0.* keys", func() {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov("ethernet0.ctxPerDev", "PerDevice"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
				})
			})

			When("NIC type is unset (defaults to VMXNet3 treatment)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type = ""
				})

				It("backfills vmxnet3 fields", func() {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov("ethernet0.ctxPerDev", "PerVM"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerVM))
				})
			})

			When("NIC vmxnet3 field already set (spec wins)", func() {
				BeforeEach(func() {
					mode := vmopv1.TxContextThreadingModePerQueue
					vm.Spec.Network.Interfaces[0].VMXNet3 =
						&vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
							CtxPerDev: &mode,
						}
				})

				It("does not overwrite existing value", func() {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov("ethernet0.ctxPerDev", "PerDevice"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerQueue))
				})
			})

			When("multiple NICs with mixed extraConfig keys", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
						{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
						{Name: "eth2", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					}
				})

				It("lands keys on the correct interfaces; orphan ethernet5 is dropped", func() {
					// Devices with sequential keys 4000-4002 → ethernetIdx 0-2.
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{
							vmxnet3Dev(4000),
							vmxnet3Dev(4001),
							vmxnet3Dev(4002),
						},
						ov("ethernet0.ctxPerDev", "PerDevice"),
						ov("ethernet2.ctxPerDev", "PerVM"),
						ov("ethernet5.ctxPerDev", "PerQueue"), // no matching device → drop
					)
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())

					ifaces := vm.Spec.Network.Interfaces
					Expect(ifaces[0].VMXNet3).ToNot(BeNil())
					Expect(*ifaces[0].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerDevice))

					Expect(ifaces[1].VMXNet3).To(BeNil())

					Expect(ifaces[2].VMXNet3).ToNot(BeNil())
					Expect(*ifaces[2].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerVM))
				})
			})

			When("NIC device keys are non-zero-based (deviceKey=4001 → ethernetIdx=1)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
						{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					}
				})

				It("uses device-key-derived ethernetX index, not spec array position", func() {
					// Keys 4001, 4002 → ethernetIdx 1, 2.
					// spec[0] ↔ device(4001) → look up "ethernet1.*"
					// spec[1] ↔ device(4002) → look up "ethernet2.*"
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{
							vmxnet3Dev(4001),
							vmxnet3Dev(4002),
						},
						ov("ethernet1.ctxPerDev", "PerDevice"),
						ov("ethernet2.ctxPerDev", "PerVM"),
					)
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())

					ifaces := vm.Spec.Network.Interfaces
					Expect(ifaces[0].VMXNet3).ToNot(BeNil())
					Expect(*ifaces[0].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerDevice))

					Expect(ifaces[1].VMXNet3).ToNot(BeNil())
					Expect(*ifaces[1].VMXNet3.CtxPerDev).To(
						Equal(vmopv1.TxContextThreadingModePerVM))
				})
			})

			When("no network interfaces defined", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces = nil
				})

				It("returns no mutation", func() {
					moVM = moVMWithNICsAndExtraConfig(
						[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
						ov("ethernet0.ctxPerDev", "PerDevice"))
					mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
				})
			})
		})

		// ------------------------------------------------------------------ //
		// Schema-upgrade ordering: FillEmptyNetworkInterfaceTypesFromMoVM must
		// run before BackfillExtraConfigFromMoVM so SRIOV interfaces are known.
		// ------------------------------------------------------------------ //

		Context("schema-upgrade ordering guard", func() {
			It("SRIOV type (set by earlier fill step) blocks vmxnet3 backfill", func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
							// Type already set by FillEmptyNetworkInterfaceTypesFromMoVM
							Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV,
						},
					},
				}
				moVM = moVMWithNICsAndExtraConfig(
					[]vimtypes.BaseVirtualDevice{vmxnet3Dev(4000)},
					ov("ethernet0.ctxPerDev", "PerDevice"))

				mutated, err := virtualmachine.BackfillExtraConfigFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
				Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
			})
		})
	})
}
