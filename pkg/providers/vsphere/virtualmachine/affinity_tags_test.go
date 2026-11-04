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
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

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
		Expect(virtualmachine.GetVMTagsFromExtraConfig(nil)).To(BeEmpty())
		names := []string{"z:last", "", "app:", "app:"}
		var spec vimtypes.VirtualMachineConfigSpec
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, nil, &spec, names, nil)
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
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, nil, &spec, []string{"app:nginx"}, nil)
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.ExtraConfig).To(HaveLen(2))
		Expect(spec.ExtraConfig[0].GetOptionValue()).To(Equal(&vimtypes.OptionValue{
			Key: "unrelated", Value: "keep",
		}))
		Expect(spec.ExtraConfig[1].GetOptionValue().Value).To(Equal("app:nginx"))
	})
	It("records sorted unique names including empty-valued labels and self hashes", func() {
		var spec vimtypes.VirtualMachineConfigSpec
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, nil, &spec, []string{"app:", "1234567890abcdef", "app:", "af-hash-app:"}, nil)
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}
		value, ok := object.OptionValueList(config.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal("1234567890abcdef,af-hash-app:,app:"))
		Expect(virtualmachine.GetVMTagsFromExtraConfig(&config)).To(Equal([]string{"1234567890abcdef", "af-hash-app:", "app:"}))
	})
	It("diffs only recorded names and preserves unrelated outgoing operations", func() {
		config := vimtypes.VirtualMachineConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{
			&vimtypes.OptionValue{Key: virtualmachine.ExtraConfigVMTagsKey, Value: "old,keep"},
		}}
		uuid := vimtypes.TagSpec{Id: vimtypes.TagId{Uuid: "policy-tag"}}
		foreign := vimtypes.TagSpec{Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: "old", Category: "other"}}}
		spec := vimtypes.VirtualMachineConfigSpec{TagSpecs: []vimtypes.TagSpec{uuid, foreign}}
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, &config, &spec, []string{"new", "keep", "new"}, nil)
		Expect(spec.TagSpecs).To(HaveLen(4))
		Expect(spec.TagSpecs[:2]).To(Equal([]vimtypes.TagSpec{uuid, foreign}))
		Expect(spec.TagSpecs[2].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		Expect(spec.TagSpecs[2].Id.NameId.Tag).To(Equal("old"))
		Expect(spec.TagSpecs[3].Operation).To(Equal(vimtypes.ArrayUpdateOperationAdd))
		Expect(spec.TagSpecs[3].Id.NameId.Tag).To(Equal("new"))
		Expect(spec.TagSpecs[3].Id.NameId.Category).To(Equal(vm.Namespace))
		config.ExtraConfig = spec.ExtraConfig
		spec = vimtypes.VirtualMachineConfigSpec{}
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, &config, &spec, []string{"keep", "new"}, nil)
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
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, &config, &spec, nil, nil)
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
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, &config, &spec, nil, nil)
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.TagSpecs[0].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		config.ExtraConfig = spec.ExtraConfig
		value, ok := object.OptionValueList(config.ExtraConfig).GetString(virtualmachine.ExtraConfigVMTagsKey)
		Expect(ok).To(BeTrue())
		Expect(value).To(BeEmpty())
		spec = vimtypes.VirtualMachineConfigSpec{}
		virtualmachine.ReconcileVMTagsDuringVMUpdate(ctx, vm, &config, &spec, nil, nil)
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
			"0d65cedea5c5987a", "vmoperator:af-pds-pde-host-0d65cedea5c5987a-app:nginx"),
		Entry("grouped AAF self", virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
			[]virtualmachine.LabelPair{{Key: "app", Value: "database"}, {Key: "app", Value: "cache"}},
			"38640f88bae92217", "vmoperator:aaf-pds-pde-host-38640f88bae92217"),
		Entry("prefixed key with empty value", virtualmachine.AffinityPolicyTypeAffinity,
			[]virtualmachine.LabelPair{{Key: "example.com/app", Value: ""}},
			"1b34492bd93511ff", "vmoperator:af-pds-pde-host-1b34492bd93511ff-example.com/app:"),
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
		Entry("reserved AF key", virtualmachine.AffinityPolicyTypeAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "vmoperator.vmware.com/x", Value: "y"}}, "reserved affinity target key"),
		Entry("reserved key in AAF group", virtualmachine.AffinityPolicyTypeGroupedAntiAffinity, preferred, host,
			[]virtualmachine.LabelPair{{Key: "app", Value: "db"}, {Key: "vmoperator.vmware.com/x", Value: "y"}},
			"reserved affinity target key"),
	)
})

