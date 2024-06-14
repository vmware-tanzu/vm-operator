// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

func guestIDTests() {

	Context("UpdateVMGuestIDReconfiguredCondition", func() {

		var (
			vmCtx      pkgctx.VirtualMachineContext
			vmopVM     vmopv1.VirtualMachine
			configSpec vimtypes.VirtualMachineConfigSpec
			taskInfo   *vimtypes.TaskInfo
		)

		BeforeEach(func() {
			// Init VM with the condition set to verify it's actually deleted.
			vmopVM = vmopv1.VirtualMachine{
				Status: vmopv1.VirtualMachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vmopv1.GuestIDReconfiguredCondition,
							Status: metav1.ConditionFalse,
						},
					},
				},
			}
			vmCtx = pkgctx.VirtualMachineContext{
				VM: &vmopVM,
			}
			configSpec = vimtypes.VirtualMachineConfigSpec{}
			taskInfo = &vimtypes.TaskInfo{}
		})

		JustBeforeEach(func() {
			vmutil.UpdateVMGuestIDReconfiguredCondition(vmCtx, configSpec, taskInfo)
		})

		Context("ConfigSpec doesn't have a guest ID", func() {

			BeforeEach(func() {
				configSpec.GuestId = ""
			})

			It("should delete the existing VM's guest ID condition", func() {
				Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
			})
		})

		Context("ConfigSpec has a guest ID", func() {

			BeforeEach(func() {
				configSpec.GuestId = "test-guest-id-value"
			})

			When("TaskInfo is nil", func() {

				BeforeEach(func() {
					taskInfo = nil
				})

				It("should delete the VM's guest ID condition", func() {
					Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
				})
			})

			When("TaskInfo.Error is nil", func() {

				BeforeEach(func() {
					taskInfo.Error = nil
				})

				It("should delete the VM's guest ID condition", func() {
					Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
				})
			})

			When("TaskInfo.Error.Fault is not an InvalidPropertyFault", func() {

				BeforeEach(func() {
					taskInfo.Error = &vimtypes.LocalizedMethodFault{
						Fault: &vimtypes.InvalidName{
							Name: "some-invalid-name",
						},
					}
				})

				It("should delete the VM's guest ID condition", func() {
					Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
				})
			})

			When("TaskInfo.Error contains an invalid property error NOT about guestID", func() {

				BeforeEach(func() {
					taskInfo.Error = &vimtypes.LocalizedMethodFault{
						Fault: &vimtypes.InvalidArgument{
							InvalidProperty: "config.version",
						},
					}
				})

				It("should delete the VM's guest ID condition", func() {
					Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
				})
			})

			When("TaskInfo.Error contains an invalid property error of guestID", func() {

				BeforeEach(func() {
					taskInfo.Error = &vimtypes.LocalizedMethodFault{
						Fault: &vimtypes.InvalidArgument{
							InvalidProperty: "configSpec.guestId",
						},
					}
				})

				It("should set the VM's guest ID condition to false with the invalid value in the reason", func() {
					c := conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)
					Expect(c).NotTo(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("Invalid"))
					Expect(c.Message).To(Equal("The specified guest ID value is not supported: test-guest-id-value"))
				})
			})
		})
	})
}
