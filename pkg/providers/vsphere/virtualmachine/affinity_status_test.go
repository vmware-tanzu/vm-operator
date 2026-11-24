// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("AffinityStatusEnabled", Label(testlabels.Controller), func() {
	DescribeTable("returns whether the affinity status gate is on",
		func(taggingAPI, hardAffinity, expected bool) {
			ctx := pkgcfg.UpdateContext(
				pkgcfg.NewContextWithDefaultConfig(),
				func(config *pkgcfg.Config) {
					config.Features.TaggingAPI = taggingAPI
					config.Features.VMHardAffinityDuringExecution = hardAffinity
				})
			Expect(virtualmachine.AffinityStatusEnabled(ctx)).To(Equal(expected))
		},
		Entry("both features enabled", true, true, true),
		Entry("only TaggingAPI enabled", true, false, false),
		Entry("only VMHardAffinityDuringExecution enabled", false, true, false),
		Entry("both features disabled", false, false, false),
	)
})

var _ = Describe("RelatedAffinityPolicyTags", Label(testlabels.Controller), func() {
	const ns = "team-a"

	var (
		ctx       context.Context
		vm        *vmopv1.VirtualMachine
		initObjs  []ctrlclient.Object
		k8sClient ctrlclient.Client
	)

	term := func(key, value, topology string) vmopv1.VMAffinityTerm {
		return vmopv1.VMAffinityTerm{
			TopologyKey: topology,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{key: value},
			},
		}
	}

	kindsByName := func(tags []vspherepolv1.Tag) map[string]vspherepolv1.TagKind {
		m := map[string]vspherepolv1.TagKind{}
		for _, tag := range tags {
			m[tag.Spec.Name] = tag.Spec.Kind
		}
		return m
	}

	// declaredTags returns the Tags a VM with the affinity spec would own.
	declaredTags := func(affinity *vmopv1.AffinitySpec) []vspherepolv1.Tag {
		declarer := builder.DummyVirtualMachine()
		declarer.Namespace = ns
		declarer.Spec.Affinity = affinity
		tags, err := virtualmachine.DesiredAffinityTags(ctx, declarer)
		Expect(err).ToNot(HaveOccurred())
		Expect(tags).ToNot(BeEmpty())
		return tags
	}

	toObjects := func(tags []vspherepolv1.Tag) []ctrlclient.Object {
		objs := make([]ctrlclient.Object, 0, len(tags))
		for i := range tags {
			objs = append(objs, tags[i].DeepCopy())
		}
		return objs
	}

	antiAffinityDB := func() *vmopv1.AffinitySpec {
		return &vmopv1.AffinitySpec{
			VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
				RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
					term("app", "db", corev1.LabelHostname),
				},
			},
		}
	}

	affinityWeb := func() *vmopv1.AffinitySpec {
		return &vmopv1.AffinitySpec{
			VMAffinity: &vmopv1.VMAffinitySpec{
				RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
					term("tier", "web", corev1.LabelHostname),
				},
			},
		}
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
		initObjs = nil
		k8sClient = nil
	})

	JustBeforeEach(func() {
		if k8sClient == nil {
			k8sClient = builder.NewFakeClient(initObjs...)
		}
	})

	When("the VM has no affinity and no labels", func() {
		It("returns nothing", func() {
			tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(tags).To(BeEmpty())
		})
	})

	When("the VM owns a grouped anti-affinity policy", func() {
		BeforeEach(func() {
			vm.Spec.Affinity = antiAffinityDB()
		})

		When("the Tags exist", func() {
			BeforeEach(func() {
				tags := declaredTags(vm.Spec.Affinity)
				for i := range tags {
					tags[i].Status.ID = "urn:tag:" + tags[i].Spec.Name
				}
				initObjs = toObjects(tags)
			})

			It("returns only the self Tag, with its cached status", func() {
				tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(HaveLen(1))
				Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindGroupedAntiAffinitySelf))
				Expect(tags[0].Status.ID).To(Equal("urn:tag:" + tags[0].Spec.Name))
			})
		})

		When("the Tags do not exist yet", func() {
			It("returns the self Tag identity with an empty status", func() {
				tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(HaveLen(1))
				Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindGroupedAntiAffinitySelf))
				Expect(tags[0].Namespace).To(Equal(ns))
				Expect(tags[0].Name).To(Equal(
					virtualmachine.ConcreteTagResourceName(ns, tags[0].Spec.Name)))
				Expect(tags[0].Status).To(Equal(vspherepolv1.TagStatus{}))
			})
		})

		When("the VM also carries the anti-affinity target label", func() {
			BeforeEach(func() {
				vm.Labels = map[string]string{"app": "db"}
				initObjs = toObjects(declaredTags(vm.Spec.Affinity))
			})

			It("does not return the grouped anti-affinity target Tag", func() {
				tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(kindsByName(tags)).To(HaveLen(1))
				Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindGroupedAntiAffinitySelf))
			})
		})

		When("reading a Tag fails", func() {
			BeforeEach(func() {
				k8sClient = builder.NewFakeClientWithInterceptors(interceptor.Funcs{
					Get: func(
						context.Context,
						ctrlclient.WithWatch,
						ctrlclient.ObjectKey,
						ctrlclient.Object,
						...ctrlclient.GetOption) error {

						return fmt.Errorf("injected get failure")
					},
				})
			})

			It("returns the error", func() {
				_, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).To(MatchError(ContainSubstring("injected get failure")))
			})
		})
	})

	When("the VM owns an affinity policy", func() {
		BeforeEach(func() {
			vm.Spec.Affinity = affinityWeb()
			initObjs = toObjects(declaredTags(vm.Spec.Affinity))
		})

		It("returns the affinity target Tag", func() {
			tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(tags).To(HaveLen(1))
			Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindAffinityTarget))
		})

		When("the VM also carries the target label", func() {
			BeforeEach(func() {
				vm.Labels = map[string]string{"tier": "web"}
			})

			It("returns the affinity target Tag once", func() {
				tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(tags).To(HaveLen(1))
				Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindAffinityTarget))
			})
		})
	})

	When("the VM only carries labels", func() {
		BeforeEach(func() {
			vm.Labels = map[string]string{"tier": "web", "app": "db"}
			tags := declaredTags(affinityWeb())
			tags = append(tags, declaredTags(antiAffinityDB())...)
			for i := range tags {
				tags[i].Status.ID = "urn:tag:" + tags[i].Spec.Name
			}
			initObjs = toObjects(tags)
		})

		It("returns only the carried affinity target Tag, with its cached status", func() {
			tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(tags).To(HaveLen(1))
			Expect(tags[0].Spec.Kind).To(Equal(vspherepolv1.TagKindAffinityTarget))
			Expect(tags[0].Spec.Key).To(Equal("tier"))
			Expect(tags[0].Status.ID).To(Equal("urn:tag:" + tags[0].Spec.Name))
		})

		When("listing Tags fails", func() {
			BeforeEach(func() {
				k8sClient = builder.NewFakeClientWithInterceptors(interceptor.Funcs{
					List: func(
						context.Context,
						ctrlclient.WithWatch,
						ctrlclient.ObjectList,
						...ctrlclient.ListOption) error {

						return fmt.Errorf("injected list failure")
					},
				})
			})

			It("returns the error", func() {
				_, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
				Expect(err).To(MatchError(ContainSubstring("injected list failure")))
			})
		})
	})

	When("the VM owns and carries several policies", func() {
		BeforeEach(func() {
			vm.Spec.Affinity = antiAffinityDB()
			vm.Labels = map[string]string{"tier": "web"}
			initObjs = toObjects(append(
				declaredTags(vm.Spec.Affinity),
				declaredTags(affinityWeb())...))
		})

		It("returns them sorted by Tag name", func() {
			tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(tags).To(HaveLen(2))
			Expect(tags[0].Name < tags[1].Name).To(BeTrue())
			Expect(kindsByName(tags)).To(ConsistOf(
				vspherepolv1.TagKindAffinityTarget,
				vspherepolv1.TagKindGroupedAntiAffinitySelf,
			))
		})
	})

	When("hashed tags are disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(c *pkgcfg.Config) {
				c.Features.VMHardAffinityDuringExecution = false
			})
			vm.Spec.Affinity = antiAffinityDB()
		})

		It("returns no owned Tags", func() {
			tags, err := virtualmachine.RelatedAffinityPolicyTags(ctx, k8sClient, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(tags).To(BeEmpty())
		})
	})
	Context("OwnedAffinityPolicyTagNames", func() {
		When("the VM has no affinity", func() {
			It("returns no names", func() {
				vm.Labels = map[string]string{"tier": "web"}
				names, err := virtualmachine.OwnedAffinityPolicyTagNames(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(names).To(BeEmpty())
			})
		})

		When("the VM requests a grouped anti-affinity policy", func() {
			It("returns only the self Tag name", func() {
				vm.Spec.Affinity = antiAffinityDB()
				names, err := virtualmachine.OwnedAffinityPolicyTagNames(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var self []string
				for _, tag := range declaredTags(vm.Spec.Affinity) {
					if tag.Spec.Kind == vspherepolv1.TagKindGroupedAntiAffinitySelf {
						self = append(self, tag.Name)
					}
				}
				Expect(self).To(HaveLen(1))
				Expect(names.UnsortedList()).To(ConsistOf(self))
			})
		})

		When("the VM requests an affinity policy", func() {
			It("returns the affinity target Tag name", func() {
				vm.Spec.Affinity = affinityWeb()
				names, err := virtualmachine.OwnedAffinityPolicyTagNames(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var targets []string
				for _, tag := range declaredTags(vm.Spec.Affinity) {
					if tag.Spec.Kind == vspherepolv1.TagKindAffinityTarget {
						targets = append(targets, tag.Name)
					}
				}
				Expect(targets).To(HaveLen(1))
				Expect(names.UnsortedList()).To(ConsistOf(targets))
			})
		})
	})
})

var _ = Describe("AffinityComplianceFromVCenter", Label(testlabels.Controller), func() {
	DescribeTable("maps vCenter compliance",
		func(in string, expected vmopv1.VirtualMachineAffinityCompliance) {
			Expect(virtualmachine.AffinityComplianceFromVCenter(in)).To(Equal(expected))
		},
		Entry("Compliant", "Compliant", vmopv1.VirtualMachineAffinityComplianceCompliant),
		Entry("NonCompliant", "NonCompliant", vmopv1.VirtualMachineAffinityComplianceNotCompliant),
		Entry("NotApplicable", "NotApplicable", vmopv1.VirtualMachineAffinityComplianceNotApplicable),
		Entry("Unknown", "Unknown", vmopv1.VirtualMachineAffinityComplianceUnknown),
		Entry("REST form is not a property value", "COMPLIANT", vmopv1.VirtualMachineAffinityComplianceUnknown),
		Entry("empty", "", vmopv1.VirtualMachineAffinityComplianceUnknown),
		Entry("unrecognized", "SOMETHING_NEW", vmopv1.VirtualMachineAffinityComplianceUnknown),
	)
})

var _ = Describe("AffinityPolicyStatusFromTag", Label(testlabels.Controller), func() {
	newTag := func(
		kind vspherepolv1.TagKind,
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
		targets ...vspherepolv1.TagLabelPair) *vspherepolv1.Tag {

		return &vspherepolv1.Tag{
			Spec: vspherepolv1.TagSpec{
				Kind: kind,
				Policy: &vspherepolv1.TagPolicyIdentity{
					Strictness: string(strictness),
					Topology:   string(topology),
					Targets:    targets,
				},
			},
		}
	}

	appDB := vspherepolv1.TagLabelPair{Key: "app", Value: "db"}

	DescribeTable("maps the kind, strictness, and topology",
		func(
			kind vspherepolv1.TagKind,
			strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
			topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
			expected vmopv1.VirtualMachineAffinityPolicyStatus) {

			status, ok := virtualmachine.AffinityPolicyStatusFromTag(
				newTag(kind, strictness, topology, appDB),
				vmopv1.VirtualMachineAffinityComplianceCompliant)
			Expect(ok).To(BeTrue())
			expected.Targets = []vmopv1.VirtualMachineAffinityLabel{{Key: "app", Value: "db"}}
			expected.Compliance = vmopv1.VirtualMachineAffinityComplianceCompliant
			Expect(status).To(Equal(expected))
		},
		Entry("affinity, required/required, host",
			vspherepolv1.TagKindAffinityTarget,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
			vmopv1.VirtualMachineAffinityPolicyStatus{
				Type:        vmopv1.VirtualMachineAffinityPolicyTypeAffinity,
				Strictness:  vmopv1.VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingRequiredDuringExecution,
				TopologyKey: corev1.LabelHostname,
			}),
		Entry("anti-affinity, required/preferred, host",
			vspherepolv1.TagKindGroupedAntiAffinitySelf,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
			vmopv1.VirtualMachineAffinityPolicyStatus{
				Type:        vmopv1.VirtualMachineAffinityPolicyTypeAntiAffinity,
				Strictness:  vmopv1.VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingPreferredDuringExecution,
				TopologyKey: corev1.LabelHostname,
			}),
		Entry("affinity, preferred/preferred, zone",
			vspherepolv1.TagKindAffinityTarget,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone,
			vmopv1.VirtualMachineAffinityPolicyStatus{
				Type:        vmopv1.VirtualMachineAffinityPolicyTypeAffinity,
				Strictness:  vmopv1.VirtualMachineAffinityPolicyStrictnessPreferredDuringSchedulingPreferredDuringExecution,
				TopologyKey: corev1.LabelTopologyZone,
			}),
	)

	It("leaves the policy ID empty and keeps the given compliance", func() {
		status, ok := virtualmachine.AffinityPolicyStatusFromTag(
			newTag(vspherepolv1.TagKindAffinityTarget,
				vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
				vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost, appDB),
			vmopv1.VirtualMachineAffinityCompliancePending)
		Expect(ok).To(BeTrue())
		Expect(status.PolicyID).To(BeEmpty())
		Expect(status.Compliance).To(Equal(vmopv1.VirtualMachineAffinityCompliancePending))
	})

	It("sorts targets by key, then value, and removes duplicates", func() {
		// "a-b:x" sorts before "a:y" as a string, but key "a" sorts first.
		status, ok := virtualmachine.AffinityPolicyStatusFromTag(
			newTag(vspherepolv1.TagKindGroupedAntiAffinitySelf,
				vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
				vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
				vspherepolv1.TagLabelPair{Key: "a-b", Value: "x"},
				vspherepolv1.TagLabelPair{Key: "a", Value: "z"},
				vspherepolv1.TagLabelPair{Key: "a", Value: "y"},
				vspherepolv1.TagLabelPair{Key: "a", Value: "y"}),
			vmopv1.VirtualMachineAffinityComplianceUnknown)
		Expect(ok).To(BeTrue())
		Expect(status.Targets).To(Equal([]vmopv1.VirtualMachineAffinityLabel{
			{Key: "a", Value: "y"},
			{Key: "a", Value: "z"},
			{Key: "a-b", Value: "x"},
		}))
	})

	It("does not modify the Tag", func() {
		tag := newTag(vspherepolv1.TagKindGroupedAntiAffinitySelf,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
			vspherepolv1.TagLabelPair{Key: "b", Value: "x"},
			vspherepolv1.TagLabelPair{Key: "a", Value: "x"})
		orig := tag.DeepCopy()
		_, ok := virtualmachine.AffinityPolicyStatusFromTag(
			tag, vmopv1.VirtualMachineAffinityComplianceUnknown)
		Expect(ok).To(BeTrue())
		Expect(tag).To(Equal(orig))
	})

	DescribeTable("returns false for a Tag without a policy mapping",
		func(mutate func(*vspherepolv1.Tag)) {
			tag := newTag(vspherepolv1.TagKindAffinityTarget,
				vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
				vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost, appDB)
			mutate(tag)
			_, ok := virtualmachine.AffinityPolicyStatusFromTag(
				tag, vmopv1.VirtualMachineAffinityComplianceCompliant)
			Expect(ok).To(BeFalse())
		},
		Entry("no policy", func(t *vspherepolv1.Tag) { t.Spec.Policy = nil }),
		Entry("Original kind", func(t *vspherepolv1.Tag) {
			t.Spec.Kind = vspherepolv1.TagKindOriginal
		}),
		Entry("GroupedAntiAffinityTarget kind", func(t *vspherepolv1.Tag) {
			t.Spec.Kind = vspherepolv1.TagKindGroupedAntiAffinityTarget
		}),
		Entry("unknown strictness", func(t *vspherepolv1.Tag) {
			t.Spec.Policy.Strictness = "Sometimes"
		}),
		Entry("unknown topology", func(t *vspherepolv1.Tag) {
			t.Spec.Policy.Topology = "Rack"
		}),
	)
})

var _ = Describe("SortAffinityPolicyStatuses", Label(testlabels.Controller), func() {
	entry := func(
		topologyKey string,
		strictness vmopv1.VirtualMachineAffinityPolicyStrictness,
		typ vmopv1.VirtualMachineAffinityPolicyType,
		policyID string,
		targets ...string) vmopv1.VirtualMachineAffinityPolicyStatus {

		e := vmopv1.VirtualMachineAffinityPolicyStatus{
			TopologyKey: topologyKey,
			Strictness:  strictness,
			Type:        typ,
			PolicyID:    policyID,
		}
		for _, v := range targets {
			e.Targets = append(e.Targets, vmopv1.VirtualMachineAffinityLabel{Key: "app", Value: v})
		}
		return e
	}

	const (
		pp = vmopv1.VirtualMachineAffinityPolicyStrictnessPreferredDuringSchedulingPreferredDuringExecution
		rp = vmopv1.VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingPreferredDuringExecution
		af = vmopv1.VirtualMachineAffinityPolicyTypeAffinity
		aa = vmopv1.VirtualMachineAffinityPolicyTypeAntiAffinity
	)

	It("sorts by topology key, strictness, type, targets, and policy ID", func() {
		expected := []vmopv1.VirtualMachineAffinityPolicyStatus{
			entry(corev1.LabelHostname, pp, af, "", "db"),
			entry(corev1.LabelHostname, pp, aa, "", "db"),
			entry(corev1.LabelHostname, rp, af, "", "db"),
			entry(corev1.LabelHostname, rp, aa, "", "a"),
			entry(corev1.LabelHostname, rp, aa, "", "a", "b"),
			entry(corev1.LabelHostname, rp, aa, "", "b"),
			entry(corev1.LabelHostname, rp, aa, "p1", "c"),
			entry(corev1.LabelHostname, rp, aa, "p2", "c"),
			entry(corev1.LabelTopologyZone, pp, af, "", "db"),
		}
		reversed := make([]vmopv1.VirtualMachineAffinityPolicyStatus, len(expected))
		for i := range expected {
			reversed[len(expected)-1-i] = expected[i]
		}
		virtualmachine.SortAffinityPolicyStatuses(reversed)
		Expect(reversed).To(Equal(expected))
	})
})
