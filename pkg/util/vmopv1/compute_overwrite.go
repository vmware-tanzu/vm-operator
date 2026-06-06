// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// computeFieldDef describes one logical compute field group.
//
// hotPluggable evaluates at runtime whether the field can be reconfigured
// while the VM is powered on, given both the desired spec and the live config
// (e.g. CPU size is hot-addable only when CpuHotAddEnabled is set and the
// desired count is higher than the live count).
type computeFieldDef struct {
	// fieldName is the spec path shown in condition messages.
	fieldName string
	// minHWVer is the minimum hardware version required; 0 means no gate.
	minHWVer vimtypes.HardwareVersion
	// hotPluggable reports whether the field can be changed while powered on.
	hotPluggable func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo) bool
	// differs returns true when a write to cs is needed: either the desired
	// spec value differs from the live config (ci), or a class-derived value
	// already written in cs differs from what spec wants (spec overrides class).
	// cs may be partially populated by fields that ran earlier in the loop.
	differs func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool
	// apply writes the desired value from vm.Spec to cs.
	// ci is provided for fields (e.g. vnumaNodeCount) that need the live config
	// as a fallback when cs does not yet carry the required context.
	apply func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec)
}

// alwaysHotPluggable and neverHotPluggable are convenience sentinels for the
// hotPluggable function field.
var (
	alwaysHotPluggable = func(_ vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo) bool {
		return true
	}
	neverHotPluggable = func(_ vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo) bool {
		return false
	}
)

// computeFields returns the ordered list of compute fields to reconcile.
// Order matters: cpuSizeField is listed before vnumaNodeCountField so that
// when vnumaNodeCountField.differs/apply run, cs.NumCPUs already reflects any
// spec- or class-driven CPU change from the current reconcile cycle.
func computeFields(hwVer vimtypes.HardwareVersion) []computeFieldDef {
	return []computeFieldDef{
		cpuSizeField(),
		memorySizeField(),
		cpuAllocationField(),
		memoryAllocationField(),
		latencySensitivityField(),
		coresPerSocketField(hwVer),
		vnumaNodeCountField(),
		cpuHotAddFlagField(),
		iommuField(),
		nestedHVField(),
		perfCountersField(),
		memHotAddFlagField(),
		memReservationLockedField(),
	}
}

// OverwriteSpecComputeConfig applies compute fields from vm.Spec to cs.
//
// poweredOn=false (powered-off): applies all hw-compatible fields; blockedPowerOff
// is always empty since all changes can take effect after a power-off reconfigure.
//
// poweredOn=true (powered-on): applies only currently hot-pluggable fields
// (diff-only — unchanged fields not written). Power-off-required fields that
// differ from liveCI are returned in blockedPowerOff so callers can surface
// them via the VirtualMachineConditionComputeConfigSynced condition.
//
// blockedHW lists fields skipped because the VM's hardware version (derived
// from liveCI.Version) is below the minimum required (e.g.
// "cpuAdvanced.hotAddEnabled (requires hwVer >= 11)").
func OverwriteSpecComputeConfig(
	vm vmopv1.VirtualMachine,
	liveCI vimtypes.VirtualMachineConfigInfo,
	poweredOn bool,
	cs *vimtypes.VirtualMachineConfigSpec) (blockedHW, blockedPowerOff []string) {

	hwVer, _ := vimtypes.ParseHardwareVersion(liveCI.Version)
	for _, f := range computeFields(hwVer) {
		// Skip immediately if no change is needed: spec already matches live
		// config and no class-derived value in cs needs to be overridden.
		if !f.differs(vm, liveCI, cs) {
			continue
		}

		// Hardware version gate: field requires a minimum hw version not met.
		// vSphere silently ignores fields it cannot apply for the current
		// hardware version, so we skip writing entirely and surface the block.
		if f.minHWVer > 0 && hwVer < f.minHWVer {
			blockedHW = append(blockedHW,
				fmt.Sprintf("%s (requires hwVer >= %d)", f.fieldName, f.minHWVer))
			continue
		}

		if poweredOn && !f.hotPluggable(vm, liveCI) {
			// Powered-on: cannot apply this field right now.
			blockedPowerOff = append(blockedPowerOff, f.fieldName)
			continue
		}

		f.apply(vm, liveCI, cs)
	}
	return blockedHW, blockedPowerOff
}

