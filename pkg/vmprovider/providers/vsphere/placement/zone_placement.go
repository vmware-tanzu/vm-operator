// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	goctx "context"
	"fmt"
	"math/rand"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
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
	var err error

	if zonePlacement {
		zones, err = topology.GetAvailabilityZones(vmCtx, client)
	} else {
		// Consider candidates only within the already assigned zone.
		// NOTE: GetAvailabilityZone() will return a "default" AZ when the FSS is not enabled.
		zone, err := topology.GetAvailabilityZone(vmCtx, client,
			vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey])
		if err == nil {
			zones = append(zones, zone)
		}
	}

	if err != nil {
		return nil, err
	}

	candidates := map[string][]string{}
	for _, zone := range zones {
		nsInfo, ok := zone.Spec.Namespaces[vmCtx.VM.Namespace]
		if ok {
			// NOTE: PoolMoId will be an array soon.
			candidates[zone.Name] = []string{nsInfo.PoolMoId}
		}
	}

	return candidates, nil
}

// PlaceVMCluster() will take resources pools so this will go away.
func rpMoIDToCluster(
	ctx goctx.Context,
	vcClient *vim25.Client,
	rpMoID string) (*object.ClusterComputeResource, error) {

	rp := object.NewResourcePool(vcClient, vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: rpMoID})

	clusterMO, err := rp.Owner(ctx)
	if err != nil {
		return nil, err
	}

	return object.NewClusterComputeResource(vcClient, clusterMO.Reference()), nil
}

// getPlacementRecommendations calls DRS PlaceVM to determine clusters suitable for placement.
func getPlacementRecommendations(
	vmCtx context.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec *vimtypes.VirtualMachineConfigSpec) map[string][]vimtypes.ManagedObjectReference {

	recommendations := map[string][]vimtypes.ManagedObjectReference{}

	for zoneName, rpMoIDs := range candidates {
		// TODO: For PlaceVMCluster() we'll need to pass in the SetResourcePolicy child RPs. Yuck.
		// Currently, HA only has one cluster per zone so simplify.
		rpMoID := rpMoIDs[0]

		cluster, err := rpMoIDToCluster(vmCtx, vcClient, rpMoID)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to get CCR from RP", "rpMoID", rpMoID)
			continue
		}

		hostMoIDs, err := PlaceVMForCreate(vmCtx, cluster, configSpec)
		if err != nil {
			vmCtx.Logger.Error(err, "PlaceVM failed", "cluster", cluster.Reference().Value)
			continue
		}

		if len(hostMoIDs) == 0 {
			vmCtx.Logger.Info("No compatible hosts for placement", "cluster", cluster.Reference().Value)
			continue
		}

		recommendations[zoneName] = hostMoIDs
	}

	vmCtx.Logger.V(5).Info("Placement recommendations", "recommendations", recommendations)

	return recommendations
}

// MakePlacementDecision selects a zone/host for placement.
func MakePlacementDecision(recommendations map[string][]vimtypes.ManagedObjectReference) (string, string) {
	// For now, use first zone returned by map iterator.
	for zoneName, hostMoIDs := range recommendations {
		idx := rand.Intn(len(hostMoIDs)) //nolint:gosec
		hostMoID := hostMoIDs[idx]

		return zoneName, hostMoID.Reference().Value
	}

	return "", ""
}

// Placement determines if the VM needs placement, and if so, determines where to place the VM
// and updates the Labels and Annotations with the placement decision.
func Placement(
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

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

	recommendations := getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)
	if len(recommendations) == 0 {
		return fmt.Errorf("no placement recommendations available")
	}

	zoneName, hostMoID := MakePlacementDecision(recommendations)
	vmCtx.Logger.V(5).Info("Placement decision results", "zone", zoneName, "hostMoID", hostMoID)

	if zonePlacement {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = map[string]string{}
		}
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
	}

	if instanceStoragePlacement {
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
