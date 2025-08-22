// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Validating RetrieveVMGroupMembers",
	Label(
		testlabels.EnvTest,
		testlabels.API,
		testlabels.Webhook,
	),
	func() {
		var (
			ctx           *builder.UnitTestContext
			vmGroup       *vmopv1.VirtualMachineGroup
			vmChildGroup  *vmopv1.VirtualMachineGroup
			visitedGroups *sets.Set[string]
		)

		BeforeEach(func() {
			vmGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: builder.DummyVMGroupName,
				},
			}
			vmChildGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: builder.DummyVMGroupName + "-child",
				},
			}
			visitedGroups = &sets.Set[string]{}
		})

		JustBeforeEach(func() {
			ctx = builder.NewUnitTestContext(vmGroup, vmChildGroup)

			Expect(ctx.Client.Status().Update(ctx, vmGroup)).Should(Succeed())
			Expect(ctx.Client.Status().Update(ctx, vmChildGroup)).Should(Succeed())
		})

		AfterEach(func() {
			ctx = nil
			vmGroup = nil
			visitedGroups = nil
			visitedGroups = nil
		})

		When("there is no vm group", func() {
			It("should return NotFound err", func() {
				vmGroupSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKey{Namespace: "", Name: uuid.NewString()}, visitedGroups)
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(vmGroupSet).To(BeNil())
			})
		})

		When("there is no vm group members", func() {
			It("should return empty set without error", func() {
				vmGroupSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmGroupSet).To(HaveLen(0))
			})
		})

		When("there are vm group members but members are not group linked", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
					{Name: builder.DummyVMGroupName + "-child", Kind: "VirtualMachineGroup"}}
				vmChildGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-1", Kind: "VirtualMachine"},
				}
			})

			It("should return empty set without error", func() {
				vmGroupSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmGroupSet).To(HaveLen(0))
			})
		})

		When("there are vm group members and members are group linked", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
					{Name: builder.DummyVMGroupName + "-child", Kind: "VirtualMachineGroup"}}
				vmChildGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-1", Kind: "VirtualMachine"},
				}
				conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmChildGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)

			})

			It("should return a set of group linked vm names without error", func() {
				vmGroupSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmGroupSet).To(HaveLen(2))
			})
		})

		When("there are duplicated vm group members", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"}}
				conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			})

			It("should return duplicated error", func() {
				_, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(
					fmt.Sprintf("duplicate member %q found in the group", builder.DummyVirtualMachineName+"-0")))
			})
		})

		When("there are duplicated vm group members within a nested group", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
					{Name: builder.DummyVMGroupName + "-child", Kind: "VirtualMachineGroup"}}
				vmChildGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
				}
				conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmChildGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			})

			It("should return duplicated error", func() {
				_, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf(
					"duplicate member(s) found in the group: %q",
					[]string{builder.DummyVirtualMachineName + "-0"})))
			})
		})

		When("there is a loop among groups", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "VirtualMachine"},
					{Name: builder.DummyVMGroupName + "-child", Kind: "VirtualMachineGroup"},
				}
				vmChildGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-1", Kind: "VirtualMachine"},
					{Name: builder.DummyVMGroupName, Kind: "VirtualMachineGroup"},
				}
				conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmChildGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(&vmChildGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			})

			It("should return duplicated error", func() {
				_, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("a loop is detected among groups: %q visisted", vmGroup.Name)))
			})
		})

		When("there is a unknown member kind", func() {
			BeforeEach(func() {
				vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{Name: builder.DummyVirtualMachineName + "-0", Kind: "unknown"},
				}
				conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			})

			It("should return error if there is a unknown member kind", func() {
				_, err := vmopv1util.RetrieveVMGroupMembers(ctx, ctx.Client,
					ctrlclient.ObjectKeyFromObject(vmGroup), visitedGroups)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("a member with unknown kind"))
			})
		})

	})

var _ = Describe("UpdateGroupLinkedCondition",
	Label(
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {
		var (
			ctx        *builder.UnitTestContext
			rootGroup  *vmopv1.VirtualMachineGroup
			childGroup *vmopv1.VirtualMachineGroup
			childVM    *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			rootGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmg-root",
				},
			}

			childGroup = &vmopv1.VirtualMachineGroup{
				TypeMeta: metav1.TypeMeta{
					Kind: "VirtualMachineGroup",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmg-child",
				},
			}

			childVM = &vmopv1.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind: "VirtualMachine",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "vm-child",
				},
			}
		})

		JustBeforeEach(func() {
			ctx = builder.NewUnitTestContext(rootGroup, childGroup, childVM)
		})

		AfterEach(func() {
			ctx = nil
		})

		When("member has no group name", func() {
			BeforeEach(func() {
				childVM.Spec.GroupName = ""
				childGroup.Spec.GroupName = ""
			})

			It("should delete the group linked condition", func() {
				// Add group link condition to verify the condition is actually deleted.
				conditions.MarkTrue(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
				conditions.MarkTrue(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)

				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childVM, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeNil())

				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childGroup, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeNil())
			})
		})

		When("group is not found", func() {
			BeforeEach(func() {
				childVM.Spec.GroupName = "non-existent-group"
				childGroup.Spec.GroupName = "non-existent-group"
			})

			It("should mark group linked condition as false with NotFound reason", func() {
				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childVM, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotFound"))

				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childGroup, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotFound"))
			})
		})

		When("member is not found in group's boot order", func() {
			BeforeEach(func() {
				childVM.Spec.GroupName = rootGroup.Name
				childGroup.Spec.GroupName = rootGroup.Name
				rootGroup.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							// Set incorrect kinds to make members not found.
							{
								Name: childVM.Name,
								Kind: "VirtualMachineGroup",
							},
							{
								Name: childGroup.Name,
								Kind: "VirtualMachine",
							},
						},
					},
				}
			})

			It("should mark condition as false with NotMember reason", func() {
				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childVM, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
				Expect(conditions.Get(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotMember"))

				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childGroup, ctx.Client)).To(Succeed())
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).ToNot(BeNil())
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Status).To(Equal(metav1.ConditionFalse))
				Expect(conditions.Get(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked).Reason).To(Equal("NotMember"))
			})
		})

		When("member is found in group's boot order", func() {
			BeforeEach(func() {
				childVM.Spec.GroupName = rootGroup.Name
				childGroup.Spec.GroupName = rootGroup.Name
				rootGroup.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{
								Name: childVM.Name,
								Kind: "VirtualMachine",
							},
							{
								Name: childGroup.Name,
								Kind: "VirtualMachineGroup",
							},
						},
					},
				}
			})

			It("should mark group linked condition as true", func() {
				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childVM, ctx.Client)).To(Succeed())
				Expect(conditions.IsTrue(childVM, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())

				Expect(vmopv1util.UpdateGroupLinkedCondition(ctx, childGroup, ctx.Client)).To(Succeed())
				Expect(conditions.IsTrue(childGroup, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
			})
		})
	},
)
