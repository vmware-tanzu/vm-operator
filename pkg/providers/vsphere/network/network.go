// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

type NetworkInterfaceResults struct { //nolint:revive
	Results                   []NetworkInterfaceResult
	UpdatedEthCards           bool
	OrphanedNetworkInterfaces []ctrlclient.Object
}

type NetworkInterfaceResult struct { //nolint:revive
	// ObjectName is the name of the network interface CR backing this result.
	// Empty for named (testing-only) networks which have no CR.
	ObjectName string
	// ObjectProviderType is the network provider type of the CR. Combined with
	// ObjectName it uniquely identifies the CR across all network providers.
	ObjectProviderType pkgcfg.NetworkProviderType

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
	AcceptRA        bool
	MTU             int64
	Nameservers     []string
	SearchDomains   []string
	Routes          []NetworkInterfaceRoute
}

type NetworkInterfaceIPConfig struct { //nolint:revive
	IPCIDR  string // IP address in CIDR notation e.g. 192.168.10.42/24
	IsIPv4  bool
	Gateway string
}

type NetworkInterfaceRoute struct { //nolint:revive
	To     string
	Via    string
	Metric int32
}

// Device contains the information from the network interface CR needed to create or
// configure a virtual ethernet card device on a VM.
type Device struct {
	ProviderType pkgcfg.NetworkProviderType
	InterfaceObj ctrlclient.Object

	Backing    object.NetworkReference
	NetworkID  string
	MacAddress string
	ExternalID string
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

	// vpcIgnoreMacAddr is the empty MAC address that VPC returns that we will ignore.
	vpcIgnoreMacAddr = "00:00:00:00:00:00"
)

var (
	// RetryTimeout is var so tests can change it to shorten tests until we get rid of the poll.
	RetryTimeout = 15 * time.Second

	// ErrNetworkInterfaceTypeNotSupported is returned when the network interface specifies
	// type that is not supported by the configured network provider. The VM validation
	// webhook enforces what is supported so this is not an expected error.
	ErrNetworkInterfaceTypeNotSupported = errors.New("network provider is not supported")

	// ErrNetworkInterfaceNotReady is returned when the network interface is not ready.
	ErrNetworkInterfaceNotReady = pkgerr.NoRequeueErrorf("network interface is not ready")

	// ErrNetworkInterfaceBackingNotSupported is returned when the network interface specifies a
	// backing type that is not supported.
	ErrNetworkInterfaceBackingNotSupported = pkgerr.NoRequeueErrorf("network interface backing type not supported")
)

// CreateNetworkDevices ensures all network interface CRs exist for the VM's network
// spec, then checks whether every interface is ready. If all are ready it returns
// the corresponding Device values. If any are not ready it returns an error;
// the caller should rely on watches to be re-triggered when the CRs become ready.
func CreateNetworkDevices(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
) ([]Device, error) {

	networkSpec := vm.Spec.Network
	if networkSpec == nil || networkSpec.Disabled {
		return nil, nil
	}

	if !pkgcfg.FromContext(ctx).Features.PerNamespaceNetworkProvider {
		switch pkgcfg.FromContext(ctx).NetworkProviderType {
		case pkgcfg.NetworkProviderTypeNamed:
			// Named network is a testing only hack that does not have a CR so
			// handle it separately.
			return createNetworkDevicesForNamedNetwork(ctx, vm, finder)
		case "":
			return nil, fmt.Errorf("no network provider set")
		}
	}

	objs := make([]ctrlclient.Object, 0, len(networkSpec.Interfaces))
	var errs []error

	// Create all the network interface CRs. The VM may have multiple interfaces so we
	// want to create them all up front so they can be reconciled concurrently.
	for _, interfaceSpec := range networkSpec.Interfaces {
		group, err := getNetworkInterfaceAPIGroup(interfaceSpec)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("error getting network APIGroup for %s: %w", interfaceSpec.Name, err))
			continue
		}

		var obj ctrlclient.Object

		switch group {
		case netopv1alpha1.SchemeGroupVersion.Group:
			obj, err = createNetOPNetworkInterface(ctx, vm, client, interfaceSpec)
		case ncpv1alpha1.SchemeGroupVersion.Group:
			obj, err = createNCPNetworkInterface(ctx, vm, client, interfaceSpec)
		case vpcv1alpha1.SchemeGroupVersion.Group:
			obj, err = createVPCNetworkInterface(ctx, vm, client, interfaceSpec)
		default:
			err = fmt.Errorf("unsupported network API Group: %q", group)
		}

		if err != nil {
			// TODO: Update per-interface Status.
			errs = append(errs,
				fmt.Errorf("error creating interface %s: %w", interfaceSpec.Name, err))
			continue
		}

		objs = append(objs, obj)
	}

	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	devices := make([]Device, 0, len(objs))

	// For each interface, if its network interface CR is ready, get the information
	// needed from it for the device.
	for i, interfaceSpec := range networkSpec.Interfaces {
		obj := objs[i]

		var dev Device
		var err error

		switch ifaceCR := obj.(type) {
		case *netopv1alpha1.NetworkInterface:
			dev, err = getNetOPNetworkInterfaceDevice(ctx, vimClient, ifaceCR, nil, interfaceSpec)
		case *ncpv1alpha1.VirtualNetworkInterface:
			dev, err = getNCPNetworkInterfaceDevice(ctx, vimClient, nil, ifaceCR)
		case *vpcv1alpha1.SubnetPort:
			dev, err = getVPCSubnetPortDevice(ctx, vimClient, nil, ifaceCR)
		default:
			err = fmt.Errorf("unsupported network interface CR type: %T", obj)
		}

		if err != nil {
			// TODO: Update per-interface Status.
			errs = append(errs,
				fmt.Errorf("error getting device for interface %s: %w", interfaceSpec.Name, err))
			continue
		}

		devices = append(devices, dev)
	}

	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	return devices, nil
}

