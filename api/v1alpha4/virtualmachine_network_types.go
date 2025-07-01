// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
)

// VirtualMachineNetworkRouteSpec defines a static route for a guest.
type VirtualMachineNetworkRouteSpec struct {
	// To is either "default", or an IP4 or IP6 address.
	To string `json:"to"`

	// Via is an IP4 or IP6 address.
	Via string `json:"via"`

	// +optional
	// +kubebuilder:validation:Minimum=1

	// Metric is the weight/priority of the route.
	Metric int32 `json:"metric,omitempty"`
}

// VirtualMachineNetworkInterfaceSpec describes the desired state of a VM's
// network interface.
type VirtualMachineNetworkInterfaceSpec struct {
	// +kubebuilder:validation:Pattern="^[a-z0-9]{2,}$"

	// Name describes the unique name of this network interface, used to
	// distinguish it from other network interfaces attached to this VM.
	//
	// When the bootstrap provider is Cloud-Init and GuestDeviceName is not
	// specified, the device inside the guest will be renamed to this value.
	// Please note it is up to the user to ensure the provided name does not
	// conflict with any other devices inside the guest, ex. dvd, cdrom, sda, etc.
	Name string `json:"name"`

	// +optional

	// Network is the name of the network resource to which this interface is
	// connected.
	//
	// If no network is provided, then this interface will be connected to the
	// Namespace's default network.
	Network *vmopv1common.PartialObjectRef `json:"network,omitempty"`

	// +optional
	// +kubebuilder:validation:Pattern=^\w\w+$

	// GuestDeviceName is used to rename the device inside the guest when the
	// bootstrap provider is Cloud-Init. Please note it is up to the user to
	// ensure the provided device name does not conflict with any other devices
	// inside the guest, ex. dvd, cdrom, sda, etc.
	GuestDeviceName string `json:"guestDeviceName,omitempty"`

	// +optional

	// Addresses is an optional list of IP4 or IP6 addresses to assign to this
	// interface.
	//
	// Please note this field is only supported if the connected network
	// supports manual IP allocation.
	//
	// Please note IP4 and IP6 addresses must include the network prefix length,
	// ex. 192.168.0.10/24 or 2001:db8:101::a/64.
	//
	// Please note this field may not contain IP4 addresses if DHCP4 is set
	// to true or IP6 addresses if DHCP6 is set to true.
	Addresses []string `json:"addresses,omitempty"`

	// +optional

	// DHCP4 indicates whether or not this interface uses DHCP for IP4
	// networking.
	//
	// Please note this field is only supported if the network connection
	// supports DHCP.
	//
	// Please note this field is mutually exclusive with IP4 addresses in the
	// Addresses field and the Gateway4 field.
	DHCP4 bool `json:"dhcp4,omitempty"`

	// +optional

	// DHCP6 indicates whether or not this interface uses DHCP for IP6
	// networking.
	//
	// Please note this field is only supported if the network connection
	// supports DHCP.
	//
	// Please note this field is mutually exclusive with IP6 addresses in the
	// Addresses field and the Gateway6 field.
	DHCP6 bool `json:"dhcp6,omitempty"`

	// +optional

	// Gateway4 is the default, IP4 gateway for this interface.
	//
	// If unset, the gateway from the network provider will be used. However,
	// if set to "None", the network provider gateway will be ignored.
	//
	// Please note this field is only supported if the network connection
	// supports manual IP allocation.
	//
	// Please note the IP address must include the network prefix length, ex.
	// 192.168.0.1/24.
	//
	// Please note this field is mutually exclusive with DHCP4.
	Gateway4 string `json:"gateway4,omitempty"`

	// +optional

	// Gateway6 is the primary IP6 gateway for this interface.
	//
	// If unset, the gateway from the network provider will be used. However,
	// if set to "None", the network provider gateway will be ignored.
	//
	// Please note this field is only supported if the network connection
	// supports manual IP allocation.
	//
	// Please note the IP address must include the network prefix length, ex.
	// 2001:db8:101::1/64.
	//
	// Please note this field is mutually exclusive with DHCP6.
	Gateway6 string `json:"gateway6,omitempty"`

	// +optional

	// MTU is the Maximum Transmission Unit size in bytes.
	//
	// Please note this feature is available only with the following bootstrap
	// providers: CloudInit.
	MTU *int64 `json:"mtu,omitempty"`

	// +optional

	// Nameservers is a list of IP4 and/or IP6 addresses used as DNS
	// nameservers.
	//
	// Please note this feature is available only with the following bootstrap
	// providers: CloudInit and Sysprep.
	//
	// When using CloudInit and UseGlobalNameserversAsDefault is either unset or
	// true, if nameservers is not provided, the global nameservers will be used
	// instead.
	//
	// Please note that Linux allows only three nameservers
	// (https://linux.die.net/man/5/resolv.conf).
	Nameservers []string `json:"nameservers,omitempty"`

	// +optional

	// Routes is a list of optional, static routes.
	//
	// Please note this feature is available only with the following bootstrap
	// providers: CloudInit.
	Routes []VirtualMachineNetworkRouteSpec `json:"routes,omitempty"`

	// +optional

	// SearchDomains is a list of search domains used when resolving IP
	// addresses with DNS.
	//
	// Please note this feature is available only with the following bootstrap
	// providers: CloudInit.
	//
	// When using CloudInit and UseGlobalSearchDomainsAsDefault is either unset
	// or true, if search domains is not provided, the global search domains
	// will be used instead.
	SearchDomains []string `json:"searchDomains,omitempty"`
}

