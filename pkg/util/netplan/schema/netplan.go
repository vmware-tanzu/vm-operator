// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    config, err := UnmarshalConfig(bytes)
//    bytes, err = config.Marshal()

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
)

func UnmarshalConfig(data []byte) (Config, error) {
	var r Config
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *Config) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type Config struct {
	Network NetworkConfig `json:"network"`
}

type NetworkConfig struct {
	Bonds        map[string]BondConfig        `json:"bonds,omitempty"`
	Bridges      map[string]BridgeConfig      `json:"bridges,omitempty"`
	DummyDevices map[string]DummyDeviceConfig `json:"dummy-devices,omitempty"`
	Ethernets    map[string]EthernetConfig    `json:"ethernets,omitempty"`
	Renderer     *Renderer                    `json:"renderer,omitempty"`
	Tunnels      map[string]TunnelConfig      `json:"tunnels,omitempty"`
	Version      int64                        `json:"version"`
	Vlans        map[string]VLANConfig        `json:"vlans,omitempty"`
	Vrfs         map[string]VrfsConfig        `json:"vrfs,omitempty"`
	Wifis        map[string]WifiConfig        `json:"wifis,omitempty"`
}

type BondConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// All devices matching this ID list will be added to the bond.
	Interfaces []string `json:"interfaces,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// Customization parameters for special bonding options. Time intervals may need to be
	// expressed as a number of seconds or milliseconds: the default value type is specified
	// below. If necessary, time intervals can be qualified using a time suffix (such as “s” for
	// seconds, “ms” for milliseconds) to allow for more control over its behavior.
	Parameters *BondParameters `json:"parameters,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
}

type AddressMappingClass struct {
	// An IP address label, equivalent to the ip address label command. Currently supported on
	// the networkd backend only.
	Label string `json:"label"`
	// Default: forever. This can be forever or 0 and corresponds to the PreferredLifetime
	// option in systemd-networkd’s Address section. Currently supported on the networkd backend
	// only.
	Lifetime PreferredLifetime `json:"lifetime"`
}

// Several DHCP behavior overrides are available. Most currently only have any effect when
// using the networkd backend, with the exception of use-routes and route-metric.
//
// Overrides only have an effect if the corresponding dhcp4 or dhcp6 is set to true.
//
// If both dhcp4 and dhcp6 are true, the networkd backend requires that dhcp4-overrides and
// dhcp6-overrides contain the same keys and values. If the values do not match, an error
// will be shown and the network configuration will not be applied.
//
// When using the NetworkManager backend, different values may be specified for
// dhcp4-overrides and dhcp6-overrides, and will be applied to the DHCP client processes as
// specified in the netplan YAML.
type DHCPOverrides struct {
	// Use this value for the hostname which is sent to the DHCP server, instead of machine’s
	// hostname. Currently only has an effect on the networkd backend.
	Hostname *string `json:"hostname,omitempty"`
	// Use this value for default metric for automatically-added routes. Use this to prioritize
	// routes for devices by setting a lower metric on a preferred interface. Available for both
	// the networkd and NetworkManager backends.
	RouteMetric *int64 `json:"route-metric,omitempty"`
	// Default: true. When true, the machine’s hostname will be sent to the DHCP server.
	// Currently only has an effect on the networkd backend.
	SendHostname *bool `json:"send-hostname,omitempty"`
	// Default: true. When true, the DNS servers received from the DHCP server will be used and
	// take precedence over any statically configured ones. Currently only has an effect on the
	// networkd backend.
	UseDNS *bool `json:"use-dns,omitempty"`
	// Takes a boolean, or the special value “route”. When true, the domain name received from
	// the DHCP server will be used as DNS search domain over this link, similar to the effect
	// of the Domains= setting. If set to “route”, the domain name received from the DHCP server
	// will be used for routing DNS queries only, but not for searching, similar to the effect
	// of the Domains= setting when the argument is prefixed with “~”.
	UseDomains *string `json:"use-domains,omitempty"`
	// Default: true. When true, the hostname received from the DHCP server will be set as the
	// transient hostname of the system. Currently only has an effect on the networkd backend.
	UseHostname *bool `json:"use-hostname,omitempty"`
	// Default: true. When true, the MTU received from the DHCP server will be set as the MTU of
	// the network interface. When false, the MTU advertised by the DHCP server will be ignored.
	// Currently only has an effect on the networkd backend.
	UseMTU *bool `json:"use-mtu,omitempty"`
	// Default: true. When true, the NTP servers received from the DHCP server will be used by
	// systemd-timesyncd and take precedence over any statically configured ones. Currently only
	// has an effect on the networkd backend.
	UseNTP *bool `json:"use-ntp,omitempty"`
	// Default: true. When true, the routes received from the DHCP server will be installed in
	// the routing table normally. When set to false, routes from the DHCP server will be
	// ignored: in this case, the user is responsible for adding static routes if necessary for
	// correct network operation. This allows users to avoid installing a default gateway for
	// interfaces configured via DHCP. Available for both the networkd and NetworkManager
	// backends.
	UseRoutes *bool `json:"use-routes,omitempty"`
}

// Set DNS servers and search domains, for manual address configuration.
type NameserverConfig struct {
	// A list of IPv4 or IPv6 addresses
	Addresses []string `json:"addresses,omitempty"`
	// A list of search domains.
	Search []string `json:"search,omitempty"`
}

