// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine/backfill"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("ComputeConfigFromMoVM", func() {

	var (
		ctx  = context.Background()
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
	})

	When("moVM.Config is nil", func() {
		BeforeEach(func() {
			moVM.Config = nil
		})
		It("returns false, nil without panicking", func() {
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Resources).To(BeNil())
			Expect(vm.Spec.CPUAdvanced).To(BeNil())
			Expect(vm.Spec.MemoryAdvanced).To(BeNil())
		})
	})

	// ------------------------------------------------------------------ //
	// Group A — spec.resources.size (guest-visible CPU and memory)
	// ------------------------------------------------------------------ //

	Context("Group A — spec.resources.size", func() {

		DescribeTable("size.cpu backfilled from Hardware.NumCPU",
			func(numCPU int32, expectCPU *int64) {
				moVM.Config.Hardware.NumCPU = numCPU
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectCPU == nil {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Size != nil {
						Expect(vm.Spec.Resources.Size.CPU).To(BeNil())
					}
					Expect(mutated).To(BeFalse())
				} else {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size.CPU).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(*expectCPU))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("NumCPU=4 → size.cpu=4", int32(4), ptr.To(int64(4))),
			Entry("NumCPU=1 → size.cpu=1", int32(1), ptr.To(int64(1))),
			Entry("NumCPU=0 → no backfill", int32(0), nil),
		)

		DescribeTable("size.memory backfilled from Hardware.MemoryMB (MiB → bytes)",
			func(memoryMB int32, expectBytes *int64) {
				moVM.Config.Hardware.MemoryMB = memoryMB
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectBytes == nil {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Size != nil {
						Expect(vm.Spec.Resources.Size.Memory).To(BeNil())
					}
					Expect(mutated).To(BeFalse())
				} else {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size.Memory).ToNot(BeNil())
					Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(*expectBytes))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("MemoryMB=8192 → 8 GiB bytes", int32(8192), ptr.To(int64(8192*1024*1024))),
			Entry("MemoryMB=0 → no backfill", int32(0), nil),
		)

		It("spec.resources.size.cpu already set → not overwritten (spec wins)", func() {
			q := resource.MustParse("2")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			moVM.Config.Hardware.NumCPU = 8
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(2)))
		})

		It("spec.resources.size.memory already set → not overwritten (spec wins)", func() {
			q := resource.MustParse("4Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
			moVM.Config.Hardware.MemoryMB = 8192
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(4 * 1024 * 1024 * 1024)))
		})
	})

	// ------------------------------------------------------------------ //
	// Group B — spec.resources.requests and limits (host allocation)
	// ------------------------------------------------------------------ //

	Context("Group B — spec.resources.requests and limits", func() {

		DescribeTable("CPU allocation backfill",
			func(reservation, limit *int64, expectReqMHz, expectLimitMHz *int64) {
				moVM.Config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: reservation,
					Limit:       limit,
				}
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())

				if expectReqMHz != nil {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests.CPU).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests.CPU.Value()).To(Equal(*expectReqMHz))
					Expect(mutated).To(BeTrue())
				} else {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Requests != nil {
						Expect(vm.Spec.Resources.Requests.CPU).To(BeNil())
					}
				}

				if expectLimitMHz != nil {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits.CPU).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits.CPU.Value()).To(Equal(*expectLimitMHz))
					Expect(mutated).To(BeTrue())
				} else {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Limits != nil {
						Expect(vm.Spec.Resources.Limits.CPU).To(BeNil())
					}
				}
			},
			Entry("Reservation=2000 MHz → requests.cpu=2000",
				ptr.To(int64(2000)), nil, ptr.To(int64(2000)), nil),
			Entry("Limit=4000 MHz → limits.cpu=4000",
				nil, ptr.To(int64(4000)), nil, ptr.To(int64(4000))),
			Entry("Reservation=0 → no backfill (default no-reservation)",
				ptr.To(int64(0)), nil, nil, nil),
			Entry("Limit=-1 → no backfill (unlimited)",
				nil, ptr.To(int64(-1)), nil, nil),
			Entry("Limit=0 → no backfill",
				nil, ptr.To(int64(0)), nil, nil),
			Entry("Reservation=1000 and Limit=2000 → both backfilled",
				ptr.To(int64(1000)), ptr.To(int64(2000)),
				ptr.To(int64(1000)), ptr.To(int64(2000))),
		)

		It("CpuAllocation == nil → no CPU requests/limits backfilled", func() {
			moVM.Config.CpuAllocation = nil
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(vm.Spec.Resources).To(BeNil())
		})

		DescribeTable("memory allocation backfill (MiB → bytes)",
			func(reservation, limit *int64, expectReqBytes, expectLimitBytes *int64) {
				moVM.Config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: reservation,
					Limit:       limit,
				}
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())

				if expectReqBytes != nil {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests.Memory).ToNot(BeNil())
					Expect(vm.Spec.Resources.Requests.Memory.Value()).To(Equal(*expectReqBytes))
					Expect(mutated).To(BeTrue())
				} else {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Requests != nil {
						Expect(vm.Spec.Resources.Requests.Memory).To(BeNil())
					}
				}

				if expectLimitBytes != nil {
					Expect(vm.Spec.Resources).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits.Memory).ToNot(BeNil())
					Expect(vm.Spec.Resources.Limits.Memory.Value()).To(Equal(*expectLimitBytes))
					Expect(mutated).To(BeTrue())
				} else {
					if vm.Spec.Resources != nil && vm.Spec.Resources.Limits != nil {
						Expect(vm.Spec.Resources.Limits.Memory).To(BeNil())
					}
				}
			},
			Entry("Reservation=8192 MiB → requests.memory=8 GiB",
				ptr.To(int64(8192)), nil, ptr.To(int64(8192*1024*1024)), nil),
			Entry("Limit=16384 MiB → limits.memory=16 GiB",
				nil, ptr.To(int64(16384)), nil, ptr.To(int64(16384*1024*1024))),
			Entry("Reservation=0 → no backfill",
				ptr.To(int64(0)), nil, nil, nil),
			Entry("Limit=-1 → no backfill (unlimited)",
				nil, ptr.To(int64(-1)), nil, nil),
		)
	})

	// ------------------------------------------------------------------ //
	// Group C — spec.cpuAdvanced.latencySensitivity
	// ------------------------------------------------------------------ //

	Context("Group C — spec.cpuAdvanced.latencySensitivity", func() {

		DescribeTable("LatencySensitivity level mapping",
			func(level vimtypes.LatencySensitivitySensitivityLevel, threads int32,
				expectLevel *vmopv1.VirtualMachineLatencySensitivityLevel) {

				moVM.Config.LatencySensitivity = &vimtypes.LatencySensitivity{Level: level}
				moVM.Config.Hardware.SimultaneousThreads = threads
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectLevel == nil {
					if vm.Spec.CPUAdvanced != nil {
						Expect(vm.Spec.CPUAdvanced.LatencySensitivity).To(BeNil())
					}
					Expect(mutated).To(BeFalse())
				} else {
					Expect(vm.Spec.CPUAdvanced).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.LatencySensitivity).ToNot(BeNil())
					Expect(*vm.Spec.CPUAdvanced.LatencySensitivity).To(Equal(*expectLevel))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("High + SimultaneousThreads=2 → HighWithHyperthreading",
				vimtypes.LatencySensitivitySensitivityLevelHigh, int32(2),
				ptr.To(vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading)),
			Entry("High + SimultaneousThreads=1 → High",
				vimtypes.LatencySensitivitySensitivityLevelHigh, int32(1),
				ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh)),
			Entry("High + SimultaneousThreads=0 → High (0 means single-threaded)",
				vimtypes.LatencySensitivitySensitivityLevelHigh, int32(0),
				ptr.To(vmopv1.VirtualMachineLatencySensitivityHigh)),
			Entry("Normal → Normal",
				vimtypes.LatencySensitivitySensitivityLevelNormal, int32(0),
				ptr.To(vmopv1.VirtualMachineLatencySensitivityNormal)),
			Entry("Low → no backfill",
				vimtypes.LatencySensitivitySensitivityLevelLow, int32(0), nil),
		)

		It("LatencySensitivity == nil → spec field stays nil", func() {
			moVM.Config.LatencySensitivity = nil
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			if vm.Spec.CPUAdvanced != nil {
				Expect(vm.Spec.CPUAdvanced.LatencySensitivity).To(BeNil())
			}
		})

		It("spec.cpuAdvanced.latencySensitivity already set → not overwritten (spec wins)", func() {
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				LatencySensitivity: ptr.To(vmopv1.VirtualMachineLatencySensitivityNormal),
			}
			moVM.Config.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
			}
			moVM.Config.Hardware.SimultaneousThreads = 2
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(*vm.Spec.CPUAdvanced.LatencySensitivity).To(Equal(vmopv1.VirtualMachineLatencySensitivityNormal))
		})
	})

	// ------------------------------------------------------------------ //
	// Group D — spec.cpuAdvanced.topology
	// ------------------------------------------------------------------ //

	Context("Group D — spec.cpuAdvanced.topology", func() {

		DescribeTable("coresPerSocket backfill from Hardware.NumCoresPerSocket",
			func(numCoresPerSocket *int32, expectCPS *int32) {
				moVM.Config.Hardware.NumCoresPerSocket = numCoresPerSocket
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectCPS == nil {
					if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
						Expect(vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(BeNil())
					}
					Expect(mutated).To(BeFalse())
				} else {
					Expect(vm.Spec.CPUAdvanced).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology.CoresPerSocket).ToNot(BeNil())
					Expect(*vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(Equal(*expectCPS))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("NumCoresPerSocket=4 → coresPerSocket=4", ptr.To(int32(4)), ptr.To(int32(4))),
			Entry("NumCoresPerSocket=1 → coresPerSocket=1", ptr.To(int32(1)), ptr.To(int32(1))),
			Entry("NumCoresPerSocket=0 → no backfill", ptr.To(int32(0)), nil),
			Entry("NumCoresPerSocket=nil → no backfill", nil, nil),
		)

		DescribeTable("vnumaNodeCount derived from NumaInfo.CoresPerNumaNode",
			func(numCPU int32, autoCores *bool, coresPerNode *int32, expectNodeCount *int32) {
				moVM.Config.Hardware.NumCPU = numCPU
				moVM.Config.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
					AutoCoresPerNumaNode: autoCores,
					CoresPerNumaNode:     coresPerNode,
				}
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectNodeCount == nil {
					if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
						Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(BeNil())
					}
				} else {
					Expect(vm.Spec.CPUAdvanced).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).ToNot(BeNil())
					Expect(*vm.Spec.CPUAdvanced.Topology.VNUMANodeCount).To(Equal(*expectNodeCount))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("AutoCores=nil, NumCPU=8, CoresPerNode=2 → vnumaNodeCount=4",
				int32(8), nil, ptr.To(int32(2)), ptr.To(int32(4))),
			Entry("AutoCores=false, NumCPU=8, CoresPerNode=2 → vnumaNodeCount=4",
				int32(8), ptr.To(false), ptr.To(int32(2)), ptr.To(int32(4))),
			Entry("AutoCores=true → no backfill (auto mode)",
				int32(8), ptr.To(true), ptr.To(int32(2)), nil),
			Entry("NumCPU=7, CoresPerNode=2 → no backfill (uneven division)",
				int32(7), nil, ptr.To(int32(2)), nil),
			Entry("CoresPerNode=0 → no backfill",
				int32(8), nil, ptr.To(int32(0)), nil),
			Entry("NumCPU=0 → no backfill",
				int32(0), nil, ptr.To(int32(2)), nil),
		)

		DescribeTable("exposeVnumaOnCpuHotadd backfill from NumaInfo.VnumaOnCpuHotaddExposed",
			func(exposed *bool, expectExposed *bool) {
				moVM.Config.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
					VnumaOnCpuHotaddExposed: exposed,
				}
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if expectExposed == nil {
					if vm.Spec.CPUAdvanced != nil && vm.Spec.CPUAdvanced.Topology != nil {
						Expect(vm.Spec.CPUAdvanced.Topology.ExposeVNUMAOnCPUHotAdd).To(BeNil())
					}
					Expect(mutated).To(BeFalse())
				} else {
					Expect(vm.Spec.CPUAdvanced).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology).ToNot(BeNil())
					Expect(vm.Spec.CPUAdvanced.Topology.ExposeVNUMAOnCPUHotAdd).ToNot(BeNil())
					Expect(*vm.Spec.CPUAdvanced.Topology.ExposeVNUMAOnCPUHotAdd).To(Equal(*expectExposed))
					Expect(mutated).To(BeTrue())
				}
			},
			Entry("VnumaOnCpuHotaddExposed=true → exposeVnumaOnCpuHotadd=true",
				ptr.To(true), ptr.To(true)),
			Entry("VnumaOnCpuHotaddExposed=false → no backfill (false equals unset default)",
				ptr.To(false), nil),
			Entry("VnumaOnCpuHotaddExposed=nil → no backfill",
				nil, nil),
		)

		It("NumaInfo == nil → no NumaInfo-derived topology fields set", func() {
			moVM.Config.NumaInfo = nil
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
		})

		It("spec.cpuAdvanced.topology.coresPerSocket already set → not overwritten (spec wins)", func() {
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: ptr.To(int32(2)),
				},
			}
			moVM.Config.Hardware.NumCoresPerSocket = ptr.To(int32(8))
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
			Expect(*vm.Spec.CPUAdvanced.Topology.CoresPerSocket).To(Equal(int32(2)))
		})
	})

	// ------------------------------------------------------------------ //
	// Group E — spec.cpuAdvanced boolean flags (only true is backfilled)
	// ------------------------------------------------------------------ //

	Context("Group E — spec.cpuAdvanced boolean flags", func() {

		DescribeTable("only true is backfilled; false and nil are skipped",
			func(setup func(*vimtypes.VirtualMachineConfigInfo),
				checkSpec func(*vmopv1.VirtualMachineCPUAdvancedSpec)) {

				setup(moVM.Config)
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if checkSpec == nil {
					Expect(mutated).To(BeFalse())
					return
				}
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.CPUAdvanced).ToNot(BeNil())
				checkSpec(vm.Spec.CPUAdvanced)
			},
			Entry("CpuHotAddEnabled=true → hotAddEnabled=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.CpuHotAddEnabled = ptr.To(true) },
				func(s *vmopv1.VirtualMachineCPUAdvancedSpec) {
					Expect(s.HotAddEnabled).To(HaveValue(BeTrue()))
				},
			),
			Entry("CpuHotAddEnabled=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.CpuHotAddEnabled = ptr.To(false) },
				nil,
			),
			Entry("CpuHotAddEnabled=nil → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.CpuHotAddEnabled = nil },
				nil,
			),
			Entry("Flags.VvtdEnabled=true → iommuEnabled=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.Flags.VvtdEnabled = ptr.To(true) },
				func(s *vmopv1.VirtualMachineCPUAdvancedSpec) {
					Expect(s.IOMMUEnabled).To(HaveValue(BeTrue()))
				},
			),
			Entry("Flags.VvtdEnabled=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.Flags.VvtdEnabled = ptr.To(false) },
				nil,
			),
			Entry("NestedHVEnabled=true → nestedHardwareVirtualizationEnabled=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.NestedHVEnabled = ptr.To(true) },
				func(s *vmopv1.VirtualMachineCPUAdvancedSpec) {
					Expect(s.NestedHardwareVirtualizationEnabled).To(HaveValue(BeTrue()))
				},
			),
			Entry("NestedHVEnabled=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.NestedHVEnabled = ptr.To(false) },
				nil,
			),
			Entry("VPMCEnabled=true → performanceCountersEnabled=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.VPMCEnabled = ptr.To(true) },
				func(s *vmopv1.VirtualMachineCPUAdvancedSpec) {
					Expect(s.PerformanceCountersEnabled).To(HaveValue(BeTrue()))
				},
			),
			Entry("VPMCEnabled=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.VPMCEnabled = ptr.To(false) },
				nil,
			),
		)
	})

	// ------------------------------------------------------------------ //
	// Group F — spec.memoryAdvanced boolean flags (only true is backfilled)
	// ------------------------------------------------------------------ //

	Context("Group F — spec.memoryAdvanced boolean flags", func() {

		DescribeTable("only true is backfilled; false and nil are skipped",
			func(setup func(*vimtypes.VirtualMachineConfigInfo),
				checkSpec func(*vmopv1.VirtualMachineMemoryAdvancedSpec)) {

				setup(moVM.Config)
				mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
				Expect(err).ToNot(HaveOccurred())
				if checkSpec == nil {
					Expect(mutated).To(BeFalse())
					return
				}
				Expect(mutated).To(BeTrue())
				Expect(vm.Spec.MemoryAdvanced).ToNot(BeNil())
				checkSpec(vm.Spec.MemoryAdvanced)
			},
			Entry("MemoryHotAddEnabled=true → hotAddEnabled=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.MemoryHotAddEnabled = ptr.To(true) },
				func(s *vmopv1.VirtualMachineMemoryAdvancedSpec) {
					Expect(s.HotAddEnabled).To(HaveValue(BeTrue()))
				},
			),
			Entry("MemoryHotAddEnabled=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.MemoryHotAddEnabled = ptr.To(false) },
				nil,
			),
			Entry("MemoryReservationLockedToMax=true → reservationLockedToMax=true",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.MemoryReservationLockedToMax = ptr.To(true) },
				func(s *vmopv1.VirtualMachineMemoryAdvancedSpec) {
					Expect(s.ReservationLockedToMax).To(HaveValue(BeTrue()))
				},
			),
			Entry("MemoryReservationLockedToMax=false → no backfill",
				func(ci *vimtypes.VirtualMachineConfigInfo) { ci.MemoryReservationLockedToMax = ptr.To(false) },
				nil,
			),
		)
	})

	// ------------------------------------------------------------------ //
	// Mutation tracking
	// ------------------------------------------------------------------ //

	Context("mutation tracking", func() {
		It("no fields to backfill → mutated=false", func() {
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
		})

		It("at least one field backfilled → mutated=true", func() {
			moVM.Config.Hardware.NumCPU = 4
			mutated, err := backfill.ComputeConfigFromMoVM(ctx, vm, moVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeTrue())
		})
	})
})