// ─────────────────────────────────────────────────────────────────────────────
// Field definitions
// ─────────────────────────────────────────────────────────────────────────────

func cpuSizeField() computeFieldDef {
	return computeFieldDef{
		fieldName: "resources.size.cpu",
		hotPluggable: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo) bool {
			if !ptr.DerefWithDefault(ci.CpuHotAddEnabled, false) {
				return false
			}
			if res := vm.Spec.Resources; res != nil && res.Size != nil && res.Size.CPU != nil {
				return int32(res.Size.CPU.Value()) > ci.Hardware.NumCPU
			}
			return false
		},
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			res := vm.Spec.Resources
			if res == nil || res.Size == nil || res.Size.CPU == nil {
				return false
			}
			specCPU := int32(res.Size.CPU.Value()) //nolint:gosec
			return specCPU != ci.Hardware.NumCPU ||
				(cs.NumCPUs != 0 && specCPU != cs.NumCPUs)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			if res := vm.Spec.Resources; res != nil && res.Size != nil && res.Size.CPU != nil {
				cs.NumCPUs = int32(res.Size.CPU.Value()) //nolint:gosec
			}
		},
	}
}

func memorySizeField() computeFieldDef {
	return computeFieldDef{
		fieldName: "resources.size.memory",
		hotPluggable: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo) bool {
			if !ptr.DerefWithDefault(ci.MemoryHotAddEnabled, false) {
				return false
			}
			if res := vm.Spec.Resources; res != nil && res.Size != nil && res.Size.Memory != nil {
				return res.Size.Memory.Value()/1024/1024 > int64(ci.Hardware.MemoryMB)
			}
			return false
		},
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			res := vm.Spec.Resources
			if res == nil || res.Size == nil || res.Size.Memory == nil {
				return false
			}
			specMB := res.Size.Memory.Value() / 1024 / 1024
			return specMB != int64(ci.Hardware.MemoryMB) ||
				(cs.MemoryMB != 0 && specMB != cs.MemoryMB)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			if res := vm.Spec.Resources; res != nil && res.Size != nil && res.Size.Memory != nil {
				cs.MemoryMB = res.Size.Memory.Value() / 1024 / 1024
			}
		},
	}
}

func cpuAllocationField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "resources.allocation.cpu",
		hotPluggable: alwaysHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desiredRes, desiredLim := desiredCPUAllocation(vm)
			var liveRes, liveLim int64
			if ci.CpuAllocation != nil {
				liveRes = ptr.DerefWithDefault(ci.CpuAllocation.Reservation, 0)
				liveLim = ptr.DerefWithDefault(ci.CpuAllocation.Limit, -1)
			} else {
				liveLim = -1
			}
			if desiredRes != liveRes || desiredLim != liveLim {
				return true
			}
			if cs.CpuAllocation == nil {
				return false
			}
			csRes := ptr.DerefWithDefault(cs.CpuAllocation.Reservation, int64(0))
			csLim := ptr.DerefWithDefault(cs.CpuAllocation.Limit, int64(-1))
			return desiredRes != csRes || desiredLim != csLim
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			res, lim := desiredCPUAllocation(vm)
			if cs.CpuAllocation == nil {
				cs.CpuAllocation = &vimtypes.ResourceAllocationInfo{}
			}
			cs.CpuAllocation.Reservation = ptr.To(res)
			cs.CpuAllocation.Limit = ptr.To(lim)
		},
	}
}

func memoryAllocationField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "resources.allocation.memory",
		hotPluggable: alwaysHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desiredRes, desiredLim := desiredMemoryAllocation(vm)
			var liveRes, liveLim int64
			if ci.MemoryAllocation != nil {
				liveRes = ptr.DerefWithDefault(ci.MemoryAllocation.Reservation, 0)
				liveLim = ptr.DerefWithDefault(ci.MemoryAllocation.Limit, -1)
			} else {
				liveLim = -1
			}
			if desiredRes != liveRes || desiredLim != liveLim {
				return true
			}
			if cs.MemoryAllocation == nil {
				return false
			}
			csRes := ptr.DerefWithDefault(cs.MemoryAllocation.Reservation, int64(0))
			csLim := ptr.DerefWithDefault(cs.MemoryAllocation.Limit, int64(-1))
			return desiredRes != csRes || desiredLim != csLim
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			res, lim := desiredMemoryAllocation(vm)
			if cs.MemoryAllocation == nil {
				cs.MemoryAllocation = &vimtypes.ResourceAllocationInfo{}
			}
			cs.MemoryAllocation.Reservation = ptr.To(res)
			cs.MemoryAllocation.Limit = ptr.To(lim)
		},
	}
}

