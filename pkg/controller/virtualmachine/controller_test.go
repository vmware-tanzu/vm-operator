
/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/


package virtualmachine_test

import (
	"time"

	. "vmware.com/kubevsphere/pkg/apis/vmoperator/v1beta1"
	. "vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VirtualMachine controller", func() {
	var instance VirtualMachine
	var expectedKey string
	var client VirtualMachineInterface
	var before chan struct{}
	var after chan struct{}

	BeforeEach(func() {
		instance = VirtualMachine{}
		instance.Name = "instance-1"
		expectedKey = "virtualmachine-controller-test-handler/instance-1"
	})

	AfterEach(func() {
		client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when creating a new object", func() {
		It("invoke the reconcile method", func() {
			client = cs.VmoperatorV1beta1().VirtualMachines("virtualmachine-controller-test-handler")
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
