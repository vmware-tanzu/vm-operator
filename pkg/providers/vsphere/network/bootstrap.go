// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgnil "github.com/vmware-tanzu/vm-operator/pkg/util/nil"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// Bootstrap contains the customization data for a single network interface,
// derived from the network provider CR status and overlaid with the user's
// VirtualMachineNetworkInterfaceSpec.
type Bootstrap struct {
	// Name is the interface name from the InterfaceSpec.
	Name string
	// GuestDeviceName is the device name to set inside the guest. Defaults to Name.
	GuestDeviceName string

	// MacAddress is the MAC address to assign to the interface, lowercased.
	// The MAC address is usually provided by the network interface CR, but
	// can be specified by the user in the interface spec. When the MAC address
	// is not specified by either, VC will generate it. That means the address
	// is not known until after the device is created and needs to be backfilled
	// into the Device.
	MacAddress string

	// NoIPAM indicates that neither DHCP nor static-pool assignment is active;
	// the guest must configure addressing out-of-band (e.g. cloud-init NoCloud).
	NoIPAM bool

	// DHCP4 indicates that IPv4 DHCP is active for this interface, either
	// because the provider reported it or the user explicitly requested it via
	// interfaceSpec.DHCP4.  Once true it cannot be cleared by the spec.
	DHCP4 bool

	// DHCP6 indicates that IPv6 DHCP is active for this interface.  Same
	// semantics as DHCP4.
	DHCP6 bool

	// MTU is the MTU to configure inside the guest, taken from interfaceSpec.MTU.
	// Zero means "use the OS default".
	MTU int64

	// Nameservers is the ordered list of DNS resolver addresses for this
	// interface.  Populated from interfaceSpec.Nameservers, or falls back to
	// the VM-level nameservers when CloudInit UseGlobalNameserversAsDefault is
	// true (the default).
	Nameservers []string

	// SearchDomains is the ordered list of DNS search domains for this
	// interface.  Same fallback semantics as Nameservers.
	SearchDomains []string

	// Routes is the list of static routes to configure inside the guest,
	// copied verbatim from interfaceSpec.Routes.
	Routes []NetworkInterfaceRoute

	// IPConfigs is the list of static IP configurations for this interface.
	// Populated from the provider CR status when using StaticPool assignment,
	// or replaced entirely by interfaceSpec.Addresses when the user supplies
	// explicit addresses.
	IPConfigs []NetworkInterfaceIPConfig
}

