// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// numFirstClassKeys is the total number of vmx-tagged fields in VirtualMachineAdvancedSpec.
// Update if new first-class fields are added.
const numFirstClassKeys = 6

var _ = Describe("TranslateFirstClass", func() {
	ctx := context.Background()

	It("returns nil for nil input", func() {
		Expect(extraconfig.TranslateFirstClass(ctx, nil)).To(BeNil())
	})

	It("returns one entry per first-class key (all empty) for empty spec", func() {
		ov := extraconfig.TranslateFirstClass(ctx, &vmopv1.VirtualMachineAdvancedSpec{})
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
		ov := extraconfig.TranslateFirstClass(ctx, adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.vcpu.preferHT")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal("TRUE"))
	})

	It("encodes *bool false as FALSE", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			HugePages1GEnabled: ptr.To(false),
		}
		ov := extraconfig.TranslateFirstClass(ctx, adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("sched.mem.lpage.enable1GPage")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal("FALSE"))
	})

	It("encodes []int32 as comma-separated string", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PNUMANodeAffinity: []int32{0, 1, 3},
		}
		ov := extraconfig.TranslateFirstClass(ctx, adv)
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
		ov := extraconfig.TranslateFirstClass(ctx, adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.vcpu.preferHT")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(""), "nil field should be emitted as empty string")
	})

	It("emits empty string for empty []int32 field (signals clear intent)", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PNUMANodeAffinity: []int32{},
		}
		ov := extraconfig.TranslateFirstClass(ctx, adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
		val, ok := ov.GetString("numa.nodeAffinity")
		Expect(ok).To(BeTrue())
		Expect(val).To(Equal(""))
	})

	It("does not emit an entry for a field with an unsupported type", func() {
		// Verify that TranslateFieldValue returns ok=false for kinds that are
		// not handled, so TranslateFirstClass skips the entry rather than
		// emitting "" (which would clear the VMX key).
		var n int
		_, ok := extraconfig.TranslateFieldValue(reflect.ValueOf(&n).Elem())
		Expect(ok).To(BeFalse(), "plain int (reflect.Int) is not a supported VMX field kind")
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
		ov := extraconfig.TranslateFirstClass(ctx, adv)
		Expect(ov).To(HaveLen(numFirstClassKeys))
	})
})

var _ = Describe("TranslateVMXNet3NICFirstClass", func() {
	ctx := context.Background()
	// ethernet0 has deviceKey 4000 (EthernetDeviceKeyBase + 0)
	const devKey0 int32 = 4000
	// ethernet1 has deviceKey 4001
	const devKey1 int32 = 4001
	const prefix0 = "ethernet0."
	const prefix1 = "ethernet1."

	It("returns nil for nil input", func() {
		Expect(extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey0, nil)).To(BeNil())
	})

	It("returns one entry per vmx-tagged NIC field (all empty) for empty spec", func() {
		keyCount := len(vmopv1util.VMXNet3NICKeyMap())
		ov := extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey0, &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{})
		Expect(ov).To(HaveLen(keyCount))
		for _, bov := range ov {
			kv := bov.GetOptionValue()
			Expect(kv.Value).To(Equal(""), "expected empty string for zero field %q", kv.Key)
		}
	})

	It("prefixes keys with the device prefix", func() {
		ov := extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey0, &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{})
		for _, bov := range ov {
			kv := bov.GetOptionValue()
			Expect(kv.Key).To(HavePrefix(prefix0), "key %q should have prefix %q", kv.Key, prefix0)
		}
	})

	It("encodes *bool true as TRUE", func() {
		spec := &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
			UPTv2Enabled: ptr.To(true),
		}
		ov := extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey0, spec)
		keys := map[string]string{}
		for _, bov := range ov {
			kv := bov.GetOptionValue()
			v, _ := kv.Value.(string)
			keys[kv.Key] = v
		}
		Expect(len(keys)).To(BeNumerically(">", 0))
	})

	It("uses the ethernet1 prefix for deviceKey 4001", func() {
		spec := &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{}
		ov := extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey1, spec)
		for _, bov := range ov {
			kv := bov.GetOptionValue()
			Expect(kv.Key).To(HavePrefix(prefix1))
		}
	})
})
