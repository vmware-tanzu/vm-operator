// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// VirtualEthernetCardOption describes the options common to all virtual
// Ethernet card types.
// It corresponds to vim.vm.device.VirtualEthernetCardOption.
type VirtualEthernetCardOption struct {
	// SupportedOUI describes the valid Organizational Unique Identifiers (OUIs)
	// supported for statically assigned MAC addresses (e.g., "00:50:56").
	SupportedOUI ChoiceOption `json:"supportedOUI"`

	// MacType describes the supported MAC address types.
	MacType ChoiceOption `json:"macType"`

	// WakeOnLanEnabled indicates whether Wake-on-LAN is settable on this
	// device.
	WakeOnLanEnabled BoolOption `json:"wakeOnLanEnabled"`

	// +optional

	// VMDirectPathGen2Supported indicates whether VMDirectPath Gen 2 is
	// available on this device.
	//
	// Deprecated: As of vSphere API 8.0, VMDirectPath Gen 2 is no longer
	// supported and there is no replacement.
	VMDirectPathGen2Supported *bool `json:"vmDirectPathGen2Supported,omitempty"`

	// +optional

	// UptCompatibilityEnabled indicates whether Universal Pass-through (UPT)
	// is settable on this device.
	//
	// Deprecated: As of vSphere API 8.0, VMDirectPath Gen 2 is no longer
	// supported and there is no replacement.
	UptCompatibilityEnabled *BoolOption `json:"uptCompatibilityEnabled,omitempty"`
}

// VirtualPCNet32Option describes the options for the AMD Lance PCNet32 virtual
// Ethernet adapter. It corresponds to vim.vm.device.VirtualPCNet32Option.
type VirtualPCNet32Option struct {
	VirtualEthernetCardOption `json:",inline"`

	// SupportsMorphing indicates that this Ethernet card supports morphing
	// into a vmxnet adapter when appropriate, gaining its added performance
	// capabilities.
	SupportsMorphing bool `json:"supportsMorphing"`
}

// VirtualVmxnet3Option describes the options for the Vmxnet3 virtual Ethernet
// adapter. It corresponds to vim.vm.device.VirtualVmxnet3Option.
type VirtualVmxnet3Option struct {
	VirtualVmxnetOption `json:",inline"`

	// +optional

	// Uptv2Enabled indicates whether UPTv2 (Uniform Pass-through version 2)
	// is settable on this device.
	Uptv2Enabled *BoolOption `json:"uptv2Enabled,omitempty"`

	// +optional

	// StrictLatencyConfigOption describes the strict latency configuration
	// options for this adapter.
	StrictLatencyConfigOption *VirtualVmxnet3OptionStrictLatencyConfigOption `json:"strictLatencyConfigOption,omitempty"`
}

// VirtualVmxnet3VrdmaOption describes the options for the Vmxnet3 VRDMA
// virtual Ethernet adapter.
// It corresponds to vim.vm.device.VirtualVmxnet3VrdmaOption.
type VirtualVmxnet3VrdmaOption struct {
	VirtualVmxnet3Option `json:",inline"`

	// +optional

	// DeviceProtocol describes the supported VRDMA device protocols.
	// Acceptable values are specified by VirtualVmxnet3VrdmaOptionDeviceProtocols.
	DeviceProtocol *ChoiceOption `json:"deviceProtocol,omitempty"`
}

// VirtualVmxnet3OptionStrictLatencyConfigOption describes the strict latency
// configuration options for a Vmxnet3 adapter.
// It corresponds to vim.vm.device.VirtualVmxnet3Option.StrictLatencyConfigOption.
type VirtualVmxnet3OptionStrictLatencyConfigOption struct {
	// Allowed indicates whether strict latency configuration is permitted on
	// this adapter.
	Allowed BoolOption `json:"allowed"`

	// MeasureLatency indicates whether latency measurement is enabled on this
	// adapter.
	MeasureLatency BoolOption `json:"measureLatency"`

	// MaxTxQueues describes the minimum, maximum, and default number of
	// transmit queues on this adapter.
	MaxTxQueues IntOption `json:"maxTxQueues"`

	// MaxRxQueues describes the minimum, maximum, and default number of
	// receive queues on this adapter.
	MaxRxQueues IntOption `json:"maxRxQueues"`

	// TxDataRingDescSize describes the minimum, maximum, and default transmit
	// data ring descriptor size on this adapter.
	TxDataRingDescSize IntOption `json:"txDataRingDescSize"`

	// RxDataRingDescSize describes the minimum, maximum, and default receive
	// data ring descriptor size on this adapter.
	RxDataRingDescSize IntOption `json:"rxDataRingDescSize"`

	// DisableOffload describes the type of offload disable operation supported
	// on this adapter (e.g., "TSO_LRO").
	DisableOffload ChoiceOption `json:"disableOffload"`
}

// VirtualE1000Option describes the options for an Intel E1000 virtual Ethernet
// adapter.
// It corresponds to vim.vm.device.VirtualE1000Option, which carries no data
// beyond the base Ethernet card option.
type VirtualE1000Option struct {
	VirtualEthernetCardOption `json:",inline"`
}

// VirtualE1000eOption describes the options for an Intel E1000e virtual
// Ethernet adapter.
// It corresponds to vim.vm.device.VirtualE1000eOption, which carries no data
// beyond the base Ethernet card option.
type VirtualE1000eOption struct {
	VirtualEthernetCardOption `json:",inline"`
}

// VirtualSriovEthernetCardOption describes the options for an SR-IOV enabled
// virtual Ethernet adapter.
// It corresponds to vim.vm.device.VirtualSriovEthernetCardOption, which
// carries no data beyond the base Ethernet card option.
type VirtualSriovEthernetCardOption struct {
	VirtualEthernetCardOption `json:",inline"`
}

// VirtualVmxnetOption describes the options for a VMware Vmxnet virtual
// Ethernet adapter.
// It corresponds to vim.vm.device.VirtualVmxnetOption, which carries no data
// beyond the base Ethernet card option.
type VirtualVmxnetOption struct {
	VirtualEthernetCardOption `json:",inline"`
}

// VirtualVmxnet2Option describes the options for a VMware Vmxnet2 virtual
// Ethernet adapter.
// It corresponds to vim.vm.device.VirtualVmxnet2Option, which carries no data
// beyond the base Vmxnet adapter option.
type VirtualVmxnet2Option struct {
	VirtualVmxnetOption `json:",inline"`
}
