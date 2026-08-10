// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

// NetworkDeviceStatus defines the network interface IP configuration including
// gateway, subnet mask and IP address as seen by OVF properties.
//
// +k8s:conversion-gen=false
type NetworkDeviceStatus struct {
	// +optional

	// Gateway4 is the gateway for the IPv4 address family for this device.
	Gateway4 string

	// +optional

	// Gateway6 is the gateway for the IPv6 address family for this device.
	// This may be a link-local address, since IPv6 routers commonly
	// advertise their gateway that way.
	Gateway6 string

	// +optional

	// MacAddress is the MAC address of the network device.
	MacAddress string

	// +optional

	// IpAddresses represents one or more IPv4 addresses assigned to the
	// network device in CIDR notation, ex. "192.0.2.1/16".
	IPAddresses []string

	// +optional

	// IPv6Addresses represents one or more IPv6 addresses assigned to the
	// network device in CIDR notation, ex. "2001:db8::1/64". This is not
	// filtered for link-local, loopback, or unspecified addresses; use the
	// IsUsableIP template function to filter those out explicitly.
	IPv6Addresses []string
}

// NetworkStatus describes the observed state of the VM's network configuration.
//
// +k8s:conversion-gen=false
type NetworkStatus struct {
	// +optional

	// Devices describe a list of current status information for each
	// network interface that is desired to be attached to the
	// VirtualMachineTemplate.
	Devices []NetworkDeviceStatus

	// +optional

	// Nameservers describe a list of the DNS servers accessible by one of the
	// VM's configured network devices.
	Nameservers []string
}

// VirtualMachineTemplate defines the specification for configuring
// VirtualMachine Template. A Virtual Machine Template is created during VM
// customization to populate OVF properties. Then by utilizing Golang-based
// templating, Virtual Machine Template provides access to dynamic configuration
// data.
//
// +k8s:conversion-gen=false
type VirtualMachineTemplate struct {
	// +optional

	// Net describes the observed state of the VM's network configuration.
	Net NetworkStatus

	// VM represents a pointer to a VirtualMachine instance that consist of the
	// desired specification and the observed status
	VM *VirtualMachine
}
