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
// Each mapping's ZoneName holds the VM's topology.kubernetes.io/zone label. An
// empty ZoneName means the VM has no pinned zone.
//
// Placement candidates are constrained to a zone only when every VM has the
// same, non-empty zone.
//
// When the VMHardAffinityDuringExecution feature is enabled, each VM's zone is also
// sent as its CandidateVsphereZone. Placement fails for the entire group if any
// VM's zone is not a placement candidate, since group placement is
// all-or-nothing.
func GroupPlacement(
	ctx context.Context,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	namespace, childRPName string,
	vmToZoneMappings []VMZonePlacementMapping) (map[string]Result, error) {

	// If all vms share the same zone, set it as preferredZoneNameForGroup.
	preferredZoneNameForGroup := sharedZoneName(vmToZoneMappings)

	candidates, err := getPlacementCandidates(ctx, client, vcClient, preferredZoneNameForGroup, namespace, childRPName)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrNoPlacementCandidates
	}

	// Build the mappings sent to PlaceVmsXCluster, where ZoneName is the VM's
	// candidate zone. A VM's zone must be a key in candidates (zone name to RP
	// MoIDs).
	perVMZones := pkgcfg.FromContext(ctx).Features.VMHardAffinityDuringExecution
	candidateMappings := make([]VMZonePlacementMapping, len(vmToZoneMappings))
	vmNameToZoneName := make(map[string]string, len(vmToZoneMappings))
	for i, m := range vmToZoneMappings {
		candidateMappings[i].ConfigSpec = m.ConfigSpec

		// Do not set candidateZone if capability is disabled or if vm does not have
		// preferred zone.
		if !perVMZones || m.ZoneName == "" {
			continue
		}
		if _, ok := candidates[m.ZoneName]; !ok {
			return nil, fmt.Errorf("VM %s zone %s is not a placement candidate: %w",
				m.ConfigSpec.Name, m.ZoneName, ErrNoPlacementCandidates)
		}
		candidateMappings[i].ZoneName = m.ZoneName
		vmNameToZoneName[m.ConfigSpec.Name] = m.ZoneName
	}

	needDatastorePlacement := pkgcfg.FromContext(ctx).Features.FastDeploy
	recommendations, err := getGroupPlacementRecommendations(
		ctx,
		vcClient,
		finder,
		candidates,
		candidateMappings,
		needDatastorePlacement)
	if err != nil {
		return nil, err
	}

	// Invert candidates (zone name to RP MoIDs) so a recommended RP can be
	// mapped back to its zone.
	resourcePoolToZoneName := make(map[string]string, len(candidates))
	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			resourcePoolToZoneName[rpMoID] = zoneName
		}
	}

	vmNameToRecZoneName := make(map[string]string, len(recommendations))
	for vmName, recommendation := range recommendations {
		zoneName, ok := resourcePoolToZoneName[recommendation.PoolMoRef.Value]
		if !ok {
			// This should never happen: placement returned a non-candidate RP.
			return nil, fmt.Errorf("no zone assignment for ResourcePool %s",
				recommendation.PoolMoRef.Value)
		}

		// PlaceVmsXCluster should honor the VM's candidate zone. Fail if it
		// recommended an RP in a different zone.
		if want := vmNameToZoneName[vmName]; want != "" && want != zoneName {
			return nil, fmt.Errorf("%w: VM %s preassigned zone %s, recommended zone %s",
				ErrGroupPlacementZoneMismatch, vmName, want, zoneName)
		}

		vmNameToRecZoneName[vmName] = zoneName
	}

	results := make(map[string]Result, len(recommendations))
	for vmName, recommendation := range recommendations {
		if needDatastorePlacement {
			// Get the name and type of the datastores.
			if err := getDatastoreProperties(ctx, vcClient, &recommendation); err != nil {
				return nil, err
			}
		}

		result := Result{
			ZoneName:   vmNameToRecZoneName[vmName],
			PoolMoRef:  recommendation.PoolMoRef,
			HostMoRef:  recommendation.HostMoRef,
			Datastores: recommendation.Datastores,
		}

		results[vmName] = result
	}

	return results, nil
}

// sharedZoneName returns the zone name if every mapping has the same,
// non-empty zone. Otherwise, it returns an empty string.
func sharedZoneName(vmToZoneMappings []VMZonePlacementMapping) string {
	if len(vmToZoneMappings) == 0 || vmToZoneMappings[0].ZoneName == "" {
		return ""
	}
	for _, m := range vmToZoneMappings[1:] {
		if m.ZoneName != vmToZoneMappings[0].ZoneName {
			return ""
		}
	}
	return vmToZoneMappings[0].ZoneName
}

func getGroupPlacementRecommendations(
	ctx context.Context,
	vcClient *vim25.Client,
	finder *find.Finder,
	candidates map[string][]string,
	vmToZoneMappings []VMZonePlacementMapping,
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
		vmToZoneMappings,
		true,
		needDatastorePlacement)
}