type BondParameters struct {
	// Set the aggregation selection mode. Possible values are stable, bandwidth, and count.
	// This option is only used in 802.3ad mode.
	AdSelect *AdSelect `json:"ad-select,omitempty"`
	// If the bond should drop duplicate frames received on inactive ports, set this option to
	// false. If they should be delivered, set this option to true. The default value is false,
	// and is the desirable behavior in most situations.
	AllSlavesActive *bool `json:"all-slaves-active,omitempty"`
	// Specify whether to use any ARP IP target being up as sufficient for a slave to be
	// considered up; or if all the targets must be up. This is only used for active-backup mode
	// when arp-validate is enabled. Possible values are any and all.
	ARPAllTargets *ARPAllTargets `json:"arp-all-targets,omitempty"`
	// Set the interval value for how frequently ARP link monitoring should happen. The default
	// value is 0, which disables ARP monitoring. For the networkd backend, this maps to the
	// ARPIntervalSec= property. If no time suffix is specified, the value will be interpreted
	// as milliseconds.
	ARPInterval *string `json:"arp-interval,omitempty"`
	// IPs of other hosts on the link which should be sent ARP requests in order to validate
	// that a slave is up. This option is only used when arp-interval is set to a value other
	// than 0. At least one IP address must be given for ARP link monitoring to function. Only
	// IPv4 addresses are supported. You can specify up to 16 IP addresses. The default value is
	// an empty list.
	ARPIPTargets []string `json:"arp-ip-targets,omitempty"`
	// Configure how ARP replies are to be validated when using ARP link monitoring. Possible
	// values are none, active, backup, and all.
	ARPValidate *ARPValidate `json:"arp-validate,omitempty"`
	// Specify the delay before disabling a link once the link has been lost. The default value
	// is 0. This maps to the DownDelaySec= property for the networkd renderer. This option is
	// only valid for the miimon link monitor. If no time suffix is specified, the value will be
	// interpreted as milliseconds.
	DownDelay *string `json:"down-delay,omitempty"`
	// Set whether to set all slaves to the same MAC address when adding them to the bond, or
	// how else the system should handle MAC addresses. The possible values are none, active,
	// and follow.
	FailOverMACPolicy *FailOverMACPolicy `json:"fail-over-mac-policy,omitempty"`
	// Specify how many ARP packets to send after failover. Once a link is up on a new slave, a
	// notification is sent and possibly repeated if this value is set to a number greater than
	// 1. The default value is 1 and valid values are between 1 and 255. This only affects
	// active-backup mode.
	GratuitousARP *int64 `json:"gratuitous-arp,omitempty"`
	// Set the rate at which LACPDUs are transmitted. This is only useful in 802.3ad mode.
	// Possible values are slow (30 seconds, default), and fast (every second).
	LACPRate *LACPRate `json:"lacp-rate,omitempty"`
	// Specify the interval between sending learning packets to each slave. The value range is
	// between 1 and 0x7fffffff. The default value is 1. This option only affects balance-tlb
	// and balance-alb modes. Using the networkd renderer, this field maps to the
	// LearnPacketIntervalSec= property. If no time suffix is specified, the value will be
	// interpreted as seconds.
	LearnPacketInterval *string `json:"learn-packet-interval,omitempty"`
	// Specifies the interval for MII monitoring (verifying if an interface of the bond has
	// carrier). The default is 0; which disables MII monitoring. This is equivalent to the
	// MIIMonitorSec= field for the networkd backend. If no time suffix is specified, the value
	// will be interpreted as milliseconds.
	MiiMonitorInterval *string `json:"mii-monitor-interval,omitempty"`
	// The minimum number of links up in a bond to consider the bond interface to be up.
	MinLinks *int64 `json:"min-links,omitempty"`
	// Set the bonding mode used for the interfaces. The default is balance-rr (round robin).
	// Possible values are balance-rr, active-backup, balance-xor, broadcast, 802.3ad,
	// balance-tlb, and balance-alb. For OpenVSwitch active-backup and the additional modes
	// balance-tcp and balance-slb are supported. #[serde(skip_serializing_if = "Option
	Mode *BondMode `json:"mode,omitempty"`
	// In balance-rr mode, specifies the number of packets to transmit on a slave before
	// switching to the next. When this value is set to 0, slaves are chosen at random.
	// Allowable values are between 0 and 65535. The default value is 1. This setting is only
	// used in balance-rr mode.
	PacketsPerSlave *int64 `json:"packets-per-slave,omitempty"`
	// Specify a device to be used as a primary slave, or preferred device to use as a slave for
	// the bond (ie. the preferred device to send data through), whenever it is available. This
	// only affects active-backup, balance-alb, and balance-tlb modes.
	Primary *string `json:"primary,omitempty"`
	// Set the reselection policy for the primary slave. On failure of the active slave, the
	// system will use this policy to decide how the new active slave will be chosen and how
	// recovery will be handled. The possible values are always, better, and failure.
	PrimaryReselectPolicy *PrimaryReselectPolicy `json:"primary-reselect-policy,omitempty"`
	// In modes balance-rr, active-backup, balance-tlb and balance-alb, a failover can switch
	// IGMP traffic from one slave to another.
	//
	// This parameter specifies how many IGMP membership reports are issued on a failover event.
	// Values range from 0 to 255. 0 disables sending membership reports. Otherwise, the first
	// membership report is sent on failover and subsequent reports are sent at 200ms intervals.
	ResendIGMP *int64 `json:"resend-igmp,omitempty"`
	// Specifies the transmit hash policy for the selection of slaves. This is only useful in
	// balance-xor, 802.3ad and balance-tlb modes. Possible values are layer2, layer3+4,
	// layer2+3, encap2+3, and encap3+4.
	TransmitHashPolicy *TransmitHashPolicy `json:"transmit-hash-policy,omitempty"`
	// Specify the delay before enabling a link once the link is physically up. The default
	// value is 0. This maps to the UpDelaySec= property for the networkd renderer. This option
	// is only valid for the miimon link monitor. If no time suffix is specified, the value will
	// be interpreted as milliseconds.
	UpDelay *string `json:"up-delay,omitempty"`
}

// The routes block defines standard static routes for an interface. At least to must be
// specified. If type is local or nat a default scope of host is assumed. If type is unicast
// and no gateway (via) is given or type is broadcast, multicast or anycast a default scope
// of link is assumend. Otherwise, a global scope is the default setting.
//
// For from, to, and via, both IPv4 and IPv6 addresses are recognized, and must be in the
// form addr/prefixlen or addr.
type RoutingConfig struct {
	// The receive window to be advertised for the route, represented by number of segments.
	// Must be a positive integer value.
	AdvertisedReceiveWindow *int64 `json:"advertised-receive-window,omitempty"`
	// The congestion window to be used for the route, represented by number of segments. Must
	// be a positive integer value.
	CongestionWindow *int64 `json:"congestion-window,omitempty"`
	// Set a source IP address for traffic going through the route. (NetworkManager: as of
	// v1.8.0)
	From *string `json:"from,omitempty"`
	// The relative priority of the route. Must be a positive integer value.
	Metric *int64 `json:"metric,omitempty"`
	// The MTU to be used for the route, in bytes. Must be a positive integer value.
	MTU *int64 `json:"mtu,omitempty"`
	// When set to “true”, specifies that the route is directly connected to the interface.
	// (NetworkManager: as of v1.12.0 for IPv4 and v1.18.0 for IPv6)
	OnLink *bool `json:"on-link,omitempty"`
	// The route scope, how wide-ranging it is to the network. Possible values are “global”,
	// “link”, or “host”.
	Scope *RouteScope `json:"scope,omitempty"`
	// The table number to use for the route. In some scenarios, it may be useful to set routes
	// in a separate routing table. It may also be used to refer to routing policy rules which
	// also accept a table parameter. Allowed values are positive integers starting from 1. Some
	// values are already in use to refer to specific routing tables: see
	// /etc/iproute2/rt_tables. (NetworkManager: as of v1.10.0)
	Table *int64 `json:"table,omitempty"`
	// Destination address for the route.
	To *string `json:"to,omitempty"`
	// The type of route. Valid options are “unicast” (default), “anycast”, “blackhole”,
	// “broadcast”, “local”, “multicast”, “nat”, “prohibit”, “throw”, “unreachable” or
	// “xresolve”.
	Type *RouteType `json:"type,omitempty"`
	// Address to the gateway to use for this route.
	Via *string `json:"via,omitempty"`
}

