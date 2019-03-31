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

var _ = Describe("VirtualMachineService", func() {
	var instance VirtualMachineService
	var expected VirtualMachineService
	var client VirtualMachineServiceInterface
	typ := "ClusterIP"
	port := VirtualMachineServicePort{
		Name:       "foo",
		Protocol:   "TCP",
		Port:       22,
		TargetPort: 22,
	}
	selector := map[string]string{"foo": "bar"}

	BeforeEach(func() {
		instance = VirtualMachineService{Spec: VirtualMachineServiceSpec{
			Type:     typ,
			Ports:    []VirtualMachineServicePort{port},
			Selector: selector,
		}}
		instance.Name = "instance-vm-service"

		expected = instance
	})

	AfterEach(func() {
		_ = client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for an invalid config", func() {
			It("should fail to create the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachineServices("virtualmachineservice-test-invalid")

				By("returning failure from the create request when type not specified")
				typeInvalid := VirtualMachineService{
					Spec: VirtualMachineServiceSpec{
						Ports:    []VirtualMachineServicePort{port},
						Selector: selector,
					},
				}
				_, err := client.Create(&typeInvalid)
				Expect(err).Should(HaveOccurred())

				By("returning failure from the create request when port not specified")
				portInvalid := VirtualMachineService{
					Spec: VirtualMachineServiceSpec{
						Type:     typ,
						Selector: selector,
					},
				}
				_, err = client.Create(&portInvalid)
				Expect(err).Should(HaveOccurred())

				By("returning failure from the create request when selector not specified")
				selectorInvalid := VirtualMachineService{
					Spec: VirtualMachineServiceSpec{
						Type:  typ,
						Ports: []VirtualMachineServicePort{port},
					},
				}
				_, err = client.Create(&selectorInvalid)
				Expect(err).Should(HaveOccurred())
			})
		})
		Context("for a valid config", func() {
			It("should provide CRUD access to the object", func() {
				client = cs.VmoperatorV1alpha1().VirtualMachineServices("virtualmachineservice-test-valid")

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
