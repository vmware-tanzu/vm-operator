// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import "k8s.io/apimachinery/pkg/api/resource"

// VirtualMachineResourceQuantity holds a pair of CPU and memory resource
// quantities. It is used for spec.resources.size, spec.resources.requests,
// and spec.resources.limits.
//
// Requires the TelcoVMServiceAPI supervisor capability.
type VirtualMachineResourceQuantity struct {
	// +optional
	// +kubebuilder:validation:XValidation:rule="type(self) != string || !self.matches('^-?[0-9]+(\\.[0-9]+)?m$')",message="CPU must be a whole number without the 'm' (milli) suffix: use '4' for 4 vCPUs or '2000' for 2000 MHz, not '4000m' or '2000m'"

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
	CPU resource.Quantity `json:"cpu,omitempty"`

	// +optional

	// Memory is a memory resource quantity (e.g. "8Gi").
	//
	// For spec.resources.size, maps to ConfigSpec.MemoryMB (guest-visible).
	// For spec.resources.requests and spec.resources.limits, maps to
	// MemoryAllocation.Reservation and MemoryAllocation.Limit respectively.
	Memory resource.Quantity `json:"memory,omitempty"`
}

// VirtualMachineResourcesSpec describes the desired compute resource
// allocation for a VirtualMachine. All sub-fields are optional; a nil
// sub-field defers to the VirtualMachineClass value via field-level merge.
// When no VirtualMachineClass is referenced, spec.resources.size.cpu and
// spec.resources.size.memory are required.
//
// Requires the TelcoVMServiceAPI supervisor capability.
type VirtualMachineResourcesSpec struct {
	// +optional

	// Size is the guest-visible compute allocation.
	// size.cpu is the vCPU count; size.memory is the guest RAM.
	// Maps to ConfigSpec.NumCPUs and ConfigSpec.MemoryMB.
	// When set, overrides the corresponding VMClass hardware values.
	Size *VirtualMachineResourceQuantity `json:"size,omitempty"`

	// +optional

	// Requests is the host-level resource reservation (host guarantee).
	// requests.cpu is in MHz; requests.memory is in bytes.
	// Maps to CpuAllocation.Reservation and MemoryAllocation.Reservation.
	// Changes apply immediately on a powered-on VM.
	Requests *VirtualMachineResourceQuantity `json:"requests,omitempty"`

	// +optional

	// Limits is the host-level resource allocation ceiling.
	// limits.cpu is in MHz (nil = unlimited); limits.memory is in bytes
	// (nil = unlimited).
	// Maps to CpuAllocation.Limit and MemoryAllocation.Limit.
	// Changes apply immediately on a powered-on VM.
	Limits *VirtualMachineResourceQuantity `json:"limits,omitempty"`
}

// +kubebuilder:validation:Enum=Low;Normal;High;HighWithHyperthreading

// VirtualMachineLatencySensitivityLevel defines the vSphere CPU scheduler
// latency sensitivity level for a VirtualMachine.
type VirtualMachineLatencySensitivityLevel string