// InterfaceBootstrap computes the final Bootstrap for a network interface by
// applying the interface spec overrides on top of the provider-derived initial
// state. The initial Bootstrap carries values populated from the network provider
// CR status (DHCP4, DHCP6, NoIPAM, IPConfigs, MacAddress).
//
// Use NetOPInterfaceBootstrap, NCPInterfaceBootstrap, or VPCInterfaceBootstrap
// when starting from a concrete network interface CR.
//
//nolint:gocyclo
func InterfaceBootstrap(
	_ context.Context,
	vm *vmopv1.VirtualMachine,
	initial Bootstrap,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
) Bootstrap {
	bootstrap := initial

	bootstrap.Name = interfaceSpec.Name
	bootstrap.GuestDeviceName = interfaceSpec.GuestDeviceName
	if bootstrap.GuestDeviceName == "" {
		bootstrap.GuestDeviceName = bootstrap.Name
	}

	bootstrap.MacAddress = strings.ToLower(bootstrap.MacAddress)

	if interfaceSpec.MTU != nil {
		bootstrap.MTU = *interfaceSpec.MTU
	}

	// TODO: There is currently no way to unset DHCP flags if the network interface
	// CR has set them. The interface spec DHCP4/6 fields probably should have been
	// pointers, but changing that now is kind of a conversion pain.
	// However, a reasonable behavior to expect from the network provider is to not
	// enable DHCP got an IP address for that family was specified, but we could also
	// enforce that here, and/or add NoIPAM to our interface spec.
	if interfaceSpec.DHCP4 {
		bootstrap.DHCP4 = true
	}
	if interfaceSpec.DHCP6 {
		bootstrap.DHCP6 = true
	}

	if len(interfaceSpec.Addresses) > 0 {
		if interfaceSpec.Gateway4 == "" || interfaceSpec.Gateway6 == "" {
			// Backfill the gateways from the network provider if not specified in
			// the interface spec before setting the user specified addresses. This
			// allows for just IP reservation to be done.
			// Our bootstrap is modeled after the network interface CRs, but that ends
			// up being cumbersome here. Instead, the gateways should be pulled up
			// in the top level results instead of being a part of the IP config.
			// Modify the passed by value interface spec so the code block after this
			// just works.
			for i := range bootstrap.IPConfigs {
				gw := bootstrap.IPConfigs[i].Gateway
				if gw == "" {
					continue
				}

				if bootstrap.IPConfigs[i].IsIPv4 && interfaceSpec.Gateway4 == "" {
					interfaceSpec.Gateway4 = gw
				} else if !bootstrap.IPConfigs[i].IsIPv4 && interfaceSpec.Gateway6 == "" {
					interfaceSpec.Gateway6 = gw
				}
				if interfaceSpec.Gateway4 != "" && interfaceSpec.Gateway6 != "" {
					break
				}
			}
		}

		bootstrap.IPConfigs = make([]NetworkInterfaceIPConfig, 0, len(interfaceSpec.Addresses))
		for _, addr := range interfaceSpec.Addresses {
			ip, _, err := net.ParseCIDR(addr)
			if err != nil {
				continue
			}

			ipConfig := NetworkInterfaceIPConfig{
				IPCIDR: addr,
				IsIPv4: ip.To4() != nil,
			}

			bootstrap.IPConfigs = append(bootstrap.IPConfigs, ipConfig)
		}
	}

	if gw4, gw6 := interfaceSpec.Gateway4, interfaceSpec.Gateway6; gw4 != "" || gw6 != "" {
		// Set the gateway to their user-specified values. For multiple IPs, doing this for
		// every address may not end up making sense but otherwise hard to determine what
		// else to do but for addresses from our interface spec we do have the CIDR. It does
		// really end up mattering since for the network customization we'll use the first
		// gateway. Like mentioned above, how this is modeled vs our needs is a little funky
		// and should pull out the gateways out of returned IP configuration.
		for i := range bootstrap.IPConfigs {
			if gw4 != "" && bootstrap.IPConfigs[i].IsIPv4 {
				if gw4 == gatewayIgnored {
					// Clear network provider gateway.
					bootstrap.IPConfigs[i].Gateway = ""
				} else {
					bootstrap.IPConfigs[i].Gateway = gw4
				}
			} else if gw6 != "" && !bootstrap.IPConfigs[i].IsIPv4 {
				if gw6 == gatewayIgnored {
					// Clear network provider gateway.
					bootstrap.IPConfigs[i].Gateway = ""
				} else {
					bootstrap.IPConfigs[i].Gateway = gw6
				}
			}
		}
	}

	for _, route := range interfaceSpec.Routes {
		bootstrap.Routes = append(bootstrap.Routes, NetworkInterfaceRoute{
			To:     route.To,
			Via:    route.Via,
			Metric: route.Metric,
		})
	}

	var defaultToGlobalNameservers, defaultToGlobalSearchDomains bool
	if bsSpec := vm.Spec.Bootstrap; bsSpec != nil && bsSpec.CloudInit != nil {
		ci := bsSpec.CloudInit
		defaultToGlobalNameservers = ptr.DerefWithDefault(ci.UseGlobalNameserversAsDefault, true)
		defaultToGlobalSearchDomains = ptr.DerefWithDefault(ci.UseGlobalSearchDomainsAsDefault, true)
	}

	networkSpec := vm.Spec.Network

	if n := interfaceSpec.Nameservers; len(n) > 0 {
		bootstrap.Nameservers = n
	} else if defaultToGlobalNameservers && networkSpec != nil {
		bootstrap.Nameservers = networkSpec.Nameservers
	}

	if d := interfaceSpec.SearchDomains; len(d) > 0 {
		bootstrap.SearchDomains = d
	} else if defaultToGlobalSearchDomains && networkSpec != nil {
		bootstrap.SearchDomains = networkSpec.SearchDomains
	}

	return bootstrap
}

// NetOPInterfaceBootstrap computes the Bootstrap for a VDS NetOP NetworkInterface CR by
// extracting the initial state from CR status and applying the interfaceSpec overrides.
func NetOPInterfaceBootstrap(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	netIf *netopv1alpha1.NetworkInterface,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	macAddress string,
) Bootstrap {
	initial := bootstrapFromNetOP(netIf)
	initial.MacAddress = macAddress

	return InterfaceBootstrap(ctx, vm, initial, interfaceSpec)
}

func bootstrapFromNetOP(netIf *netopv1alpha1.NetworkInterface) Bootstrap {
	initial := Bootstrap{}

	v4Mode := EffectiveNetOPIPv4AssignmentMode(netIf.Status)
	v6Mode := EffectiveNetOPIPv6AssignmentMode(netIf.Status)

	if v4Mode == netopv1alpha1.NetworkInterfaceIPAssignmentModeNone &&
		v6Mode == netopv1alpha1.NetworkInterfaceIPAssignmentModeNone {
		initial.NoIPAM = true
		return initial
	}

	initial.DHCP4 = v4Mode == netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
	initial.DHCP6 = v6Mode == netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP

	for _, ip := range netIf.Status.IPConfigs {
		switch ip.IPFamily {
		case corev1.IPv4Protocol:
			if v4Mode != netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool {
				continue
			}
		case corev1.IPv6Protocol:
			if v6Mode != netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool {
				continue
			}
		default:
			continue
		}

		ipConfig := NetworkInterfaceIPConfig{
			IPCIDR:  ipCIDRFromNetOPIPConfig(ip),
			IsIPv4:  ip.IPFamily == corev1.IPv4Protocol,
			Gateway: ip.Gateway,
		}
		initial.IPConfigs = append(initial.IPConfigs, ipConfig)
	}

	return initial
}

