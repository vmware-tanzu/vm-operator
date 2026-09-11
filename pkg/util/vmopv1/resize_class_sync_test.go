// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("SyncClassSizeAndAllocationToSpec", func() {
	var (
		vm      vmopv1.VirtualMachine
		classCS vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		vm = vmopv1.VirtualMachine{}
		classCS = vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		vmopv1util.SyncClassSizeAndAllocationToSpec(&vm, classCS)
	})

	Context("empty classCS — no fields set", func() {
		It("leaves spec untouched", func() {
			Expect(vm.Spec.Resources).To(BeNil())
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced).To(BeNil())
		})
	})

	Context("classCS has full compute configuration including topology and flags", func() {
		BeforeEach(func() {
			classCS = vimtypes.VirtualMachineConfigSpec{
				NumCPUs:           16,
				MemoryMB:          32768,
				NumCoresPerSocket: ptr.To(int32(8)),
				VirtualNuma: &vimtypes.VirtualMachineVirtualNuma{
					CoresPerNumaNode: ptr.To(int32(8)),
				},
				LatencySensitivity: &vimtypes.LatencySensitivity{
					Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
				},
				CpuHotAddEnabled: ptr.To(true),
				NestedHVEnabled:  ptr.To(true),
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(3200)),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(16384)),
				},
			}
		})
		It("syncs only size and allocation — leaves topology and flags untouched", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(16)))
			Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(32768 * 1024 * 1024)))
			Expect(vm.Spec.Resources.Requests.CPU.Value()).To(Equal(int64(3200)))
			Expect(vm.Spec.Resources.Requests.Memory.Value()).To(Equal(int64(16384 * 1024 * 1024)))
			if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
				Expect(vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(BeNil())
				Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(BeNil())
			}
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced).To(BeNil())
		})
	})

	Context("resize from best-effort-xsmall to guaranteed-xsmall (VMResizeCPUMemory path)", func() {
		BeforeEach(func() {
			vm.Spec.Resources = nil

			classCS = vimtypes.VirtualMachineConfigSpec{
				NumCPUs:  2,
				MemoryMB: 2048,
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(2000)),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(2048)),
				},
				MemoryReservationLockedToMax: ptr.To(true),
			}
		})

		It("sets size, full reservations, and memory reservation lock", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(2)))
			Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(2048 * 1024 * 1024)))
			Expect(vm.Spec.Resources.Requests.CPU.Value()).To(Equal(int64(2000)))
			Expect(vm.Spec.Resources.Requests.Memory.Value()).To(Equal(int64(2048 * 1024 * 1024)))
			Expect(vm.Spec.MemoryAdvanced.ReservationLockedToMax).To(Equal(ptr.To(true)))
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced.HotAddEnabled).To(BeNil())
		})
	})

	Context("resize from guaranteed-xsmall to best-effort-xsmall (VMResizeCPUMemory path)", func() {
		BeforeEach(func() {
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{
					CPU:    ptr.To(resource.MustParse("2")),
					Memory: ptr.To(resource.MustParse("2Gi")),
				},
				Requests: &vmopv1.VirtualMachineResourceQuantity{
					CPU:    ptr.To(resource.MustParse("2000")),
					Memory: ptr.To(resource.MustParse("2Gi")),
				},
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}

			classCS = vimtypes.VirtualMachineConfigSpec{
				NumCPUs:  2,
				MemoryMB: 2048,
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(0)),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(0)),
				},
				MemoryReservationLockedToMax: ptr.To(false),
			}
		})

		It("clears CPU and memory reservations and the memory reservation lock", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(2)))
			Expect(vm.Spec.Resources.Requests.CPU).To(BeNil())
			Expect(vm.Spec.Resources.Requests.Memory).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced.ReservationLockedToMax).To(Equal(ptr.To(false)))
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced.HotAddEnabled).To(BeNil())
		})
	})
})
