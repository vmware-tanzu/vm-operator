// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
)

type Constraints struct {
	// ChildRPName is the name of the child ResourcePool that must exist under the NS/Zone's RP.
	ChildRPName string

	// Zones when non-empty is the set of allowed zone names that the possible placement candidates
	// will further be filtered by.
	Zones sets.Set[string]

	// TODO: ClusterModules?
}

type Result struct {
	ZonePlacement            bool
	InstanceStoragePlacement bool
	ZoneName                 string
	HostMoRef                *vimtypes.ManagedObjectReference
	PoolMoRef                vimtypes.ManagedObjectReference
	// TODO: Datastore, whatever else as we need it.
}

func doesVMNeedPlacement(vmCtx pkgctx.VirtualMachineContext) (res Result, needZonePlacement, needInstanceStoragePlacement bool) {
	res.ZonePlacement = true

	if zoneName := vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey]; zoneName != "" {
		// Zone has already been selected.
		res.ZoneName = zoneName
	} else {
		// VM does not have a zone already assigned so we need to select one.
		needZonePlacement = true
	}

	if pkgcfg.FromContext(vmCtx).Features.InstanceStorage {
		if instancestorage.IsPresent(vmCtx.VM) {
			res.InstanceStoragePlacement = true

			if hostMoID := vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]; hostMoID != "" {
				// Host has already been selected.
				res.HostMoRef = &vimtypes.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
			} else {
				// VM has InstanceStorage volumes so we need to select a host.
				needInstanceStoragePlacement = true
			}
		}
	}

	return
}

// lookupChildRPs lookups the child ResourcePool under each parent ResourcePool. A VM with a ResourcePolicy
// may specify a child ResourcePool that the VM will be created under.
func lookupChildRPs(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	rpMoIDs []string,
	zoneName, childRPName string) []string {

	childRPMoIDs := make([]string, 0, len(rpMoIDs))

	for _, rpMoID := range rpMoIDs {
		rp := object.NewResourcePool(vcClient, vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

		childRP, err := vcenter.GetChildResourcePool(vmCtx, rp, childRPName)
		if err != nil {
			vmCtx.Logger.Error(err, "Skipping this resource pool since failed to get child ResourcePool",
				"zone", zoneName, "parentRPMoID", rpMoID, "childRPName", childRPName)
			continue
		}

		childRPMoIDs = append(childRPMoIDs, childRP.Reference().Value)
	}

	return childRPMoIDs
}

// getPlacementCandidates determines the candidate resource pools for VM placement.
func getPlacementCandidates(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	zonePlacement bool,
	childRPName string) (map[string][]string, error) {

	candidates := map[string][]string{}

	// When FSS_WCP_WORKLOAD_DOMAIN_ISOLATION is enabled, use namespaced Zone CR to get candidate resource pools.
	if pkgcfg.FromContext(vmCtx).Features.WorkloadDomainIsolation {
		var zones []topologyv1.Zone
		if zonePlacement {
			z, err := topology.GetZones(vmCtx, client, vmCtx.VM.Namespace)
			if err != nil {
				return nil, err
			}
			zones = z
		} else {
			zone, err := topology.GetZone(vmCtx, client, vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey], vmCtx.VM.Namespace)
			if err != nil {
				return nil, err
			}
			zones = append(zones, zone)
		}

		for _, zone := range zones {
			// Filter out the zone that is to be deleted, so we don't have it as a candidate when doing placement.
			if zonePlacement && !zone.DeletionTimestamp.IsZero() {
				continue
			}
			rpMoIDs := zone.Spec.ManagedVMs.PoolMoIDs
			if childRPName != "" {
				childRPMoIDs := lookupChildRPs(vmCtx, vcClient, rpMoIDs, zone.Name, childRPName)
				if len(childRPMoIDs) == 0 {
					vmCtx.Logger.Info("Zone had no candidates after looking up children ResourcePools",
						"zone", zone.Name, "rpMoIDs", rpMoIDs, "childRPName", childRPName)
					continue
				}
				rpMoIDs = childRPMoIDs
			}

			candidates[zone.Name] = rpMoIDs
		}
		return candidates, nil
	}

	// When FSS_WCP_WORKLOAD_DOMAIN_ISOLATION is disabled, use cluster scoped AvailabilityZone CR to get candidate resource pools.
	var azs []topologyv1.AvailabilityZone
	if zonePlacement {
		az, err := topology.GetAvailabilityZones(vmCtx, client)
		if err != nil {
			return nil, err
		}

		azs = az
	} else {
		// Consider candidates only within the already assigned zone.
		// NOTE: GetAvailabilityZone() will return a "default" AZ when WCP_FaultDomains FSS is not enabled.
		az, err := topology.GetAvailabilityZone(vmCtx, client, vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey])
		if err != nil {
			return nil, err
		}

		azs = append(azs, az)
	}

	for _, az := range azs {
		nsInfo, ok := az.Spec.Namespaces[vmCtx.VM.Namespace]
		if !ok {
			continue
		}

		var rpMoIDs []string
		if len(nsInfo.PoolMoIDs) != 0 {
			rpMoIDs = nsInfo.PoolMoIDs
		} else {
			rpMoIDs = []string{nsInfo.PoolMoId}
		}

		if childRPName != "" {
			childRPMoIDs := lookupChildRPs(vmCtx, vcClient, rpMoIDs, az.Name, childRPName)
			if len(childRPMoIDs) == 0 {
				vmCtx.Logger.Info("AvailabilityZone had no candidates after looking up children ResourcePools",
					"az", az.Name, "rpMoIDs", rpMoIDs, "childRPName", childRPName)
				continue
			}
			rpMoIDs = childRPMoIDs
		}

		candidates[az.Name] = rpMoIDs
	}

	return candidates, nil
}

