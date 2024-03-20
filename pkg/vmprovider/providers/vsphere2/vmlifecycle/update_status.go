// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	goctx "context"
	"fmt"
	"net"
	"reflect"
	"slices"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
)

var (
	// VMStatusPropertiesSelector is the minimum properties needed to be retrieved in order to populate
	// the Status. Callers may provide a MO with more. This often saves us a second round trip in the
	// common steady state.
	VMStatusPropertiesSelector = []string{
		"config.changeTrackingEnabled",
		"config.extraConfig",
		"guest",
		"summary",
	}
)

func UpdateStatus(
	vmCtx context.VirtualMachineContextA2,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine,
	moVM *mo.VirtualMachine) error {

	vm := vmCtx.VM

	// This is implicitly true: ensure the condition is set since it is how we determine the old v1a1 Phase.
	conditions.MarkTrue(vmCtx.VM, vmopv1.VirtualMachineConditionCreated)
	// TODO: Might set other "prereq" conditions too for version conversion but we'd have to fib a little.

	if vm.Status.Image == nil {
		// If unset, we don't know if this was a cluster or namespace scoped image at create time.
		vm.Status.Image = &common.LocalObjectRef{
			Name:       vm.Spec.ImageName,
			APIVersion: vmopv1.SchemeGroupVersion.String(),
		}
	}
	if vm.Status.Class == nil {
		// In v1a2 we know this will always be the namespace scoped class since v1a2 doesn't have
		// the bindings. Our handling of this field will be more complicated once we really
		// support class changes and resizing/reconfiguring the VM the fly in response.
		vm.Status.Class = &common.LocalObjectRef{
			Kind:       "VirtualMachineClass",
			APIVersion: vmopv1.SchemeGroupVersion.String(),
			Name:       vm.Spec.ClassName,
		}
	}

	if moVM == nil {
		// In the common case, our caller will have already gotten the MO properties in order to determine
		// if it had any reconciliation to do, and there was nothing to do since the VM is in the steady
		// state so that MO is still entirely valid here.
		// NOTE: The properties must have been retrieved with at least vmStatusPropertiesSelector.
		moVM = &mo.VirtualMachine{}
		if err := vcVM.Properties(vmCtx, vcVM.Reference(), VMStatusPropertiesSelector, moVM); err != nil {
			// Leave the current Status unchanged for now.
			return fmt.Errorf("failed to get VM properties for status update: %w", err)
		}
	}

	var errs []error
	var err error
	summary := moVM.Summary

	vm.Status.PowerState = convertPowerState(summary.Runtime.PowerState)
	vm.Status.UniqueID = vcVM.Reference().Value
	vm.Status.BiosUUID = summary.Config.Uuid
	vm.Status.InstanceUUID = summary.Config.InstanceUuid
	hardwareVersion, _ := types.ParseHardwareVersion(summary.Config.HwVersion)
	vm.Status.HardwareVersion = int32(hardwareVersion)
	updateGuestNetworkStatus(vmCtx.VM, moVM.Guest)

	vm.Status.Host, err = getRuntimeHostHostname(vmCtx, vcVM, summary.Runtime.Host)
	if err != nil {
		errs = append(errs, err)
	}

	MarkVMToolsRunningStatusCondition(vmCtx.VM, moVM.Guest)
	MarkCustomizationInfoCondition(vmCtx.VM, moVM.Guest)
	MarkBootstrapCondition(vmCtx.VM, moVM.Config)

	if config := moVM.Config; config != nil {
		vm.Status.ChangeBlockTracking = config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}

	zoneName := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
	if zoneName == "" {
		cluster, err := virtualmachine.GetVMClusterComputeResource(vmCtx, vcVM)
		if err != nil {
			errs = append(errs, err)
		} else {
			zoneName, err = topology.LookupZoneForClusterMoID(vmCtx, k8sClient, cluster.Reference().Value)
			if err != nil {
				errs = append(errs, err)
			} else {
				if vm.Labels == nil {
					vm.Labels = map[string]string{}
				}
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
			}
		}
	}

	if zoneName != "" {
		vm.Status.Zone = zoneName
	}

	return k8serrors.NewAggregate(errs)
}