func latencySensitivityField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.latencySensitivity",
		hotPluggable: alwaysHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desiredLevel, desiredThreads := desiredLatencySensitivity(vm)
			var liveLevel vimtypes.LatencySensitivitySensitivityLevel
			if ci.LatencySensitivity != nil {
				liveLevel = ci.LatencySensitivity.Level
			} else {
				liveLevel = vimtypes.LatencySensitivitySensitivityLevelNormal
			}
			if desiredLevel != liveLevel {
				return true
			}
			if desiredThreads > 0 && ci.Hardware.SimultaneousThreads != desiredThreads {
				return true
			}
			return cs.LatencySensitivity != nil && desiredLevel != cs.LatencySensitivity.Level
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			level, threads := desiredLatencySensitivity(vm)
			cs.LatencySensitivity = &vimtypes.LatencySensitivity{Level: level}
			if threads > 0 {
				cs.SimultaneousThreads = threads
			}
		},
	}
}

func coresPerSocketField(hwVer vimtypes.HardwareVersion) computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.topology.coresPerSocket",
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			topo := topologySpec(vm)
			if topo != nil && topo.CoresPerSocket != nil && *topo.CoresPerSocket > 0 {
				// Explicit spec: differs when live doesn't match.
				live := ptr.DerefWithDefault(ci.Hardware.NumCoresPerSocket, int32(1))
				if *topo.CoresPerSocket != live {
					return true
				}
				return cs.NumCoresPerSocket != nil && *topo.CoresPerSocket != *cs.NumCoresPerSocket
			}
			// Nil or 0 spec: reset to auto/default — semantics vary by hardware version.
			// On vmx-20+, NumCoresPerSocket=0 enables auto mode.
			// On pre-vmx-20, 0 is rejected; the only reset value is 1.
			var resetVal int32
			switch {
			case hwVer >= vimtypes.VMX20:
				if ci.Hardware.AutoCoresPerSocket == nil || !*ci.Hardware.AutoCoresPerSocket {
					return true
				}
			case ptr.DerefWithDefault(ci.Hardware.NumCoresPerSocket, int32(1)) > 1:
				return true
			default:
				resetVal = 1
			}
			return cs.NumCoresPerSocket != nil && *cs.NumCoresPerSocket != resetVal
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			topo := topologySpec(vm)
			if topo != nil && topo.CoresPerSocket != nil && *topo.CoresPerSocket > 0 {
				cs.NumCoresPerSocket = topo.CoresPerSocket
				return
			}
			// Nil or 0 spec: reset to auto/default.
			if hwVer >= vimtypes.VMX20 {
				// Writing 0 enables automatic socket sizing on vmx-20+.
				cs.NumCoresPerSocket = ptr.To(int32(0))
			} else {
				// 0 is rejected by vSphere on pre-vmx-20; write 1 (the minimum).
				cs.NumCoresPerSocket = ptr.To(int32(1))
			}
		},
	}
}

