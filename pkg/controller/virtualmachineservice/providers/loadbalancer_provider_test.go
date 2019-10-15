// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"context"
	"fmt"

	ptr "github.com/kubernetes/utils/pointer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpfake "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned/fake"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dummyObjectName = "dummy"
	dummyNamespace  = "dummy"
)

var _ = Describe("Loadbalancer Provider", func() {
	var (
		err error

		ctx       context.Context
		vmService *v1alpha1.VirtualMachineService

		loadBalancerProvider nsxtLoadbalancerProvider

		lb string

		ncpClient clientset.Interface
	)

	Context("Create Loadbalancer", func() {

		Context("Get load balancer provider by type", func() {
			It("should successfully get nsx-t load balancer provider", func() {
				//TODO:  () Using static ncp client for now, replace it with runtime ncp client
				Skip("Can't locate a kubeconfig in pipeline env, can test this locally")
				loadbalancerProvider := GetLoadbalancerProviderByType(NSXTLoadBalancer)
				Expect(loadbalancerProvider).NotTo(BeNil())
			})

			It("should fail to get unknown type load balancer provide", func() {
				loadbalancerProvider := GetLoadbalancerProviderByType("dummy")
				Expect(loadbalancerProvider).To(BeNil())
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

				vmService = &v1alpha1.VirtualMachineService{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-vmservice",
						Namespace: dummyNamespace,
					},
					Spec: v1alpha1.VirtualMachineServiceSpec{
						Type:         v1alpha1.VirtualMachineServiceTypeClusterIP,
						Ports:        nil,
						Selector:     nil,
						ClusterIP:    "TEST",
						ExternalName: "TEST",
					},
				}
			})

			Context("virtual network doesn't exist", func() {
				BeforeEach(func() {
					lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
				})

				It("nsx-t network provider should fail to create a lb when virtual network doesn't exist", func() {
					Expect(k8serrors.IsNotFound(err)).To(Equal(true))
					Expect(lb).To(Equal(""))
				})
			})

			Context("virtual network exists", func() {
				BeforeEach(func() {
					vnet := &ncpv1alpha1.VirtualNetwork{
						TypeMeta: metav1.TypeMeta{
							Kind:       "VirtualNetwork",
							APIVersion: "vmware.com/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-network",
							Namespace: vmService.GetNamespace(),
						},
					}
					_, err = ncpClient.VmwareV1alpha1().VirtualNetworks(dummyNamespace).Create(vnet)
					Expect(err).To(BeNil())
					lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
				})
				Context("create load balancer", func() {
					It("nsx-t network provider should successfully create a lb with virtual network", func() {
						Expect(err).To(BeNil())
						Expect(lb).NotTo(Equal(""))
					})

					It("nsx-t network provider should successfully get a lb with virtual network", func() {
						lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(BeNil())
						Expect(lb).NotTo(Equal(""))
					})

					It("nsx-t network provider should fail to get a lb with error", func() {
						errorClient := &ncpfake.Clientset{}
						errorClient.AddReactor("get", "loadbalancers", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
							return true, nil, fmt.Errorf("an error occurred while getting load balancer")
						})
						loadBalancerProvider = nsxtLoadbalancerProvider{client: errorClient}
						lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
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
						lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
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
						lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
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
					virtualMachines []vmoperatorv1alpha1.VirtualMachine
				)
				BeforeEach(func() {
					virtualMachines = []vmoperatorv1alpha1.VirtualMachine{}
				})

				It("nsx-t load balancer provider should not get virtual network name with empty vm ", func() {
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
					Expect(err).To(MatchError("no virtual machine matched selector"))
					Expect(vnetName).To(Equal(""))
				})

				It("nsx-t load balancer provider should successfully get virtual network name", func() {
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet",
										NetworkType: vmoperator.NsxtNetworkType,
									},
								},
							},
						},
					}
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet",
										NetworkType: vmoperator.NsxtNetworkType,
									},
									{
										NetworkName: "dummy-vnet-2",
										NetworkType: vmoperator.NsxtNetworkType,
									},
								},
							},
						},
					}
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet",
										NetworkType: dummyObjectName,
									},
								},
							},
						},
					}
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet",
										NetworkType: vmoperator.NsxtNetworkType,
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet-d",
										NetworkType: vmoperator.NsxtNetworkType,
									},
								},
							},
						},
					}
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet",
										NetworkType: vmoperator.NsxtNetworkType,
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
							Spec: v1alpha1.VirtualMachineSpec{
								ImageName:  "test",
								ClassName:  "test",
								PowerState: "on",
								NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
									{
										NetworkName: "dummy-vnet-d",
										NetworkType: dummyObjectName,
									},
								},
							},
						},
					}
					vnetName, err := loadBalancerProvider.GetNetworkName(virtualMachines)
					Expect(err).To(MatchError(`virtual machine "dummy/dummy-vm" doesn't have nsx-t virtual network`))
					Expect(vnetName).To(Equal(""))
				})
			})

			Context("Prepare patch operations for load balancer owner reference patch update", func() {
				var (
					loadBalancer *ncpv1alpha1.LoadBalancer
				)
				BeforeEach(func() {
					vnet := &ncpv1alpha1.VirtualNetwork{
						TypeMeta: metav1.TypeMeta{
							Kind:       "VirtualNetwork",
							APIVersion: "vmware.com/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-network",
							Namespace: dummyNamespace,
						},
					}
					_, err = ncpClient.VmwareV1alpha1().VirtualNetworks(dummyNamespace).Create(vnet)
					Expect(err).To(BeNil())
					lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
					Expect(err).To(BeNil())
					loadBalancer, err = ncpClient.VmwareV1alpha1().LoadBalancers(vmService.Namespace).Get(lb, metav1.GetOptions{})
					Expect(err).To(BeNil())

				})

				It("Should successfully assemble first lb's owner reference patch request", func() {
					payload, err := loadBalancerProvider.PrepareLoadBalancerOwnerRefPatchOperation(loadBalancer, vmService)
					Expect(err).To(BeNil())
					Expect(string(payload)).To(Equal(`[{"op":"add","path":"/metadata/ownerReferences","value":[{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","name":"dummy-vmservice","uid":"","controller":false,"blockOwnerDeletion":true}]}]`))
				})

				It("Should successfully assemble multiple owner references patch request", func() {
					loadBalancer.OwnerReferences = []metav1.OwnerReference{
						{
							UID:                vmService.UID,
							Name:               vmService.Name,
							Controller:         ptr.BoolPtr(false),
							BlockOwnerDeletion: ptr.BoolPtr(true),
							Kind:               ServiceOwnerRefKind,
							APIVersion:         ServiceOwnerRefVersion,
						},
					}
					secondVMService := &v1alpha1.VirtualMachineService{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-vmservice-2",
							Namespace: dummyNamespace,
						},
						Spec: v1alpha1.VirtualMachineServiceSpec{
							Type:         v1alpha1.VirtualMachineServiceTypeClusterIP,
							Ports:        nil,
							Selector:     nil,
							ClusterIP:    "TEST",
							ExternalName: "TEST",
						},
					}
					payload, err := loadBalancerProvider.PrepareLoadBalancerOwnerRefPatchOperation(loadBalancer, secondVMService)
					Expect(err).To(BeNil())
					Expect(string(payload)).To(Equal(`[{"op":"add","path":"/metadata/ownerReferences/-","value":{"apiVersion":"vmoperator.vmware.com/v1alpha1","kind":"VirtualMachineService","name":"dummy-vmservice-2","uid":"","controller":false,"blockOwnerDeletion":true}}]`))
				})
			})

			Context("Update LoadBalancer OwnerReferences", func() {
				It("Should fail without a load balancer created", func() {
					err := loadBalancerProvider.UpdateLoadBalancerOwnerReference(ctx, "dummy", vmService)
					Expect(k8serrors.IsNotFound(err)).To(Equal(true))
				})

				Context("With load balancer", func() {
					BeforeEach(func() {
						vnet := &ncpv1alpha1.VirtualNetwork{
							TypeMeta: metav1.TypeMeta{
								Kind:       "VirtualNetwork",
								APIVersion: "vmware.com/v1alpha1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "dummy-network",
								Namespace: dummyNamespace,
							},
						}
						_, err = ncpClient.VmwareV1alpha1().VirtualNetworks(dummyNamespace).Create(vnet)
						Expect(err).To(BeNil())
						lb, err = loadBalancerProvider.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(BeNil())
					})

					It("Should fail since it is not support json patch in client-go testing now", func() {
						//TODO: Need to update this Test case after client-go version updated
						err := loadBalancerProvider.UpdateLoadBalancerOwnerReference(ctx, lb, vmService)
						Expect(err).To(MatchError(`invalid JSON document`))
					})
				})
			})

		})
	})

})
