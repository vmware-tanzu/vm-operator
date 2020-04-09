// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

const (
	DefaultEthernetCardType = "vmxnet3"
	NsxtNetworkType         = "nsx-t"
	retryInterval           = 100 * time.Millisecond
	retryTimeout            = 15 * time.Second
)

// NetworkProvider sets up network for different type of network
type NetworkProvider interface {
	// CreateVnic creates the VirtualEthernetCard for the network interface
	CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (vimtypes.BaseVirtualDevice, error)
}

func getEthCardType(vif *v1alpha1.VirtualMachineNetworkInterface) string {
	ethCardType := vif.EthernetCardType
	if vif.EthernetCardType == "" {
		ethCardType = DefaultEthernetCardType
	}
	return ethCardType
}

// createVnicOnNamedNetwork creates vnic on named network
func createVnicOnNamedNetwork(ctx context.Context, networkName, ethCardType string, finder *find.Finder) (vimtypes.BaseVirtualDevice, error) {
	networkRef, err := finder.Network(ctx, networkName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find network %q", networkName)
	}
	return createCommonVnic(ctx, networkRef, ethCardType, finder)
}

// createCommonVnic creates vnid on a network given the network reference
func createCommonVnic(ctx context.Context, network object.NetworkReference, ethCardType string, finder *find.Finder) (vimtypes.BaseVirtualDevice, error) {
	backing, err := network.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create new ethernet card backing info for network %v", network)
	}
	dev, err := object.EthernetCardTypes().CreateEthernetCard(ethCardType, backing)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create new ethernet card %q for network %v", ethCardType, network)
	}
	return dev, nil
}

func setVnicKey(dev vimtypes.BaseVirtualDevice, key int32) vimtypes.BaseVirtualDevice {
	nic := dev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	nic.Key = key
	return dev
}

type defaultNetworkProvider struct {
	finder *find.Finder
}

// DefaultNetworkProvider returns a defaultNetworkProvider instance
func DefaultNetworkProvider(finder *find.Finder) *defaultNetworkProvider {
	return &defaultNetworkProvider{
		finder: finder,
	}
}

func (np *defaultNetworkProvider) CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (vimtypes.BaseVirtualDevice, error) {
	return createVnicOnNamedNetwork(ctx, vif.NetworkName, getEthCardType(vif), np.finder)
}

type nsxtNetworkProvider struct {
	finder  *find.Finder
	client  clientset.Interface
	cluster *object.ClusterComputeResource
}

// NsxtNetworkProvider returns a defaultNetworkProvider instance
func NsxtNetworkProvider(finder *find.Finder, client clientset.Interface, cluster *object.ClusterComputeResource) *nsxtNetworkProvider {
	return &nsxtNetworkProvider{
		finder:  finder,
		client:  client,
		cluster: cluster,
	}
}

// NetworkProviderByType returns the network provider based on network type
func NetworkProviderByType(networkType string, finder *find.Finder, client clientset.Interface, cluster *object.ClusterComputeResource) (NetworkProvider, error) {
	switch networkType {
	case NsxtNetworkType:
		return NsxtNetworkProvider(finder, client, cluster), nil
	case "":
		return DefaultNetworkProvider(finder), nil
	}
	return nil, fmt.Errorf("failed to create network provider for network type '%s'", networkType)
}

// GenerateNsxVnetifName generates the vnetif name for the VM
func (np *nsxtNetworkProvider) GenerateNsxVnetifName(networkName, vmName string) string {
	return fmt.Sprintf("%s-%s-lsp", networkName, vmName)
}

// matchOpaqueNetwork takes the network ID, returns whether the opaque network matches the networkID
func (np *nsxtNetworkProvider) matchOpaqueNetwork(ctx context.Context, network object.NetworkReference, networkID string) bool {
	obj, ok := network.(*object.OpaqueNetwork)
	if !ok {
		return false
	}

	var net mo.OpaqueNetwork

	if err := obj.Properties(ctx, obj.Reference(), []string{"summary"}, &net); err != nil {
		return false
	}

	summary, _ := net.Summary.(*vimtypes.OpaqueNetworkSummary)
	return summary.OpaqueNetworkId == networkID
}

