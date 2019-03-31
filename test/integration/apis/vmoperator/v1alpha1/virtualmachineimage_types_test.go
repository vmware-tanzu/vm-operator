/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	. "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"
)

var _ = Describe("VirtualMachineImage", func() {
	var instance VirtualMachineImage
	var client VirtualMachineImageInterface

	BeforeEach(func() {
		instance = VirtualMachineImage{}
		instance.Name = "instance-vm-image"
	})

	AfterEach(func() {
		_ = client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for a valid config", func() {
			It("should provide read-only CRUD access to the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachineImages("virtualmachineimage-test-valid")

				By("returning failure from the create request")
				_, err := client.Create(&instance)
				Expect(err).Should(HaveOccurred())

				By("returning failure fromn a delete requests")
				err = client.Delete(instance.Name, &metav1.DeleteOptions{})
				Expect(err).Should(HaveOccurred())

				By("returning the item for list requests")
				result, err := client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(4))
				first := result.Items[0]

				By("returning the first item from the list request")
				actual, err := client.Get(first.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actual.Spec).To(Equal(first.Spec))
			})
		})
	})
})