// The routing-policy block defines extra routing policy for a network, where traffic may be
// handled specially based on the source IP, firewall marking, etc.
//
// For from, to, both IPv4 and IPv6 addresses are recognized, and must be in the form
// addr/prefixlen or addr.
type RoutingPolicy struct {
	// Set a source IP address to match traffic for this policy rule.
	From *string `json:"from,omitempty"`
	// Have this routing policy rule match on traffic that has been marked by the iptables
	// firewall with this value. Allowed values are positive integers starting from 1.
	Mark *int64 `json:"mark,omitempty"`
	// Specify a priority for the routing policy rule, to influence the order in which routing
	// rules are processed. A higher number means lower priority: rules are processed in order
	// by increasing priority number.
	Priority *int64 `json:"priority,omitempty"`
	// The table number to match for the route. In some scenarios, it may be useful to set
	// routes in a separate routing table. It may also be used to refer to routes which also
	// accept a table parameter. Allowed values are positive integers starting from 1. Some
	// values are already in use to refer to specific routing tables: see
	// /etc/iproute2/rt_tables.
	Table int64 `json:"table"`
	// Match on traffic going to the specified destination.
	To *string `json:"to,omitempty"`
	// Match this policy rule based on the type of service number applied to the traffic.
	TypeOfService *string `json:"type-of-service,omitempty"`
}

type BridgeConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// All devices matching this ID list will be added to the bridge. This may be an empty list,
	// in which case the bridge will be brought online with no member interfaces.
	Interfaces []string `json:"interfaces,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// Customization parameters for special bridging options. Time intervals may need to be
	// expressed as a number of seconds or milliseconds: the default value type is specified
	// below. If necessary, time intervals can be qualified using a time suffix (such as “s” for
	// seconds, “ms” for milliseconds) to allow for more control over its behavior.
	Parameters *BridgeParameters `json:"parameters,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
}

// Customization parameters for special bridging options. Time intervals may need to be
// expressed as a number of seconds or milliseconds: the default value type is specified
// below. If necessary, time intervals can be qualified using a time suffix (such as “s” for
// seconds, “ms” for milliseconds) to allow for more control over its behavior.
type BridgeParameters struct {
	// Set the period of time to keep a MAC address in the forwarding database after a packet is
	// received. This maps to the AgeingTimeSec= property when the networkd renderer is used. If
	// no time suffix is specified, the value will be interpreted as seconds.
	AgeingTime *string `json:"ageing-time,omitempty"`
	// Specify the period of time the bridge will remain in Listening and Learning states before
	// getting to the Forwarding state. This field maps to the ForwardDelaySec= property for the
	// networkd renderer. If no time suffix is specified, the value will be interpreted as
	// seconds.
	ForwardDelay *string `json:"forward-delay,omitempty"`
	// Specify the interval between two hello packets being sent out from the root and
	// designated bridges. Hello packets communicate information about the network topology.
	// When the networkd renderer is used, this maps to the HelloTimeSec= property. If no time
	// suffix is specified, the value will be interpreted as seconds.
	HelloTime *string `json:"hello-time,omitempty"`
	// Set the maximum age of a hello packet. If the last hello packet is older than that value,
	// the bridge will attempt to become the root bridge. This maps to the MaxAgeSec= property
	// when the networkd renderer is used. If no time suffix is specified, the value will be
	// interpreted as seconds.
	MaxAge *string `json:"max-age,omitempty"`
	// Set the cost of a path on the bridge. Faster interfaces should have a lower cost. This
	// allows a finer control on the network topology so that the fastest paths are available
	// whenever possible.
	PathCost *int64 `json:"path-cost,omitempty"`
	// Set the port priority to . The priority value is a number between 0 and 63. This metric
	// is used in the designated port and root port selection algorithms.
	PortPriority *int64 `json:"port-priority,omitempty"`
	// Set the priority value for the bridge. This value should be a number between 0 and 65535.
	// Lower values mean higher priority. The bridge with the higher priority will be elected as
	// the root bridge.
	Priority *int64 `json:"priority,omitempty"`
	// Define whether the bridge should use Spanning Tree Protocol. The default value is “true”,
	// which means that Spanning Tree should be used.
	Stp *bool `json:"stp,omitempty"`
}

// Purpose: Use the dummy-devices key to create virtual interfaces.
//
// Structure: The key consists of a mapping of interface names. Dummy devices are virtual
// devices that can be used to route packets to without actually transmitting them.
type DummyDeviceConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
}