// matchDistributedPortGroup takes the network ID, returns whether the distributed port group matches the networkID
func (np *nsxtNetworkProvider) matchDistributedPortGroup(ctx context.Context, network object.NetworkReference, networkID string) bool {
	obj, ok := network.(*object.DistributedVirtualPortgroup)
	if !ok {
		return false
	}

	hostMoIDs, err := getClusterHostMoIDs(ctx, np.cluster)
	if err != nil {
		log.Error(err, "Unable to get the list of hosts for cluster")
		return false
	}

	var configInfo []vimtypes.ObjectContent

	err = obj.Properties(ctx, obj.Reference(), []string{"config.logicalSwitchUuid", "host"}, &configInfo)
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

// searchNetworkReference takes in nsx-t logical switch UUID and returns the reference of the network
func (np *nsxtNetworkProvider) searchNetworkReference(ctx context.Context, networkID string) (object.NetworkReference, error) {
	networks, err := np.finder.NetworkList(ctx, "*")
	if err != nil {
		return nil, err
	}
	for _, network := range networks {
		if np.matchDistributedPortGroup(ctx, network, networkID) {
			return network, nil
		}
		if np.matchOpaqueNetwork(ctx, network, networkID) {
			return network, nil
		}
	}
	return nil, fmt.Errorf("opaque network with ID '%s' not found", networkID)
}

// setVnetifOwner sets owner reference for vnetif object
func (np *nsxtNetworkProvider) setVnetifOwner(vm *v1alpha1.VirtualMachine, vnetif *ncpv1alpha1.VirtualNetworkInterface) {
	owner := []metav1.OwnerReference{
		{
			Name:       vm.Name,
			APIVersion: vm.APIVersion,
			Kind:       vm.Kind,
			UID:        vm.UID,
		},
	}
	vnetif.SetOwnerReferences(owner)
}

// ownerMatch checks the owner reference for the vnetif object
// owner of VirtualMachine type should be object vm
func (np *nsxtNetworkProvider) ownerMatch(vm *v1alpha1.VirtualMachine, vnetif *ncpv1alpha1.VirtualNetworkInterface) bool {
	match := false
	for _, owner := range vnetif.GetOwnerReferences() {
		if owner.Kind != vm.Kind {
			continue
		}
		// TODO: shouldn't we check that owner.UID/vm.UID are not empty?
		if owner.UID != vm.UID {
			return false
		} else {
			match = true
		}
	}
	return match
}

// createVirtualNetworkInterface creates a NCP vnetif for a given VM network interface
func (np *nsxtNetworkProvider) createVirtualNetworkInterface(ctx context.Context, vm *v1alpha1.VirtualMachine, vmif *v1alpha1.VirtualMachineNetworkInterface) (*ncpv1alpha1.VirtualNetworkInterface, error) {
	if vm == nil {
		return nil, errors.Errorf("Virtual machine can not be nil when creating vnetif")
	}
	if vmif == nil {
		return nil, errors.Errorf("Virtual machine network interface can not be nil when creating vnetif")
	}

	// Create vnetif object
	vnetifName := np.GenerateNsxVnetifName(vmif.NetworkName, vm.Name)
	vnetif := &ncpv1alpha1.VirtualNetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vnetifName,
			Namespace: vm.Namespace,
		},
		Spec: ncpv1alpha1.VirtualNetworkInterfaceSpec{
			VirtualNetwork: vmif.NetworkName,
		},
	}
	np.setVnetifOwner(vm, vnetif)

	_, err := np.client.VmwareV1alpha1().VirtualNetworkInterfaces(vm.Namespace).Create(vnetif)
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return nil, err
		}
		// Update owner reference if vnetif already exist
		currentVnetif, err := np.client.VmwareV1alpha1().VirtualNetworkInterfaces(vm.Namespace).Get(vnetifName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if !np.ownerMatch(vm, vnetif) { // shouldn't that be currentVnetif here?
			copiedVnetif := currentVnetif.DeepCopy()
			np.setVnetifOwner(vm, copiedVnetif)
			_, err = np.client.VmwareV1alpha1().VirtualNetworkInterfaces(vm.Namespace).Update(copiedVnetif)
			if err != nil {
				return nil, err
			}
		}
	} else {
		log.Info("Successfully created a VirtualNetworkInterface",
			"VirtualMachine", types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name},
			"VirtualNetworkInterface", types.NamespacedName{Namespace: vm.Namespace, Name: vnetif.Name})
	}

	// Wait until the vnetif status is available
	// TODO: Rather than synchronously block here, place a watch on the VirtualNetworkInterface
	result, err := np.waitForVnetIFStatus(vm.Namespace, vmif.NetworkName, vm.Name)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// vnetifIsReady checks the readiness of vnetif object