// getNetworkInterfaceAPIGroup returns the API Group name for this interface. This
// is used to determine which network interface CR type to create for the interface.
// The VM validation webhook enforces this group is support in this namespace.
func getNetworkInterfaceAPIGroup(
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (string, error) {

	network := interfaceSpec.Network
	if network == nil || network.APIVersion == "" {
		// The VM mutation webhook will default in the network type based
		// on the default network provider for this namespace.
		return "", pkgerr.NoRequeueErrorf("no network API Version")
	}

	gv, err := schema.ParseGroupVersion(network.APIVersion)
	if err != nil {
		// The VM validation webhook will check for invalid values.
		return "", pkgerr.NoRequeueErrorf("invalid API Version: %s", network.APIVersion)
	}

	return gv.Group, nil
}

func getNetOPNetworkInterfaceDevice(
	ctx context.Context,
	vimClient *vim25.Client,
	netIf *netopv1alpha1.NetworkInterface,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (Device, error) {

	var dev Device

	readyCond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceReady)
	if readyCond == nil || readyCond.Status != corev1.ConditionTrue {
		failCond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceFailure)
		if failCond != nil && failCond.Status == corev1.ConditionTrue {
			return dev, fmt.Errorf("%w: failure - %s: %s", ErrNetworkInterfaceNotReady,
				failCond.Reason, failCond.Message)
		}

		if readyCond != nil && readyCond.Status == corev1.ConditionFalse &&
			readyCond.Reason != "" && readyCond.Message != "" {
			return dev, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
				readyCond.Reason, readyCond.Message)
		}

		return dev, ErrNetworkInterfaceNotReady
	}

	networkID := netIf.Status.NetworkID
	backing, err := getNetworkInterfaceBacking(ctx, vimClient, clusterMoRef, networkID, true, false)
	if err != nil {
		return dev, err
	}

	// The NetworkInterface does not have a MAC address in its Spec, nor does NetOP
	// generate one in Status. If one was specified in the interface spec, set that
	// here so the device will be created with that address. This allows us to support
	// (unadvertised) user specified MAC on VDS.
	macAddress := netIf.Status.MacAddress
	if interfaceSpec.MACAddr != "" {
		macAddress = interfaceSpec.MACAddr
	}

	return Device{
		ProviderType: pkgcfg.NetworkProviderTypeVDS,
		InterfaceObj: netIf,
		Backing:      backing,
		NetworkID:    networkID,
		MacAddress:   macAddress,
		ExternalID:   netIf.Status.ExternalID,
	}, nil
}

