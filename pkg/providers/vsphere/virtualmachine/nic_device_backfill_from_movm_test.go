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
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func backfillNICDevicePropertiesTests() {

	moVMWithDevices := func(devs ...vimtypes.BaseVirtualDevice) mo.VirtualMachine {
		return mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				Hardware: vimtypes.VirtualHardware{
					Device: devs,
				},
			},
		}
	}

	vmxnet3 := func(numaNode int32, uptv2Enabled *bool) *vimtypes.VirtualVmxnet3 {
		dev := &vimtypes.VirtualVmxnet3{Uptv2Enabled: uptv2Enabled}
		dev.NumaNode = numaNode
		return dev
	}

	sriovDev := func(numaNode int32) *vimtypes.VirtualSriovEthernetCard {
		dev := &vimtypes.VirtualSriovEthernetCard{}
		dev.NumaNode = numaNode
		return dev
	}

	Describe("BackfillNICDevicePropertiesFromMoVM", func() {
		var (
			vm   *vmopv1.VirtualMachine
			moVM mo.VirtualMachine
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: "eth0",
								Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
							},
						},
					},
				},
			}
			moVM = moVMWithDevices()
		})

		// ------------------------------------------------------------------ //
		// Guard conditions
		// ------------------------------------------------------------------ //

		When("moVM.Config is nil", func() {
			BeforeEach(func() {
				moVM = mo.VirtualMachine{Config: nil}
			})

			It("returns no mutation", func() {
				mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
			})
		})

		When("VM has no network spec", func() {
			BeforeEach(func() {
				vm.Spec.Network = nil
				moVM = moVMWithDevices(vmxnet3(1, ptr.To(true)))
			})

			It("returns no mutation", func() {
				mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
			})
		})

		When("VM has no interfaces", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = nil
				moVM = moVMWithDevices(vmxnet3(1, ptr.To(true)))
			})

			It("returns no mutation", func() {
				mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
			})
		})

		// ------------------------------------------------------------------ //
		// VNUMANodeID backfill
		// ------------------------------------------------------------------ //

		Context("VNUMANodeID from VirtualDevice.NumaNode", func() {
			When("NumaNode is positive", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(2, nil))
				})

				It("backfills VNUMANodeID", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(2)))
				})
			})

			When("NumaNode is zero (unset / indistinguishable from no-affinity)", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(0, nil))
				})

				It("does not backfill VNUMANodeID", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).To(BeNil())
				})
			})

			When("NumaNode is negative (explicitly no affinity)", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(-1, nil))
				})

				It("does not backfill VNUMANodeID", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).To(BeNil())
				})
			})

			When("VNUMANodeID is already set (spec wins)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].VNUMANodeID = ptr.To(int32(5))
					moVM = moVMWithDevices(vmxnet3(7, nil))
				})

				It("does not overwrite existing value", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(5)))
				})
			})

			When("NIC is SRIOV (VNUMANodeID still backfilled — it is on VirtualDevice base)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type =
						vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
					moVM = moVMWithDevices(sriovDev(3))
				})

				It("backfills VNUMANodeID regardless of NIC type", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VNUMANodeID).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VNUMANodeID).To(Equal(int32(3)))
				})
			})
		})

		// ------------------------------------------------------------------ //
		// UPTv2Enabled backfill
		// ------------------------------------------------------------------ //

		Context("UPTv2Enabled from VirtualVmxnet3.Uptv2Enabled", func() {
			When("Uptv2Enabled is true", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(0, ptr.To(true)))
				})

				It("backfills vmxnet3.UPTv2Enabled = true", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeTrue())
				})
			})

			When("Uptv2Enabled is false", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(0, ptr.To(false)))
				})

				It("backfills vmxnet3.UPTv2Enabled = false", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeFalse())
				})
			})

			When("Uptv2Enabled is nil on device (not configured)", func() {
				BeforeEach(func() {
					moVM = moVMWithDevices(vmxnet3(0, nil))
				})

				It("does not mutate", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
				})
			})

			When("UPTv2Enabled already set in spec (spec wins)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].VMXNet3 =
						&vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
							UPTv2Enabled: ptr.To(false),
						}
					moVM = moVMWithDevices(vmxnet3(0, ptr.To(true)))
				})

				It("does not overwrite existing value", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeFalse())
				})
			})

			When("NIC type is SRIOV (not VMXNet3 — UPTv2 skipped)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type =
						vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV
					moVM = moVMWithDevices(vmxnet3(0, ptr.To(true)))
				})

				It("does not backfill UPTv2Enabled for SRIOV interface", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeFalse())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).To(BeNil())
				})
			})

			When("NIC type is unset (defaults to VMXNet3 treatment)", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces[0].Type = ""
					moVM = moVMWithDevices(vmxnet3(0, ptr.To(true)))
				})

				It("backfills UPTv2Enabled for untyped interface", func() {
					mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(mutated).To(BeTrue())
					Expect(vm.Spec.Network.Interfaces[0].VMXNet3).ToNot(BeNil())
					Expect(*vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled).To(BeTrue())
				})
			})
		})

		// ------------------------------------------------------------------ //
		// Multi-NIC and alignment
		// ------------------------------------------------------------------ //

		Context("multi-NIC index alignment", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
					{Name: "eth1", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV},
					{Name: "eth2", Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3},
				}
			})

			It("aligns device[0] → interfaces[0], device[1] → interfaces[1], etc.", func() {
				moVM = moVMWithDevices(
					vmxnet3(1, ptr.To(true)),                    // interfaces[0]
					sriovDev(2),                                  // interfaces[1] — only VNUMANodeID
					vmxnet3(3, ptr.To(false)),                    // interfaces[2]
					vmxnet3(4, ptr.To(true)),                     // no spec interface → ignored
				)

				mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				ifaces := vm.Spec.Network.Interfaces

				// eth0: VMXNet3 with numaNode=1, uptv2=true
				Expect(ifaces[0].VNUMANodeID).ToNot(BeNil())
				Expect(*ifaces[0].VNUMANodeID).To(Equal(int32(1)))
				Expect(ifaces[0].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[0].VMXNet3.UPTv2Enabled).To(BeTrue())

				// eth1: SRIOV with numaNode=2; no UPTv2
				Expect(ifaces[1].VNUMANodeID).ToNot(BeNil())
				Expect(*ifaces[1].VNUMANodeID).To(Equal(int32(2)))
				Expect(ifaces[1].VMXNet3).To(BeNil())

				// eth2: VMXNet3 with numaNode=3, uptv2=false
				Expect(ifaces[2].VNUMANodeID).ToNot(BeNil())
				Expect(*ifaces[2].VNUMANodeID).To(Equal(int32(3)))
				Expect(ifaces[2].VMXNet3).ToNot(BeNil())
				Expect(*ifaces[2].VMXNet3.UPTv2Enabled).To(BeFalse())
			})

			It("extra spec interfaces beyond device count are left unchanged", func() {
				moVM = moVMWithDevices(vmxnet3(1, ptr.To(true))) // only 1 device, 3 spec ifaces

				mutated, err := virtualmachine.BackfillNICDevicePropertiesFromMoVM(vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				ifaces := vm.Spec.Network.Interfaces
				Expect(ifaces[0].VNUMANodeID).ToNot(BeNil())
				Expect(ifaces[1].VNUMANodeID).To(BeNil())
				Expect(ifaces[2].VNUMANodeID).To(BeNil())
			})
		})
	})
}
