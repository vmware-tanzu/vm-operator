// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyImage              = "dummy-image"
	dummyClass              = "dummy-class"
	storageClass            = "my-storage-class"
	finalizer               = "vmoperator.vmware.com/virtualmachinegroup"
	virtualMachineKind      = "VirtualMachine"
	virtualMachineGroupKind = "VirtualMachineGroup"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {
		var (
			ctx *builder.IntegrationTestContext

			vm1Key, vm2Key, vm3Key   types.NamespacedName
			vm1, vm2, vm3            *vmopv1.VirtualMachine
			vmGroup1Key, vmGroup2Key types.NamespacedName
		)

		setupGroupWithMembers := func(groupKey types.NamespacedName, members []vmopv1.VirtualMachineGroupBootOrderGroup, parentGroupName ...string) {
			GinkgoHelper()
			group := &vmopv1.VirtualMachineGroup{}
			Expect(ctx.Client.Get(ctx, groupKey, group)).To(Succeed())
			groupCopy := group.DeepCopy()
			groupCopy.Spec.BootOrder = members
			if len(parentGroupName) > 0 {
				groupCopy.Spec.GroupName = parentGroupName[0]
			}
			Expect(ctx.Client.Patch(ctx, groupCopy, client.MergeFrom(group))).To(Succeed())
		}

		assignVMToGroup := func(vmKey types.NamespacedName, groupName string) {
			GinkgoHelper()
			vm := &vmopv1.VirtualMachine{}
			Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
			vmCopy := vm.DeepCopy()
			vmCopy.Spec.GroupName = groupName
			Expect(ctx.Client.Patch(ctx, vmCopy, client.MergeFrom(vm))).To(Succeed())
		}

		reconcileVMG := func(vmgKey types.NamespacedName) {
			GinkgoHelper()
			vmg := &vmopv1.VirtualMachineGroup{}
			Expect(ctx.Client.Get(ctx, vmgKey, vmg)).To(Succeed())

			// Add a random annotation to trigger reconciliation of the group.
			vmgCopy := vmg.DeepCopy()
			if vmgCopy.Annotations == nil {
				vmgCopy.Annotations = map[string]string{}
			}
			vmgCopy.Annotations["test/trigger-reconcile"] = time.Now().Format(time.RFC3339Nano)
			Expect(ctx.Client.Patch(ctx, vmgCopy, client.MergeFrom(vmg))).To(Succeed())
		}

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()
			vmGroup1Key = types.NamespacedName{
				Name:      "vmg-1-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}
			vmGroup2Key = types.NamespacedName{
				Name:      "vmg-2-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}

			vm1Key = types.NamespacedName{
				Name:      "vm-1-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}
			vm2Key = types.NamespacedName{
				Name:      "vm-2-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}

			vm3Key = types.NamespacedName{
				Name:      "vm-3-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}

			// Create VM and VM Group objects used in the tests.
			vmGroup1 := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmGroup1Key.Namespace,
					Name:      vmGroup1Key.Name,
				},
			}
			Expect(ctx.Client.Create(ctx, vmGroup1)).To(Succeed())

			vm1 = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm1Key.Namespace,
					Name:      vm1Key.Name,
					UID:       types.UID(uuid.NewString()),
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm1)).To(Succeed())

			vm2 = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm2Key.Namespace,
					Name:      vm2Key.Name,
					UID:       types.UID(uuid.NewString()),
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm2)).To(Succeed())

			vm3 = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm3Key.Namespace,
					Name:      vm3Key.Name,
					UID:       types.UID(uuid.NewString()),
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm3)).To(Succeed())

			vmGroup2 := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmGroup2Key.Namespace,
					Name:      vmGroup2Key.Name,
				},
			}
			Expect(ctx.Client.Create(ctx, vmGroup2)).To(Succeed())
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			intgFakeVMProvider.Reset()
		})

		Context("Finalizer", func() {
			It("should add finalizer to all VirtualMachineGroup objects", func() {
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(vmGroup1.GetFinalizers()).To(ContainElement(finalizer))

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					g.Expect(vmGroup2.GetFinalizers()).To(ContainElement(finalizer))
					// Using a longer timeout to ensure all VM Groups above are reconciled.
				}, "5s", "100ms").Should(Succeed(), "waiting for VirtualMachineGroup finalizer")
			})
		})

		Context("GroupName", func() {
			When("group name is not set", func() {
				It("should not have the group linked condition", func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeNil())
				})
			})

			When("group name is set to a non-existent group", func() {
				BeforeEach(func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.GroupName = "non-existent-group"
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should set the group linked condition to false with NotFound reason", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
						g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
						g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotFound"))
						g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType)).ToNot(BeNil())
						g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Status).To(Equal(metav1.ConditionFalse))
						g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Reason).To(Equal("Error"))
						g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Message).To(ContainSubstring("group is not linked to its parent group"))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("group name is set to an existing group", func() {
				BeforeEach(func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.GroupName = vmGroup2Key.Name
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				When("the parent group members do not contain this child group", func() {
					It("should set the child group linked condition to false with NotMember reason", func() {
						Eventually(func(g Gomega) {
							vmGroup1 := &vmopv1.VirtualMachineGroup{}
							g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
							g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
							g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
							g.Expect(conditions.Get(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotMember"))
							g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType)).ToNot(BeNil())
							g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Status).To(Equal(metav1.ConditionFalse))
							g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Reason).To(Equal("Error"))
							g.Expect(conditions.Get(vmGroup1, vmopv1.ReadyConditionType).Message).To(ContainSubstring("group is not linked to its parent group"))
						}, "5s", "100ms").Should(Succeed())
					})
				})

				When("the parent group members contain this child group", func() {
					BeforeEach(func() {
						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						vmGroup2Copy := vmGroup2.DeepCopy()
						vmGroup2Copy.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
							{
								Members: []vmopv1.GroupMember{
									{
										Kind: virtualMachineGroupKind,
										Name: vmGroup1Key.Name,
									},
								},
							},
						}
						Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
					})

					It("should set the child group linked condition to true", func() {
						Eventually(func(g Gomega) {
							vmGroup1 := &vmopv1.VirtualMachineGroup{}
							g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
							g.Expect(conditions.IsTrue(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
							g.Expect(conditions.IsTrue(vmGroup1, vmopv1.ReadyConditionType)).To(BeTrue())
						}, "5s", "100ms").Should(Succeed())
					})
				})
			})
		})

		Context("Members", func() {
			JustBeforeEach(func() {
				setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm1Key.Name},
							{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
						},
					},
				})
			})

			When("members are not found", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vm1)).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					Expect(ctx.Client.Delete(ctx, vmGroup2)).To(Succeed())
				})

				It("should mark the group not ready and the member condition as false", func() {
					Eventually(func(g Gomega) {
						// Manually trigger reconcile to ensure the group member status is updated after member deletion.
						reconcileVMG(vmGroup1Key)

						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Conditions[0].Reason).To(Equal("MembersNotReady"))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Reason).To(Equal("NotFound"))
						g.Expect(vmGroup1.Status.Members[1].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Reason).To(Equal("NotFound"))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("members do not have the correct group name", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Spec.GroupName = "vmgroup-invalid"
					Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Spec.GroupName = "vmgroup-invalid"
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
				})

				It("should mark the group not ready and the member condition as false", func() {
					Eventually(func(g Gomega) {
						// Manually trigger reconcile to ensure the group member status is updated after member's spec.groupName update.
						reconcileVMG(vmGroup1Key)

						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Conditions[0].Reason).To(Equal("MembersNotReady"))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[0].Conditions[0].Reason).To(Equal("NotMember"))
						g.Expect(vmGroup1.Status.Members[1].Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Type).To(Equal(vmopv1.VirtualMachineGroupMemberConditionGroupLinked))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Members[1].Conditions[0].Reason).To(Equal("NotMember"))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("members are found and have the correct group name", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Spec.GroupName = vmGroup1Key.Name
					Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					// Update the VM status.uniqueID to simulate the VM already placed (to mark the parent group ready).
					vm1Updated := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1Updated)).To(Succeed())
					vm1StatusCopy := vm1Updated.DeepCopy()
					vm1StatusCopy.Status.UniqueID = "vm-unique-id-1"
					Expect(ctx.Client.Status().Patch(ctx, vm1StatusCopy, client.MergeFrom(vm1Updated))).To(Succeed())

					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Spec.GroupName = vmGroup1Key.Name
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
				})

				It("should mark the group ready, member condition as true, and owner reference set to the group", func() {
					Eventually(func(g Gomega) {
						// No need to manually trigger reconcile of the group because it's requeued
						// when a member is updated with spec.groupName set to that group.

						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))

						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						for _, ms := range vmGroup1.Status.Members {
							g.Expect(conditions.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
							switch ms.Kind {
							case virtualMachineKind:
								g.Expect(conditions.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue())
							case virtualMachineGroupKind:
								g.Expect(conditions.IsTrue(&ms, vmopv1.ReadyConditionType)).To(BeTrue())
							}
						}

						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.OwnerReferences).To(HaveLen(1))
						g.Expect(vm1.OwnerReferences[0].Kind).To(Equal(virtualMachineGroupKind))
						g.Expect(vm1.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.OwnerReferences).To(HaveLen(1))
						g.Expect(vmGroup2.OwnerReferences[0].Kind).To(Equal(virtualMachineGroupKind))
						g.Expect(vmGroup2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))
					}, "5s", "100ms").Should(Succeed())
				})
			})
		})

		Context("Placement", func() {
			var (
				groupPlacementCallCount int

				setVMUniqueID = func(vmKey types.NamespacedName, uniqueID string) {
					GinkgoHelper()
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					vmCopy := vm.DeepCopy()
					vmCopy.Status.UniqueID = uniqueID
					Expect(ctx.Client.Status().Patch(ctx, vmCopy, client.MergeFrom(vm))).To(Succeed())
				}

				setVMZoneLabel = func(vmKey types.NamespacedName, zoneName string) {
					GinkgoHelper()
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					vmCopy := vm.DeepCopy()
					if vmCopy.Labels == nil {
						vmCopy.Labels = make(map[string]string)
					}
					vmCopy.Labels[corev1.LabelTopologyZone] = zoneName
					Expect(ctx.Client.Patch(ctx, vmCopy, client.MergeFrom(vm))).To(Succeed())
				}

				verifyGroupPlacementReadyCondition = func(groupKey types.NamespacedName, memberCount int, reason string) {
					GinkgoHelper()
					Eventually(func(g Gomega) {
						group := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, groupKey, group)).To(Succeed())
						g.Expect(group.Status.Members).To(HaveLen(memberCount))
						for _, ms := range group.Status.Members {
							if ms.Kind == virtualMachineKind {
								g.Expect(conditions.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue())
								if reason != "" {
									g.Expect(conditions.Get(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady).Reason).To(Equal(reason))
								}
							}
						}
					}, "30s", "100ms").Should(Succeed())
				}

				setProviderPlaceVirtualMachineGroupFn = func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.PlaceVirtualMachineGroupFn = func(
						ctx context.Context,
						group *vmopv1.VirtualMachineGroup,
						groupPlacement []providers.VMGroupPlacement) error {

						groupPlacementCallCount++

						for _, grpPlacement := range groupPlacement {
							for _, vm := range grpPlacement.VMMembers {
								found := false
								for i := range grpPlacement.VMGroup.Status.Members {
									ms := &grpPlacement.VMGroup.Status.Members[i]
									if ms.Name == vm.Name && ms.Kind == "VirtualMachine" {
										conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
										found = true
										break
									}
								}

								if !found {
									ms := vmopv1.VirtualMachineGroupMemberStatus{
										Name: vm.Name,
										Kind: "VirtualMachine",
									}
									conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
									grpPlacement.VMGroup.Status.Members = append(grpPlacement.VMGroup.Status.Members, ms)
								}
							}
						}

						return nil
					}
					intgFakeVMProvider.Unlock()
				}
			)

			BeforeEach(func() {
				groupPlacementCallCount = 0
				setProviderPlaceVirtualMachineGroupFn()

				By("setting up group-1 with members vm-1, vm-3, and vmgroup-2")
				setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm1Key.Name},
							{Kind: virtualMachineKind, Name: vm3Key.Name},
						},
					},
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
						},
					},
				})

				By("setting up group-2 with member vm-2")
				setupGroupWithMembers(vmGroup2Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm2Key.Name},
						},
					},
				}, vmGroup1Key.Name)
			})

			JustBeforeEach(func() {
				// Assign VMs to groups in JustBeforeEach() so that we can modify
				// VM objects before triggering reconciliation for group placement.
				By("assigning VMs to groups")
				assignVMToGroup(vm1Key, vmGroup1Key.Name)
				assignVMToGroup(vm3Key, vmGroup1Key.Name)
				assignVMToGroup(vm2Key, vmGroup2Key.Name)

				By("triggering reconciliation of groups")
				reconcileVMG(vmGroup1Key)
				reconcileVMG(vmGroup2Key)
			})

			When("VM already has uniqueID", func() {
				BeforeEach(func() {
					By("setting uniqueID to VMs before adding to group")
					setVMUniqueID(vm1Key, "vm-unique-id-1")
					setVMUniqueID(vm2Key, "vm-unique-id-2")
					setVMUniqueID(vm3Key, "vm-unique-id-3")

					By("verifying VM has uniqueID before adding to group")
					Eventually(func(g Gomega) {
						for _, vmKey := range []types.NamespacedName{vm1Key, vm2Key, vm3Key} {
							vm := &vmopv1.VirtualMachine{}
							g.Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
							g.Expect(vm.Status.UniqueID).ToNot(BeEmpty())
						}
					}, "5s", "100ms").Should(Succeed())
				})

				It("should skip placing VM by group and mark PlacementReady true with AlreadyPlaced reason", func() {
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "AlreadyPlaced")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "AlreadyPlaced")
					Expect(groupPlacementCallCount).To(BeZero(), "VM with uniqueID should not be placed by group")
				})
			})

			When("VM member status has PlacementReady condition true and matching UID", func() {
				JustBeforeEach(func() {
					By("waiting for initial group placement")
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")

					By("resetting group placement call count after initial placement")
					groupPlacementCallCount = 0
				})

				It("should skip placing VM by group and mark PlacementReady true", func() {
					By("triggering reconciliation of groups")
					reconcileVMG(vmGroup1Key)
					reconcileVMG(vmGroup2Key)

					By("verifying group placement call count remains 0")
					Consistently(func(g Gomega) {
						g.Expect(groupPlacementCallCount).To(BeZero())
					}, "2s", "100ms").Should(Succeed())

					By("verifying group placement ready condition remains true")
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")
				})
			})

			When("VM doesn't have PlacementReady nor uniqueID but has zone label", func() {
				BeforeEach(func() {
					By("setting zone label to VMs before adding to group")
					setVMZoneLabel(vm1Key, "zone-a")
					setVMZoneLabel(vm2Key, "zone-b")
					setVMZoneLabel(vm3Key, "zone-c")
				})

				It("should skip placing VM by group and mark PlacementReady true", func() {
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")
					Expect(groupPlacementCallCount).To(BeZero(), "VM with zone label should not be placed by group")
				})
			})

			When("VM needs placement by group", func() {
				It("should place the VM by provider", func() {
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")
					Expect(groupPlacementCallCount).To(BeEquivalentTo(1), "VM should be placed by group")
				})
			})
		})

		Context("PowerState", func() {
			var updateGroupPowerStateTime time.Time

			BeforeEach(func() {
				// Set the time earlier to be able to compare with the status time.
				updateGroupPowerStateTime = time.Now().Add(-5 * time.Minute)

				By("setting up group-1 with members vm-1 and vmgroup-2")
				setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm1Key.Name},
						},
					},
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
						},
					},
				})

				By("setting up group-2 with members vm-2")
				setupGroupWithMembers(vmGroup2Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm2Key.Name},
						},
					},
				}, vmGroup1Key.Name)

				By("setting up group name and group linked condition for vm-1 to enqueue its group reconciliation")
				assignVMToGroup(vm1Key, vmGroup1Key.Name)

				vm1Updated := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm1Key, vm1Updated)).To(Succeed())
				vm1UpdatedCopy := vm1Updated.DeepCopy()
				conditions.MarkTrue(vm1UpdatedCopy, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				Expect(ctx.Client.Status().Patch(ctx, vm1UpdatedCopy, client.MergeFrom(vm1Updated))).To(Succeed())

				By("setting up group name and group linked condition for vm-2 to enqueue its group reconciliation")
				assignVMToGroup(vm2Key, vmGroup2Key.Name)

				vm2Updated := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm2Key, vm2Updated)).To(Succeed())
				vm2UpdatedCopy := vm2Updated.DeepCopy()
				conditions.MarkTrue(vm2UpdatedCopy, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				Expect(ctx.Client.Status().Patch(ctx, vm2UpdatedCopy, client.MergeFrom(vm2Updated))).To(Succeed())
			})

			When("group.Spec.PowerState is empty", func() {
				BeforeEach(func() {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = ""
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should have expected group status", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						g.Expect(vmGroup1.Status.Members[0].PowerState).To(BeNil())
						g.Expect(conditions.Has(&vmGroup1.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeFalse())
						g.Expect(vmGroup1.Status.Members[1].PowerState).To(BeNil())
						g.Expect(conditions.Has(&vmGroup1.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeFalse())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).To(BeNil())
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("group.Spec.PowerState is set and already synced with member", func() {
				BeforeEach(func() {
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(ctx.Client.Status().Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					// Mimic mutating webhook to set last updated power state time annotation.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should have expected group status", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						for _, ms := range vmGroup1.Status.Members {
							if ms.Kind == virtualMachineKind {
								g.Expect(ms.PowerState).To(HaveValue(Equal(vmopv1.VirtualMachinePowerStateOn)))
								g.Expect(conditions.IsTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeTrue())
							} else {
								g.Expect(ms.PowerState).To(BeNil())
								g.Expect(conditions.Has(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeFalse())
							}
						}
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("power off the group with try soft power off mode", func() {
				BeforeEach(func() {
					// Set VM power state status to be different from group.
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(ctx.Client.Status().Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					vmGroup1Copy.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
					// Mimic mutating webhook to set last updated power state time annotation.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should power off all members with try soft power off mode", func() {
					Eventually(func(g Gomega) {
						By("group should have expected member status")
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						for _, ms := range vmGroup1.Status.Members {
							if ms.Kind == virtualMachineKind {
								g.Expect(ms.PowerState).To(HaveValue(Equal(vmopv1.VirtualMachinePowerStateOn)))
								g.Expect(conditions.IsFalse(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeTrue())
								// Reason should always be "Pending" because there's no VM controller to update the status.
								g.Expect(conditions.GetReason(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("Pending"))
							} else {
								g.Expect(ms.PowerState).To(BeNil())
								g.Expect(conditions.Has(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeFalse())
							}
						}

						By("group should have last updated power state time in status")
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))

						By("all group members should have expected power state and power off mode")
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vm1.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vmGroup2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))

						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						g.Expect(vm2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("power on the group with delay", func() {
				const (
					bootOrder1Delay = 1 * time.Minute
					bootOrder2Delay = 2 * time.Minute
				)

				BeforeEach(func() {
					// Set VM power state status to be different from group.
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
					Expect(ctx.Client.Status().Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					vmGroup1Copy.Spec.BootOrder[0].PowerOnDelay = &metav1.Duration{Duration: bootOrder1Delay}
					vmGroup1Copy.Spec.BootOrder[1].PowerOnDelay = &metav1.Duration{Duration: bootOrder2Delay}
					// Mimic mutating webhook to set last updated power state time annotation.
					// This is used to verify the power on delay is applied correctly.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should power on all members with the expected delay", func() {
					Eventually(func(g Gomega) {
						By("group should have expected member status")
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
						for _, ms := range vmGroup1.Status.Members {
							if ms.Kind == virtualMachineKind {
								g.Expect(ms.PowerState).To(HaveValue(Equal(vmopv1.VirtualMachinePowerStateOff)))
								g.Expect(conditions.IsFalse(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeTrue())
								// Reason should always be "Pending" because there's no VM controller to update the status.
								g.Expect(conditions.GetReason(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("Pending"))
							} else {
								g.Expect(ms.PowerState).To(BeNil())
								g.Expect(conditions.Has(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeFalse())
							}
						}

						By("group should have last updated power state time in status")
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))

						By("group members CRs should have expected changes")
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						g.Expect(vm1.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vm1ApplyPowerStateTime, err := time.Parse(time.RFC3339, vm1.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						g.Expect(vm1ApplyPowerStateTime).To(BeTemporally("~", updateGroupPowerStateTime.Add(bootOrder1Delay), 5*time.Second))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vmGroup2ApplyPowerStateTime, err := time.Parse(time.RFC3339, vmGroup2.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						// Boot order delay is cumulative.
						bootOrder2DelayCum := bootOrder1Delay + bootOrder2Delay
						g.Expect(vmGroup2ApplyPowerStateTime).To(BeTemporally("~", updateGroupPowerStateTime.Add(bootOrder2DelayCum), 5*time.Second))

						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						g.Expect(vm2.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
						vm2ApplyPowerStateTime, err := time.Parse(time.RFC3339, vm2.Annotations[constants.ApplyPowerStateTimeAnnotation])
						g.Expect(err).To(Not(HaveOccurred()))
						g.Expect(vm2ApplyPowerStateTime).To(BeTemporally("~", updateGroupPowerStateTime.Add(bootOrder2DelayCum), 5*time.Second))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			When("VM's power state is changed directly outside of group", func() {
				BeforeEach(func() {
					// First set up the group with power state on.
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					// Mimic mutating webhook to set last updated power state time annotation.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())

					// Wait for the group to be reconciled and status updated.
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
						g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))
					}, "5s", "100ms").Should(Succeed())

					// Set VM power state status to match group initially.
					vm1 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy := vm1.DeepCopy()
					vm1Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(ctx.Client.Status().Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

					// Now change VM's power state directly outside of group.
					vm1 = &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
					vm1Copy = vm1.DeepCopy()
					vm1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())
					vm1Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
					Expect(ctx.Client.Status().Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())
				})

				It("should detect power state mismatch without overwriting VM spec", func() {
					Eventually(func(g Gomega) {
						By("group should have expected member status")
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(vmGroup1.Status.Members).To(HaveLen(2))

						var vm1Member *vmopv1.VirtualMachineGroupMemberStatus
						for _, ms := range vmGroup1.Status.Members {
							if ms.Kind == virtualMachineKind && ms.Name == vm1Key.Name {
								vm1Member = &ms
								break
							}
						}
						g.Expect(vm1Member).ToNot(BeNil(), "VM member should be found in status")
						g.Expect(vm1Member.PowerState).To(HaveValue(Equal(vmopv1.VirtualMachinePowerStateOff)))
						g.Expect(conditions.IsFalse(vm1Member, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(BeTrue())
						g.Expect(conditions.GetReason(vm1Member, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("NotSynced"))

						By("VM spec should not be overwritten by group")
						g.Expect(vmGroup1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))

						By("group Ready condition should be False due to member not synced")
						g.Expect(vmGroup1.Status.Conditions).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Conditions[0].Type).To(Equal(vmopv1.ReadyConditionType))
						g.Expect(vmGroup1.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						g.Expect(vmGroup1.Status.Conditions[0].Reason).To(Equal("MembersNotReady"))
					}, "5s", "100ms").Should(Succeed())
				})
			})
		})

		Context("Deletion", func() {
			BeforeEach(func() {
				// Use Eventually to retry the delete operation in case of conflicts
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(ctx.Client.Delete(ctx, vmGroup1)).To(Succeed())
				}).Should(Succeed(), "deleting vmGroup1 should succeed after retries")
			})

			// Please note it is not possible to validate garbage collection with
			// the fake client or with envtest, because neither of them implement
			// the Kubernetes garbage collector.
			It("should remove finalizer and delete the object", func() {
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					err := ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