// Common properties for physical device types
type EthernetConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (SR-IOV devices only) Delay rebinding of SR-IOV virtual functions to its driver after
	// changing the embedded-switch-mode setting to a later stage. Can be enabled when
	// bonding/VF LAG is in use. Defaults to false.
	DelayVirtualFunctionsRebind *bool `json:"delay-virtual-functions-rebind,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// (SR-IOV devices only) Change the operational mode of the embedded switch of a supported
	// SmartNIC PCI device (e.g. Mellanox ConnectX-5). Possible values are switchdev or legacy,
	// if unspecified the vendor’s default configuration is used.
	EmbeddedSwitchMode *EmbeddedSwitchMode `json:"embedded-switch-mode,omitempty"`
	// (networkd backend only) Whether to emit LLDP packets. Off by default.
	EmitLldp *bool `json:"emit-lldp,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) If set to true, the Generic Receive Offload (GRO) is enabled.
	// When unset, the kernel’s default will be used.
	GenericReceiveOffload *bool `json:"generic-receive-offload,omitempty"`
	// (networkd backend only) If set to true, the Generic Segmentation Offload (GSO) is
	// enabled. When unset, the kernel’s default will be used.
	GenericSegmentationOffload *bool `json:"generic-segmentation-offload,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// (networkd backend only) If set to true, the Generic Receive Offload (GRO) is enabled.
	// When unset, the kernel’s default will be used.
	LargeReceiveOffload *bool `json:"large-receive-offload,omitempty"`
	// (SR-IOV devices only) The link property declares the device as a Virtual Function of the
	// selected Physical Function device, as identified by the given netplan id.
	Link *string `json:"link,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// This selects a subset of available physical devices by various hardware properties. The
	// following configuration will then apply to all matching devices, as soon as they appear.
	// All specified properties must match.
	Match *MatchConfig `json:"match,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// This provides additional configuration for the network device for openvswitch. If
	// openvswitch is not available on the system, netplan treats the presence of openvswitch
	// configuration as an error.
	//
	// Any supported network device that is declared with the openvswitch mapping (or any
	// bond/bridge that includes an interface with an openvswitch configuration) will be created
	// in openvswitch instead of the defined renderer. In the case of a vlan definition declared
	// the same way, netplan will create a fake VLAN bridge in openvswitch with the requested
	// vlan properties.
	Openvswitch *OpenVSwitchConfig `json:"openvswitch,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// (networkd backend only) If set to true, the hardware offload for checksumming of ingress
	// network packets is enabled. When unset, the kernel’s default will be used.
	ReceiveChecksumOffload *bool `json:"receive-checksum-offload,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
	// When matching on unique properties such as path or MAC, or with additional assumptions
	// such as “there will only ever be one wifi device”, match rules can be written so that
	// they only match one device. Then this property can be used to give that device a more
	// specific/desirable/nicer name than the default from udev’s ifnames. Any additional device
	// that satisfies the match rules will then fail to get renamed and keep the original kernel
	// name (and dmesg will show an error).
	SetName *string `json:"set-name,omitempty"`
	// (networkd backend only) If set to true, the TCP Segmentation Offload (TSO) is enabled.
	// When unset, the kernel’s default will be used.
	TCPSegmentationOffload *bool `json:"tcp-segmentation-offload,omitempty"`
	// (networkd backend only) If set to true, the TCP6 Segmentation Offload
	// (tx-tcp6-segmentation) is enabled. When unset, the kernel’s default will be used.
	Tcp6SegmentationOffload *bool `json:"tcp6-segmentation-offload,omitempty"`
	// (networkd backend only) If set to true, the hardware offload for checksumming of egress
	// network packets is enabled. When unset, the kernel’s default will be used.
	TransmitChecksumOffload *bool `json:"transmit-checksum-offload,omitempty"`
	// (SR-IOV devices only) In certain special cases VFs might need to be configured outside of
	// netplan. For such configurations virtual-function-count can be optionally used to set an
	// explicit number of Virtual Functions for the given Physical Function. If unset, the
	// default is to create only as many VFs as are defined in the netplan configuration. This
	// should be used for special cases only.
	VirtualFunctionCount *int64 `json:"virtual-function-count,omitempty"`
	// Enable wake on LAN. Off by default.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting wake on LAN.
	Wakeonlan *bool `json:"wakeonlan,omitempty"`
}

// This selects a subset of available physical devices by various hardware properties. The
// following configuration will then apply to all matching devices, as soon as they appear.
// All specified properties must match.
type MatchConfig struct {
	// Kernel driver name, corresponding to the DRIVER udev property. A sequence of globs is
	// supported, any of which must match. Matching on driver is only supported with networkd.
	Driver []string `json:"driver,omitempty"`
	// Device’s MAC address in the form “XX:XX:XX:XX:XX:XX”. Globs are not allowed.
	Macaddress *string `json:"macaddress,omitempty"`
	// Current interface name. Globs are supported, and the primary use case for matching on
	// names, as selecting one fixed name can be more easily achieved with having no match: at
	// all and just using the ID (see above). (NetworkManager: as of v1.14.0)
	Name *string `json:"name,omitempty"`
}

// This provides additional configuration for the network device for openvswitch. If
// openvswitch is not available on the system, netplan treats the presence of openvswitch
// configuration as an error.
//
// Any supported network device that is declared with the openvswitch mapping (or any
// bond/bridge that includes an interface with an openvswitch configuration) will be created
// in openvswitch instead of the defined renderer. In the case of a vlan definition declared
// the same way, netplan will create a fake VLAN bridge in openvswitch with the requested
// vlan properties.
type OpenVSwitchConfig struct {
	// Valid for bridge interfaces. Specify an external OpenFlow controller.
	Controller *ControllerConfig `json:"controller,omitempty"`
	// Passed-through directly to OpenVSwitch
	ExternalIDS *string `json:"external-ids,omitempty"`
	// Valid for bridge interfaces. Accepts secure or standalone (the default).
	FailMode *FailMode `json:"fail-mode,omitempty"`
	// Valid for bond interfaces. Accepts active, passive or off (the default).
	LACP *LACP `json:"lacp,omitempty"`
	// Valid for bridge interfaces. False by default.
	McastSnooping *bool `json:"mcast-snooping,omitempty"`
	// Passed-through directly to OpenVSwitch
	OtherConfig *string `json:"other-config,omitempty"`
	// OpenvSwitch patch ports. Each port is declared as a pair of names which can be referenced
	// as interfaces in dependent virtual devices (bonds, bridges).
	Ports []string `json:"ports,omitempty"`
	// Valid for bridge interfaces or the network section. List of protocols to be used when
	// negotiating a connection with the controller. Accepts OpenFlow10, OpenFlow11, OpenFlow12,
	// OpenFlow13, OpenFlow14, OpenFlow15 and OpenFlow16.
	Protocols []OpenFlowProtocol `json:"protocols,omitempty"`
	// Valid for bridge interfaces. False by default.
	RTSP *bool `json:"rtsp,omitempty"`
	// Valid for global openvswitch settings. Options for configuring SSL server endpoint for
	// the switch.
	SSL *SSLConfig `json:"ssl,omitempty"`
}

// Valid for bridge interfaces. Specify an external OpenFlow controller.
type ControllerConfig struct {
	// Set the list of addresses to use for the controller targets. The syntax of these
	// addresses is as defined in ovs-vsctl(8). Example: addresses: [tcp:127.0.0.1:6653,
	// "ssl:[fe80::1234%eth0]:6653"]
	Addresses []string `json:"addresses,omitempty"`
	// Set the connection mode for the controller. Supported options are in-band and
	// out-of-band. The default is in-band.
	ConnectionMode *ConnectionMode `json:"connection-mode,omitempty"`
}

// Valid for global openvswitch settings. Options for configuring SSL server endpoint for
// the switch.
type SSLConfig struct {
	// Path to a file containing the CA certificate to be used.
	CACERT *string `json:"ca-cert,omitempty"`
	// Path to a file containing the server certificate.
	Certificate *string `json:"certificate,omitempty"`
	// Path to a file containing the private key for the server.
	PrivateKey *string `json:"private-key,omitempty"`
}

// Tunnels allow traffic to pass as if it was between systems on the same local network,
// although systems may be far from each other but reachable via the Internet. They may be
// used to support IPv6 traffic on a network where the ISP does not provide the service, or
// to extend and “connect” separate local networks. Please see
// <https://en.wikipedia.org/wiki/Tunneling_protocol> for more general information about
// tunnels.
type TunnelConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// Define keys to use for the tunnel. The key can be a number or a dotted quad (an IPv4
	// address). For wireguard it can be a base64-encoded private key or (as of networkd v242+)
	// an absolute path to a file, containing the private key (since 0.100). It is used for
	// identification of IP transforms. This is only required for vti and vti6 when using the
	// networkd backend, and for gre or ip6gre tunnels when using the NetworkManager backend.
	//
	// This field may be used as a scalar (meaning that a single key is specified and to be used
	// for input, output and private key), or as a mapping, where you can further specify
	// input/output/private.
	Key *TunnelKey `json:"key,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Defines the address of the local endpoint of the tunnel.
	Local *string `json:"local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Firewall mark for outgoing WireGuard packets from this interface, optional.
	Mark *string `json:"mark,omitempty"`
	// Defines the tunnel mode. Valid options are sit, gre, ip6gre, ipip, ipip6, ip6ip6, vti,
	// vti6 and wireguard. Additionally, the networkd backend also supports gretap and ip6gretap
	// modes. In addition, the NetworkManager backend supports isatap tunnels.
	Mode *TunnelMode `json:"mode,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// A list of peers
	Peers []WireGuardPeer `json:"peers"`
	// UDP port to listen at or auto. Optional, defaults to auto.
	Port *string `json:"port,omitempty"`
	// Defines the address of the remote endpoint of the tunnel.
	Remote *string `json:"remote,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
	// Defines the TTL of the tunnel.
	TTL *int64 `json:"ttl,omitempty"`
}

type TunnelKey struct {
	Simple  *string  `json:"Simple,omitempty"`
	Complex *Complex `json:"Complex,omitempty"`
}

type Complex struct {
	// The input key for the tunnel
	Input *string `json:"input,omitempty"`
	// The output key for the tunnel
	Output *string `json:"output,omitempty"`
	// A base64-encoded private key required for WireGuard tunnels. When the systemd-networkd
	// backend (v242+) is used, this can also be an absolute path to a file containing the
	// private key.
	Private *string `json:"private,omitempty"`
}

// A list of peers
type WireGuardPeer struct {
	// A list of IP (v4 or v6) addresses with CIDR masks from which this peer is allowed to send
	// incoming traffic and to which outgoing traffic for this peer is directed. The catch-all
	// 0.0.0.0/0 may be specified for matching all IPv4 addresses, and ::/0 may be specified for
	// matching all IPv6 addresses.
	AllowedIPS []string `json:"allowed-ips,omitempty"`
	// Remote endpoint IPv4/IPv6 address or a hostname, followed by a colon and a port number.
	Endpoint *string `json:"endpoint,omitempty"`
	// An interval in seconds, between 1 and 65535 inclusive, of how often to send an
	// authenticated empty packet to the peer for the purpose of keeping a stateful firewall or
	// NAT mapping valid persistently. Optional.
	Keepalive *int64 `json:"keepalive,omitempty"`
	// Define keys to use for the WireGuard peers.
	Keys *WireGuardPeerKey `json:"keys,omitempty"`
}

// Define keys to use for the WireGuard peers.
//
// This field can be used as a mapping, where you can further specify the public and shared
// keys.
type WireGuardPeerKey struct {
	// A base64-encoded public key, required for WireGuard peers.
	Public *string `json:"public,omitempty"`
	// A base64-encoded preshared key. Optional for WireGuard peers. When the systemd-networkd
	// backend (v242+) is used, this can also be an absolute path to a file containing the
	// preshared key.
	Shared *string `json:"shared,omitempty"`
}

type VLANConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// VLAN ID, a number between 0 and 4094.
	ID *int64 `json:"id,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// netplan ID of the underlying device definition on which this VLAN gets created.
	Link *string `json:"link,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
}

// Purpose: Use the vrfs key to create Virtual Routing and Forwarding (VRF) interfaces.
//
// Structure: The key consists of a mapping of VRF interface names. The interface used in
// the link option (enp5s0 in the example below) must also be defined in the Netplan
// configuration. The general configuration structure for VRFs is shown below.
type VrfsConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// All devices matching this ID list will be added to the VRF. This may be an empty list, in
	// which case the VRF will be brought online with no member interfaces.
	Interfaces []string `json:"interfaces"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
	// The numeric routing table identifier. This setting is compulsory.
	Table int64 `json:"table"`
}

