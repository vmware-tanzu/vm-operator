// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
)

// IPFamily represents the IP Family (IPv4 or IPv6). This type is used
// to express the family of an IP expressed by a type (i.e. service.Spec.IPFamily)
// NOTE: Copied from k8s.io/api/core/v1" because VM Operator is using old version
type IPFamily string

const (
	// IPv4Protocol indicates that this IP is IPv4 protocol
	IPv4Protocol IPFamily = "IPv4"
	// IPv6Protocol indicates that this IP is IPv6 protocol
	IPv6Protocol IPFamily = "IPv6"
)

// IPConfig represents an IP configuration.
type IPConfig struct {
	// IP setting.
	IP string
	// IPFamily specifies the IP family (IPv4 vs IPv6) the IP belongs to.
	IPFamily IPFamily
	// Gateway setting.
	Gateway string
	// SubnetMask setting.
	SubnetMask string
}

const (
	NsxtNetworkType = "nsx-t"
	VdsNetworkType  = "vsphere-distributed"

	defaultEthernetCardType = "vmxnet3"
	retryInterval           = 100 * time.Millisecond
	retryTimeout            = 15 * time.Second
)

type NetworkInterfaceInfo struct {
	Device          vimtypes.BaseVirtualDevice
	Customization   *vimtypes.CustomizationAdapterMapping
	IPConfiguration IPConfig
}

type NetworkInterfaceInfoList []NetworkInterfaceInfo

func (l NetworkInterfaceInfoList) GetVirtualDeviceList() object.VirtualDeviceList {
	var devList object.VirtualDeviceList
	for _, info := range l {
		devList = append(devList, info.Device.(vimtypes.BaseVirtualDevice))
	}
	return devList
}

func (l NetworkInterfaceInfoList) GetInterfaceCustomizations() []vimtypes.CustomizationAdapterMapping {
	var mappings []vimtypes.CustomizationAdapterMapping
	for _, info := range l {
		mappings = append(mappings, *info.Customization)
	}
	return mappings
}

func (l NetworkInterfaceInfoList) GetIPConfigs() []IPConfig {
	var ipConfigs []IPConfig
	for _, info := range l {
		ipConfigs = append(ipConfigs, info.IPConfiguration)
	}
	return ipConfigs
}

// NetworkProvider sets up network for different type of network
type NetworkProvider interface {
	// EnsureNetworkInterface returns the NetworkInterfaceInfo for the vif.
	EnsureNetworkInterface(vmCtx VMContext, vif *vmopv1alpha1.VirtualMachineNetworkInterface) (*NetworkInterfaceInfo, error)
}

type networkProvider struct {
	nsxt  NetworkProvider
	netOp NetworkProvider
	named NetworkProvider

	scheme *runtime.Scheme
}

func NewNetworkProvider(
	k8sClient ctrlruntime.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
	cluster *object.ClusterComputeResource,
	scheme *runtime.Scheme) NetworkProvider {

	return &networkProvider{
		nsxt:   newNsxtNetworkProvider(k8sClient, finder, cluster, scheme),
		netOp:  newNetOpNetworkProvider(k8sClient, vimClient, finder, cluster, scheme),
		named:  newNamedNetworkProvider(finder),
		scheme: scheme,
	}
}

func (np *networkProvider) EnsureNetworkInterface(vmCtx VMContext, vif *vmopv1alpha1.VirtualMachineNetworkInterface) (*NetworkInterfaceInfo, error) {
	if providerRef := vif.ProviderRef; providerRef != nil {
		// ProviderRef is only supported for NetOP types.
		gvk, err := apiutil.GVKForObject(&netopv1alpha1.NetworkInterface{}, np.scheme)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot get GroupVersionKind for NetworkInterface object")
		}

		if gvk.Group != providerRef.APIGroup || gvk.Version != providerRef.APIVersion || gvk.Kind != providerRef.Kind {
			err := fmt.Errorf("unsupported NetworkInterface ProviderRef: %+v Supported: %+v", providerRef, gvk)
			return nil, err
		}

		return np.netOp.EnsureNetworkInterface(vmCtx, vif)
	}

	switch vif.NetworkType {
	case NsxtNetworkType:
		return np.nsxt.EnsureNetworkInterface(vmCtx, vif)
	case VdsNetworkType:
		return np.netOp.EnsureNetworkInterface(vmCtx, vif)
	case "":
		return np.named.EnsureNetworkInterface(vmCtx, vif)
	default:
		return nil, fmt.Errorf("failed to create network provider for network type %q", vif.NetworkType)
	}
}

