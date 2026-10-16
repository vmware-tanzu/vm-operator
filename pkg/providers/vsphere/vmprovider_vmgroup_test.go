// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

type placementFaultFolder struct {
	mo.Folder
	faults []vimtypes.PlaceVmsXClusterResultPlacementFaults
}

type placementPoolFolder struct {
	simulator.Folder
	poolsByVM map[string]vimtypes.ManagedObjectReference
	request   atomic.Pointer[vimtypes.PlaceVmsXClusterSpec]
	calls     atomic.Int32
}

// PlaceVmsXCluster returns recommendations from the explicitly configured pools.
func (f *placementPoolFolder) PlaceVmsXCluster(
	ctx *simulator.Context,
	req *vimtypes.PlaceVmsXCluster) soap.HasFault {

	f.request.Store(&req.PlacementSpec)
	f.calls.Add(1)
	response := &methods.PlaceVmsXClusterBody{Res: &vimtypes.PlaceVmsXClusterResponse{}}
	for _, spec := range req.PlacementSpec.VmPlacementSpecs {
		vmRequest := *req
		vmRequest.PlacementSpec.ResourcePools = []vimtypes.ManagedObjectReference{f.poolsByVM[spec.ConfigSpec.Name]}
		vmRequest.PlacementSpec.VmPlacementSpecs = []vimtypes.PlaceVmsXClusterSpecVmPlacementSpec{spec}
		vmResponse := f.Folder.PlaceVmsXCluster(ctx, &vmRequest).(*methods.PlaceVmsXClusterBody)
		if vmResponse.Fault() != nil {
			return vmResponse
		}
		response.Res.Returnval.PlacementInfos = append(response.Res.Returnval.PlacementInfos, vmResponse.Res.Returnval.PlacementInfos...)
		response.Res.Returnval.Faults = append(response.Res.Returnval.Faults, vmResponse.Res.Returnval.Faults...)
	}
	return response
}

