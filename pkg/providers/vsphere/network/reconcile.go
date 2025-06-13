// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

func ReconcileNetworkInterfaces(
	ctx context.Context,
	results *NetworkInterfaceResults,
	currentEthCards object.VirtualDeviceList,
) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	for idx, r := range results.Results {
		matchingIdx := FindMatchingEthCard(currentEthCards, r.Device.(vimtypes.BaseVirtualEthernetCard))
		if matchingIdx >= 0 {
			// Exact match. Claim it by removing the device from the current ethernet cards.
			matchDev := currentEthCards[matchingIdx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			results.Results[idx].DeviceKey = matchDev.Key
			results.Results[idx].MacAddress = matchDev.MacAddress
			currentEthCards = slices.Delete(currentEthCards, matchingIdx, matchingIdx+1)
		} else {
			existingIdx := findExistingEthCardForOrphanedCR(ctx, r.Name, results.OrphanedNetworkInterfaces, currentEthCards)
			if existingIdx >= 0 {
				editDev := currentEthCards[existingIdx]

				ethDev := editDev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
				ethDev.Backing = r.Device.GetVirtualDevice().Backing
				ethDev.AddressType = r.Device.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType
				ethDev.MacAddress = r.Device.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress
				ethDev.ExternalId = r.Device.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId

				deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
					Device:    editDev,
					Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				})

				currentEthCards = slices.Delete(currentEthCards, existingIdx, existingIdx+1)
			} else {
				deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
					Device:    r.Device,
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				})
			}

			results.UpdatedEthCards = true
		}
	}

	// Remove any unmatched existing interfaces.
	removeDeviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(currentEthCards))
	for _, dev := range currentEthCards {
		removeDeviceChanges = append(removeDeviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

// findExistingEthCardForOrphanedCR trys to an orphaned interface and if it has
// a matching current ethernet card, so the current card can be edited.
func findExistingEthCardForOrphanedCR(
	_ context.Context,
	interfaceName string,
	orphanedObjects []ctrlclient.Object,
	currentEthCards object.VirtualDeviceList) int {

	// TODO: Not perfect but until we annotate the interface CR with its interface spec name.
	suffix := "-" + interfaceName

	for _, obj := range orphanedObjects {
		if !strings.HasSuffix(obj.GetName(), suffix) {
			continue
		}

		switch netIf := obj.(type) {
		case *netopv1alpha1.NetworkInterface:
			return findMatchingEthCardNetOpNetIf(netIf, currentEthCards)
		case *ncpv1alpha1.VirtualNetworkInterface:
			return findMatchingEthCardNCPNetIf(netIf, currentEthCards)
		case *vpcv1alpha1.SubnetPort:
			return findMatchingEthCardVPCSubnetPort(netIf, currentEthCards)
		}
	}

	return -1
}

func FindMatchingEthCard(
	currentEthCards object.VirtualDeviceList,
	ethCard vimtypes.BaseVirtualEthernetCard) int {

	ethDev := ethCard.GetVirtualEthernetCard()
	matchingIdx := -1

	for idx := range currentEthCards {
		curDev := currentEthCards[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()

		if ethDev.AddressType == string(vimtypes.VirtualEthernetCardMacTypeManual) {
			if ethDev.MacAddress != curDev.MacAddress {
				continue
			}
		}

		if ethDev.ExternalId != "" {
			if ethDev.ExternalId != curDev.ExternalId {
				continue
			}
		}

		if curDev.Backing == nil {
			continue
		}

		var backingMatch bool
		switch a := ethDev.Backing.(type) {
		case *vimtypes.VirtualEthernetCardNetworkBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardNetworkBackingInfo(a, curDev.Backing)
		case *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardDistributedVirtualPortBackingInfo(a, curDev.Backing)
		case *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardOpaqueNetworkBackingInfo(a, curDev.Backing)
		}

		if backingMatch {
			matchingIdx = idx
			break
		}
	}

	return matchingIdx
}