func rpMoIDToCluster(
	ctx context.Context,
	vcClient *vim25.Client,
	rpMoRef vimtypes.ManagedObjectReference) (*object.ClusterComputeResource, error) {

	cluster, err := object.NewResourcePool(vcClient, rpMoRef).Owner(ctx)
	if err != nil {
		return nil, err
	}

	return object.NewClusterComputeResource(vcClient, cluster.Reference()), nil
}

// getPlacementRecommendations calls DRS PlaceVM to determine clusters suitable for placement.
func getPlacementRecommendations(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec vimtypes.VirtualMachineConfigSpec) map[string][]Recommendation {

	recommendations := map[string][]Recommendation{}

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID}

			cluster, err := rpMoIDToCluster(vmCtx, vcClient, rpMoRef)
			if err != nil {
				vmCtx.Logger.Error(err, "failed to get CCR from RP", "zone", zoneName, "rpMoID", rpMoID)
				continue
			}

			recs, err := PlaceVMForCreate(vmCtx, cluster, configSpec)
			if err != nil {
				vmCtx.Logger.Error(err, "PlaceVM failed", "zone", zoneName,
					"clusterMoID", cluster.Reference().Value, "rpMoID", rpMoID)
				continue
			}

			if len(recs) == 0 {
				vmCtx.Logger.Info("No placement recommendations", "zone", zoneName,
					"clusterMoID", cluster.Reference().Value, "rpMoID", rpMoID)
				continue
			}

			// Replace the resource pool returned by PlaceVM - that is the cluster's root RP - with
			// our more specific namespace or namespace child RP since this VM needs to be under the
			// more specific RP. This makes the recommendations returned here the same as what zonal
			// would return.
			for idx := range recs {
				recs[idx].PoolMoRef = rpMoRef
			}

			recommendations[zoneName] = append(recommendations[zoneName], recs...)
		}
	}

	vmCtx.Logger.V(5).Info("Placement recommendations", "recommendations", recommendations)

	return recommendations
}