var _ = Describe("Local mutable affinity", Label(testlabels.Controller), func() {
	var vmCtx pkgctx.VirtualMachineContext
	term := func(value, topology string) vmopv1.VMAffinityTerm {
		return vmopv1.VMAffinityTerm{TopologyKey: topology, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": value}}}
	}
	create := func() vimtypes.VirtualMachineConfigSpec {
		var spec vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false, nil)).To(Succeed())
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
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out, nil)).To(Succeed())
		Expect(out.TagSpecs).To(Equal([]vimtypes.TagSpec{unrelated}))
		record, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(ContainSubstring("policies"))
		// The AF target tag is recorded, but never attached, for a VM that does
		// not carry the source label.
		Expect(virtualmachine.GetVMTagsFromExtraConfig(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: out.ExtraConfig})).
			To(Equal([]string{"vmoperator:af-pds-pde-host-0d65cedea5c5987a-app:nginx"}))
		applied := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: out.ExtraConfig}
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = append(
			vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution, term("nginx", corev1.LabelHostname))
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out, nil)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		vmCtx.VM.Spec.Affinity = nil
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out, nil)).To(Succeed())
		Expect(out.VmPlacementPolicies).To(BeEmpty())
		Expect(out.TagSpecs).To(HaveLen(1))
		Expect(out.TagSpecs[0].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		Expect(out.TagSpecs[0].Id.NameId.Tag).To(Equal("vmoperator:af-pds-pde-host-0d65cedea5c5987a-app:nginx"))
		record, ok = object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(Equal("{}"))
		applied.ExtraConfig = out.ExtraConfig
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, applied, &out, nil)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})

	It("detaches the recorded AF target tag of a term this VM does not match when the term is removed", func() {
		vmCtx.VM.Labels = map[string]string{"app": "database"}
		af := vmCtx.VM.Spec.Affinity.VMAffinity
		af.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{term("database", corev1.LabelHostname)}
		applied := create()
		af.RequiredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{term("web", corev1.LabelHostname)}
		config := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: applied.ExtraConfig}
		var added vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &added, nil)).To(Succeed())
		for _, tag := range added.TagSpecs {
			Expect(tag.Id.NameId.Tag).ToNot(ContainSubstring("web"), "the VM must not be made a web member")
		}
		recorded := virtualmachine.GetVMTagsFromExtraConfig(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: added.ExtraConfig})
		Expect(recorded).To(ContainElement(HaveSuffix("-app:web")))

		config = &vimtypes.VirtualMachineConfigInfo{ExtraConfig: added.ExtraConfig}
		af.RequiredDuringSchedulingPreferredDuringExecution = nil
		var removed vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &removed, nil)).To(Succeed())
		Expect(removed.TagSpecs).To(HaveLen(1))
		Expect(removed.TagSpecs[0].Operation).To(Equal(vimtypes.ArrayUpdateOperationRemove))
		Expect(removed.TagSpecs[0].Id.NameId.Tag).To(HaveSuffix("-app:web"))
	})

	It("keeps a missing empty policy snapshot as a no-op and retries after failure", func() {
		affinity := vmCtx.VM.Spec.Affinity
		vmCtx.VM.Spec.Affinity = nil
		out := vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out, nil)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		vmCtx.VM.Spec.Affinity = affinity
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &out, nil)).To(Succeed())
		retry := vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, nil, &retry, nil)).To(Succeed())
		Expect(retry).To(Equal(out), "failed tasks leave the applied snapshot unchanged")
	})

	It("records exact AF policy and local membership and leaves an unchanged update idle", func() {
		spec := create()
		Expect(virtualmachine.GetVMTagsFromExtraConfig(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig})).To(Equal([]string{"vmoperator:af-pds-pde-host-0d65cedea5c5987a-app:nginx", "vmoperator:app:nginx"}))
		Expect(spec.VmPlacementPolicies).To(HaveLen(1))
		Expect(spec.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:af-pds-pde-host-0d65cedea5c5987a-app:nginx"))
		record, ok := object.OptionValueList(spec.ExtraConfig).GetString("vmservice.placementpolicies")
		Expect(ok).To(BeTrue())
		Expect(record).To(ContainSubstring("policies"))
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &out, nil)).To(Succeed())
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
		Expect(p.SelfTag.NameId.Tag).To(Equal("vmoperator:aaf-pds-pde-host-38640f88bae92217"))
		Expect(p.AntiAffinedVmGroupTags).To(HaveLen(2))
		Expect(p.AntiAffinedVmGroupTags[0].NameId.Tag).To(Equal("vmoperator:app:cache"))
		Expect(p.AntiAffinedVmGroupTags[1].NameId.Tag).To(Equal("vmoperator:app:database"))
		Expect(spec.TagSpecs).To(HaveLen(1))
		Expect(spec.TagSpecs[0].Id).To(Equal(p.SelfTag))
		vmCtx.VM.Labels = map[string]string{"app": "cache"}
		spec = create()
		Expect(virtualmachine.GetVMTagsFromExtraConfig(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig})).To(ConsistOf("vmoperator:aaf-pds-pde-host-38640f88bae92217", "vmoperator:app:cache"))
		vmCtx.VM.Spec.Affinity = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &out, nil)).To(Succeed())
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
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &placed, true, nil)).To(Succeed())
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
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out, nil)).To(Succeed())
		Expect(out.TagSpecs).To(HaveLen(1))
		Expect(out.TagSpecs[0].Id.NameId.Tag).To(Equal("vmoperator:app:nginx"))
		Expect(out.VmPlacementPolicies).To(BeEmpty())
		_, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeFalse())
		var retry vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &retry, nil)).To(Succeed())
		Expect(retry).To(Equal(out))
	})
	It("changes AF strictness while retaining the shared original and clears only once", func() {
		spec := create()
		config := &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}
		af := vmCtx.VM.Spec.Affinity.VMAffinity
		af.RequiredDuringSchedulingPreferredDuringExecution = af.PreferredDuringSchedulingPreferredDuringExecution
		af.PreferredDuringSchedulingPreferredDuringExecution = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out, nil)).To(Succeed())
		Expect(out.VmPlacementPolicies).To(HaveLen(1))
		Expect(out.TagSpecs).To(HaveLen(2))
		for _, tag := range out.TagSpecs {
			Expect(tag.Id.NameId.Tag).NotTo(Equal("vmoperator:app:nginx"))
		}
		config.ExtraConfig = out.ExtraConfig
		vmCtx.VM.Spec.Affinity = nil
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out, nil)).To(Succeed())
		Expect(out.TagSpecs).To(HaveLen(2))
		record, ok := object.OptionValueList(out.ExtraConfig).GetString(virtualmachine.ExtraConfigPlacementPoliciesKey)
		Expect(ok).To(BeTrue())
		Expect(record).To(Equal("{}"))
		config.ExtraConfig = out.ExtraConfig
		out = vimtypes.VirtualMachineConfigSpec{}
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out, nil)).To(Succeed())
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
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, isPlacement, nil)).To(Succeed())
			Expect(spec.VmPlacementPolicies).To(Equal([]vimtypes.BaseVmPlacementPolicy{classPolicy}))
			Expect(spec.ExtraConfig).To(BeEmpty(), "class policies must not acquire a managed affinity receipt")
		}
	}, Entry("mutable without affinity", true, true), Entry("legacy ineligible affinity", false, false))
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
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &generated, nil)).To(Succeed())

		classPolicies := make([]vimtypes.BaseVmPlacementPolicy, 4, 8)
		classPolicies[0] = &vimtypes.VmPlacementPolicy{TagsToAttach: []string{"class-tag"}}
		classPolicies[1] = &vimtypes.VmVmAffinity{AffinedVmsTag: vimtypes.TagId{Uuid: "class-affinity"}}
		classPolicies[2] = &vimtypes.VmVmAntiAffinity{AntiAffinedVmsTag: vimtypes.TagId{Uuid: "class-anti-affinity"}}
		classPolicies[3] = &vimtypes.VmToVmGroupsAntiAffinity{SelfTag: vimtypes.TagId{Uuid: "class-self"}}
		out := vimtypes.VirtualMachineConfigSpec{VmPlacementPolicies: classPolicies}
		before, err := vimtypes.DeepCopy(out)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, config, &out, nil)).To(Succeed())
		Expect(out.VmPlacementPolicies[:len(classPolicies)]).To(Equal(before.VmPlacementPolicies))
		Expect(out.VmPlacementPolicies[len(classPolicies):]).To(ConsistOf(generated.VmPlacementPolicies))
		Expect(classPolicies).To(Equal(before.VmPlacementPolicies))
		Expect(classPolicies[len(classPolicies):cap(classPolicies)]).To(ConsistOf(nil, nil, nil, nil),
			"appending generated policies must not modify the caller's backing slice")
		Expect(out.ExtraConfig).To(Equal(generated.ExtraConfig), "class policies must stay outside the affinity receipt")
		Expect(out.TagSpecs).To(Equal(generated.TagSpecs))
	}, Entry("adding affinity", "add"), Entry("changing affinity", "change"),
		Entry("clearing affinity", "clear"))

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
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &generated, isPlacement, nil)).To(Succeed())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, isPlacement, nil)).To(Succeed())
		Expect(spec).To(Equal(generated))
		Expect(spec.VmPlacementPolicies).To(HaveLen(1))
		Expect(base).To(Equal(before), "replacing policies must not mutate the class configuration")
	}, Entry("mutable create", true, false), Entry("legacy placement", false, true))

	It("replaces create policies for placement without changing create state or receipts", func() {
		spec := create()
		before, err := vimtypes.DeepCopy(spec)
		Expect(err).NotTo(HaveOccurred())
		placed := spec
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &placed, true, nil)).To(Succeed())
		Expect(spec).To(Equal(before))
		Expect(placed.VmPlacementPolicies).To(HaveLen(1))
		Expect(placed.VmPlacementPolicies[0].(*vimtypes.VmVmAffinity).AffinedVmsTag.NameId.Tag).To(Equal("vmoperator:app:nginx"))
		Expect(placed.TagSpecs).To(Equal(spec.TagSpecs))
		Expect(placed.ExtraConfig).To(Equal(spec.ExtraConfig))

		var update vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig}, &update, nil)).To(Succeed())
		Expect(update).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
	})

	It("preserves unrelated names, UUID operations, and independent placement/create state", func() {
		base := vimtypes.VirtualMachineConfigSpec{TagSpecs: []vimtypes.TagSpec{
			{Id: vimtypes.TagId{Uuid: "user-uuid"}},
			{ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd}, Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Category: "ns", Tag: "user:keep"}}},
		}}
		spec, err := vimtypes.DeepCopy(base)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false, nil)).To(Succeed())
		before, err := vimtypes.DeepCopy(spec)
		Expect(err).NotTo(HaveOccurred())
		placed, err := virtualmachine.CreateConfigSpecForPlacement(vmCtx, spec, nil, nil)
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
	})

	It("does not add a tag twice when the spec already has its add operation", func() {
		spec := create()
		before, err := vimtypes.DeepCopy(spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &spec, false, nil)).To(Succeed())
		Expect(spec.TagSpecs).To(Equal(before.TagSpecs))
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
	It("keeps host and zone policies for CAPV and CAPW VMs", func() {
		vmCtx.VM.Spec.Affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution = []vmopv1.VMAffinityTerm{
			term("nginx", corev1.LabelHostname), term("nginx", corev1.LabelTopologyZone)}
		for _, key := range []string{"capv.vmware.com/cluster.role", "capw.vmware.com/cluster.role"} {
			vmCtx.VM.Labels[key] = "worker"
			Expect(create().VmPlacementPolicies).To(HaveLen(2))
			delete(vmCtx.VM.Labels, key)
		}
	})
	It("leaves configuration and receipts unchanged when mutable affinity is disabled", func() {
		applied := create()
		pkgcfg.SetContext(vmCtx, func(c *pkgcfg.Config) { c.Features.VMHardAffinityDuringExecution = false })
		vmCtx.VM.Spec.Affinity = nil
		var out vimtypes.VirtualMachineConfigSpec
		Expect(virtualmachine.UpdateConfigSpecAffinity(vmCtx.Context, vmCtx.VM, &vimtypes.VirtualMachineConfigInfo{ExtraConfig: applied.ExtraConfig}, &out, nil)).To(Succeed())
		Expect(out).To(Equal(vimtypes.VirtualMachineConfigSpec{}))
		before, err := vimtypes.DeepCopy(applied)
		Expect(err).NotTo(HaveOccurred())
		Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(vmCtx.Context, vmCtx.VM, &applied, false, nil)).To(Succeed())
		Expect(applied).To(Equal(before))
	})
})

