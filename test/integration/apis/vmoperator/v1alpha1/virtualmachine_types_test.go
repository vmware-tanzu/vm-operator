/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	. "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"
)

var _ = Describe("VirtualMachine", func() {
	var instance VirtualMachine
	var expected VirtualMachine
	var client VirtualMachineInterface
	var vmClass VirtualMachineClass
	var vmClassClient VirtualMachineClassInterface

	BeforeEach(func() {
		instance = VirtualMachine{Spec: VirtualMachineSpec{ImageName: "foo"}}
		instance.Name = "instance-vm"

		vmClass = VirtualMachineClass{}
		vmClass.Name = "vmClass-1"
		vmClassSpec := VirtualMachineClassSpec{}
		vmClassSpec.Hardware.Memory, _ = resource.ParseQuantity("1Mi")
		vmClassSpec.Policies.Resources.Limits.Memory, _ = resource.ParseQuantity("1Mi")
		vmClassSpec.Policies.Resources.Requests.Memory, _ = resource.ParseQuantity("1Mi")

		instance.Spec.ClassName = vmClass.Name
		expected = instance
	})

	AfterEach(func() {
		_ = client.Delete(instance.Name, &metav1.DeleteOptions{})
		_ = vmClassClient.Delete(vmClass.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for an invalid config", func() {
			It("should fail to create the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachines("virtualmachine-test-invalid")
				vmClassClient = cs.VmoperatorV1alpha1().VirtualMachineClasses("virtualmachine-test-invalid")

				By("returning failure from the create request when image isn't specified")
				imageInvalid := VirtualMachine{}
				_, err := client.Create(&imageInvalid)
				Expect(err).Should(HaveOccurred())
			})
		})
		Context("for a valid config", func() {
			It("should provide CRUD access to the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachines("virtualmachine-test-valid")
				vmClassClient = cs.VmoperatorV1alpha1().VirtualMachineClasses("virtualmachine-test-valid")

				By("returning success from the create request")
				actual, err := client.Create(&instance)
				Expect(err).ShouldNot(HaveOccurred())

				By("defaulting the expected fields")
				Expect(actual.Spec).To(Equal(expected.Spec))

				By("returning the item for list requests")
				result, err := client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0].Spec).To(Equal(expected.Spec))

				By("returning the item for get requests")
				actual, err = client.Get(instance.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actual.Spec).To(Equal(expected.Spec))

				By("deleting the item for delete requests")
				err = client.Delete(instance.Name, &metav1.DeleteOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				result, err = client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				// Object must still exist due to use of finalizer
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0].DeletionTimestamp.IsZero()).Should(BeFalse())
			})
		})
	})
})
