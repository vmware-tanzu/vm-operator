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

var _ = Describe("SyncClassComputeToSpec", func() {
	var (
		vm      vmopv1.VirtualMachine
		classCS vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		vm = vmopv1.VirtualMachine{}
		classCS = vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		vmopv1util.SyncClassComputeToSpec(&vm, classCS)
	})

	Context("empty classCS — no fields set", func() {
		It("leaves spec untouched", func() {
			Expect(vm.Spec.Resources).To(BeNil())
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced).To(BeNil())
		})
	})

	Context("spec already has cpu=8, classCS.NumCPUs=16 (class overrides stale backfilled value)", func() {
		BeforeEach(func() {
			q := resource.MustParse("8")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			classCS.NumCPUs = 16
		})
		It("overwrites spec.resources.size.cpu with 16", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(16)))
		})
	})

	Context("classCS.CpuAllocation.Limit = -1 (unlimited) — no pre-existing user limit", func() {
		BeforeEach(func() {
			classCS.CpuAllocation = &vimtypes.ResourceAllocationInfo{
				Limit: ptr.To(int64(-1)),
			}
		})
		It("leaves spec.resources.limits nil (nothing to clear)", func() {
			if vm.Spec.Resources != nil {
				Expect(vm.Spec.Resources.Limits).To(BeNil())
			}
		})
	})

	Context("classCS.CpuAllocation.Limit = -1 (unlimited) — vm has pre-existing user cpu limit", func() {
		BeforeEach(func() {
			q := resource.MustParse("7000")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Limits: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			classCS.CpuAllocation = &vimtypes.ResourceAllocationInfo{
				Limit: ptr.To(int64(-1)),
			}
		})
		It("clears spec.resources.limits.cpu (class says unlimited — override user cap)", func() {
			Expect(vm.Spec.Resources).NotTo(BeNil())
			Expect(vm.Spec.Resources.Limits).NotTo(BeNil())
			Expect(vm.Spec.Resources.Limits.CPU).To(BeNil())
		})
	})

	Context("classCS.MemoryAllocation.Limit = -1 (unlimited) — no pre-existing user limit", func() {
		BeforeEach(func() {
			classCS.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Limit: ptr.To(int64(-1)),
			}
		})
		It("leaves spec.resources.limits nil (nothing to clear)", func() {
			if vm.Spec.Resources != nil {
				Expect(vm.Spec.Resources.Limits).To(BeNil())
			}
		})
	})

	Context("classCS.MemoryAllocation.Limit = -1 (unlimited) — vm has pre-existing user memory limit", func() {
		BeforeEach(func() {
			q := resource.MustParse("4Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Limits: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
			classCS.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Limit: ptr.To(int64(-1)),
			}
		})
		It("clears spec.resources.limits.memory (class says unlimited — override user cap)", func() {
			Expect(vm.Spec.Resources).NotTo(BeNil())
			Expect(vm.Spec.Resources.Limits).NotTo(BeNil())
			Expect(vm.Spec.Resources.Limits.Memory).To(BeNil())
		})
	})

	Context("classCS.NumCoresPerSocket = 0 (auto sentinel)", func() {
		BeforeEach(func() {
			classCS.NumCoresPerSocket = ptr.To(int32(0))
		})
		It("sets spec.cpuAdvanced.topology.coresPerSocket = 0 (auto sentinel)", func() {
			Expect(vm.Spec.CPUAdvanced).NotTo(BeNil())
			Expect(vm.Spec.CPUAdvanced.Topology).NotTo(BeNil())
			Expect(vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(Equal(ptr.To(int32(0))))
		})
	})

	Context("classCS VirtualNuma set but NumCPUs = 0 (unknown) — cannot derive nodeCount", func() {
		BeforeEach(func() {
			classCS.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{
				CoresPerNumaNode: ptr.To(int32(4)),
			}
		})
		It("does not set spec vnumaNodeCount", func() {
			if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
				Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(BeNil())
			}
		})
	})

	Context("classCS VirtualNuma.CoresPerNumaNode = nil", func() {
		BeforeEach(func() {
			classCS.NumCPUs = 16
			classCS.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{}
		})
		It("does not set spec vnumaNodeCount", func() {
			if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
				Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(BeNil())
			}
		})
	})

	Context("classCS VirtualNuma.CoresPerNumaNode = 0 (auto sentinel — guard skips derivation)", func() {
		BeforeEach(func() {
			classCS.NumCPUs = 16
			classCS.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{
				CoresPerNumaNode: ptr.To(int32(0)),
			}
		})
		It("does not set spec vnumaNodeCount", func() {
			if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
				Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(BeNil())
			}
		})
	})

	Context("classCS LatencySensitivity = High + SimultaneousThreads = 2", func() {
		BeforeEach(func() {
			classCS.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
			}
			classCS.SimultaneousThreads = 2
		})
		It("sets spec.cpuAdvanced.latencySensitivity = HighWithHyperthreading", func() {
			Expect(vm.Spec.CPUAdvanced.LatencySensitivity).To(
				Equal(ptr.To(vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading)))
		})
	})

	Context("classCS LatencySensitivity = Low (not mapped)", func() {
		BeforeEach(func() {
			classCS.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelLow,
			}
		})
		It("does not set spec latencySensitivity", func() {
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
		})
	})

	Context("full class ConfigSpec with all compute fields set", func() {
		BeforeEach(func() {
			classCS = vimtypes.VirtualMachineConfigSpec{
				NumCPUs:  16,
				MemoryMB: 32768,
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(3200)),
					Limit:       ptr.To(int64(6400)),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(16384)),
				},
				NumCoresPerSocket: ptr.To(int32(8)),
				VirtualNuma: &vimtypes.VirtualMachineVirtualNuma{
					CoresPerNumaNode: ptr.To(int32(8)),
				},
				LatencySensitivity: &vimtypes.LatencySensitivity{
					Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
				},
				CpuHotAddEnabled: ptr.To(true),
				Flags: &vimtypes.VirtualMachineFlagInfo{
					VvtdEnabled: ptr.To(true),
				},
				NestedHVEnabled:              ptr.To(true),
				VPMCEnabled:                  ptr.To(true),
				MemoryHotAddEnabled:          ptr.To(true),
				MemoryReservationLockedToMax: ptr.To(true),
			}
		})
		It("syncs all fields into spec", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(16)))
			Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(32768 * 1024 * 1024)))
			Expect(vm.Spec.Resources.Requests.CPU.Value()).To(Equal(int64(3200)))
			Expect(vm.Spec.Resources.Limits.CPU.Value()).To(Equal(int64(6400)))
			Expect(vm.Spec.Resources.Requests.Memory.Value()).To(Equal(int64(16384 * 1024 * 1024)))
			Expect(vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(Equal(ptr.To(int32(8))))
			Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(Equal(ptr.To(int32(2)))) // 16/8
			Expect(*vm.Spec.CPUAdvanced.LatencySensitivity).To(Equal(vmopv1.VirtualMachineLatencySensitivityHigh))
			Expect(vm.Spec.CPUAdvanced.HotAddEnabled).To(Equal(ptr.To(true)))
			Expect(vm.Spec.CPUAdvanced.IOMMUEnabled).To(Equal(ptr.To(true)))
			Expect(vm.Spec.CPUAdvanced.NestedHardwareVirtualizationEnabled).To(Equal(ptr.To(true)))
			Expect(vm.Spec.CPUAdvanced.PerformanceCountersEnabled).To(Equal(ptr.To(true)))
			Expect(vm.Spec.MemoryAdvanced.HotAddEnabled).To(Equal(ptr.To(true)))
			Expect(vm.Spec.MemoryAdvanced.ReservationLockedToMax).To(Equal(ptr.To(true)))
		})
	})

	Context("resize from best-effort-xsmall to guaranteed-xsmall", func() {
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
				LatencySensitivity: &vimtypes.LatencySensitivity{
					Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
				},
				MemoryReservationLockedToMax: ptr.To(true),
			}
		})

		It("sets size, full reservations, latency sensitivity, and memory locked-to-max", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(2)))
			Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(2048 * 1024 * 1024)))
			Expect(vm.Spec.Resources.Requests.CPU.Value()).To(Equal(int64(2000)))
			Expect(vm.Spec.Resources.Requests.Memory.Value()).To(Equal(int64(2048 * 1024 * 1024)))
			Expect(*vm.Spec.CPUAdvanced.LatencySensitivity).To(Equal(vmopv1.VirtualMachineLatencySensitivityHigh))
			Expect(vm.Spec.MemoryAdvanced.ReservationLockedToMax).To(Equal(ptr.To(true)))
		})
	})

	Context("resize from guaranteed-xsmall to best-effort-xsmall", func() {
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
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh),
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
				LatencySensitivity: &vimtypes.LatencySensitivity{
					Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
				},
				MemoryReservationLockedToMax: ptr.To(false),
			}
		})

		It("clears reservations and resets latency and memory lock", func() {
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(2)))
			Expect(vm.Spec.Resources.Requests.CPU).To(BeNil())
			Expect(vm.Spec.Resources.Requests.Memory).To(BeNil())
			Expect(*vm.Spec.CPUAdvanced.LatencySensitivity).To(Equal(vmopv1.VirtualMachineLatencySensitivityNormal))
			Expect(vm.Spec.MemoryAdvanced.ReservationLockedToMax).To(Equal(ptr.To(false)))
		})
	})
})

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
