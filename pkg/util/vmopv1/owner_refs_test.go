// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	apiVersion = "vmoperator.vmware.com/v1alpha4"
	groupKind  = "VirtualMachineGroup"
)

var _ = Describe("RemoveStaleGroupOwnerRef", func() {

	var (
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
						APIVersion: apiVersion,
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
						APIVersion: apiVersion,
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
						APIVersion: apiVersion,
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
						APIVersion: apiVersion,
						Kind:       groupKind,
						Name:       oldGroupName,
						UID:        oldGroupUID,
					},
					{
						APIVersion: apiVersion,
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
				Expect(newVM.OwnerReferences[0].APIVersion).To(Equal(apiVersion))
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
						APIVersion: apiVersion,
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