func getNCPNetworkInterfaceDevice(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	vnetIf *ncpv1alpha1.VirtualNetworkInterface) (Device, error) {

	var dev Device

	var readyCond *ncpv1alpha1.VirtualNetworkCondition
	for _, cond := range vnetIf.Status.Conditions {
		// NOTE: For whatever reason, the contributed NCP code used Contains.
		if strings.Contains(cond.Type, "Ready") && strings.Contains(cond.Status, "True") {
			readyCond = &cond
		}
	}

	if readyCond == nil {
		for _, cond := range vnetIf.Status.Conditions {
			if strings.Contains(cond.Type, "Ready") && !strings.Contains(cond.Status, "True") {
				if cond.Reason != "" && cond.Message != "" {
					return dev, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
						cond.Reason, cond.Message)
				}

				return dev, ErrNetworkInterfaceNotReady
			}
		}

		return dev, ErrNetworkInterfaceNotReady
	}

	if vnetIf.Status.ProviderStatus == nil {
		return dev, pkgerr.NoRequeueNoErr("ready network interface does not have provider status")
	}

	networkID := vnetIf.Status.ProviderStatus.NsxLogicalSwitchID
	backing, err := getNetworkInterfaceBacking(ctx, vimClient, clusterMoRef, networkID, false, true)
	if err != nil {
		return dev, err
	}

	return Device{
		ProviderType: pkgcfg.NetworkProviderTypeNSXT,
		InterfaceObj: vnetIf,
		Backing:      backing,
		NetworkID:    networkID,
		MacAddress:   vnetIf.Status.MacAddress, // MAC from InterfaceSpec not supported
		ExternalID:   vnetIf.Status.InterfaceID,
	}, nil
}

func getVPCSubnetPortDevice(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	subnetPort *vpcv1alpha1.SubnetPort) (Device, error) {

	var dev Device

	var readyCond *vpcv1alpha1.Condition
	for _, cond := range subnetPort.Status.Conditions {
		if cond.Type == vpcv1alpha1.Ready {
			readyCond = &cond
			break
		}
	}

	if readyCond == nil {
		return dev, ErrNetworkInterfaceNotReady
	}

	if readyCond.Status != corev1.ConditionTrue {
		return dev, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
			readyCond.Reason, readyCond.Message)
	}

	networkID := subnetPort.Status.NetworkInterfaceConfig.LogicalSwitchUUID
	if networkID == "" {
		return dev, pkgerr.NoRequeueErrorf("ready network interface does not have LogicalSwitchUUID")
	}

	backing, err := getNetworkInterfaceBacking(ctx, vimClient, clusterMoRef, networkID, false, true)
	if err != nil {
		return dev, err
	}

	// A MAC address in the InterfaceSpec will have been set in the SubnetPort
	// Spec, so if set we expect it to be reflected in the Status.
	macAddress := subnetPort.Status.NetworkInterfaceConfig.MACAddress
	if macAddress == vpcIgnoreMacAddr {
		// Ignore an all zeros MAC if VPC goes kooky.
		macAddress = ""
	}

	return Device{
		ProviderType: pkgcfg.NetworkProviderTypeVPC,
		InterfaceObj: subnetPort,
		Backing:      backing,
		NetworkID:    networkID,
		MacAddress:   macAddress,
		ExternalID:   subnetPort.Status.Attachment.ID,
	}, nil
}

