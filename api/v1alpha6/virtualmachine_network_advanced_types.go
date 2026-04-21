// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Defines NIC device type enum, VMXNet3 tuning spec, and related
// types for advanced network interface configuration in VirtualMachine.

package v1alpha6

// VirtualMachineNetworkInterfaceType specifies the NIC device type.
type VirtualMachineNetworkInterfaceType string

const (
	// VirtualMachineNetworkInterfaceTypeVMXNet3 specifies a VMXNet3 paravirtual
	// NIC. This is the default and recommended type for most workloads.
	VirtualMachineNetworkInterfaceTypeVMXNet3 VirtualMachineNetworkInterfaceType = "VMXNet3"

	// VirtualMachineNetworkInterfaceTypeSRIOV specifies an SR-IOV NIC.
	VirtualMachineNetworkInterfaceTypeSRIOV VirtualMachineNetworkInterfaceType = "SRIOV"
)

// TxContextThreadingMode specifies the transmit context threading mode for a
// VMXNet3 interface.
// This is a "weak enum": constants are well-known values; the field accepts any string for forward compatibility.
// +kubebuilder:validation:Pattern="^(PerDevice|PerVM|PerQueue|[1-9])$"
type TxContextThreadingMode string

const (
	// TxContextThreadingModePerDevice configures one TX thread per vNIC.
	TxContextThreadingModePerDevice TxContextThreadingMode = "PerDevice"

	// TxContextThreadingModePerVM configures one TX thread for the whole VM (default).
	TxContextThreadingModePerVM TxContextThreadingMode = "PerVM"

	// TxContextThreadingModePerQueue configures 2-8 TX threads per vNIC queue
	// (scheduler-determined). Recommended for 100G workloads with pnicFeatures
	// including ReceiveSideScaling.
	TxContextThreadingModePerQueue TxContextThreadingMode = "PerQueue"
)

// CoalescingScheme specifies the interrupt coalescing scheme for a VMXNet3
// interface.
// This is a "weak enum": constants are well-known values; the field accepts any string for forward compatibility.
//
// +kubebuilder:validation:XValidation:rule="self == 'Disabled' || self == 'Adapt' || self == 'Static' || self == 'RateBasedCoalescing' || size(self) < 128",message="must be enum value or string < 128 chars"
type CoalescingScheme string

const (
	// CoalescingSchemeDisabled disables interrupt coalescing entirely.
	// Recommended for latency-sensitive (LS=High) non-DPDK workloads because it
	// ensures each packet triggers an immediate interrupt.
	CoalescingSchemeDisabled CoalescingScheme = "Disabled"

	// CoalescingSchemeAdapt uses adaptive coalescing, dynamically adjusting the
	// interrupt rate based on VM and system load. CoalescingParams is ignored
	// when this scheme is set.
	CoalescingSchemeAdapt CoalescingScheme = "Adapt"

	// CoalescingSchemeStatic queues a fixed number of packets before triggering
	// an interrupt. CoalescingParams sets the Tx,Rx packet queue limit (range 1-64,
	// default "64").
	CoalescingSchemeStatic CoalescingScheme = "Static"

	// CoalescingSchemeRateBasedCoalescing uses rate-based coalescing (RBC)
	// CoalescingParams sets the interrupt rate in interrupts/sec (range 100-100000, default "4000").
	CoalescingSchemeRateBasedCoalescing CoalescingScheme = "RateBasedCoalescing"
)

// PNICQueueFeature names one physical NIC queue offload feature for VMXNet3
// pnicFeatures.
// This is a "weak enum": constants are well-known values; the field accepts any string for forward compatibility.
//
// +kubebuilder:validation:XValidation:rule="self == 'LargeReceiveOffload' || self == 'ReceiveSideScaling' || (self.matches('^\\d+$') && int(self) > 0 && (int(self) & (int(self) - 1)) == 0)",message="must be enum value or power of 2 integer string"
type PNICQueueFeature string

const (
	// PNICQueueFeatureLargeReceiveOffload enables large receive offload (LRO).
	PNICQueueFeatureLargeReceiveOffload PNICQueueFeature = "LargeReceiveOffload"

	// PNICQueueFeatureReceiveSideScaling enables receive-side scaling (RSS)
	// hardware queues, allowing the physical NIC to distribute incoming packets
	// across multiple receive queues. Typically set alongside
	// ctxPerDev=PerQueue for maximum throughput on 100G workloads.
	PNICQueueFeatureReceiveSideScaling PNICQueueFeature = "ReceiveSideScaling"
)

