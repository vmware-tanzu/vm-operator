// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

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
		loadBalancerProvider nsxtLoadbalancerProvider
		ncpClient            clientset.Interface
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
				Expect(loadbalancerProvider).To(Equal(noopLoadbalancerProvider{}))
			})

			It("should successfully get nsx-t load balancer provider", func() {
				loadbalancerProvider := NsxtLoadBalancerProvider(ncpClient)
				Expect(loadbalancerProvider).NotTo(BeNil())
			})
		})

		Context("ltest noop loadbalancer provider", func() {
			var (
				lbprovider *noopLoadbalancerProvider
			)
			JustBeforeEach(func() {
				lbprovider = &noopLoadbalancerProvider{}
			})
			Context("test GetNetworkName", func() {
				var (
					networkName string
				)
				JustBeforeEach(func() {
					networkName, err = lbprovider.GetNetworkName([]vmoperatorv1alpha1.VirtualMachine{}, nil)
				})
				It("should return empty", func() {
					Expect(networkName).To(Equal(""))
					Expect(err).To(BeNil())
				})
			})
			Context("test EnsureLoadBalancer", func() {
				JustBeforeEach(func() {
					err = lbprovider.EnsureLoadBalancer(context.Background(), nil, "")
				})
				It("should return empty", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})
			Context("test GetVMServiceAnnotations", func() {
				var (
					annotations map[string]string
				)
				JustBeforeEach(func() {
					annotations, err = lbprovider.GetVMServiceAnnotations(context.Background(), nil)
				})
				It("should return empty", func() {
					Expect(annotations).To(BeNil())
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("testing GetVMServiceAnnotations when VMService has healthCheckNodePort defined", func() {
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
				loadBalancerProvider = nsxtLoadbalancerProvider{ncpClient}
				vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
			})
			It("should get health check node port in the annotation", func() {
				vmServiceAnnotations, err = loadBalancerProvider.GetVMServiceAnnotations(ctx, vmService)
				Expect(vmServiceAnnotations).ToNot(BeNil())
				port := vmServiceAnnotations[ServiceLoadBalancerHealthCheckNodePortTagKey]
				Expect(port).To(Equal("30012"))
			})
		})
	})

})
