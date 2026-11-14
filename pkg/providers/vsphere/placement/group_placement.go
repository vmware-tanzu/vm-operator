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
// Deleting zones are excluded unless every VM is pinned to the same zone.
// Admission restricts new VMs pinned to deleting zones to privileged accounts
// and CAPV service accounts, regardless of the VM's labels identifying its owner.
//
// Placement candidates are constrained to a single zone only when every VM has
// the same, non-empty zone. Each pinned zone must be a placement candidate, and
// each recommendation must honor the VM's pinned zone, regardless of feature flags.
// A violation fails placement for the entire group.
//
// When VMHardAffinityDuringExecution is enabled, each VM's zone is also sent as
// its CandidateVsphereZone. The field is omitted when the feature is disabled.
func GroupPlacement(
	ctx context.Context,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	namespace, childRPName string,
	vmPlacementInputs []VMPlacementInput) (map[string]Result, error) {

	// Honor a shared, admitted zone override, including a deleting zone.
	sharedZone := sharedZoneName(vmPlacementInputs)

	candidates, err := getPlacementCandidates(ctx, client, vcClient, sharedZone, namespace, childRPName)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil, ErrNoPlacementCandidates
	}

	needDatastorePlacement := pkgcfg.FromContext(ctx).Features.FastDeploy

	// Build the mappings sent to PlaceVmsXCluster, where ZoneName is the VM's
	// candidate zone. A VM's zone must be a key in candidates (zone name to RP
	// MoIDs).
	candidateMappings := make([]VMPlacementInput, len(vmPlacementInputs))
	vmPinnedZoneMap := make(map[string]string, len(vmPlacementInputs))
	for i, m := range vmPlacementInputs {
		candidateMappings[i].ConfigSpec = m.ConfigSpec

		// Only VMs with a pinned zone require zone validation.
		if m.ZoneName == "" {
			continue
		}
		if _, ok := candidates[m.ZoneName]; !ok {
			return nil, fmt.Errorf("for VM %s zone %s is not a placement candidate: %w",
				m.ConfigSpec.Name, m.ZoneName, ErrNoPlacementCandidates)
		}
		candidateMappings[i].ZoneName = m.ZoneName
		vmPinnedZoneMap[m.ConfigSpec.Name] = m.ZoneName
	}
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

	results := make(map[string]Result, len(recommendations))
	for vmName, recommendation := range recommendations {
		zoneName, ok := resourcePoolToZoneName[recommendation.PoolMoRef.Value]
		if !ok {
			// This should never happen: placement returned a non-candidate RP.
			return nil, fmt.Errorf("no zone assignment for ResourcePool %s",
				recommendation.PoolMoRef.Value)
		}

		// Reject recommendations outside the pinned zone, even when the feature
		// flag prevents sending CandidateVsphereZone to PlaceVmsXCluster.
		if want := vmPinnedZoneMap[vmName]; want != "" && want != zoneName {
			return nil, fmt.Errorf("%w: VM %s requested zone %s, recommended zone %s",
				ErrGroupPlacementZoneMismatch, vmName, want, zoneName)
		}

		if needDatastorePlacement {
			// Get the name and type of the datastores.
			if err := getDatastoreProperties(ctx, vcClient, &recommendation); err != nil {
				return nil, err
			}
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

// sharedZoneName returns the zone name if every input has the same,
// non-empty zone. Otherwise, it returns an empty string.
func sharedZoneName(vmPlacementInputs []VMPlacementInput) string {
	if len(vmPlacementInputs) == 0 || vmPlacementInputs[0].ZoneName == "" {
		return ""
	}
	for _, m := range vmPlacementInputs[1:] {
		if m.ZoneName != vmPlacementInputs[0].ZoneName {
			return ""
		}
	}
	return vmPlacementInputs[0].ZoneName
}

func getGroupPlacementRecommendations(
	ctx context.Context,
	vcClient *vim25.Client,
	finder *find.Finder,
	candidates map[string][]string,
	vmPlacementInputs []VMPlacementInput,
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
		vmPlacementInputs,
		true,
		needDatastorePlacement)
}