// PlaceVmsXCluster returns the injected per-VM placement faults.
func (f *placementFaultFolder) PlaceVmsXCluster(
	_ *simulator.Context,
	_ *vimtypes.PlaceVmsXCluster) soap.HasFault {

	return &methods.PlaceVmsXClusterBody{
		Res: &vimtypes.PlaceVmsXClusterResponse{
			Returnval: vimtypes.PlaceVmsXClusterResult{Faults: f.faults},
		},
	}
}

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
			setPlacementPools := func(vm1Zone, vm2Zone string) *placementPoolFolder {
				GinkgoHelper()
				pools := map[string]vimtypes.ManagedObjectReference{
					vm1.Name: ctx.GetResourcePoolForNamespace(vm1.Namespace, vm1Zone, "").Reference(),
					vm2.Name: ctx.GetResourcePoolForNamespace(vm2.Namespace, vm2Zone, "").Reference(),
				}
				folder := &placementPoolFolder{
					Folder:    *ctx.SimulatorContext().Map.Get(ctx.VCClient.Client.ServiceContent.RootFolder).(*simulator.Folder),
					poolsByVM: pools,
				}
				ctx.SimulatorContext().Map.Handler = func(
					simCtx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PlaceVmsXCluster" {
						simCtx.Session.Put(folder)
						return folder, nil
					}
					return nil, nil
				}
				return folder
			}

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

			It("should keep per-VM DRS faults separate across root and child group members", func() {
				faults := []vimtypes.PlaceVmsXClusterResultPlacementFaults{
					{
						VmName:       vm2.Name,
						ResourcePool: vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: "resgroup-1"},
						Faults: []vimtypes.LocalizedMethodFault{{
							Fault:            &vimtypes.NoCompatibleHost{},
							LocalizedMessage: "candidate zone is incompatible",
						}},
					},
					{
						VmName:       vm2.Name,
						ResourcePool: vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: "resgroup-2"},
						Faults: []vimtypes.LocalizedMethodFault{{
							Fault:            &vimtypes.NoCompatibleHost{},
							LocalizedMessage: "no compatible host in second pool",
						}},
					},
				}
				ctx.SimulatorContext().Map.Handler = func(
					simCtx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PlaceVmsXCluster" {
						folder := &placementFaultFolder{faults: faults}
						folder.Self = m.This
						// Session objects take precedence over the handler's return value.
						simCtx.Session.Put(folder)
						return folder, nil
					}
					return nil, nil
				}

				childGroup := vmGroup.DeepCopy()
				childGroup.Name = "child-group"
				err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, []providers.VMGroupPlacement{
					{VMGroup: vmGroup, VMMembers: []*vmopv1.VirtualMachine{vm1}},
					{VMGroup: childGroup, VMMembers: []*vmopv1.VirtualMachine{vm2}},
				})
				Expect(err).To(HaveOccurred())
				var placementErr *placement.PlacementFaultsError
				Expect(errors.As(err, &placementErr)).To(BeTrue())
				Expect(placementErr.Faults).To(HaveLen(2))
				Expect(err.Error()).To(ContainSubstring("candidate zone is incompatible"))
				Expect(err.Error()).To(ContainSubstring("no compatible host in second pool"))

				Expect(vmGroup.Status.Members).To(HaveLen(1))
				assertNotReadyMemberStatusForVM(vm1, vmGroup.Status.Members[0], "GroupPlacementFailed")
				c := pkgcond.Get(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
				Expect(c.Message).To(Equal("group placement failed because another member could not be placed"))

				Expect(childGroup.Status.Members).To(HaveLen(1))
				assertNotReadyMemberStatusForVM(vm2, childGroup.Status.Members[0], "PlacementFailed")
				c = pkgcond.Get(&childGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
				Expect(c.Message).To(Equal(err.Error()))
			})

			It("should report 50 failed VMs separately in a group of 500 members", func() {
				childGroup := vmGroup.DeepCopy()
				childGroup.Name = "child-group"
				groupPlacements := []providers.VMGroupPlacement{{VMGroup: vmGroup}, {VMGroup: childGroup}}
				var faults []vimtypes.PlaceVmsXClusterResultPlacementFaults
				for i := range 500 {
					vm := vm1.DeepCopy()
					vm.Name = fmt.Sprintf("member-%03d", i)
					groupIndex := i % 2
					vm.Spec.GroupName = groupPlacements[groupIndex].VMGroup.Name
					groupPlacements[groupIndex].VMMembers = append(groupPlacements[groupIndex].VMMembers, vm)
					if i%20 < 2 {
						faults = append(faults, vimtypes.PlaceVmsXClusterResultPlacementFaults{
							VmName:       vm.Name,
							ResourcePool: vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: "resgroup-1"},
							Faults: []vimtypes.LocalizedMethodFault{{
								LocalizedMessage: "no compatible host",
								Fault: &vimtypes.NoCompatibleHost{Error: []vimtypes.LocalizedMethodFault{
									{LocalizedMessage: "no compatible host: "},
								}},
							}},
						})
					}
				}
				ctx.SimulatorContext().Map.Handler = func(
					simCtx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PlaceVmsXCluster" {
						folder := &placementFaultFolder{faults: faults}
						folder.Self = m.This
						simCtx.Session.Put(folder)
						return folder, nil
					}
					return nil, nil
				}
				err := vmProvider.PlaceVirtualMachineGroup(ctx, vmGroup, groupPlacements)
				Expect(err).To(HaveOccurred())
				var placementErr *placement.PlacementFaultsError
				Expect(errors.As(err, &placementErr)).To(BeTrue())
				Expect(placementErr.Faults).To(HaveLen(50))
				Expect(strings.Count(err.Error(), "no compatible host")).To(Equal(50))
				var failed, blocked int
				for _, gp := range groupPlacements {
					Expect(gp.VMGroup.Status.Members).To(HaveLen(250))
					for i, member := range gp.VMGroup.Status.Members {
						Expect(member.Placement).To(BeNil())
						condition := pkgcond.Get(&member, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
						Expect(condition).ToNot(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						if vmErr := placementErr.ErrorForVM(gp.VMMembers[i].Name); vmErr != nil {
							failed++
							Expect(condition.Reason).To(Equal("PlacementFailed"))
							Expect(condition.Message).To(Equal(vmErr.Error()))
							Expect(strings.Count(condition.Message, "no compatible host")).To(Equal(1))
						} else {
							blocked++
							Expect(condition.Reason).To(Equal("GroupPlacementFailed"))
							Expect(condition.Message).To(Equal("group placement failed because another member could not be placed"))
						}
					}
				}
				Expect(failed).To(Equal(50))
				Expect(blocked).To(Equal(450))
			})

			It("should record PlacementFailed and preserve unexpected placement errors", func() {
				ctx.SimulatorContext().Map.Handler = func(
					_ *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "PlaceVmsXCluster" {
						return nil, &vimtypes.InvalidRequest{}
					}
					return nil, nil
				}

				err := placeGroup()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("InvalidRequest"))
				Expect(vmGroup.Status.Members).To(HaveLen(2))
				for i, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
					assertNotReadyMemberStatusForVM(vm, vmGroup.Status.Members[i], "PlacementFailed")
					c := pkgcond.Get(&vmGroup.Status.Members[i], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
					Expect(c.Message).To(Equal(err.Error()))
				}
			})

			DescribeTable("unpinned VKS workers use eligible group zones",
				func(deleteAllZones bool) {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMAffinityDuringExecution = true
						config.Features.VMHardAffinityDuringExecution = true
					})
					for _, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
						vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = "worker"
						delete(vm.Labels, corev1.LabelTopologyZone)
					}
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">=", 2))
					for i, zoneName := range ctx.ZoneNames {
						if !deleteAllZones && i != 0 {
							continue
						}
						zone := &topologyv1.Zone{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: vm1.Namespace, Name: zoneName}, zone)).To(Succeed())
						zone.Finalizers = append(zone.Finalizers, "test")
						Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
						Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					}
					if !deleteAllZones {
						setPlacementPools(ctx.ZoneNames[1], ctx.ZoneNames[1])
					}
					err := placeGroup()
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					if deleteAllZones {
						Expect(err).To(MatchError(placement.ErrNoPlacementCandidates))
						for i, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
							assertNotReadyMemberStatusForVM(vm, vmGroup.Status.Members[i], "NoPlacementCandidates")
						}
					} else {
						Expect(err).ToNot(HaveOccurred())
						for _, member := range vmGroup.Status.Members {
							Expect(member.Placement).ToNot(BeNil())
							Expect(member.Placement.Zone).To(Equal(ctx.ZoneNames[1]))
						}
					}
				},
				Entry("places workers in an active zone", false),
				Entry("rejects placement when all zones are deleting", true),
			)

			DescribeTable("pinned zone eligibility",
				func(deleting, hasPools, perVMZones bool, peerZone string) {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMHardAffinityDuringExecution = perVMZones
					})
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">=", 2))
					zoneName := ctx.ZoneNames[0]
					zone := &topologyv1.Zone{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: vm1.Namespace, Name: zoneName}, zone)).To(Succeed())
					zone.Finalizers = []string{"test"}
					if !hasPools {
						zone.Spec.ManagedVMs.PoolMoIDs = nil
					}
					Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
					if deleting {
						Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
						Expect(zone.DeletionTimestamp.IsZero()).To(BeFalse())
						if hasPools {
							Expect(zone.Spec.ManagedVMs.PoolMoIDs).ToNot(BeEmpty())
						}
					}
					vm1.Labels[corev1.LabelTopologyZone] = zoneName
					switch peerZone {
					case "shared":
						vm2.Labels[corev1.LabelTopologyZone] = zoneName
					case "different":
						vm2.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[1]
					case "unpinned":
						delete(vm2.Labels, corev1.LabelTopologyZone)
					}

					if hasPools && !deleting && perVMZones {
						peerPlacementZone := ctx.ZoneNames[1]
						if peerZone == "shared" {
							peerPlacementZone = zoneName
						}
						setPlacementPools(zoneName, peerPlacementZone)
					}
					err := placeGroup()
					if (!hasPools || deleting) && (perVMZones || peerZone == "shared") {
						Expect(err).To(MatchError(placement.ErrNoPlacementCandidates))
						Expect(vmGroup.Status.Members).To(HaveLen(2))
						for i, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
							assertNotReadyMemberStatusForVM(vm, vmGroup.Status.Members[i], "NoPlacementCandidates")
							c := pkgcond.Get(&vmGroup.Status.Members[i], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
							Expect(c.Message).To(Equal(err.Error()))
							if perVMZones {
								Expect(c.Message).To(ContainSubstring(zoneName))
							}
						}
					} else {
						Expect(err).ToNot(HaveOccurred())
						if deleting || !hasPools {
							for _, member := range vmGroup.Status.Members {
								Expect(member.Placement).ToNot(BeNil())
								Expect(member.Placement.Zone).ToNot(Equal(zoneName))
							}
						}
					}
				},
				Entry("shared deleting zone is rejected", true, true, true, "shared"),
				Entry("deleting pinned zone is rejected with a different peer zone", true, true, true, "different"),
				Entry("deleting pinned zone is rejected with an unpinned peer", true, true, true, "unpinned"),
				Entry("active shared zone remains eligible", false, true, true, "shared"),
				Entry("active pinned zone remains eligible with a different peer", false, true, true, "different"),
				Entry("active pinned zone remains eligible with an unpinned peer", false, true, true, "unpinned"),
				Entry("legacy unpinned peer excludes deleting zones", true, true, false, "unpinned"),
				Entry("shared zone without pools is rejected", false, false, true, "shared"),
				Entry("different peer does not make a zone without pools eligible", false, false, true, "different"),
				Entry("unpinned peer does not make a zone without pools eligible", false, false, true, "unpinned"),
				Entry("deleting zone without pools is rejected", true, false, true, "different"),
				Entry("legacy shared deleting zone is rejected", true, true, false, "shared"),
				Entry("legacy mixed zones exclude deleting zones", true, true, false, "different"),
			)

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

				assertPerVMZonePlacement := func(err error, folder *placementPoolFolder, pinnedZones ...string) {
					GinkgoHelper()
					Expect(err).ToNot(HaveOccurred())
					Expect(folder.calls.Load()).To(Equal(int32(1)))
					request := folder.request.Load()
					Expect(request).ToNot(BeNil())
					Expect(request.VmPlacementSpecs).To(HaveLen(2))
					for i, spec := range request.VmPlacementSpecs {
						Expect(spec.CandidateVsphereZone).To(Equal(pinnedZones[i]))
					}
					var poolIDs []string
					for _, pool := range request.ResourcePools {
						poolIDs = append(poolIDs, pool.Value)
					}
					Expect(slices.IsSorted(poolIDs)).To(BeTrue(), "candidate pools must have stable ordering")
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

					folder := setPlacementPools(ctx.ZoneNames[0], ctx.ZoneNames[1])
					assertPerVMZonePlacement(placeGroup(), folder, ctx.ZoneNames[0], ctx.ZoneNames[1])
				})

				It("should place the pinned member in its zone when one member has no zone label", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]

					folder := setPlacementPools(ctx.ZoneNames[0], ctx.ZoneNames[1])
					assertPerVMZonePlacement(placeGroup(), folder, ctx.ZoneNames[0], "")
				})

				It("should reject a deterministic recommendation outside a member's pinned zone", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					vm1.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[0]
					vm2.Labels[corev1.LabelTopologyZone] = ctx.ZoneNames[1]
					setPlacementPools(ctx.ZoneNames[1], ctx.ZoneNames[0])

					err := placeGroup()
					Expect(err).To(MatchError(placement.ErrGroupPlacementZoneMismatch))
					Expect(vmGroup.Status.Members).To(HaveLen(2))
					for i, vm := range []*vmopv1.VirtualMachine{vm1, vm2} {
						assertNotReadyMemberStatusForVM(vm, vmGroup.Status.Members[i], "ZoneMismatch")
						c := pkgcond.Get(&vmGroup.Status.Members[i], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
						Expect(c.Message).To(Equal(err.Error()))
					}
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
						Expect(c.Reason).To(Equal("NoPlacementCandidates"))
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