// getZonalPlacementRecommendations calls DRS PlaceVmsXCluster to determine clusters suitable for placement.
func getZonalPlacementRecommendations(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec vimtypes.VirtualMachineConfigSpec,
	needsHost bool) map[string][]Recommendation {

	rpMOToZone := map[vimtypes.ManagedObjectReference]string{}
	var candidateRPMoRefs []vimtypes.ManagedObjectReference

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID}
			candidateRPMoRefs = append(candidateRPMoRefs, rpMoRef)
			rpMOToZone[rpMoRef] = zoneName
		}
	}

	var recs []Recommendation

	if len(candidateRPMoRefs) == 1 {
		// If there is only one candidate, we might be able to skip some work.

		if needsHost {
			// This is a hack until PlaceVmsXCluster() supports instance storage disks.
			vmCtx.Logger.Info("Falling back into non-zonal placement since the only candidate needs host selected",
				"rpMoID", candidateRPMoRefs[0].Value)
			return getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)
		}

		recs = append(recs, Recommendation{
			PoolMoRef: candidateRPMoRefs[0],
		})
		vmCtx.Logger.V(5).Info("Implied placement since there was only one candidate", "rec", recs[0])

	} else {
		var err error

		recs, err = ClusterPlaceVMForCreate(vmCtx, vcClient, candidateRPMoRefs, configSpec, needsHost)
		if err != nil {
			vmCtx.Logger.Error(err, "PlaceVmsXCluster failed")
			return nil
		}
	}

	recommendations := map[string][]Recommendation{}
	for _, rec := range recs {
		if rpZoneName, ok := rpMOToZone[rec.PoolMoRef]; ok {
			recommendations[rpZoneName] = append(recommendations[rpZoneName], rec)
		} else {
			vmCtx.Logger.V(4).Info("Received unexpected ResourcePool recommendation",
				"poolMoRef", rec.PoolMoRef)
		}
	}

	vmCtx.Logger.V(5).Info("Placement recommendations", "recommendations", recommendations)

	return recommendations
}

// MakePlacementDecision selects one of the recommendations for placement.
func MakePlacementDecision(recommendations map[string][]Recommendation) (string, Recommendation) {
	// Use an explicit rand.Intn() instead of first entry returned by map iterator.
	zoneNames := make([]string, 0, len(recommendations))
	for zoneName := range recommendations {
		zoneNames = append(zoneNames, zoneName)
	}
	zoneName := zoneNames[rand.Intn(len(zoneNames))] //nolint:gosec

	recs := recommendations[zoneName]
	return zoneName, recs[rand.Intn(len(recs))] //nolint:gosec
}

// Placement determines if the VM needs placement, and if so, determines where to place the VM
// and updates the Labels and Annotations with the placement decision.
func Placement(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	configSpec vimtypes.VirtualMachineConfigSpec,
	constraints Constraints) (*Result, error) {

	existingRes, zonePlacement, instanceStoragePlacement := doesVMNeedPlacement(vmCtx)
	if !zonePlacement && !instanceStoragePlacement {
		return &existingRes, nil
	}

	candidates, err := getPlacementCandidates(vmCtx, client, vcClient, zonePlacement, constraints.ChildRPName)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no placement candidates available")
	}

	if constraints.Zones.Len() > 0 {
		// The VM's candidates may be limited due to external constraints, such as the
		// requested zones of its PVCs. Apply those constraints here.
		var disallowedZones []string
		allowedCandidates := map[string][]string{}

		for zoneName, rpMoIDs := range candidates {
			if constraints.Zones.Has(zoneName) {
				allowedCandidates[zoneName] = rpMoIDs
			} else {
				disallowedZones = append(disallowedZones, zoneName)
			}
		}

		if len(disallowedZones) > 0 {
			vmCtx.Logger.V(6).Info("Removed candidate zones due to constraints",
				"candidateZones", maps.Keys(candidates), "disallowedZones", disallowedZones)
		}

		if len(allowedCandidates) == 0 {
			return nil, fmt.Errorf("no placement candidates available after applying zone constraints: %s",
				strings.Join(constraints.Zones.UnsortedList(), ","))
		}

		candidates = allowedCandidates
	}

	// TBD: May want to get the host for vGPU and other passthru devices too.
	needsHost := instanceStoragePlacement

	var recommendations map[string][]Recommendation
	if zonePlacement {
		recommendations = getZonalPlacementRecommendations(vmCtx, vcClient, candidates, configSpec, needsHost)
	} else /* instanceStoragePlacement */ {
		recommendations = getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)
	}
	if len(recommendations) == 0 {
		return nil, fmt.Errorf("no placement recommendations available")
	}

	zoneName, rec := MakePlacementDecision(recommendations)
	vmCtx.Logger.V(5).Info("Placement decision result", "zone", zoneName, "recommendation", rec)

	result := &Result{
		ZonePlacement:            zonePlacement,
		InstanceStoragePlacement: instanceStoragePlacement,
		ZoneName:                 zoneName,
		PoolMoRef:                rec.PoolMoRef,
		HostMoRef:                rec.HostMoRef,
	}

	return result, nil
}