// createEthernetCard creates an ethernet card with the network reference backing
func createEthernetCard(ctx context.Context, network object.NetworkReference, ethCardType string) (vimtypes.BaseVirtualDevice, error) {
	if ethCardType == "" {
		ethCardType = defaultEthernetCardType
	}

	backing, err := network.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get ethernet card backing info for network %v", network.Reference())
	}

	dev, err := object.EthernetCardTypes().CreateEthernetCard(ethCardType, backing)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create ethernet card %q for network %v", ethCardType, network.Reference())
	}

	return dev, nil
}

func configureEthernetCard(ethDev vimtypes.BaseVirtualDevice, externalID, macAddress string) {
	card := ethDev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()

	card.ExternalId = externalID
	if macAddress != "" {
		card.MacAddress = macAddress
		card.AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
	} else {
		card.AddressType = string(vimtypes.VirtualEthernetCardMacTypeGenerated)
	}
}

func newNamedNetworkProvider(finder *find.Finder) *namedNetworkProvider {
	return &namedNetworkProvider{
		finder: finder,
	}
}

type namedNetworkProvider struct {
	finder *find.Finder
}

func (np *namedNetworkProvider) EnsureNetworkInterface(
	vmCtx VMContext,
	vif *vmopv1alpha1.VirtualMachineNetworkInterface) (*NetworkInterfaceInfo, error) {

	networkRef, err := np.finder.Network(vmCtx, vif.NetworkName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find network %q", vif.NetworkName)
	}

	ethDev, err := createEthernetCard(vmCtx, networkRef, vif.EthernetCardType)
	if err != nil {
		return nil, err
	}

	return &NetworkInterfaceInfo{
		Device: ethDev,
		Customization: &vimtypes.CustomizationAdapterMapping{
			Adapter: vimtypes.CustomizationIPSettings{
				Ip: &vimtypes.CustomizationDhcpIpGenerator{},
			},
		},
		IPConfiguration: IPConfig{},
	}, nil
}

// +kubebuilder:rbac:groups=netoperator.vmware.com,resources=networkinterfaces;vmxnet3networkinterfaces,verbs=get;list;watch;create;update;patch;delete

// newNetOpNetworkProvider returns a netOpNetworkProvider instance
func newNetOpNetworkProvider(
	k8sClient ctrlruntime.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
	cluster *object.ClusterComputeResource,
	scheme *runtime.Scheme) *netOpNetworkProvider {

	return &netOpNetworkProvider{
		k8sClient: k8sClient,
		scheme:    scheme,
		vimClient: vimClient,
		finder:    finder,
		cluster:   cluster,
	}
}

type netOpNetworkProvider struct {
	k8sClient ctrlruntime.Client
	scheme    *runtime.Scheme
	vimClient *vim25.Client
	finder    *find.Finder
	cluster   *object.ClusterComputeResource
}

// BMV: This is similar to what NSX does but isn't really right: we can only have one
// interface per network. Although if we had multiple interfaces per network, we really
// don't have a way to identify each NIC so true reconciliation is broken.
func (np *netOpNetworkProvider) networkInterfaceName(networkName, vmName string) string {
	return fmt.Sprintf("%s-%s", networkName, vmName)
}

