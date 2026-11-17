// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VMExternalNetworkInterfacesIndexerFunc", func() {
	When("the object is not a VirtualMachine", func() {
		It("should return nil", func() {
			Expect(kubeutil.VMExternalNetworkInterfacesIndexerFunc(&corev1.Pod{})).To(BeNil())
		})
	})

	When("the object is a VirtualMachine", func() {
		var vm *vmopv1.VirtualMachine

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: "eth0",
								ExternalRef: &vmopv1common.LocalObjectRef{
									APIVersion: netopv1alpha1.SchemeGroupVersion.String(),
									Kind:       "NetworkInterface",
									Name:       "my-netif",
								},
							},
							{
								Name: "eth1",
								Network: &vmopv1common.PartialObjectRef{
									Name: "my-network",
								},
							},
						},
					},
				},
			}
		})

		It("should return the external references", func() {
			Expect(kubeutil.VMExternalNetworkInterfacesIndexerFunc(vm)).To(Equal([]string{
				"netoperator.vmware.com/NetworkInterface/my-netif",
			}))
		})
	})
})

var _ = Describe("NetworkInterfaceToVMMapper", func() {
	var (
		ctx    context.Context
		vm     *vmopv1.VirtualMachine
		client ctrlclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		vm = builder.DummyBasicVirtualMachine("my-vm", "my-ns")
		vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
			Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name: "eth0",
					ExternalRef: &vmopv1common.LocalObjectRef{
						APIVersion: netopv1alpha1.SchemeGroupVersion.String(),
						Kind:       "NetworkInterface",
						Name:       "my-netif",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		client = fake.NewClientBuilder().
			WithScheme(builder.NewScheme()).
			WithObjects(vm).
			WithIndex(
				&vmopv1.VirtualMachine{},
				kubeutil.VMExternalNetworkInterfacesIndexKey,
				kubeutil.VMExternalNetworkInterfacesIndexerFunc).
			Build()
	})

	vmRequest := func() reconcile.Request {
		return reconcile.Request{NamespacedName: ctrlclient.ObjectKeyFromObject(vm)}
	}

	When("the network interface is referenced by a VM", func() {
		It("should return the VM", func() {
			obj := &netopv1alpha1.NetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      "my-netif",
				},
			}
			Expect(kubeutil.NetworkInterfaceToVMMapper(client)(ctx, obj)).To(ConsistOf(vmRequest()))
		})
	})

	When("a different kind has the referenced name", func() {
		It("should not return the VM", func() {
			obj := &vpcv1alpha1.SubnetPort{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      "my-netif",
				},
			}
			Expect(kubeutil.NetworkInterfaceToVMMapper(client)(ctx, obj)).To(BeEmpty())
		})
	})

	When("the network interface is owned by and referenced by the VM", func() {
		It("should return the VM once", func() {
			obj := &netopv1alpha1.NetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      "my-netif",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: vmopv1.GroupVersion.String(),
							Kind:       "VirtualMachine",
							Name:       vm.Name,
						},
					},
				},
			}
			Expect(kubeutil.NetworkInterfaceToVMMapper(client)(ctx, obj)).To(ConsistOf(vmRequest()))
		})
	})

	When("the network interface is labeled with the VM name", func() {
		BeforeEach(func() {
			vm.Spec.Network = nil
		})

		It("should return the VM", func() {
			obj := &vpcv1alpha1.SubnetPort{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      "my-subnetport",
					Labels: map[string]string{
						"vmoperator.vmware.com/vm-name": vm.Name,
					},
				},
			}
			Expect(kubeutil.NetworkInterfaceToVMMapper(client)(ctx, obj)).To(ConsistOf(vmRequest()))
		})
	})
})