// Common properties for physical device types
type WifiConfig struct {
	// Accept Router Advertisement that would have the kernel configure IPv6 by itself. When
	// enabled, accept Router Advertisements. When disabled, do not respond to Router
	// Advertisements. If unset use the host kernel default setting.
	AcceptRa *bool `json:"accept-ra,omitempty"`
	// This provides pre-configured connections to NetworkManager. Note that users can of course
	// select other access points/SSIDs. The keys of the mapping are the SSIDs, and the values
	// are mappings with the following supported properties:
	AccessPoints map[string]AccessPointConfig `json:"access-points,omitempty"`
	// Allows specifying the management policy of the selected interface. By default, netplan
	// brings up any configured interface if possible. Using the activation-mode setting users
	// can override that behavior by either specifying manual, to hand over control over the
	// interface state to the administrator or (for networkd backend only) off to force the link
	// in a down state at all times. Any interface with activation-mode defined is implicitly
	// considered optional. Supported officially as of networkd v248+.
	ActivationMode *Lowercase `json:"activation-mode,omitempty"`
	// Add static addresses to the interface in addition to the ones received through DHCP or
	// RA. Each sequence entry is in CIDR notation, i. e. of the form addr/prefixlen. addr is an
	// IPv4 or IPv6 address as recognized by inet_pton(3) and prefixlen the number of bits of
	// the subnet.
	//
	// For virtual devices (bridges, bonds, vlan) if there is no address configured and DHCP is
	// disabled, the interface may still be brought online, but will not be addressable from the
	// network.
	Addresses []AddressMapping `json:"addresses,omitempty"`
	// Designate the connection as “critical to the system”, meaning that special care will be
	// taken by to not release the assigned IP when the daemon is restarted. (not recognized by
	// NetworkManager)
	Critical *bool `json:"critical,omitempty"`
	// (networkd backend only) Sets the source of DHCPv4 client identifier. If mac is specified,
	// the MAC address of the link is used. If this option is omitted, or if duid is specified,
	// networkd will generate an RFC4361-compliant client identifier for the interface by
	// combining the link’s IAID and DUID.
	DHCPIdentifier *string `json:"dhcp-identifier,omitempty"`
	// Enable DHCP for IPv4. Off by default.
	Dhcp4 *bool `json:"dhcp4,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp4Overrides *DHCPOverrides `json:"dhcp4-overrides,omitempty"`
	// Enable DHCP for IPv6. Off by default. This covers both stateless DHCP - where the DHCP
	// server supplies information like DNS nameservers but not the IP address - and stateful
	// DHCP, where the server provides both the address and the other information.
	//
	// If you are in an IPv6-only environment with completely stateless autoconfiguration (SLAAC
	// with RDNSS), this option can be set to cause the interface to be brought up. (Setting
	// accept-ra alone is not sufficient.) Autoconfiguration will still honour the contents of
	// the router advertisement and only use DHCP if requested in the RA.
	//
	// Note that rdnssd(8) is required to use RDNSS with networkd. No extra software is required
	// for NetworkManager.
	Dhcp6 *bool `json:"dhcp6,omitempty"`
	// (networkd backend only) Overrides default DHCP behavior
	Dhcp6Overrides *DHCPOverrides `json:"dhcp6-overrides,omitempty"`
	// (networkd backend only) Whether to emit LLDP packets. Off by default.
	EmitLldp *bool `json:"emit-lldp,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway4 *string `json:"gateway4,omitempty"`
	// Deprecated, see Default routes. Set default gateway for IPv4/6, for manual address
	// configuration. This requires setting addresses too. Gateway IPs must be in a form
	// recognized by inet_pton(3). There should only be a single gateway per IP address family
	// set in your global config, to make it unambiguous. If you need multiple default routes,
	// please define them via routing-policy.
	Gateway6 *string `json:"gateway6,omitempty"`
	// (networkd backend only) If set to true, the Generic Receive Offload (GRO) is enabled.
	// When unset, the kernel’s default will be used.
	GenericReceiveOffload *bool `json:"generic-receive-offload,omitempty"`
	// (networkd backend only) If set to true, the Generic Segmentation Offload (GSO) is
	// enabled. When unset, the kernel’s default will be used.
	GenericSegmentationOffload *bool `json:"generic-segmentation-offload,omitempty"`
	// (networkd backend only) Allow the specified interface to be configured even if it has no
	// carrier.
	IgnoreCarrier *bool `json:"ignore-carrier,omitempty"`
	// Configure method for creating the address for use with RFC4862 IPv6 Stateless Address
	// Autoconfiguration (only supported with NetworkManager backend). Possible values are eui64
	// or stable-privacy.
	Ipv6AddressGeneration *Ipv6AddressGeneration `json:"ipv6-address-generation,omitempty"`
	// Define an IPv6 address token for creating a static interface identifier for IPv6
	// Stateless Address Autoconfiguration. This is mutually exclusive with
	// ipv6-address-generation.
	Ipv6AddressToken *string `json:"ipv6-address-token,omitempty"`
	// Set the IPv6 MTU (only supported with networkd backend). Note that needing to set this is
	// an unusual requirement.
	Ipv6MTU *int64 `json:"ipv6-mtu,omitempty"`
	// Enable IPv6 Privacy Extensions (RFC 4941) for the specified interface, and prefer
	// temporary addresses. Defaults to false - no privacy extensions. There is currently no way
	// to have a private address but prefer the public address.
	Ipv6Privacy *bool `json:"ipv6-privacy,omitempty"`
	// (networkd backend only) If set to true, the Generic Receive Offload (GRO) is enabled.
	// When unset, the kernel’s default will be used.
	LargeReceiveOffload *bool `json:"large-receive-offload,omitempty"`
	// Configure the link-local addresses to bring up. Valid options are ‘ipv4’ and ‘ipv6’,
	// which respectively allow enabling IPv4 and IPv6 link local addressing. If this field is
	// not defined, the default is to enable only IPv6 link-local addresses. If the field is
	// defined but configured as an empty set, IPv6 link-local addresses are disabled as well as
	// IPv4 link- local addresses.
	//
	// This feature enables or disables link-local addresses for a protocol, but the actual
	// implementation differs per backend. On networkd, this directly changes the behavior and
	// may add an extra address on an interface. When using the NetworkManager backend, enabling
	// link-local has no effect if the interface also has DHCP enabled.
	//
	// Example to enable only IPv4 link-local: `link-local: [ ipv4 ]` Example to enable all
	// link-local addresses: `link-local: [ ipv4, ipv6 ]` Example to disable all link-local
	// addresses: `link-local: [ ]`
	LinkLocal []string `json:"link-local,omitempty"`
	// Set the device’s MAC address. The MAC address must be in the form “XX:XX:XX:XX:XX:XX”.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MAC addresses.
	Macaddress *string `json:"macaddress,omitempty"`
	// This selects a subset of available physical devices by various hardware properties. The
	// following configuration will then apply to all matching devices, as soon as they appear.
	// All specified properties must match.
	Match *MatchConfig `json:"match,omitempty"`
	// Set the Maximum Transmission Unit for the interface. The default is 1500. Valid values
	// depend on your network interface.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting MTU.
	MTU *int64 `json:"mtu,omitempty"`
	// Set DNS servers and search domains, for manual address configuration.
	Nameservers *NameserverConfig `json:"nameservers,omitempty"`
	// This provides additional configuration for the network device for openvswitch. If
	// openvswitch is not available on the system, netplan treats the presence of openvswitch
	// configuration as an error.
	//
	// Any supported network device that is declared with the openvswitch mapping (or any
	// bond/bridge that includes an interface with an openvswitch configuration) will be created
	// in openvswitch instead of the defined renderer. In the case of a vlan definition declared
	// the same way, netplan will create a fake VLAN bridge in openvswitch with the requested
	// vlan properties.
	Openvswitch *OpenVSwitchConfig `json:"openvswitch,omitempty"`
	// An optional device is not required for booting. Normally, networkd will wait some time
	// for device to become configured before proceeding with booting. However, if a device is
	// marked as optional, networkd will not wait for it. This is only supported by networkd,
	// and the default is false.
	Optional *bool `json:"optional,omitempty"`
	// Specify types of addresses that are not required for a device to be considered online.
	// This changes the behavior of backends at boot time to avoid waiting for addresses that
	// are marked optional, and thus consider the interface as “usable” sooner. This does not
	// disable these addresses, which will be brought up anyway.
	OptionalAddresses []string `json:"optional-addresses,omitempty"`
	// (networkd backend only) If set to true, the hardware offload for checksumming of ingress
	// network packets is enabled. When unset, the kernel’s default will be used.
	ReceiveChecksumOffload *bool `json:"receive-checksum-offload,omitempty"`
	// Use the given networking backend for this definition. Currently supported are networkd
	// and NetworkManager. This property can be specified globally in network:, for a device
	// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
	//
	// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
	// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
	// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
	// for it. There can be only one defined per VF.
	Renderer *Renderer `json:"renderer,omitempty"`
	// Configure static routing for the device
	Routes []RoutingConfig `json:"routes,omitempty"`
	// Configure policy routing for the device
	RoutingPolicy []RoutingPolicy `json:"routing-policy,omitempty"`
	// When matching on unique properties such as path or MAC, or with additional assumptions
	// such as “there will only ever be one wifi device”, match rules can be written so that
	// they only match one device. Then this property can be used to give that device a more
	// specific/desirable/nicer name than the default from udev’s ifnames. Any additional device
	// that satisfies the match rules will then fail to get renamed and keep the original kernel
	// name (and dmesg will show an error).
	SetName *string `json:"set-name,omitempty"`
	// (networkd backend only) If set to true, the TCP Segmentation Offload (TSO) is enabled.
	// When unset, the kernel’s default will be used.
	TCPSegmentationOffload *bool `json:"tcp-segmentation-offload,omitempty"`
	// (networkd backend only) If set to true, the TCP6 Segmentation Offload
	// (tx-tcp6-segmentation) is enabled. When unset, the kernel’s default will be used.
	Tcp6SegmentationOffload *bool `json:"tcp6-segmentation-offload,omitempty"`
	// (networkd backend only) If set to true, the hardware offload for checksumming of egress
	// network packets is enabled. When unset, the kernel’s default will be used.
	TransmitChecksumOffload *bool `json:"transmit-checksum-offload,omitempty"`
	// Enable wake on LAN. Off by default.
	//
	// Note: This will not work reliably for devices matched by name only and rendered by
	// networkd, due to interactions with device renaming in udev. Match devices by MAC when
	// setting wake on LAN.
	Wakeonlan *bool `json:"wakeonlan,omitempty"`
	// This enables WakeOnWLan on supported devices. Not all drivers support all options. May be
	// any combination of any, disconnect, magic_pkt, gtk_rekey_failure, eap_identity_req,
	// four_way_handshake, rfkill_release or tcp (NetworkManager only). Or the exclusive default
	// flag (the default).
	Wakeonwlan []WakeOnWLAN `json:"wakeonwlan,omitempty"`
}

