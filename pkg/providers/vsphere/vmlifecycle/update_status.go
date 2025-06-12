// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"fmt"
	"net"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vmdk"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

type ReconcileStatusData struct {
	// NetworkDeviceKeysToSpecIdx maps the network device's DeviceKey to its
	// corresponding index in the VM's Spec.Network.Interfaces[].
	NetworkDeviceKeysToSpecIdx map[int32]int
}

func ReconcileStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine,
	data ReconcileStatusData) error {

	vm := vmCtx.VM

	// This is implicitly true: ensure the condition is set since it is how we determine the old v1a1 Phase.
	conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)

	if !vmopv1util.IsClasslessVM(*vmCtx.VM) {
		if vm.Status.Class == nil {
			// When resize is enabled, don't backfill the class from the Spec since we don't know if the
			// Spec class has been applied to the VM. When resize is enabled, this field is updated after
			// a successful resize.
			if f := pkgcfg.FromContext(vmCtx).Features; !f.VMResize && !f.VMResizeCPUMemory {
				vm.Status.Class = &common.LocalObjectRef{
					APIVersion: vmopv1.GroupVersion.String(),
					Kind:       "VirtualMachineClass",
					Name:       vm.Spec.ClassName,
				}
			}
		}
	} else {
		vm.Status.Class = nil
	}

	var (
		err         error
		errs        []error
		extraConfig map[string]string
		summary     = vmCtx.MoVM.Summary
	)

	if err := UpdateGroupLinkedCondition(vmCtx, k8sClient); err != nil {
		errs = append(errs, err)
	}

	if config := vmCtx.MoVM.Config; config != nil {
		extraConfig = object.OptionValueList(config.ExtraConfig).StringMap()
	}

	vm.Status.PowerState = convertPowerState(summary.Runtime.PowerState)
	vm.Status.UniqueID = vcVM.Reference().Value
	vm.Status.BiosUUID = summary.Config.Uuid
	vm.Status.InstanceUUID = summary.Config.InstanceUuid
	hardwareVersion, _ := vimtypes.ParseHardwareVersion(summary.Config.HwVersion)
	vm.Status.HardwareVersion = int32(hardwareVersion)
	updateGuestNetworkStatus(vmCtx.VM, vmCtx.MoVM.Guest, extraConfig, data.NetworkDeviceKeysToSpecIdx)
	updateStorageStatus(vmCtx.VM, vmCtx.MoVM)

	if pkgcfg.FromContext(vmCtx).AsyncSignalEnabled {
		updateProbeStatus(vmCtx, vm, vmCtx.MoVM, extraConfig)
	}

	vm.Status.NodeName, err = getRuntimeHostHostname(vmCtx, vcVM, summary.Runtime.Host)
	if err != nil {
		errs = append(errs, err)
	}

	if summary.Guest != nil && summary.Guest.HostName != "" {
		if vm.Status.Network == nil {
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vm.Status.Network.HostName = summary.Guest.HostName
	}

	MarkReconciliationCondition(vmCtx.VM)
	MarkVMToolsRunningStatusCondition(vmCtx.VM, vmCtx.MoVM.Guest)
	MarkCustomizationInfoCondition(vmCtx.VM, vmCtx.MoVM.Guest)
	MarkBootstrapCondition(vmCtx.VM, extraConfig)

	if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
		MarkVMClassConfigurationSynced(vmCtx, vmCtx.VM, k8sClient)
	}

	zoneName := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
	if zoneName == "" {
		clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(vmCtx, vcVM.Client(), vmCtx.MoVM.ResourcePool.Value)
		if err != nil {
			errs = append(errs, err)
		} else {
			zoneName, err = topology.LookupZoneForClusterMoID(vmCtx, k8sClient, clusterMoRef.Value)
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

	return apierrorsutil.NewAggregate(errs)
}

// UpdateGroupLinkedCondition updates the GroupLinked condition for the VM if
// the VM is a member of a group.
func UpdateGroupLinkedCondition(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) error {
	if vmCtx.VM.Spec.GroupName == "" {
		conditions.Delete(
			vmCtx.VM,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked)

		return nil
	}

	var (
		obj vmopv1.VirtualMachineGroup
		key = ctrlclient.ObjectKey{
			Name:      vmCtx.VM.Spec.GroupName,
			Namespace: vmCtx.VM.Namespace,
		}
	)

	if err := k8sClient.Get(vmCtx, key, &obj); err != nil {
		if !apierrors.IsNotFound(err) {
			conditions.MarkError(
				vmCtx.VM,
				vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				"Error",
				err)

			return err
		}

		conditions.MarkFalse(
			vmCtx.VM,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"NotFound",
			"")

		return nil
	}

	for _, bo := range obj.Spec.BootOrder {
		for _, m := range bo.Members {
			if m.Kind == "VirtualMachine" {
				if m.Name == vmCtx.VM.Name {
					conditions.MarkTrue(
						vmCtx.VM,
						vmopv1.VirtualMachineGroupMemberConditionGroupLinked)

					return nil
				}
			}
		}
	}

	conditions.MarkFalse(
		vmCtx.VM,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
		"NotMember",
		"")

	return nil
}

func getRuntimeHostHostname(
	ctx context.Context,
	vcVM *object.VirtualMachine,
	host *vimtypes.ManagedObjectReference) (string, error) {

	if host != nil {
		return object.NewHostSystem(vcVM.Client(), *host).ObjectName(ctx)
	}
	return "", nil
}

func guestNicInfoToInterfaceStatus(
	name string,
	deviceKey int32,
	guestNicInfo *vimtypes.GuestNicInfo) vmopv1.VirtualMachineNetworkInterfaceStatus {

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

func guestIPStackInfoToIPStackStatus(guestIPStack *vimtypes.GuestStackInfo) vmopv1.VirtualMachineNetworkIPStackStatus {
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

func convertPowerState(powerState vimtypes.VirtualMachinePowerState) vmopv1.VirtualMachinePowerState {
	switch powerState {
	case vimtypes.VirtualMachinePowerStatePoweredOff:
		return vmopv1.VirtualMachinePowerStateOff
	case vimtypes.VirtualMachinePowerStatePoweredOn:
		return vmopv1.VirtualMachinePowerStateOn
	case vimtypes.VirtualMachinePowerStateSuspended:
		return vmopv1.VirtualMachinePowerStateSuspended
	}
	return ""
}

func convertNetIPConfigInfoIPAddresses(ipAddresses []vimtypes.NetIpConfigInfoIpAddress) []vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus {
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

func convertNetDNSConfigInfo(dnsConfig *vimtypes.NetDnsConfigInfo) *vmopv1.VirtualMachineNetworkDNSStatus {
	return &vmopv1.VirtualMachineNetworkDNSStatus{
		DHCP:          dnsConfig.Dhcp,
		DomainName:    dnsConfig.DomainName,
		HostName:      dnsConfig.HostName,
		Nameservers:   util.Dedupe(dnsConfig.IpAddress),
		SearchDomains: util.Dedupe(dnsConfig.SearchDomain),
	}
}

func convertNetDhcpConfigInfo(dhcpConfig *vimtypes.NetDhcpConfigInfo) *vmopv1.VirtualMachineNetworkDHCPStatus {
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

func convertNetIPRouteConfigInfo(routeConfig *vimtypes.NetIpRouteConfigInfo) []vmopv1.VirtualMachineNetworkIPRouteStatus {
	if len(routeConfig.IpRoute) == 0 {
		return nil
	}

	// Try to skip routes that are likely not interesting or useful to external users - especially on
	// TKG nodes - that would otherwise just clutter the Status output.
	skipRoute := func(ipRoute vimtypes.NetIpRouteConfigInfoIpRoute) bool {
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

func convertKeyValueSlice(s []vimtypes.KeyValue) []common.KeyValuePair {
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
	guestInfo *vimtypes.GuestInfo) {

	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		conditions.MarkFalse(vm, vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason, "VMware Tools is not running")
	case string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsRunning), string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, vmopv1.VirtualMachineToolsCondition)
	default:
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "Unknown", "Unexpected VMware Tools running status")
	}
}

func MarkCustomizationInfoCondition(vm *vmopv1.VirtualMachine, guestInfo *vimtypes.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, vmopv1.GuestCustomizationCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, "")
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, "")
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, "%s", errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, "Unknown", "%s", errorMsg)
	}
}
func MarkReconciliationCondition(vm *vmopv1.VirtualMachine) {
	switch vm.Labels[vmopv1.PausedVMLabelKey] {
	case "devops":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by DevOps")
	case "admin":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by Admin")
	case "both":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by Admin, DevOps")
	default:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineReconcileReady)
	}
}

