// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

//nolint:revive
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type NetworkInterfaceResults struct {
	Results                   []NetworkInterfaceResult
	UpdatedEthCards           bool
	OrphanedNetworkInterfaces []ctrlclient.Object
}

type NetworkInterfaceResult struct {
	ObjectName string
	IPConfigs  []NetworkInterfaceIPConfig
	MacAddress string
	ExternalID string
	NetworkID  string
	Backing    object.NetworkReference

	Device    vimtypes.BaseVirtualDevice
	DeviceKey int32

	// Fields from the InterfaceSpec used later during customization.
	Name            string
	GuestDeviceName string
	NoIPAM          bool
	DHCP4           bool
	DHCP6           bool
	MTU             int64
	Nameservers     []string
	SearchDomains   []string
	Routes          []NetworkInterfaceRoute
}

type NetworkInterfaceIPConfig struct {
	IPCIDR  string // IP address in CIDR notation e.g. 192.168.10.42/24
	IsIPv4  bool
	Gateway string
}

type NetworkInterfaceRoute struct {
	To     string
	Via    string
	Metric int32
}

const (
	retryInterval           = 100 * time.Millisecond
	defaultEthernetCardType = "vmxnet3"
	gatewayIgnored          = "None"

	// VMNameLabel is the label put on a network interface CR that identifies its VM by name.
	VMNameLabel = pkg.VMOperatorKey + "/vm-name"
	// VMInterfaceNameLabel is the label put on the network interface CR identifies its name
	// in the VM network interface spec.
	VMInterfaceNameLabel = pkg.VMOperatorKey + "/vm-interface-name"
)

var (
	// RetryTimeout is var so tests can change it to shorten tests until we get rid of the poll.
	RetryTimeout = 15 * time.Second
)

// CreateAndWaitForNetworkInterfaces creates the appropriate CRs for the VM's network
// interfaces, and then waits for them to be reconciled by NCP (NSX-T) or NetOP (VDS).
//
// Networking has always been kind of a pain and clunky for us, and unfortunately this
// code suffers gotchas and other not-so-great limitations.
//
//   - Historically, this code used wait.PollImmediate() and we continue to do so here,
//     but eventually we should Watch() these resources. Note though, that in the very
//     common case, the CR is reconciled before our poll timeout, so that does save us
//     from bailing out of the Reconcile.
//   - NCP, NetOP and VPC CR Status inform us of the backing and IPAM info. However, for
//     our InterfaceSpec we allow for DHCP but neither NCP nor NetOP has a way for us to
//     mark the CR to don't do IPAM or to check DHCP is even enabled on the network. So
//     this burns an IP, and the user must know that DHCP is actually configured.
//   - CR naming has mostly been working by luck, and sometimes didn't offer very good
//     discoverability. Here, with v1a2 we now have a "name" field in our InterfaceSpec,
//     so we use that. A longer term option is to use GenerateName to ensure a unique name,
//     and then client.List() and filter by the OwnerRef to find the VM's network CRs, and to
//     annotate the CRs to help identify which VM InterfaceSpec it corresponds to.
//     Note that for existing v1a1 VMs we may need to add legacy name support here to
//     find their interface CRs.
func CreateAndWaitForNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
	clusterMoRef *vimtypes.ManagedObjectReference,
	networkSpec *vmopv1.VirtualMachineNetworkSpec) (NetworkInterfaceResults, error) {

	networkType := pkgcfg.FromContext(vmCtx).NetworkProviderType
	if networkType == "" {
		return NetworkInterfaceResults{}, fmt.Errorf("no network provider set")
	}

	var defaultToGlobalNameservers, defaultToGlobalSearchDomains bool
	if bootstrap := vmCtx.VM.Spec.Bootstrap; bootstrap != nil && bootstrap.CloudInit != nil {
		defaultToGlobalNameservers = ptr.DerefWithDefault(bootstrap.CloudInit.UseGlobalNameserversAsDefault, true)
		defaultToGlobalSearchDomains = ptr.DerefWithDefault(bootstrap.CloudInit.UseGlobalSearchDomainsAsDefault, true)
	}

	results := make([]NetworkInterfaceResult, 0, len(networkSpec.Interfaces))

	for i := range networkSpec.Interfaces {
		interfaceSpec := &networkSpec.Interfaces[i]

		var result *NetworkInterfaceResult
		var err error

		switch networkType {
		case pkgcfg.NetworkProviderTypeVDS:
			result, err = createNetOPNetworkInterface(vmCtx, client, vimClient, interfaceSpec)
		case pkgcfg.NetworkProviderTypeNSXT:
			result, err = createNCPNetworkInterface(vmCtx, client, vimClient, clusterMoRef, interfaceSpec)
		case pkgcfg.NetworkProviderTypeVPC:
			result, err = createVPCNetworkInterface(vmCtx, client, vimClient, clusterMoRef, interfaceSpec)
		case pkgcfg.NetworkProviderTypeNamed:
			result, err = createNamedNetworkInterface(vmCtx, finder, interfaceSpec)
		default:
			err = fmt.Errorf("unsupported network provider envvar value: %q", networkType)
		}

		if err != nil {
			return NetworkInterfaceResults{},
				fmt.Errorf("network interface %q error: %w", interfaceSpec.Name, err)
		}

		applyInterfaceSpecToResult(
			networkSpec,
			interfaceSpec,
			defaultToGlobalNameservers,
			defaultToGlobalSearchDomains,
			result)

		results = append(results, *result)
	}

	return NetworkInterfaceResults{
		Results: results,
	}, nil
}

