// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

func GroupPlacement(
	ctx context.Context,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	namespace, childRPName string,
	configSpecs []vimtypes.VirtualMachineConfigSpec) (map[string]Result, error) {

	candidates, resourcePoolToZoneName, err := getPlacementCandidates(ctx, client, vcClient, "", namespace, childRPName)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no placement candidates available")
	}

	recommendations, err := getGroupPlacementRecommendations(ctx, vcClient, finder, candidates, configSpecs)
	if err != nil {
		return nil, err
	}

	results := map[string]Result{}
	for vmName, vmRecommendations := range recommendations {
		selectedRecommendation := vmRecommendations[rand.Intn(len(vmRecommendations))] // nolint:gosec

		if pkgcfg.FromContext(ctx).Features.FastDeploy {
			// Get the name and type of the datastores.
			if err := getDatastoreProperties(ctx, vcClient, &selectedRecommendation); err != nil {
				return nil, err
			}
		}

		zoneName := resourcePoolToZoneName[selectedRecommendation.PoolMoRef.Value]

		result := Result{
			ZoneName:   zoneName,
			PoolMoRef:  selectedRecommendation.PoolMoRef,
			HostMoRef:  selectedRecommendation.HostMoRef,
			Datastores: selectedRecommendation.Datastores,
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
	configSpecs []vimtypes.VirtualMachineConfigSpec) (map[string][]Recommendation, error) {

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

	return ClusterPlaceVMForCreate(
		ctx,
		vcClient,
		finder,
		candidateRPMoRefs,
		configSpecs,
		false,
		true)
}
