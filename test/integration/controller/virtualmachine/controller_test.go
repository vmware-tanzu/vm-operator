/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachine_test

import (
	"time"

	. "gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/client/clientset_generated/clientset/typed/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/test/integration"

	"github.com/golang/glog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VirtualMachine controller", func() {
	var instanceName string
	var expectedKey string

	var before chan struct{}
	var after chan struct{}
	var beforeDelete chan struct{}
	var afterDelete chan struct{}

	var imageClient v1alpha1.VirtualMachineImageInterface
	var vmClient v1alpha1.VirtualMachineInterface
	var vmClass VirtualMachineClass
	var vmClassClient v1alpha1.VirtualMachineClassInterface

	namespace := integration.DefaultNamespace

	BeforeEach(func() {
		instanceName = "instance-1"
		expectedKey = namespace + "/instance-1"

		imageClient = cs.VmoperatorV1alpha1().VirtualMachineImages(namespace)
		vmClient = cs.VmoperatorV1alpha1().VirtualMachines(namespace)
		vmClass = VirtualMachineClass{}
		vmClass.Name = "vmClass-1"
		vmClassSpec := VirtualMachineClassSpec{}
		vmClassSpec.Hardware.Memory, _ = resource.ParseQuantity("1Mi")
		vmClassSpec.Policies.Resources.Limits.Memory, _ = resource.ParseQuantity("1Mi")
		vmClassSpec.Policies.Resources.Requests.Memory, _ = resource.ParseQuantity("1Mi")

		vmClassClient = cs.VmoperatorV1alpha1().VirtualMachineClasses(namespace)
		_, _ = vmClassClient.Create(&vmClass)
	})

	AfterEach(func() {
		_ = vmClient.Delete(instanceName, &metav1.DeleteOptions{})
		_ = vmClassClient.Delete(vmClass.Name, &metav1.DeleteOptions{})
	})

	Describe("when creating/deleting a VM object", func() {
		It("invoke the reconcile method", func() {
			before = make(chan struct{})
			after = make(chan struct{})
			beforeDelete = make(chan struct{})
			afterDelete = make(chan struct{})

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
				Spec:       VirtualMachineSpec{ImageName: first.Name, ClassName: vmClass.Name},
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

			// Sleep for a while before invoking delete to
			// complete any ongoing reconciliation loops.
			// TODO:Varun - Find out if there is a better way to do this.
			time.Sleep(time.Second * 5)

			// Update reconcile callbacks
			controller.BeforeReconcile = func(key string) {
				controller.BeforeReconcile = nil
				defer close(beforeDelete)
				actualKey = key
			}
			controller.AfterReconcile = func(key string, err error) {
				controller.AfterReconcile = nil
				defer close(afterDelete)
				actualKey = key
				actualErr = err
			}

			err = vmClient.Delete(instance.Name, &metav1.DeleteOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			// Verify reconcile function is called against the correct key
			select {
			case <-beforeDelete:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never called")
			}

			select {
			case <-afterDelete:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never finished")
			}
		})
	})
})
