/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine_test

import (
	"github.com/golang/glog"
	"time"
	"vmware.com/kubevsphere/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
)

var _ = XDescribe("VirtualMachine controller", func() {
	var instanceName string
	var expectedKey string
	var before chan struct{}
	var after chan struct{}
	ns := "virtualmachine-controller-test-handler"
	var imageClient v1alpha1.VirtualMachineImageInterface
	var vmClient v1alpha1.VirtualMachineInterface

	BeforeEach(func() {
		imageClient = cs.VmoperatorV1alpha1().VirtualMachineImages(ns)
		vmClient = cs.VmoperatorV1alpha1().VirtualMachines(ns)
		instanceName = "instance-1"
		expectedKey = "virtualmachine-controller-test-handler/instance-1"
	})

	AfterEach(func() {
		_ = vmClient.Delete(instanceName, &metav1.DeleteOptions{})
	})

	Describe("when creating a new object", func() {
		It("invoke the reconcile method", func() {
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

			// Pick the first VM image
			list, err := imageClient.List(metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(list.Items)).ShouldNot(BeZero())
			first := list.Items[0]

			glog.Infof("Cloning %s from %s", instanceName, first.Name)

			instance := VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: instanceName},
				Spec:       VirtualMachineSpec{ImageName: first.Name},
			}
			// Create an instance
			_, err = vmClient.Create(&instance)
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