// NCPInterfaceBootstrap computes the Bootstrap for an NSX-T NCP VirtualNetworkInterface CR
// by extracting the initial state from CR status and applying the interfaceSpec overrides.
func NCPInterfaceBootstrap(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	vnetIf *ncpv1alpha1.VirtualNetworkInterface,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	macAddress string,
) Bootstrap {
	initial := bootstrapFromNCP(vnetIf)
	initial.MacAddress = macAddress

	return InterfaceBootstrap(ctx, vm, initial, interfaceSpec)
}

func bootstrapFromNCP(vnetIf *ncpv1alpha1.VirtualNetworkInterface) Bootstrap {
	initial := Bootstrap{}

	if ipAddress := vnetIf.Status.IPAddresses; len(ipAddress) == 0 ||
		(len(ipAddress) == 1 && ipAddress[0].IP == "") {
		initial.DHCP4 = true
	} else {
		for _, ipAddr := range ipAddress {
			if ipAddr.IP == "" {
				continue
			}
			isIPv4 := net.ParseIP(ipAddr.IP).To4() != nil
			ipConfig := NetworkInterfaceIPConfig{
				IPCIDR:  ipCIDRNotation(ipAddr.IP, ipAddr.SubnetMask, isIPv4),
				IsIPv4:  isIPv4,
				Gateway: ipAddr.Gateway,
			}
			initial.IPConfigs = append(initial.IPConfigs, ipConfig)
		}
	}

	return initial
}

// VPCInterfaceBootstrap computes the Bootstrap for a VPC SubnetPort CR by extracting
// the initial state from CR status and applying the interfaceSpec overrides.
func VPCInterfaceBootstrap(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	subnetPort *vpcv1alpha1.SubnetPort,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	macAddress string,
) Bootstrap {
	initial := bootstrapFromVPC(ctx, subnetPort)
	initial.MacAddress = macAddress

	return InterfaceBootstrap(ctx, vm, initial, interfaceSpec)
}

func bootstrapFromVPC(
	ctx context.Context,
	subnetPort *vpcv1alpha1.SubnetPort) Bootstrap {

	initial := Bootstrap{}

	for _, ipAddr := range subnetPort.Status.NetworkInterfaceConfig.IPAddresses {
		if ipAddr.IPAddress == "" {
			// For DHCP and NoIPAM, IPAddress will be unset but Gateway will be set.
			continue
		}
		ip, _, err := net.ParseCIDR(ipAddr.IPAddress)
		if err != nil {
			pkglog.FromContextOrDefault(ctx).Error(err, "bad CIDR from SubnetPort",
				"subnetPort", subnetPort.Name)
			continue
		}
		ipConfig := NetworkInterfaceIPConfig{
			IPCIDR:  ipAddr.IPAddress,
			IsIPv4:  ip.To4() != nil,
			Gateway: ipAddr.Gateway,
		}
		initial.IPConfigs = append(initial.IPConfigs, ipConfig)
	}

	if len(initial.IPConfigs) == 0 {
		if !subnetPort.Status.NetworkInterfaceConfig.DHCPDeactivatedOnSubnet {
			initial.DHCP4 = true
		} else {
			initial.NoIPAM = true
		}
	}

	return initial
}

// devAndBootstrapToNetworkInterfaceResult combines a fully-computed Device and
// Bootstrap to produce the old style NetworkInterfaceResult. This is used by
// the CreateAndWait workflow to keep that API unchanged.
func devAndBootstrapToNetworkInterfaceResult(
	d Device,
	b Bootstrap,
) NetworkInterfaceResult {
	var objName string
	if !pkgnil.IsNil(d.InterfaceObj) {
		// Named network won't have an object.
		objName = d.InterfaceObj.GetName()
	}

	return NetworkInterfaceResult{
		ObjectName:      objName,
		ExternalID:      d.ExternalID,
		NetworkID:       d.NetworkID,
		Backing:         d.Backing,
		Name:            b.Name,
		GuestDeviceName: b.GuestDeviceName,
		MacAddress:      b.MacAddress,
		NoIPAM:          b.NoIPAM,
		DHCP4:           b.DHCP4,
		DHCP6:           b.DHCP6,
		MTU:             b.MTU,
		Nameservers:     b.Nameservers,
		SearchDomains:   b.SearchDomains,
		Routes:          b.Routes,
		IPConfigs:       b.IPConfigs,
	}
}