func MarkBootstrapCondition(
	vm *vmopv1.VirtualMachine,
	extraConfig map[string]string) {

	if len(extraConfig) == 0 {
		conditions.MarkUnknown(
			vm, vmopv1.GuestBootstrapCondition, "NoExtraConfig", "")
		return
	}

	status, reason, msg, ok := util.GetBootstrapConditionValues(extraConfig)
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
		conditions.MarkFalse(vm, vmopv1.GuestBootstrapCondition, reason, "%s", msg)
	}
}

func MarkVMClassConfigurationSynced(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	k8sClient ctrlclient.Client) {

	className := vm.Spec.ClassName
	if className == "" {
		conditions.Delete(vm, vmopv1.VirtualMachineClassConfigurationSynced)
		return
	}

	// NOTE: This performs the same checks as vmopv1util.ResizeNeeded() but we can't use
	// just the return value of that function because we need more details, and we want
	// to avoid having to fetch the class if we can.

	lraName, lraUID, lraGeneration, exists := vmopv1util.GetLastResizedAnnotation(*vm)
	if !exists || lraName == "" {
		_, sameClassResize := vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation]
		if sameClassResize {
			conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "SameClassResize", "")
		} else {
			// Brownfield VM so just marked as synced.
			conditions.MarkTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)
		}
		return
	}

	if vm.Spec.ClassName != lraName {
		// Most common need resize case.
		conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassNameChanged", "")
		return
	}

	// Depending on what we did in the prior session update code for this VM, we might already
	// have fetched the class but the way the code is structured today, it isn't very easy for
	// us to pass that to here. So just refetch it again.
	//
	// Note that for this situation we'll only do a resize if the SameVMClassResizeAnnotation
	// is present but use this condition to inform if the class has changed and a user could
	// opt-in to a resize.

	vmClass := vmopv1.VirtualMachineClass{}
	if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: className, Namespace: vm.Namespace}, &vmClass); err != nil {
		if apierrors.IsNotFound(err) {
			conditions.MarkUnknown(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassNotFound", "")
		} else {
			conditions.MarkUnknown(vm, vmopv1.VirtualMachineClassConfigurationSynced, err.Error(), "")
		}
		return
	}

	if string(vmClass.UID) == lraUID && vmClass.Generation == lraGeneration {
		conditions.MarkTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)
	} else {
		conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassUpdated", "")
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
		hn, dn, ns, sd := args.HostName, args.DomainName, args.DNSServers, args.SearchSuffixes
		lhn, ldn, lns, lsd := len(hn) > 0, len(dn) > 0, len(ns) > 0, len(sd) > 0
		if lhn || ldn || lns || lsd {
			nc.DNS = &vmopv1.VirtualMachineNetworkConfigDNSStatus{}
			if lhn {
				nc.DNS.HostName = hn
			}
			if ldn {
				nc.DNS.DomainName = dn
			}
			if lns {
				nc.DNS.Nameservers = ns
			}
			if lsd {
				nc.DNS.SearchDomains = sd
			}
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
//
//nolint:gocyclo
func updateGuestNetworkStatus(
	vm *vmopv1.VirtualMachine,
	gi *vimtypes.GuestInfo,
	extraConfig map[string]string,
	deviceKeyToSpecIdx map[int32]int) {

	var (
		primaryIP4      string
		primaryIP6      string
		ifaceStatuses   []vmopv1.VirtualMachineNetworkInterfaceStatus
		ipStackStatuses []vmopv1.VirtualMachineNetworkIPStackStatus
	)

	validatePrimaryIP := func(ip string) net.IP {
		if a := net.ParseIP(ip); len(a) > 0 {
			// Ignore local IP addresses, i.e. addresses that are only valid on
			// the guest OS. Please note this does not include private, or RFC
			// 1918 (IPv4) and RFC 4193 (IPv6) addresses, ex. 192.168.0.2.
			if !a.IsUnspecified() &&
				!a.IsLinkLocalMulticast() &&
				!a.IsLinkLocalUnicast() &&
				!a.IsLoopback() {
				return a
			}
		}
		return nil
	}

	if gi != nil {
		if ip := gi.IpAddress; ip != "" {
			if a := validatePrimaryIP(ip); a != nil {
				if a.To4() != nil {
					primaryIP4 = ip
				} else {
					primaryIP6 = ip
				}
			}
		}

		if len(gi.Net) > 0 {
			var ifaceSpecs []vmopv1.VirtualMachineNetworkInterfaceSpec
			if vm.Spec.Network != nil {
				ifaceSpecs = vm.Spec.Network.Interfaces
			}

			for i := range gi.Net {
				deviceKey := gi.Net[i].DeviceConfigId

				// Skip pseudo devices.
				if deviceKey < 0 {
					continue
				}

				var ifaceName string
				if idx, ok := deviceKeyToSpecIdx[deviceKey]; ok && idx < len(ifaceSpecs) {
					ifaceName = ifaceSpecs[idx].Name
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

	if bs := vm.Spec.Bootstrap; bs != nil && bs.CloudInit != nil {
		// When CloudInit is used, prefer the local IPs that the VMware datasource publishes.
		if ip4 := extraConfig[constants.CloudInitGuestInfoLocalIPv4Key]; ip4 != "" {
			if a := validatePrimaryIP(ip4); a != nil && a.To4() != nil {
				primaryIP4 = ip4
			}
		}

		if ip6 := extraConfig[constants.CloudInitGuestInfoLocalIPv6Key]; ip6 != "" {
			if a := validatePrimaryIP(ip6); a != nil && a.To4() == nil {
				primaryIP6 = ip6
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

// updateStorageStatus updates the status for all storage-related fields.
func updateStorageStatus(vm *vmopv1.VirtualMachine, moVM mo.VirtualMachine) {
	updateChangeBlockTracking(vm, moVM)
	updateVolumeStatus(vm, moVM)
	updateStorageUsage(vm, moVM)
}

func updateChangeBlockTracking(vm *vmopv1.VirtualMachine, moVM mo.VirtualMachine) {
	if moVM.Config != nil {
		vm.Status.ChangeBlockTracking = moVM.Config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}
}

func updateStorageUsage(vm *vmopv1.VirtualMachine, moVM mo.VirtualMachine) {

	var (
		other     int64
		disksUsed int64
		disksReqd int64
	)
	// Get the storage consumed by non-disks.
	if moVM.LayoutEx != nil {
		for i := range moVM.LayoutEx.File {
			f := moVM.LayoutEx.File[i]
			switch vimtypes.VirtualMachineFileLayoutExFileType(f.Type) {
			case vimtypes.VirtualMachineFileLayoutExFileTypeDiskDescriptor,
				vimtypes.VirtualMachineFileLayoutExFileTypeDiskExtent,
				vimtypes.VirtualMachineFileLayoutExFileTypeDigestDescriptor,
				vimtypes.VirtualMachineFileLayoutExFileTypeDigestExtent:

				// Skip disks

			default:
				other += f.UniqueSize
			}
		}
	}

	// Get the storage consumed by disks.
	for i := range vm.Status.Volumes {
		v := vm.Status.Volumes[i]
		if v.Type == vmopv1.VirtualMachineStorageDiskTypeClassic {
			if v.Used != nil {
				i, _ := v.Used.AsInt64()
				disksUsed += i
			}
			if v.Requested != nil {
				i, _ := v.Requested.AsInt64()
				disksReqd += i
			}
		}
	}

	if disksReqd == 0 && disksUsed == 0 && other == 0 {
		return
	}

	if vm.Status.Storage == nil {
		vm.Status.Storage = &vmopv1.VirtualMachineStorageStatus{}
	}

	if disksReqd > 0 {
		if vm.Status.Storage.Requested == nil {
			vm.Status.Storage.Requested = &vmopv1.VirtualMachineStorageStatusRequested{}
		}
		vm.Status.Storage.Requested.Disks = nil
	}
	if disksUsed > 0 || other > 0 {
		if vm.Status.Storage.Used == nil {
			vm.Status.Storage.Used = &vmopv1.VirtualMachineStorageStatusUsed{}
		}
		vm.Status.Storage.Used.Disks = nil
		vm.Status.Storage.Used.Other = nil
	}

	if disksReqd > 0 {
		vm.Status.Storage.Requested.Disks = BytesToResourceGiB(disksReqd)
	}
	if disksUsed > 0 {
		vm.Status.Storage.Used.Disks = BytesToResourceGiB(disksUsed)
	}

	if other > 0 {
		vm.Status.Storage.Used.Other = BytesToResourceGiB(other)
	}

	vm.Status.Storage.Total = BytesToResourceGiB(disksReqd + other)
}

func updateVolumeStatus(vm *vmopv1.VirtualMachine, moVM mo.VirtualMachine) {
	if moVM.Config == nil ||
		moVM.LayoutEx == nil ||
		len(moVM.LayoutEx.Disk) == 0 ||
		len(moVM.LayoutEx.File) == 0 ||
		len(moVM.Config.Hardware.Device) == 0 {

		return
	}

	existingDisksInStatus := map[string]int{}
	for i := range vm.Status.Volumes {
		if vol := vm.Status.Volumes[i]; vol.DiskUUID != "" {
			existingDisksInStatus[vol.DiskUUID] = i
		}
	}

	existingDisksInConfig := map[string]struct{}{}

	for i := range moVM.Config.Hardware.Device {
		vd, ok := moVM.Config.Hardware.Device[i].(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}

		var (
			diskUUID string
			fileName string
			isFCD    = vd.VDiskId != nil && vd.VDiskId.Id != ""
		)

		switch tb := vd.Backing.(type) {
		case *vimtypes.VirtualDiskFlatVer2BackingInfo:
			diskUUID = tb.Uuid
			fileName = tb.FileName
		case *vimtypes.VirtualDiskSeSparseBackingInfo:
			diskUUID = tb.Uuid
			fileName = tb.FileName
		case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
			diskUUID = tb.Uuid
			fileName = tb.FileName
		case *vimtypes.VirtualDiskSparseVer2BackingInfo:
			diskUUID = tb.Uuid
			fileName = tb.FileName
		case *vimtypes.VirtualDiskRawDiskVer2BackingInfo:
			diskUUID = tb.Uuid
			fileName = tb.DescriptorFileName
		}

		var diskPath object.DatastorePath
		if !diskPath.FromString(fileName) {
			continue
		}

		existingDisksInConfig[diskUUID] = struct{}{}

		ctx := context.TODO()

		if diskIndex, ok := existingDisksInStatus[diskUUID]; ok {
			// The disk is already in the list of volume statuses, so update the
			// existing status with the usage information.
			di, _ := vmdk.GetVirtualDiskInfoByUUID(ctx, nil, moVM, false, diskUUID)
			vm.Status.Volumes[diskIndex].Used = BytesToResourceGiB(di.UniqueSize)
			if di.CryptoKey.ProviderID != "" || di.CryptoKey.KeyID != "" {
				vm.Status.Volumes[diskIndex].Crypto = &vmopv1.VirtualMachineVolumeCryptoStatus{
					ProviderID: di.CryptoKey.ProviderID,
					KeyID:      di.CryptoKey.KeyID,
				}
			}
		} else if !isFCD {
			// The disk is a classic, non-FCD that must be added to the list of
			// volume statuses.
			di, _ := vmdk.GetVirtualDiskInfoByUUID(ctx, nil, moVM, false, diskUUID)
			dp := diskPath.Path
			volStatus := vmopv1.VirtualMachineVolumeStatus{
				Name:      strings.TrimSuffix(path.Base(dp), path.Ext(dp)),
				Type:      vmopv1.VirtualMachineStorageDiskTypeClassic,
				Attached:  true,
				DiskUUID:  diskUUID,
				Limit:     BytesToResourceGiB(di.CapacityInBytes),
				Requested: BytesToResourceGiB(di.CapacityInBytes),
				Used:      BytesToResourceGiB(di.UniqueSize),
			}
			if di.CryptoKey.ProviderID != "" || di.CryptoKey.KeyID != "" {
				volStatus.Crypto = &vmopv1.VirtualMachineVolumeCryptoStatus{
					ProviderID: di.CryptoKey.ProviderID,
					KeyID:      di.CryptoKey.KeyID,
				}
			}
			vm.Status.Volumes = append(vm.Status.Volumes, volStatus)
		}
	}

	// Remove any status entries for classic disks that no longer exist in
	// config.hardware.device.
	vm.Status.Volumes = slices.DeleteFunc(vm.Status.Volumes,
		func(e vmopv1.VirtualMachineVolumeStatus) bool {
			switch e.Type {
			case vmopv1.VirtualMachineStorageDiskTypeClassic:
				_, keep := existingDisksInConfig[e.DiskUUID]
				return !keep
			default:
				return false
			}
		})

	// This sort order is consistent with the logic from the volumes controller.
	vmopv1.SortVirtualMachineVolumeStatuses(vm.Status.Volumes)
}

const byteToGiB = 1 /* B */ * 1024 /* KiB */ * 1024 /* MiB */ * 1024 /* GiB */

// BytesToResourceGiB returns the resource.Quantity GiB value for the specified
// number of bytes.
func BytesToResourceGiB(b int64) *resource.Quantity {
	return ptr.To(resource.MustParse(fmt.Sprintf("%dGi", b/byteToGiB)))
}

type probeResult uint8

const (
	probeResultUnknown probeResult = iota
	probeResultSuccess
	probeResultFailure
)

const (
	probeReasonReady    string = "Ready"
	probeReasonNotReady string = "NotReady"
	probeReasonUnknown  string = "Unknown"
)

func heartbeatValue(value string) int {
	switch value {
	case string(vmopv1.GreenHeartbeatStatus):
		return 3
	case string(vmopv1.YellowHeartbeatStatus):
		return 2
	case string(vmopv1.RedHeartbeatStatus):
		return 1
	case string(vmopv1.GrayHeartbeatStatus):
		return 0
	default:
		return -1
	}
}

// updateProbeStatus updates a VM's status with the results of the configured
// readiness probes.
// Please note, this function returns early if the configured probe is TCP.
func updateProbeStatus(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	extraConfig map[string]string) {

	p := vm.Spec.ReadinessProbe
	if p == nil || p.TCPSocket != nil {
		return
	}

	var (
		result    probeResult
		resultMsg string
	)

	switch {
	case p.GuestHeartbeat != nil:
		result, resultMsg = updateProbeStatusHeartbeat(vm, moVM)
	case p.GuestInfo != nil:
		result, resultMsg = updateProbeStatusGuestInfo(vm, extraConfig)
	}

	var cond *metav1.Condition
	switch result {
	case probeResultSuccess:
		cond = conditions.TrueCondition(vmopv1.ReadyConditionType)
	case probeResultFailure:
		cond = conditions.FalseCondition(
			vmopv1.ReadyConditionType, probeReasonNotReady, "%s", resultMsg)
	default:
		cond = conditions.UnknownCondition(
			vmopv1.ReadyConditionType, probeReasonUnknown, "%s", resultMsg)
	}

	// Emit event whe the condition is added or its status changes.
	if c := conditions.Get(vm, cond.Type); c == nil || c.Status != cond.Status {
		recorder := vmoprecord.FromContext(ctx)
		if cond.Status == metav1.ConditionTrue {
			recorder.Eventf(vm, probeReasonReady, "")
		} else {
			recorder.Eventf(vm, cond.Reason, cond.Message)
		}

		// Log the time when the VM changes its readiness condition.
		logr.FromContextOrDiscard(ctx).Info(
			"VM resource readiness probe condition updated",
			"condition.status", cond.Status,
			"time", cond.LastTransitionTime,
			"reason", cond.Reason)
	}

	conditions.Set(vm, cond)
}

func updateProbeStatusHeartbeat(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (probeResult, string) {

	chb := string(moVM.GuestHeartbeatStatus)
	mhb := string(vm.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus)
	chv := heartbeatValue(chb)
	if chv < 0 {
		return probeResultUnknown, ""
	}
	if mhv := heartbeatValue(mhb); chv < mhv {
		return probeResultFailure, fmt.Sprintf(
			"heartbeat status %q is below threshold", chb)
	}
	return probeResultSuccess, ""
}

func updateProbeStatusGuestInfo(
	vm *vmopv1.VirtualMachine,
	extraConfig map[string]string) (probeResult, string) { //nolint:unparam

	if len(vm.Spec.ReadinessProbe.GuestInfo) == 0 {
		return probeResultUnknown, ""
	}

	for _, gi := range vm.Spec.ReadinessProbe.GuestInfo {
		key := fmt.Sprintf("guestinfo.%s", gi.Key)

		actualVal, ok := extraConfig[key]
		if !ok {
			return probeResultFailure, ""
		}

		if gi.Value == "" {
			// Matches everything.
			continue
		}

		expectedValRx, err := regexp.Compile(gi.Value)
		if err != nil {
			// Treat an invalid expressions as a wildcard too.
			continue
		}

		if !expectedValRx.MatchString(actualVal) {
			return probeResultFailure, ""
		}
	}

	return probeResultSuccess, ""
}
