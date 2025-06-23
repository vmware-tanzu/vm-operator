// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// MapEthernetDevicesToSpecIdx maps the VM's ethernet devices to the corresponding
// entry in the VM's Spec.
func MapEthernetDevicesToSpecIdx(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vmMO mo.VirtualMachine) map[int32]int {

	if vmCtx.VM.Spec.Network == nil || vmMO.Config == nil || vmMO.Config.Hardware.Device == nil {
		return nil
	}

	devices := object.VirtualDeviceList(vmMO.Config.Hardware.Device)
	ethCards := devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	devKeyToSpecIdx := make(map[int32]int)

	if !pkgcfg.FromContext(vmCtx).Features.MutableNetworks {
		// For immutable, just zip these lists together. This assumes that the
		// devices are in the same order as the VM Spec.Network.Interfaces.
		for i := range min(len(ethCards), len(vmCtx.VM.Spec.Network.Interfaces)) {
			devKeyToSpecIdx[ethCards[i].GetVirtualDevice().Key] = i
		}
		return devKeyToSpecIdx
	}

	for i, interfaceSpec := range vmCtx.VM.Spec.Network.Interfaces {
		matchingIdx := findMatchingEthCardForInterfaceSpec(vmCtx, client, interfaceSpec, ethCards)
		if matchingIdx >= 0 {
			devKeyToSpecIdx[ethCards[matchingIdx].GetVirtualDevice().Key] = i
			ethCards = append(ethCards[:matchingIdx], ethCards[matchingIdx+1:]...)
		}
	}

	return devKeyToSpecIdx
}
func findMatchingEthCardForInterfaceSpec(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	ethCards object.VirtualDeviceList) int {

	matchingIdx := -1

	switch pkgcfg.FromContext(vmCtx).NetworkProviderType {
	case pkgcfg.NetworkProviderTypeVDS:
		matchingIdx = findMatchingEthCardVDS(vmCtx, client, interfaceSpec, ethCards)
	case pkgcfg.NetworkProviderTypeNSXT:
		matchingIdx = findMatchingEthCardNSXT(vmCtx, client, interfaceSpec, ethCards)
	case pkgcfg.NetworkProviderTypeVPC:
		matchingIdx = findMatchingEthCardVPC(vmCtx, client, interfaceSpec, ethCards)
	case pkgcfg.NetworkProviderTypeNamed:
		matchingIdx = findMatchingEthCardNamed(vmCtx, client, interfaceSpec, ethCards)
	}

	return matchingIdx
}

func findMatchingEthCardVDS(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	ethCards object.VirtualDeviceList) int {

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
		return -1
	}

	netIf := &netopv1alpha1.NetworkInterface{}
	netIfKey := types.NamespacedName{
		Namespace: vmCtx.VM.Namespace,
		Name:      NetOPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, true),
	}

	// Check if a networkIf object exists with the older (v1a1) naming convention.
	if err := client.Get(vmCtx, netIfKey, netIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return -1
		}

		// If NotFound set the netIf to the new v1a2 naming convention.
		netIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NetOPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, false),
			Namespace: vmCtx.VM.Namespace,
		}

		if err := client.Get(vmCtx, ctrlclient.ObjectKeyFromObject(netIf), netIf); err != nil {
			return -1
		}
	}

	return findMatchingEthCardNetOpNetIf(netIf, ethCards)
}

func findMatchingEthCardNetOpNetIf(
	netIf *netopv1alpha1.NetworkInterface,
	ethCards object.VirtualDeviceList) int {

	for i, dev := range ethCards {
		bEthCard, ok := dev.(vimtypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		ethCard := bEthCard.GetVirtualEthernetCard()
		if id := netIf.Status.ExternalID; id != "" {
			if ethCard.ExternalId != id {
				continue
			}
		} else if ethCard.ExternalId != "" {
			// This ethernet device has an external ID but the network interface CR does
			// not. In VDS, the external ID is only set on newly created interface CRs so
			// don't fallback to matching by just the backing.
			continue
		}

		dvpg, ok := ethCard.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
		if ok && dvpg.Port.PortgroupKey == netIf.Status.NetworkID {
			return i
		}
	}

	return -1
}

func findMatchingEthCardNSXT(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	ethCards object.VirtualDeviceList) int {

	var (
		networkRefName string
		networkRefType metav1.TypeMeta
	)

	if netRef := interfaceSpec.Network; netRef != nil {
		// If Name is empty, NCP will use the namespace default.
		networkRefName = netRef.Name
		networkRefType = netRef.TypeMeta
	}

	if kind := networkRefType.Kind; kind != "" && kind != "VirtualNetwork" {
		return -1
	}

	vnetIf := &ncpv1alpha1.VirtualNetworkInterface{}
	vnetIfKey := types.NamespacedName{
		Namespace: vmCtx.VM.Namespace,
		Name:      NCPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, true),
	}

	// check if a networkIf object exists with the older (v1a1) naming convention
	if err := client.Get(vmCtx, vnetIfKey, vnetIf); err != nil {
		if !apierrors.IsNotFound(err) {
			return -1
		}

		// if notFound set the vnetIf to use the new v1a2 naming convention
		vnetIf.ObjectMeta = metav1.ObjectMeta{
			Name:      NCPCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name, false),
			Namespace: vmCtx.VM.Namespace,
		}

		if err := client.Get(vmCtx, ctrlclient.ObjectKeyFromObject(vnetIf), vnetIf); err != nil {
			return -1
		}
	}

	return findMatchingEthCardNCPNetIf(vnetIf, ethCards)
}

func findMatchingEthCardNCPNetIf(
	netIf *ncpv1alpha1.VirtualNetworkInterface,
	ethCards object.VirtualDeviceList) int {

	for i, dev := range ethCards {
		bEthCard, ok := dev.(vimtypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		ethCard := bEthCard.GetVirtualEthernetCard()
		if ethCard.ExternalId == netIf.Status.InterfaceID || ethCard.MacAddress == netIf.Status.MacAddress {
			return i
		}
	}

	return -1
}

func findMatchingEthCardVPC(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	ethCards object.VirtualDeviceList) int {

	var networkRefName string
	if netRef := interfaceSpec.Network; netRef != nil {
		networkRefName = netRef.Name
	}

	subnetPort := &vpcv1alpha1.SubnetPort{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VPCCRName(vmCtx.VM.Name, networkRefName, interfaceSpec.Name),
			Namespace: vmCtx.VM.Namespace,
		},
	}

	if err := client.Get(vmCtx, ctrlclient.ObjectKeyFromObject(subnetPort), subnetPort); err != nil {
		return -1
	}

	return findMatchingEthCardVPCSubnetPort(subnetPort, ethCards)
}

func findMatchingEthCardVPCSubnetPort(
	subnetPort *vpcv1alpha1.SubnetPort,
	ethCards object.VirtualDeviceList) int {

	for i, dev := range ethCards {
		bEthCard, ok := dev.(vimtypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		ethCard := bEthCard.GetVirtualEthernetCard()
		if ethCard.ExternalId == subnetPort.Status.Attachment.ID || ethCard.MacAddress == subnetPort.Status.NetworkInterfaceConfig.MACAddress {
			return i
		}
	}

	return -1
}

func findMatchingEthCardNamed(
	_ pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	ethCards object.VirtualDeviceList) int {

	if interfaceSpec.Network == nil || interfaceSpec.Network.Name == "" {
		return -1
	}

	for i, dev := range ethCards {
		network, ok := dev.GetVirtualDevice().Backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
		if ok && network.DeviceName == interfaceSpec.Network.Name {
			return i
		}
	}

	return -1
}
