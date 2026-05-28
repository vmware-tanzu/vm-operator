// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// ovList is a helper to build a BaseOptionValue slice.
func ovList(pairs ...string) pkgutil.OptionValues {
	if len(pairs)%2 != 0 {
		panic("ovList: odd number of arguments")
	}
	out := make(pkgutil.OptionValues, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out[i/2] = &vimtypes.OptionValue{Key: pairs[i], Value: pairs[i+1]}
	}
	return out
}

var _ = Describe("LoadManagedKeys", func() {

	It("returns nil when key absent", func() {
		Expect(extraconfig.LoadManagedKeys(nil)).To(BeNil())
		Expect(extraconfig.LoadManagedKeys(pkgutil.OptionValues{})).To(BeNil())
	})

	It("returns nil when key empty", func() {
		obs := ovList(vsphereconst.ExtraConfigManagedKeysKey, "")
		Expect(extraconfig.LoadManagedKeys(obs)).To(BeNil())
	})

	It("parses comma-separated keys", func() {
		obs := ovList(vsphereconst.ExtraConfigManagedKeysKey, "foo,bar,baz")
		Expect(extraconfig.LoadManagedKeys(obs)).To(ConsistOf("foo", "bar", "baz"))
	})

	It("trims spaces", func() {
		obs := ovList(vsphereconst.ExtraConfigManagedKeysKey, " foo , bar ")
		Expect(extraconfig.LoadManagedKeys(obs)).To(ConsistOf("foo", "bar"))
	})
})

var _ = Describe("SemanticDiff", func() {

	ctx := context.Background()

	It("returns nil when merged is empty", func() {
		Expect(extraconfig.SemanticDiff(ctx, nil, nil)).To(BeNil())
	})

	It("passes through non-first-class keys using string equality", func() {
		observed := ovList("foo", "old")
		merged := ovList("foo", "new")
		out := extraconfig.SemanticDiff(ctx, observed, merged)
		Expect(out).To(HaveLen(1))
		v, _ := out.GetString("foo")
		Expect(v).To(Equal("new"))
	})

	It("suppresses non-first-class key when value unchanged", func() {
		observed := ovList("foo", "bar")
		merged := ovList("foo", "bar")
		Expect(extraconfig.SemanticDiff(ctx, observed, merged)).To(BeNil())
	})

	It("suppresses first-class key when semantically equal (true vs TRUE)", func() {
		observed := ovList("numa.vcpu.preferHT", "true")
		merged := ovList("numa.vcpu.preferHT", "TRUE")
		Expect(extraconfig.SemanticDiff(ctx, observed, merged)).To(BeNil())
	})

	It("emits first-class key when semantically different", func() {
		observed := ovList("numa.vcpu.preferHT", "TRUE")
		merged := ovList("numa.vcpu.preferHT", "FALSE")
		out := extraconfig.SemanticDiff(ctx, observed, merged)
		Expect(out).To(HaveLen(1))
		v, _ := out.GetString("numa.vcpu.preferHT")
		Expect(v).To(Equal("FALSE"))
	})

	It("emits first-class key when not in observed", func() {
		merged := ovList("numa.vcpu.preferHT", "TRUE")
		out := extraconfig.SemanticDiff(ctx, nil, merged)
		Expect(out).To(HaveLen(1))
	})

	It("emits reset (empty value) for first-class key when present in merged as empty", func() {
		observed := ovList("numa.vcpu.preferHT", "TRUE")
		merged := ovList("numa.vcpu.preferHT", "")
		out := extraconfig.SemanticDiff(ctx, observed, merged)
		Expect(out).To(HaveLen(1))
		v, _ := out.GetString("numa.vcpu.preferHT")
		Expect(v).To(Equal(""))
	})

	It("suppresses first-class reset when key not in observed", func() {
		merged := ovList("numa.vcpu.preferHT", "")
		Expect(extraconfig.SemanticDiff(ctx, nil, merged)).To(BeNil())
	})
})

var _ = Describe("TranslateFirstClass + SemanticDiff round-trip", func() {
	ctx := context.Background()

	It("produces no diff when spec matches observed (different string form)", func() {
		adv := &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled:    ptr.To(true),
			HugePages1GEnabled: ptr.To(false),
		}
		adv.ExtraConfig = []vmopv1common.KeyValuePair{{Key: "custom.key", Value: "v"}}

		// Observed values use lowercase variants (as ESXi might return).
		observed := ovList(
			"numa.vcpu.preferHT", "true",
			"sched.mem.lpage.enable1GPage", "false",
			"custom.key", "v",
			vsphereconst.ExtraConfigManagedKeysKey, "custom.key",
		)

		// Assembly: TranslateFirstClass + bag key.
		allDesiredEC := pkgutil.OptionValues(nil).Merge(extraconfig.TranslateFirstClass(ctx, adv)...).
			Merge(&vimtypes.OptionValue{Key: "custom.key", Value: "v"})

		out := extraconfig.SemanticDiff(ctx, observed, allDesiredEC)
		Expect(out).To(BeNil(), "expected no diff for semantically identical state")
	})
})