// applyInterfaceSpecToResult applies the InterfaceSpec to results. Much of the InterfaceSpec - like DHCP -
// cannot be specified to the underlying network provider so apply those overrides to the results.
func applyInterfaceSpecToResult(
	networkSpec *vmopv1.VirtualMachineNetworkSpec,
	interfaceSpec *vmopv1.VirtualMachineNetworkInterfaceSpec,
	defaultToGlobalNameservers bool,
	defaultToGlobalSearchDomains bool,
	result *NetworkInterfaceResult) {

	result.Name = interfaceSpec.Name
	result.GuestDeviceName = interfaceSpec.GuestDeviceName
	if result.GuestDeviceName == "" {
		result.GuestDeviceName = result.Name
	}
	result.MacAddress = strings.ToLower(result.MacAddress)

	if interfaceSpec.MTU != nil {
		result.MTU = *interfaceSpec.MTU
	}

	if interfaceSpec.DHCP4 {
		result.DHCP4 = true
	}
	if interfaceSpec.DHCP6 {
		// We don't really support IPv6 yet so this is only enabled when specified in
		// the interface spec.
		result.DHCP6 = true
	}

	if len(interfaceSpec.Addresses) > 0 {
		if interfaceSpec.Gateway4 == "" || interfaceSpec.Gateway6 == "" {
			// Backfill the gateways from the network provider if not specified in
			// the interface spec before setting the user specified addresses. This
			// allows for just IP reservation to be done.
			// Our result is modeled after the network interface CRs, but that ends
			// up being cumbersome here. Instead, the gateways should be pulled up
			// in the top level results instead of being a part of the IP config.
			for i := range result.IPConfigs {
				gw := result.IPConfigs[i].Gateway
				if gw == "" {
					continue
				}

				if interfaceSpec.Gateway4 == "" { //nolint:gocritic
					if result.IPConfigs[i].IsIPv4 {
						interfaceSpec.Gateway4 = gw
					}
				} else if interfaceSpec.Gateway6 == "" {
					if !result.IPConfigs[i].IsIPv4 {
						interfaceSpec.Gateway6 = gw
					}
				} else {
					break
				}
			}
		}

		result.IPConfigs = make([]NetworkInterfaceIPConfig, 0, len(interfaceSpec.Addresses))
		for _, addr := range interfaceSpec.Addresses {
			ip, _, err := net.ParseCIDR(addr)
			if err != nil {
				continue
			}

			ipConfig := NetworkInterfaceIPConfig{
				IPCIDR: addr,
				IsIPv4: ip.To4() != nil,
			}

			result.IPConfigs = append(result.IPConfigs, ipConfig)
		}
	}

	if gw4, gw6 := interfaceSpec.Gateway4, interfaceSpec.Gateway6; gw4 != "" || gw6 != "" {
		// Set the gateway to their user-specified values. For multiple IPs, doing this for
		// every address may not end up making sense but otherwise hard to determine what
		// else to do but for addresses from our interface spec we do have the CIDR. It does
		// really end up mattering since for the network customization we'll use the first
		// gateway. Like mentioned above, how this is modeled vs our needs is a little funky
		// and should pull out the gateways out of returned IP configuration.
		for i := range result.IPConfigs {
			if gw4 != "" && result.IPConfigs[i].IsIPv4 {
				if gw4 == gatewayIgnored {
					// Clear network provider gateway.
					result.IPConfigs[i].Gateway = ""
				} else {
					result.IPConfigs[i].Gateway = gw4
				}
			} else if gw6 != "" && !result.IPConfigs[i].IsIPv4 {
				if gw6 == gatewayIgnored {
					// Clear network provider gateway.
					result.IPConfigs[i].Gateway = ""
				} else {
					result.IPConfigs[i].Gateway = gw6
				}
			}
		}
	}

	for _, route := range interfaceSpec.Routes {
		result.Routes = append(result.Routes, NetworkInterfaceRoute{To: route.To, Via: route.Via, Metric: route.Metric})
	}

	if n := interfaceSpec.Nameservers; len(n) > 0 {
		result.Nameservers = n
	} else if defaultToGlobalNameservers {
		result.Nameservers = networkSpec.Nameservers
	}

	if d := interfaceSpec.SearchDomains; len(d) > 0 {
		result.SearchDomains = d
	} else if defaultToGlobalSearchDomains {
		result.SearchDomains = networkSpec.SearchDomains
	}
}

func createNamedNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	finder *find.Finder,
	interfaceSpec *vmopv1.VirtualMachineNetworkInterfaceSpec) (*NetworkInterfaceResult, error) {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)
	if netRef := interfaceSpec.Network; netRef != nil {
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	if networkRefType.Kind != "" || networkRefType.APIVersion != "" {
		return nil, fmt.Errorf("network TypeMeta not supported for name network: %v", networkRefType)
	}

	if networkRefName == "" {
		return nil, fmt.Errorf("network name is required")
	}

	backing, err := finder.Network(vmCtx, networkRefName)
	if err != nil {
		return nil, fmt.Errorf("unable to find named network %q: %w", networkRefName, err)
	}

	return &NetworkInterfaceResult{
		NetworkID:  networkRefName,
		Backing:    backing,
		MacAddress: interfaceSpec.MACAddr,
	}, nil
}

// NetOPCRName returns the name to be used for the NetOP NetworkInterface CR.
func NetOPCRName(vmName, networkName, interfaceName string, isV1A1 bool) string {
	var name string

	if isV1A1 {
		// Old naming convention: each network can really only have 1 NIC.
		if networkName != "" {
			name = fmt.Sprintf("%s-%s", networkName, vmName)
		} else {
			name = vmName
		}
	} else {
		if networkName != "" {
			name = fmt.Sprintf("%s-%s-%s", vmName, networkName, interfaceName)
		} else {
			name = fmt.Sprintf("%s-%s", vmName, interfaceName)
		}
	}

	return name
}

func createNetOPNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	interfaceSpec *vmopv1.VirtualMachineNetworkInterfaceSpec) (*NetworkInterfaceResult, error) {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		// If Name is empty, NetOP will try to select the namespace default.
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	if kind := networkRefType.Kind; kind != "" && kind != "Network" {
		return nil, fmt.Errorf("network kind %q is not supported for VDS", kind)
	}

	netIf := &netopv1alpha1.NetworkInterface{}
	netIfKey := types.NamespacedName{
		Namespace: vmCtx.VM.Namespace,
		Name:      NetOPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, true),
	}

	// Check if a networkIf object exists with the older (v1a1) naming convention.
	if err := client.Get(vmCtx, netIfKey, netIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// If NotFound set the netIf to the new v1a2 naming convention.
		netIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NetOPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, false),
			Namespace: vmCtx.VM.Namespace,
		}
	}

	_, err := controllerutil.CreateOrPatch(vmCtx, client, netIf, func() error {
		if err := SetNetworkInterfaceOwnerRef(vmCtx.VM, netIf, client.Scheme()); err != nil {
			// If this fails we likely have an object name collision, and we're in a tough spot.
			return err
		}

		/* We can only set this once we know all the hosts have been upgraded.
		if netIf.ResourceVersion == "" {
			// For new interfaces, set the ExternalID so we can better uniquely identify them.
			netIf.Spec.ExternalID = uuid.NewString()
		}
		*/

		if netIf.Labels == nil {
			netIf.Labels = map[string]string{}
		}
		netIf.Labels[VMNameLabel] = vmCtx.VM.Name
		netIf.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		// NetOp will update the Spec with the default network name so we don't clear that
		// here if using the default network.
		if networkRefName != "" {
			netIf.Spec.NetworkName = networkRefName
		}
		// NetOP only defines a VMXNet3 type, but it doesn't really matter for our purposes.
		netIf.Spec.Type = netopv1alpha1.NetworkInterfaceTypeVMXNet3
		return nil
	})

	if err != nil {
		return nil, err
	}

	netIf, err = waitForReadyNetworkInterface(vmCtx, client, netIf.Name)
	if err != nil {
		return nil, err
	}

	// The NetworkInterface does not accept an input MAC address (nor does it ever
	// generate one to set in the Status) but if the user requested a specific one
	// assign that here so we'll use it for the device's MAC.
	// To better really support this, we should add a MAC address field to the Spec.
	if interfaceSpec.MACAddr != "" {
		netIf.Status.MacAddress = interfaceSpec.MACAddr
	}

	return netOpNetIfToResult(vimClient, netIf), nil
}

func netOpNetIfToResult(
	vimClient *vim25.Client,
	netIf *netopv1alpha1.NetworkInterface) *NetworkInterfaceResult {

	pgObjRef := vimtypes.ManagedObjectReference{
		Type:  "DistributedVirtualPortgroup",
		Value: netIf.Status.NetworkID,
	}

	result := &NetworkInterfaceResult{
		ObjectName: netIf.Name,
		MacAddress: netIf.Status.MacAddress,
		ExternalID: netIf.Status.ExternalID,
		NetworkID:  netIf.Status.NetworkID,
		Backing:    object.NewDistributedVirtualPortgroup(vimClient, pgObjRef),
	}

	switch netIf.Status.IPAssignmentMode {
	case netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP:
		result.DHCP4 = true
	case netopv1alpha1.NetworkInterfaceIPAssignmentModeNone:
		result.NoIPAM = true
	default: // netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
		for _, ip := range netIf.Status.IPConfigs {
			ipConfig := NetworkInterfaceIPConfig{
				IPCIDR:  ipCIDRNotation(ip.IP, ip.SubnetMask, ip.IPFamily == corev1.IPv4Protocol),
				IsIPv4:  ip.IPFamily == corev1.IPv4Protocol,
				Gateway: ip.Gateway,
			}
			result.IPConfigs = append(result.IPConfigs, ipConfig)
		}
	}

	return result
}

func findNetOPCondition(
	netIf *netopv1alpha1.NetworkInterface,
	condType netopv1alpha1.NetworkInterfaceConditionType) *netopv1alpha1.NetworkInterfaceCondition {

	for i := range netIf.Status.Conditions {
		if netIf.Status.Conditions[i].Type == condType {
			return &netIf.Status.Conditions[i]
		}
	}
	return nil
}

func waitForReadyNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	name string) (*netopv1alpha1.NetworkInterface, error) {

	netIf := &netopv1alpha1.NetworkInterface{}
	netIfKey := types.NamespacedName{Namespace: vmCtx.VM.Namespace, Name: name}

	// TODO: Watch() this type instead.
	err := wait.PollUntilContextTimeout(vmCtx, retryInterval, RetryTimeout, true, func(_ context.Context) (bool, error) {
		if err := client.Get(vmCtx, netIfKey, netIf); err != nil {
			return false, ctrlclient.IgnoreNotFound(err)
		}

		cond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceReady)
		return cond != nil && cond.Status == corev1.ConditionTrue, nil
	})

	if err != nil {
		if wait.Interrupted(err) {
			// Try to return a more meaningful error when timed out.
			if cond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceFailure); cond != nil && cond.Status == corev1.ConditionTrue {
				return nil, fmt.Errorf("network interface failure: %s - %s", cond.Reason, cond.Message)
			}
			if cond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceReady); cond != nil && cond.Status == corev1.ConditionFalse {
				return nil, fmt.Errorf("network interface is not ready: %s - %s", cond.Reason, cond.Message)
			}
			return nil, fmt.Errorf("network interface is not ready yet")
		}

		return nil, err
	}

	return netIf, nil
}