func getNetworkInterfaceBacking(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	networkID string,
	supportMoID, supportLSUUID bool) (object.NetworkReference, error) {

	// Determine if this is a MoID of a backing type we support.
	if supportMoID {
		switch {
		case strings.HasPrefix(networkID, "dvportgroup-"):
			return object.NewDistributedVirtualPortgroup(
				vimClient,
				vimtypes.ManagedObjectReference{
					Type:  string(vimtypes.ManagedObjectTypeDistributedVirtualPortgroup),
					Value: networkID,
				},
			), nil

		case strings.HasPrefix(networkID, "network-"):
			if !pkgcfg.FromContext(ctx).Features.PerNamespaceNetworkProvider {
				return nil, ErrNetworkInterfaceBackingNotSupported
			}

			return object.NewNetwork(
				vimClient,
				vimtypes.ManagedObjectReference{
					Type:  string(vimtypes.ManagedObjectTypeNetwork),
					Value: networkID,
				},
			), nil
		}
	}

	// Determine if this is a UUID for a LogicalSwitchUUID.
	if supportLSUUID {
		if _, err := uuid.Parse(networkID); err == nil {
			// For a stretched SV setup, each CCR may have a different DVPG.
			// For placement and create, we use the OpaqueBacking. Otherwise,
			// resolve the DVPG for the VM's CCR.
			var backing object.NetworkReference
			if clusterMoRef != nil {
				ccr := object.NewClusterComputeResource(vimClient, *clusterMoRef)
				backing, err = searchNsxtNetworkReference(ctx, ccr, networkID)
			} else {
				backing = newNSXOpaqueNetwork(networkID)
			}

			return backing, err
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrNetworkInterfaceBackingNotSupported, networkID)
}

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

	if !pkgcfg.FromContext(vmCtx).Features.PerNamespaceNetworkProvider {
		switch pkgcfg.FromContext(vmCtx).NetworkProviderType {
		case pkgcfg.NetworkProviderTypeNamed:
			// Named network is a testing only hack that does not have a CR so
			// handle it separately.
			return createAndWaitNamedNetworkInterfaces(vmCtx, vmCtx.VM, finder)
		case "":
			return NetworkInterfaceResults{}, fmt.Errorf("no network provider set")
		}
	}

	results := make([]NetworkInterfaceResult, 0, len(networkSpec.Interfaces))
	var errs []error

	for _, interfaceSpec := range networkSpec.Interfaces {
		group, err := getNetworkInterfaceAPIGroup(interfaceSpec)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("error getting network APIGroup for %s: %w", interfaceSpec.Name, err))
			continue
		}

		var dev Device
		var bs Bootstrap

		switch group {
		case netopv1alpha1.SchemeGroupVersion.Group:
			dev, bs, err = createAndWaitNetOPNetworkInterface(vmCtx, client, vimClient, clusterMoRef, interfaceSpec)
		case ncpv1alpha1.SchemeGroupVersion.Group:
			dev, bs, err = createAndWaitNCPNetworkInterface(vmCtx, client, vimClient, clusterMoRef, interfaceSpec)
		case vpcv1alpha1.SchemeGroupVersion.Group:
			dev, bs, err = createAndWaitVPCNetworkInterface(vmCtx, client, vimClient, clusterMoRef, interfaceSpec)
		default:
			err = fmt.Errorf("unsupported network API Group: %q", group)
		}

		if err != nil {
			errs = append(errs,
				fmt.Errorf("network interface %q error: %w", interfaceSpec.Name, err))
			continue
		}

		results = append(results, devAndBootstrapToNetworkInterfaceResult(dev, bs))
	}

	if len(errs) != 0 {
		return NetworkInterfaceResults{}, errors.Join(errs...)
	}

	return NetworkInterfaceResults{
		Results: results,
	}, nil
}