func getRuntimeHostHostname(
	ctx goctx.Context,
	vcVM *object.VirtualMachine,
	host *types.ManagedObjectReference) (string, error) {

	if host != nil {
		return object.NewHostSystem(vcVM.Client(), *host).ObjectName(ctx)
	}
	return "", nil
}

func guestNicInfoToInterfaceStatus(
	name string,
	deviceKey int32,
	guestNicInfo *types.GuestNicInfo) vmopv1.VirtualMachineNetworkInterfaceStatus {

	status := vmopv1.VirtualMachineNetworkInterfaceStatus{
		Name:      name,
		DeviceKey: deviceKey,
	}

	if guestNicInfo.MacAddress != "" {
		status.IP = &vmopv1.VirtualMachineNetworkInterfaceIPStatus{
			MACAddr: guestNicInfo.MacAddress,
		}
	}

	if guestIPConfig := guestNicInfo.IpConfig; guestIPConfig != nil {
		if status.IP == nil {
			status.IP = &vmopv1.VirtualMachineNetworkInterfaceIPStatus{}
		}

		status.IP.AutoConfigurationEnabled = guestIPConfig.AutoConfigurationEnabled
		status.IP.Addresses = convertNetIPConfigInfoIPAddresses(guestIPConfig.IpAddress)

		if guestIPConfig.Dhcp != nil {
			status.IP.DHCP = convertNetDhcpConfigInfo(guestIPConfig.Dhcp)
		}
	}

	if dnsConfig := guestNicInfo.DnsConfig; dnsConfig != nil {
		status.DNS = convertNetDNSConfigInfo(dnsConfig)
	}

	return status
}

func guestIPStackInfoToIPStackStatus(guestIPStack *types.GuestStackInfo) vmopv1.VirtualMachineNetworkIPStackStatus {
	status := vmopv1.VirtualMachineNetworkIPStackStatus{}

	if dhcpConfig := guestIPStack.DhcpConfig; dhcpConfig != nil {
		status.DHCP = convertNetDhcpConfigInfo(dhcpConfig)
	}

	if dnsConfig := guestIPStack.DnsConfig; dnsConfig != nil {
		status.DNS = convertNetDNSConfigInfo(dnsConfig)
	}

	if ipRouteConfig := guestIPStack.IpRouteConfig; ipRouteConfig != nil {
		status.IPRoutes = convertNetIPRouteConfigInfo(ipRouteConfig)
	}

	status.KernelConfig = convertKeyValueSlice(guestIPStack.IpStackConfig)

	return status
}

func convertPowerState(powerState types.VirtualMachinePowerState) vmopv1.VirtualMachinePowerState {
	switch powerState {
	case types.VirtualMachinePowerStatePoweredOff:
		return vmopv1.VirtualMachinePowerStateOff
	case types.VirtualMachinePowerStatePoweredOn:
		return vmopv1.VirtualMachinePowerStateOn
	case types.VirtualMachinePowerStateSuspended:
		return vmopv1.VirtualMachinePowerStateSuspended
	}
	return ""
}

func convertNetIPConfigInfoIPAddresses(ipAddresses []types.NetIpConfigInfoIpAddress) []vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus {
	if len(ipAddresses) == 0 {
		return nil
	}

	out := make([]vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus, 0, len(ipAddresses))
	for _, guestIPAddr := range ipAddresses {
		ipAddrStatus := vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus{
			Address: guestIPAddr.IpAddress,
			Origin:  guestIPAddr.Origin,
			State:   guestIPAddr.State,
		}
		if guestIPAddr.Lifetime != nil {
			ipAddrStatus.Lifetime = metav1.NewTime(*guestIPAddr.Lifetime)
		}

		out = append(out, ipAddrStatus)
	}
	return out
}

func convertNetDNSConfigInfo(dnsConfig *types.NetDnsConfigInfo) *vmopv1.VirtualMachineNetworkDNSStatus {
	return &vmopv1.VirtualMachineNetworkDNSStatus{
		DHCP:          dnsConfig.Dhcp,
		DomainName:    dnsConfig.DomainName,
		HostName:      dnsConfig.HostName,
		Nameservers:   dnsConfig.IpAddress,
		SearchDomains: dnsConfig.SearchDomain,
	}
}

