/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineclass_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	. "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VirtualMachineClass controller", func() {
	var instance VirtualMachineClass
	var expectedKey string
	var client VirtualMachineClassInterface
	var before chan struct{}
	var after chan struct{}

	BeforeEach(func() {
		instance = VirtualMachineClass{}
		instance.Name = "instance-1"
		instance.Spec = VirtualMachineClassSpec{}
		instance.Spec.Hardware.Memory, _ = resource.ParseQuantity("1Mi")
		instance.Spec.Policies.Resources.Limits.Memory, _ = resource.ParseQuantity("1Mi")
		instance.Spec.Policies.Resources.Requests.Memory, _ = resource.ParseQuantity("1Mi")

		expectedKey = "virtualmachineclass-controller-test-handler/instance-1"
	})

	AfterEach(func() {
		_ = client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when creating a new object", func() {
		It("invoke the reconcile method", func() {
			client = cs.VmoperatorV1alpha1().VirtualMachineClasses("virtualmachineclass-controller-test-handler")
			before = make(chan struct{})
			after = make(chan struct{})

			actualKey := ""
			var actualErr error = nil

			// Setup test callbacks to be called when the message is reconciled
			controller.BeforeReconcile = func(key string) {
				defer close(before)
				actualKey = key
			}
			controller.AfterReconcile = func(key string, err error) {
				defer close(after)
				actualKey = key
				actualErr = err
			}

			// Create an instance
			_, err := client.Create(&instance)
			Expect(err).ShouldNot(HaveOccurred())

			// Verify reconcile function is called against the correct key
			select {
			case <-before:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never called")
			}

			select {
			case <-after:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never finished")
			}
		})
	})
})