func createNetworkDevicesForNamedNetwork(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	finder *find.Finder) ([]Device, error) {

	devices := make([]Device, 0, len(vm.Spec.Network.Interfaces))
	for _, interfaceSpec := range vm.Spec.Network.Interfaces {
		dev, _, err := createAndWaitNamedNetworkInterface(ctx, vm, finder, interfaceSpec)
		if err != nil {
			return nil, err
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

func createAndWaitNamedNetworkInterfaces(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	finder *find.Finder) (NetworkInterfaceResults, error) {

	results := NetworkInterfaceResults{}

	for _, interfaceSpec := range vm.Spec.Network.Interfaces {
		dev, bs, err := createAndWaitNamedNetworkInterface(ctx, vm, finder, interfaceSpec)
		if err != nil {
			return NetworkInterfaceResults{},
				fmt.Errorf("named network interface %q error: %w", interfaceSpec.Name, err)
		}
		results.Results = append(results.Results, devAndBootstrapToNetworkInterfaceResult(dev, bs))
	}

	return results, nil
}

func createAndWaitNamedNetworkInterface(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	finder *find.Finder,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (Device, Bootstrap, error) {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	if networkRefType.Kind != "" || networkRefType.APIVersion != "" {
		return Device{}, Bootstrap{}, fmt.Errorf("network TypeMeta not supported for name network: %v", networkRefType)
	}

	if networkRefName == "" {
		return Device{}, Bootstrap{}, fmt.Errorf("network name is required")
	}

	backing, err := finder.Network(ctx, networkRefName)
	if err != nil {
		return Device{}, Bootstrap{}, fmt.Errorf("unable to find named network %q: %w", networkRefName, err)
	}

	dev := Device{
		InterfaceObj: nil,
		Backing:      backing,
		NetworkID:    networkRefName,
		MacAddress:   interfaceSpec.MACAddr,
		ExternalID:   "",
	}

	bootstrap := InterfaceBootstrap(
		ctx,
		vm,
		Bootstrap{MacAddress: dev.MacAddress},
		interfaceSpec)

	return dev, bootstrap, nil
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
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (
	*netopv1alpha1.NetworkInterface, error) {

	var networkName string
	if netRef := interfaceSpec.Network; netRef != nil {
		// If unset, NetOP will select the NS default.
		networkName = netRef.Name
	}

	netIf := &netopv1alpha1.NetworkInterface{}
	netIfKey := types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      NetOPCRName(vm.Name, networkName, interfaceSpec.Name, true),
	}

	// Check if an object exists with the older (v1a1) naming convention.
	if err := client.Get(ctx, netIfKey, netIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// If NotFound set the netIf to the new v1a2 naming convention.
		netIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NetOPCRName(vm.Name, networkName, interfaceSpec.Name, false),
			Namespace: vm.Namespace,
		}
	}

	_, err := controllerutil.CreateOrPatch(ctx, client, netIf, func() error {
		if err := SetNetworkInterfaceOwnerRef(vm, netIf, client.Scheme()); err != nil {
			return err
		}

		if netIf.Labels == nil {
			netIf.Labels = map[string]string{}
		}
		netIf.Labels[VMNameLabel] = vm.Name
		netIf.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		// NetOp will update the Spec with the default network name so we don't clear that
		// here if using the default network.
		if networkName != "" {
			netIf.Spec.NetworkName = networkName
		}

		// NetOP only defines a VMXNet3 type, but it doesn't really matter for our purposes.
		netIf.Spec.Type = netopv1alpha1.NetworkInterfaceTypeVMXNet3

		// IPFamilyPolicy is a new field introduced for IPv6 support.
		// Only set it when the WorkloadIPv6 capability is active so that this
		// field is not sent to NetOP when IPv6 is disabled.
		if pkgcfg.FromContext(ctx).Features.WorkloadIPv6 {
			netIf.Spec.IPFamilyPolicy = IPAMModesToNetOPInterfaceIPFamilyPolicy(interfaceSpec.IPAMModes)
		}

		return nil
	})

	return netIf, err
}

func createAndWaitNetOPNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
) (Device, Bootstrap, error) {

	netIf, err := createNetOPNetworkInterface(vmCtx, vmCtx.VM, client, interfaceSpec)
	if err != nil {
		return Device{}, Bootstrap{}, err
	}

	dev, err := getNetOPNetworkInterfaceDevice(vmCtx, vimClient, netIf, clusterMoRef, interfaceSpec)
	if err != nil {
		if errors.Is(err, ErrNetworkInterfaceNotReady) {
			netIf, err = waitForReadyNetworkInterface(vmCtx, client, netIf.Name)
		}
		if err != nil {
			return Device{}, Bootstrap{}, err
		}

		dev, err = getNetOPNetworkInterfaceDevice(vmCtx, vimClient, netIf, clusterMoRef, interfaceSpec)
		if err != nil {
			return Device{}, Bootstrap{}, err
		}
	}

	bootstrap := NetOPInterfaceBootstrap(
		vmCtx,
		vmCtx.VM,
		netIf,
		interfaceSpec,
		dev.MacAddress)

	return dev, bootstrap, nil
}

// EffectiveNetOPIPv4AssignmentMode returns how IPv4 is assigned according to NetworkInterface status.
// When IPAssignmentMode is unset, NetOP assumes static pool if any IPv4 address is present in IPConfigs,
// otherwise DHCP.
func EffectiveNetOPIPv4AssignmentMode(st netopv1alpha1.NetworkInterfaceStatus) netopv1alpha1.NetworkInterfaceIPAssignmentMode {
	if st.IPAssignmentMode != "" {
		return st.IPAssignmentMode
	}
	for i := range st.IPConfigs {
		ip := &st.IPConfigs[i]
		if ip.IPFamily == corev1.IPv4Protocol && ip.IP != "" {
			return netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
		}
	}
	return netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
}

// EffectiveNetOPIPv6AssignmentMode returns how IPv6 is assigned according to NetworkInterface status.
// When IPv6AssignmentMode is unset, NetOP defaults to none (no IPv6 assignment).
func EffectiveNetOPIPv6AssignmentMode(st netopv1alpha1.NetworkInterfaceStatus) netopv1alpha1.NetworkInterfaceIPAssignmentMode {
	if st.IPv6AssignmentMode != "" {
		return st.IPv6AssignmentMode
	}
	return netopv1alpha1.NetworkInterfaceIPAssignmentModeNone
}

// IPAMModesToNetOPInterfaceIPFamilyPolicy maps spec IPAMModes (Kubernetes IP families) to
// NetOP NetworkInterfaceIPFamilyPolicy. When both families are present it returns DualStack.
func IPAMModesToNetOPInterfaceIPFamilyPolicy(ipamModes []corev1.IPFamily) netopv1alpha1.NetworkInterfaceIPFamilyPolicy {
	hasV4, hasV6 := false, false
	for _, f := range ipamModes {
		switch f {
		case corev1.IPv4Protocol:
			hasV4 = true
		case corev1.IPv6Protocol:
			hasV6 = true
		}
	}

	switch {
	case hasV4 && hasV6:
		return netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack
	case hasV6:
		return netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only
	case hasV4:
		return netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only
	default:
		// No modes so use NetOP default.
		return ""
	}
}

// IPAMModesToVPCInterfaceIPType converts IPAMModes to the VPC SubnetPort
// InterfaceIPType, which tells NSX Operator which IP address families to
// activate on the port.
func IPAMModesToVPCInterfaceIPType(ipamModes []corev1.IPFamily) vpcv1alpha1.IPAddressType {
	hasV4 := slices.Contains(ipamModes, corev1.IPv4Protocol)
	hasV6 := slices.Contains(ipamModes, corev1.IPv6Protocol)
	switch {
	case hasV4 && hasV6:
		return vpcv1alpha1.IPAddressTypeIPv4IPv6
	case hasV6:
		return vpcv1alpha1.IPAddressTypeIPv6
	case hasV4:
		return vpcv1alpha1.IPAddressTypeIPv4
	default:
		return ""
	}
}

// DeriveStaticIPAllocationType determines which IP families must use NSX's
// strict static IP pool. Only an explicit DHCP4=false or DHCP6=false triggers
// this: the user is saying "I want this family but NOT via DHCP" — NSX must
// allocate from the static pool and will fail if the subnet is not configured
// for static allocation.
//
// interfaceSpec.Addresses is communicated to NSX via AddressBindings (set
// separately); StaticIPAllocationType is not needed for the explicit-IP case.
func DeriveStaticIPAllocationType(
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
) vpcv1alpha1.StaticIPAllocationType {

	hasIPv4Mode := slices.Contains(interfaceSpec.IPAMModes, corev1.IPv4Protocol)
	hasIPv6Mode := slices.Contains(interfaceSpec.IPAMModes, corev1.IPv6Protocol)

	wantStaticV4 := hasIPv4Mode && interfaceSpec.DHCP4 != nil && !*interfaceSpec.DHCP4
	wantStaticV6 := hasIPv6Mode && interfaceSpec.DHCP6 != nil && !*interfaceSpec.DHCP6

	switch {
	case wantStaticV4 && wantStaticV6:
		return vpcv1alpha1.StaticIPAllocationTypeIPv4IPv6
	case wantStaticV6:
		return vpcv1alpha1.StaticIPAllocationTypeIPv6
	case wantStaticV4:
		return vpcv1alpha1.StaticIPAllocationTypeIPv4
	default:
		return ""
	}
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
				return nil, fmt.Errorf("%w: failure - %s: %s", ErrNetworkInterfaceNotReady,
					cond.Reason, cond.Message)
			}
			if cond := findNetOPCondition(netIf, netopv1alpha1.NetworkInterfaceReady); cond != nil && cond.Status == corev1.ConditionFalse {
				return nil, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
					cond.Reason, cond.Message)
			}
			return nil, ErrNetworkInterfaceNotReady
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

// createNCPNetworkInterface creates or patches the NCP VirtualNetworkInterface CR for the
// given interface spec. It does not wait for the CR to become ready.
func createNCPNetworkInterface(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (*ncpv1alpha1.VirtualNetworkInterface, error) {

	var networkName string
	if netRef := interfaceSpec.Network; netRef != nil {
		networkName = netRef.Name
	}

	vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
	vnetIfKey := types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      NCPCRName(vm.Name, networkName, interfaceSpec.Name, true),
	}

	// Check if a VirtualNetworkInterface exists with the older (v1a1) naming convention.
	if err := client.Get(ctx, vnetIfKey, vnetIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// If NotFound set the vnetIf to the new v1a2 naming convention.
		vnetIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NCPCRName(vm.Name, networkName, interfaceSpec.Name, false),
			Namespace: vm.Namespace,
		}
	}

	_, err := controllerutil.CreateOrPatch(ctx, client, vnetIf, func() error {
		if err := SetNetworkInterfaceOwnerRef(vm, vnetIf, client.Scheme()); err != nil {
			return err
		}

		if vnetIf.Labels == nil {
			vnetIf.Labels = map[string]string{}
		}
		vnetIf.Labels[VMNameLabel] = vm.Name
		vnetIf.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		vnetIf.Spec.VirtualNetwork = networkName
		return nil
	})

	return vnetIf, err
}

func createAndWaitNCPNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
) (Device, Bootstrap, error) {

	vnetIf, err := createNCPNetworkInterface(vmCtx, vmCtx.VM, client, interfaceSpec)
	if err != nil {
		return Device{}, Bootstrap{}, err
	}

	dev, err := getNCPNetworkInterfaceDevice(vmCtx, vimClient, clusterMoRef, vnetIf)
	if err != nil {
		if errors.Is(err, ErrNetworkInterfaceNotReady) {
			vnetIf, err = waitForReadyNCPNetworkInterface(vmCtx, client, vnetIf.Name)
		}
		if err != nil {
			return Device{}, Bootstrap{}, err
		}

		dev, err = getNCPNetworkInterfaceDevice(vmCtx, vimClient, clusterMoRef, vnetIf)
		if err != nil {
			return Device{}, Bootstrap{}, err
		}
	}

	bootstrap := NCPInterfaceBootstrap(
		vmCtx,
		vmCtx.VM,
		vnetIf,
		interfaceSpec,
		dev.MacAddress)

	return dev, bootstrap, nil
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
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec) (*vpcv1alpha1.SubnetPort, error) {

	var (
		networkName string
		networkType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		networkName = netRef.Name
		networkType = netRef.TypeMeta
	}

	subnetPort := &vpcv1alpha1.SubnetPort{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VPCCRName(vm.Name, networkName, interfaceSpec.Name),
			Namespace: vm.Namespace,
		},
	}

	switch networkType.Kind {
	case "SubnetSet", "":
		subnetPort.Spec.SubnetSet = networkName
	case "Subnet":
		subnetPort.Spec.Subnet = networkName
	default:
		return nil, pkgerr.NoRequeueErrorf("network kind %q is not supported for VPC", networkType.Kind)
	}

	_, err := controllerutil.CreateOrPatch(ctx, client, subnetPort, func() error {
		if err := SetNetworkInterfaceOwnerRef(vm, subnetPort, client.Scheme()); err != nil {
			return err
		}

		if subnetPort.Labels == nil {
			subnetPort.Labels = map[string]string{}
		}
		subnetPort.Labels[VMNameLabel] = vm.Name
		subnetPort.Labels[VMInterfaceNameLabel] = interfaceSpec.Name

		if subnetPort.Annotations == nil {
			subnetPort.Annotations = make(map[string]string)
		}
		subnetPort.Annotations[pkgconst.VPCAttachmentRef] = "virtualmachine/" + vm.Name + "/" + interfaceSpec.Name

		subnetPort.Spec.AddressBindings = nil

		switch {
		case len(interfaceSpec.Addresses) > 0:
			for _, ipCidr := range interfaceSpec.Addresses {
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
					continue
				}

				// Despite being a list, VPC currently only supports just one PortAddressBinding.
				subnetPort.Spec.AddressBindings = []vpcv1alpha1.PortAddressBinding{
					{
						IPAddress:  ip.String(),
						MACAddress: strings.ToLower(interfaceSpec.MACAddr),
					},
				}
				break
			}
		case interfaceSpec.MACAddr != "":
			subnetPort.Spec.AddressBindings = []vpcv1alpha1.PortAddressBinding{
				{
					MACAddress: strings.ToLower(interfaceSpec.MACAddr),
				},
			}
		}

		if pkgcfg.FromContext(ctx).Features.WorkloadIPv6 {
			subnetPort.Spec.InterfaceIPType = IPAMModesToVPCInterfaceIPType(interfaceSpec.IPAMModes)
			subnetPort.Spec.StaticIPAllocationType = DeriveStaticIPAllocationType(interfaceSpec)
		}

		return nil
	})

	return subnetPort, err
}

