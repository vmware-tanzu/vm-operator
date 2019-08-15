/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	. "github.com/vmware-tanzu/vm-operator/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"
)

var _ = Describe("VirtualMachineClass", func() {
	var instance VirtualMachineClass
	var expected VirtualMachineClass
	var client VirtualMachineClassInterface

	BeforeEach(func() {
		instance = VirtualMachineClass{}
		instance.Name = "instance-1"
		instance.Spec = VirtualMachineClassSpec{}
		instance.Spec.Hardware.Memory, _ = resource.ParseQuantity("1Mi")
		instance.Spec.Policies.Resources.Limits.Memory, _ = resource.ParseQuantity("1Mi")
		instance.Spec.Policies.Resources.Requests.Memory, _ = resource.ParseQuantity("1Mi")

		expected = instance
	})

	AfterEach(func() {
		_ = client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for a valid config", func() {
			It("should provide CRUD access to the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachineClasses()

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
				Expect(result.Items).To(HaveLen(0))
			})
		})
	})
})
