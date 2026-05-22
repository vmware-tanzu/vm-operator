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

var _ = Describe("BackfillExtraConfigFromMoVM", func() {

	ctx := context.Background()

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

			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
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
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Advanced).To(BeNil())
		})
	})

	When("moVM ExtraConfig is empty", func() {
		It("returns no mutation", func() {
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
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
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
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
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(*vm.Spec.Advanced.VMXSwapEnabled).To(BeFalse())
		})
	})

	DescribeTable("auto/default/dontcare sentinels leave spec field nil and report no mutation",
		func(key, raw string) {
			moVM = moVMWithExtraConfig(ov(key, raw))

			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse(),
				"auto sentinel %q for key %q should not mutate spec", raw, key)
			// spec.Advanced must stay nil — no empty struct created.
			Expect(vm.Spec.Advanced).To(BeNil())
		},
		Entry("PreferHTEnabled auto",        "numa.vcpu.preferHT",                 "auto"),
		Entry("PreferHTEnabled AUTO",        "numa.vcpu.preferHT",                 "AUTO"),
		Entry("PreferHTEnabled DEFAULT",     "numa.vcpu.preferHT",                 "DEFAULT"),
		Entry("PreferHTEnabled default",     "numa.vcpu.preferHT",                 "default"),
		Entry("HugePages1GEnabled dontcare", "sched.mem.lpage.enable1GPage",       "dontcare"),
		Entry("HugePages1GEnabled auto",     "sched.mem.lpage.enable1GPage",       "auto"),
		Entry("TimeTracker auto",            "timeTracker.lowLatency",             "auto"),
		Entry("TimeTracker DEFAULT",         "timeTracker.lowLatency",             "DEFAULT"),
		Entry("CPUAffinity DONTCARE",        "sched.cpu.affinity.exclusiveNoStats","DONTCARE"),
		Entry("VMXSwap default",             "sched.swap.vmxSwapEnabled",          "default"),
	)

	When("all vmx-tagged keys carry auto sentinels", func() {
		BeforeEach(func() {
			moVM = moVMWithExtraConfig(
				ov("numa.vcpu.preferHT",                  "DEFAULT"),
				ov("sched.mem.lpage.enable1GPage",        "auto"),
				ov("timeTracker.lowLatency",              "auto"),
				ov("sched.cpu.affinity.exclusiveNoStats", "dontcare"),
				ov("sched.swap.vmxSwapEnabled",           "auto"),
			)
		})

		It("leaves spec.Advanced nil — no empty struct created", func() {
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Advanced).To(BeNil())
		})
	})

	DescribeTable("extended bool forms are accepted",
		func(key, raw string, wantTrue bool) {
			moVM = moVMWithExtraConfig(ov(key, raw))

			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeTrue(), "key %q raw %q should mutate", key, raw)
			Expect(vm.Spec.Advanced).ToNot(BeNil())
			Expect(vm.Spec.Advanced.PreferHTEnabled).ToNot(BeNil())
			Expect(*vm.Spec.Advanced.PreferHTEnabled).To(Equal(wantTrue))
		},
		Entry("yes → true",  "numa.vcpu.preferHT", "yes",  true),
		Entry("no → false",  "numa.vcpu.preferHT", "no",   false),
		Entry("on → true",   "numa.vcpu.preferHT", "on",   true),
		Entry("off → false",  "numa.vcpu.preferHT", "off",  false),
		Entry("1 → true",    "numa.vcpu.preferHT", "1",    true),
		Entry("0 → false",   "numa.vcpu.preferHT", "0",    false),
	)

	DescribeTable("unknown / bookkeeping keys are silently dropped",
		func(key string) {
			moVM = moVMWithExtraConfig(ov(key, "value"))
			mutated, err := backfill.ExtraConfigFromMoVM(ctx, vm, moVM)
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