type AccessPointConfig struct {
	Auth *AuthConfig `json:"auth,omitempty"`
	// Possible bands are 5GHz (for 5GHz 802.11a) and 2.4GHz (for 2.4GHz 802.11), do not
	// restrict the 802.11 frequency band of the network if unset (the default).
	Band *WirelessBand `json:"band,omitempty"`
	// If specified, directs the device to only associate with the given access point.
	Bssid *string `json:"bssid,omitempty"`
	// Wireless channel to use for the Wi-Fi connection. Because channel numbers overlap between
	// bands, this property takes effect only if the band property is also set.
	Channel *int64 `json:"channel,omitempty"`
	// Set to true to change the SSID scan technique for connecting to hidden WiFi networks.
	// Note this may have slower performance compared to false (the default) when connecting to
	// publicly broadcast SSIDs.
	Hidden *bool `json:"hidden,omitempty"`
	// Possible access point modes are infrastructure (the default), ap (create an access point
	// to which other devices can connect), and adhoc (peer to peer networks without a central
	// access point). ap is only supported with NetworkManager.
	Mode *AccessPointMode `json:"mode,omitempty"`
	// Enable WPA2 authentication and set the passphrase for it. If neither this nor an auth
	// block are given, the network is assumed to be open. The setting
	Password *string `json:"password,omitempty"`
}