// VirtualMachineNetworkInterfaceVMXNet3Spec contains tuning options specific to
// VMXNet3 network interfaces. Fields with 'vmx' annotation map to ethernetX.* VMX keys, where X
// is the device index derived from the vSphere device key at runtime.
//
// These fields are only valid when the interface Type is VMXNet3. The CRD
// admission webhook rejects this struct when Type is set to an incompatible
// value.
//
// +kubebuilder:validation:XValidation:rule="!has(self.coalescingParams) || size(self.coalescingParams) < 128",message="coalescingParams must have length < 128"
// +kubebuilder:validation:XValidation:rule="!has(self.coalescingParams) || !has(self.coalescingScheme) || self.coalescingScheme != 'RateBasedCoalescing' || (self.coalescingParams.matches('^\\d+$') && int(self.coalescingParams) >= 0)",message="coalescingParams must be valid number when coalescingScheme is RateBasedCoalescing"
type VirtualMachineNetworkInterfaceVMXNet3Spec struct {
	// +optional

	// UPTv2Enabled enables UPT v2 (Uniform Passthrough v2) for this interface.
	// UPT allows the guest to drive the physical NIC virtual function directly
	// via SR-IOV while preserving vMotion support by dynamically switching
	// between passthrough and emulation mode. UPTv1 is deprecated.
	//
	// Requires: spec.minHardwareVersion >= 20, SmartNIC with UPT support,
	// full VM memory reservation, and VMXNet3 v7 guest driver.
	UPTv2Enabled *bool `json:"uptv2Enabled,omitempty"`

	// +optional

	// CtxPerDev sets the TX context threading mode for this interface.
	// PerVM (default) gives one TX thread for the whole VM.
	// PerDevice gives one TX thread per vNIC.
	// PerQueue gives 2-8 TX threads per vNIC queue (scheduler-determined);
	// recommended for 100G workloads combined with pnicFeatures including ReceiveSideScaling.
	// Visible in esxtop as NetWorld-Dev-<name>-Tx threads.
	// Accepts known enum values (PerDevice, PerVM, PerQueue) or single digit integers (1-9)
	// for direct VMX values.
	CtxPerDev *TxContextThreadingMode `json:"ctxPerDev,omitempty" vmx:"ethernet%d.ctxPerDev"`

	// +optional

	// RSSOffloadEnabled enables RSS (Receive Side Scaling) offload, allowing
	// the physical NIC to distribute incoming packets across multiple receive
	// queues using a hardware-computed hash. Reduces hypervisor CPU overhead
	// and improves multi-core utilization for high-throughput workloads.
	// Requires pNIC RSS support.
	RSSOffloadEnabled *bool `json:"rssOffloadEnabled,omitempty" vmx:"ethernet%d.rssoffload"`

	// +optional

	// UDPRSSEnabled extends RSS to UDP traffic. By default RSS only distributes
	// TCP flows. Enabling this also distributes UDP flows, improving throughput
	// for UDP-heavy workloads such as GTP-U tunnels, QUIC, or media streaming.
	UDPRSSEnabled *bool `json:"udpRSSEnabled,omitempty" vmx:"ethernet%d.udpRSS"`

	// +optional
	// +kubebuilder:validation:MaxItems=16

	// +listType=set
	//
	// PNICFeatures lists physical NIC queue offload features to enable. The
	// primary use is including ReceiveSideScaling, which allows the vNIC to leverage physical
	// NIC RSS hardware queues. Typically set to ["ReceiveSideScaling"] alongside
	// ctxPerDev=PerQueue for maximum 100G throughput. Omitted or empty means no
	// extra pNIC queue features beyond defaults.
	// Accepts known enum values (LargeReceiveOffload, ReceiveSideScaling) or integer strings
	// representing powers of 2 (1,2,4,8,...) for direct VMX bitmask values.
	PNICFeatures []PNICQueueFeature `json:"pnicFeatures,omitempty" vmx:"ethernet%d.pnicfeatures"`

	// +optional

	// CoalescingScheme sets the interrupt coalescing scheme for this interface.
	// Use CoalescingSchemeDisabled for latency-sensitive (LS=High) non-DPDK
	// workloads to minimise interrupt latency.
	// Accepts known enum values (Disabled, Adapt, Static, RateBasedCoalescing) or any
	// string with length < 128 for forward compatibility.
	CoalescingScheme *CoalescingScheme `json:"coalescingScheme,omitempty" vmx:"ethernet%d.coalescingScheme"`

	// +optional

	// CoalescingParams sets the coalescing parameter when coalescingScheme is
	// RateBasedCoalescing or Static. The format depends on the scheme:
	//   - RateBasedCoalescing: integer string for interrupts/sec (e.g. "4000")
	//   - Static: integer string for packet queue limit (e.g. "64")
	// Ignored when coalescingScheme is Disabled or Adapt.
	// Must be length < 128 and valid 32-bit unsigned integer for RateBasedCoalescing.
	CoalescingParams *string `json:"coalescingParams,omitempty" vmx:"ethernet%d.coalescingParams"`
}
