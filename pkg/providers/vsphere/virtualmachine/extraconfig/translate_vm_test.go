// Copyright (c) 2026 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// numFirstClassKeys is the total number of vmx-tagged fields in VirtualMachineAdvancedSpec.
// Update if new first-class fields are added.
const numFirstClassKeys = 6

var _ = Describe("TranslateFirstClass", func() {

	It("returns nil for nil input", func() {
		Expect(extraconfig.TranslateFirstClass(nil)).To(BeNil())
	})

	It("returns one entry per first-class key (all empty) for empty spec", func() {
		ov := extraconfig.TranslateFirstClass(&vmopv1.VirtualMachineAdvancedSpec{})
		Expect(ov).To(HaveLen(numFirstClassKeys))
		for _, bov := range ov {
			kv := bov.GetOptionValue()
			Expect(kv.Value).To(Equal(""), "expected empty string for zero field %q", kv.Key)
		}
	})

	It("encodes *bool true as TRUE", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled: ptr.To(true),
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.vcpu.preferHT")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal("TRUE"))
	})

	It("encodes *bool false as FALSE", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			HugePages1GEnabled: ptr.To(false),
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("sched.mem.lpage.enable1GPage")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal("FALSE"))
	})

	It("encodes []int32 as comma-separated string", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PNUMANodeAffinity: []int32{0, 1, 3},
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.nodeAffinity")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal("0,1,3"))
	})

	It("emits empty string for nil *bool field (signals clear intent)", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled:    nil,
			HugePages1GEnabled: ptr.To(true),
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.vcpu.preferHT")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(""), "nil field should be emitted as empty string")
	})

	It("emits empty string for empty []int32 field (signals clear intent)", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PNUMANodeAffinity: []int32{},
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.nodeAffinity")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(""))
	})

	It("encodes all six first-class fields when set", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled:                    ptr.To(true),
			HugePages1GEnabled:                 ptr.To(true),
			TimeTrackerLowLatencyEnabled:       ptr.To(false),
			CPUAffinityExclusiveNoStatsEnabled: ptr.To(true),
			VMXSwapEnabled:                     ptr.To(false),
			PNUMANodeAffinity:                  []int32{0},
		}
		ov := extraconfig.TranslateFirstClass(adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
	})
})