// Netplan supports advanced authentication settings for ethernet and wifi interfaces, as
// well as individual wifi networks, by means of the auth block.
type AuthConfig struct {
	// The identity to pass over the unencrypted channel if the chosen EAP method supports
	// passing a different tunnelled identity.
	AnonymousIdentity *string `json:"anonymous-identity,omitempty"`
	// Path to a file with one or more trusted certificate authority (CA) certificates.
	CACertificate *string `json:"ca-certificate,omitempty"`
	// Path to a file containing the certificate to be used by the client during authentication.
	ClientCertificate *string `json:"client-certificate,omitempty"`
	// Path to a file containing the private key corresponding to client-certificate.
	ClientKey *string `json:"client-key,omitempty"`
	// Password to use to decrypt the private key specified in client-key if it is encrypted.
	ClientKeyPassword *string `json:"client-key-password,omitempty"`
	// The identity to use for EAP.
	Identity *string `json:"identity,omitempty"`
	// The supported key management modes are none (no key management); psk (WPA with pre-shared
	// key, common for home wifi); eap (WPA with EAP, common for enterprise wifi); and 802.1x
	// (used primarily for wired Ethernet connections).
	KeyManagement *KeyManagmentMode `json:"key-management,omitempty"`
	// The EAP method to use. The supported EAP methods are tls (TLS), peap (Protected EAP), and
	// ttls (Tunneled TLS).
	Method *AuthMethod `json:"method,omitempty"`
	// The password string for EAP, or the pre-shared key for WPA-PSK.
	Password *string `json:"password,omitempty"`
	// Phase 2 authentication mechanism.
	Phase2Auth *string `json:"phase2-auth,omitempty"`
}

// Allows specifying the management policy of the selected interface. By default, netplan
// brings up any configured interface if possible. Using the activation-mode setting users
// can override that behavior by either specifying manual, to hand over control over the
// interface state to the administrator or (for networkd backend only) off to force the link
// in a down state at all times. Any interface with activation-mode defined is implicitly
// considered optional. Supported officially as of networkd v248+.
type Lowercase string

const (
	LowercaseOff Lowercase = "Off"
	Manual       Lowercase = "Manual"
)

// Default: forever. This can be forever or 0 and corresponds to the PreferredLifetime
// option in systemd-networkd’s Address section. Currently supported on the networkd backend
// only.
type PreferredLifetime string

const (
	Forever PreferredLifetime = "forever"
	The0    PreferredLifetime = "0"
)

type Ipv6AddressGeneration string

const (
	Eui64         Ipv6AddressGeneration = "eui64"
	StablePrivacy Ipv6AddressGeneration = "stable-privacy"
)

// Specify whether to use any ARP IP target being up as sufficient for a slave to be
// considered up; or if all the targets must be up. This is only used for active-backup mode
// when arp-validate is enabled. Possible values are any and all.
type ARPAllTargets string

const (
	ARPAllTargetsAll ARPAllTargets = "all"
	ARPAllTargetsAny ARPAllTargets = "any"
)

// Configure how ARP replies are to be validated when using ARP link monitoring. Possible
// values are none, active, backup, and all.
type ARPValidate string

const (
	ARPValidateAll  ARPValidate = "all"
	ARPValidateNone ARPValidate = "none"
	Active          ARPValidate = "active"
	Backup          ARPValidate = "backup"
)

// Set the aggregation selection mode. Possible values are stable, bandwidth, and count.
// This option is only used in 802.3ad mode.
type AdSelect string

const (
	Bandwidth AdSelect = "bandwidth"
	Count     AdSelect = "count"
	Stable    AdSelect = "stable"
)

// Set whether to set all slaves to the same MAC address when adding them to the bond, or
// how else the system should handle MAC addresses. The possible values are none, active,
// and follow.
type FailOverMACPolicy string

const (
	Activv                FailOverMACPolicy = "activv"
	FailOverMACPolicyNone FailOverMACPolicy = "none"
	Follow                FailOverMACPolicy = "follow"
)

// Set the rate at which LACPDUs are transmitted. This is only useful in 802.3ad mode.
// Possible values are slow (30 seconds, default), and fast (every second).
type LACPRate string

const (
	Fast LACPRate = "fast"
	Slow LACPRate = "slow"
)

// 802.3ad
type BondMode string

const (
	ActiveBackup      BondMode = "active-backup"
	BalanceAlb        BondMode = "balance-alb"
	BalanceRr         BondMode = "balance-rr"
	BalanceTlb        BondMode = "balance-tlb"
	BalanceXor        BondMode = "balance-xor"
	BondModeBroadcast BondMode = "broadcast"
	The8023Ad         BondMode = "802.3ad"
)

// Set the reselection policy for the primary slave. On failure of the active slave, the
// system will use this policy to decide how the new active slave will be chosen and how
// recovery will be handled. The possible values are always, better, and failure.
type PrimaryReselectPolicy string

const (
	Always  PrimaryReselectPolicy = "always"
	Better  PrimaryReselectPolicy = "better"
	Failure PrimaryReselectPolicy = "failure"
)

// Specifies the transmit hash policy for the selection of slaves. This is only useful in
// balance-xor, 802.3ad and balance-tlb modes. Possible values are layer2, layer3+4,
// layer2+3, encap2+3, and encap3+4.
type TransmitHashPolicy string

const (
	Encap23 TransmitHashPolicy = "encap2+3"
	Encap34 TransmitHashPolicy = "encap3+4"
	Layer2  TransmitHashPolicy = "layer2"
	Layer23 TransmitHashPolicy = "layer2+3"
	Layer34 TransmitHashPolicy = "layer3+4"
)

// Use the given networking backend for this definition. Currently supported are networkd
// and NetworkManager. This property can be specified globally in network:, for a device
// type (in e. g. ethernets:) or for a particular device definition. Default is networkd.
//
// (Since 0.99) The renderer property has one additional acceptable value for vlan objects
// (i. e. defined in vlans:): sriov. If a vlan is defined with the sriov renderer for an
// SR-IOV Virtual Function interface, this causes netplan to set up a hardware VLAN filter
// for it. There can be only one defined per VF.
type Renderer string

const (
	NetworkManager Renderer = "NetworkManager"
	Networkd       Renderer = "networkd"
	Sriov          Renderer = "sriov"
)

// The route scope, how wide-ranging it is to the network. Possible values are “global”,
// “link”, or “host”.
type RouteScope string

const (
	Global RouteScope = "global"
	Host   RouteScope = "host"
	Link   RouteScope = "link"
)

// The type of route. Valid options are “unicast” (default), “anycast”, “blackhole”,
// “broadcast”, “local”, “multicast”, “nat”, “prohibit”, “throw”, “unreachable” or
// “xresolve”.
type RouteType string

const (
	Anycast            RouteType = "anycast"
	Blackhole          RouteType = "blackhole"
	Local              RouteType = "local"
	Multicast          RouteType = "multicast"
	Nat                RouteType = "nat"
	Prohibit           RouteType = "prohibit"
	RouteTypeBroadcast RouteType = "broadcast"
	Throw              RouteType = "throw"
	Unicast            RouteType = "unicast"
	Unreachable        RouteType = "unreachable"
	Xresolve           RouteType = "xresolve"
)

