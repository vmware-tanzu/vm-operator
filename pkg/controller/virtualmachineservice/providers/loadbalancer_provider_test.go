// +build !integration

/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

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

		nl LoadbalancerProvider

		lb string

		ncpClient clientset.Interface
	)

	Context("Create Loadbalancer", func() {
		Context("nsx-t loadbalancer provider", func() {
			BeforeEach(func() {
				ncpClient = ncpfake.NewSimpleClientset()
				nl = NsxtLoadBalancerProvider(ncpClient)
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
					lb, err = nl.EnsureLoadBalancer(ctx, vmService, "dummy-network")
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
					lb, err = nl.EnsureLoadBalancer(ctx, vmService, "dummy-network")
				})
				Context("create load balancer", func() {
					It("nsx-t network provider should successfully create a lb with virtual network", func() {
						Expect(err).To(BeNil())
						Expect(lb).NotTo(Equal(""))
					})

					It("nsx-t network provider should successfully get a lb with virtual network", func() {
						lb, err = nl.EnsureLoadBalancer(ctx, vmService, "dummy-network")
						Expect(err).To(BeNil())
						Expect(lb).NotTo(Equal(""))
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
					vnetName, err := nl.GetNetworkName(virtualMachines)
					Expect(err).ToNot(BeNil())
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
					vnetName, err := nl.GetNetworkName(virtualMachines)
					Expect(err).To(BeNil())
					Expect(vnetName).To(Equal("dummy-vnet"))
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
					vnetName, err := nl.GetNetworkName(virtualMachines)
					Expect(err).ToNot(BeNil())
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
					vnetName, err := nl.GetNetworkName(virtualMachines)
					Expect(err).ToNot(BeNil())
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
					vnetName, err := nl.GetNetworkName(virtualMachines)
					Expect(err).ToNot(BeNil())
					Expect(vnetName).To(Equal(""))
				})
			})
		})
	})

})
