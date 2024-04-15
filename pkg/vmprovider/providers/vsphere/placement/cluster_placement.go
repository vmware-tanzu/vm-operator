// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	goctx "context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// Recommendation is the info about a placement recommendation.
type Recommendation struct {
	PoolMoRef types.ManagedObjectReference
	HostMoRef *types.ManagedObjectReference
	// TODO: Datastore, whatever else as we need it.
}

func relocateSpecToRecommendation(relocateSpec *types.VirtualMachineRelocateSpec) *Recommendation {
	// Instance Storage requires the host.
	if relocateSpec == nil || relocateSpec.Pool == nil || relocateSpec.Host == nil {
		return nil
	}

	return &Recommendation{
		PoolMoRef: *relocateSpec.Pool,
		HostMoRef: relocateSpec.Host,
	}
}

func clusterPlacementActionToRecommendation(action types.ClusterClusterInitialPlacementAction) *Recommendation {
	return &Recommendation{
		PoolMoRef: action.Pool,
		HostMoRef: action.TargetHost,
	}
}

func CheckPlacementRelocateSpec(spec *types.VirtualMachineRelocateSpec) error {
	if spec == nil {
		return fmt.Errorf("RelocateSpec is nil")
	}
	if spec.Host == nil {
		return fmt.Errorf("RelocateSpec does not have a host")
	}
	if spec.Pool == nil {
		return fmt.Errorf("RelocateSpec does not have a resource pool")
	}
	if spec.Datastore == nil {
		return fmt.Errorf("RelocateSpec does not have a datastore")
	}
	return nil
}

func ParseRelocateVMResponse(
	vmCtx context.VirtualMachineContext,
	res *types.PlacementResult) *types.VirtualMachineRelocateSpec {

	for _, r := range res.Recommendations {
		if r.Reason == string(types.RecommendationReasonCodeXvmotionPlacement) {
			for _, a := range r.Action {
				if pa, ok := a.(*types.PlacementAction); ok {
					if err := CheckPlacementRelocateSpec(pa.RelocateSpec); err != nil {
						vmCtx.Logger.V(6).Info("Skipped RelocateSpec",
							"reason", err.Error(), "relocateSpec", pa.RelocateSpec)
						continue
					}

					return pa.RelocateSpec
				}
			}
		}
	}

	return nil
}

func CloneVMRelocateSpec(
	vmCtx context.VirtualMachineContext,
	cluster *object.ClusterComputeResource,
	vmRef types.ManagedObjectReference,
	cloneSpec *types.VirtualMachineCloneSpec) (*types.VirtualMachineRelocateSpec, error) {

	placementSpec := types.PlacementSpec{
		PlacementType: string(types.PlacementSpecPlacementTypeClone),
		CloneSpec:     cloneSpec,
		RelocateSpec:  &cloneSpec.Location,
		CloneName:     cloneSpec.Config.Name,
		Vm:            &vmRef,
	}

	resp, err := cluster.PlaceVm(vmCtx, placementSpec)
	if err != nil {
		return nil, err
	}

	rSpec := ParseRelocateVMResponse(vmCtx, resp)
	if rSpec == nil {
		return nil, fmt.Errorf("no valid placement action")
	}

	return rSpec, nil
}

// PlaceVMForCreate determines the suitable placement candidates in the cluster.
func PlaceVMForCreate(
	ctx goctx.Context,
	cluster *object.ClusterComputeResource,
	configSpec types.VirtualMachineConfigSpec) ([]Recommendation, error) {

	placementSpec := types.PlacementSpec{
		PlacementType: string(types.PlacementSpecPlacementTypeCreate),
		ConfigSpec:    &configSpec,
	}

	resp, err := cluster.PlaceVm(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	var recommendations []Recommendation

	for _, r := range resp.Recommendations {
		if r.Reason != string(types.RecommendationReasonCodeXvmotionPlacement) {
			continue
		}

		for _, a := range r.Action {
			if pa, ok := a.(*types.PlacementAction); ok {
				if r := relocateSpecToRecommendation(pa.RelocateSpec); r != nil {
					recommendations = append(recommendations, *r)
				}
			}
		}
	}

	return recommendations, nil
}

// ClusterPlaceVMForCreate determines the suitable cluster placement among the specified ResourcePools.
func ClusterPlaceVMForCreate(
	vmCtx context.VirtualMachineContext,
	vcClient *vim25.Client,
	resourcePoolsMoRefs []types.ManagedObjectReference,
	configSpec types.VirtualMachineConfigSpec,
	needsHost bool) ([]Recommendation, error) {

	// Work around PlaceVmsXCluster bug that crashes vpxd when ConfigSpec.Files is nil.
	configSpec.Files = new(types.VirtualMachineFileInfo)

	placementSpec := types.PlaceVmsXClusterSpec{
		ResourcePools: resourcePoolsMoRefs,
		VmPlacementSpecs: []types.PlaceVmsXClusterSpecVmPlacementSpec{
			{
				ConfigSpec: configSpec,
			},
		},
		HostRecommRequired: &needsHost,
	}

	vmCtx.Logger.V(6).Info("PlaceVmxCluster request", "placementSpec", placementSpec)

	resp, err := object.NewRootFolder(vcClient).PlaceVmsXCluster(vmCtx, placementSpec)
	if err != nil {
		return nil, err
	}

	vmCtx.Logger.V(6).Info("PlaceVmxCluster response", "resp", resp)

	if len(resp.Faults) != 0 {
		var faultMgs []string
		for _, f := range resp.Faults {
			msgs := make([]string, 0, len(f.Faults))
			for _, ff := range f.Faults {
				msgs = append(msgs, ff.LocalizedMessage)
			}
			faultMgs = append(faultMgs,
				fmt.Sprintf("ResourcePool %s faults: %s", f.ResourcePool.Value, strings.Join(msgs, ", ")))
		}
		return nil, fmt.Errorf("PlaceVmsXCluster faults: %v", faultMgs)
	}

	var recommendations []Recommendation

	for _, info := range resp.PlacementInfos {
		if info.Recommendation.Reason != string(types.RecommendationReasonCodeXClusterPlacement) {
			continue
		}

		for _, a := range info.Recommendation.Action {
			if ca, ok := a.(*types.ClusterClusterInitialPlacementAction); ok {
				if r := clusterPlacementActionToRecommendation(*ca); r != nil {
					recommendations = append(recommendations, *r)
				}
			}
		}
	}

	return recommendations, nil
}