const (
	// VirtualMachineLatencySensitivityLow configures the CPU scheduler to
	// favor throughput over latency.
	VirtualMachineLatencySensitivityLow VirtualMachineLatencySensitivityLevel = "Low"

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
// Requires the TelcoVMServiceAPI supervisor capability.
type VirtualMachineCPUTopologySpec struct {
	// +optional
	// +kubebuilder:validation:Minimum=1

	// CoresPerSocket controls the number of cores per virtual socket,
	// allowing users to tune the NUMA topology visible to the guest OS.
	// Maps to ConfigSpec.NumCoresPerSocket.
	// Requires power-off to apply.
	CoresPerSocket *int32 `json:"coresPerSocket,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=1

	// CoresPerNUMANode sets the number of cores per virtual NUMA node.
	// When set, virtual NUMA node count = total vCPU count / CoresPerNUMANode.
	// Used alongside CoresPerSocket for full CPU topology control.
	// Maps to ConfigSpec.VirtualNuma.CoresPerNumaNode.
	// Requires hardware version vmx-20 or later.
	// Requires power-off to apply.
	CoresPerNUMANode *int32 `json:"coresPerNumaNode,omitempty"`

	// +optional

	// ExposeVNUMAOnCPUHotAdd controls whether virtual NUMA topology is
	// exposed to the guest when vCPUs are hot-added.
	// Only relevant when cpuAdvanced.hotAddEnabled is true and virtual NUMA
	// is configured (CoresPerNUMANode is set).
	// Maps to ConfigSpec.VirtualNuma.ExposeVnumaOnCpuHotadd.
	// Requires hardware version vmx-20 or later.
	// Requires power-off to apply.
	ExposeVNUMAOnCPUHotAdd *bool `json:"exposeVnumaOnCpuHotadd,omitempty"`
}

// VirtualMachineCPUAdvancedSpec describes advanced CPU scheduling and topology
// configuration for a VirtualMachine.
//
// Requires the TelcoVMServiceAPI supervisor capability.
type VirtualMachineCPUAdvancedSpec struct {
	// +optional

	// LatencySensitivity sets the vSphere CPU scheduler latency sensitivity
	// level. Maps to ConfigSpec.LatencySensitivity.Level and
	// ConfigSpec.SimultaneousThreads (for HighWithHyperthreading).
	// Use High or HighWithHyperthreading for Telco VNF workloads requiring
	// deterministic scheduling. Requires full CPU and memory reservation
	// (requests = size) when set to High or HighWithHyperthreading.
	// If not set, the VMClass value is used; if VMClass is absent, vSphere
	// defaults to Normal.
	// Requires power-off to apply.
	LatencySensitivity *VirtualMachineLatencySensitivityLevel `json:"latencySensitivity,omitempty"`

	// +optional

	// Topology describes CPU topology configuration (cores per socket,
	// cores per NUMA node, vNUMA exposure on hot-add). All sub-fields are
	// optional and used together to define the guest CPU layout.
	Topology *VirtualMachineCPUTopologySpec `json:"topology,omitempty"`

	// +optional

	// HotAddEnabled allows vCPUs to be added to the VM while it is powered
	// on (CPU hot-add). Maps to ConfigSpec.CpuHotAddEnabled.
	// Requires hardware version vmx-20 or later.
	// Not compatible with LatencySensitivity High or HighWithHyperthreading.
	// Requires power-off to apply.
	HotAddEnabled *bool `json:"hotAddEnabled,omitempty"`

	// +optional

	// IOMMUEnabled enables Intel Virtualization Technology for Directed I/O
	// (VT-d / IOMMU) for this VM. Required for SR-IOV and PCI passthrough
	// workloads. Requires EFI firmware (spec.bootOptions.firmware = "efi").
	// Maps to ConfigSpec.Flags.VvtdEnabled.
	IOMMUEnabled *bool `json:"iommuEnabled,omitempty"`

	// +optional

	// NestedHardwareVirtualizationEnabled exposes hardware-assisted
	// virtualization to the guest OS, enabling the guest to run its own
	// hypervisor or use hardware VMX instructions. Required for nested
	// virtualization workloads.
	// Maps to ConfigSpec.NestedHVEnabled.
	NestedHardwareVirtualizationEnabled *bool `json:"nestedHardwareVirtualizationEnabled,omitempty"`

	// +optional

	// PerformanceCountersEnabled enables virtualized CPU performance
	// counters (vPMC), allowing profiling tools inside the guest OS to
	// access hardware performance counter data.
	// Maps to ConfigSpec.VPMCEnabled.
	PerformanceCountersEnabled *bool `json:"performanceCountersEnabled,omitempty"`
}

// VirtualMachineMemoryAdvancedSpec describes advanced memory configuration
// for a VirtualMachine.
//
// Requires the TelcoVMServiceAPI supervisor capability.
type VirtualMachineMemoryAdvancedSpec struct {
	// +optional

	// HotAddEnabled allows memory to be added to the VM while it is powered
	// on (memory hot-add). Maps to ConfigSpec.MemoryHotAddEnabled.
	// Requires hardware version vmx-20 or later.
	// Requires power-off to apply.
	HotAddEnabled *bool `json:"hotAddEnabled,omitempty"`

	// +optional

	// ReservationLockedToMax pins the host memory reservation to the full
	// guest-visible memory size. Required for SR-IOV workloads, which need
	// full guest RAM pinned on the host.
	// Maps to ConfigSpec.MemoryReservationLockedToMax.
	// Requires power-off to apply.
	ReservationLockedToMax *bool `json:"reservationLockedToMax,omitempty"`
}
