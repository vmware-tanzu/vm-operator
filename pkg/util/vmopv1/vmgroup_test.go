// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

var _ = Describe("RemoveStaleGroupOwnerRef", func() {
	const (
		groupKind    = "VirtualMachineGroup"
		oldGroupName = "old-group"
		newGroupName = "new-group"
		oldGroupUID  = types.UID("old-group-owner-uid")
		newGroupUID  = types.UID("new-group-owner-uid")
	)

	Context("VirtualMachine", func() {
		var (
			oldVM *vmopv1.VirtualMachine
			newVM *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			oldVM = builder.DummyVirtualMachine()
			newVM = builder.DummyVirtualMachine()
		})

		When("old group name is empty", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = ""
				newVM.Spec.GroupName = newGroupName
			})

			It("should return false without modifying owner references", func() {
				originalRefs := newVM.OwnerReferences
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeFalse())
				Expect(newVM.OwnerReferences).To(Equal(originalRefs))
			})
		})

		When("group name hasn't changed", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = oldGroupName
				newVM.Spec.GroupName = oldGroupName
			})

			It("should return false without modifying owner references", func() {
				originalRefs := newVM.OwnerReferences
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeFalse())
				Expect(newVM.OwnerReferences).To(Equal(originalRefs))
			})
		})

		When("group name changed but no stale group owner reference exists", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = oldGroupName
				newVM.Spec.GroupName = newGroupName
				newVM.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       newGroupName,
						UID:        newGroupUID,
					},
				}
			})

			It("should return false without modifying owner references", func() {
				originalRefs := newVM.OwnerReferences
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeFalse())
				Expect(newVM.OwnerReferences).To(Equal(originalRefs))
			})
		})

		When("group name changed and stale owner reference exists", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = oldGroupName
				newVM.Spec.GroupName = newGroupName
				newVM.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       oldGroupName,
						UID:        oldGroupUID,
					},
				}
			})

			It("should return true and remove the stale owner reference", func() {
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeTrue())
				Expect(newVM.OwnerReferences).To(BeEmpty())
			})
		})

		When("group name removed and stale owner reference exists", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = oldGroupName
				newVM.Spec.GroupName = ""
				newVM.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       oldGroupName,
						UID:        newGroupUID,
					},
				}
			})

			It("should return true and remove the stale owner reference", func() {
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeTrue())
				Expect(newVM.OwnerReferences).To(BeEmpty())
			})
		})

		When("multiple VirtualMachineGroup owner references exist", func() {
			BeforeEach(func() {
				oldVM.Spec.GroupName = oldGroupName
				newVM.Spec.GroupName = newGroupName
				newVM.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       oldGroupName,
						UID:        oldGroupUID,
					},
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       newGroupName,
						UID:        newGroupUID,
					},
				}
			})

			It("should only remove the stale owner reference", func() {
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVM, oldVM)
				Expect(result).To(BeTrue())
				Expect(newVM.OwnerReferences).To(HaveLen(1))
				Expect(newVM.OwnerReferences[0].APIVersion).To(Equal(vmopv1.GroupVersion.String()))
				Expect(newVM.OwnerReferences[0].Kind).To(Equal(groupKind))
				Expect(newVM.OwnerReferences[0].Name).To(Equal(newGroupName))
				Expect(newVM.OwnerReferences[0].UID).To(Equal(newGroupUID))
			})
		})
	})

	Context("VirtualMachineGroup", func() {
		var (
			oldVMG = &vmopv1.VirtualMachineGroup{}
			newVMG = &vmopv1.VirtualMachineGroup{}
		)

		When("group name changed and stale owner reference exists", func() {
			BeforeEach(func() {
				oldVMG.Spec.GroupName = oldGroupName
				newVMG.Spec.GroupName = newGroupName
				newVMG.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       groupKind,
						Name:       oldGroupName,
						UID:        oldGroupUID,
					},
				}
			})

			It("should return true and remove the stale owner reference", func() {
				result := vmopv1util.RemoveStaleGroupOwnerRef(newVMG, oldVMG)
				Expect(result).To(BeTrue())
				Expect(newVMG.OwnerReferences).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("GroupToMembersMapperFn", func() {
	const (
		namespaceName  = "fake-namespace"
		groupName      = "group-name"
		childGroupName = "child-group-name"
		vmName         = "vm-name"
		vmKind         = "VirtualMachine"
		groupKind      = "VirtualMachineGroup"
	)
	var (
		ctx                 context.Context
		k8sClient           ctrlclient.Client
		groupObj            *vmopv1.VirtualMachineGroup
		withObjs            []ctrlclient.Object
		reqs                []reconcile.Request
		linkedTrueCondition = metav1.Condition{
			Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			Status: metav1.ConditionTrue,
		}
	)
	BeforeEach(func() {
		ctx = context.Background()
		withObjs = nil
		groupObj = &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      groupName,
			},
		}
	})
	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClient(withObjs...)
	})
	Context("Group to VirtualMachine kind members", func() {
		JustBeforeEach(func() {
			reqs = vmopv1util.GroupToMembersMapperFn(ctx, k8sClient, vmKind)(ctx, groupObj)
		})
		When("group status has no linked VM kind members", func() {
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("group status has linked VM kind member", func() {
			BeforeEach(func() {
				groupObj.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{
						Kind:       vmKind,
						Name:       vmName,
						Conditions: []metav1.Condition{linkedTrueCondition},
					},
				}
			})
			When("VM has a different group name", func() {
				BeforeEach(func() {
					vmObj := &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      vmName,
						},
						Spec: vmopv1.VirtualMachineSpec{
							GroupName: "different-group",
						},
					}
					withObjs = append(withObjs, vmObj)
				})
				Specify("no reconcile requests should be returned", func() {
					Expect(reqs).To(BeEmpty())
				})
			})
			When("VM status has both linked and placement ready conditions true", func() {
				BeforeEach(func() {
					vmObj := &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      vmName,
						},
						Spec: vmopv1.VirtualMachineSpec{
							GroupName: groupName,
						},
						Status: vmopv1.VirtualMachineStatus{
							Conditions: []metav1.Condition{
								linkedTrueCondition,
								metav1.Condition{
									Type:   vmopv1.VirtualMachineConditionPlacementReady,
									Status: metav1.ConditionTrue,
								},
							},
						},
					}
					withObjs = append(withObjs, vmObj)
				})
				Specify("no reconcile requests should be returned", func() {
					Expect(reqs).To(BeEmpty())
				})
			})
			When("VM status has neither linked nor placement ready conditions", func() {
				BeforeEach(func() {
					vmObj := &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      vmName,
						},
						Spec: vmopv1.VirtualMachineSpec{
							GroupName: groupName,
						},
						Status: vmopv1.VirtualMachineStatus{
							Conditions: []metav1.Condition{},
						},
					}
					withObjs = append(withObjs, vmObj)
				})
				Specify("one reconcile request should be returned", func() {
					Expect(reqs).To(HaveExactElements(
						reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: namespaceName,
								Name:      vmName,
							},
						},
					))
				})
			})
			When("VM status has linked condition true and placement ready condition false", func() {
				BeforeEach(func() {
					vmObj := &vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      vmName,
						},
						Spec: vmopv1.VirtualMachineSpec{
							GroupName: groupName,
						},
						Status: vmopv1.VirtualMachineStatus{
							Conditions: []metav1.Condition{
								linkedTrueCondition,
								metav1.Condition{
									Type:   vmopv1.VirtualMachineConditionPlacementReady,
									Status: metav1.ConditionFalse,
								},
							},
						},
					}
					withObjs = append(withObjs, vmObj)
				})
				Specify("one reconcile request should be returned", func() {
					Expect(reqs).To(HaveExactElements(
						reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: namespaceName,
								Name:      vmName,
							},
						},
					))
				})
			})
		})
	})
	Context("Group to VirtualMachineGroup kind members", func() {
		JustBeforeEach(func() {
			reqs = vmopv1util.GroupToMembersMapperFn(ctx, k8sClient, groupKind)(ctx, groupObj)
		})
		When("parent group status has no linked VMGroup kind members", func() {
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("parent group status has linked VMGroup kind member", func() {
			BeforeEach(func() {
				groupObj.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
					{
						Kind:       groupKind,
						Name:       childGroupName,
						Conditions: []metav1.Condition{linkedTrueCondition},
					},
				}
			})
			When("child group member has a different parent group name", func() {
				BeforeEach(func() {
					childGroupObj := &vmopv1.VirtualMachineGroup{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      childGroupName,
						},
						Spec: vmopv1.VirtualMachineGroupSpec{
							GroupName: "different-group",
						},
					}
					withObjs = append(withObjs, childGroupObj)
				})
				Specify("no reconcile requests should be returned", func() {
					Expect(reqs).To(BeEmpty())
				})
			})
			When("child group member has linked condition true", func() {
				BeforeEach(func() {
					childGroupObj := &vmopv1.VirtualMachineGroup{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      childGroupName,
						},
						Spec: vmopv1.VirtualMachineGroupSpec{
							GroupName: groupName,
						},
						Status: vmopv1.VirtualMachineGroupStatus{
							Conditions: []metav1.Condition{
								linkedTrueCondition,
								metav1.Condition{
									Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
									Status: metav1.ConditionTrue,
								},
							},
						},
					}
					withObjs = append(withObjs, childGroupObj)
				})
				Specify("no reconcile requests should be returned", func() {
					Expect(reqs).To(BeEmpty())
				})
			})
			When("child group member has linked condition false", func() {
				BeforeEach(func() {
					childGroupObj := &vmopv1.VirtualMachineGroup{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      childGroupName,
						},
						Spec: vmopv1.VirtualMachineGroupSpec{
							GroupName: groupName,
						},
						Status: vmopv1.VirtualMachineGroupStatus{
							Conditions: []metav1.Condition{
								metav1.Condition{
									Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
									Status: metav1.ConditionFalse,
								},
							},
						},
					}
					withObjs = append(withObjs, childGroupObj)
				})
				Specify("one reconcile request should be returned", func() {
					Expect(reqs).To(HaveExactElements(
						reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: namespaceName,
								Name:      childGroupName,
							},
						},
					))
				})
			})
		})
	})
})

