// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

var _ = Describe(
	"SetDefaultNetworkInterfaceTypes",
	Label(
		testlabels.API,
		testlabels.Mutation,
		testlabels.Webhook,
	),
	func() {

		var (
			vm *vmopv1.VirtualMachine
		)

		expectMutationSuccess := func() {
			wasMutated, err := mutation.SetDefaultNetworkInterfaceTypes(vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeTrue())
		}

		expectNoMutation := func() {
			wasMutated, err := mutation.SetDefaultNetworkInterfaceTypes(vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
		}

		BeforeEach(func() {
			vm = builder.DummyVirtualMachine()
		})

		When("VM has no network interfaces", func() {
			BeforeEach(func() {
				vm.Spec.Network = nil
			})

			It("should not mutate", func() {
				expectNoMutation()
			})
		})

		When("VM has empty network interfaces array", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{},
				}
			})

			It("should not mutate", func() {
				expectNoMutation()
			})
		})

		When("VM has interface with empty type", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0"},
					},
				}
			})

			It("should set type to VMXNet3", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		When("VM has interface with existing type", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV},
					},
				}
			})

			It("should not overwrite existing type", func() {
				expectNoMutation()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})

		When("VM has mixed empty and set interface types", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{Name: "eth0"}, // empty
						{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV}, // set
						{Name: "eth2"}, // empty
					},
				}
			})

			It("should set empty types to VMXNet3 and preserve existing types", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
				Expect(vm.Spec.Network.Interfaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
				Expect(vm.Spec.Network.Interfaces[2].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})
	},
)