func convertNetDhcpConfigInfo(dhcpConfig *types.NetDhcpConfigInfo) *vmopv1.VirtualMachineNetworkDHCPStatus {
	if ipv4, ipv6 := dhcpConfig.Ipv4, dhcpConfig.Ipv6; ipv4 != nil || ipv6 != nil {
		status := &vmopv1.VirtualMachineNetworkDHCPStatus{}

		if ipv4 != nil {
			status.IP4.Enabled = ipv4.Enable
			status.IP4.Config = convertKeyValueSlice(ipv4.Config)
		}
		if ipv6 != nil {
			status.IP6.Enabled = ipv6.Enable
			status.IP6.Config = convertKeyValueSlice(ipv6.Config)
		}

		return status
	}

	return nil
}

func convertNetIPRouteConfigInfo(routeConfig *types.NetIpRouteConfigInfo) []vmopv1.VirtualMachineNetworkIPRouteStatus {
	if len(routeConfig.IpRoute) == 0 {
		return nil
	}

	// Try to skip routes that are likely not interesting or useful to external users - especially on
	// TKG nodes - that would otherwise just clutter the Status output.
	skipRoute := func(ipRoute types.NetIpRouteConfigInfoIpRoute) bool {
		network, prefix := ipRoute.Network, ipRoute.PrefixLength

		ip := net.ParseIP(network)
		if ip == nil {
			return true
		}

		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}

		if ip.To4() != nil {
			return prefix == 32
		}

		return ip.To16() == nil || ip.IsInterfaceLocalMulticast() || ip.IsMulticast()
	}

	out := make([]vmopv1.VirtualMachineNetworkIPRouteStatus, 0, 1)
	for _, ipRoute := range routeConfig.IpRoute {
		if skipRoute(ipRoute) {
			continue
		}

		out = append(out, vmopv1.VirtualMachineNetworkIPRouteStatus{
			Gateway: vmopv1.VirtualMachineNetworkIPRouteGatewayStatus{
				Device:  ipRoute.Gateway.Device,
				Address: ipRoute.Gateway.IpAddress,
			},
			NetworkAddress: fmt.Sprintf("%s/%d", ipRoute.Network, ipRoute.PrefixLength),
		})
	}
	return out
}

func convertKeyValueSlice(s []types.KeyValue) []common.KeyValuePair {
	if len(s) == 0 {
		return nil
	}

	out := make([]common.KeyValuePair, 0, len(s))
	for i := range s {
		out = append(out, common.KeyValuePair{Key: s[i].Key, Value: s[i].Value})
	}
	return out
}

func MarkVMToolsRunningStatusCondition(
	vm *vmopv1.VirtualMachine,
	guestInfo *types.GuestInfo) {

	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(types.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		msg := "VMware Tools is not running"
		conditions.MarkFalse(vm, vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason, msg)
	case string(types.VirtualMachineToolsRunningStatusGuestToolsRunning), string(types.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, vmopv1.VirtualMachineToolsCondition)
	default:
		msg := "Unexpected VMware Tools running status"
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "Unknown", msg)
	}
}

func MarkCustomizationInfoCondition(vm *vmopv1.VirtualMachine, guestInfo *types.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, vmopv1.GuestCustomizationCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, "")
	case string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, "")
	case string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(types.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, "Unknown", errorMsg)
	}
}

func MarkBootstrapCondition(
	vm *vmopv1.VirtualMachine,
	configInfo *types.VirtualMachineConfigInfo) {

	if configInfo == nil {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoConfigInfo", "")
		return
	}

	if len(configInfo.ExtraConfig) == 0 {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoExtraConfig", "")
		return
	}

	status, reason, msg, ok := util.GetBootstrapConditionValues(configInfo)
	if !ok {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoBootstrapStatus", "")
		return
	}
	if status {
		c := conditions.TrueCondition(vmopv1.GuestBootstrapCondition)
		if reason != "" {
			c.Reason = reason
		}
		c.Message = msg
		conditions.Set(vm, c)
	} else {
		conditions.MarkFalse(vm, vmopv1.GuestBootstrapCondition, reason, msg)
	}
}

var (
	emptyNetConfig   vmopv1.VirtualMachineNetworkConfigStatus
	emptyIfaceConfig vmopv1.VirtualMachineNetworkConfigInterfaceStatus
)

