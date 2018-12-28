
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package v1beta1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	. "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1beta1"
)

var _ = Describe("VirtualMachine", func() {
	var instance VirtualMachine
	var expected VirtualMachine
	var client VirtualMachineInterface

	BeforeEach(func() {
		instance = VirtualMachine{}
		instance.Name = "instance-1"

		expected = instance
	})

	AfterEach(func() {
		client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for a valid config", func() {
			It("should provide CRUD access to the object", func() {
				client = cs.VmoperatorV1beta1().VirtualMachines("virtualmachine-test-valid")

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
