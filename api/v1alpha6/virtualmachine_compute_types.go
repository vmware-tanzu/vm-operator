// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import "k8s.io/apimachinery/pkg/api/resource"

// VirtualMachineResourceQuantity holds a pair of CPU and memory resource
// quantities. It is used for spec.resources.size, spec.resources.requests,
// and spec.resources.limits.
type VirtualMachineResourceQuantity struct {
	// +optional
	// +kubebuilder:validation:XValidation:rule="type(self) != string || !self.endsWith('m')",message="CPU must be a whole number (e.g. '4' for vCPUs or '2000' for 2000 MHz). The 'm' (milli) suffix is not supported."

	// CPU is a CPU resource quantity.
	//
	// For spec.resources.size, the value represents the guest-visible vCPU
	// count and must be a whole number (e.g. "4"). Maps to
	// ConfigSpec.NumCPUs.
	//
	// For spec.resources.requests and spec.resources.limits, the value
	// represents a host-level CPU allocation in MHz (e.g. "2000" for
	// 2 GHz). Maps to CpuAllocation.Reservation and CpuAllocation.Limit
	// respectively.
	//
	// For spec.resources.size, an increase can be applied while the VM is
	// powered on when cpuAdvanced.hotAddEnabled is already enabled on the
	// live VM; a decrease always requires power-off.
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// +optional

	// Memory is a memory resource quantity in bytes (e.g. "8Gi").
	//
	// For spec.resources.size, maps to ConfigSpec.MemoryMB (guest-visible).
	// For spec.resources.requests and spec.resources.limits, maps to
	// MemoryAllocation.Reservation and MemoryAllocation.Limit respectively.
	//
	// For spec.resources.size, an increase can be applied while the VM is
	// powered on when memoryAdvanced.hotAddEnabled is already enabled on the
	// live VM; a decrease always requires power-off. Note that enabling
	// memoryAdvanced.hotAddEnabled at the vSphere level is a necessary but
	// not sufficient condition: the guest OS and its drivers must also
	// support memory hot-add for the guest to recognize the additional
	// memory. Not all guest operating systems support this capability.
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// VirtualMachineResourcesSpec describes the desired compute resource
// allocation for a VirtualMachine. All sub-fields are optional; a nil
// sub-field defers to the VirtualMachineClass value via field-level merge.
type VirtualMachineResourcesSpec struct {
	// +optional

	// Size is the guest-visible compute allocation.
	// size.cpu is the vCPU count; size.memory is the guest RAM.
	// Maps to ConfigSpec.NumCPUs and ConfigSpec.MemoryMB.
	// When set, overrides the corresponding VMClass hardware values.
	Size *VirtualMachineResourceQuantity `json:"size,omitempty"`

	// +optional

	// Requests is the host-level resource reservation (host guarantee).
	// requests.cpu is in MHz (e.g. "2000"); requests.memory is in bytes
	// (e.g. "8Gi").
	// Maps to CpuAllocation.Reservation and MemoryAllocation.Reservation.
	// Can be reconfigured while the VM is powered on.
	Requests *VirtualMachineResourceQuantity `json:"requests,omitempty"`

	// +optional

	// Limits is the host-level resource allocation ceiling.
	// limits.cpu is in MHz (e.g. "2000"); limits.memory is in bytes
	// (e.g. "8Gi").
	// Each must be either greater than 0, or -1 to explicitly request no
	// ceiling; a nil value also means unlimited, deferring to the VM Class
	// or vSphere default. No other negative value is valid.
	// Maps to CpuAllocation.Limit and MemoryAllocation.Limit.
	// Can be reconfigured while the VM is powered on.
	Limits *VirtualMachineResourceQuantity `json:"limits,omitempty"`
}

// +kubebuilder:validation:Enum=Normal;High;HighWithHyperthreading

// VirtualMachineLatencySensitivityLevel defines the vSphere CPU scheduler
// latency sensitivity level for a VirtualMachine.
type VirtualMachineLatencySensitivityLevel string

const (
	// VirtualMachineLatencySensitivityNormal is the default scheduling mode.
	VirtualMachineLatencySensitivityNormal VirtualMachineLatencySensitivityLevel = "Normal"

	// VirtualMachineLatencySensitivityHigh minimizes scheduling latency at
	// the cost of throughput. Requires full CPU and memory reservation
	// (requests = size). Recommended for latency-sensitive Telco VNF
	// workloads.
	VirtualMachineLatencySensitivityHigh VirtualMachineLatencySensitivityLevel = "High"

	// VirtualMachineLatencySensitivityHighWithHyperthreading combines High
	// latency sensitivity with simultaneous multi-threading (HT). Sets
	// ConfigSpec.LatencySensitivity.Level=High and
	// ConfigSpec.SimultaneousThreads=2. Requires full CPU and memory
	// reservation (requests = size).
	VirtualMachineLatencySensitivityHighWithHyperthreading VirtualMachineLatencySensitivityLevel = "HighWithHyperthreading"
)

// VirtualMachineCPUTopologySpec describes the guest CPU topology.
//
// +kubebuilder:validation:XValidation:rule="!has(self.vnumaNodeCount) || self.vnumaNodeCount == 0 || (has(self.coresPerSocket) && self.coresPerSocket > 0)",message="vnumaNodeCount requires coresPerSocket to be set to an explicit (non-zero) value"
type VirtualMachineCPUTopologySpec struct {
	// +optional
	// +kubebuilder:validation:Minimum=0

	// CoresPerSocket controls the number of cores per virtual socket.
	// When unset or 0, any explicit cores-per-socket configuration is cleared,
	// reverting to the vSphere default (auto sizing). 0 is a sentinel for
	// "auto" and is equivalent to leaving the field unset.
	// Maps to ConfigSpec.NumCoresPerSocket.
	// Requires power-off to apply.
	CoresPerSocket *int32 `json:"coresPerSocket,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0

	// VNUMANodeCount sets the number of virtual NUMA nodes. vSphere derives
	// coresPerNumaNode as total vCPU count / VNUMANodeCount; the vCPU count
	// should be evenly divisible by VNUMANodeCount for a balanced topology.
	// The derived coresPerNumaNode must be a multiple or divisor of
	// CoresPerSocket; the VM will fail to power on if this constraint is
	// violated.
	// When unset or 0, any manual vNUMA configuration is cleared, enabling
	// automatic vNUMA sizing. 0 is a sentinel for "auto" and is equivalent to
	// leaving the field unset.
	// When set to a value > 0, must be set together with CoresPerSocket.
	// Maps to ConfigSpec.VirtualNuma.CoresPerNumaNode.
	// Requires hardware version vmx-20 or later.
	// Requires power-off to apply.
	VNUMANodeCount *int32 `json:"vnumaNodeCount,omitempty"`
}

// VirtualMachineCPUAdvancedSpec describes advanced CPU scheduling and topology
// configuration for a VirtualMachine.
type VirtualMachineCPUAdvancedSpec struct {
	// +optional

	// LatencySensitivity sets the vSphere CPU scheduler latency sensitivity
	// level. Maps to ConfigSpec.LatencySensitivity.Level and
	// ConfigSpec.SimultaneousThreads (for HighWithHyperthreading).
	// Requires full CPU and memory reservation (requests = size)
	// when set to High or HighWithHyperthreading.
	// When unset, the setting is cleared, reverting to the vSphere default.
	// Can be reconfigured while the VM is powered on.
	LatencySensitivity *VirtualMachineLatencySensitivityLevel `json:"latencySensitivity,omitempty"`

	// +optional

	// Topology describes CPU topology configuration (cores per socket,
	// cores per NUMA node, vNUMA exposure on hot-add). All sub-fields are
	// optional and used together to define the guest CPU layout.
	Topology *VirtualMachineCPUTopologySpec `json:"topology,omitempty"`

	// +optional

	// HotAddEnabled allows vCPUs to be added to the VM while it is powered
	// on (CPU hot-add). Maps to ConfigSpec.CpuHotAddEnabled.
	// Requires hardware version vmx-11 or later.
	// Requires power-off to apply.
	HotAddEnabled *bool `json:"hotAddEnabled,omitempty"`

	// +optional

	// IOMMUEnabled enables Intel Virtualization Technology for Directed I/O
	// (VT-d / IOMMU) for this VM. Required for SR-IOV and PCI passthrough
	// workloads. Maps to ConfigSpec.Flags.VvtdEnabled.
	// Minimum hardware version depends on CPU vendor:
	// vmx-14 or later on Intel CPUs; vmx-18 or later on AMD CPUs.
	// Requires power-off to apply.
	IOMMUEnabled *bool `json:"iommuEnabled,omitempty"`

	// +optional

	// NestedHardwareVirtualizationEnabled exposes hardware-assisted
	// virtualization to the guest OS, enabling the guest to run its own
	// hypervisor or use hardware VMX instructions. Required for nested
	// virtualization workloads.
	// Maps to ConfigSpec.NestedHVEnabled.
	// Requires hardware version vmx-9 or later.
	// Requires power-off to apply.
	NestedHardwareVirtualizationEnabled *bool `json:"nestedHardwareVirtualizationEnabled,omitempty"`

	// +optional

	// PerformanceCountersEnabled enables virtualized CPU performance
	// counters (vPMC), allowing profiling tools inside the guest OS to
	// access hardware performance counter data.
	// Maps to ConfigSpec.VPMCEnabled.
	// Requires hardware version vmx-9 or later.
	// Requires power-off to apply.
	PerformanceCountersEnabled *bool `json:"performanceCountersEnabled,omitempty"`
}

// VirtualMachineMemoryAdvancedSpec describes advanced memory configuration
// for a VirtualMachine.
type VirtualMachineMemoryAdvancedSpec struct {
	// +optional

	// HotAddEnabled allows memory to be added to the VM while it is powered
	// on (memory hot-add). Maps to ConfigSpec.MemoryHotAddEnabled.
	// Requires hardware version vmx-7 or later.
	// Requires power-off to apply.
	HotAddEnabled *bool `json:"hotAddEnabled,omitempty"`

	// +optional

	// ReservationLockedToMax pins the host memory reservation to the full
	// guest-visible memory size. Required for SR-IOV workloads, which need
	// full guest RAM pinned on the host.
	// Maps to ConfigSpec.MemoryReservationLockedToMax.
	// Can be reconfigured while the VM is powered on.
	ReservationLockedToMax *bool `json:"reservationLockedToMax,omitempty"`
}