func createAndWaitVPCNetworkInterface(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	clusterMoRef *vimtypes.ManagedObjectReference,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
) (Device, Bootstrap, error) {

	subnetPort, err := createVPCNetworkInterface(vmCtx, vmCtx.VM, client, interfaceSpec)
	if err != nil {
		return Device{}, Bootstrap{}, err
	}

	dev, err := getVPCSubnetPortDevice(vmCtx, vimClient, clusterMoRef, subnetPort)
	if err != nil {
		if errors.Is(err, ErrNetworkInterfaceNotReady) {
			subnetPort, err = waitForReadyVPCSubnetPort(vmCtx, client, subnetPort.Name)
		}
		if err != nil {
			return Device{}, Bootstrap{}, err
		}

		dev, err = getVPCSubnetPortDevice(vmCtx, vimClient, clusterMoRef, subnetPort)
		if err != nil {
			return Device{}, Bootstrap{}, err
		}
	}

	bootstrap := VPCInterfaceBootstrap(
		vmCtx,
		vmCtx.VM,
		subnetPort,
		interfaceSpec,
		dev.MacAddress)

	return dev, bootstrap, nil
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
					return nil, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
						cond.Reason, cond.Message)
				}
			}
			return nil, ErrNetworkInterfaceNotReady
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
					return nil, fmt.Errorf("%w: %s - %s", ErrNetworkInterfaceNotReady,
						cond.Reason, cond.Message)
				}
			}
			// TODO: NCP also has an annotation but that usually doesn't provide very useful details.
			return nil, ErrNetworkInterfaceNotReady
		}

		return nil, err
	}

	if vnetIf.Status.ProviderStatus == nil {
		return nil, pkgerr.NoRequeueNoErr("ready network interface does not have provider status")
	}

	return vnetIf, nil
}