type EmbeddedSwitchMode string

const (
	Legacy    EmbeddedSwitchMode = "legacy"
	Switchdev EmbeddedSwitchMode = "switchdev"
)

type ConnectionMode string

const (
	InBand    ConnectionMode = "in-band"
	OutOfBand ConnectionMode = "out-of-band"
)

type FailMode string

const (
	Secure     FailMode = "Secure"
	Standalone FailMode = "Standalone"
)

type LACP string

const (
	LACPActive LACP = "Active"
	LACPOff    LACP = "Off"
	Passive    LACP = "Passive"
)

type OpenFlowProtocol string

const (
	OpenFlow10 OpenFlowProtocol = "OpenFlow10"
	OpenFlow11 OpenFlowProtocol = "OpenFlow11"
	OpenFlow12 OpenFlowProtocol = "OpenFlow12"
	OpenFlow13 OpenFlowProtocol = "OpenFlow13"
	OpenFlow14 OpenFlowProtocol = "OpenFlow14"
	OpenFlow15 OpenFlowProtocol = "OpenFlow15"
	OpenFlow16 OpenFlowProtocol = "OpenFlow16"
)

// Defines the tunnel mode. Valid options are sit, gre, ip6gre, ipip, ipip6, ip6ip6, vti,
// vti6 and wireguard. Additionally, the networkd backend also supports gretap and ip6gretap
// modes. In addition, the NetworkManager backend supports isatap tunnels.
type TunnelMode string

const (
	Gre       TunnelMode = "gre"
	Gretap    TunnelMode = "gretap"
	ISATAP    TunnelMode = "isatap"
	Ip6Gre    TunnelMode = "ip6gre"
	Ip6Gretap TunnelMode = "ip6gretap"
	Ip6Ip6    TunnelMode = "ip6ip6"
	Ipip      TunnelMode = "ipip"
	Ipip6     TunnelMode = "ipip6"
	Sit       TunnelMode = "sit"
	Vti       TunnelMode = "vti"
	Vti6      TunnelMode = "vti6"
	Wireguard TunnelMode = "wireguard"
)

// 802.1x
type KeyManagmentMode string

const (
	EAP                  KeyManagmentMode = "eap"
	KeyManagmentModeNone KeyManagmentMode = "none"
	Psk                  KeyManagmentMode = "psk"
	Sae                  KeyManagmentMode = "sae"
	The8021X             KeyManagmentMode = "802.1x"
)

type AuthMethod string

const (
	Peap AuthMethod = "peap"
	TLS  AuthMethod = "tls"
	Ttls AuthMethod = "ttls"
)

// 2.4Ghz
//
// 5Ghz
type WirelessBand string

const (
	The24GHz WirelessBand = "2.4GHz"
	The5GHz  WirelessBand = "5GHz"
)

// Possible access point modes are infrastructure (the default), ap (create an access point
// to which other devices can connect), and adhoc (peer to peer networks without a central
// access point). ap is only supported with NetworkManager.
type AccessPointMode string

const (
	Adhoc          AccessPointMode = "adhoc"
	Ap             AccessPointMode = "ap"
	Infrastructure AccessPointMode = "infrastructure"
)

// This enables WakeOnWLan on supported devices. Not all drivers support all options. May be
// any combination of any, disconnect, magic_pkt, gtk_rekey_failure, eap_identity_req,
// four_way_handshake, rfkill_release or tcp (NetworkManager only). Or the exclusive default
// flag (the default).
type WakeOnWLAN string

const (
	Default          WakeOnWLAN = "default"
	Disconnect       WakeOnWLAN = "disconnect"
	EAPIdentityReq   WakeOnWLAN = "eap_identity_req"
	FourWayHandshake WakeOnWLAN = "four_way_handshake"
	GtkRekeyFailure  WakeOnWLAN = "gtk_rekey_failure"
	MagicPkt         WakeOnWLAN = "magic_pkt"
	RfkillRelease    WakeOnWLAN = "rfkill_release"
	TCP              WakeOnWLAN = "tcp"
	WakeOnWLANAny    WakeOnWLAN = "any"
)

type AddressMapping struct {
	AddressMappingClass *AddressMappingClass
	String              *string
}

func (x *AddressMapping) UnmarshalJSON(data []byte) error {
	x.AddressMappingClass = nil
	var c AddressMappingClass
	object, err := unmarshalUnion(data, nil, nil, nil, &x.String, false, nil, true, &c, false, nil, false, nil, false)
	if err != nil {
		return err
	}
	if object {
		x.AddressMappingClass = &c
	}
	return nil
}

func (x *AddressMapping) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, nil, nil, x.String, false, nil, x.AddressMappingClass != nil, x.AddressMappingClass, false, nil, false, nil, false)
}

func unmarshalUnion(data []byte, pi **int64, pf **float64, pb **bool, ps **string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) (bool, error) {
	if pi != nil {
		*pi = nil
	}
	if pf != nil {
		*pf = nil
	}
	if pb != nil {
		*pb = nil
	}
	if ps != nil {
		*ps = nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return false, err
	}

	switch v := tok.(type) {
	case json.Number:
		if pi != nil {
			i, err := v.Int64()
			if err == nil {
				*pi = &i
				return false, nil
			}
		}
		if pf != nil {
			f, err := v.Float64()
			if err == nil {
				*pf = &f
				return false, nil
			}
			return false, errors.New("Unparsable number")
		}
		return false, errors.New("Union does not contain number")
	case float64:
		return false, errors.New("Decoder should not return float64")
	case bool:
		if pb != nil {
			*pb = &v
			return false, nil
		}
		return false, errors.New("Union does not contain bool")
	case string:
		if haveEnum {
			return false, json.Unmarshal(data, pe)
		}
		if ps != nil {
			*ps = &v
			return false, nil
		}
		return false, errors.New("Union does not contain string")
	case nil:
		if nullable {
			return false, nil
		}
		return false, errors.New("Union does not contain null")
	case json.Delim:
		if v == '{' {
			if haveObject {
				return true, json.Unmarshal(data, pc)
			}
			if haveMap {
				return false, json.Unmarshal(data, pm)
			}
			return false, errors.New("Union does not contain object")
		}
		if v == '[' {
			if haveArray {
				return false, json.Unmarshal(data, pa)
			}
			return false, errors.New("Union does not contain array")
		}
		return false, errors.New("Cannot handle delimiter")
	}
	return false, errors.New("Cannot unmarshal union")

}

func marshalUnion(pi *int64, pf *float64, pb *bool, ps *string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) ([]byte, error) {
	if pi != nil {
		return json.Marshal(*pi)
	}
	if pf != nil {
		return json.Marshal(*pf)
	}
	if pb != nil {
		return json.Marshal(*pb)
	}
	if ps != nil {
		return json.Marshal(*ps)
	}
	if haveArray {
		return json.Marshal(pa)
	}
	if haveObject {
		return json.Marshal(pc)
	}
	if haveMap {
		return json.Marshal(pm)
	}
	if haveEnum {
		return json.Marshal(pe)
	}
	if nullable {
		return json.Marshal(nil)
	}
	return nil, errors.New("Union must not be null")
}
