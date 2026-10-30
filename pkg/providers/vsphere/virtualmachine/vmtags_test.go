// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Applied VM tag records", Label(testlabels.Controller), func() {
	var (
		ctx context.Context
		vm  *vmopv1.VirtualMachine
	)
	BeforeEach(func() {
		ctx = logr.NewContext(context.Background(), suite.GetLogger())
		vm = builder.DummyVirtualMachine()
	})
	It("treats an absent receipt as empty and leaves caller-owned names unchanged", func() {
		Expect(virtualmachine.RecordedVMTags(nil)).To(BeEmpty())
		names := []string{"z:last", "", "app:", "app:"}
		var spec vimtypes.VirtualMachineConfigSpec
		virtualmachine.ReconcileVMTags(ctx, vm, nil, &spec, names)
		Expect(names).To(Equal([]string{"z:last", "", "app:", "app:"}))
		Expect(spec.TagSpecs).To(HaveLen(2))
		Expect(spec.TagSpecs[0].Id.NameId.Tag).To(Equal("app:"))
		Expect(spec.TagSpecs[1].Id.NameId.Tag).To(Equal("z:last"))
		Expect(spec.TagSpecs[0].Operation).To(Equal(vimtypes.ArrayUpdateOperationAdd))
		Expect(spec.TagSpecs[1].Operation).To(Equal(vimtypes.ArrayUpdateOperationAdd))
	})
	It("does not duplicate an existing managed addition and preserves other ExtraConfig", func() {
		spec := vimtypes.VirtualMachineConfigSpec{
			TagSpecs: []vimtypes.TagSpec{{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{
					Tag: "app:nginx", Category: vm.Namespace,
				}},
			}},
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{Key: "unrelated", Value: "keep"},
			},
		}
		virtualmachine.ReconcileVMTags(ctx, vm, nil, &spec, []string{"app:nginx"})
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.ExtraConfig).To(HaveLen(2))
		Expect(spec.ExtraConfig[0].GetOptionValue()).To(Equal(&vimtypes.OptionValue{
			Key: "unrelated", Value: "keep",
		}))
		Expect(spec.ExtraConfig[1].GetOptionValue().Value).To(Equal("app:nginx"))
	})
	It("records sorted unique names including empty-valued labels and self hashes", func() {
		var spec vimtypes.VirtualMachineConfigSpec
		virtualmachine.ReconcileVMTags(ctx, vm, nil, &spec, []string{"app:", "1234567890abcdef", "app:", "af-hash-app:"})
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}
		value, ok := object.OptionValueList(config.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal("1234567890abcdef,af-hash-app:,app:"))
		Expect(virtualmachine.RecordedVMTags(&config)).To(Equal([]string{"1234567890abcdef", "af-hash-app:", "app:"}))
	})
	It("diffs only recorded names and preserves unrelated outgoing operations", func() {
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: "old,keep"},
		}}
		uuid := vimtypes.TagSpec{Id: vimtypes.TagId{Uuid: "policy-tag"}}
		foreign := vimtypes.TagSpec{Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: "old", Category: "other"}}}
		spec := vimtypes.VirtualMachineConfigSpec{TagSpecs: []vimtypes.TagSpec{uuid, foreign}}
		virtualmachine.ReconcileVMTags(ctx, vm, &config, &spec, []string{"new", "keep", "new"})
		Expect(spec.TagSpecs).To(HaveLen(4))
		Expect(spec.TagSpecs[:2]).To(Equal([]vimtypes.TagSpec{uuid, foreign}))
		Expect(spec.TagSpecs[2].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		Expect(spec.TagSpecs[2].Id.NameId.Tag).To(Equal("old"))
		Expect(spec.TagSpecs[3].Operation).To(Equal(vimtypes.ArrayUpdateOperationAdd))
		Expect(spec.TagSpecs[3].Id.NameId.Tag).To(Equal("new"))
		Expect(spec.TagSpecs[3].Id.NameId.Category).To(Equal(vm.Namespace))
		config.ExtraConfig = spec.ExtraConfig
		spec = vimtypes.VirtualMachineConfigSpec{}
		virtualmachine.ReconcileVMTags(ctx, vm, &config, &spec, []string{"keep", "new"})
		Expect(spec).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})
	It("keeps one receipt clear when retained-VM cleanup already scheduled it", func() {
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: "app:nginx,self"},
		}}
		spec := vimtypes.VirtualMachineConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: "unrelated", Value: "keep"},
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: ""},
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigPlacementPoliciesKey, Value: ""},
		}}
		virtualmachine.ReconcileVMTags(ctx, vm, &config, &spec, nil)
		Expect(spec.ExtraConfig).To(ConsistOf(
			&vimtypes.OptionValue{Key: "unrelated", Value: "keep"},
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: ""},
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigPlacementPoliciesKey, Value: ""},
		))
		Expect(spec.TagSpecs).To(HaveLen(2))
		Expect(spec.TagSpecs[0].Id.NameId.Tag).To(Equal("app:nginx"))
		Expect(spec.TagSpecs[1].Id.NameId.Tag).To(Equal("self"))
		for _, tag := range spec.TagSpecs {
			Expect(tag.Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
			Expect(tag.Id.NameId.Category).To(Equal(vm.Namespace))
		}
	})
	It("clears the record after removing its last tag and is then a no-op", func() {
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: "self"},
		}}
		var spec vimtypes.VirtualMachineConfigSpec
		virtualmachine.ReconcileVMTags(ctx, vm, &config, &spec, nil)
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.TagSpecs[0].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		config.ExtraConfig = spec.ExtraConfig
		value, ok := object.OptionValueList(config.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		Expect(ok).To(BeTrue())
		Expect(value).To(BeEmpty())
		spec = vimtypes.VirtualMachineConfigSpec{}
		virtualmachine.ReconcileVMTags(ctx, vm, &config, &spec, nil)
		Expect(spec).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})
})

