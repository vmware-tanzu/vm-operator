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
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
)

func doesVMNeedPlacement(vmCtx context.VirtualMachineContext) (zonePlacement, instanceStoragePlacement bool) {
	if lib.IsWcpFaultDomainsFSSEnabled() {
		// If the VM does not have a zone already assigned, a zone needs to be selected.
		if vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] == "" {
			zonePlacement = true
		}
	}

	if lib.IsInstanceStorageFSSEnabled() {
		// If the VM has InstanceStorage volumes, a host needs to be selected.
		if instancestorage.IsConfigured(vmCtx.VM) {
			if vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] == "" {
				instanceStoragePlacement = true
			}
		}
	}

	return
}

// getPlacementCandidates determines the candidate resource pools for VM placement.
func getPlacementCandidates(
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client,
	zonePlacement bool) (map[string][]string, error) {

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
		if ok {
			if len(nsInfo.PoolMoIDs) != 0 {
				candidates[zone.Name] = nsInfo.PoolMoIDs
			} else {
				candidates[zone.Name] = []string{nsInfo.PoolMoId}
			}
		}
	}

	return candidates, nil
}

func rpMoIDToCluster(
	ctx goctx.Context,
	vcClient *vim25.Client,
	rpMoID string) (*object.ClusterComputeResource, error) {

	rp := object.NewResourcePool(vcClient, types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

	cluster, err := rp.Owner(ctx)
	if err != nil {
		return nil, err
	}

	return object.NewClusterComputeResource(vcClient, cluster.Reference()), nil
}

// getPlacementRecommendations calls DRS PlaceVM to determine clusters suitable for placement.
func getPlacementRecommendations(
	vmCtx context.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec *types.VirtualMachineConfigSpec) map[string][]Recommendation {

	recommendations := map[string][]Recommendation{}

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			cluster, err := rpMoIDToCluster(vmCtx, vcClient, rpMoID)
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

			recommendations[zoneName] = append(recommendations[zoneName], recs...)
		}
	}

	vmCtx.Logger.V(5).Info("Placement recommendations", "recommendations", recommendations)

	return recommendations
}

// getZonalPlacementRecommendations calls DRS PlaceVmsXCluster to determine clusters suitable for placement.
func getZonalPlacementRecommendations(
	vmCtx context.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec *types.VirtualMachineConfigSpec,
	childRPName string) map[string][]Recommendation {

	rpMOToZone := map[types.ManagedObjectReference]string{}
	var candidateRPMoRefs []types.ManagedObjectReference

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := types.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID}

			if childRPName != "" {
				// If this VM has SetResourcePolicy - because it is apart of a TKG - lookup the
				// child RP under the namespace's RP since it may have more specific resource limits.
				rp := object.NewResourcePool(vcClient, rpMoRef)

				childRP, err := vcenter.GetChildResourcePool(vmCtx, rp, childRPName)
				if err != nil {
					vmCtx.Logger.Error(err, "Failed to get child ResourcePool",
						"parentRPMoID", rpMoRef.Value, "childRPName", childRPName)
					continue
				}

				rpMoRef = childRP.Reference()
			}

			candidateRPMoRefs = append(candidateRPMoRefs, rpMoRef)
			rpMOToZone[rpMoRef] = zoneName
		}
	}

	if len(candidateRPMoRefs) == 1 {
		vmCtx.Logger.Info(
			"Falling back into non-zonal placement since there is only one candidate",
			"rpMoID", candidateRPMoRefs[0].Value)
		return getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)

	}

	recs, err := ClusterPlaceVMForCreate(vmCtx, vcClient, candidateRPMoRefs, configSpec)
	if err != nil {
		vmCtx.Logger.Error(err, "PlaceVmsXCluster failed")
		return nil
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
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	configSpec *types.VirtualMachineConfigSpec,
	childRPName string) error {

	zonePlacement, instanceStoragePlacement := doesVMNeedPlacement(vmCtx)
	if !zonePlacement && !instanceStoragePlacement {
		return nil
	}

	candidates, err := getPlacementCandidates(vmCtx, client, zonePlacement)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		return fmt.Errorf("no placement candidates available")
	}

	var recommendations map[string][]Recommendation
	if zonePlacement {
		recommendations = getZonalPlacementRecommendations(vmCtx, vcClient, candidates, configSpec, childRPName)
	} else /* instanceStoragePlacement */ {
		recommendations = getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)
	}
	if len(recommendations) == 0 {
		return fmt.Errorf("no placement recommendations available")
	}

	zoneName, rec := MakePlacementDecision(recommendations)
	vmCtx.Logger.V(5).Info("Placement decision result", "zone", zoneName, "recommendation", rec)

	if zonePlacement {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = map[string]string{}
		}
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
	}

	if instanceStoragePlacement {
		if rec.HostMoRef == nil {
			return fmt.Errorf("recommendation is missing host required for instance storage")
		}

		hostMoID := rec.HostMoRef.Value

		hostFQDN, err := vcenter.GetESXHostFQDN(vmCtx, vcClient, hostMoID)
		if err != nil {
			return err
		}

		if vmCtx.VM.Annotations == nil {
			vmCtx.VM.Annotations = map[string]string{}
		}
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey] = hostMoID
		vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeAnnotationKey] = hostFQDN
	}

	return nil
}
