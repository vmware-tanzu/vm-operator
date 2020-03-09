// +build !integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

var _ = Describe("Utils", func() {
	Context("k8s object compare function test", func() {
		Context("k8s service compare", func() {
			var (
				port = corev1.ServicePort{
					Name:     "foo",
					Protocol: "TCP",
					Port:     42,
				}

				service = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","metadata":{"annotations":{},"name":"dummy-service","namespace":"dummy-ns"},"spec":{"ports":[{"name":"foo","port":42,"protocol":"TCP"}],"type":"LoadBalancer"}}`},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeLoadBalancer,
						Ports: []corev1.ServicePort{port},
					},
				}
			)
			It("should be equal for same service", func() {
				ok, err := ServiceEqual(service, service)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
			})

			It("should not be equal for different service", func() {
				newService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","metadata":{"annotations":{},"name":"dummy-service","namespace":"default"},"spec":{"ports":[{"name":"foo","port":42,"protocol":"TCP","targetPort":42}],"selector":{"foo":"bar"},"type":"LoadBalancer"}}`},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}
				ok, err := ServiceEqual(newService, service)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

			It("should return error for invalid annotation", func() {
				invaildService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"d"}`},
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}
				ok, err := ServiceEqual(service, invaildService)
				Expect(err).Should(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

			It("should return false for nil annotation", func() {
				invaildService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: nil,
					},
					Spec: corev1.ServiceSpec{
						Type:  corev1.ServiceTypeClusterIP,
						Ports: []corev1.ServicePort{port},
					},
				}
				ok, err := ServiceEqual(service, invaildService)
				Expect(err).Should(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})

		Context("virtual machine service compare", func() {
			var (
				vmPort = vmoperatorv1alpha1.VirtualMachineServicePort{
					Name:       "foo",
					Protocol:   "TCP",
					Port:       42,
					TargetPort: 42,
				}

				vmService = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","metadata":{"annotations":{},"name":"dummy-service","namespace":"default"},"spec":{"ports":[{"name":"foo","port":42,"protocol":"TCP","targetPort":42}],"selector":{"foo":"bar"},"type":"LoadBalancer"}}`},
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmPort},
						Selector: map[string]string{"foo": "bar"},
					},
				}
			)

			It("should be equal for same vm service", func() {
				ok, err := VMServiceEqual(vmService, vmService)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
			})

			It("should not be equal for different service", func() {
				newVMService := &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "dummy-ns",
						Name:      "dummy-service",
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmPort},
						Selector: map[string]string{"test": "test"},
					},
				}

				ok, err := VMServiceEqual(newVMService, vmService)
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

			It("should return error for invalid annotation", func() {
				invalidVMService := &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: `{"d"}`},
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:     vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{vmPort},
						Selector: map[string]string{"test": "test"},
					},
				}

				ok, err := VMServiceEqual(vmService, invalidVMService)
				Expect(err).Should(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

			It("should return false for nil annotation", func() {
				invalidVMService := &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "dummy-ns",
						Name:        "dummy-service",
						Annotations: nil,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:  vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer,
						Ports: []vmoperatorv1alpha1.VirtualMachineServicePort{vmPort},
					},
				}
				ok, err := VMServiceEqual(vmService, invalidVMService)
				Expect(err).Should(HaveOccurred())
				Expect(ok).To(BeFalse())
			})

		})

	})

})
