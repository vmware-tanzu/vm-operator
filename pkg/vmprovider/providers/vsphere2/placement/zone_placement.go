// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	goctx "context"
	"fmt"
	"math/rand"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vcenter"
)

type Result struct {
	ZonePlacement            bool
	InstanceStoragePlacement bool
	ZoneName                 string
	HostMoRef                *types.ManagedObjectReference
	PoolMoRef                types.ManagedObjectReference
	// TODO: Datastore, whatever else as we need it.
}

func doesVMNeedPlacement(vmCtx context.VirtualMachineContextA2) (res Result, needZonePlacement, needInstanceStoragePlacement bool) {
	res.ZonePlacement = true

	if zoneName := vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey]; zoneName != "" {
		// Zone has already been selected.
		res.ZoneName = zoneName
	} else {
		// VM does not have a zone already assigned so we need to select one.
		needZonePlacement = true
	}

	if pkgconfig.FromContext(vmCtx).Features.InstanceStorage {
		if instancestorage.IsPresent(vmCtx.VM) {
			res.InstanceStoragePlacement = true

			if hostMoID := vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]; hostMoID != "" {
				// Host has already been selected.
				res.HostMoRef = &types.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
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
	vmCtx context.VirtualMachineContextA2,
	vcClient *vim25.Client,
	rpMoIDs []string,
	zoneName, childRPName string) []string {

	childRPMoIDs := make([]string, 0, len(rpMoIDs))

	for _, rpMoID := range rpMoIDs {
		rp := object.NewResourcePool(vcClient, types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

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
	vmCtx context.VirtualMachineContextA2,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	zonePlacement bool,
	childRPName string) (map[string][]string, error) {

	var zones []topologyv1.AvailabilityZone

	if zonePlacement {
		z, err := topology.GetAvailabilityZones(vmCtx, client)
		if err != nil {
			return nil, err
		}

		zones = z
	} else {
		// Consider candidates only within the already assigned zone.
		// NOTE: GetAvailabilityZone() will return a "default" AZ when the FSS is not enabled.
		zone, err := topology.GetAvailabilityZone(vmCtx, client, vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey])
		if err != nil {
			return nil, err
		}

		zones = append(zones, zone)
	}

	candidates := map[string][]string{}

	for _, zone := range zones {
		nsInfo, ok := zone.Spec.Namespaces[vmCtx.VM.Namespace]
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

func rpMoIDToCluster(
	ctx goctx.Context,
	vcClient *vim25.Client,
	rpMoRef types.ManagedObjectReference) (*object.ClusterComputeResource, error) {

	cluster, err := object.NewResourcePool(vcClient, rpMoRef).Owner(ctx)
	if err != nil {
		return nil, err
	}

	return object.NewClusterComputeResource(vcClient, cluster.Reference()), nil
}

// getPlacementRecommendations calls DRS PlaceVM to determine clusters suitable for placement.
func getPlacementRecommendations(
	vmCtx context.VirtualMachineContextA2,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec *types.VirtualMachineConfigSpec) map[string][]Recommendation {

	recommendations := map[string][]Recommendation{}

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID}

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
	vmCtx context.VirtualMachineContextA2,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec *types.VirtualMachineConfigSpec,
	needsHost bool) map[string][]Recommendation {

	rpMOToZone := map[types.ManagedObjectReference]string{}
	var candidateRPMoRefs []types.ManagedObjectReference

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID}
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
	vmCtx context.VirtualMachineContextA2,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	configSpec *types.VirtualMachineConfigSpec,
	childRPName string) (*Result, error) {

	existingRes, zonePlacement, instanceStoragePlacement := doesVMNeedPlacement(vmCtx)
	if !zonePlacement && !instanceStoragePlacement {
		return &existingRes, nil
	}

	candidates, err := getPlacementCandidates(vmCtx, client, vcClient, zonePlacement, childRPName)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no placement candidates available")
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