// VirtualMachineNetworkSpec defines a VM's desired network configuration.
type VirtualMachineNetworkSpec struct {
	// +optional

	// HostName describes the value the guest uses as its host name. If omitted,
	// the name of the VM will be used.
	//
	// Please note, this feature is available with the following bootstrap
	// providers: CloudInit, LinuxPrep, and Sysprep.
	//
	// This field must adhere to the format specified in RFC-1034, Section 3.5
	// for DNS labels:
	//
	//   * The total length is restricted to 63 characters or less.
	//   * The total length is restricted to 15 characters or less on Windows
	//     systems.
	//   * The value may begin with a digit per RFC-1123.
	//   * Underscores are not allowed.
	//   * Dashes are permitted, but not at the start or end of the value.
	//   * Symbol unicode points, such as emoji, are permitted, ex. ✓. However,
	//     please note that the use of emoji, even where allowed, may not
	//     compatible with the guest operating system, so it recommended to
	//     stick with more common characters for this value.
	//   * The value may be a valid IP4 or IP6 address. Please note, the use of
	//     an IP address for a host name is not compatible with all guest
	//     operating systems and is discouraged. Additionally, using an IP
	//     address for the host name is disallowed if spec.network.domainName is
	//     non-empty.
	//
	// Please note, the combined values of spec.network.hostName and
	// spec.network.domainName may not exceed 255 characters in length.
	HostName string `json:"hostName,omitempty"`

	// +optional

	// DomainName describes the value the guest uses as its domain name.
	//
	// Please note, this feature is available with the following bootstrap
	// providers: CloudInit, LinuxPrep, and Sysprep.
	//
	// This field must adhere to the format specified in RFC-1034, Section 3.5
	// for DNS names:
	//
	//   * When joined with the host name, the total length is restricted to 255
	//     characters or less.
	//   * Individual segments must be 63 characters or less.
	//   * The top-level domain( ex. ".com"), is at least two letters with no
	//     special characters.
	//   * Underscores are not allowed.
	//   * Dashes are permitted, but not at the start or end of the value.
	//   * Long, top-level domain names (ex. ".london") are permitted.
	//   * Symbol unicode points, such as emoji, are disallowed in the top-level
	//     domain.
	//
	// Please note, the combined values of spec.network.hostName and
	// spec.network.domainName may not exceed 255 characters in length.
	//
	// When deploying a guest running Microsoft Windows, this field describes
	// the domain the computer should join.
	DomainName string `json:"domainName,omitempty"`

	// +optional

	// Disabled is a flag that indicates whether or not to disable networking
	// for this VM.
	//
	// When set to true, the VM is not configured with a default interface nor
	// any specified from the Interfaces field.
	Disabled bool `json:"disabled,omitempty"`

	// +optional

	// Nameservers is a list of IP4 and/or IP6 addresses used as DNS
	// nameservers. These are applied globally.
	//
	// Please note global nameservers are only available with the following
	// bootstrap providers: LinuxPrep and Sysprep. The Cloud-Init bootstrap
	// provider supports per-interface nameservers. However, when Cloud-Init
	// is used and UseGlobalNameserversAsDefault is true, the global
	// nameservers will be used when the per-interface nameservers is not
	// provided.
	//
	// Please note that Linux allows only three nameservers
	// (https://linux.die.net/man/5/resolv.conf).
	Nameservers []string `json:"nameservers,omitempty"`

	// +optional

	// SearchDomains is a list of search domains used when resolving IP
	// addresses with DNS. These are applied globally.
	//
	// Please note global search domains are only available with the following
	// bootstrap providers: LinuxPrep and Sysprep. The Cloud-Init bootstrap
	// provider supports per-interface search domains. However, when Cloud-Init
	// is used and UseGlobalSearchDomainsAsDefault is true, the global search
	// domains will be used when the per-interface search domains is not provided.
	SearchDomains []string `json:"searchDomains,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=10

	// Interfaces is the list of network interfaces used by this VM.
	//
	// If the Interfaces field is empty and the Disabled field is false, then
	// a default interface with the name eth0 will be created.
	//
	// The maximum number of network interface allowed is 10 because a vSphere
	// virtual machine may not have more than 10 virtual ethernet card devices.
	Interfaces []VirtualMachineNetworkInterfaceSpec `json:"interfaces,omitempty"`
}

// VirtualMachineNetworkDNSStatus describes the observed state of the guest's
// RFC 1034 client-side DNS settings.
type VirtualMachineNetworkDNSStatus struct {
	// +optional

	// DHCP indicates whether or not dynamic host control protocol (DHCP) was
	// used to configure DNS configuration.
	DHCP bool `json:"dhcp,omitempty"`

	// +optional

	// HostName is the host name portion of the DNS name. For example,
	// the "my-vm" part of "my-vm.domain.local".
	HostName string `json:"hostName,omitempty"`

	// +optional

	// DomainName is the domain name portion of the DNS name. For example,
	// the "domain.local" part of "my-vm.domain.local".
	DomainName string `json:"domainName,omitempty"`

	// +optional

	// Nameservers is a list of the IP addresses for the DNS servers to use.
	//
	// IP4 addresses are specified using dotted decimal notation. For example,
	// "192.0.2.1".
	//
	// IP6 addresses are 128-bit addresses represented as eight fields of up to
	// four hexadecimal digits. A colon separates each field (:). For example,
	// 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
	// symbol '::' to represent multiple 16-bit groups of contiguous 0's only
	// once in an address as described in RFC 2373.
	Nameservers []string `json:"nameservers,omitempty"`

	// +optional

	// SearchDomains is a list of domains in which to search for hosts, in the
	// order of preference.
	SearchDomains []string `json:"searchDomains,omitempty"`
}

// VirtualMachineNetworkConfigDNSStatus describes the configured state of the
// RFC 1034 client-side DNS settings.
type VirtualMachineNetworkConfigDNSStatus struct {
	// +optional

	// HostName is the host name portion of the DNS name. For example,
	// the "my-vm" part of "my-vm.domain.local".
	HostName string `json:"hostName,omitempty"`

	// +optional

	// DomainName is the domain name portion of the DNS name. For example,
	// the "domain.local" part of "my-vm.domain.local".
	DomainName string `json:"domainName,omitempty"`

	// +optional

	// Nameservers is a list of the IP addresses for the DNS servers to use.
	//
	// IP4 addresses are specified using dotted decimal notation. For example,
	// "192.0.2.1".
	//
	// IP6 addresses are 128-bit addresses represented as eight fields of up to
	// four hexadecimal digits. A colon separates each field (:). For example,
	// 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
	// symbol '::' to represent multiple 16-bit groups of contiguous 0's only
	// once in an address as described in RFC 2373.
	Nameservers []string `json:"nameservers,omitempty"`

	// +optional

	// SearchDomains is a list of domains in which to search for hosts, in the
	// order of preference.
	SearchDomains []string `json:"searchDomains,omitempty"`
}

// VirtualMachineNetworkDHCPOptionsStatus describes the observed state of
// DHCP options.
type VirtualMachineNetworkDHCPOptionsStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=key

	// Config describes platform-dependent settings for the DHCP client.
	//
	// The key part is a unique number while the value part is the platform
	// specific configuration command. For example on Linux and BSD systems
	// using the file dhclient.conf output would be reported at system scope:
	// key='1', value='timeout 60;' key='2', value='reboot 10;'. The output
	// reported per interface would be:
	// key='1', value='prepend domain-name-servers 192.0.2.1;'
	// key='2', value='require subnet-mask, domain-name-servers;'.
	Config []vmopv1common.KeyValuePair `json:"config,omitempty"`

	// +optional

	// Enabled reports the status of the DHCP client services.
	Enabled bool `json:"enabled,omitempty"`
}

// VirtualMachineNetworkConfigDHCPOptionsStatus describes the configured
// DHCP options.
type VirtualMachineNetworkConfigDHCPOptionsStatus struct {
	// +optional

	// Enabled describes whether DHCP is enabled.
	Enabled bool `json:"enabled,omitempty"`
}

// VirtualMachineNetworkDHCPStatus describes the observed state of the
// client-side, system-wide DHCP settings for IP4 and IP6.
type VirtualMachineNetworkDHCPStatus struct {
	// +optional

	// IP4 describes the observed state of the IP4 DHCP client settings.
	IP4 VirtualMachineNetworkDHCPOptionsStatus `json:"ip4,omitempty"`

	// +optional

	// IP6 describes the observed state of the IP6 DHCP client settings.
	IP6 VirtualMachineNetworkDHCPOptionsStatus `json:"ip6,omitempty"`
}

// VirtualMachineNetworkConfigDHCPStatus describes the configured state of the
// system-wide DHCP settings for IP4 and IP6.
type VirtualMachineNetworkConfigDHCPStatus struct {
	// +optional

	// IP4 describes the configured state of the IP4 DHCP settings.
	IP4 *VirtualMachineNetworkConfigDHCPOptionsStatus `json:"ip4,omitempty"`

	// +optional

	// IP6 describes the configured state of the IP6 DHCP settings.
	IP6 *VirtualMachineNetworkConfigDHCPOptionsStatus `json:"ip6,omitempty"`
}

// VirtualMachineNetworkIPRouteGatewayStatus describes the observed state of
// a guest network's IP route's next hop gateway.
type VirtualMachineNetworkIPRouteGatewayStatus struct {
	// +optional

	// Device is the name of the device in the guest for which this gateway
	// applies.
	Device string `json:"device,omitempty"`

	// +optional

	// Address is the IP4 or IP6 address of the gateway.
	Address string `json:"address,omitempty"`
}

// VirtualMachineNetworkIPRouteStatus describes the observed state of a
// guest network's IP routes.
type VirtualMachineNetworkIPRouteStatus struct {
	// +optional

	// Gateway describes where to send the packets to next.
	Gateway VirtualMachineNetworkIPRouteGatewayStatus `json:"gateway,omitempty"`

	// +optional

	// NetworkAddress is the IP4 or IP6 address of the destination network.
	//
	// Addresses include the network's prefix length, ex. 192.168.0.0/24 or
	// 2001:DB8:101::230:6eff:fe04:d9ff::/64.
	//
	// IP6 addresses are 128-bit addresses represented as eight fields of up to
	// four hexadecimal digits. A colon separates each field (:). For example,
	// 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of symbol
	// '::' to represent multiple 16-bit groups of contiguous 0's only once in
	// an address as described in RFC 2373.
	NetworkAddress string `json:"networkAddress,omitempty"`
}

// VirtualMachineNetworkRouteStatus describes the observed state of a
// guest network's routes.
type VirtualMachineNetworkRouteStatus struct {
	// +optional

	// IPRoutes contain the VM's routing tables for all address families.
	IPRoutes []VirtualMachineNetworkIPRouteStatus `json:"ipRoutes,omitempty"`
}

// VirtualMachineNetworkInterfaceIPAddrStatus describes information about a
// specific IP address.
type VirtualMachineNetworkInterfaceIPAddrStatus struct {
	// +optional

	// Address is an IP4 or IP6 address and their network prefix length.
	//
	// An IP4 address is specified using dotted decimal notation. For example,
	// "192.0.2.1".
	//
	// IP6 addresses are 128-bit addresses represented as eight fields of up to
	// four hexadecimal digits. A colon separates each field (:). For example,
	// 2001:DB8:101::230:6eff:fe04:d9ff. The address can also consist of the
	// symbol '::' to represent multiple 16-bit groups of contiguous 0's only
	// once in an address as described in RFC 2373.
	Address string `json:"address,omitempty"`

	// +optional

	// Lifetime describes when this address will expire.
	Lifetime metav1.Time `json:"lifetime,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=dhcp;linklayer;manual;other;random

	// Origin describes how this address was configured.
	Origin string `json:"origin,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=deprecated;duplicate;inaccessible;invalid;preferred;tentative;unknown

	// State describes the state of this IP address.
	State string `json:"state,omitempty"`
}

// VirtualMachineNetworkInterfaceIPStatus describes the observed state of a
// VM's network interface's IP configuration.
type VirtualMachineNetworkInterfaceIPStatus struct {
	// +optional

	// AutoConfigurationEnabled describes whether or not ICMPv6 router
	// solicitation requests are enabled or disabled from a given interface.
	//
	// These requests acquire an IP6 address and default gateway route from
	// zero-to-many routers on the connected network.
	//
	// If not set then ICMPv6 is not available on this VM.
	AutoConfigurationEnabled *bool `json:"autoConfigurationEnabled,omitempty"`

	// +optional

	// DHCP describes the VM's observed, client-side, interface-specific DHCP
	// options.
	DHCP *VirtualMachineNetworkDHCPStatus `json:"dhcp,omitempty"`

	// +optional

	// Addresses describes observed IP addresses for this interface.
	Addresses []VirtualMachineNetworkInterfaceIPAddrStatus `json:"addresses,omitempty"`

	// +optional

	// MACAddr describes the observed MAC address for this interface.
	MACAddr string `json:"macAddr,omitempty"`
}

// VirtualMachineNetworkConfigInterfaceIPStatus describes the configured state
// of a VM's network interface's IP configuration.
type VirtualMachineNetworkConfigInterfaceIPStatus struct {
	// +optional

	// DHCP describes the interface's configured DHCP options.
	DHCP *VirtualMachineNetworkConfigDHCPStatus `json:"dhcp,omitempty"`

	// +optional

	// Addresses describes configured IP addresses for this interface.
	// Addresses include the network's prefix length, ex. 192.168.0.0/24 or
	// 2001:DB8:101::230:6eff:fe04:d9ff::/64.
	Addresses []string `json:"addresses,omitempty"`

	// +optional

	// Gateway4 describes the interface's configured, default, IP4 gateway.
	//
	// Please note the IP address include the network prefix length, ex.
	// 192.168.0.1/24.
	Gateway4 string `json:"gateway4,omitempty"`

	// +optional

	// Gateway6 describes the interface's configured, default, IP6 gateway.
	//
	// Please note the IP address includes the network prefix length, ex.
	// 2001:db8:101::1/64.
	Gateway6 string `json:"gateway6,omitempty"`
}

// VirtualMachineNetworkInterfaceStatus describes the observed state of a
// VM's network interface.
type VirtualMachineNetworkInterfaceStatus struct {
	// +optional

	// Name describes the corresponding network interface with the same name
	// in the VM's desired network interface list. If unset, then there is no
	// corresponding entry for this interface.
	//
	// Please note this name is not necessarily related to the name of the
	// device as it is surfaced inside of the guest.
	Name string `json:"name,omitempty"`

	// +optional

	// DeviceKey describes the unique hardware device key of this network
	// interface.
	DeviceKey int32 `json:"deviceKey,omitempty"`

	// +optional

	// IP describes the observed state of the interface's IP configuration.
	IP *VirtualMachineNetworkInterfaceIPStatus `json:"ip,omitempty"`

	// +optional

	// DNS describes the observed state of the interface's DNS configuration.
	DNS *VirtualMachineNetworkDNSStatus `json:"dns,omitempty"`
}

// VirtualMachineNetworkConfigInterfaceStatus describes the configured state of
// network interface.
type VirtualMachineNetworkConfigInterfaceStatus struct {
	// +optional

	// Name describes the corresponding network interface with the same name
	// in the VM's desired network interface list.
	//
	// Please note this name is not necessarily related to the name of the
	// device as it is surfaced inside of the guest.
	Name string `json:"name,omitempty"`

	// +optional

	// IP describes the interface's configured IP information.
	IP *VirtualMachineNetworkConfigInterfaceIPStatus `json:"ip,omitempty"`

	// +optional

	// DNS describes the interface's configured DNS information.
	DNS *VirtualMachineNetworkConfigDNSStatus `json:"dns,omitempty"`
}

// VirtualMachineNetworkIPStackStatus describes the observed state of a
// VM's IP stack.
type VirtualMachineNetworkIPStackStatus struct {
	// +optional

	// DHCP describes the VM's observed, client-side, system-wide DHCP options.
	DHCP *VirtualMachineNetworkDHCPStatus `json:"dhcp,omitempty"`

	// +optional

	// DNS describes the VM's observed, client-side DNS configuration.
	DNS *VirtualMachineNetworkDNSStatus `json:"dns,omitempty"`

	// +optional

	// IPRoutes contain the VM's routing tables for all address families.
	IPRoutes []VirtualMachineNetworkIPRouteStatus `json:"ipRoutes,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=key

	// KernelConfig describes the observed state of the VM's kernel IP
	// configuration settings.
	//
	// The key part contains a unique number while the value part contains the
	// 'key=value' as provided by the underlying provider. For example, on
	// Linux and/or BSD, the systcl -a output would be reported as:
	// key='5', value='net.ipv4.tcp_keepalive_time = 7200'.
	KernelConfig []vmopv1common.KeyValuePair `json:"kernelConfig,omitempty"`
}