var _ = Describe("Canonical affinity policy hash", Label(testlabels.Controller), func() {
	const (
		preferred = vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution
		required  = vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution
		hard      = vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution
		host      = vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost
		zone      = vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone
	)

	DescribeTable("matches canonical source reference vectors",
		func(kind virtualmachine.AffinityPolicyType, targets []virtualmachine.LabelPair, hash, name string) {
			policyHash, err := virtualmachine.NewAffinityPolicyHash(kind, preferred, host, targets)
			Expect(err).NotTo(HaveOccurred())
			Expect(policyHash.Hash).To(Equal(hash))
			Expect(policyHash.TagName).To(Equal(name))
			Expect(policyHash.Type).To(Equal(kind))
			Expect(policyHash.Strictness).To(Equal(preferred))
			Expect(policyHash.Topology).To(Equal(host))
		},
		Entry("AF target", virtualmachine.AffinityPolicyTypeAffinity,
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}},
			"0d65cedea5c5987a", "vmoperator:af-pp-host-0d65cedea5c5987a-app:nginx"),
		Entry("grouped AAF self", virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
			[]virtualmachine.LabelPair{{Key: "app", Value: "database"}, {Key: "app", Value: "cache"}},
			"38640f88bae92217", "vmoperator:aaf-pp-host-38640f88bae92217"),
		Entry("prefixed key with empty value", virtualmachine.AffinityPolicyTypeAffinity,
			[]virtualmachine.LabelPair{{Key: "example.com/app", Value: ""}},
			"1b34492bd93511ff", "vmoperator:af-pp-host-1b34492bd93511ff-example.com/app:"),
	)

	DescribeTable("includes policy type, strictness, and topology in tag names",
		func(strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
			topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology, prefix string) {
			for _, kind := range []virtualmachine.AffinityPolicyType{
				virtualmachine.AffinityPolicyTypeAffinity, virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
			} {
				policyHash, err := virtualmachine.NewAffinityPolicyHash(kind, strictness, topology,
					[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}})
				Expect(err).NotTo(HaveOccurred())
				expected := fmt.Sprintf("vmoperator:aaf-%s-%s", prefix, policyHash.Hash)
				if kind == virtualmachine.AffinityPolicyTypeAffinity {
					expected = fmt.Sprintf("vmoperator:af-%s-%s-app:nginx", prefix, policyHash.Hash)
				}
				Expect(policyHash.TagName).To(Equal(expected))
			}
		},
		Entry("preferred host", preferred, host, "pp-host"),
		Entry("required placement host", required, host, "pr-host"),
		Entry("required execution host", hard, host, "rr-host"),
		Entry("preferred zone", preferred, zone, "pp-zone"),
		Entry("required placement zone", required, zone, "pr-zone"),
		Entry("required execution zone", hard, zone, "rr-zone"),
	)

	It("canonicalizes duplicates and ordering without aliasing caller inputs", func() {
		targets := []virtualmachine.LabelPair{
			{Key: "app", Value: "database"}, {Key: "app", Value: "cache"}, {Key: "app", Value: "database"},
		}
		policyHash, err := virtualmachine.NewAffinityPolicyHash(
			virtualmachine.AffinityPolicyTypeGroupedAntiAffinity, preferred, host, targets)
		Expect(err).NotTo(HaveOccurred())
		Expect(policyHash.Targets).To(Equal([]virtualmachine.LabelPair{
			{Key: "app", Value: "cache"}, {Key: "app", Value: "database"},
		}))
		Expect(targets).To(HaveLen(3))
		Expect(targets[0].Value).To(Equal("database"))
		targets[0].Value = "changed"
		Expect(policyHash.Targets[1].Value).To(Equal("database"))
		Expect(policyHash.Hash).To(Equal("38640f88bae92217"))
	})

	It("distinguishes type, all strictness levels, topology, and resolved targets", func() {
		hashes := map[string]bool{}
		for _, kind := range []virtualmachine.AffinityPolicyType{
			virtualmachine.AffinityPolicyTypeAffinity, virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
		} {
			for _, strictness := range []vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness{preferred, required, hard} {
				for _, topology := range []vimtypes.VmPlacementPolicyVmPlacementPolicyTopology{host, zone} {
					for _, value := range []string{"nginx", "cache", ""} {
						policyHash, err := virtualmachine.NewAffinityPolicyHash(kind, strictness, topology,
							[]virtualmachine.LabelPair{{Key: "app", Value: value}})
						Expect(err).NotTo(HaveOccurred())
						Expect(hashes).NotTo(HaveKey(policyHash.Hash))
						hashes[policyHash.Hash] = true
					}
				}
			}
		}
		Expect(hashes).To(HaveLen(36))
	})

	DescribeTable("rejects invalid policy hash inputs",
		func(kind virtualmachine.AffinityPolicyType,
			strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
			topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
			targets []virtualmachine.LabelPair, message string) {
			policyHash, err := virtualmachine.NewAffinityPolicyHash(kind, strictness, topology, targets)
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(policyHash).To(Equal(virtualmachine.AffinityPolicyHash{}))
		},
		Entry("unknown type", virtualmachine.AffinityPolicyType("VmVmAntiAffinity"), preferred, host,
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}}, "unsupported affinity policy type"),
		Entry("unknown strictness", virtualmachine.AffinityPolicyTypeAffinity,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness("invalid"), host,
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}}, "unsupported affinity policy strictness"),
		Entry("unknown topology", virtualmachine.AffinityPolicyTypeAffinity, preferred,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopology("invalid"),
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}}, "unsupported affinity policy topology"),
		Entry("empty targets", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{}, "requires at least one target"),
		Entry("multiple AF targets", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx"}, {Key: "app", Value: "cache"}}, "one distinct target"),
		Entry("empty key", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "", Value: "nginx"}}, "invalid affinity target key"),
		Entry("ambiguous key delimiter", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "app:nginx", Value: ""}}, "invalid affinity target key"),
		Entry("invalid value", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "app", Value: "nginx:cache"}}, "invalid affinity target value"),
	)
})

