// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
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

			vm1Key, vm2Key, vm3Key                types.NamespacedName
			vmGroup1Key, vmGroup2Key, vmGroup3Key types.NamespacedName
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()

			vmGroup1Key = types.NamespacedName{
				Name:      "vmgroup-1",
				Namespace: ctx.Namespace,
			}
			vmGroup2Key = types.NamespacedName{
				Name:      "vmgroup-2",
				Namespace: ctx.Namespace,
			}
			vmGroup3Key = types.NamespacedName{
				Name:      "vmgroup-3",
				Namespace: ctx.Namespace,
			}
			vm1Key = types.NamespacedName{
				Name:      "vm-1",
				Namespace: ctx.Namespace,
			}
			vm2Key = types.NamespacedName{
				Name:      "vm-2",
				Namespace: ctx.Namespace,
			}
			vm3Key = types.NamespacedName{
				Name:      "vm-3",
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

			vm1 := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm1Key.Namespace,
					Name:      vm1Key.Name,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm1)).To(Succeed())

			vm2 := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm2Key.Namespace,
					Name:      vm2Key.Name,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:    dummyImage,
					ClassName:    dummyClass,
					StorageClass: storageClass,
				},
			}
			Expect(ctx.Client.Create(ctx, vm2)).To(Succeed())

			vm3 := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm3Key.Namespace,
					Name:      vm3Key.Name,
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

			vmGroup3 := &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vmGroup3Key.Namespace,
					Name:      vmGroup3Key.Name,
				},
			}
			Expect(ctx.Client.Create(ctx, vmGroup3)).To(Succeed())
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
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

					vmGroup3 := &vmopv1.VirtualMachineGroup{}
					g.Expect(ctx.Client.Get(ctx, vmGroup3Key, vmGroup3)).To(Succeed())
					g.Expect(vmGroup3.GetFinalizers()).To(ContainElement(finalizer))
				}).Should(Succeed(), "waiting for VirtualMachineGroup finalizer")
			})
		})

		XContext("Members", func() {
			When("group members are not found", func() {
				BeforeEach(func() {
					// Use Eventually to retry the update operation in case of conflicts
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						vmGroup1.Spec.Members = []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: "vm-non-exist",
							},
							{
								Kind: virtualMachineGroupKind,
								Name: "non-non-exist",
							},
						}
						g.Expect(ctx.Client.Update(ctx, vmGroup1)).To(Succeed())
					}).Should(Succeed(), "updating VirtualMachineGroup with non-existent members should succeed after retries")
				})

				It("should set the VM Group condition MembersOwnerRefReady to False", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(conditions.IsFalse(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
						g.Expect(conditions.GetReason(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(Equal("ReconcileMembersError"))
						msg := conditions.GetMessage(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
						g.Expect(msg).To(ContainSubstring(fmt.Sprintf("VirtualMachine.vmoperator.vmware.com %q not found", "vm-non-exist")))
						g.Expect(msg).To(ContainSubstring(fmt.Sprintf("VirtualMachineGroup.vmoperator.vmware.com %q not found", "non-non-exist")))
					}).Should(Succeed())
				})
			})

			When("group members have Spec.GroupName set to different groups", func() {
				BeforeEach(func() {
					// Use Eventually to retry the update operations in case of conflicts
					Eventually(func(g Gomega) {
						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						vmGroup2.Spec.GroupName = "some-other-group"
						g.Expect(ctx.Client.Update(ctx, vmGroup2)).To(Succeed())
					}).Should(Succeed(), "updating vmGroup2 GroupName should succeed after retries")

					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						vmGroup1.Spec.Members = []vmopv1.GroupMember{
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup2Key.Name,
							},
						}
						g.Expect(ctx.Client.Update(ctx, vmGroup1)).To(Succeed())
					}).Should(Succeed(), "updating vmGroup1 Members should succeed after retries")
				})

				It("should set the VM Group condition MembersOwnerRefReady to False", func() {
					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						g.Expect(conditions.IsFalse(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
						msg := conditions.GetMessage(vmGroup1, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
						g.Expect(msg).To(ContainSubstring(fmt.Sprintf("group %s is not a member of group %s", vmGroup2Key.Name, vmGroup1Key.Name)))
					}).Should(Succeed())
				})
			})

			When("group members exist and Spec.GroupName is set to the current group", func() {
				BeforeEach(func() {
					// Use Eventually to retry the update operations in case of conflicts
					Eventually(func(g Gomega) {
						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						vmGroup2.Spec.GroupName = vmGroup1Key.Name
						vmGroup2.Spec.Members = []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm2Key.Name,
							},
						}
						g.Expect(ctx.Client.Update(ctx, vmGroup2)).To(Succeed())
					}).Should(Succeed(), "updating vmGroup2 should succeed after retries")

					Eventually(func(g Gomega) {
						vmGroup3 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup3Key, vmGroup3)).To(Succeed())
						vmGroup3.Spec.GroupName = vmGroup1Key.Name
						vmGroup3.Spec.Members = []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm3Key.Name,
							},
						}
						g.Expect(ctx.Client.Update(ctx, vmGroup3)).To(Succeed())
					}).Should(Succeed(), "updating vmGroup3 should succeed after retries")

					Eventually(func(g Gomega) {
						vmGroup1 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup1Key, vmGroup1)).To(Succeed())
						vmGroup1.Spec.Members = []vmopv1.GroupMember{
							{
								Kind: virtualMachineKind,
								Name: vm1Key.Name,
							},
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup2Key.Name,
							},
							{
								Kind: virtualMachineGroupKind,
								Name: vmGroup3Key.Name,
							},
						}
						g.Expect(ctx.Client.Update(ctx, vmGroup1)).To(Succeed())
					}).Should(Succeed(), "updating vmGroup1 should succeed after retries")
				})

				It("should set the VM Group condition MembersOwnerRefReady to True and update the owner references on the group members", func() {
					Eventually(func(g Gomega) {
						// vm1 is a direct member of vmGroup1.
						vm1 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm1Key, vm1)).To(Succeed())
						g.Expect(vm1.OwnerReferences).To(HaveLen(1))
						g.Expect(vm1.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

						// vmGroup2 is a direct member of vmGroup1.
						vmGroup2 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup2Key, vmGroup2)).To(Succeed())
						g.Expect(vmGroup2.OwnerReferences).To(HaveLen(1))
						g.Expect(vmGroup2.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

						// vmGroup3 is a direct member of vmGroup1.
						vmGroup3 := &vmopv1.VirtualMachineGroup{}
						g.Expect(ctx.Client.Get(ctx, vmGroup3Key, vmGroup3)).To(Succeed())
						g.Expect(vmGroup3.OwnerReferences).To(HaveLen(1))
						g.Expect(vmGroup3.OwnerReferences[0].Name).To(Equal(vmGroup1Key.Name))

						// vm2 is a direct member of vmGroup2.
						vm2 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm2Key, vm2)).To(Succeed())
						g.Expect(vm2.OwnerReferences).To(HaveLen(1))
						g.Expect(vm2.OwnerReferences[0].Name).To(Equal(vmGroup2Key.Name))

						// vm3 is a direct member of vmGroup3.
						vm3 := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vm3Key, vm3)).To(Succeed())
						g.Expect(vm3.OwnerReferences).To(HaveLen(1))
						g.Expect(vm3.OwnerReferences[0].Name).To(Equal(vmGroup3Key.Name))
					}).Should(Succeed(), "waiting for VirtualMachineGroup owner references to be set")
				})
			})
		})

		Context("Placement", func() {
			// TODO(sai): Add tests for ReconcilePlacement.
		})

		Context("PowerState", func() {
			// TODO(sai): Add tests for ReconcilePowerState.
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
