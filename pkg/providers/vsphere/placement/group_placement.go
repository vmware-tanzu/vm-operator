// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// GroupPlacement places the given group members together via a single
// DRS PlaceVmsXCluster call.
//
// preferredZoneName should be set by the caller only when every VM being
// placed in this call already has the same, non-empty
// topology.kubernetes.io/zone label. If any member has no zone label, or
// members disagree on zone, the caller must pass an empty string so
// candidates span all zone.
func GroupPlacement(
	ctx context.Context,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	namespace, childRPName, preferredZoneName string,
	configSpecs []vimtypes.VirtualMachineConfigSpec) (map[string]Result, error) {

	candidates, err := getPlacementCandidates(ctx, client, vcClient, preferredZoneName, namespace, childRPName)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrNoPlacementCandidates
	}

	needDatastorePlacement := pkgcfg.FromContext(ctx).Features.FastDeploy
	recommendations, err := getGroupPlacementRecommendations(
		ctx,
		vcClient,
		finder,
		candidates,
		configSpecs,
		needDatastorePlacement)
	if err != nil {
		return nil, err
	}

	resourcePoolToZoneName := make(map[string]string, len(candidates))
	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			resourcePoolToZoneName[rpMoID] = zoneName
		}
	}

	results := make(map[string]Result, len(recommendations))
	for vmName, recommendation := range recommendations {
		if needDatastorePlacement {
			// Get the name and type of the datastores.
			if err := getDatastoreProperties(ctx, vcClient, &recommendation); err != nil {
				return nil, err
			}
		}

		zoneName, ok := resourcePoolToZoneName[recommendation.PoolMoRef.Value]
		if !ok {
			// This should never happen: placement returned a non-candidate RP.
			return nil, fmt.Errorf("no zone assignment for ResourcePool %s",
				recommendation.PoolMoRef.Value)
		}

		result := Result{
			ZoneName:   zoneName,
			PoolMoRef:  recommendation.PoolMoRef,
			HostMoRef:  recommendation.HostMoRef,
			Datastores: recommendation.Datastores,
		}

		results[vmName] = result
	}

	return results, nil
}

func getGroupPlacementRecommendations(
	ctx context.Context,
	vcClient *vim25.Client,
	finder *find.Finder,
	candidates map[string][]string,
	configSpecs []vimtypes.VirtualMachineConfigSpec,
	needDatastorePlacement bool) (map[string]Recommendation, error) {

	var candidateRPMoRefs []vimtypes.ManagedObjectReference

	for _, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeResourcePool),
				Value: rpMoID,
			}
			candidateRPMoRefs = append(candidateRPMoRefs, rpMoRef)
		}
	}

	return getClusterPlacementRecommendations(
		ctx,
		vcClient,
		finder,
		candidateRPMoRefs,
		configSpecs,
		true,
		needDatastorePlacement)
}