// createNetworkInterface creates a NetOP NetworkInterface for the VM network interface
func (np *netOpNetworkProvider) createNetworkInterface(
	vmCtx VMContext,
	vmIf *vmopv1alpha1.VirtualMachineNetworkInterface) (*netopv1alpha1.NetworkInterface, error) {

	if vmIf.ProviderRef == nil {
		// Create or Update our NetworkInterface CR when ProviderRef is unset.
		netIf := &netopv1alpha1.NetworkInterface{
			ObjectMeta: metav1.ObjectMeta{
				Name:      np.networkInterfaceName(vmIf.NetworkName, vmCtx.VM.Name),
				Namespace: vmCtx.VM.Namespace,
			},
		}

		// The only type defined by NetOP (but it doesn't care).
		cardType := netopv1alpha1.NetworkInterfaceTypeVMXNet3

		_, err := controllerutil.CreateOrUpdate(vmCtx, np.k8sClient, netIf, func() error {
			if err := controllerutil.SetOwnerReference(vmCtx.VM, netIf, np.scheme); err != nil {
				return err
			}

			netIf.Spec = netopv1alpha1.NetworkInterfaceSpec{
				NetworkName: vmIf.NetworkName,
				Type:        cardType,
			}
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return np.waitForReadyNetworkInterface(vmCtx, vmIf)
}

func (np *netOpNetworkProvider) networkForPortGroupId(portGroupId string) (object.NetworkReference, error) {
	pgObjRef := vimtypes.ManagedObjectReference{
		Type:  "DistributedVirtualPortgroup",
		Value: portGroupId,
	}

	return object.NewDistributedVirtualPortgroup(np.vimClient, pgObjRef), nil
}

func (np *netOpNetworkProvider) getNetworkRef(ctx context.Context, networkType, networkID string) (object.NetworkReference, error) {
	switch networkType {
	case VdsNetworkType:
		return np.networkForPortGroupId(networkID)
	case NsxtNetworkType:
		return searchNsxtNetworkReference(ctx, np.finder, np.cluster, networkID)
	default:
		return nil, fmt.Errorf("unsupported NetOP network type %s", networkType)
	}
}

func (np *netOpNetworkProvider) createEthernetCard(
	vmCtx VMContext,
	vif *vmopv1alpha1.VirtualMachineNetworkInterface,
	netIf *netopv1alpha1.NetworkInterface) (vimtypes.BaseVirtualDevice, error) {

	networkRef, err := np.getNetworkRef(vmCtx, vif.NetworkType, netIf.Status.NetworkID)
	if err != nil {
		return nil, err
	}

	ethDev, err := createEthernetCard(vmCtx, networkRef, vif.EthernetCardType)
	if err != nil {
		return nil, err
	}

	configureEthernetCard(ethDev, netIf.Status.ExternalID, netIf.Status.MacAddress)

	return ethDev, nil
}

func (np *netOpNetworkProvider) waitForReadyNetworkInterface(
	vmCtx VMContext,
	vmIf *vmopv1alpha1.VirtualMachineNetworkInterface) (*netopv1alpha1.NetworkInterface, error) {

	var name string
	if vmIf.ProviderRef != nil {
		name = vmIf.ProviderRef.Name
	} else {
		name = np.networkInterfaceName(vmIf.NetworkName, vmCtx.VM.Name)
	}

	var netIf *netopv1alpha1.NetworkInterface
	netIfKey := types.NamespacedName{Namespace: vmCtx.VM.Namespace, Name: name}

	// TODO: Watch() this type instead.
	err := wait.PollImmediate(retryInterval, retryTimeout, func() (bool, error) {
		instance := &netopv1alpha1.NetworkInterface{}
		if err := np.k8sClient.Get(vmCtx, netIfKey, instance); err != nil {
			return false, err
		}

		for _, cond := range instance.Status.Conditions {
			if cond.Type == netopv1alpha1.NetworkInterfaceReady && cond.Status == corev1.ConditionTrue {
				netIf = instance
				return true, nil
			}
		}

		return false, nil
	})

	return netIf, err
}

func (np *netOpNetworkProvider) goscCustomization(netIf *netopv1alpha1.NetworkInterface) *vimtypes.CustomizationAdapterMapping {
	var adapter *vimtypes.CustomizationIPSettings

	if len(netIf.Status.IPConfigs) == 0 {
		adapter = &vimtypes.CustomizationIPSettings{
			Ip: &vimtypes.CustomizationDhcpIpGenerator{},
		}
	} else {
		ipConfig := netIf.Status.IPConfigs[0]
		if ipConfig.IPFamily == netopv1alpha1.IPv4Protocol {
			adapter = &vimtypes.CustomizationIPSettings{
				Ip:         &vimtypes.CustomizationFixedIp{IpAddress: ipConfig.IP},
				SubnetMask: ipConfig.SubnetMask,
				Gateway:    []string{ipConfig.Gateway},
			}
		} else if ipConfig.IPFamily == netopv1alpha1.IPv6Protocol {
			subnetMask := net.ParseIP(ipConfig.SubnetMask)
			var ipMask net.IPMask = make([]byte, net.IPv6len)
			copy(ipMask, subnetMask)
			ones, _ := ipMask.Size()

			adapter = &vimtypes.CustomizationIPSettings{
				IpV6Spec: &vimtypes.CustomizationIPSettingsIpV6AddressSpec{
					Ip: []vimtypes.BaseCustomizationIpV6Generator{
						&vimtypes.CustomizationFixedIpV6{
							IpAddress:  ipConfig.IP,
							SubnetMask: int32(ones),
						},
					},
					Gateway: []string{ipConfig.Gateway},
				},
			}
		} else {
			adapter = &vimtypes.CustomizationIPSettings{}
		}
	}

	// Note that NetOP VDS doesn't current specify the MacAddress (we have VC generate it), so we
	// rely on the customization order matching the sorted bus order that GOSC does. This is quite
	// brittle, and something we're going to need to revisit. Assuming Reconfigure() generates the
	// MacAddress, we could later fix up the MacAddress, but interface matching is not straight
	// forward either (see reconcileVMNicDeviceChanges()).
	return &vimtypes.CustomizationAdapterMapping{
		MacAddress: netIf.Status.MacAddress,
		Adapter:    *adapter,
	}
}

func (np *netOpNetworkProvider) EnsureNetworkInterface(
	vmCtx VMContext,
	vif *vmopv1alpha1.VirtualMachineNetworkInterface) (*NetworkInterfaceInfo, error) {

	netIf, err := np.createNetworkInterface(vmCtx, vif)
	if err != nil {
		return nil, err
	}

	ethDev, err := np.createEthernetCard(vmCtx, vif, netIf)
	if err != nil {
		return nil, err
	}

	return &NetworkInterfaceInfo{
		Device:          ethDev,
		Customization:   np.goscCustomization(netIf),
		IPConfiguration: np.getIPConfig(netIf),
	}, nil
}

func (np *netOpNetworkProvider) getIPConfig(netIf *netopv1alpha1.NetworkInterface) IPConfig {
	var ipConfig IPConfig
	if len(netIf.Status.IPConfigs) > 0 {
		ipConfig = IPConfig{
			IP:         netIf.Status.IPConfigs[0].IP,
			Gateway:    netIf.Status.IPConfigs[0].Gateway,
			SubnetMask: netIf.Status.IPConfigs[0].SubnetMask,
			IPFamily:   IPFamily(netIf.Status.IPConfigs[0].IPFamily),
		}
	}

	return ipConfig
}

type nsxtNetworkProvider struct {
	k8sClient ctrlruntime.Client
	finder    *find.Finder
	cluster   *object.ClusterComputeResource
	scheme    *runtime.Scheme
}

// newNsxtNetworkProvider returns a nsxtNetworkProvider instance
func newNsxtNetworkProvider(
	client ctrlruntime.Client,
	finder *find.Finder,
	cluster *object.ClusterComputeResource,
	scheme *runtime.Scheme) *nsxtNetworkProvider {

	return &nsxtNetworkProvider{
		k8sClient: client,
		finder:    finder,
		cluster:   cluster,
		scheme:    scheme,
	}
}

// virtualNetworkInterfaceName returns the VirtualNetworkInterface name for the VM
func (np *nsxtNetworkProvider) virtualNetworkInterfaceName(networkName, vmName string) string {
	vnetifName := fmt.Sprintf("%s-lsp", vmName)
	if networkName != "" {
		vnetifName = fmt.Sprintf("%s-%s", networkName, vnetifName)
	}
	return vnetifName
}

// createVirtualNetworkInterface creates a NCP VirtualNetworkInterface for a given VM network interface
func (np *nsxtNetworkProvider) createVirtualNetworkInterface(
	vmCtx VMContext,
	vmIf *vmopv1alpha1.VirtualMachineNetworkInterface) (*ncpv1alpha1.VirtualNetworkInterface, error) {

	vnetIf := &ncpv1alpha1.VirtualNetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      np.virtualNetworkInterfaceName(vmIf.NetworkName, vmCtx.VM.Name),
			Namespace: vmCtx.VM.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(vmCtx, np.k8sClient, vnetIf, func() error {
		if err := controllerutil.SetOwnerReference(vmCtx.VM, vnetIf, np.scheme); err != nil {
			return err
		}

		vnetIf.Spec.VirtualNetwork = vmIf.NetworkName
		return nil
	})

	if err != nil {
		return nil, err
	}

	if result == controllerutil.OperationResultCreated {
		vmCtx.Logger.Info("Successfully created VirtualNetworkInterface",
			"name", types.NamespacedName{Namespace: vnetIf.Namespace, Name: vnetIf.Name})
	}

	return np.waitForReadyVirtualNetworkInterface(vmCtx, vmIf)
}

func (np *nsxtNetworkProvider) createEthernetCard(
	vmCtx VMContext,
	vif *vmopv1alpha1.VirtualMachineNetworkInterface,
	vnetIf *ncpv1alpha1.VirtualNetworkInterface) (vimtypes.BaseVirtualDevice, error) {

	if vnetIf.Status.ProviderStatus == nil || vnetIf.Status.ProviderStatus.NsxLogicalSwitchID == "" {
		err := fmt.Errorf("failed to get for nsx-t opaque network ID for vnetIf '%+v'", vnetIf)
		vmCtx.Logger.Error(err, "Ready VirtualNetworkInterface did not have expected Status.ProviderStatus")
		return nil, err
	}

	networkRef, err := searchNsxtNetworkReference(vmCtx, np.finder, np.cluster, vnetIf.Status.ProviderStatus.NsxLogicalSwitchID)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to search for nsx-t network associated with vnetIf", "vnetIf", vnetIf)
		return nil, err
	}

	ethDev, err := createEthernetCard(vmCtx, networkRef, vif.EthernetCardType)
	if err != nil {
		return nil, err
	}

	configureEthernetCard(ethDev, vnetIf.Status.InterfaceID, vnetIf.Status.MacAddress)

	return ethDev, nil
}

func (np *nsxtNetworkProvider) waitForReadyVirtualNetworkInterface(
	vmCtx VMContext,
	vmIf *vmopv1alpha1.VirtualMachineNetworkInterface) (*ncpv1alpha1.VirtualNetworkInterface, error) {

	vnetIfName := np.virtualNetworkInterfaceName(vmIf.NetworkName, vmCtx.VM.Name)

	var vnetIf *ncpv1alpha1.VirtualNetworkInterface
	vnetIfKey := types.NamespacedName{Namespace: vmCtx.VM.Namespace, Name: vnetIfName}

	// TODO: Watch() this type instead.
	err := wait.PollImmediate(retryInterval, retryTimeout, func() (bool, error) {
		instance := &ncpv1alpha1.VirtualNetworkInterface{}
		if err := np.k8sClient.Get(vmCtx, vnetIfKey, instance); err != nil {
			return false, err
		}

		for _, condition := range instance.Status.Conditions {
			if strings.Contains(condition.Type, "Ready") && strings.Contains(condition.Status, "True") {
				vnetIf = instance
				return true, nil
			}
		}

		return false, nil
	})

	return vnetIf, err
}

func (np *nsxtNetworkProvider) goscCustomization(vnetIf *ncpv1alpha1.VirtualNetworkInterface) *vimtypes.CustomizationAdapterMapping {
	var adapter *vimtypes.CustomizationIPSettings

	addrs := vnetIf.Status.IPAddresses
	if len(addrs) == 0 || (len(addrs) == 1 && addrs[0].IP == "") {
		adapter = &vimtypes.CustomizationIPSettings{
			Ip: &vimtypes.CustomizationDhcpIpGenerator{},
		}
	} else {
		ipAddr := addrs[0]
		adapter = &vimtypes.CustomizationIPSettings{
			Ip:         &vimtypes.CustomizationFixedIp{IpAddress: ipAddr.IP},
			SubnetMask: ipAddr.SubnetMask,
			Gateway:    []string{ipAddr.Gateway},
		}
	}

	return &vimtypes.CustomizationAdapterMapping{
		MacAddress: vnetIf.Status.MacAddress,
		Adapter:    *adapter,
	}
}

func (np *nsxtNetworkProvider) EnsureNetworkInterface(
	vmCtx VMContext,
	vif *vmopv1alpha1.VirtualMachineNetworkInterface) (*NetworkInterfaceInfo, error) {

	vnetIf, err := np.createVirtualNetworkInterface(vmCtx, vif)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to create vnetIf for vif", "vif", vif)
		return nil, err
	}

	ethDev, err := np.createEthernetCard(vmCtx, vif, vnetIf)
	if err != nil {
		return nil, err
	}

	return &NetworkInterfaceInfo{
		Device:          ethDev,
		Customization:   np.goscCustomization(vnetIf),
		IPConfiguration: np.getIPConfig(vnetIf),
	}, nil
}

func (np *nsxtNetworkProvider) getIPConfig(vnetIf *ncpv1alpha1.VirtualNetworkInterface) IPConfig {
	var ipConfig IPConfig
	if len(vnetIf.Status.IPAddresses) > 0 {
		ipAddr := vnetIf.Status.IPAddresses[0]
		ipConfig.IP = ipAddr.IP
		ipConfig.Gateway = ipAddr.Gateway
		ipConfig.SubnetMask = ipAddr.SubnetMask
		ipConfig.IPFamily = IPv4Protocol
	}

	return ipConfig
}

// matchOpaqueNetwork takes the network ID, returns whether the opaque network matches the networkID
func matchOpaqueNetwork(ctx context.Context, network object.NetworkReference, networkID string) bool {
	obj, ok := network.(*object.OpaqueNetwork)
	if !ok {
		return false
	}

	var opaqueNet mo.OpaqueNetwork
	if err := obj.Properties(ctx, obj.Reference(), []string{"summary"}, &opaqueNet); err != nil {
		return false
	}

	summary, _ := opaqueNet.Summary.(*vimtypes.OpaqueNetworkSummary)
	return summary.OpaqueNetworkId == networkID
}

// matchDistributedPortGroup takes the network ID, returns whether the distributed port group matches the networkID
func matchDistributedPortGroup(
	ctx context.Context,
	network object.NetworkReference,
	networkID string,
	hostMoIDs []vimtypes.ManagedObjectReference) bool {

	obj, ok := network.(*object.DistributedVirtualPortgroup)
	if !ok {
		return false
	}

	var configInfo []vimtypes.ObjectContent
	err := obj.Properties(ctx, obj.Reference(), []string{"config.logicalSwitchUuid", "host"}, &configInfo)
	if err != nil {
		return false
	}

	if len(configInfo) > 0 {
		// Check "logicalSwitchUuid" property
		lsIDMatch := false
		for _, dynamicProperty := range configInfo[0].PropSet {
			if dynamicProperty.Name == "config.logicalSwitchUuid" && dynamicProperty.Val == networkID {
				lsIDMatch = true
				break
			}
		}

		// logicalSwitchUuid did not match
		if !lsIDMatch {
			return false
		}

		foundAllHosts := false
		for _, dynamicProperty := range configInfo[0].PropSet {
			// In the case of a single NSX Overlay Transport Zone for all the clusters and DVS's,
			// multiple DVPGs(associated with different DVS's) will have the same "logicalSwitchUuid".
			// So matching "logicalSwitchUuid" is necessary condition, but not sufficient.
			// Checking if the DPVG has all the hosts in the cluster, along with the above would be sufficient
			if dynamicProperty.Name == "host" {
				if hosts, ok := dynamicProperty.Val.(vimtypes.ArrayOfManagedObjectReference); ok {
					foundAllHosts = true
					dvsHostSet := make(map[string]bool, len(hosts.ManagedObjectReference))
					for _, dvsHost := range hosts.ManagedObjectReference {
						dvsHostSet[dvsHost.Value] = true
					}

					for _, hostMoRef := range hostMoIDs {
						if _, ok := dvsHostSet[hostMoRef.Value]; !ok {
							foundAllHosts = false
							break
						}
					}
				}
			}
		}

		// Implicit that lsID Matches at this point
		return foundAllHosts
	}
	return false
}

// searchNsxtNetworkReference takes in nsx-t logical switch UUID and returns the reference of the network
func searchNsxtNetworkReference(ctx context.Context, finder *find.Finder, cluster *object.ClusterComputeResource, networkID string) (object.NetworkReference, error) {
	networks, err := finder.NetworkList(ctx, "*")
	if err != nil {
		return nil, err
	}

	// Get the list of ESX host moRef objects for this cluster
	// TODO: ps: []string{"host"} instead of nil?
	var computeResource mo.ComputeResource
	if err := cluster.Properties(ctx, cluster.Reference(), nil, &computeResource); err != nil {
		return nil, err
	}

	for _, network := range networks {
		if matchDistributedPortGroup(ctx, network, networkID, computeResource.Host) {
			return network, nil
		}
		if matchOpaqueNetwork(ctx, network, networkID) {
			return network, nil
		}
	}
	return nil, fmt.Errorf("opaque network with ID '%s' not found", networkID)
}
