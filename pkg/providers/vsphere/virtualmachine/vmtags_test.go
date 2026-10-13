// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("AffinityLabelPairs", func() {
	var (
		vm          *vmopv1.VirtualMachine
		vmCtx       pkgctx.VirtualMachineContext
		constraints virtualmachine.AffinityRuleConstraints
	)

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()
		vm.Name = "test-vm"

		constraints = virtualmachine.AffinityRuleConstraints{
			ConfigureHostRules: true,
			ConfigureZoneRules: true,
		}

		vmCtx = pkgctx.VirtualMachineContext{
			Context: pkgcfg.NewContext(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	When("the VM has no affinity spec", func() {
		It("returns nil", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(BeNil())
		})
	})

	When("the VM affinity references matchLabels", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "nginx"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns the referenced pair", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "app", Value: "nginx"},
			))
		})
	})

	When("the VM affinity references matchExpressions with the In operator", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"nginx", "apache"},
									},
								},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns one pair per value", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "app", Value: "nginx"},
				virtualmachine.LabelPair{Key: "app", Value: "apache"},
			))
		})
	})

	When("the VM affinity references an unsupported matchExpressions operator", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("ignores the term rather than failing", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(BeEmpty())
		})
	})

	When("the VM affinity references a mix of eligible and ineligible terms", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpExists,
									},
								},
							},
							TopologyKey: corev1.LabelHostname,
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "prod"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("skips the failing term and still returns the eligible pair", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "env", Value: "prod"},
			))
		})
	})

	When("the VM anti-affinity references matchLabels", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
					PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tier": "backend"},
							},
							TopologyKey: corev1.LabelTopologyZone,
						},
					},
				},
			}
		})

		It("returns the referenced pair", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "tier", Value: "backend"},
			))
		})
	})

	When("the VM references the same pair from multiple terms", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "nginx"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
					PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "nginx"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns the pair only once", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "app", Value: "nginx"},
			))
		})
	})

	When("the referenced label has an empty value", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": ""},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns the pair with the empty value", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "app", Value: ""},
			))
		})
	})

	When("the referenced label key is prefixed", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"example.com/app": "nginx"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns the pair with the full prefixed key", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "example.com/app", Value: "nginx"},
			))
		})
	})

	When("the VM references pairs via both host and zone topology terms", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Affinity = &vmopv1.AffinitySpec{
				VMAffinity: &vmopv1.VMAffinitySpec{
					RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"zone-pair": "z"},
							},
							TopologyKey: corev1.LabelTopologyZone,
						},
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"host-pair": "h"},
							},
							TopologyKey: corev1.LabelHostname,
						},
					},
				},
			}
		})

		It("returns both pairs regardless of topology key", func() {
			Expect(virtualmachine.AffinityLabelPairs(vmCtx, constraints)).To(ConsistOf(
				virtualmachine.LabelPair{Key: "zone-pair", Value: "z"},
				virtualmachine.LabelPair{Key: "host-pair", Value: "h"},
			))
		})
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
	It("joins the key and value with a colon", func() {
		Expect(virtualmachine.TagName("app", "nginx")).To(Equal("app:nginx"))
	})

	It("handles an empty value", func() {
		Expect(virtualmachine.TagName("app", "")).To(Equal("app:"))
	})

	It("handles a prefixed key", func() {
		Expect(virtualmachine.TagName("example.com/app", "nginx")).To(
			Equal("example.com/app:nginx"))
	})
})