var _ = Describe("MemberToGroupMapperFn", func() {
	var (
		ctx      context.Context
		groupKey types.NamespacedName
		vmKey    types.NamespacedName
		vmgKey   types.NamespacedName
	)
	BeforeEach(func() {
		ctx = context.Background()
		nsName := "fake-namespace"
		groupKey = types.NamespacedName{
			Namespace: nsName,
			Name:      "group-name",
		}
		vmKey = types.NamespacedName{
			Namespace: nsName,
			Name:      "vm-name",
		}
		vmgKey = types.NamespacedName{
			Namespace: nsName,
			Name:      "vmg-name",
		}
	})

	It("should enqueue a reconcile from a linked VM kind member to its group", func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vmKey.Namespace,
				Name:      vmKey.Name,
			},
		}
		reqs := vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vm)
		Expect(reqs).To(BeEmpty())

		vm.Spec.GroupName = groupKey.Name
		reqs = vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vm)
		Expect(reqs).To(BeEmpty())

		vm.Status.Conditions = []metav1.Condition{
			{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			},
		}
		reqs = vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vm)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].NamespacedName).To(Equal(groupKey))
	})

	It("should enqueue a reconcile from a linked VMGroup kind member to its group", func() {
		vmg := &vmopv1.VirtualMachineGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vmgKey.Namespace,
				Name:      vmgKey.Name,
			},
		}
		reqs := vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vmg)
		Expect(reqs).To(BeEmpty())

		vmg.Spec.GroupName = groupKey.Name
		reqs = vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vmg)
		Expect(reqs).To(BeEmpty())

		vmg.Status.Conditions = []metav1.Condition{
			{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			},
		}
		reqs = vmopv1util.MemberToGroupMapperFn(ctx)(ctx, vmg)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].NamespacedName).To(Equal(groupKey))
	})
})
