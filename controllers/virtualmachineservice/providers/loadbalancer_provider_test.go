// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

const (
	dummyNamespace = "dummy"
)

var _ = Describe("Loadbalancer Provider", func() {
	var (
		err                  error
		ctx                  context.Context
		vmService            *vmoperatorv1alpha1.VirtualMachineService
		loadBalancerProvider *nsxtLoadbalancerProvider
	)

	Context("Create Loadbalancer", func() {
		Context("Get default load balancer type", func() {
			var (
				origlbProvider string
			)
			BeforeEach(func() {
				origlbProvider = os.Getenv("LB_Provider")
			})
			AfterEach(func() {
				os.Setenv("LB_PROVIDER", origlbProvider)
			})
			JustBeforeEach(func() {
				SetLBProvider()
			})
			Context("LB_PROVIDER is non-empty", func() {
				BeforeEach(func() {
					By("setting LB_PROVIDER to a random string")
					Expect(os.Setenv("LB_PROVIDER", "random")).ShouldNot(HaveOccurred())
				})
				It("should defaults to empty", func() {
					Expect(LBProvider).Should(Equal("random"))
				})
			})
			Context("LB_PROVIDER is empty", func() {
				Context("is not VDS networking", func() {
					BeforeEach(func() {
						By("unsetting LB_PROVIDER")
						Expect(os.Unsetenv("LB_PROVIDER")).ShouldNot(HaveOccurred())
					})
					It("should defaults to nsx-t", func() {
						Expect(LBProvider).Should(Equal(NSXTLoadBalancer))
					})
				})
				Context("is VDS networking", func() {
					var (
						origvSphereNetworking string
					)
					BeforeEach(func() {
						origvSphereNetworking = os.Getenv("VSPHERE_NETWORKING")
						By("unsetting LB_PROVIDER")
						Expect(os.Unsetenv("LB_PROVIDER")).ShouldNot(HaveOccurred())
						By("setting VSPHERE_NETWORKING")
						Expect(os.Setenv("VSPHERE_NETWORKING", "true")).ShouldNot(HaveOccurred())
					})
					AfterEach(func() {
						os.Setenv("VSHPERE_NETWORKING", origvSphereNetworking)
					})
					It("should be noop loadbalancer provider", func() {
						Expect(LBProvider).Should(Equal(""))
					})
				})
			})
		})

		Context("Get load balancer provider by type", func() {
			It("should successfully get nsx-t load balancer provider", func() {
				// TODO:  () Using static ncp client for now, replace it with runtime ncp client
				// TODO: This should be an integration test
				Skip("Can't locate a kubeconfig in pipeline env, can test this locally")
				cfg, err := config.GetConfig()
				Expect(err).ShouldNot(HaveOccurred())
				mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
				Expect(err).ShouldNot(HaveOccurred())
				loadbalancerProvider, err := GetLoadbalancerProviderByType(mgr, NSXTLoadBalancer)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(loadbalancerProvider).NotTo(BeNil())
			})

			It("should successfully get a noop loadbalancer provider", func() {
				loadbalancerProvider, err := GetLoadbalancerProviderByType(nil, "dummy")
				Expect(err).NotTo(HaveOccurred())
				Expect(loadbalancerProvider).To(Equal(NoopLoadbalancerProvider{}))
			})

			It("should successfully get nsx-t load balancer provider", func() {
				loadbalancerProvider := NsxtLoadBalancerProvider()
				Expect(loadbalancerProvider).NotTo(BeNil())
			})
		})

		Context("ltest noop loadbalancer provider", func() {
			var (
				lbprovider *NoopLoadbalancerProvider
			)
			JustBeforeEach(func() {
				lbprovider = &NoopLoadbalancerProvider{}
			})
			Context("test EnsureLoadBalancer", func() {
				JustBeforeEach(func() {
					err = lbprovider.EnsureLoadBalancer(context.Background(), nil)
				})
				It("should return empty", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})
			Context("test GetServiceAnnotations", func() {
				var (
					annotations map[string]string
				)
				JustBeforeEach(func() {
					annotations, err = lbprovider.GetServiceAnnotations(context.Background(), nil)
				})
				It("should return empty", func() {
					Expect(annotations).To(BeNil())
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("testing GetServiceAnnotations when VMService has healthCheckNodePort defined", func() {
			var (
				vmServiceAnnotations map[string]string
			)

			BeforeEach(func() {
				vmService = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "dummy-vmservice",
						Namespace:   dummyNamespace,
						Annotations: make(map[string]string),
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:         vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP,
						Ports:        nil,
						Selector:     map[string]string{ClusterNameKey: "test"},
						ClusterIP:    "TEST",
						ExternalName: "TEST",
					},
				}
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
				loadBalancerProvider = &nsxtLoadbalancerProvider{}
			})

			It("should get health check node port in the annotation", func() {
				vmServiceAnnotations, err = loadBalancerProvider.GetServiceAnnotations(ctx, vmService)
				Expect(vmServiceAnnotations).ToNot(BeNil())
				port := vmServiceAnnotations[ServiceLoadBalancerHealthCheckNodePortTagKey]
				Expect(port).To(Equal("30012"))
			})
		})

		Context("testing GetToBeRemovedServiceAnnotations when VMService does not have healthCheckNodePort defined", func() {
			var (
				vmServiceAnnotations map[string]string
			)

			BeforeEach(func() {
				vmService = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "dummy-vmservice",
						Namespace:   dummyNamespace,
						Annotations: make(map[string]string),
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:         vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP,
						Ports:        nil,
						Selector:     map[string]string{ClusterNameKey: "test"},
						ClusterIP:    "TEST",
						ExternalName: "TEST",
					},
				}
				loadBalancerProvider = &nsxtLoadbalancerProvider{}
			})

			It("should get health check node port in the to be removed annotation", func() {
				vmServiceAnnotations, err = loadBalancerProvider.GetToBeRemovedServiceAnnotations(ctx, vmService)
				Expect(vmServiceAnnotations).ToNot(BeNil())
				_, exist := vmServiceAnnotations[ServiceLoadBalancerHealthCheckNodePortTagKey]
				Expect(exist).To(BeTrue())
			})
		})

		Context("testing GetServiceLabels when VMService have externalTrafficPolicy annotation defined", func() {
			var (
				labels map[string]string
			)

			BeforeEach(func() {
				vmService = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "dummy-vmservice",
						Namespace:   dummyNamespace,
						Annotations: make(map[string]string),
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:         vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP,
						Ports:        nil,
						Selector:     map[string]string{ClusterNameKey: "test"},
						ClusterIP:    "TEST",
						ExternalName: "TEST",
					},
				}
				vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeCluster)
				loadBalancerProvider = &nsxtLoadbalancerProvider{}
			})
			When("etp is Cluster", func() {
				It("should not create any label", func() {
					labels, err = loadBalancerProvider.GetServiceLabels(ctx, vmService)
					Expect(len(labels)).To(Equal(0))
				})

			})
			When("etp is Local", func() {
				BeforeEach(func() {
					vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				})
				It("should create one label for ServiceProxyName", func() {
					labels, err = loadBalancerProvider.GetServiceLabels(ctx, vmService)
					Expect(len(labels)).To(Equal(1))
					Expect(labels[LabelServiceProxyName]).To(Equal(NSXTServiceProxy))
				})

			})
		})

		Context("testing GetToBeRemovedServiceLabels", func() {
			var (
				labels map[string]string
			)

			BeforeEach(func() {
				vmService = &vmoperatorv1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "dummy-vmservice",
						Namespace:   dummyNamespace,
						Annotations: make(map[string]string),
					},
					Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
						Type:         vmoperatorv1alpha1.VirtualMachineServiceTypeClusterIP,
						Ports:        nil,
						Selector:     map[string]string{ClusterNameKey: "test"},
						ClusterIP:    "TEST",
						ExternalName: "TEST",
					},
				}
				loadBalancerProvider = &nsxtLoadbalancerProvider{}
			})

			JustBeforeEach(func() {
				labels, err = loadBalancerProvider.GetToBeRemovedServiceLabels(ctx, vmService)
			})

			When("etp is not present", func() {
				It("should remove ServiceProxyName label", func() {
					_, exists := labels[LabelServiceProxyName]
					Expect(exists).To(BeTrue())
				})
			})
			When("etp is Local", func() {
				BeforeEach(func() {
					vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeLocal)
				})
				It("should not remove ServiceProxyName label", func() {
					_, exists := labels[LabelServiceProxyName]
					Expect(exists).To(BeFalse())
				})

			})
			When("etp is Cluster", func() {
				BeforeEach(func() {
					vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey] = string(corev1.ServiceExternalTrafficPolicyTypeCluster)
				})
				It("should remove ServiceProxyName label", func() {
					_, exists := labels[LabelServiceProxyName]
					Expect(exists).To(BeTrue())
				})

			})
		})

	})

})
