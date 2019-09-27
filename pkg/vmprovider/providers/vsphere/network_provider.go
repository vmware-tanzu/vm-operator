/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
)

const (
	DefaultEthernetCardType = "vmxnet3"
	NsxtNetworkType         = "nsx-t"

	retryInterval = 100 * time.Millisecond
	retryTimeout  = 15 * time.Second
)

// NetworkProvider sets up network for different type of network
type NetworkProvider interface {
	// CreateVnic creates the VirtualEthernetCard for the network interface
	CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (types.BaseVirtualDevice, error)
}

func getEthCardType(vif *v1alpha1.VirtualMachineNetworkInterface) string {
	ethCardType := vif.EthernetCardType
	if vif.EthernetCardType == "" {
		ethCardType = DefaultEthernetCardType
	}
	return ethCardType
}

func createCommonVnic(ctx context.Context, networkName, ethCardType string, finder *find.Finder) (types.BaseVirtualDevice, error) {
	ref, err := finder.Network(ctx, networkName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find network %q", networkName)
	}
	backing, err := ref.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create new ethernet card backing info for network %q", networkName)
	}
	dev, err := object.EthernetCardTypes().CreateEthernetCard(ethCardType, backing)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create new ethernet card %q for network %q", ethCardType, networkName)
	}
	return dev, nil
}

func setVnicKey(dev types.BaseVirtualDevice, key int32) types.BaseVirtualDevice {
	nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
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

func (np *defaultNetworkProvider) CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (types.BaseVirtualDevice, error) {
	return createCommonVnic(ctx, vif.NetworkName, getEthCardType(vif), np.finder)
}

type nsxtNetworkProvider struct {
	finder *find.Finder
	client clientset.Interface
}

// NsxtNetworkProvider returns a defaultNetworkProvider instance
func NsxtNetworkProvider(finder *find.Finder, client clientset.Interface) *nsxtNetworkProvider {
	return &nsxtNetworkProvider{
		finder: finder,
		client: client,
	}
}

// GenerateNsxVnetifName generates the vnetif name for the VM
func (np *nsxtNetworkProvider) GenerateNsxVnetifName(networkName, vmName string) string {
	return fmt.Sprintf("%s-%s-lsp", networkName, vmName)
}

// searchOpaqueNetworkName takes in nsx-t logical switch UUID and returns the name of the nsx-t network
func (np *nsxtNetworkProvider) searchOpaqueNetworkName(ctx context.Context, networkID string) (string, error) {
	networks, err := np.finder.NetworkList(ctx, "*")
	if err != nil {
		return "", err
	}
	for _, network := range networks {
		obj, ok := network.(*object.OpaqueNetwork)
		if !ok {
			continue
		}

		var net mo.OpaqueNetwork

		if err := obj.Properties(ctx, obj.Reference(), []string{"summary"}, &net); err != nil {
			return "", err
		}

		summary, _ := net.Summary.(*vimTypes.OpaqueNetworkSummary)
		if summary.OpaqueNetworkId == networkID {
			return obj.InventoryPath, nil
		}
	}
	return "", fmt.Errorf("opaque network with ID '%s' not found", networkID)
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
		if !np.ownerMatch(vm, vnetif) {
			copiedVnetif := currentVnetif.DeepCopy()
			np.setVnetifOwner(vm, copiedVnetif)
			_, err = np.client.VmwareV1alpha1().VirtualNetworkInterfaces(vm.Namespace).Update(copiedVnetif)
			if err != nil {
				return nil, err
			}
		}
	}

	// Wait until the vnetif status is available
	result, err := np.waitForVnetIFStatus(vm.Namespace, vmif.NetworkName, vm.Name)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// vnetifIsReady checks the readyness of vnetif object
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

func (np *nsxtNetworkProvider) CreateVnic(ctx context.Context, vm *v1alpha1.VirtualMachine, vif *v1alpha1.VirtualMachineNetworkInterface) (types.BaseVirtualDevice, error) {
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
	networkName, err := np.searchOpaqueNetworkName(ctx, vnetif.Status.ProviderStatus.NsxLogicalSwitchID)
	if err != nil {
		log.Error(err, "Failed to search for nsx-t network associated with vnetif", "vnetif", vnetif)
		return nil, err
	}
	// config vnic
	dev, err := createCommonVnic(ctx, networkName, getEthCardType(vif), np.finder)
	if err != nil {
		return nil, err
	}
	nic := dev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	nic.ExternalId = vnetif.Status.InterfaceID
	nic.MacAddress = vnetif.Status.MacAddress
	nic.AddressType = string(types.VirtualEthernetCardMacTypeManual)
	return dev, nil
}
