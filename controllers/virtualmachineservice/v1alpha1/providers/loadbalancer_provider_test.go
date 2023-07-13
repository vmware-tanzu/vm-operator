// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha1/utils"
)

const (
	dummyNamespace = "dummy"
)

var _ = Describe("Loadbalancer Provider", func() {
	var (
		ctx        context.Context
		vmService  *vmopv1.VirtualMachineService
		lbProvider LoadbalancerProvider
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Get load balancer provider by type", func() {
		It("should successfully get nsx-t load balancer provider", func() {
			lbProvider, err := GetLoadbalancerProviderByType(nil, NSXTLoadBalancer)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(lbProvider).ToNot(BeNil())
		})

		It("should successfully get a noop loadbalancer provider", func() {
			lbProvider, err := GetLoadbalancerProviderByType(nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(lbProvider).To(Equal(NoopLoadbalancerProvider{}))
		})
	})

	Context("noop loadbalancer provider", func() {
		JustBeforeEach(func() {
			lbProvider = &NoopLoadbalancerProvider{}
		})

		Context("EnsureLoadBalancer", func() {
			It("should return success", func() {
				err := lbProvider.EnsureLoadBalancer(ctx, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("GetServiceLabels", func() {
			It("should return empty", func() {
				annotations, err := lbProvider.GetServiceLabels(ctx, nil)
				Expect(annotations).To(BeNil())
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("GetServiceAnnotations", func() {
			It("should return empty", func() {
				annotations, err := lbProvider.GetServiceAnnotations(ctx, nil)
				Expect(annotations).To(BeNil())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("GetServiceAnnotations when VMService has healthCheckNodePort defined", func() {
		BeforeEach(func() {
			vmService = &vmopv1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dummy-vmservice",
					Namespace:   dummyNamespace,
					Annotations: make(map[string]string),
				},
				Spec: vmopv1.VirtualMachineServiceSpec{
					Type:         vmopv1.VirtualMachineServiceTypeClusterIP,
					ClusterIP:    "TEST",
					ExternalName: "TEST",
				},
			}
			vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
			lbProvider = NsxtLoadBalancerProvider()
		})

		It("should get health check node port in the annotation", func() {
			vmServiceAnnotations, err := lbProvider.GetServiceAnnotations(ctx, vmService)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmServiceAnnotations).ToNot(BeNil())
			port := vmServiceAnnotations[ServiceLoadBalancerHealthCheckNodePortTagKey]
			Expect(port).To(Equal("30012"))
		})
	})

	Context("GetToBeRemovedServiceAnnotations when VMService does not have healthCheckNodePort defined", func() {
		BeforeEach(func() {
			vmService = &vmopv1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dummy-vmservice",
					Namespace:   dummyNamespace,
					Annotations: make(map[string]string),
				},
				Spec: vmopv1.VirtualMachineServiceSpec{
					Type:         vmopv1.VirtualMachineServiceTypeClusterIP,
					ClusterIP:    "TEST",
					ExternalName: "TEST",
				},
			}
			lbProvider = NsxtLoadBalancerProvider()
		})

		It("should get health check node port in the to be removed annotation", func() {
			vmServiceAnnotations, err := lbProvider.GetToBeRemovedServiceAnnotations(ctx, vmService)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmServiceAnnotations).ToNot(BeNil())
			Expect(vmServiceAnnotations).To(HaveKey(ServiceLoadBalancerHealthCheckNodePortTagKey))
		})
	})

	Context("GetServiceLabels when VMService have externalTrafficPolicy annotation defined", func() {
		BeforeEach(func() {
			vmService = &vmopv1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dummy-vmservice",
					Namespace:   dummyNamespace,
					Annotations: make(map[string]string),
				},
				Spec: vmopv1.VirtualMachineServiceSpec{
					Type:         vmopv1.VirtualMachineServiceTypeClusterIP,
					ClusterIP:    "TEST",
					ExternalName: "TEST",
				},
			}
			vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeCluster)
			lbProvider = NsxtLoadBalancerProvider()
		})

		Context("EnsureLoadBalancer", func() {
			It("should return success", func() {
				err := lbProvider.EnsureLoadBalancer(ctx, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("etp is Cluster", func() {
			It("should not create any label", func() {
				labels, err := lbProvider.GetServiceLabels(ctx, vmService)
				Expect(err).ToNot(HaveOccurred())
				Expect(labels).To(BeEmpty())
			})
		})

		Context("etp is Local", func() {
			BeforeEach(func() {
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
			})

			It("should create one label for ServiceProxyName", func() {
				labels, err := lbProvider.GetServiceLabels(ctx, vmService)
				Expect(err).ToNot(HaveOccurred())
				Expect(labels).To(HaveLen(1))
				Expect(labels[LabelServiceProxyName]).To(Equal(NSXTServiceProxy))
			})
		})
	})

	Context("GetToBeRemovedServiceLabels", func() {
		var (
			labels map[string]string
			err    error
		)

		BeforeEach(func() {
			vmService = &vmopv1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dummy-vmservice",
					Namespace:   dummyNamespace,
					Annotations: make(map[string]string),
				},
				Spec: vmopv1.VirtualMachineServiceSpec{
					Type:         vmopv1.VirtualMachineServiceTypeClusterIP,
					ClusterIP:    "TEST",
					ExternalName: "TEST",
				},
			}
			lbProvider = NsxtLoadBalancerProvider()
		})

		JustBeforeEach(func() {
			labels, err = lbProvider.GetToBeRemovedServiceLabels(ctx, vmService)
		})

		Context("etp is not present", func() {
			It("should remove ServiceProxyName label", func() {
				Expect(err).ToNot(HaveOccurred())
				_, exists := labels[LabelServiceProxyName]
				Expect(exists).To(BeTrue())
			})
		})

		Context("etp is Local", func() {
			BeforeEach(func() {
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
			})

			It("should not remove ServiceProxyName label", func() {
				Expect(err).ToNot(HaveOccurred())
				_, exists := labels[LabelServiceProxyName]
				Expect(exists).To(BeFalse())
			})
		})

		Context("etp is Cluster", func() {
			BeforeEach(func() {
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeCluster)
			})

			It("should remove ServiceProxyName label", func() {
				Expect(err).ToNot(HaveOccurred())
				_, exists := labels[LabelServiceProxyName]
				Expect(exists).To(BeTrue())
			})
		})
	})
})