func (np *nsxtNetworkProvider) vnetifIsReady(vnetif *ncpv1alpha1.VirtualNetworkInterface) bool {
	if vnetif.Status.Conditions == nil {
		return false
	}
	for _, condition := range vnetif.Status.Conditions {
		if !strings.Contains(condition.Type, "Ready") {
			continue
		}
		return strings.Contains(condition.Status, "True")
	}
	return false
}

// waitForVnetIFStatus will poll until the vnetif object's status becomes ready
func (np *nsxtNetworkProvider) waitForVnetIFStatus(namespace, networkName, vmName string) (*ncpv1alpha1.VirtualNetworkInterface, error) {
	vnetifName := np.GenerateNsxVnetifName(networkName, vmName)

	client := np.client

	var result *ncpv1alpha1.VirtualNetworkInterface
	err := wait.PollImmediate(retryInterval, retryTimeout, func() (bool, error) {
		vnetif, err := client.VmwareV1alpha1().VirtualNetworkInterfaces(namespace).Get(vnetifName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if !np.vnetifIsReady(vnetif) {
			return false, nil
		}

		result = vnetif
		return true, nil
	})

	return result, err
}

func (np *nsxtNetworkProvider) CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (vimtypes.BaseVirtualDevice, error) {
	// create NCP resource
	vnetif, err := np.createVirtualNetworkInterface(ctx, vm, vif)
	if err != nil {
		log.Error(err, "Failed to create vnetif for vif", "vif", vif)
		return nil, err
	}
	if vnetif.Status.ProviderStatus == nil || vnetif.Status.ProviderStatus.NsxLogicalSwitchID == "" {
		err = fmt.Errorf("Failed to get for nsx-t opaque network ID for vnetif '%+v'", vnetif)
		log.Error(err, "Failed to get for nsx-t opaque network ID for vnetif")
		return nil, err
	}
	networkRef, err := np.searchNetworkReference(ctx, vnetif.Status.ProviderStatus.NsxLogicalSwitchID)
	if err != nil {
		log.Error(err, "Failed to search for nsx-t network associated with vnetif", "vnetif", vnetif)
		return nil, err
	}
	// config vnic
	dev, err := createCommonVnic(ctx, networkRef, getEthCardType(vif), np.finder)
	if err != nil {
		return nil, err
	}
	nic := dev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	nic.ExternalId = vnetif.Status.InterfaceID
	nic.MacAddress = vnetif.Status.MacAddress
	nic.AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
	return dev, nil
}

// Get Host MoIDs for a cluster
func getClusterHostMoIDs(ctx context.Context, cluster *object.ClusterComputeResource) ([]vimtypes.ManagedObjectReference, error) {

	var computeResource mo.ComputeResource
	obj := cluster.Reference()

	// Get the list of ESX host moRef objects for this cluster
	err := cluster.Properties(ctx, obj, nil, &computeResource)
	if err != nil {
		log.Error(err, "Failed to get cluster properties")
		return nil, err
	}
	return computeResource.Host, nil
}