var _ = Describe("TagName", func() {
	It("prefixes the key and value joined with a colon", func() {
		Expect(virtualmachine.TagName("app", "nginx")).To(Equal("vmoperator:app:nginx"))
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

func newAppDBHashes(
	pair virtualmachine.LabelPair,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
) (af, aaf virtualmachine.AffinityPolicyHash) {

	af, err := virtualmachine.NewAffinityPolicyHash(
		virtualmachine.AffinityPolicyTypeAffinity,
		strictness, topology, []virtualmachine.LabelPair{pair})
	Expect(err).ToNot(HaveOccurred())
	aaf, err = virtualmachine.NewAffinityPolicyHash(
		virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
		strictness, topology, []virtualmachine.LabelPair{pair})
	Expect(err).ToNot(HaveOccurred())
	return af, aaf
}

var _ = Describe("Affinity tag naming helpers",
	Label(testlabels.Controller), func() {

		const ns = "team-a"

		var (
			pair = virtualmachine.LabelPair{Key: "app", Value: "db"}

			strictness = vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution
			topology   = vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost

			afHash  virtualmachine.AffinityPolicyHash
			aafHash virtualmachine.AffinityPolicyHash
		)

		BeforeEach(func() {
			afHash, aafHash = newAppDBHashes(pair, strictness, topology)
		})

		Context("naming table", func() {
			It("produces the vCenter and prefix-free names for every tag kind", func() {
				h := aafHash.Hash
				a := afHash.Hash

				// Original.
				Expect(virtualmachine.TagName("app", "db")).To(Equal("vmoperator:app:db"))
				Expect(virtualmachine.PrefixFreeTagName("app", "db")).To(Equal("app:db"))

				// AF hashed.
				Expect(afHash.TagName).To(Equal("vmoperator:af-rds-pde-host-" + a + "-app:db"))
				Expect(virtualmachine.PrefixFreePolicyTagName(afHash)).
					To(Equal("af-rds-pde-host-" + a + "-app:db"))

				// AAF self.
				Expect(aafHash.TagName).To(Equal("vmoperator:aaf-rds-pde-host-" + h))
				Expect(virtualmachine.PrefixFreePolicyTagName(aafHash)).
					To(Equal("aaf-rds-pde-host-" + h))

				// AAF derived target.
				Expect(virtualmachine.PrefixFreeGroupedAntiAffinityTargetName(aafHash, pair)).
					To(Equal("aaf-rds-pde-host-" + h + "-app:db"))
				Expect(virtualmachine.GroupedAntiAffinityTargetTagName(aafHash, pair)).
					To(Equal("vmoperator:aaf-rds-pde-host-" + h + "-app:db"))
			})

			It("round trips the prefix and leaves unprefixed names alone", func() {
				for _, name := range []string{"app:db", "aaf-rds-pde-host-abc", "x:"} {
					vc := virtualmachine.AddAffinityTagPrefix(name)
					Expect(vc).To(Equal("vmoperator:" + name))
					Expect(virtualmachine.StripAffinityTagPrefix(vc)).To(Equal(name))
					Expect(virtualmachine.StripAffinityTagPrefix(name)).To(Equal(name))
				}
			})

			It("keeps the derived target distinct from the self tag", func() {
				Expect(virtualmachine.PrefixFreeGroupedAntiAffinityTargetName(aafHash, pair)).
					ToNot(Equal(virtualmachine.PrefixFreePolicyTagName(aafHash)))
			})

			It("is DNS-subdomain safe: \"tag-\" followed by 16 hex characters", func() {
				Expect(virtualmachine.ConcreteTagResourceName(ns, "app:db")).
					To(MatchRegexp(`^tag-[0-9a-f]{16}$`))
			})
		})
	})

var _ = Describe("Desired affinity Tags and label-derived Tags",
	Label(testlabels.Controller), func() {

		const ns = "team-a"

		var (
			ctx context.Context
			vm  *vmopv1.VirtualMachine
		)

		term := func(value, topology string) vmopv1.VMAffinityTerm {
			return vmopv1.VMAffinityTerm{
				TopologyKey: topology,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": value},
				},
			}
		}

		tagNames := func(tags []vspherepolv1.Tag) []string {
			var names []string
			for _, tag := range tags {
				names = append(names, tag.Spec.Name)
			}
			return names
		}

		BeforeEach(func() {
			ctx = pkgcfg.NewContextWithDefaultConfig()
			pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
				c.Features.TaggingAPI = true
				c.Features.VMAffinityDuringExecution = true
				c.Features.VMPlacementPolicies = true
				c.Features.VMHardAffinityDuringExecution = true
			})
			vm = builder.DummyVirtualMachine()
			vm.Namespace = ns
			vm.Labels = nil
		})

		Context("DesiredAffinityTags", func() {
			It("returns nothing without affinity", func() {
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())
			})

			It("returns nothing when hashed tags are disabled", func() {
				pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
					c.Features.VMHardAffinityDuringExecution = false
				})
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("db", corev1.LabelHostname)},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())
			})

			It("produces original, self and derived target tags for grouped anti-affinity", func() {
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("db", corev1.LabelHostname)},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				h, err := virtualmachine.NewAffinityPolicyHash(
					virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
					vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
					vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
					[]virtualmachine.LabelPair{{Key: "app", Value: "db"}})
				Expect(err).ToNot(HaveOccurred())

				Expect(tagNames(tags)).To(ConsistOf(
					"app:db",
					"aaf-rds-pde-host-"+h.Hash,
					"aaf-rds-pde-host-"+h.Hash+"-app:db",
				))
				for i := range tags {
					Expect(virtualmachine.ValidateTagIdentity(&tags[i])).To(Succeed())
					Expect(tags[i].Namespace).To(Equal(ns))
				}
			})

			It("produces original and hashed tags for affinity and skips ineligible topologies", func() {
				pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
					c.Features.VMAffinityDuringExecution = false
				})
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							term("web", corev1.LabelHostname),
							term("cache", corev1.LabelTopologyZone),
						},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				// Host rules are not eligible, so only the zone pair remains.
				Expect(tagNames(tags)).To(ConsistOf(
					"app:cache",
					ContainSubstring("af-pds-pde-zone-"),
				))
			})

			It("skips a reserved label key", func() {
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
							TopologyKey: corev1.LabelHostname,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"vmoperator.vmware.com/x": "y"},
							},
						}},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())
			})

			It("skips only the reserved pair of an affinity rule", func() {
				reserved := vmopv1.VMAffinityTerm{
					TopologyKey: corev1.LabelHostname,
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"vmoperator.vmware.com/x": "y"},
					},
				}
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							term("db", corev1.LabelHostname), reserved,
						},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tagNames(tags)).To(ConsistOf(
					"app:db",
					MatchRegexp(`^af-pds-pde-host-[0-9a-f]{16}-app:db$`),
				))
			})

			It("returns an error when two identities share one Tag name", func() {
				h, err := virtualmachine.NewAffinityPolicyHash(
					virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
					vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
					vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
					[]virtualmachine.LabelPair{{Key: "app", Value: "db"}})
				Expect(err).ToNot(HaveOccurred())
				// An affinity label key shaped like the derived target name
				// gives an original tag with the derived target's name.
				forgedKey := "aaf-rds-pde-host-" + h.Hash + "-app"
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
							TopologyKey: corev1.LabelHostname,
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{forgedKey: "db"},
							},
						}},
					},
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("db", corev1.LabelHostname)},
					},
				}
				_, err = virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).To(MatchError(ContainSubstring("conflicting identities for Tag")))
			})

			It("skips a whole anti-affinity group with a reserved key, as the policy does", func() {
				vm.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							term("db", corev1.LabelHostname),
							{
								TopologyKey: corev1.LabelHostname,
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"vmoperator.vmware.com/x": "y"},
								},
							},
						},
					},
				}
				tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())

				// The ConfigSpec must not carry a policy whose target Tags are
				// never created.
				var spec vimtypes.VirtualMachineConfigSpec
				Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(ctx, vm, &spec, false, nil)).To(Succeed())
				Expect(spec.VmPlacementPolicies).To(BeEmpty())
				Expect(spec.TagSpecs).To(BeEmpty())
			})
		})

		// Coverage part 1: every tag that B attaches today is covered by a
		// Tag in the desired set, so the CR layer cannot drift from B's generator.
		Context("coverage of B's attached tags", func() {
			type kind string
			const (
				af  kind = "affinity"
				aaf kind = "anti-affinity"
			)

			build := func(k kind, strictness, topology string, carries bool) {
				terms := []vmopv1.VMAffinityTerm{
					term("db", topology), term("web", topology),
				}
				if carries {
					vm.Labels = map[string]string{"app": "db"}
				}
				switch k {
				case af:
					a := &vmopv1.VMAffinitySpec{}
					switch strictness {
					case "pp":
						a.PreferredDuringSchedulingPreferredDuringExecution = terms
					case "rp":
						a.RequiredDuringSchedulingPreferredDuringExecution = terms
					case "rr":
						a.RequiredDuringSchedulingRequiredDuringExecution = terms
					}
					vm.Spec.Affinity = &vmopv1.AffinitySpec{VMAffinity: a}
				case aaf:
					a := &vmopv1.VMAntiAffinitySpec{}
					switch strictness {
					case "pp":
						a.PreferredDuringSchedulingPreferredDuringExecution = terms
					case "rp":
						a.RequiredDuringSchedulingPreferredDuringExecution = terms
					case "rr":
						a.RequiredDuringSchedulingRequiredDuringExecution = terms
					}
					vm.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: a}
				}
			}

			attached := func(spec vimtypes.VirtualMachineConfigSpec) []string {
				var names []string
				for _, ts := range spec.TagSpecs {
					if ts.Operation == vimtypes.ArrayUpdateOperationAdd && ts.Id.NameId != nil {
						names = append(names, ts.Id.NameId.Tag)
					}
				}
				return names
			}

			for _, c := range []struct {
				k          kind
				strictness string
				topology   string
				carries    bool
			}{
				{af, "pp", corev1.LabelHostname, true},
				{af, "rr", corev1.LabelTopologyZone, false},
				{aaf, "rp", corev1.LabelHostname, true},
				{aaf, "pp", corev1.LabelTopologyZone, false},
			} {
				name := fmt.Sprintf("%s %s %s carries=%v",
					c.k, c.strictness, strings.TrimPrefix(c.topology, "topology.kubernetes.io/"), c.carries)
				It(name, func() {
					build(c.k, c.strictness, c.topology, c.carries)

					tags, err := virtualmachine.DesiredAffinityTags(ctx, vm)
					Expect(err).ToNot(HaveOccurred())
					want := sets.New(virtualmachine.TagVCenterNames(tags)...)

					for _, placement := range []bool{false, true} {
						var spec vimtypes.VirtualMachineConfigSpec
						Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
							ctx, vm, &spec, placement, nil)).To(Succeed())
						for _, name := range attached(spec) {
							Expect(want.Has(name)).To(BeTrue(),
								"%q (placement=%v) is attached but has no Tag; desired=%v",
								name, placement, sets.List(want))
						}
					}
				})
			}
		})

		Context("ListLabelDerivedTags", func() {
			var (
				c        ctrlclient.Client
				declared []vspherepolv1.Tag
			)

			BeforeEach(func() {
				// A declaring VM has an anti-affinity against app=db.
				declarer := builder.DummyVirtualMachine()
				declarer.Namespace = ns
				declarer.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{term("db", corev1.LabelHostname)},
					},
				}
				var err error
				declared, err = virtualmachine.DesiredAffinityTags(ctx, declarer)
				Expect(err).ToNot(HaveOccurred())
				Expect(declared).To(HaveLen(3))

				var objs []ctrlclient.Object
				for i := range declared {
					objs = append(objs, declared[i].DeepCopy())
				}
				c = builder.NewFakeClient(objs...)
			})

			It("gives a label-only VM prefixed original and derived target names", func() {
				vm.Labels = map[string]string{"app": "db"}
				tags, err := virtualmachine.ListLabelDerivedTags(ctx, c, vm, nil)
				Expect(err).ToNot(HaveOccurred())

				names := virtualmachine.TagVCenterNames(tags)
				Expect(names).To(HaveLen(2))
				Expect(names).To(ContainElement("vmoperator:app:db"))
				Expect(names).To(ContainElement(MatchRegexp(`^vmoperator:aaf-rds-pde-host-[0-9a-f]{16}-app:db$`)))
				for _, name := range names {
					Expect(name).To(HavePrefix(virtualmachine.AffinityTagPrefix))
				}
			})

			It("returns nothing for a VM that does not carry the label", func() {
				vm.Labels = map[string]string{"app": "web"}
				tags, err := virtualmachine.ListLabelDerivedTags(ctx, c, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())
			})

			It("includes owned Tags that the cache does not list yet", func() {
				vm.Labels = map[string]string{"app": "db"}
				empty := builder.NewFakeClient()
				tags, err := virtualmachine.ListLabelDerivedTags(ctx, empty, vm, declared)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(HaveLen(2))
			})

			It("drops an index hit whose source label the VM no longer carries", func() {
				// The List ignores the field selector, as a stale index entry
				// would, so matches() must recheck the VM's labels.
				vm.Labels = map[string]string{"app": "web"}
				stale := builder.NewFakeClientWithInterceptors(interceptor.Funcs{
					List: func(_ context.Context, _ ctrlclient.WithWatch, list ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
						l := list.(*vspherepolv1.TagList)
						for i := range declared {
							l.Items = append(l.Items, *declared[i].DeepCopy())
						}
						return nil
					},
				})
				tags, err := virtualmachine.ListLabelDerivedTags(ctx, stale, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(BeEmpty())
			})

			It("returns the list error rather than a partial set", func() {
				vm.Labels = map[string]string{"app": "db"}
				failing := builder.NewFakeClientWithInterceptors(interceptor.Funcs{
					List: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectList, ...ctrlclient.ListOption) error {
						return fmt.Errorf("injected list failure")
					},
				})
				_, err := virtualmachine.ListLabelDerivedTags(ctx, failing, vm, nil)
				Expect(err).To(MatchError(ContainSubstring("list affinity Tags")))
			})
		})
	})