func vnumaNodeCountField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.topology.vnumaNodeCount",
		minHWVer:     vimtypes.VMX20,
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			// Resolve post-reconcile CPU count: cs.NumCPUs is already updated by
			// cpuSizeField (which runs first) if spec or class changed the CPU count.
			numCPUs := cs.NumCPUs
			if numCPUs == 0 {
				numCPUs = ci.Hardware.NumCPU
			}
			topo := topologySpec(vm)
			if topo != nil && topo.VNUMANodeCount != nil && *topo.VNUMANodeCount > 0 {
				nodeCount := *topo.VNUMANodeCount
				if numCPUs > 0 {
					desiredCoresPerNode := numCPUs / nodeCount
					// Check vs live.
					var liveCoresPerNode int32
					if ci.NumaInfo != nil {
						liveCoresPerNode = ptr.DerefWithDefault(ci.NumaInfo.CoresPerNumaNode, 0)
					}
					if desiredCoresPerNode != liveCoresPerNode {
						return true
					}
					// Check vs class cs.
					if cs.VirtualNuma != nil && cs.VirtualNuma.CoresPerNumaNode != nil {
						return desiredCoresPerNode != *cs.VirtualNuma.CoresPerNumaNode
					}
					return false
				}
				return true
			}
			// Nil or 0 spec: desired is auto. Differs if live or class is not in auto mode.
			if ci.NumaInfo != nil && (ci.NumaInfo.AutoCoresPerNumaNode == nil || !*ci.NumaInfo.AutoCoresPerNumaNode) {
				return true
			}
			return cs.VirtualNuma != nil && cs.VirtualNuma.CoresPerNumaNode != nil &&
				*cs.VirtualNuma.CoresPerNumaNode != 0
		},
		apply: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			topo := topologySpec(vm)
			if topo != nil && topo.VNUMANodeCount != nil && *topo.VNUMANodeCount > 0 {
				nodeCount := *topo.VNUMANodeCount
				numCPUs := cs.NumCPUs
				if numCPUs == 0 {
					numCPUs = ci.Hardware.NumCPU
				}
				if numCPUs > 0 {
					if cs.VirtualNuma == nil {
						cs.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{}
					}
					cs.VirtualNuma.CoresPerNumaNode = ptr.To(numCPUs / nodeCount)
				}
				return
			}
			// Nil or 0 spec: write 0 to enable automatic vNUMA node sizing.
			if cs.VirtualNuma == nil {
				cs.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{}
			}
			cs.VirtualNuma.CoresPerNumaNode = ptr.To(int32(0))
		},
	}
}

func cpuHotAddFlagField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.hotAddEnabled",
		minHWVer:     vimtypes.VMX11,
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredCPUHotAddEnabled(vm)
			return desired != ptr.DerefWithDefault(ci.CpuHotAddEnabled, false) ||
				(cs.CpuHotAddEnabled != nil && desired != *cs.CpuHotAddEnabled)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			cs.CpuHotAddEnabled = desiredCPUHotAddEnabled(vm)
		},
	}
}

func iommuField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.iommuEnabled",
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredIOMMU(vm)
			liveVvtd := ptr.DerefWithDefault(ci.Flags.VvtdEnabled, false)
			if desired != liveVvtd {
				return true
			}
			return cs.Flags != nil && cs.Flags.VvtdEnabled != nil && desired != *cs.Flags.VvtdEnabled
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			if cs.Flags == nil {
				cs.Flags = &vimtypes.VirtualMachineFlagInfo{}
			}
			cs.Flags.VvtdEnabled = desiredIOMMU(vm)
		},
	}
}

func nestedHVField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.nestedHardwareVirtualizationEnabled",
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredNestedHV(vm)
			return desired != ptr.DerefWithDefault(ci.NestedHVEnabled, false) ||
				(cs.NestedHVEnabled != nil && desired != *cs.NestedHVEnabled)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			cs.NestedHVEnabled = desiredNestedHV(vm)
		},
	}
}

func perfCountersField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "cpuAdvanced.performanceCountersEnabled",
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredPerfCounters(vm)
			return desired != ptr.DerefWithDefault(ci.VPMCEnabled, false) ||
				(cs.VPMCEnabled != nil && desired != *cs.VPMCEnabled)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			cs.VPMCEnabled = desiredPerfCounters(vm)
		},
	}
}

func memHotAddFlagField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "memoryAdvanced.hotAddEnabled",
		minHWVer:     vimtypes.VMX7,
		hotPluggable: neverHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredMemHotAdd(vm)
			return desired != ptr.DerefWithDefault(ci.MemoryHotAddEnabled, false) ||
				(cs.MemoryHotAddEnabled != nil && desired != *cs.MemoryHotAddEnabled)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			cs.MemoryHotAddEnabled = desiredMemHotAdd(vm)
		},
	}
}