// ipCIDRFromNetOPIPConfig builds the CIDR string for a NetOP IPConfig, preferring
// the Prefix field over the deprecated SubnetMask field.
func ipCIDRFromNetOPIPConfig(ip netopv1alpha1.IPConfig) string {
	isIPv4 := ip.IPFamily == corev1.IPv4Protocol
	mask := ip.SubnetMask
	// Prefix is an integer (e.g. 24, 64) while ipCIDRNotation expects a subnet mask
	// string (e.g. "255.255.255.0", "ffff:ffff:ffff:ffff::"). Convert via net.CIDRMask
	// so we can reuse ipCIDRNotation for both paths.
	if ip.Prefix != nil {
		bits := 32
		if !isIPv4 {
			bits = 128
		}
		mask = net.IP(net.CIDRMask(int(*ip.Prefix), bits)).String()
	}
	return ipCIDRNotation(ip.IP, mask, isIPv4)
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

	return createDefaultEthCardFromDevice(ctx, result.Backing, result.MacAddress, result.ExternalID)
}

// CreateDefaultEthCardFromNetworkDevice creates a default Ethernet card from a Device.
// This is used during VM create when the VM Class ConfigSpec does not have a device entry for
// a VM Spec network interface.
func CreateDefaultEthCardFromNetworkDevice(
	ctx context.Context,
	dev *Device) (vimtypes.BaseVirtualDevice, error) {

	return createDefaultEthCardFromDevice(ctx, dev.Backing, dev.MacAddress, dev.ExternalID)
}

func createDefaultEthCardFromDevice(
	ctx context.Context,
	backing object.NetworkReference,
	macAddress string,
	externalID string) (vimtypes.BaseVirtualDevice, error) {

	cardBacking, err := backing.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get ethernet card backing info for network %v: %w", backing.Reference(), err)
	}

	dev, err := object.EthernetCardTypes().CreateEthernetCard(defaultEthernetCardType, cardBacking)
	if err != nil {
		return nil, fmt.Errorf("unable to create ethernet card network %v: %w", backing.Reference(), err)
	}

	ethCard := dev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	ethCard.ExternalId = externalID
	if macAddress != "" {
		ethCard.MacAddress = macAddress
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

	return applyNetworkDeviceToEthCard(ctx, ethCard, result.Backing, result.NetworkID, result.MacAddress, result.ExternalID)
}

// ApplyNetworkDeviceToVirtualEthCard applies a Device to an existing Ethernet device
// from the class ConfigSpec. This is used during VM create.
func ApplyNetworkDeviceToVirtualEthCard(
	ctx context.Context,
	ethCard *vimtypes.VirtualEthernetCard,
	dev *Device) error {

	return applyNetworkDeviceToEthCard(ctx, ethCard, dev.Backing, "", dev.MacAddress, dev.ExternalID)
}

func applyNetworkDeviceToEthCard(
	ctx context.Context,
	ethCard *vimtypes.VirtualEthernetCard,
	backing object.NetworkReference,
	networkID string,
	macAddress string,
	externalID string) error {

	cardBacking, err := backing.EthernetCardBackingInfo(ctx)
	if err != nil {
		errRef := networkID
		if errRef == "" {
			errRef = backing.Reference().Value
		}
		return fmt.Errorf("unable to get ethernet card backing info for network %v: %w", errRef, err)
	}
	ethCard.Backing = cardBacking

	ethCard.ExternalId = externalID
	if macAddress != "" {
		ethCard.MacAddress = macAddress
		ethCard.AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
	} else { //nolint:staticcheck,revive
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
