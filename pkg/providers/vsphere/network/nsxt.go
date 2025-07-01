// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

type nsxOpaqueBacking struct {
	vimtypes.ManagedObjectReference // Won't be set.
	logicalSwitchUID                string
}

// NSX-T/VPC provides us with the logical switch UID, not the opaque network
// MoRef, so implement a simpler version of govmomi's OpaqueNetwork here.
var _ object.NetworkReference = &nsxOpaqueBacking{}

func newNSXOpaqueNetwork(lsUID string) *nsxOpaqueBacking {
	return &nsxOpaqueBacking{
		logicalSwitchUID: lsUID,
	}
}

func (n nsxOpaqueBacking) GetInventoryPath() string {
	return ""
}

// EthernetCardBackingInfo returns the VirtualDeviceBackingInfo for this Network.
func (n nsxOpaqueBacking) EthernetCardBackingInfo(ctx context.Context) (vimtypes.BaseVirtualDeviceBackingInfo, error) {
	summary, err := n.Summary(ctx)
	if err != nil {
		return nil, err
	}

	backing := &vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo{
		OpaqueNetworkId:   summary.OpaqueNetworkId,
		OpaqueNetworkType: summary.OpaqueNetworkType,
	}

	return backing, nil
}

func (n nsxOpaqueBacking) Summary(_ context.Context) (*vimtypes.OpaqueNetworkSummary, error) {
	return &vimtypes.OpaqueNetworkSummary{
		OpaqueNetworkId:   n.logicalSwitchUID,
		OpaqueNetworkType: "nsx.LogicalSwitch",
	}, nil
}

// searchNsxtNetworkReference takes in NSX-T LogicalSwitchUUID and returns the reference of the network.
func searchNsxtNetworkReference(
	ctx context.Context,
	ccr *object.ClusterComputeResource,
	networkID string) (object.NetworkReference, error) {

	// This is more or less how the old code did it. We could save repeated work by moving this
	// into the callers since it will always be for the same CCR, but the common case is one NIC,
	// or at most a handful, so that's for later.
	var obj mo.ClusterComputeResource
	if err := ccr.Properties(ctx, ccr.Reference(), []string{"network"}, &obj); err != nil {
		return nil, err
	}

	var dvpgsMoRefs []vimtypes.ManagedObjectReference
	for _, n := range obj.Network {
		if n.Type == "DistributedVirtualPortgroup" {
			dvpgsMoRefs = append(dvpgsMoRefs, n.Reference())
		}
	}

	if len(dvpgsMoRefs) == 0 {
		return nil, fmt.Errorf("ClusterComputeResource %s has no DVPGs", ccr.Reference().Value)
	}

	var dvpgs []mo.DistributedVirtualPortgroup
	err := property.DefaultCollector(ccr.Client()).Retrieve(ctx, dvpgsMoRefs, []string{"config.logicalSwitchUuid"}, &dvpgs)
	if err != nil {
		return nil, err
	}

	var dvpgMoRefs []vimtypes.ManagedObjectReference
	for _, dvpg := range dvpgs {
		if dvpg.Config.LogicalSwitchUuid == networkID {
			dvpgMoRefs = append(dvpgMoRefs, dvpg.Reference())
		}
	}

	switch len(dvpgMoRefs) {
	case 1:
		return object.NewDistributedVirtualPortgroup(ccr.Client(), dvpgMoRefs[0]), nil
	case 0:
		return nil, fmt.Errorf("no DVPG with NSX network ID %q found", networkID)
	default:
		// The LogicalSwitchUuid is supposed to be unique per CCR, so this is likely an NCP
		// misconfiguration, and we don't know which one to pick.
		return nil, fmt.Errorf("multiple DVPGs (%d) with NSX network ID %q found", len(dvpgMoRefs), networkID)
	}
}