var _ = Describe("Group anti-affinity placement tags",
	Label(testlabels.Controller), func() {

		const ns = "team-a"

		var (
			ctx       context.Context
			requester *vmopv1.VirtualMachine
			peer      *vmopv1.VirtualMachine
			unrelated *vmopv1.VirtualMachine
		)

		names := func(spec vimtypes.VirtualMachineConfigSpec) []string {
			var out []string
			for _, ts := range spec.TagSpecs {
				if ts.Operation == vimtypes.ArrayUpdateOperationAdd && ts.Id.NameId != nil {
					out = append(out, ts.Id.NameId.Tag)
				}
			}
			return out
		}

		placementSpec := func(vm *vmopv1.VirtualMachine) *vimtypes.VirtualMachineConfigSpec {
			var spec vimtypes.VirtualMachineConfigSpec
			Expect(virtualmachine.AddAffinityPoliciesToConfigSpec(
				ctx, vm, &spec, true,
				&virtualmachine.LabelDerivedTags{Client: builder.NewFakeClient()})).To(Succeed())
			return &spec
		}

		BeforeEach(func() {
			ctx = pkgcfg.NewContextWithDefaultConfig()
			pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
				c.Features.TaggingAPI = true
				c.Features.VMAffinityDuringExecution = true
				c.Features.VMPlacementPolicies = true
				c.Features.VMHardAffinityDuringExecution = true
			})

			newVM := func(name string, labels map[string]string) *vmopv1.VirtualMachine {
				vm := builder.DummyVirtualMachine()
				vm.Name = name
				vm.Namespace = ns
				vm.Labels = labels
				return vm
			}
			requester = newVM("requester", map[string]string{"role": "web"})
			requester.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "db"},
						},
					}},
				},
			}
			peer = newVM("peer", map[string]string{"app": "db"})
			unrelated = newVM("unrelated", map[string]string{"app": "web"})
		})

		It("gives a peer that carries the targeted label the target tags", func() {
			members := []virtualmachine.AffinityPlacementMember{
				{VM: requester, ConfigSpec: placementSpec(requester)},
				{VM: peer, ConfigSpec: placementSpec(peer)},
				{VM: unrelated, ConfigSpec: placementSpec(unrelated)},
			}
			Expect(names(*members[1].ConfigSpec)).To(BeEmpty())

			Expect(virtualmachine.AddGroupAntiAffinityPlacementTags(ctx, members)).To(Succeed())

			// Placement targets the derived and the original names.
			Expect(names(*members[1].ConfigSpec)).To(ConsistOf(
				"vmoperator:app:db",
				MatchRegexp(`^vmoperator:aaf-rds-pde-host-[0-9a-f]{16}-app:db$`)))
			Expect(names(*members[2].ConfigSpec)).To(BeEmpty())
		})

		It("keeps tags already on the spec and does not duplicate them", func() {
			peerSpec := placementSpec(peer)
			peerSpec.TagSpecs = append(peerSpec.TagSpecs, vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Id:              vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: "vmoperator:app:db", Category: ns}},
			}, vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Id:              vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: "other", Category: ns}},
			})
			members := []virtualmachine.AffinityPlacementMember{
				{VM: requester, ConfigSpec: placementSpec(requester)},
				{VM: peer, ConfigSpec: peerSpec},
			}

			Expect(virtualmachine.AddGroupAntiAffinityPlacementTags(ctx, members)).To(Succeed())
			Expect(names(*peerSpec)).To(ConsistOf(
				"other", "vmoperator:app:db",
				MatchRegexp(`^vmoperator:aaf-rds-pde-host-[0-9a-f]{16}-app:db$`)))
		})

		It("does not write into the spare capacity of the caller's TagSpecs", func() {
			peerSpec := placementSpec(peer)
			other := vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Id:              vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: "other", Category: ns}},
			}
			callerTagSpecs := make([]vimtypes.TagSpec, 1, 8)
			callerTagSpecs[0] = other
			peerSpec.TagSpecs = callerTagSpecs
			members := []virtualmachine.AffinityPlacementMember{
				{VM: requester, ConfigSpec: placementSpec(requester)},
				{VM: peer, ConfigSpec: peerSpec},
			}

			Expect(virtualmachine.AddGroupAntiAffinityPlacementTags(ctx, members)).To(Succeed())
			Expect(names(*peerSpec)).To(HaveLen(3))
			// The caller's backing array is untouched beyond its length.
			Expect(callerTagSpecs).To(Equal([]vimtypes.TagSpec{other}))
			Expect(callerTagSpecs[:cap(callerTagSpecs)][1:]).To(HaveEach(Equal(vimtypes.TagSpec{})))
		})

		It("does nothing when the tagging API is disabled", func() {
			pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) { c.Features.TaggingAPI = false })
			members := []virtualmachine.AffinityPlacementMember{
				{VM: requester, ConfigSpec: placementSpec(requester)},
				{VM: peer, ConfigSpec: placementSpec(peer)},
			}
			Expect(virtualmachine.AddGroupAntiAffinityPlacementTags(ctx, members)).To(Succeed())
			Expect(names(*members[1].ConfigSpec)).To(BeEmpty())
		})
	})

