// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"VirtualMachineGroup",
	Label(testlabels.VCSim),
	Label(testlabels.Group), func() {

		var (
			parentCtx   context.Context
			initObjects []client.Object
			testConfig  builder.VCSimTestConfig
			ctx         *builder.TestContextForVCSim
			vmProvider  providers.VirtualMachineProviderInterface
			nsInfo      builder.WorkloadNamespaceInfo

			vm1     *vmopv1.VirtualMachine
			vm2     *vmopv1.VirtualMachine
			vmClass *vmopv1.VirtualMachineClass
			vmGroup *vmopv1.VirtualMachineGroup
		)

		BeforeEach(func() {
			parentCtx = newVMTestParentContext()
			testConfig = newVMTestConfig()

			vm1 = builder.DummyBasicVirtualMachine("group-placement-vm-1", "")
			vm2 = builder.DummyBasicVirtualMachine("group-placement-vm-2", "")
			vmClass = builder.DummyVirtualMachineClassGenName()

			vmGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm-group-test",
				},
				Spec: vmopv1.VirtualMachineGroupSpec{
					BootOrder: make([]vmopv1.VirtualMachineGroupBootOrderGroup, 1),
				},
			}
			vmGroup.Spec.BootOrder[0].Members = append(vmGroup.Spec.BootOrder[0].Members,
				vmopv1.GroupMember{Kind: "VirtualMachine", Name: vm1.Name},
				vmopv1.GroupMember{Kind: "VirtualMachine", Name: vm2.Name})
		})

		JustBeforeEach(func() {
			ctx, vmProvider, nsInfo = newVMTestContext(parentCtx, testConfig, initObjects...)
			ctx.MarkImageCacheReady(ctx.ContentLibraryItem1Cache)

			vmClass.Namespace = nsInfo.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
			Expect(ctx.Client.Status().Update(ctx, vmClass)).To(Succeed())

			vmGroup.Namespace = nsInfo.Namespace

			initVM := func(vm *vmopv1.VirtualMachine) {
				vm.Namespace = nsInfo.Namespace
				vm.Spec.ClassName = vmClass.Name
				vm.Spec.ImageName = ctx.ContentLibraryItem1Name
				vm.Spec.Image.Kind = cvmiKind
				vm.Spec.Image.Name = ctx.ContentLibraryItem1Name
				vm.Spec.StorageClass = ctx.StorageClassName
				vm.Spec.GroupName = vmGroup.Name
			}
			initVM(vm1)
			initVM(vm2)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			initObjects = nil
			vmProvider = nil
			nsInfo = builder.WorkloadNamespaceInfo{}

			vm1 = nil
			vm2 = nil
			vmClass = nil
			vmGroup = nil
		})

		assertMemberStatusForVM := func(vm *vmopv1.VirtualMachine, ms vmopv1.VirtualMachineGroupMemberStatus) {
			GinkgoHelper()

			Expect(ms.Name).To(Equal(vm.Name), "Unexpected Name")
			Expect(ms.Kind).To(Equal("VirtualMachine"), "Unexpected Kind")
			Expect(ms.Placement).ToNot(BeNil(), "Missing Placement")
			Expect(pkgcond.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue(), "No placement ready condition")
			Expect(ms.Placement.Zone).ToNot(BeEmpty(), "Missing Placement Zone")
			Expect(ms.Placement.Pool).ToNot(BeEmpty(), "Missing Placement Pool")
			Expect(ms.Placement.Node).ToNot(BeEmpty(), "Missing Placement Node")
			if pkgcfg.FromContext(ctx).Features.FastDeploy {
				Expect(ms.Placement.Datastores).ToNot(BeEmpty(), "Missing Placement Datastores")
				// Verify against VirtualMachineImageCache.Status
			} else {
				Expect(ms.Placement.Datastores).To(BeEmpty(), "Has Placement Datastores")
			}
		}

		assertNotReadyMemberStatusForVM := func(
			vm *vmopv1.VirtualMachine,
			ms vmopv1.VirtualMachineGroupMemberStatus,
			reason string) {

			GinkgoHelper()

			Expect(ms.Name).To(Equal(vm.Name), "Unexpected Name")
			Expect(ms.Kind).To(Equal("VirtualMachine"), "Unexpected Kind")
			Expect(ms.Placement).To(BeNil(), "Has Placement")

			c := pkgcond.Get(ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
			Expect(c).ToNot(BeNil(), "Condition missing")
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(reason))
		}

		Context("Group placement with VMs specifying affinity policies", func() {
			It("should process preferred VM affinity policies during group placement", func() {
				// Add preferred affinity policy to vm1
				vm1.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app": "database",
									},
								},
								TopologyKey: "topology.kubernetes.io/zone",
							},
						},
					},
				}

				groupPlacements := []providers.VMGroupPlacement{
					{
						VMGroup: vmGroup,
						VMMembers: []*vmopv1.VirtualMachine{
							vm1,
							vm2,
						},
					},
				}

				err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmGroup.Status.Members).To(HaveLen(2))
				assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
				assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
			})

			It("should process required VM affinity policies during group placement", func() {
				// Add required affinity policy to vm1
				vm1.Spec.Affinity = &vmopv1.AffinitySpec{
					VMAffinity: &vmopv1.VMAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"tier": "frontend",
									},
								},
								TopologyKey: "topology.kubernetes.io/zone",
							},
						},
					},
				}

				groupPlacements := []providers.VMGroupPlacement{
					{
						VMGroup: vmGroup,
						VMMembers: []*vmopv1.VirtualMachine{
							vm1,
							vm2,
						},
					},
				}

				err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmGroup.Status.Members).To(HaveLen(2))
				assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
				assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
			})
		})

		Context("Group placement with VMs specifying a preferred zone", func() {
			placeGroup := func() error {
				GinkgoHelper()

				groupPlacements := []providers.VMGroupPlacement{
					{
						VMGroup: vmGroup,
						VMMembers: []*vmopv1.VirtualMachine{
							vm1,
							vm2,
						},
					},
				}

				return vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
			}

			When("VMHardAffinityDuringExecution is disabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMHardAffinityDuringExecution = false
					})
				})

				It("should constrain placement to the shared zone when all members agree", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					zoneName := ctx.ZoneNames[0]
					vm1.Labels[corev1.LabelTopologyZone] = zoneName
					vm2.Labels[corev1.LabelTopologyZone] = zoneName

					Expect(placeGroup()).To(Succeed())
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
					assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])

					Expect(vmGroup.Status.Members[0].Placement.Zone).To(Equal(zoneName))
					Expect(vmGroup.Status.Members[1].Placement.Zone).To(Equal(zoneName))
				})

				It("should not constrain placement to a single zone when members disagree", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]
					vm2.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[1]

					Expect(placeGroup()).To(Succeed())
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
					assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
				})

				It("should not constrain placement to a single zone when one member has no zone label", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]

					Expect(placeGroup()).To(Succeed())
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
					assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
				})
			})

			When("VMHardAffinityDuringExecution is enabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMHardAffinityDuringExecution = true
					})
				})

				// vcsim ignores CandidateVsphereZone, so when members do not share a
				// zone the recommended zone is not deterministic. Placement must
				// either honor each pinned member's zone or fail with a mismatch.
				assertPerVMZonePlacement := func(err error, pinnedZones ...string) {
					GinkgoHelper()

					if err != nil {
						Expect(err).To(MatchError(placement.ErrGroupPlacementZoneMismatch))
						return
					}
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					for i, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
						assertMemberStatusForVM(vm, vmGroup.Status.Members[i])
						if pinnedZones[i] != "" {
							Expect(vmGroup.Status.Members[i].Placement.Zone).To(Equal(pinnedZones[i]))
						}
					}
				}

				It("should constrain placement to the shared zone when all members agree", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					zoneName := ctx.ZoneNames[0]
					vm1.Labels[corev1.LabelTopologyZone] = zoneName
					vm2.Labels[corev1.LabelTopologyZone] = zoneName

					Expect(placeGroup()).To(Succeed())
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
					assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])

					Expect(vmGroup.Status.Members[0].Placement.Zone).To(Equal(zoneName))
					Expect(vmGroup.Status.Members[1].Placement.Zone).To(Equal(zoneName))
				})

				It("should place each member in its own zone when members are in different zones", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]
					vm2.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[1]

					assertPerVMZonePlacement(placeGroup(), ctx.ZoneNames[0], ctx.ZoneNames[1])
				})

				It("should place the pinned member in its zone when one member has no zone label", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]

					assertPerVMZonePlacement(placeGroup(), ctx.ZoneNames[0], "")
				})

				It("should fail when a member's zone is not a placement candidate", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = "zone-does-not-exist"
					vm2.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]

					err := placeGroup()
					Expect(err).To(MatchError(placement.ErrNoPlacementCandidates))
					Expect(err).To(MatchError(ContainSubstring("zone-does-not-exist")))

					Expect(vmGroup.Status.Members).To(HaveLen(2))
					for i := range vmGroup.Status.Members {
						c := pkgcond.Get(&vmGroup.Status.Members[i], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal("PendingPlacement"))
						Expect(c.Message).To(ContainSubstring("zone-does-not-exist"))
					}
				})
			})
		})

		Context("VSpherePolicies is enabled", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})

			It("should process VM with PolicyEval during group placement", func() {
				groupPlacements := []providers.VMGroupPlacement{
					{
						VMGroup: vmGroup,
						VMMembers: []*vmopv1.VirtualMachine{
							vm1,
							vm2,
						},
					},
				}

				err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).To(HaveOccurred())

				Expect(vmGroup.Status.Members).To(HaveLen(2))
				assertNotReadyMemberStatusForVM(vm1, vmGroup.Status.Members[0], "NotReady")
				assertNotReadyMemberStatusForVM(vm2, vmGroup.Status.Members[1], "NotReady")

				markPolicyEvalReady := func(vm *vmopv1.VirtualMachine) {
					policyEval := &vspherepolv1.PolicyEvaluation{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: vm.Namespace,
						Name:      "vm-" + vm.Name},
						policyEval)).To(Succeed())
					policyEval.Status.ObservedGeneration = policyEval.Generation
					pkgcond.MarkTrue(policyEval, vspherepolv1.ReadyConditionType)
					Expect(ctx.Client.Status().Update(ctx, policyEval)).To(Succeed())
				}

				markPolicyEvalReady(vm1)
				err = vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(vsphere.ErrVMGroupPlacementConfigSpec))

				Expect(vmGroup.Status.Members).To(HaveLen(2))
				assertNotReadyMemberStatusForVM(vm1, vmGroup.Status.Members[0], "PendingPlacement")
				assertNotReadyMemberStatusForVM(vm2, vmGroup.Status.Members[1], "NotReady")

				markPolicyEvalReady(vm2)
				err = vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).ToNot(HaveOccurred())

				Expect(vmGroup.Status.Members).To(HaveLen(2))
				assertMemberStatusForVM(vm1, vmGroup.Status.Members[0])
				assertMemberStatusForVM(vm2, vmGroup.Status.Members[1])
			})
		})
	})