var _ = Describe("Local mutable affinity", Label(testlabels.Controller), func() {
	var vmCtx pkgctx.VirtualMachineContext
	term := func(value, topology string) vmopv1.VMAffinityTerm {
		return vmopv1.VMAffinityTerm{TopologyKey: topology, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": value}}}
	}
	create := func() vimtypes.VirtualMachineConfigSpec {
		var spec vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false)).To(Succeed())
		return spec
	}
	BeforeEach(func() {
		vmCtx = pkgctx.VirtualMachineContext{Context: pkgcfg.NewContextWithDefaultConfig(), VM: builder.DummyVirtualMachine()}
		vmCtx.VM.Namespace = "ns"
		vmCtx.VM.Labels = map[string]string{"app": "nginx"}
		vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{VMAffinity: &vmopv1.VMAffinitySpec{
			PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("nginx", corev1.LabelHostname)},
		}}
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) {
			c.Features.TaggingAPI = false
			c.Features.VMAffinityDuringExecution = true
			c.Features.VMPlacementPolicies = true
			c.Features.VMHardAffinityDuringExecution = true
		})
	})
	It("compares policy-only receipts, suppresses equivalent intent, and clears once", func() {
		vmCtx.VM.Labels["app"] = "other"
		unrelated := vimtypes.TagSpec{Id: vimtypes.TagId{Uuid: "user-uuid"}}
		out := vimtypes.VirtualMachineConfigSpec{TagSpecs: []vimtypes.TagSpec{unrelated}}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out)).To(Succeed())
		Expect(out.TagSpecs).To(Equal([]vimtypes.TagSpec{unrelated}))
		record, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(ContainSubstring("policies"))
		_, ok = object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		Expect(ok).To(BeFalse())
		applied := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: out.ExtraConfig}
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = append(
			vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution, term("nginx", corev1.LabelHostname))
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		vmCtx.VM.Spec.Affinity = nil
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out)).To(Succeed())
		Expect(out.VmPlacementPolicies).To(BeEmpty())
		record, ok = object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(Equal("{}"))
		applied.ExtraConfig = out.ExtraConfig
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})

	It("keeps a missing empty policy snapshot as a no-op and retries after failure", func() {
		affinity := vmCtx.VM.Spec.Affinity
		vmCtx.VM.Spec.Affinity = nil
		out := vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		vmCtx.VM.Spec.Affinity = affinity
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out)).To(Succeed())
		retry := vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &retry)).To(Succeed())
		Expect(retry).To(Equal(out), "failed tasks leave the applied snapshot unchanged")
	})

	It("records exact AF policy and local membership and leaves an unchanged update idle", func() {
		spec := create()
		Expect(virtualmachine.RecordedVMTags(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig})).To(Equal([]string{"vmoperator:af-pp-host-0d65cedea5c5987a-app:nginx", "vmoperator:app:nginx"}))
		Expect(spec.VmPlacementPolicies).To(HaveLen(1))
		Expect(spec.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:af-pp-host-0d65cedea5c5987a-app:nginx"))
		record, ok := object.OptionValueList(spec.ExtraConfig).GetString("vmservice.placementpolicies")
		Expect(ok).To(BeTrue())
		Expect(record).To(ContainSubstring("policies"))
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})
	It("canonicalizes equivalent selectors, policy ordering, and duplicate terms", func() {
		first := create()
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{{
			TopologyKey: corev1.LabelHostname, LabelSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"nginx", "nginx"}}}},
		}}
		second := create()
		Expect(second).To(Equal(first))
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = append(vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution, term("nginx", corev1.LabelHostname))
		Expect(create()).To(Equal(first))
		vmCtx.VM.Name = "other"
		vmCtx.VM.Namespace = "another"
		third := create()
		Expect(third.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal(first.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag))
		Expect(third.TagSpecs[0].Id.NameId.Category).To(Equal("another"))
	})
	It("keeps original grouped targets and requester-only self membership", func() {
		vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
			term("database", corev1.LabelHostname), term("cache", corev1.LabelHostname), term("database", corev1.LabelHostname),
		}}}
		spec := create()
		p := spec.VmPlacementPolicies[0].(*vimtypes.VmToVmGroupsAntiAffinity)
		Expect(p.SelfTag.NameId.Tag).To(Equal("vmoperator:aaf-pp-host-38640f88bae92217"))
		Expect(p.AntiAffinedVmGroupTags).To(HaveLen(2))
		Expect(p.AntiAffinedVmGroupTags[0].NameId.Tag).To(Equal("vmoperator:app:cache"))
		Expect(p.AntiAffinedVmGroupTags[1].NameId.Tag).To(Equal("vmoperator:app:database"))
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.TagSpecs[0].Id).To(Equal(p.SelfTag))
		vmCtx.VM.Labels = map[string]string{"app": "cache"}
		spec = create()
		Expect(virtualmachine.RecordedVMTags(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig})).To(ConsistOf("vmoperator:aaf-pp-host-38640f88bae92217", "vmoperator:app:cache"))
		vmCtx.VM.Spec.Affinity = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &out)).To(Succeed())
		Expect(out.TagSpecs).To(HaveLen(2))
		for _, tag := range out.TagSpecs {
			Expect(tag.Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		}
	})
	It("keeps all six term lists and topology groups separate", func() {
		afTerms := []vmopv1.VMAffinityTerm{
			term("nginx", corev1.LabelHostname), term("nginx", corev1.LabelTopologyZone),
		}
		aafTerms := []vmopv1.VMAffinityTerm{
			term("database", corev1.LabelHostname), term("cache", corev1.LabelHostname),
			term("database", corev1.LabelHostname), term("nginx", corev1.LabelTopologyZone),
		}
		vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
			VMAffinity: &vmopv1.VMAffinitySpec{
				PreferredDuringSchedulingPreferredDuringExecution: afTerms,
				RequiredDuringSchedulingPreferredDuringExecution:  afTerms,
				RequiredDuringSchedulingRequiredDuringExecution:   afTerms,
			},
			VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
				PreferredDuringSchedulingPreferredDuringExecution: aafTerms,
				RequiredDuringSchedulingPreferredDuringExecution:  aafTerms,
				RequiredDuringSchedulingRequiredDuringExecution:   aafTerms,
			},
		}
		spec := create()
		var expected []vimtypes.BaseVmPlacementPolicy
		for _, strictness := range []vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness{
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution,
		} {
			for _, topology := range []vimtypes.VmPlacementPolicyVmPlacementPolicyTopology{
				vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
				vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone,
			} {
				nameID := func(name string) vimtypes.TagId {
					return vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Category: "ns", Tag: name}}
				}
				policyHash, err := virtualmachine.NewAffinityPolicyHash(virtualmachine.AffinityPolicyTypeAffinity,
					strictness, topology, []virtualmachine.LabelPair{{Key: "app", Value: "nginx"}})
				Expect(err).NotTo(HaveOccurred())
				expected = append(expected, &vimtypes.VmVmAffinity{
					AffinedVmsTag: nameID(policyHash.TagName), PolicyStrictness: string(strictness), PolicyTopology: string(topology),
				})
				pairs := []virtualmachine.LabelPair{{Key: "app", Value: "nginx"}}
				targets := []vimtypes.TagId{nameID("vmoperator:app:nginx")}
				if topology == vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost {
					pairs = []virtualmachine.LabelPair{{Key: "app", Value: "cache"}, {Key: "app", Value: "database"}}
					targets = []vimtypes.TagId{nameID("vmoperator:app:cache"), nameID("vmoperator:app:database")}
				}
				policyHash, err = virtualmachine.NewAffinityPolicyHash(virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
					strictness, topology, pairs)
				Expect(err).NotTo(HaveOccurred())
				expected = append(expected, &vimtypes.VmToVmGroupsAntiAffinity{
					SelfTag: nameID(policyHash.TagName), AntiAffinedVmGroupTags: targets,
					PolicyStrictness: string(strictness), PolicyTopology: string(topology),
				})
			}
		}
		Expect(spec.VmPlacementPolicies).To(ConsistOf(expected))
		Expect(spec.TagSpecs).To(HaveLen(13), "six AF hashes, six AAF self hashes, and one shared original")

		// Placement must generate the full attachment set even without a create receipt.
		var placed vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &placed, true)).To(Succeed())
		Expect(placed.TagSpecs).To(Equal(spec.TagSpecs))
		Expect(placed.ExtraConfig).To(BeEmpty())
		for _, policy := range placed.VmPlacementPolicies {
			if af, ok := policy.(*vimtypes.VmVmAffinity); ok {
				Expect(af.AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:app:nginx"))
			}
		}
	})
	It("skips an entire invalid selector without retaining its valid labels", func() {
		invalid := term("nginx", corev1.LabelHostname)
		invalid.LabelSelector.MatchExpressions = []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{invalid}
		vmCtx.VM.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
			PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{invalid, term("cache", corev1.LabelHostname)},
		}
		spec := create()
		Expect(spec.VmPlacementPolicies).To(HaveLen(1))
		policy := spec.VmPlacementPolicies[0].(*vimtypes.VmToVmGroupsAntiAffinity)
		Expect(policy.AntiAffinedVmGroupTags).To(HaveLen(1))
		Expect(policy.AntiAffinedVmGroupTags[0].NameId.Tag).To(Equal("vmoperator:app:cache"))
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.TagSpecs[0].Id).To(Equal(policy.SelfTag))
	})
	It("does not tag label-only VMs or fabricate target membership", func() {
		vmCtx.VM.Labels = map[string]string{"app": "other"}
		Expect(create().TagSpecs).To(BeEmpty())
		vmCtx.VM.Spec.Affinity = nil
		Expect(create()).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})
	It("changes policy independently from unchanged tags and retries from persisted state", func() {
		// A grouped policy remains desired while a nonparticipating label changes.
		vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("nginx", corev1.LabelHostname)}}}
		applied := create()
		vmCtx.VM.Labels = nil
		var out vimtypes.VirtualMachineConfigSpec
		config := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: applied.ExtraConfig}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out)).To(Succeed())
		Expect(out.TagSpecs).To(HaveLen(1))
		Expect(out.TagSpecs[0].Id.NameId.Tag).To(Equal("vmoperator:app:nginx"))
		Expect(out.VmPlacementPolicies).To(BeEmpty())
		_, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeFalse())
		var retry vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &retry)).To(Succeed())
		Expect(retry).To(Equal(out))
	})
	It("changes AF strictness while retaining the shared original and clears only once", func() {
		spec := create()
		config := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}
		af := vmCtx.VM.Spec.Affinity.VMAffinity
		af.RequiredDuringSchedulingPreferredDuringExecution = af.PreferredDuringSchedulingPreferredDuringExecution
		af.PreferredDuringSchedulingPreferredDuringExecution = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out)).To(Succeed())
		Expect(out.VmPlacementPolicies).To(HaveLen(1))
		Expect(out.TagSpecs).To(HaveLen(2))
		for _, tag := range out.TagSpecs {
			Expect(tag.Id.NameId.Tag).NotTo(Equal("vmoperator:app:nginx"))
		}
		config.ExtraConfig = out.ExtraConfig
		vmCtx.VM.Spec.Affinity = nil
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out)).To(Succeed())
		Expect(out.TagSpecs).To(HaveLen(2))
		record, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(Equal("{}"))
		config.ExtraConfig = out.ExtraConfig
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})
	DescribeTable("preserves class policies without generated affinity during create and placement", func(mutable, eligible bool) {
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) {
			c.Features.VMHardAffinityDuringExecution = mutable
			c.Features.VMAffinityDuringExecution = eligible
		})
		if eligible {
			vmCtx.VM.Spec.Affinity = nil
		}
		classPolicy := &vimtypes.VmVmAffinity{
			AffinedVmsTag:    vimtypes.TagId{Uuid: "class-affinity"},
			PolicyStrictness: string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:   string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
		}
		for _, isPlacement := range []bool{false, true} {
			spec := vimtypes.VirtualMachineConfigSpec{VmPlacementPolicies: []vimtypes.BaseVmPlacementPolicy{classPolicy}}
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, isPlacement)).To(Succeed())
			Expect(spec.VmPlacementPolicies).To(Equal([]vimtypes.BaseVmPlacementPolicy{classPolicy}))
			Expect(spec.ExtraConfig).To(BeEmpty(), "class policies must not acquire a managed affinity receipt")
		}
	}, Entry("mutable without affinity", true, true), Entry("legacy without affinity", false, true),
		Entry("mutable ineligible affinity", true, false), Entry("legacy ineligible affinity", false, false))
	DescribeTable("preserves outgoing class policies during affinity updates", func(operation string) {
		applied := create()
		config := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: applied.ExtraConfig}
		switch operation {
		case "add":
			config = nil
		case "change":
			af := vmCtx.VM.Spec.Affinity.VMAffinity
			af.RequiredDuringSchedulingPreferredDuringExecution = af.PreferredDuringSchedulingPreferredDuringExecution
			af.PreferredDuringSchedulingPreferredDuringExecution = nil
		case "clear":
			vmCtx.VM.Spec.Affinity = nil
		}
		generated := vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &generated)).To(Succeed())

		classPolicies := make([]vimtypes.BaseVmPlacementPolicy, 4, 8)
		classPolicies[0] = &vimtypes.VmPlacementPolicy{TagsToAttach: []string{"class-tag"}}
		classPolicies[1] = &vimtypes.VmVmAffinity{AffinedVmsTag: vimtypes.TagId{Uuid: "class-affinity"}}
		classPolicies[2] = &vimtypes.VmVmAntiAffinity{AntiAffinedVmsTag: vimtypes.TagId{Uuid: "class-anti-affinity"}}
		classPolicies[3] = &vimtypes.VmToVmGroupsAntiAffinity{SelfTag: vimtypes.TagId{Uuid: "class-self"}}
		out := vimtypes.VirtualMachineConfigSpec{VmPlacementPolicies: classPolicies}
		before, err := vimtypes.DeepCopy(out)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out)).To(Succeed())
		Expect(out.VmPlacementPolicies[:len(classPolicies)]).To(Equal(before.VmPlacementPolicies))
		Expect(out.VmPlacementPolicies[len(classPolicies):]).To(ConsistOf(generated.VmPlacementPolicies))
		Expect(classPolicies).To(Equal(before.VmPlacementPolicies))
		Expect(classPolicies[len(classPolicies):cap(classPolicies)]).To(ConsistOf(nil, nil, nil, nil),
			"appending generated policies must not modify the caller's backing slice")
		Expect(out.ExtraConfig).To(Equal(generated.ExtraConfig), "class policies must stay outside the affinity receipt")
		Expect(out.TagSpecs).To(Equal(generated.TagSpecs))
	}, Entry("adding affinity", "add"), Entry("changing affinity", "change"),
		Entry("clearing affinity", "clear"), Entry("unchanged affinity", "unchanged"))

	DescribeTable("replaces class policies with generated affinity during create and placement", func(mutable, isPlacement bool) {
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) { c.Features.VMHardAffinityDuringExecution = mutable })
		classPolicies := []vimtypes.BaseVmPlacementPolicy{
			&vimtypes.VmPlacementPolicy{TagsToAttach: []string{"class-tag"}},
			&vimtypes.VmVmAffinity{AffinedVmsTag: vimtypes.TagId{Uuid: "class-affinity"}},
			&vimtypes.VmVmAntiAffinity{AntiAffinedVmsTag: vimtypes.TagId{Uuid: "class-anti-affinity"}},
			&vimtypes.VmToVmGroupsAntiAffinity{SelfTag: vimtypes.TagId{Uuid: "class-self"}},
		}
		base := vimtypes.VirtualMachineConfigSpec{VmPlacementPolicies: classPolicies}
		before, err := vimtypes.DeepCopy(base)
		Expect(err).NotTo(HaveOccurred())
		spec := base
		var generated vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &generated, isPlacement)).To(Succeed())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, isPlacement)).To(Succeed())
		Expect(spec).To(Equal(generated))
		Expect(spec.VmPlacementPolicies).To(HaveLen(1))
		Expect(base).To(Equal(before), "replacing policies must not mutate the class configuration")
	}, Entry("mutable create", true, false), Entry("mutable placement", true, true),
		Entry("legacy create", false, false), Entry("legacy placement", false, true))

	It("replaces create policies for placement without changing create state or receipts", func() {
		spec := create()
		before, err := vimtypes.DeepCopy(spec)
		Expect(err).NotTo(HaveOccurred())
		placed := spec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &placed, true)).To(Succeed())
		Expect(spec).To(Equal(before))
		Expect(placed.VmPlacementPolicies).To(HaveLen(1))
		Expect(placed.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:app:nginx"))
		Expect(placed.TagSpecs).To(Equal(spec.TagSpecs))
		Expect(placed.ExtraConfig).To(Equal(spec.ExtraConfig))

		var update vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &update)).To(Succeed())
		Expect(update).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})

	It("preserves unrelated names, UUID operations, and independent placement/create state", func() {
		base := vimtypes.VirtualMachineConfigSpec{TagSpecs: []vimtypes.TagSpec{
			{Id: vimtypes.TagId{Uuid: "user-uuid"}},
			{ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd}, Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Category: "ns", Tag: "user:keep"}}},
		}}
		spec, err := vimtypes.DeepCopy(base)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false)).To(Succeed())
		before, err := vimtypes.DeepCopy(spec)
		Expect(err).NotTo(HaveOccurred())
		placed, err := virtualmachine.CreateConfigSpecForPlacement(vmCtx, spec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec).To(Equal(before))
		Expect(base.TagSpecs).To(HaveLen(2))
		Expect(placed.TagSpecs[:2]).To(Equal(base.TagSpecs))
		Expect(placed.TagSpecs).To(HaveLen(4))
		Expect(placed.TagSpecs).To(Equal(spec.TagSpecs))
		Expect(placed.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:app:nginx"))
		Expect(placed.ExtraConfig).To(Equal(spec.ExtraConfig))
		placed.TagSpecs[2].Id.NameId.Tag = "changed"
		Expect(spec).To(Equal(before))
		vmCtx.VM.Spec.Affinity = nil
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false)).To(Succeed())
		Expect(spec.TagSpecs).To(Equal(base.TagSpecs))
	})
	DescribeTable("respects execution topology and base capabilities", func(host, hard, zone bool, topology string, anti bool, policyCount int) {
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) {
			c.Features.VMAffinityDuringExecution = host
			c.Features.VMHardAffinityDuringExecution = hard
			c.Features.VMPlacementPolicies = zone
		})
		if anti {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("nginx", topology)}}}
		} else {
			vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{term("nginx", topology)}
		}
		Expect(create().VmPlacementPolicies).To(HaveLen(policyCount))
	}, Entry("host disabled", false, true, true, corev1.LabelHostname, false, 0), Entry("legacy AF", true, false, true, corev1.LabelHostname, false, 1),
		Entry("legacy AAF", true, false, true, corev1.LabelHostname, true, 1), Entry("zone placement only", true, false, true, corev1.LabelTopologyZone, false, 0),
		Entry("zone execution", true, true, true, corev1.LabelTopologyZone, false, 1), Entry("zone capability disabled", true, true, false, corev1.LabelTopologyZone, false, 0))
	It("keeps CAPV and CAPW exclusions in the standalone slice", func() {
		for _, key := range []string{"capv.vmware.com/cluster.role", "capw.vmware.com/cluster.role"} {
			vmCtx.VM.Labels[key] = "worker"
			Expect(create().VmPlacementPolicies).To(BeEmpty())
			delete(vmCtx.VM.Labels, key)
		}
	})
	It("leaves configuration and receipts unchanged when mutable affinity is disabled", func() {
		applied := create()
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) { c.Features.VMHardAffinityDuringExecution = false })
		vmCtx.VM.Spec.Affinity = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: applied.ExtraConfig}, &out)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		before, err := vimtypes.DeepCopy(applied)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &applied, false)).To(Succeed())
		Expect(applied).To(Equal(before))
	})
})