func memReservationLockedField() computeFieldDef {
	return computeFieldDef{
		fieldName:    "memoryAdvanced.reservationLockedToMax",
		hotPluggable: alwaysHotPluggable,
		differs: func(vm vmopv1.VirtualMachine, ci vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) bool {
			desired := *desiredMemReservationLocked(vm)
			return desired != ptr.DerefWithDefault(ci.MemoryReservationLockedToMax, false) ||
				(cs.MemoryReservationLockedToMax != nil && desired != *cs.MemoryReservationLockedToMax)
		},
		apply: func(vm vmopv1.VirtualMachine, _ vimtypes.VirtualMachineConfigInfo, cs *vimtypes.VirtualMachineConfigSpec) {
			cs.MemoryReservationLockedToMax = desiredMemReservationLocked(vm)
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Desired-value helpers
// ─────────────────────────────────────────────────────────────────────────────

func desiredCPUAllocation(vm vmopv1.VirtualMachine) (reservation, limit int64) {
	reservation = 0
	limit = -1
	if res := vm.Spec.Resources; res != nil {
		if req := res.Requests; req != nil && req.CPU != nil {
			reservation = req.CPU.Value()
		}
		if lim := res.Limits; lim != nil && lim.CPU != nil {
			limit = lim.CPU.Value()
		}
	}
	return
}

func desiredMemoryAllocation(vm vmopv1.VirtualMachine) (reservation, limit int64) {
	reservation = 0
	limit = -1
	if res := vm.Spec.Resources; res != nil {
		if req := res.Requests; req != nil && req.Memory != nil {
			reservation = req.Memory.Value() / 1024 / 1024
		}
		if lim := res.Limits; lim != nil && lim.Memory != nil {
			limit = lim.Memory.Value() / 1024 / 1024
		}
	}
	return
}

func desiredLatencySensitivity(vm vmopv1.VirtualMachine) (level vimtypes.LatencySensitivitySensitivityLevel, simultaneousThreads int32) {
	level = vimtypes.LatencySensitivitySensitivityLevelNormal
	if cpu := vm.Spec.CPUAdvanced; cpu != nil && cpu.LatencySensitivity != nil {
		switch *cpu.LatencySensitivity {
		case vmopv1.VirtualMachineLatencySensitivityHigh:
			level = vimtypes.LatencySensitivitySensitivityLevelHigh
		case vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading:
			level = vimtypes.LatencySensitivitySensitivityLevelHigh
			simultaneousThreads = 2
		}
	}
	return
}

func desiredCPUHotAddEnabled(vm vmopv1.VirtualMachine) *bool {
	if cpu := vm.Spec.CPUAdvanced; cpu != nil && cpu.HotAddEnabled != nil {
		return cpu.HotAddEnabled
	}
	return ptr.To(false)
}

func desiredIOMMU(vm vmopv1.VirtualMachine) *bool {
	if cpu := vm.Spec.CPUAdvanced; cpu != nil && cpu.IOMMUEnabled != nil {
		return cpu.IOMMUEnabled
	}
	return ptr.To(false)
}

func desiredNestedHV(vm vmopv1.VirtualMachine) *bool {
	if cpu := vm.Spec.CPUAdvanced; cpu != nil && cpu.NestedHardwareVirtualizationEnabled != nil {
		return cpu.NestedHardwareVirtualizationEnabled
	}
	return ptr.To(false)
}

func desiredPerfCounters(vm vmopv1.VirtualMachine) *bool {
	if cpu := vm.Spec.CPUAdvanced; cpu != nil && cpu.PerformanceCountersEnabled != nil {
		return cpu.PerformanceCountersEnabled
	}
	return ptr.To(false)
}

func desiredMemHotAdd(vm vmopv1.VirtualMachine) *bool {
	if mem := vm.Spec.MemoryAdvanced; mem != nil && mem.HotAddEnabled != nil {
		return mem.HotAddEnabled
	}
	return ptr.To(false)
}

func desiredMemReservationLocked(vm vmopv1.VirtualMachine) *bool {
	if mem := vm.Spec.MemoryAdvanced; mem != nil && mem.ReservationLockedToMax != nil {
		return mem.ReservationLockedToMax
	}
	return ptr.To(false)
}

// topologySpec returns vm.Spec.CPUAdvanced.Topology if set, nil otherwise.
func topologySpec(vm vmopv1.VirtualMachine) *vmopv1.VirtualMachineCPUTopologySpec {
	if cpu := vm.Spec.CPUAdvanced; cpu != nil {
		return cpu.Topology
	}
	return nil
}