var _ = Describe("Prefix-free tag names and Tag identity",
	Label(testlabels.Controller), func() {

		const ns = "team-a"

		var (
			pair = virtualmachine.LabelPair{Key: "app", Value: "db"}

			strictness = vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution
			topology   = vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost

			afHash  virtualmachine.AffinityPolicyHash
			aafHash virtualmachine.AffinityPolicyHash
		)

		BeforeEach(func() {
			afHash, aafHash = newAppDBHashes(pair, strictness, topology)
		})

		newTag := func(name string, spec vspherepolv1.TagSpec) *vspherepolv1.Tag {
			spec.Name = name
			return &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      virtualmachine.ConcreteTagResourceName(ns, name),
				},
				Spec: spec,
			}
		}

		policy := func(
			policyType virtualmachine.AffinityPolicyType,
			targets ...vspherepolv1.TagLabelPair) *vspherepolv1.TagPolicyIdentity {

			return &vspherepolv1.TagPolicyIdentity{
				Type:       string(policyType),
				Strictness: string(strictness),
				Topology:   string(topology),
				Targets:    targets,
			}
		}
		target := vspherepolv1.TagLabelPair{Key: "app", Value: "db"}

		originalTag := func() *vspherepolv1.Tag {
			return newTag("app:db", vspherepolv1.TagSpec{
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
				Key:        "app", Value: "db",
			})
		}
		afTag := func() *vspherepolv1.Tag {
			return newTag(virtualmachine.PrefixFreePolicyTagName(afHash),
				vspherepolv1.TagSpec{
					Kind:       vspherepolv1.TagKindAffinityTarget,
					Membership: vspherepolv1.TagMembershipSourceLabel,
					Key:        "app", Value: "db",
					Policy: policy(virtualmachine.AffinityPolicyTypeAffinity, target),
				})
		}
		selfTag := func() *vspherepolv1.Tag {
			return newTag(virtualmachine.PrefixFreePolicyTagName(aafHash),
				vspherepolv1.TagSpec{
					Kind:       vspherepolv1.TagKindGroupedAntiAffinitySelf,
					Membership: vspherepolv1.TagMembershipPolicyRequesters,
					Policy:     policy(virtualmachine.AffinityPolicyTypeGroupedAntiAffinity, target),
				})
		}
		aafTargetTag := func() *vspherepolv1.Tag {
			return newTag(virtualmachine.PrefixFreeGroupedAntiAffinityTargetName(aafHash, pair),
				vspherepolv1.TagSpec{
					Kind:       vspherepolv1.TagKindGroupedAntiAffinityTarget,
					Membership: vspherepolv1.TagMembershipSourceLabel,
					Key:        "app", Value: "db",
					Policy: policy(virtualmachine.AffinityPolicyTypeGroupedAntiAffinity, target),
				})
		}

		Context("ValidateTagIdentity", func() {
			DescribeTable("accepts a consistent identity",
				func(tag func() *vspherepolv1.Tag) {
					Expect(virtualmachine.ValidateTagIdentity(tag())).To(Succeed())
				},
				Entry("original", originalTag),
				Entry("affinity target", afTag),
				Entry("grouped anti-affinity self", selfTag),
				Entry("grouped anti-affinity target", aafTargetTag),
			)

			It("rejects a vCenter (prefixed) spec.name", func() {
				tag := originalTag()
				tag.Spec.Name = "vmoperator:app:db"
				tag.Name = virtualmachine.ConcreteTagResourceName(ns, tag.Spec.Name)
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("spec.name must equal")))
			})

			It("rejects an incorrect metadata.name", func() {
				tag := originalTag()
				tag.Name = "tag-0000000000000000"
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("metadata.name must equal")))
			})

			It("rejects invalid label syntax", func() {
				tag := originalTag()
				tag.Spec.Value = "has space"
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("invalid label value")))

				tag = originalTag()
				tag.Spec.Key = "bad key"
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("invalid label key")))
			})

			It("rejects a policy on an original and a missing policy on a derived tag", func() {
				tag := originalTag()
				tag.Spec.Policy = policy(virtualmachine.AffinityPolicyTypeAffinity, target)
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("spec.policy must be absent")))

				tag = afTag()
				tag.Spec.Policy = nil
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("spec.policy is required")))
			})

			It("rejects a wrong membership and a source label on a self tag", func() {
				tag := selfTag()
				tag.Spec.Membership = vspherepolv1.TagMembershipSourceLabel
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("PolicyRequesters")))

				tag = selfTag()
				tag.Spec.Key = "app"
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("must be absent")))
			})

			DescribeTable("rejects an inconsistent kind, membership, type or policy",
				func(tag func() *vspherepolv1.Tag, mutate func(*vspherepolv1.Tag), message string) {
					t := tag()
					mutate(t)
					Expect(virtualmachine.ValidateTagIdentity(t)).
						To(MatchError(ContainSubstring(message)))
				},
				Entry("non-System type", originalTag,
					func(t *vspherepolv1.Tag) { t.Spec.Type = vspherepolv1.TagType("User") },
					"spec.type must be System"),
				Entry("unsupported kind", originalTag,
					func(t *vspherepolv1.Tag) { t.Spec.Kind = vspherepolv1.TagKind("Unknown") },
					"unsupported spec.kind"),
				Entry("PolicyRequesters on an original tag", originalTag,
					func(t *vspherepolv1.Tag) { t.Spec.Membership = vspherepolv1.TagMembershipPolicyRequesters },
					"spec.membership must be SourceLabel"),
				Entry("PolicyRequesters on a grouped anti-affinity target", aafTargetTag,
					func(t *vspherepolv1.Tag) { t.Spec.Membership = vspherepolv1.TagMembershipPolicyRequesters },
					"spec.membership must be SourceLabel"),
				Entry("anti-affinity policy type on an affinity target", afTag,
					func(t *vspherepolv1.Tag) {
						t.Spec.Policy.Type = string(virtualmachine.AffinityPolicyTypeGroupedAntiAffinity)
					},
					"spec.policy.type must be VmVmAffinity"),
				Entry("affinity policy type on a self tag", selfTag,
					func(t *vspherepolv1.Tag) { t.Spec.Policy.Type = string(virtualmachine.AffinityPolicyTypeAffinity) },
					"spec.policy.type must be VmToVmGroupsAntiAffinity"),
				Entry("affinity target that differs from the source label", afTag,
					func(t *vspherepolv1.Tag) { t.Spec.Value = "web" },
					"spec.policy.targets must match the affinity source label"),
				Entry("reserved policy target key", aafTargetTag,
					func(t *vspherepolv1.Tag) {
						t.Spec.Policy.Targets = []vspherepolv1.TagLabelPair{{Key: "vmoperator.vmware.com/x", Value: "y"}}
					},
					"reserved label key"),
			)

			It("rejects unsorted or duplicate targets", func() {
				tag := aafTargetTag()
				tag.Spec.Policy.Targets = []vspherepolv1.TagLabelPair{
					{Key: "b", Value: "1"}, {Key: "a", Value: "1"},
				}
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("sorted and unique")))
			})

			It("rejects a source label that is not a policy target", func() {
				tag := aafTargetTag()
				tag.Spec.Key = "other"
				Expect(virtualmachine.ValidateTagIdentity(tag)).
					To(MatchError(ContainSubstring("must contain the grouped anti-affinity source label")))
			})

		})
	})
