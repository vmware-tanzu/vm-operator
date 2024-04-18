// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// ResolveBackingPostPlacement fixes up the backings where we did not know the CCR until after
// placement. This should be called if CreateAndWaitForNetworkInterfaces() was called with a nil
// clusterMoRef. Returns true if a backing was resolved, so the ConfigSpec needs to be updated.
func ResolveBackingPostPlacement(
	ctx context.Context,
	vimClient *vim25.Client,
	clusterMoRef vimtypes.ManagedObjectReference,
	results *NetworkInterfaceResults) (bool, error) {

	if len(results.Results) == 0 {
		return false, nil
	}

	networkType := pkgcfg.FromContext(ctx).NetworkProviderType
	if networkType == "" {
		return false, fmt.Errorf("no network provider set")
	}

	ccr := object.NewClusterComputeResource(vimClient, clusterMoRef)
	fixedUp := false

	for idx := range results.Results {
		if results.Results[idx].Backing != nil {
			continue
		}

		var backing object.NetworkReference
		var err error

		switch networkType {
		case pkgcfg.NetworkProviderTypeNSXT:
			backing, err = searchNsxtNetworkReference(ctx, ccr, results.Results[idx].NetworkID)
			if err != nil {
				err = fmt.Errorf("post placement NSX-T backing fixup failed: %w", err)
			}
		// VPC is an NSX-T construct that is attached to an NSX-T Project.
		case pkgcfg.NetworkProviderTypeVPC:
			backing, err = searchNsxtNetworkReference(ctx, ccr, results.Results[idx].NetworkID)
			if err != nil {
				err = fmt.Errorf("post placement NSX-T-VPC backing fixup failed: %w", err)
			}
		default:
			err = fmt.Errorf("only NSX-T networks are expected to need post placement backing fixup")
		}

		if err != nil {
			return false, err
		}

		fixedUp = true
		results.Results[idx].Backing = backing
	}

	return fixedUp, nil
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
		return nil, fmt.Errorf("no DVPG with NSX-T network ID %q found", networkID)
	default:
		// The LogicalSwitchUuid is supposed to be unique per CCR, so this is likely an NCP
		// misconfiguration, and we don't know which one to pick.
		return nil, fmt.Errorf("multiple DVPGs (%d) with NSX-T network ID %q found", len(dvpgMoRefs), networkID)
	}
}
