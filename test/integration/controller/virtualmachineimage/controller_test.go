/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	. "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = XDescribe("VirtualMachineImage controller", func() {
	var instance VirtualMachineImage
	var expectedKey string
	var client VirtualMachineImageInterface
	var before chan struct{}
	var after chan struct{}

	BeforeEach(func() {
		instance = VirtualMachineImage{}
		instance.Name = "instance-1"
		expectedKey = "virtualmachineimage-controller-test-handler/instance-1"
	})

	AfterEach(func() {
		client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	XDescribe("when creating a new object", func() {
		It("invoke the reconcile method", func() {
			client = cs.VmoperatorV1alpha1().VirtualMachineImages("virtualmachineimage-controller-test-handler")
			before = make(chan struct{})
			after = make(chan struct{})

			actualKey := ""
			var actualErr error = nil

			// Setup test callbacks to be called when the message is reconciled
			controller.BeforeReconcile = func(key string) {
				controller.BeforeReconcile = nil
				defer close(before)
				actualKey = key
			}
			controller.AfterReconcile = func(key string, err error) {
				controller.AfterReconcile = nil
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
