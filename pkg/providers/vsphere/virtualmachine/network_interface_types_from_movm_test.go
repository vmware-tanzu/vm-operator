// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

func networkInterfaceTypeTests() {

	moVMWithEthernet := func(devs ...vimtypes.BaseVirtualDevice) mo.VirtualMachine {
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: devs,
				},
			},
		}
	}

	Describe("FillEmptyNetworkInterfaceTypesFromMoVM", func() {
		var (
			vm   *vmopv1.VirtualMachine
			moVM mo.VirtualMachine
		)

		expectMutationSuccess := func() {
			wasMutated, err := virtualmachine.FillEmptyNetworkInterfaceTypesFromMoVM(vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeTrue())
		}

		expectNoMutation := func() {
			wasMutated, err := virtualmachine.FillEmptyNetworkInterfaceTypesFromMoVM(vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
		}

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
					},
				},
			}
		})

		When("moVM has VMXNet3 device", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{})
			})

			It("should set type to VMXNet3", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		When("moVM has SRIOV device", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(&vimtypes.VirtualSriovEthernetCard{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{
						VirtualDevice: vimtypes.VirtualDevice{},
					},
				})
			})

			It("should set type to SRIOV", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})

		DescribeTable("legacy NIC types",
			func(dev vimtypes.BaseVirtualDevice, expectedType vmopv1.VirtualMachineNetworkInterfaceType) {
				moVM = moVMWithEthernet(dev)
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(expectedType))
			},
			Entry("E1000",
				&vimtypes.VirtualE1000{VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}}},
				vmopv1.VirtualMachineNetworkInterfaceTypeE1000),
			Entry("E1000e",
				&vimtypes.VirtualE1000e{VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}}},
				vmopv1.VirtualMachineNetworkInterfaceTypeE1000e),
			Entry("VMXNet2",
				&vimtypes.VirtualVmxnet2{},
				vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet2),
			Entry("PCNet32",
				&vimtypes.VirtualPCNet32{VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}}},
				vmopv1.VirtualMachineNetworkInterfaceTypePCNet32),
		)

		When("moVM has mixed NIC types", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0"},
					{Name: "eth1"},
				}
				moVM = moVMWithEthernet(
					&vimtypes.VirtualE1000{VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}}},
					&vimtypes.VirtualSriovEthernetCard{VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}}},
				)
			})

			It("should set correct type for each interface", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeE1000))
				Expect(vm.Spec.Network.Interfaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV))
			})
		})

		When("VM has more interfaces than moVM NICs", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0"},
					{Name: "eth1"},
				}
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{
					VirtualVmxnet: vimtypes.VirtualVmxnet{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}},
					},
				})
			})

			It("should default extra interfaces to VMXNet3", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
				Expect(vm.Spec.Network.Interfaces[1].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		When("VM interface already has type set", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces[0].Type = vmopv1.VirtualMachineNetworkInterfaceTypeE1000
				moVM = moVMWithEthernet(&vimtypes.VirtualSriovEthernetCard{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{VirtualDevice: vimtypes.VirtualDevice{}},
				})
			})

			It("should not overwrite existing type", func() {
				expectNoMutation()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeE1000))
			})
		})

		When("VM has no network spec", func() {
			BeforeEach(func() {
				vm.Spec.Network = nil
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{
					VirtualVmxnet: vimtypes.VirtualVmxnet{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{
							VirtualDevice: vimtypes.VirtualDevice{},
						},
					},
				})
			})

			It("should not mutate", func() {
				expectNoMutation()
			})
		})

		When("VM has no interfaces", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = nil
				moVM = moVMWithEthernet(&vimtypes.VirtualVmxnet3{
					VirtualVmxnet: vimtypes.VirtualVmxnet{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{
							VirtualDevice: vimtypes.VirtualDevice{},
						},
					},
				})
			})

			It("should not mutate", func() {
				expectNoMutation()
			})
		})

		When("moVM has no config", func() {
			BeforeEach(func() {
				moVM = mo.VirtualMachine{Config: nil}
			})

			It("should default to VMXNet3", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})

		When("moVM has no ethernet devices", func() {
			BeforeEach(func() {
				moVM = moVMWithEthernet(&vimtypes.VirtualPCIPassthrough{
					VirtualDevice: vimtypes.VirtualDevice{},
				})
			})

			It("should default to VMXNet3", func() {
				expectMutationSuccess()
				Expect(vm.Spec.Network.Interfaces[0].Type).To(Equal(vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3))
			})
		})
	})
}