// NCPCRName returns the name to be used for the NCP VirtualNetworkInterface CR.
func NCPCRName(vmName, networkName, interfaceName string, isV1A1 bool) string {
	var name string

	if isV1A1 {
		name = fmt.Sprintf("%s-lsp", vmName)
		if networkName != "" {
			name = fmt.Sprintf("%s-%s", networkName, name)
		}

	} else {
		if networkName != "" {
			name = fmt.Sprintf("%s-%s-%s", vmName, networkName, interfaceName)
		} else {
			name = fmt.Sprintf("%s-%s", vmName, interfaceName)
		}
	}

	return name
}

func createNCPNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec *vmopv1.VirtualMachineNetworkInterfaceSpec) (*NetworkInterfaceResult, error) {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		// If Name is empty, NCP will use the namespace default.
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	// TODO: Do we need to still support the odd-ball NetOP in NSX-T? Sigh. Do that check here if needed.
	if kind := networkRefType.Kind; kind != "" && kind != "VirtualNetwork" {
		return nil, fmt.Errorf("network kind %q is not supported for NCP", kind)
	}

	vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
	vnetIfKey := types.NamespacedName{
		Namespace: vmCtx.VM.Namespace,
		Name:      NCPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, true),
	}

	// check if a networkIf object exists with the older (v1a1) naming convention
	if err := client.Get(vmCtx, vnetIfKey, vnetIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// if notFound set the vnetIf to use the new v1a2 naming convention
		vnetIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NCPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, false),
			Namespace: vmCtx.VM.Namespace,
		}
	}

	_, err := controllerutil.CreateOrPatch(vmCtx, client, vnetIf, func() error {
		if err := SetNetworkInterfaceOwnerRef(vmCtx.VM, vnetIf, client.Scheme()); err != nil {
			return err
		}

		if vnetIf.Labels == nil {
			vnetIf.Labels = map[string]string{}
		}
		vnetIf.Labels[VMNameLabel] = vmCtx.VM.Name
		vnetIf.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		vnetIf.Spec.VirtualNetwork = networkRefName
		return nil
	})

	if err != nil {
		return nil, err
	}

	vnetIf, err = waitForReadyNCPNetworkInterface(vmCtx, client, vnetIf.Name)
	if err != nil {
		return nil, err
	}

	return ncpNetIfToResult(vmCtx, vimClient, clusterMoRef, vnetIf)
}

func ncpNetIfToResult(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	vnetIf *ncpv1alpha1.VirtualNetworkInterface) (*NetworkInterfaceResult, error) {

	var backing object.NetworkReference
	networkID := vnetIf.Status.ProviderStatus.NsxLogicalSwitchID
	if clusterMoRef != nil {
		ccr := object.NewClusterComputeResource(vimClient, *clusterMoRef)

		networkRef, err := searchNsxtNetworkReference(ctx, ccr, networkID)
		if err != nil {
			return nil, err
		}

		backing = networkRef
	} else {
		backing = newNSXOpaqueNetwork(networkID)
	}

	result := &NetworkInterfaceResult{
		ObjectName: vnetIf.Name,
		MacAddress: vnetIf.Status.MacAddress,
		ExternalID: vnetIf.Status.InterfaceID,
		NetworkID:  networkID,
		Backing:    backing,
	}

	if ipAddress := vnetIf.Status.IPAddresses; len(ipAddress) == 0 || (len(ipAddress) == 1 && ipAddress[0].IP == "") {
		result.DHCP4 = true
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

			result.IPConfigs = append(result.IPConfigs, ipConfig)
		}
	}

	return result, nil
}

// VPCCRName returns the name to be used for the VPC SubnetPort CR.
func VPCCRName(vmName, networkName, interfaceName string) string {
	var name string

	if networkName != "" {
		name = fmt.Sprintf("%s-%s-%s", vmName, networkName, interfaceName)
	} else {
		name = fmt.Sprintf("%s-%s", vmName, interfaceName)
	}

	return name
}

func createVPCNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec *vmopv1.VirtualMachineNetworkInterfaceSpec) (*NetworkInterfaceResult, error) {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	vpcSubnetPort := &vpcv1alpha1.SubnetPort{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VPCCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name),
			Namespace: vmCtx.VM.Namespace,
		},
	}

	switch networkRefType.Kind {
	case "SubnetSet", "":
		vpcSubnetPort.Spec.SubnetSet = networkRefName
	case "Subnet":
		vpcSubnetPort.Spec.Subnet = networkRefName
	default:
		return nil, fmt.Errorf("network kind %q is not supported for VPC", networkRefType.Kind)
	}

	_, err := controllerutil.CreateOrPatch(vmCtx, client, vpcSubnetPort, func() error {
		if err := SetNetworkInterfaceOwnerRef(vmCtx.VM, vpcSubnetPort, client.Scheme()); err != nil {
			return err
		}

		if vpcSubnetPort.Labels == nil {
			vpcSubnetPort.Labels = map[string]string{}
		}
		vpcSubnetPort.Labels[VMNameLabel] = vmCtx.VM.Name
		vpcSubnetPort.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		if vpcSubnetPort.Annotations == nil {
			vpcSubnetPort.Annotations = make(map[string]string)
		}
		vpcSubnetPort.Annotations[constants.VPCAttachmentRef] = "virtualmachine/" + vmCtx.VM.Name + "/" + interfaceSpec.Name

		vpcSubnetPort.Spec.AddressBindings = nil

		// TBD: It doesn't look like VPC actually reconciles if these change.
		switch {
		case len(interfaceSpec.Addresses) > 0:
			for _, ipCidr := range interfaceSpec.Addresses {
				// Our interface spec IPs include the CIDR but VPC takes just
				// the IP address. This can lead to funkiness if the user
				// specified an invalid prefix for this network. Here is what
				// we're going to do: applyInterfaceSpecToResult() will just use
				// whatever comes back in the SubnetPort Status, ignoring the
				// user prefix. It might be nicer to allow bare IPs only when
				// using VPC.
				ip, _, err := pkgutil.ParseIP(ipCidr)

				var skipReason string
				switch {
				case err != nil:
					skipReason = err.Error()
				case ip == nil:
					skipReason = "nil ip"
				case ip.IsUnspecified():
					skipReason = "unspecified"
				case ip.IsLinkLocalMulticast():
					skipReason = "link local multicast"
				case ip.IsLinkLocalUnicast():
					skipReason = "link local unicast"
				case ip.IsLoopback():
					skipReason = "loopback"
				}

				if skipReason != "" {
					vmCtx.Logger.Info(
						"Skipping IP address",
						"ip", ipCidr,
						"reason", skipReason)
					continue
				}

				// Despite being a list, VPC currently only supports just one PortAddressBinding.
				vpcSubnetPort.Spec.AddressBindings = []vpcv1alpha1.PortAddressBinding{
					{
						IPAddress:  ip.String(),
						MACAddress: strings.ToLower(interfaceSpec.MACAddr),
					},
				}
				break
			}
		case interfaceSpec.MACAddr != "":
			// TBD: VPC will default the MAC when only specifying an IP, but
			// will not do IPAM when only the specifying the MAC, but they'll
			// have to fix that for no IPAM, right, right?
			vpcSubnetPort.Spec.AddressBindings = []vpcv1alpha1.PortAddressBinding{
				{
					MACAddress: strings.ToLower(interfaceSpec.MACAddr),
				},
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	vpcSubnetPort, err = waitForReadyVPCSubnetPort(vmCtx, client, vpcSubnetPort.Name)
	if err != nil {
		return nil, err
	}

	return vpcSubnetPortToResult(vmCtx, vimClient, clusterMoRef, vpcSubnetPort)
}

func vpcSubnetPortToResult(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	subnetPort *vpcv1alpha1.SubnetPort) (*NetworkInterfaceResult, error) {

	var backing object.NetworkReference
	networkID := subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID
	if clusterMoRef != nil {
		ccr := object.NewClusterComputeResource(vimClient, *clusterMoRef)
		// VPC is an NSX-T construct that is attached to an NSX-T Project.
		networkRef, err := searchNsxtNetworkReference(ctx, ccr, networkID)
		if err != nil {
			return nil, err
		}

		backing = networkRef
	} else {
		backing = newNSXOpaqueNetwork(networkID)
	}

	result := &NetworkInterfaceResult{
		ObjectName: subnetPort.Name,
		MacAddress: subnetPort.Status.NetworkInterfaceConfig.MACAddress,
		ExternalID: subnetPort.Status.Attachment.ID,
		NetworkID:  networkID,
		Backing:    backing,
	}

	for _, ipAddr := range subnetPort.Status.NetworkInterfaceConfig.IPAddresses {
		if ipAddr.IPAddress == "" {
			// For DHCP and NoIPAM, IPAddress will be unset but Gateway will be set.
			continue
		}
		ip, _, _ := net.ParseCIDR(ipAddr.IPAddress)
		isIPv4 := ip.To4() != nil
		ipConfig := NetworkInterfaceIPConfig{
			IPCIDR:  ipAddr.IPAddress,
			IsIPv4:  isIPv4,
			Gateway: ipAddr.Gateway,
		}

		result.IPConfigs = append(result.IPConfigs, ipConfig)
	}

	// TBD: What behavior do we want for an NoIPAM subnet but user specified addresses?
	if len(result.IPConfigs) == 0 {
		if !subnetPort.Status.NetworkInterfaceConfig.DHCPDeactivatedOnSubnet {
			result.DHCP4 = true
		} else {
			result.NoIPAM = true
		}
	}

	return result, nil
}

func waitForReadyVPCSubnetPort(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	name string) (*vpcv1alpha1.SubnetPort, error) {

	subnetPort := &vpcv1alpha1.SubnetPort{}
	subnetPortKey := types.NamespacedName{Namespace: vmCtx.VM.Namespace, Name: name}

	// TODO: Watch() this type instead.
	err := wait.PollUntilContextTimeout(vmCtx, retryInterval, RetryTimeout, true, func(_ context.Context) (bool, error) {
		if err := client.Get(vmCtx, subnetPortKey, subnetPort); err != nil {
			return false, ctrlclient.IgnoreNotFound(err)
		}

		for _, condition := range subnetPort.Status.Conditions {
			if condition.Type == vpcv1alpha1.Ready && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})

	if err != nil {
		if wait.Interrupted(err) {
			// Try to return a more meaningful error when timed out.
			for _, cond := range subnetPort.Status.Conditions {
				if cond.Type == vpcv1alpha1.Ready && cond.Status != corev1.ConditionTrue {
					return nil, fmt.Errorf("network interface is not ready: %s - %s", cond.Reason, cond.Message)
				}
			}
			return nil, fmt.Errorf("network interface is not ready yet")
		}

		return nil, err
	}

	return subnetPort, nil
}

func waitForReadyNCPNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	name string) (*ncpv1alpha1.VirtualNetworkInterface, error) {

	vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
	vnetIfKey := types.NamespacedName{Namespace: vmCtx.VM.Namespace, Name: name}

	// TODO: Watch() this type instead.
	err := wait.PollUntilContextTimeout(vmCtx, retryInterval, RetryTimeout, true, func(_ context.Context) (bool, error) {
		if err := client.Get(vmCtx, vnetIfKey, vnetIf); err != nil {
			return false, ctrlclient.IgnoreNotFound(err)
		}

		for _, condition := range vnetIf.Status.Conditions {
			// TODO: Does NCP define condition constants?
			if strings.Contains(condition.Type, "Ready") && strings.Contains(condition.Status, "True") {
				return true, nil
			}
		}

		return false, nil
	})

	if err != nil {
		if wait.Interrupted(err) {
			// Try to return a more meaningful error when timed out.
			for _, cond := range vnetIf.Status.Conditions {
				if strings.Contains(cond.Type, "Ready") && !strings.Contains(cond.Status, "True") {
					return nil, fmt.Errorf("network interface is not ready: %s - %s", cond.Reason, cond.Message)
				}
			}
			// TODO: NCP also has an annotation but that usually doesn't provide very useful details.
			return nil, fmt.Errorf("network interface is not ready yet")
		}

		return nil, err
	}

	if vnetIf.Status.ProviderStatus == nil {
		return nil, fmt.Errorf("network interface is ready but does not have provider status")
	}

	return vnetIf, nil
}

// ipCIDRNotation takes the IP and subnet mask and returns the IP in CIDR notation.
// TODO: Better error checking. Nail down exactly how we want handle IPv4inV6 addresses.
func ipCIDRNotation(ip string, mask string, isIPv4 bool) string {
	if isIPv4 {
		ipNet := net.IPNet{
			IP:   net.ParseIP(ip).To4(),
			Mask: net.IPMask(net.ParseIP(mask).To4()),
		}
		return ipNet.String()
	}

	ipNet := net.IPNet{
		IP:   net.ParseIP(ip).To16(),
		Mask: net.IPMask(net.ParseIP(mask).To16()),
	}

	return ipNet.String()
}

// CreateDefaultEthCard creates a default Ethernet card attached to the backing. This is used
// when the VM Class ConfigSpec does not have a device entry for a VM Spec network interface,
// so we need a new device.
func CreateDefaultEthCard(
	ctx context.Context,
	result *NetworkInterfaceResult) (vimtypes.BaseVirtualDevice, error) {

	backing, err := result.Backing.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get ethernet card backing info for network %v: %w", result.Backing.Reference(), err)
	}

	dev, err := object.EthernetCardTypes().CreateEthernetCard(defaultEthernetCardType, backing)
	if err != nil {
		return nil, fmt.Errorf("unable to create ethernet card network %v: %w", result.Backing.Reference(), err)
	}

	ethCard := dev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	ethCard.ExternalId = result.ExternalID
	if result.MacAddress != "" {
		ethCard.MacAddress = result.MacAddress
		ethCard.AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
	} else {
		ethCard.AddressType = string(vimtypes.VirtualEthernetCardMacTypeGenerated) // TODO: Or TypeAssigned?
	}

	return dev, nil
}

// ApplyInterfaceResultToVirtualEthCard applies the interface result from the NetOP/NCP
// provider to an existing Ethernet device from the class ConfigSpec.
func ApplyInterfaceResultToVirtualEthCard(
	ctx context.Context,
	ethCard *vimtypes.VirtualEthernetCard,
	result *NetworkInterfaceResult) error {

	backing, err := result.Backing.EthernetCardBackingInfo(ctx)
	if err != nil {
		return fmt.Errorf("unable to get ethernet card backing info for network %v: %w", result.NetworkID, err)
	}
	ethCard.Backing = backing

	ethCard.ExternalId = result.ExternalID
	if result.MacAddress != "" {
		ethCard.MacAddress = result.MacAddress
		ethCard.AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
	} else { //nolint:staticcheck
		// BMV: IMO this must be Generated/TypeAssigned to avoid major foot gun, but we have tests assuming
		// this is left as-is.
		// ethCard.MacAddress = ""
		// ethCard.AddressType = string(vimtypes.VirtualEthernetCardMacTypeGenerated)
	}

	return nil
}

func SetNetworkInterfaceOwnerRef(vm *vmopv1.VirtualMachine, object metav1.Object, scheme *runtime.Scheme) error {
	err := controllerutil.SetControllerReference(vm, object, scheme)
	if err != nil {
		var aoe *controllerutil.AlreadyOwnedError
		if !errors.As(err, &aoe) {
			return fmt.Errorf("failed to set controller owner ref for network interface: %w", err)
		}

		if owner := aoe.Owner; owner.Kind == "VirtualMachine" {
			gv, err := schema.ParseGroupVersion(owner.APIVersion)
			if err != nil {
				return err
			}

			if vmopv1.GroupName == gv.Group {
				// This network interface CR has another VM marked as the controller
				// owner. That is not expected, and to prevent us from using an
				// interface CR that does not belong to this VM and return an error.
				return fmt.Errorf("network interface CR %s is already owned by VM %s", object.GetName(), owner.Name)
			}
		}

		// Historically, we were just setting an owner ref instead of being the controller
		// owner on the network interface CRs we created. Fallback to just that since this
		// interface CR already has a controller owner but ideally we would have been using
		// a controller ref from the start so we better assert that this interface is for
		// this VM.
		if err := controllerutil.SetOwnerReference(vm, object, scheme); err != nil {
			return fmt.Errorf("failed to set owner ref for network interface: %w", err)
		}
	}

	return nil
}