// UpdateNetworkStatusConfig updates the provided VM's status.network.config
// field with information from the provided bootstrap arguments. This is useful
// for folks booting VMs without bootstrap engines who may wish to manually
// configure the VM's networking with the valid IP configuration for this VM.
//
//nolint:gocyclo
func UpdateNetworkStatusConfig(vm *vmopv1.VirtualMachine, args BootstrapArgs) {

	if vm == nil {
		panic("vm is nil")
	}

	// Define the network configuration to update. However, it is not assigned
	// to the VM's status.network.config field unless nc is non-empty before
	// this function ends.
	var nc vmopv1.VirtualMachineNetworkConfigStatus

	// Update the global DNS information.
	{
		hn, ns, sd := args.Hostname, args.DNSServers, args.SearchSuffixes
		lhn, lns, lsd := len(hn) > 0, len(ns) > 0, len(sd) > 0
		if lhn || lns || lsd {
			nc.DNS = &vmopv1.VirtualMachineNetworkConfigDNSStatus{}
		}
		if lhn {
			nc.DNS.HostName = hn
		}
		if lns {
			nc.DNS.Nameservers = ns
		}
		if lsd {
			nc.DNS.SearchDomains = sd
		}
	}

	// Iterate over each network result.
	for i := range args.NetworkResults.Results {

		// Declare the interface's config status.
		var ifc vmopv1.VirtualMachineNetworkConfigInterfaceStatus

		// Define a short alias for the indexed result.
		r := args.NetworkResults.Results[i]

		// Grab a temp copy of the result's IP configuration list to make it
		// more obvious that the purpose of the next three lines of code is not
		// to update the r.IPConfigs directly.
		ipConfigs := args.NetworkResults.Results[i].IPConfigs

		// The intended DHCP configuration is presented per interface, so if
		// there are no resulting IP configs, but DHCP4 or DHCP6 is configured,
		// go ahead and create a single, fake IP config so the DHCP info can be
		// collected the same way below.
		if len(ipConfigs) == 0 && (r.DHCP4 || r.DHCP6) {
			ipConfigs = []network.NetworkInterfaceIPConfig{{}}
		}

		// If there *are* resulting IP configs, then ensure the field ifc.IP
		// is not nil so avoid an NPE later. We do not initialize this field
		// unless there *are* resulting IP configs to avoid an empty object
		// when printing the VM's status.
		if len(ipConfigs) > 0 {
			ifc.IP = &vmopv1.VirtualMachineNetworkConfigInterfaceIPStatus{}
		}

		// Iterate over each of the result's IP configurations.
		for j := range ipConfigs {
			ipc := ipConfigs[j]

			// Assign the gateways.
			if gw := ipc.Gateway; gw != "" {
				if ipc.IsIPv4 && ifc.IP.Gateway4 == "" {
					ifc.IP.Gateway4 = gw
				} else if !ipc.IsIPv4 && ifc.IP.Gateway6 == "" {
					ifc.IP.Gateway6 = gw
				}
			}

			// Append the IP address.
			if ip := ipc.IPCIDR; ip != "" {
				ifc.IP.Addresses = append(ifc.IP.Addresses, ip)
			}

			// Update DHCP information.
			if v4, v6 := r.DHCP4, r.DHCP6; v4 || v6 {
				ifc.IP.DHCP = &vmopv1.VirtualMachineNetworkConfigDHCPStatus{}
				if v4 {
					ifc.IP.DHCP.IP4 = &vmopv1.VirtualMachineNetworkConfigDHCPOptionsStatus{
						Enabled: v4,
					}
				}
				if v6 {
					ifc.IP.DHCP.IP6 = &vmopv1.VirtualMachineNetworkConfigDHCPOptionsStatus{
						Enabled: v6,
					}
				}
			}

			// Update DNS information.
			{
				ns, sd := r.Nameservers, r.SearchDomains
				if ln, ls := len(ns), len(sd); ln > 0 || ls > 0 {
					ifc.DNS = &vmopv1.VirtualMachineNetworkConfigDNSStatus{}
					if ln > 0 {
						ifc.DNS.Nameservers = ns
					}
					if ls > 0 {
						ifc.DNS.SearchDomains = sd
					}
				}
			}
		}

		if ip := ifc.IP; ip != nil && len(ip.Addresses) > 0 {
			slices.Sort(ifc.IP.Addresses)
		}

		// Only append the interface config if it is not empty.
		if !reflect.DeepEqual(ifc, emptyIfaceConfig) {
			// Do not assign the name until the very end so as to not disrupt
			// the comparison with the empty struct.
			ifc.Name = r.Name

			nc.Interfaces = append(nc.Interfaces, ifc)
		}
	}

	// If the network config ended up empty, then ensure the VM's field
	// status.network.config is nil IFF status.network is non-nil.
	// Otherwise, assign the network config to the VM's status.network.config
	// field.
	if reflect.DeepEqual(nc, emptyNetConfig) {
		if vm.Status.Network != nil {
			vm.Status.Network.Config = nil
		}
	} else {
		if vm.Status.Network == nil {
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vm.Status.Network.Config = &nc
	}
}

// updateGuestNetworkStatus updates the provided VM's status.network
// field with information from the guestInfo.
func updateGuestNetworkStatus(vm *vmopv1.VirtualMachine, gi *types.GuestInfo) {
	var (
		primaryIP4      string
		primaryIP6      string
		ifaceStatuses   []vmopv1.VirtualMachineNetworkInterfaceStatus
		ipStackStatuses []vmopv1.VirtualMachineNetworkIPStackStatus
	)

	if gi != nil {
		if ip := gi.IpAddress; ip != "" {
			// Only act on the IP if it is valid.
			if a := net.ParseIP(ip); len(a) > 0 {

				// Ignore local IP addresses, i.e. addresses that are only valid on
				// the guest OS. Please note this does not include private, or RFC
				// 1918 (IPv4) and RFC 4193 (IPv6) addresses, ex. 192.168.0.2.
				if !a.IsUnspecified() &&
					!a.IsLinkLocalMulticast() &&
					!a.IsLinkLocalUnicast() &&
					!a.IsLoopback() {

					if a.To4() != nil {
						primaryIP4 = ip
					} else {
						primaryIP6 = ip
					}
				}
			}
		}

		if len(gi.Net) > 0 {
			var ifaceSpecs []vmopv1.VirtualMachineNetworkInterfaceSpec
			if vm.Spec.Network != nil {
				ifaceSpecs = vm.Spec.Network.Interfaces
			}

			slices.SortFunc(gi.Net, func(a, b types.GuestNicInfo) int {
				// Sort by the DeviceKey (DeviceConfigId) to order the guest info
				// list by the order in the initial ConfigSpec which is the order of
				// the []ifaceSpecs since it is immutable.
				return int(a.DeviceConfigId - b.DeviceConfigId)
			})

			ifaceIdx := 0
			for i := range gi.Net {
				deviceKey := gi.Net[i].DeviceConfigId

				// Skip pseudo devices.
				if deviceKey < 0 {
					continue
				}

				var ifaceName string
				if ifaceIdx < len(ifaceSpecs) {
					ifaceName = ifaceSpecs[ifaceIdx].Name
					ifaceIdx++
				}

				ifaceStatuses = append(
					ifaceStatuses,
					guestNicInfoToInterfaceStatus(
						ifaceName,
						deviceKey,
						&gi.Net[i]))
			}
		}

		if lip := len(gi.IpStack); lip > 0 {
			ipStackStatuses = make([]vmopv1.VirtualMachineNetworkIPStackStatus, lip)
			for i := range gi.IpStack {
				ipStackStatuses[i] = guestIPStackInfoToIPStackStatus(&gi.IpStack[i])
			}
		}
	}

	var (
		lip4 = len(primaryIP4) > 0
		lip6 = len(primaryIP6) > 0
		lip  = len(ipStackStatuses) > 0
		lis  = len(ifaceStatuses) > 0
	)

	if lip4 || lip6 || lip || lis {
		if vm.Status.Network == nil {
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vm.Status.Network.PrimaryIP4 = primaryIP4
		vm.Status.Network.PrimaryIP6 = primaryIP6
		vm.Status.Network.IPStacks = ipStackStatuses
		vm.Status.Network.Interfaces = ifaceStatuses
	} else if vm.Status.Network != nil {
		if cfg := vm.Status.Network.Config; cfg != nil {
			// Config is the only field we need to preserve.
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				Config: cfg,
			}
		} else {
			vm.Status.Network = nil
		}
	}
}
