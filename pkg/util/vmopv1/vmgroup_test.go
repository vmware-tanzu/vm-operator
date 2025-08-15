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
