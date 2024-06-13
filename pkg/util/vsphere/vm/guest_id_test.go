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

	Context("UpdateVMGuestIDCondition", func() {

		var (
			vmCtx      pkgctx.VirtualMachineContext
			vmopVM     vmopv1.VirtualMachine
			configSpec vimtypes.VirtualMachineConfigSpec
			taskInfo   *vimtypes.TaskInfo
		)

		BeforeEach(func() {
			vmopVM = vmopv1.VirtualMachine{}
			vmCtx = pkgctx.VirtualMachineContext{
				VM: &vmopVM,
			}
			configSpec = vimtypes.VirtualMachineConfigSpec{}
			taskInfo = &vimtypes.TaskInfo{}
		})

		JustBeforeEach(func() {
			vmutil.UpdateVMGuestIDCondition(vmCtx, configSpec, taskInfo)
		})

		Context("ConfigSpec doesn't have a guest ID", func() {

			BeforeEach(func() {
				configSpec.GuestId = ""
				conditions.MarkFalse(&vmopVM, vmopv1.GuestIDCondition, "Invalid", "Invalid guest ID")
			})

			It("should not change the existing VM's guest ID condition", func() {
				c := conditions.Get(&vmopVM, vmopv1.GuestIDCondition)
				Expect(c).NotTo(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
				Expect(c.Reason).To(Equal("Invalid"))
				Expect(c.Message).To(Equal("Invalid guest ID"))
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

				It("should set the VM's guest ID condition to true", func() {
					c := conditions.Get(&vmopVM, vmopv1.GuestIDCondition)
					Expect(c).NotTo(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
				})
			})

			When("TaskInfo.Error is nil", func() {

				BeforeEach(func() {
					taskInfo.Error = nil
				})

				It("should set the VM's guest ID condition to true", func() {
					c := conditions.Get(&vmopVM, vmopv1.GuestIDCondition)
					Expect(c).NotTo(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
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

				It("should set the VM's guest ID condition to true", func() {
					c := conditions.Get(&vmopVM, vmopv1.GuestIDCondition)
					Expect(c).NotTo(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
				})
			})

			When("TaskInfo.Error contains an invalid property error", func() {

				BeforeEach(func() {
					taskInfo.Error = &vimtypes.LocalizedMethodFault{
						Fault: &vimtypes.InvalidArgument{
							InvalidProperty: "configSpec.guestId",
						},
					}
				})

				It("should set the VM's guest ID condition to false", func() {
					c := conditions.Get(&vmopVM, vmopv1.GuestIDCondition)
					Expect(c).NotTo(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("Invalid"))
					Expect(c.Message).To(Equal("The specified guest ID value is not supported: test-guest-id-value"))
				})
			})
		})
	})
}
