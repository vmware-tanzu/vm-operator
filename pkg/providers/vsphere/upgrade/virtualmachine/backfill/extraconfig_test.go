// Copyright (c) 2026 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine/backfill"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("BackfillExtraConfigFromMoVM", func() {

	moVMWithExtraConfig := func(kvs ...*vimtypes.OptionValue) mo.VirtualMachine {
		ec := make([]vimtypes.BaseOptionValue, len(kvs))
		for i, kv := range kvs {
			ec[i] = kv
		}
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{ExtraConfig: ec},
		}
	}

	ov := func(k, v string) *vimtypes.OptionValue {
		return &vimtypes.OptionValue{Key: k, Value: v}
	}

	var (
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
		moVM = moVMWithExtraConfig()
	})

	DescribeTable("each vmx-tagged field round-trips",
		func(key, raw string, check func(*vmopv1.VirtualMachineAdvancedSpec)) {
			moVM = moVMWithExtraConfig(ov(key, raw))

			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
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
			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Advanced).To(BeNil())
		})
	})

	When("moVM ExtraConfig is empty", func() {
		It("returns no mutation", func() {
			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Advanced).To(BeNil())
		})
	})

	When("spec field already set — spec wins", func() {
		BeforeEach(func() {
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				PreferHTEnabled: ptr.To(true),
			}
			moVM = moVMWithExtraConfig(ov("numa.vcpu.preferHT", "FALSE"))
		})

		It("does not overwrite the existing value", func() {
			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
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
			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(*vm.Spec.Advanced.VMXSwapEnabled).To(BeFalse())
		})
	})

	DescribeTable("unknown / bookkeeping keys are silently dropped",
		func(key string) {
			moVM = moVMWithExtraConfig(ov(key, "value"))
			mutated, err := backfill.ExtraConfigFromMoVM(vm, moVM)
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