// VirtualMachineNetworkStatus defines the observed state of a VM's
// network configuration.
type VirtualMachineNetworkStatus struct {
	// +optional

	// Config describes the resolved, configured network settings for the VM,
	// such as an interface's IP address obtained from IPAM, or global DNS
	// settings.
	//
	// Please note this information does *not* represent the *observed* network
	// state of the VM, but is intended for situations where someone boots a VM
	// with no appropriate bootstrap engine and needs to know the network config
	// valid for the deployed VM.
	Config *VirtualMachineNetworkConfigStatus `json:"config,omitempty"`

	// +optional

	// HostName describes the observed hostname reported by the VirtualMachine's
	// guest operating system.
	//
	// Please note, this value is only reported if VMware Tools is installed in
	// the guest, and the value may or may not be a fully qualified domain name
	// (FQDN), it simply depends on what is reported by the guest.
	HostName string `json:"hostName,omitempty"`

	// +optional

	// Interfaces describes the status of the VM's network interfaces.
	Interfaces []VirtualMachineNetworkInterfaceStatus `json:"interfaces,omitempty"`

	// +optional

	// IPStacks describes information about the guest's configured IP networking
	// stacks.
	IPStacks []VirtualMachineNetworkIPStackStatus `json:"ipStacks,omitempty"`

	// +optional

	// PrimaryIP4 describes the VM's primary IP4 address.
	//
	// If the bootstrap provider is CloudInit then this value is set to the
	// value of the VM's "guestinfo.local-ipv4" property. Please see
	// https://bit.ly/3NJB534 for more information on how this value is
	// calculated.
	//
	// If the bootstrap provider is anything else then this field is set to the
	// value of the infrastructure VM's "guest.ipAddress" field. Please see
	// https://bit.ly/3Au0jM4 for more information.
	PrimaryIP4 string `json:"primaryIP4,omitempty"`

	// +optional

	// PrimaryIP6 describes the VM's primary IP6 address.
	//
	// If the bootstrap provider is CloudInit then this value is set to the
	// value of the VM's "guestinfo.local-ipv6" property. Please see
	// https://bit.ly/3NJB534 for more information on how this value is
	// calculated.
	//
	// If the bootstrap provider is anything else then this field is set to the
	// value of the infrastructure VM's "guest.ipAddress" field. Please see
	// https://bit.ly/3Au0jM4 for more information.
	PrimaryIP6 string `json:"primaryIP6,omitempty"`
}

type VirtualMachineNetworkConfigStatus struct {
	// +optional

	// Interfaces describes the configured state of the network interfaces.
	Interfaces []VirtualMachineNetworkConfigInterfaceStatus `json:"interfaces,omitempty"`

	// +optional

	// DNS describes the configured state of client-side DNS.
	DNS *VirtualMachineNetworkConfigDNSStatus `json:"dns,omitempty"`
}
