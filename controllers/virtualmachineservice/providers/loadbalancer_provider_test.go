// +build !integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

const (
	dummyObjectName = "dummy"
	dummyNamespace  = "dummy"
)

var _ = Describe("Loadbalancer Provider", func() {
	var (
		err                  error
		ctx                  context.Context
		vmService            *vmoperatorv1alpha1.VirtualMachineService
		loadBalancerProvider nsxtLoadbalancerProvider
		lb                   string
		ncpClient            clientset.Interface
	)

	Context("Create Loadbalancer", func() {

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

		Context("nsx-t loadbalancer provider", func() {
			BeforeEach(func() {
				ncpClient = ncpfake.NewSimpleClientset()

				loadBalancerProvider = nsxtLoadbalancerProvider{client: ncpClient}

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
			})

			Context("load balancer name", func() {
				It("load balancer name should be namespace-clustername-lb", func() {
					lb = loadBalancerProvider.getLoadbalancerName(vmService.Namespace, vmService.Spec.Selector[ClusterNameKey])
					Expect(lb).To(Equal("dummy-test-lb"))
				})

				It("create loadbalancer should return error without cluster name", func() {
					vmService.Spec.Selector = nil
					err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("virtual network doesn't exist", func() {
				BeforeEach(func() {
					err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
				})

				It("nsx-t network provider should fail to create a lb when virtual network doesn't exist", func() {
					Expect(k8serrors.IsNotFound(err)).To(Equal(true))
				})
			})

			Context("virtual network exists", func() {
				var (
					vnet *ncpv1alpha1.VirtualNetwork
				)
				BeforeEach(func() {
					vnet = &ncpv1alpha1.VirtualNetwork{
						TypeMeta: metav1.TypeMeta{
							Kind:       "VirtualNetwork",
							APIVersion: "vmware.com/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-network",
							Namespace: vmService.GetNamespace(),
						},
					}
				})
				JustBeforeEach(func() {
					_, err = ncpClient.VmwareV1alpha1().VirtualNetworks(dummyNamespace).Create(vnet)
					Expect(err).To(BeNil())
					err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
				})
				Context("create load balancer", func() {
					It("nsx-t network provider should successfully create a lb with virtual network", func() {
						Expect(err).To(BeNil())
						By("ensuring VirtualMachineService has the ncp loadbalancer annotation")
						Expect(vmService.Annotations).ToNot(BeNil())
						_, exists := vmService.Annotations[ServiceLoadBalancerTagKey]
						Expect(exists).To(BeTrue())
					})

					Context("vmservice has the healthCheckNodePort label", func() {
						BeforeEach(func() {
							vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey] = "30012"
						})
						AfterEach(func() {
							delete(vmService.Labels, utils.AnnotationServiceHealthCheckNodePortKey)
						})
						It("nsx-t network provider should successfully create a lb with virtual network", func() {
							Expect(err).To(BeNil())
							By("ensuring VirtualMachineService has the ncp healthCheckNodePort annotation")
							Expect(vmService.Annotations).ToNot(BeNil())
							hcPort, exists := vmService.Annotations[ServiceLoadBalancerHealthCheckNodePortTagKey]
							Expect(exists).To(BeTrue())
							Expect(hcPort).To(Equal("30012"))
						})

					})

					It("nsx-t network provider should successfully get a lb with virtual network", func() {
						err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(BeNil())
					})

					It("nsx-t network provider should fail to get a lb with error", func() {
						errorClient := &ncpfake.Clientset{}
						errorClient.AddReactor("get", "loadbalancers", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, fmt.Errorf("an error occurred while getting load balancer")
						})
						loadBalancerProvider = nsxtLoadbalancerProvider{client: errorClient}
						err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(MatchError("an error occurred while getting load balancer"))
					})

					It("nsx-t network provider should fail to get a lb with getting virtual network error", func() {
						errorClient := &ncpfake.Clientset{}
						errorClient.AddReactor("get", "loadbalancers", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, k8serrors.NewNotFound(ncpv1alpha1.Resource("loadbalancers"), lb)
						})
						errorClient.AddReactor("get", "virtualnetworks", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, fmt.Errorf("an error occurred while getting virtual networks")
						})
						loadBalancerProvider = nsxtLoadbalancerProvider{client: errorClient}
						err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(MatchError("an error occurred while getting virtual networks"))
					})

					It("nsx-t network provider should fail to create a lb with error occur", func() {
						errorClient := &ncpfake.Clientset{}
						errorClient.AddReactor("get", "loadbalancers", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, k8serrors.NewNotFound(ncpv1alpha1.Resource("loadbalancers"), lb)
						})

						errorClient.AddReactor("create", "loadbalancers", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, fmt.Errorf("an error occurred while create load balancers")
						})

						loadBalancerProvider = nsxtLoadbalancerProvider{client: errorClient}
						err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(MatchError("an error occurred while create load balancers"))
					})
				})

				Context("delete load balancer", func() {
					It("nsx-t network provider should successfully get a lb with virtual network", func() {
						err = ncpClient.VmwareV1alpha1().LoadBalancers(dummyNamespace).Delete(lb, &metav1.DeleteOptions{})
						Expect(err).To(BeNil())
					})
				})

			})

			Context("check virtual network vaild", func() {
				var (
					virtualMachines       []vmoperatorv1alpha1.VirtualMachine
					virtualMachineService *vmoperatorv1alpha1.VirtualMachineService
				)
				BeforeEach(func() {
					virtualMachines = []vmoperatorv1alpha1.VirtualMachine{}
					virtualMachineService = &vmoperatorv1alpha1.VirtualMachineService{}
				})

				Context("without vnet name in virtualMachineService annotation", func() {
					It("nsx-t load balancer provider should not get virtual network name with empty vm ", func() {
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(MatchError("no virtual machine matched selector"))
						Expect(vnetName).To(Equal(""))
					})

					It("nsx-t load balancer provider should successfully get virtual network name", func() {
						virtualMachines = []vmoperatorv1alpha1.VirtualMachine{
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmoperator.vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet",
											NetworkType: "nsx-t",
										},
									},
								},
							},
						}
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(BeNil())
						Expect(vnetName).To(Equal("dummy-vnet"))
					})

					It("nsx-t load balancer provider should fail to get virtual network name with two nsx-t network", func() {
						virtualMachines = []vmoperatorv1alpha1.VirtualMachine{
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet",
											NetworkType: "nsx-t",
										},
										{
											NetworkName: "dummy-vnet-2",
											NetworkType: "nsx-t",
										},
									},
								},
							},
						}
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(MatchError(`virtual machine "dummy/dummy-vm" can't connect to two NST-X virtual network `))
						Expect(vnetName).To(Equal(""))
					})

					It("nsx-t load balancer provider should not get virtual network name with no nsx-t network", func() {
						virtualMachines = []vmoperatorv1alpha1.VirtualMachine{
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet",
											NetworkType: dummyObjectName,
										},
									},
								},
							},
						}
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(MatchError(`virtual machine "dummy/dummy-vm" doesn't have nsx-t virtual network`))
						Expect(vnetName).To(Equal(""))
					})

					It("nsx-t load balancer provider should not get virtual network name with more than one nsx-t network", func() {
						virtualMachines = []vmoperatorv1alpha1.VirtualMachine{
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet",
											NetworkType: "nsx-t",
										},
									},
								},
							},
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet-d",
											NetworkType: "nsx-t",
										},
									},
								},
							},
						}
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(MatchError(`virtual machine "dummy/dummy-vm" has different virtual network with previous vms`))
						Expect(vnetName).To(Equal(""))
					})

					It("nsx-t load balancer provider should not get virtual network name with one don't have nsx-t network", func() {
						virtualMachines = []vmoperatorv1alpha1.VirtualMachine{
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet",
											NetworkType: "nsx-t",
										},
									},
								},
							},
							{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualMachine",
									APIVersion: "vmware.com/v1alpha1",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "dummy-vm",
									Namespace: vmService.GetNamespace(),
								},
								Spec: vmoperatorv1alpha1.VirtualMachineSpec{
									ImageName:  "test",
									ClassName:  "test",
									PowerState: "on",
									NetworkInterfaces: []vmoperatorv1alpha1.VirtualMachineNetworkInterface{
										{
											NetworkName: "dummy-vnet-d",
											NetworkType: dummyObjectName,
										},
									},
								},
							},
						}
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(MatchError(`virtual machine "dummy/dummy-vm" doesn't have nsx-t virtual network`))
						Expect(vnetName).To(Equal(""))
					})
				})

				Context("with vnet name in virtualMachineService annotation", func() {
					BeforeEach(func() {
						virtualMachineService.Annotations = map[string]string{
							"ncp.vmware.com/virtual-network-name": "vnet",
						}
					})
					It("nsx-t load balancer provider should successfully get virtual network name", func() {
						vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines, virtualMachineService)
						Expect(err).To(BeNil())
						Expect(vnetName).To(Equal("vnet"))
					})
					AfterEach(func() {
						virtualMachineService.Annotations = nil
					})
				})

			})

		})
	})

})