var _ = Describe("TagResourceName", func() {
	It("is deterministic for the same triple", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).To(
			Equal(virtualmachine.TagResourceName("ns", "app", "nginx")))
	})

	It("differs for different pairs", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "app", "apache")))
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "env", "nginx")))
	})

	It("differs for different namespaces with the same key/value pair", func() {
		Expect(virtualmachine.TagResourceName("ns-a", "app", "nginx")).ToNot(
			Equal(virtualmachine.TagResourceName("ns-b", "app", "nginx")))
	})

	It("is DNS-subdomain safe: \"tag-\" followed by 16 hex characters", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{16}$`))
	})

	It("handles an empty value", func() {
		name := virtualmachine.TagResourceName("ns", "app", "")
		Expect(name).To(MatchRegexp(`^tag-[0-9a-f]{16}$`))
		Expect(name).ToNot(Equal(virtualmachine.TagResourceName("ns", "app", "x")))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagResourceName("ns", "example.com/app", "nginx")).To(
			MatchRegexp(`^tag-[0-9a-f]{16}$`))
	})

	It("distinguishes an empty value from every other value for the same key", func() {
		Expect(virtualmachine.TagResourceName("ns", "app", "")).ToNot(
			Equal(virtualmachine.TagResourceName("ns", "app", "nginx")))
	})
})

var _ = Describe("TagName", func() {
	It("prefixes the key and value joined with a colon", func() {
		Expect(virtualmachine.TagName("app", "nginx")).To(Equal("vmoperator:app:nginx"))
	})

	It("handles an empty value", func() {
		Expect(virtualmachine.TagName("app", "")).To(Equal("vmoperator:app:"))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagName("example.com/app", "nginx")).To(
			Equal("vmoperator:example.com/app:nginx"))
	})

	DescribeTable("recovers the label pair after removing the ownership prefix",
		func(key, value string) {
			name := virtualmachine.TagName(key, value)
			pair, found := strings.CutPrefix(name, virtualmachine.AffinityTagPrefix)
			Expect(found).To(BeTrue())
			actualKey, actualValue, found := strings.Cut(pair, ":")
			Expect(found).To(BeTrue())
			Expect(actualKey).To(Equal(key))
			Expect(actualValue).To(Equal(value))
		},
		Entry("ordinary label", "app", "nginx"),
		Entry("empty value", "app", ""),
		Entry("qualified key", "example.com/app", "nginx"),
		Entry("key starting with the owner name", "vmoperator", "nginx"),
	)
})
