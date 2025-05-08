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
		for i := range min(len(ethCards), len(vmCtx.VM.Spec.Network.Interfaces)) {
			devKeyToSpecIdx[ethCards[i].GetVirtualDevice().Key] = i
		}
		return devKeyToSpecIdx
	}

	for i, interfaceSpec := range vmCtx.VM.Spec.Network.Interfaces {
		matchingIdx := findMatchingEthCard(vmCtx, client, interfaceSpec, ethCards)
		if matchingIdx >= 0 {
			devKeyToSpecIdx[ethCards[matchingIdx].GetVirtualDevice().Key] = i
			ethCards = append(ethCards[:matchingIdx], ethCards[matchingIdx+1:]...)
		}
	}

	return devKeyToSpecIdx
}
func findMatchingEthCard(
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

	// For NetOP until we start to set the ExternalID we just zip interfaces by backing.
	for i, dev := range ethCards {
		dvpg, ok := dev.GetVirtualDevice().Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
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

	for i, dev := range ethCards {
		bEthCard, ok := dev.(vimtypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		ethCard := bEthCard.GetVirtualEthernetCard()
		if ethCard.ExternalId == vnetIf.Status.InterfaceID || ethCard.MacAddress == vnetIf.Status.MacAddress {
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
		return -1
	}

	if err := client.Get(vmCtx, ctrlclient.ObjectKeyFromObject(vpcSubnetPort), vpcSubnetPort); err != nil {
		return -1
	}

	for i, dev := range ethCards {
		bEthCard, ok := dev.(vimtypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		ethCard := bEthCard.GetVirtualEthernetCard()
		if ethCard.ExternalId == vpcSubnetPort.Status.Attachment.ID || ethCard.MacAddress == vpcSubnetPort.Status.NetworkInterfaceConfig.MACAddress {
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
