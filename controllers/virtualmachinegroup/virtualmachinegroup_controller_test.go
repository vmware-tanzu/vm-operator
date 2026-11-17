// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinegroup"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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

var _ = Describe("Child placement status patches", Label(testlabels.Controller, testlabels.API), func() {
	DescribeTable("should skip unchanged status while preserving placement errors",
		func(updateStatus, alreadyPlaced bool) {
			rootGroup := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "root-group", Namespace: "test-ns", Finalizers: []string{finalizer},
				},
				Spec: vmopv1.VirtualMachineGroupSpec{
					BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{{
						Members: []vmopv1.GroupMember{{Name: "child-group", Kind: virtualMachineGroupKind}},
					}},
				},
			}
			vm := builder.DummyBasicVirtualMachine("child-vm", rootGroup.Namespace)
			vm.UID = types.UID("child-vm-uid")
			vm.Spec.GroupName = "child-group"
			if alreadyPlaced {
				vm.Status.UniqueID = "existing-vm"
			}
			childGroup := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "child-group", Namespace: rootGroup.Namespace},
				Spec: vmopv1.VirtualMachineGroupSpec{
					GroupName: rootGroup.Name,
					BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{{
						Members: []vmopv1.GroupMember{{Name: vm.Name, Kind: virtualMachineKind}},
					}},
				},
				Status: vmopv1.VirtualMachineGroupStatus{
					Members: []vmopv1.VirtualMachineGroupMemberStatus{{
						Name: vm.Name, Kind: virtualMachineKind, UID: vm.UID,
					}},
				},
			}
			conditions.MarkTrue(&childGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			conditions.MarkFalse(&childGroup.Status.Members[0],
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
				vmopv1.VirtualMachineGroupMemberPendingPlacementReason, "previous placement failure")

			var childStatusPatches, placementCalls int
			ctx := suite.NewUnitTestContextForControllerWithFuncs(interceptor.Funcs{
				SubResourcePatch: func(
					ctx context.Context, c client.Client, subResource string, obj client.Object,
					patch client.Patch, opts ...client.SubResourcePatchOption) error {

					if subResource == "status" && obj.GetName() == childGroup.Name {
						childStatusPatches++
					}
					return c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
				},
			}, rootGroup, childGroup, vm)
			DeferCleanup(ctx.AfterEach)
			placementErr := errors.New("group placement failed")
			vmProvider := providerfake.NewVMProvider()
			vmProvider.PlaceVirtualMachineGroupFn = func(
				_ context.Context, _ *vmopv1.VirtualMachineGroup,
				placements []providers.VMGroupPlacement) error {

				placementCalls++
				if updateStatus {
					for _, placement := range placements {
						for i := range placement.VMGroup.Status.Members {
							conditions.MarkError(&placement.VMGroup.Status.Members[i],
								vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
								vmopv1.VirtualMachineGroupMemberPendingPlacementReason, placementErr)
						}
					}
				}
				return placementErr
			}
			reconciler := virtualmachinegroup.NewReconciler(
				ctx, ctx.Client, ctx.Client, ctx.Logger, ctx.Recorder, vmProvider)

			By("reconciling twice to verify placement and child status writes")
			for range 2 {
				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(rootGroup)})
				if alreadyPlaced {
					Expect(err).ToNot(HaveOccurred())
					continue
				}
				Expect(err).To(MatchError(ContainSubstring(placementErr.Error())))
				Expect(errors.Is(err, placementErr)).To(BeTrue())
			}
			if alreadyPlaced {
				Expect(placementCalls).To(BeZero())
			} else {
				Expect(placementCalls).To(Equal(2))
			}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(childGroup), childGroup)).To(Succeed())
			condition := conditions.Get(&childGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
			Expect(condition).ToNot(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("PendingPlacement"))
			if updateStatus {
				Expect(childStatusPatches).To(Equal(1))
				Expect(condition.Message).To(Equal(placementErr.Error()))
			} else {
				Expect(childStatusPatches).To(BeZero())
				Expect(condition.Message).To(Equal("previous placement failure"))
			}
		},
		Entry("provider fails without changing member status", false, false),
		Entry("provider updates member status only on the first attempt", true, false),
		Entry("already placed members skip provider and child status patches", false, true),
	)
})

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

			groupPlacementCallCount atomic.Int32
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

		setProviderPlaceVirtualMachineGroupFn := func() {
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.PlaceVirtualMachineGroupFn = func(
				ctx context.Context,
				group *vmopv1.VirtualMachineGroup,
				groupPlacement []providers.VMGroupPlacement) error {

				groupPlacementCallCount.Add(1)

				// This simulates what the provider method would do by setting
				// placement ready on all the VM members.
				for _, grpPlacement := range groupPlacement {
					for _, vm := range grpPlacement.VMMembers {
						found := false
						for i := range grpPlacement.VMGroup.Status.Members {
							ms := &grpPlacement.VMGroup.Status.Members[i]
							if ms.Name == vm.Name && ms.Kind == virtualMachineKind {
								conditions.MarkTrue(ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
								found = true
								break
							}
						}

						if !found {
							ms := vmopv1.VirtualMachineGroupMemberStatus{
								Name: vm.Name,
								Kind: virtualMachineKind,
								UID:  vm.UID,
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

			groupPlacementCallCount.Store(0)
		})

		AfterEach(func() {
			By("deleting groups and waiting for their finalizers to be removed")
			// Namespace deletion does not garbage collect resources in envtest.
			// Remove groups before resetting the provider so placement retries
			// cannot reach the next test's callback.
			Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineGroup{},
				client.InNamespace(ctx.Namespace))).To(Succeed())
			Eventually(func(g Gomega) {
				var groups vmopv1.VirtualMachineGroupList
				g.Expect(ctx.Client.List(ctx, &groups, client.InNamespace(ctx.Namespace))).To(Succeed())
				g.Expect(groups.Items).To(BeEmpty())
			}, "30s", "100ms").Should(Succeed())

			By("deleting member VMs")
			Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachine{},
				client.InNamespace(ctx.Namespace))).To(Succeed())
			Eventually(func(g Gomega) {
				var vms vmopv1.VirtualMachineList
				g.Expect(ctx.Client.List(ctx, &vms, client.InNamespace(ctx.Namespace))).To(Succeed())
				g.Expect(vms.Items).To(BeEmpty())
			}, "30s", "100ms").Should(Succeed())

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
				setVMUniqueID = func(
					vmKey types.NamespacedName,
					uniqueID, zone string) {

					GinkgoHelper()
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					vmCopy := vm.DeepCopy()
					vmCopy.Status.UniqueID = uniqueID
					vmCopy.Status.Zone = zone
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

				setVMCAPILabel = func(vmKey types.NamespacedName) {
					GinkgoHelper()
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vmKey, vm)).To(Succeed())
					vmCopy := vm.DeepCopy()
					if vmCopy.Labels == nil {
						vmCopy.Labels = make(map[string]string)
					}
					vmCopy.Labels[kubeutil.CAPWClusterRoleLabelKey] = "worker"
					Expect(ctx.Client.Patch(ctx, vmCopy, client.MergeFrom(vm))).To(Succeed())
				}

				verifyGroupPlacementReadyCondition = func(
					groupKey types.NamespacedName,
					memberCount int,
					reason string) {

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

				verifyChildPlacementNotPersisted = func() {
					GinkgoHelper()
					Consistently(func(g Gomega) {
						group := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, group)).To(Succeed())
						g.Expect(group.Status.Members).To(HaveLen(1))
						g.Expect(conditions.Get(&group.Status.Members[0],
							vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeNil())
					}, "1s", "100ms").Should(Succeed())
				}

				verifyGroupPlacementReadyConditionWithZone = func(
					groupKey, vmKey types.NamespacedName,
					zone string) {

					GinkgoHelper()
					Eventually(func(g Gomega) {
						group := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, groupKey, group)).To(Succeed())
						for _, ms := range group.Status.Members {
							if ms.Kind == virtualMachineKind {
								if ms.Name == vmKey.Name {
									c := conditions.Get(
										&ms,
										vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
									g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
									g.Expect(c.Message).To(Equal(zone))
								}

							}
						}
					}, "30s", "100ms").Should(Succeed())
				}
			)

			BeforeEach(func() {
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
					setVMUniqueID(vm1Key, "vm-unique-id-1", "zone1")
					setVMUniqueID(vm2Key, "vm-unique-id-2", "zone2")
					setVMUniqueID(vm3Key, "vm-unique-id-3", "zone3")
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
					verifyChildPlacementNotPersisted()
					verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm1Key, "zone1")
					verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm3Key, "zone3")
					Expect(groupPlacementCallCount.Load()).To(BeZero(), "VM with uniqueID should not be placed by group")
				})
			})

			When("VM member status has PlacementReady condition true and matching UID", func() {
				JustBeforeEach(func() {
					By("waiting for initial group placement")
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")

					By("resetting group placement call count after initial placement")
					groupPlacementCallCount.Store(0)
				})

				It("should skip placing VM by group and mark PlacementReady true", func() {
					By("triggering reconciliation of groups")
					reconcileVMG(vmGroup1Key)
					reconcileVMG(vmGroup2Key)

					By("verifying group placement call count remains 0")
					Consistently(func(g Gomega) {
						g.Expect(groupPlacementCallCount.Load()).To(BeZero())
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

				It("should skip group placement and set PlacementReady after VM is placed", func() {
					By("verifying provider was not called for group placement")
					Expect(groupPlacementCallCount.Load()).To(BeZero(),
						"VMs with zone label override should not be placed by group")

					By("group status should be populated")
					Eventually(func(g Gomega) {
						group := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, group)).To(Succeed())
						g.Expect(group.Status.Members).To(HaveLen(3))
					}, "2s", "100ms").Should(Succeed())

					By("verifying PlacementReady is not set on member status")
					Consistently(func(g Gomega) {
						group := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, group)).To(Succeed())
						g.Expect(group.Status.Members).To(HaveLen(3))
						for _, ms := range group.Status.Members {
							if ms.Kind == virtualMachineKind {
								g.Expect(conditions.Get(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeNil())
							}
						}
					}, "3s").Should(Succeed())

					By("simulating VMs being placed by setting status.uniqueID")
					setVMUniqueID(vm1Key, "vm-unique-id-1", "zone-a")
					setVMUniqueID(vm2Key, "vm-unique-id-2", "zone-b")
					setVMUniqueID(vm3Key, "vm-unique-id-3", "zone-c")

					By("triggering reconciliation of groups")
					reconcileVMG(vmGroup1Key)
					reconcileVMG(vmGroup2Key)

					By("verifying group placement call count remains 0")
					Expect(groupPlacementCallCount.Load()).To(BeZero())

					By("verifying PlacementReady is now true with AlreadyPlaced reason")
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "AlreadyPlaced")
					verifyChildPlacementNotPersisted()

					By("verifying zone is preserved in PlacementReady condition message")
					verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm1Key, "zone-a")
					verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm3Key, "zone-c")
				})
			})

			When("VM needs placement by group", func() {
				It("should place the VM by provider", func() {
					verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
					verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")
					Expect(groupPlacementCallCount.Load()).To(BeEquivalentTo(1), "VM should be placed by group")
				})
			})

			When("group placement fails", func() {
				BeforeEach(func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.PlaceVirtualMachineGroupFn = func(
						_ context.Context,
						_ *vmopv1.VirtualMachineGroup,
						groupPlacements []providers.VMGroupPlacement) error {

						placementErr := errors.New("group placement failed")
						for _, placement := range groupPlacements {
							for i := range placement.VMGroup.Status.Members {
								ms := &placement.VMGroup.Status.Members[i]
								if ms.Kind == virtualMachineKind {
									conditions.MarkError(ms,
										vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
										vmopv1.VirtualMachineGroupMemberPendingPlacementReason, placementErr)
								}
							}
						}
						return placementErr
					}
					intgFakeVMProvider.Unlock()
				})

				It("should persist placement errors on root and child group VM members", func() {
					Eventually(func(g Gomega) {
						for groupKey, memberCount := range map[types.NamespacedName]int{
							vmGroup1Key: 3,
							vmGroup2Key: 1,
						} {
							group := &vmopv1.VirtualMachineGroup{}
							g.Expect(ctx.Client.Get(ctx, groupKey, group)).To(Succeed())
							g.Expect(group.Status.Members).To(HaveLen(memberCount))
							for _, ms := range group.Status.Members {
								if ms.Kind == virtualMachineKind {
									c := conditions.Get(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
									g.Expect(c).ToNot(BeNil())
									g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
									g.Expect(c.Reason).To(Equal("PendingPlacement"))
									g.Expect(c.Message).To(Equal("group placement failed"))
								}
							}
						}
					}, "30s", "100ms").Should(Succeed())
				})
			})

			When("VMAffinityDuringExecution is enabled", func() {
				var oldVMAffinityDuringExecution bool

				BeforeEach(func() {
					oldVMAffinityDuringExecution = pkgcfg.FromContext(suite.Context).Features.VMAffinityDuringExecution
					pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
						config.Features.VMAffinityDuringExecution = true
					})
				})

				AfterEach(func() {
					pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
						config.Features.VMAffinityDuringExecution = oldVMAffinityDuringExecution
					})
				})

				When("VM has a zone label and is a VKS node", func() {
					BeforeEach(func() {
						By("setting zone and CAPI labels on VMs before adding to group")
						setVMZoneLabel(vm1Key, "zone-a")
						setVMZoneLabel(vm2Key, "zone-b")
						setVMZoneLabel(vm3Key, "zone-c")
						setVMCAPILabel(vm1Key)
						setVMCAPILabel(vm2Key)
						setVMCAPILabel(vm3Key)
					})

					It("should skip group placement to respect zone override", func() {
						By("verifying provider was not called for group placement")
						Consistently(func(g Gomega) {
							g.Expect(groupPlacementCallCount.Load()).To(BeZero(),
								"VKS nodes with zone label should not be placed by group")
						}, "3s", "100ms").Should(Succeed())

						By("verifying PlacementReady is not set on member status")
						Consistently(func(g Gomega) {
							group := &vmopv1.VirtualMachineGroup{}
							g.Expect(ctx.Client.Get(ctx, vmGroup1Key, group)).To(Succeed())
							g.Expect(group.Status.Members).To(HaveLen(3))
							for _, ms := range group.Status.Members {
								if ms.Kind == virtualMachineKind {
									g.Expect(conditions.Get(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeNil())
								}
							}
						}, "3s").Should(Succeed())

						By("recording the VKS zone contract after ordinary VM placement")
						setVMUniqueID(vm1Key, "vm-unique-id-1", "zone-a")
						setVMUniqueID(vm2Key, "vm-unique-id-2", "zone-b")
						setVMUniqueID(vm3Key, "vm-unique-id-3", "zone-c")
						reconcileVMG(vmGroup1Key)
						reconcileVMG(vmGroup2Key)
						verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "AlreadyPlaced")
						verifyChildPlacementNotPersisted()
						verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm1Key, "zone-a")
						verifyGroupPlacementReadyConditionWithZone(vmGroup1Key, vm3Key, "zone-c")
						Expect(groupPlacementCallCount.Load()).To(BeZero())
					})
				})

				When("VM has a zone label and is not a VKS node", func() {
					BeforeEach(func() {
						By("setting zone label (without CAPI labels) on VMs before adding to group")
						setVMZoneLabel(vm1Key, "zone-a")
						setVMZoneLabel(vm2Key, "zone-b")
						setVMZoneLabel(vm3Key, "zone-c")
					})

					It("should place the VM by group", func() {
						verifyGroupPlacementReadyCondition(vmGroup1Key, 3, "")
						verifyGroupPlacementReadyCondition(vmGroup2Key, 1, "")
						Expect(groupPlacementCallCount.Load()).To(BeEquivalentTo(1),
							"non-VKS zone-labeled VM should be placed by group")
					})
				})
			})
		})

		Context("PowerState", func() {
			var updateGroupPowerStateTime time.Time

			getVM := func(g Gomega, key types.NamespacedName) *vmopv1.VirtualMachine {
				vm := &vmopv1.VirtualMachine{}
				g.Expect(ctx.Client.Get(ctx, key, vm)).To(Succeed())
				return vm
			}

			getVMG := func(g Gomega, key types.NamespacedName) *vmopv1.VirtualMachineGroup {
				vmg := &vmopv1.VirtualMachineGroup{}
				g.Expect(ctx.Client.Get(ctx, key, vmg)).To(Succeed())
				return vmg
			}

			setVMStatusPowerState := func(key types.NamespacedName, ps vmopv1.VirtualMachinePowerState) {
				GinkgoHelper()
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, key, vm)).To(Succeed())
				vmCopy := vm.DeepCopy()
				vmCopy.Status.PowerState = ps
				Expect(ctx.Client.Status().Patch(ctx, vmCopy, client.MergeFrom(vm))).To(Succeed())
			}

			memberPowerStateReason := func(
				g Gomega,
				groupKey types.NamespacedName,
				kind, name string) string {

				for _, ms := range getVMG(g, groupKey).Status.Members {
					if ms.Kind == kind && ms.Name == name {
						return conditions.GetReason(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)
					}
				}
				return ""
			}

			vmPowerStateReason := func(
				g Gomega,
				groupKey, vmKey types.NamespacedName) string {

				return memberPowerStateReason(g, groupKey, virtualMachineKind, vmKey.Name)
			}

			applyPowerStateTime := func(g Gomega, obj client.Object) time.Time {
				v := obj.GetAnnotations()[constants.ApplyPowerStateTimeAnnotation]
				g.Expect(v).ToNot(BeEmpty())
				t, err := time.Parse(time.RFC3339Nano, v)
				g.Expect(err).ToNot(HaveOccurred())
				return t
			}

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
					// A power-on request is closed only when every member at
					// any depth is observed powered on.
					setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
					setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)

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

			Context("power off the group", func() {
				var (
					powerOffMode vmopv1.VirtualMachinePowerOpMode
					bootOrder0   *metav1.Duration
					bootOrder1   *metav1.Duration
				)

				// expectVM1Queued asserts vm-1 (boot order index 0) is not
				// released while vmgroup-2 (boot order index 1) powers off.
				expectVM1Queued := func(g Gomega) {
					g.Expect(getVM(g, vm1Key).Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOff))
					g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Queued"))
					g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
				}

				BeforeEach(func() {
					powerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
					bootOrder0 = nil
					bootOrder1 = nil
				})

				JustBeforeEach(func() {
					setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
					setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					vmGroup1Copy.Spec.PowerOffMode = powerOffMode
					vmGroup1Copy.Spec.BootOrder[0].PowerOffDelay = bootOrder0
					vmGroup1Copy.Spec.BootOrder[1].PowerOffDelay = bootOrder1
					// Mimic mutating webhook to set last updated power state time annotation.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should release one boot order group at a time in reverse order", func() {
					By("vmgroup-2 (boot order index 1) is released first", func() {
						Eventually(func(g Gomega) {
							vmGroup2 := getVMG(g, vmGroup2Key)
							g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
							g.Expect(vmGroup2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))

							vm2 := getVM(g, vm2Key)
							g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
							g.Expect(vm2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))
							g.Expect(vmPowerStateReason(g, vmGroup2Key, vm2Key)).To(Equal("Pending"))

							expectVM1Queued(g)
						}, "5s", "100ms").Should(Succeed())
					})

					By("vm-1 (boot order index 0) waits while vm-2 is on", func() {
						Consistently(expectVM1Queued, "2s", "100ms").Should(Succeed())
					})

					By("vm-1 is released when vm-2 is powered off", func() {
						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)
						Eventually(func(g Gomega) {
							vm1 := getVM(g, vm1Key)
							g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
							g.Expect(vm1.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))
							g.Expect(vm1.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Pending"))
							g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
						}, "5s", "100ms").Should(Succeed())
					})

					By("the request is closed when vm-1 is powered off", func() {
						setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOff)
						Eventually(func(g Gomega) {
							vmGroup1 := getVMG(g, vmGroup1Key)
							g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
							g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))
							// A synced member has the condition True.
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal(string(metav1.ConditionTrue)))
						}, "5s", "100ms").Should(Succeed())
					})
				})

				It("should power off a child group again when one of its members is powered on after it powered off", func() {
					By("vmgroup-2 and vm-1 power off", func() {
						Eventually(expectVM1Queued, "5s", "100ms").Should(Succeed())
						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)
						Eventually(func(g Gomega) {
							g.Expect(getVM(g, vm1Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
							g.Expect(getVMG(g, vmGroup2Key).Status.LastUpdatedPowerStateTime).ToNot(BeNil())
						}, "5s", "100ms").Should(Succeed())
					})

					By("a user powers on vm-2 after vmgroup-2 closed its request", func() {
						vm2 := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						vm2Copy := vm2.DeepCopy()
						vm2Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						Expect(ctx.Client.Patch(ctx, vm2Copy, client.MergeFrom(vm2))).To(Succeed())
						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)
					})

					By("vmgroup-2 powers off vm-2 again, and vm-1 is not shown as queued", func() {
						Eventually(func(g Gomega) {
							g.Expect(getVM(g, vm2Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))

							var child *vmopv1.VirtualMachineGroupMemberStatus
							for _, ms := range getVMG(g, vmGroup1Key).Status.Members {
								if ms.Kind == virtualMachineGroupKind {
									child = &ms
								}
							}
							g.Expect(child).ToNot(BeNil())
							g.Expect(conditions.GetReason(child, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("Pending"))
							g.Expect(conditions.GetMessage(child, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(ContainSubstring(vm2Key.Name))

							// vm-1 was already released, so it is not queued.
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Pending"))
							g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
						}, "5s", "100ms").Should(Succeed())
					})
				})

				When("the members of the group form a loop", func() {
					BeforeEach(func() {
						// vmgroup-1 is listed first so that the completion walk
						// reaches it before vm-2.
						setupGroupWithMembers(vmGroup2Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineGroupKind, Name: vmGroup1Key.Name},
								},
							},
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineKind, Name: vm2Key.Name},
								},
							},
						}, vmGroup1Key.Name)

						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						vmGroup1Copy := vmGroup1.DeepCopy()
						vmGroup1Copy.Spec.GroupName = vmGroup2Key.Name
						Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
					})

					It("should still reconcile the members without releasing them", func() {
						Eventually(func(g Gomega) {
							vmGroup1 := getVMG(g, vmGroup1Key)
							g.Expect(vmGroup1.Status.Members).To(HaveLen(2))
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Queued"))
							g.Expect(getVMG(g, vmGroup2Key).Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOff))
							g.Expect(conditions.IsFalse(vmGroup1, vmopv1.ReadyConditionType)).To(BeTrue())
						}, "5s", "100ms").Should(Succeed())
					})
				})

				When("boot order groups have a power off delay", func() {
					const (
						bootOrder0Delay = 1 * time.Minute
						bootOrder1Delay = 2 * time.Minute
					)

					BeforeEach(func() {
						bootOrder0 = &metav1.Duration{Duration: bootOrder0Delay}
						bootOrder1 = &metav1.Duration{Duration: bootOrder1Delay}
					})

					It("should wait for the delay after the previous boot order group powers off", func() {
						By("vmgroup-2 is released at once since it is the first to power off", func() {
							Eventually(func(g Gomega) {
								vmGroup2 := getVMG(g, vmGroup2Key)
								g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
								g.Expect(vmGroup2.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))

								vm2 := getVM(g, vm2Key)
								g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
								g.Expect(vm2.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))

								expectVM1Queued(g)
							}, "5s", "100ms").Should(Succeed())
						})

						var released time.Time
						By("vm-1 is released with its delay counted from vm-2 powering off", func() {
							released = time.Now()
							setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)
							Eventually(func(g Gomega) {
								vm1 := getVM(g, vm1Key)
								g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
								g.Expect(applyPowerStateTime(g, vm1)).To(BeTemporally("~", released.Add(bootOrder0Delay), 5*time.Second))
							}, "5s", "100ms").Should(Succeed())
						})

						By("the delay does not move on later reconciles", func() {
							vm1 := &vmopv1.VirtualMachine{}
							Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
							stamp := vm1.Annotations[constants.ApplyPowerStateTimeAnnotation]

							reconcileVMG(vmGroup1Key)
							Consistently(func(g Gomega) {
								g.Expect(getVM(g, vm1Key).Annotations).To(HaveKeyWithValue(constants.ApplyPowerStateTimeAnnotation, stamp))
								g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
							}, "2s", "100ms").Should(Succeed())
						})

						By("vm-1 is released again with a new delay when a user powers it on", func() {
							vm1 := &vmopv1.VirtualMachine{}
							Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
							oldStamp := applyPowerStateTime(Default, vm1)

							vm1Copy := vm1.DeepCopy()
							vm1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
							Expect(ctx.Client.Patch(ctx, vm1Copy, client.MergeFrom(vm1))).To(Succeed())

							Eventually(func(g Gomega) {
								vm1 := getVM(g, vm1Key)
								g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
								g.Expect(applyPowerStateTime(g, vm1)).To(BeTemporally(">", oldStamp))
							}, "5s", "100ms").Should(Succeed())
						})
					})
				})

				When("power off mode is soft and a member is suspended", func() {
					BeforeEach(func() {
						powerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
					})

					It("should count the suspended member as powered off", func() {
						Eventually(expectVM1Queued, "5s", "100ms").Should(Succeed())

						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateSuspended)
						Eventually(func(g Gomega) {
							g.Expect(getVM(g, vm1Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						}, "5s", "100ms").Should(Succeed())
					})
				})

				When("power off mode is try soft and a member is suspended", func() {
					It("should wait for the suspended member to power off", func() {
						Eventually(expectVM1Queued, "5s", "100ms").Should(Succeed())

						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateSuspended)
						reconcileVMG(vmGroup1Key)
						Consistently(expectVM1Queued, "2s", "100ms").Should(Succeed())
					})
				})

				When("a member of the boot order group powering off is missing", func() {
					BeforeEach(func() {
						setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineKind, Name: vm1Key.Name},
								},
							},
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
									{Kind: virtualMachineKind, Name: "vm-missing"},
								},
							},
						})
					})

					It("should not release the next boot order group", func() {
						Eventually(expectVM1Queued, "5s", "100ms").Should(Succeed())

						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)
						reconcileVMG(vmGroup1Key)
						Consistently(expectVM1Queued, "2s", "100ms").Should(Succeed())
					})
				})
			})

			When("a child group waits for the apply power state time set by its parent", func() {
				BeforeEach(func() {
					vm2 := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
					vm2Copy := vm2.DeepCopy()
					vm2Copy.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(ctx.Client.Status().Patch(ctx, vm2Copy, client.MergeFrom(vm2))).To(Succeed())

					// Mimic the parent group releasing this group with a delay.
					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					vmGroup2Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
						constants.ApplyPowerStateTimeAnnotation:       time.Now().Add(time.Hour).Format(time.RFC3339Nano),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())
				})

				It("should not release its members until that time", func() {
					expectQueued := func(g Gomega) {
						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOff))

						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.Status.Members).To(HaveLen(1))
						g.Expect(conditions.GetReason(&vmGroup2.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("Queued"))
					}
					Eventually(expectQueued, "5s", "100ms").Should(Succeed())
					Consistently(expectQueued, "2s", "100ms").Should(Succeed())

					By("releasing its members once the time has passed")
					vmGroup2 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
					vmGroup2Copy := vmGroup2.DeepCopy()
					vmGroup2Copy.Annotations[constants.ApplyPowerStateTimeAnnotation] = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
					Expect(ctx.Client.Patch(ctx, vmGroup2Copy, client.MergeFrom(vmGroup2))).To(Succeed())

					Eventually(func(g Gomega) {
						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					}, "5s", "100ms").Should(Succeed())
				})
			})

			Context("power on the group", func() {
				var (
					bootOrder0 *metav1.Duration
					bootOrder1 *metav1.Duration
				)

				// expectVMGroup2Queued asserts vmgroup-2 (boot order index 1)
				// is not released while vm-1 (boot order index 0) powers on.
				expectVMGroup2Queued := func(g Gomega) {
					g.Expect(getVMG(g, vmGroup2Key).Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOn))
					g.Expect(getVM(g, vm2Key).Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOn))
					g.Expect(memberPowerStateReason(g, vmGroup1Key, virtualMachineGroupKind, vmGroup2Key.Name)).To(Equal("Queued"))
					g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
				}

				BeforeEach(func() {
					bootOrder0 = nil
					bootOrder1 = nil
				})

				JustBeforeEach(func() {
					setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOff)
					setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)

					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					vmGroup1Copy := vmGroup1.DeepCopy()
					vmGroup1Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					vmGroup1Copy.Spec.BootOrder[0].PowerOnDelay = bootOrder0
					vmGroup1Copy.Spec.BootOrder[1].PowerOnDelay = bootOrder1
					// Mimic mutating webhook to set last updated power state time annotation.
					vmGroup1Copy.Annotations = map[string]string{
						constants.LastUpdatedPowerStateTimeAnnotation: updateGroupPowerStateTime.Format(time.RFC3339),
					}
					Expect(ctx.Client.Patch(ctx, vmGroup1Copy, client.MergeFrom(vmGroup1))).To(Succeed())
				})

				It("should release one boot order group at a time in order", func() {
					By("vm-1 (boot order index 0) is released first", func() {
						Eventually(func(g Gomega) {
							vm1 := getVM(g, vm1Key)
							g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							g.Expect(vm1.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Pending"))

							expectVMGroup2Queued(g)
						}, "5s", "100ms").Should(Succeed())
					})

					By("vmgroup-2 (boot order index 1) waits while vm-1 is off", func() {
						Consistently(expectVMGroup2Queued, "2s", "100ms").Should(Succeed())
					})

					By("vmgroup-2 is released when vm-1 is powered on", func() {
						setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
						Eventually(func(g Gomega) {
							vmGroup2 := getVMG(g, vmGroup2Key)
							g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							g.Expect(vmGroup2.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
							g.Expect(getVM(g, vm2Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							g.Expect(memberPowerStateReason(g, vmGroup1Key, virtualMachineGroupKind, vmGroup2Key.Name)).To(Equal("Pending"))
							g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
						}, "5s", "100ms").Should(Succeed())
					})

					By("the request is closed when vm-2 is powered on", func() {
						setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)
						Eventually(func(g Gomega) {
							vmGroup1 := getVMG(g, vmGroup1Key)
							g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime).ToNot(BeNil())
							g.Expect(vmGroup1.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">", updateGroupPowerStateTime))
							// A synced member has the condition True.
							g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal(string(metav1.ConditionTrue)))
						}, "5s", "100ms").Should(Succeed())
					})
				})

				When("the child group is the first boot order group", func() {
					BeforeEach(func() {
						setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
								},
							},
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineKind, Name: vm1Key.Name},
								},
							},
						})
					})

					It("should power on a child group again when one of its members is powered off after it powered on", func() {
						By("vmgroup-2 powers on and closes its request, then vm-1 is released", func() {
							Eventually(func(g Gomega) {
								g.Expect(getVM(g, vm2Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							}, "5s", "100ms").Should(Succeed())
							setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)
							Eventually(func(g Gomega) {
								g.Expect(getVMG(g, vmGroup2Key).Status.LastUpdatedPowerStateTime).ToNot(BeNil())
								g.Expect(getVM(g, vm1Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
							}, "5s", "100ms").Should(Succeed())
						})

						By("a user powers off vm-2 after vmgroup-2 closed its request", func() {
							vm2 := &vmopv1.VirtualMachine{}
							Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
							vm2Copy := vm2.DeepCopy()
							vm2Copy.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
							Expect(ctx.Client.Patch(ctx, vm2Copy, client.MergeFrom(vm2))).To(Succeed())
							setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOff)
						})

						By("vmgroup-2 powers on vm-2 again, and vm-1 is not shown as queued", func() {
							Eventually(func(g Gomega) {
								g.Expect(getVM(g, vm2Key).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								g.Expect(memberPowerStateReason(g, vmGroup1Key, virtualMachineGroupKind, vmGroup2Key.Name)).To(Equal("Pending"))

								// vm-1 was already released, so it is not queued.
								g.Expect(vmPowerStateReason(g, vmGroup1Key, vm1Key)).To(Equal("Pending"))
								g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
							}, "5s", "100ms").Should(Succeed())
						})
					})
				})

				When("boot order groups have a power on delay", func() {
					const (
						bootOrder0Delay = 1 * time.Minute
						bootOrder1Delay = 2 * time.Minute
					)

					BeforeEach(func() {
						bootOrder0 = &metav1.Duration{Duration: bootOrder0Delay}
						bootOrder1 = &metav1.Duration{Duration: bootOrder1Delay}
					})

					It("should wait for the delay after the previous boot order group powers on", func() {
						By("vm-1 is released at once since it is the first to power on", func() {
							Eventually(func(g Gomega) {
								vm1 := getVM(g, vm1Key)
								g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								g.Expect(vm1.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))

								expectVMGroup2Queued(g)
							}, "5s", "100ms").Should(Succeed())
						})

						var released time.Time
						By("vmgroup-2 is released with its delay counted from vm-1 powering on", func() {
							released = time.Now()
							setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
							Eventually(func(g Gomega) {
								vmGroup2 := getVMG(g, vmGroup2Key)
								g.Expect(vmGroup2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								g.Expect(applyPowerStateTime(g, vmGroup2)).To(BeTemporally("~", released.Add(bootOrder1Delay), 5*time.Second))
							}, "5s", "100ms").Should(Succeed())
						})

						By("vmgroup-2 does not release vm-2 until the delay has passed", func() {
							Consistently(func(g Gomega) {
								g.Expect(getVM(g, vm2Key).Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOn))
								g.Expect(vmPowerStateReason(g, vmGroup2Key, vm2Key)).To(Equal("Queued"))
							}, "2s", "100ms").Should(Succeed())
						})

						By("the delay does not move on later reconciles", func() {
							vmGroup2 := &vmopv1.VirtualMachineGroup{}
							Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
							stamp := vmGroup2.Annotations[constants.ApplyPowerStateTimeAnnotation]

							reconcileVMG(vmGroup1Key)
							Consistently(func(g Gomega) {
								g.Expect(getVMG(g, vmGroup2Key).Annotations).To(HaveKeyWithValue(constants.ApplyPowerStateTimeAnnotation, stamp))
								g.Expect(getVMG(g, vmGroup1Key).Status.LastUpdatedPowerStateTime).To(BeNil())
							}, "2s", "100ms").Should(Succeed())
						})
					})
				})

				When("a member of the boot order group powering on is missing", func() {
					BeforeEach(func() {
						setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineKind, Name: vm1Key.Name},
									{Kind: virtualMachineKind, Name: "vm-missing"},
								},
							},
							{
								Members: []vmopv1.GroupMember{
									{Kind: virtualMachineGroupKind, Name: vmGroup2Key.Name},
								},
							},
						})
					})

					It("should not release the next boot order group", func() {
						Eventually(expectVMGroup2Queued, "5s", "100ms").Should(Succeed())

						setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
						reconcileVMG(vmGroup1Key)
						Consistently(expectVMGroup2Queued, "2s", "100ms").Should(Succeed())
					})
				})
			})

			When("VM's power state is changed directly outside of group", func() {
				BeforeEach(func() {
					// The VMs are observed on, so the power-on request closes.
					setVMStatusPowerState(vm1Key, vmopv1.VirtualMachinePowerStateOn)
					setVMStatusPowerState(vm2Key, vmopv1.VirtualMachinePowerStateOn)

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

		Context("PolicyEvaluation", func() {
			var policyEvalKey types.NamespacedName

			BeforeEach(func() {
				policyEvalKey = types.NamespacedName{
					Name:      "vm-" + vm1Key.Name,
					Namespace: ctx.Namespace,
				}

				By("setting up group-1 with member vm-1")
				setupGroupWithMembers(vmGroup1Key, []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: virtualMachineKind, Name: vm1Key.Name},
						},
					},
				})

				By("assigning vm-1 to group-1")
				assignVMToGroup(vm1Key, vmGroup1Key.Name)

				vm1Updated := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vm1Key, vm1Updated)).To(Succeed())
				vm1UpdatedCopy := vm1Updated.DeepCopy()
				conditions.MarkTrue(vm1UpdatedCopy, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				Expect(ctx.Client.Status().Patch(ctx, vm1UpdatedCopy, client.MergeFrom(vm1Updated))).To(Succeed())

				By("waiting for vm-1 to be linked to group-1")
				Eventually(func(g Gomega) {
					vmGroup1 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
					g.Expect(vmGroup1.Status.Members).To(HaveLen(1))
					g.Expect(conditions.IsTrue(&vmGroup1.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
				}, "5s", "100ms").Should(Succeed())
			})

			When("PolicyEvaluation owned by a VM member is marked as Ready", func() {
				BeforeEach(func() {
					By("creating a PolicyEvaluation owned by vm-1")
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vm1Key, vm)).To(Succeed())

					policyEval := &vspherepolv1.PolicyEvaluation{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: policyEvalKey.Namespace,
							Name:      policyEvalKey.Name,
						},
					}
					Expect(controllerutil.SetControllerReference(vm, policyEval, ctx.Client.Scheme())).To(Succeed())
					Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
				})

				It("should reconcile the group when PolicyEvaluation is marked as Ready", func() {
					setProviderPlaceVirtualMachineGroupFn()

					By("marking PolicyEvaluation as Ready")
					policyEval := &vspherepolv1.PolicyEvaluation{}
					Expect(ctx.Client.Get(ctx, policyEvalKey, policyEval)).To(Succeed())
					policyEvalCopy := policyEval.DeepCopy()
					policyEvalCopy.Status.ObservedGeneration = policyEval.Generation
					conditions.MarkTrue(policyEvalCopy, vspherepolv1.ReadyConditionType)
					Expect(ctx.Client.Status().Patch(ctx, policyEvalCopy, client.MergeFrom(policyEval))).To(Succeed())

					By("verifying the group was reconciled")
					// The group should be reconciled when the PolicyEvaluation is marked as Ready.
					// We can verify this by checking that the group's status or generation has been updated,
					// or by checking that the group's member status has been updated.
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())

						// The group should have been reconciled, which means it should have updated
						// the member status. Since the VM doesn't have a uniqueID or zone label,
						// it should attempt placement.
						g.Expect(vmGroup1.Status.Members).To(HaveLen(1))
						g.Expect(vmGroup1.Status.Members[0].Name).To(Equal(vm1Key.Name))
						g.Expect(vmGroup1.Status.Members[0].Kind).To(Equal(virtualMachineKind))

						// Verify the member has the expected conditions
						// Callback set by setProviderPlaceVirtualMachineGroupFn() should assign this.
						g.Expect(conditions.IsTrue(&vmGroup1.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)).To(BeTrue())
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

var _ = Describe("Power state requests", Label(testlabels.Controller, testlabels.API), func() {
	const (
		ns        = "test-ns"
		groupName = "root-group"
		vm1Name   = "vm-1"
		vm2Name   = "vm-2"
	)

	var (
		// t0 is the time of the previous request, and t1 of the next request,
		// which the mutating webhook only accepts in a later second.
		t0 = time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(941 * time.Millisecond)
		t1 = t0.Truncate(time.Second).Add(time.Second + 50*time.Millisecond)

		group *vmopv1.VirtualMachineGroup
		vm1   *vmopv1.VirtualMachine
		vm2   *vmopv1.VirtualMachine

		// staleGroup, when set, is returned to the reconciler in place of the
		// group, as from a cache that has not seen the latest updates to the
		// group yet.
		staleGroup *vmopv1.VirtualMachineGroup
		// inReconcile is true while the reconciler runs.
		inReconcile bool
		// failGroupStatusPatch fails the patch of the group's status, as when
		// VM Operator stops after the members were patched.
		failGroupStatusPatch bool

		ctx        *builder.UnitTestContextForController
		reconciler *virtualmachinegroup.Reconciler
	)

	newVM := func(name string, spec, status vmopv1.VirtualMachinePowerState) *vmopv1.VirtualMachine {
		vm := builder.DummyBasicVirtualMachine(name, ns)
		vm.Spec.GroupName = groupName
		vm.Spec.PowerState = spec
		vm.Status.PowerState = status
		vm.Status.UniqueID = "unique-" + name
		return vm
	}

	setRequest := func(g *vmopv1.VirtualMachineGroup, ps vmopv1.VirtualMachinePowerState, t time.Time) {
		g.Spec.PowerState = ps
		g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
		if g.Annotations == nil {
			g.Annotations = map[string]string{}
		}
		g.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = t.Format(time.RFC3339Nano)
	}

	getGroup := func() *vmopv1.VirtualMachineGroup {
		GinkgoHelper()
		g := &vmopv1.VirtualMachineGroup{}
		Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: groupName}, g)).To(Succeed())
		return g
	}

	getVM := func(name string) *vmopv1.VirtualMachine {
		GinkgoHelper()
		vm := &vmopv1.VirtualMachine{}
		Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, vm)).To(Succeed())
		return vm
	}

	setVMPowerState := func(name string, spec, status vmopv1.VirtualMachinePowerState) {
		GinkgoHelper()
		vm := getVM(name)
		vm.Spec.PowerState = spec
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
		vm.Status.PowerState = status
		Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
	}

	// updateGroup applies a change to the group, as a user does.
	updateGroup := func(mutate func(g *vmopv1.VirtualMachineGroup)) {
		GinkgoHelper()
		g := getGroup()
		mutate(g)
		Expect(ctx.Client.Update(ctx, g)).To(Succeed())
	}

	reconcile := func() error {
		inReconcile = true
		defer func() { inReconcile = false }()
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKey{Namespace: ns, Name: groupName},
		})
		return err
	}

	isPending := func() bool {
		GinkgoHelper()
		g := getGroup()
		completed := g.Status.LastUpdatedPowerStateTime
		if completed == nil {
			return true
		}
		t, err := time.Parse(time.RFC3339Nano, g.Annotations[constants.LastUpdatedPowerStateTimeAnnotation])
		Expect(err).ToNot(HaveOccurred())
		return !completed.After(t)
	}

	BeforeEach(func() {
		staleGroup = nil
		failGroupStatusPatch = false

		group = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name: groupName, Namespace: ns, Finalizers: []string{finalizer},
			},
			Spec: vmopv1.VirtualMachineGroupSpec{
				BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
					{Members: []vmopv1.GroupMember{{Name: vm1Name, Kind: virtualMachineKind}}},
					{Members: []vmopv1.GroupMember{{Name: vm2Name, Kind: virtualMachineKind}}},
				},
			},
		}
		setRequest(group, vmopv1.VirtualMachinePowerStateOn, t0)
		// The members are placed, so the reconciler uses the cached group.
		for _, name := range []string{vm1Name, vm2Name} {
			ms := vmopv1.VirtualMachineGroupMemberStatus{Name: name, Kind: virtualMachineKind}
			conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
			group.Status.Members = append(group.Status.Members, ms)
		}
		vm1 = newVM(vm1Name, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOn)
		vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOn)
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForControllerWithFuncs(interceptor.Funcs{
			Get: func(
				ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {

				if g, ok := obj.(*vmopv1.VirtualMachineGroup); ok &&
					inReconcile && staleGroup != nil && key.Name == groupName {
					// Only the first read of a reconcile is stale. A retry
					// after a conflict reads the latest group.
					staleGroup.DeepCopyInto(g)
					staleGroup = nil
					return nil
				}
				return c.Get(ctx, key, obj, opts...)
			},
			SubResourcePatch: func(
				ctx context.Context, c client.Client, subResource string, obj client.Object,
				patch client.Patch, opts ...client.SubResourcePatchOption) error {

				if failGroupStatusPatch && subResource == "status" && obj.GetName() == groupName {
					if _, ok := obj.(*vmopv1.VirtualMachineGroup); ok {
						return errors.New("vm operator stopped")
					}
				}
				return c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
			},
		}, group, vm1, vm2)
		DeferCleanup(ctx.AfterEach)
		reconciler = virtualmachinegroup.NewReconciler(
			ctx, ctx.Client, ctx.Client, ctx.Logger, ctx.Recorder, providerfake.NewVMProvider())
	})

	// completeRequest reconciles until the group's current request completes.
	completeRequest := func() {
		GinkgoHelper()
		Expect(reconcile()).To(Succeed())
		Expect(isPending()).To(BeFalse())
	}

	When("a power-off is requested right after a power-on completes, and a reconcile sees the old group", func() {
		It("should not complete the power-off, and release the members on the next reconcile", func() {
			// The view of the group before the power-on completed.
			beforeCompletion := getGroup()
			completeRequest()
			staleGroup = beforeCompletion

			By("requesting power-off in the same second as the power-on")
			updateGroup(func(g *vmopv1.VirtualMachineGroup) {
				setRequest(g, vmopv1.VirtualMachinePowerStateOff, t1)
			})

			By("reconciling with the old view of the group, which completes the power-on again")
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			Expect(getVM(vm2Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			Expect(isPending()).To(BeTrue(), "the power-off request must stay pending")

			By("reconciling with the latest view of the group")
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm2Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff),
				"the last boot order group is released first for power-off")
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			Expect(isPending()).To(BeTrue())

			By("powering off vm-2 releases vm-1, and powering off vm-1 completes the request")
			setVMPowerState(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			setVMPowerState(vm1Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
			completeRequest()
			Expect(getGroup().Status.LastUpdatedPowerStateTime.Time).To(BeTemporally("==", t1.Truncate(time.Second).Add(time.Second)))
		})
	})

	When("a power-off is requested while a power-on is still releasing boot order groups", func() {
		BeforeEach(func() {
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should power off every member, including one released by a reconcile with the old group", func() {
			staleGroup = getGroup()
			updateGroup(func(g *vmopv1.VirtualMachineGroup) {
				setRequest(g, vmopv1.VirtualMachinePowerStateOff, t1)
			})

			By("reconciling with the old view of the group, which releases vm-2 for power-on")
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm2Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			Expect(isPending()).To(BeTrue())

			By("reconciling with the latest view of the group")
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm2Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			Expect(isPending()).To(BeTrue(), "vm-1 is not powered off yet")
		})
	})

	When("VM Operator stops after releasing a boot order group, before the group status is saved", func() {
		BeforeEach(func() {
			group.Spec.BootOrder[0].PowerOffDelay = &metav1.Duration{Duration: time.Minute}
			setRequest(group, vmopv1.VirtualMachinePowerStateOff, t1)
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should release the boot order group once, and not move its apply power state time", func() {
			failGroupStatusPatch = true
			Expect(reconcile()).ToNot(Succeed())
			released := getVM(vm1Name)
			Expect(released.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			applyTime := released.Annotations[constants.ApplyPowerStateTimeAnnotation]
			Expect(applyTime).ToNot(BeEmpty())

			By("restarting")
			failGroupStatusPatch = false
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Annotations[constants.ApplyPowerStateTimeAnnotation]).To(Equal(applyTime))
			Expect(isPending()).To(BeTrue())
		})
	})

	When("VM Operator stops after every member reached the power state, before the group status is saved", func() {
		BeforeEach(func() {
			setRequest(group, vmopv1.VirtualMachinePowerStateOff, t1)
			vm1 = newVM(vm1Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should complete the request after the restart", func() {
			failGroupStatusPatch = true
			Expect(reconcile()).ToNot(Succeed())
			Expect(isPending()).To(BeTrue())

			By("restarting")
			failGroupStatusPatch = false
			completeRequest()
			Expect(getGroup().Status.LastUpdatedPowerStateTime.Time).To(BeTemporally("==", t1.Truncate(time.Second).Add(time.Second)))
		})
	})

	When("the members show the power state from before the request", func() {
		BeforeEach(func() {
			setRequest(group, vmopv1.VirtualMachinePowerStateOff, t1)
			vm1 = newVM(vm1Name, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOff)
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should release every member and complete the request", func() {
			completeRequest()
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			Expect(getVM(vm2Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
		})
	})

	When("a member is powered on directly after the request completed", func() {
		BeforeEach(func() {
			setRequest(group, vmopv1.VirtualMachinePowerStateOff, t1)
			vm1 = newVM(vm1Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should not power off the member again", func() {
			completeRequest()
			setVMPowerState(vm1Name, vmopv1.VirtualMachinePowerStateOn, vmopv1.VirtualMachinePowerStateOn)
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
		})
	})

	When("the request was completed by an earlier version of VM Operator", func() {
		BeforeEach(func() {
			// Earlier versions recorded the current time at completion.
			setRequest(group, vmopv1.VirtualMachinePowerStateOff, t1)
			group.Status.LastUpdatedPowerStateTime = &metav1.Time{Time: t1.Add(5 * time.Second)}
			vm2 = newVM(vm2Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
		})

		It("should keep the request completed and not apply the power state again", func() {
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			Expect(isPending()).To(BeFalse())
		})
	})

	When("the group has no request time annotation", func() {
		BeforeEach(func() {
			delete(group.Annotations, constants.LastUpdatedPowerStateTimeAnnotation)
		})

		It("should complete the request once", func() {
			Expect(reconcile()).To(Succeed())
			Expect(getGroup().Status.LastUpdatedPowerStateTime).ToNot(BeNil())

			setVMPowerState(vm1Name, vmopv1.VirtualMachinePowerStateOff, vmopv1.VirtualMachinePowerStateOff)
			Expect(reconcile()).To(Succeed())
			Expect(getVM(vm1Name).Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff),
				"the completed request is not applied again")
		})
	})
})

var _ = Describe("Power state of a nested group after the request completed", Label(testlabels.Controller, testlabels.API), func() {
	const (
		ns        = "test-ns"
		rootName  = "root-group"
		childName = "child-group"
		vmName    = "child-vm"
	)

	It("should leave a member powered on directly in the child group alone, and not start a new request", func() {
		requestTime := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

		newGroup := func(name, parent string, member vmopv1.GroupMember) *vmopv1.VirtualMachineGroup {
			g := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: ns, Finalizers: []string{finalizer},
				},
				Spec: vmopv1.VirtualMachineGroupSpec{
					GroupName:    parent,
					PowerState:   vmopv1.VirtualMachinePowerStateOff,
					PowerOffMode: vmopv1.VirtualMachinePowerOpModeHard,
					BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
						{Members: []vmopv1.GroupMember{member}},
					},
				},
			}
			g.Annotations = map[string]string{
				constants.LastUpdatedPowerStateTimeAnnotation: requestTime.Format(time.RFC3339Nano),
			}
			return g
		}

		root := newGroup(rootName, "", vmopv1.GroupMember{Kind: virtualMachineGroupKind, Name: childName})
		child := newGroup(childName, rootName, vmopv1.GroupMember{Kind: virtualMachineKind, Name: vmName})
		// The VM is placed, so the reconciler uses the cached group.
		ms := vmopv1.VirtualMachineGroupMemberStatus{Name: vmName, Kind: virtualMachineKind}
		conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
		conditions.MarkTrue(&ms, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
		child.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{ms}

		vm := builder.DummyBasicVirtualMachine(vmName, ns)
		vm.Spec.GroupName = childName
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
		vm.Status.UniqueID = "unique-" + vmName

		ctx := suite.NewUnitTestContextForController(root, child, vm)
		DeferCleanup(ctx.AfterEach)
		reconciler := virtualmachinegroup.NewReconciler(
			ctx, ctx.Client, ctx.Client, ctx.Logger, ctx.Recorder, providerfake.NewVMProvider())

		reconcile := func(name string) {
			GinkgoHelper()
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{Namespace: ns, Name: name},
			})
			Expect(err).ToNot(HaveOccurred())
		}
		getGroup := func(name string) *vmopv1.VirtualMachineGroup {
			GinkgoHelper()
			g := &vmopv1.VirtualMachineGroup{}
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, g)).To(Succeed())
			return g
		}

		By("completing the request on both groups")
		reconcile(rootName)
		reconcile(childName)
		reconcile(rootName)
		for _, name := range []string{rootName, childName} {
			Expect(getGroup(name).Status.LastUpdatedPowerStateTime).ToNot(BeNil(), name)
		}
		annotation := getGroup(childName).Annotations[constants.LastUpdatedPowerStateTimeAnnotation]

		By("powering on the VM directly")
		got := &vmopv1.VirtualMachine{}
		Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), got)).To(Succeed())
		got.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		Expect(ctx.Client.Update(ctx, got)).To(Succeed())
		got.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		Expect(ctx.Client.Status().Update(ctx, got)).To(Succeed())

		By("reconciling both groups again")
		for range 2 {
			reconcile(childName)
			reconcile(rootName)
		}

		Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), got)).To(Succeed())
		Expect(got.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
		Expect(getGroup(childName).Annotations[constants.LastUpdatedPowerStateTimeAnnotation]).To(Equal(annotation),
			"no new request is started on the child group")
		for _, name := range []string{rootName, childName} {
			g := getGroup(name)
			Expect(g.Status.LastUpdatedPowerStateTime.Time).To(BeTemporally(">=", requestTime), name)
		}
		for _, ms := range getGroup(childName).Status.Members {
			Expect(conditions.GetReason(&ms, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)).To(Equal("NotSynced"))
		}
	})
})
