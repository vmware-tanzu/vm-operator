// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("vsphere").WithName("placement")

func CheckPlacementRelocateSpec(spec *vimtypes.VirtualMachineRelocateSpec) bool {
	if spec == nil {
		log.Info("RelocateSpec is nil")
		return false
	}
	if spec.Host == nil {
		log.Info("RelocateSpec does not have a host", "relocateSpec", spec)
		return false
	}
	if spec.Pool == nil {
		log.Info("RelocateSpec does not have a resource pool", "relocateSpec", spec)
		return false
	}
	if spec.Datastore == nil {
		log.Info("RelocateSpec does not have a datastore", "relocateSpec", spec)
		return false
	}
	return true
}

func ParseRelocateVMResponse(res *vimtypes.PlacementResult) *vimtypes.VirtualMachineRelocateSpec {
	for _, r := range res.Recommendations {
		if r.Reason == string(vimtypes.RecommendationReasonCodeXvmotionPlacement) {
			for _, a := range r.Action {
				if pa, ok := a.(*vimtypes.PlacementAction); ok {
					if CheckPlacementRelocateSpec(pa.RelocateSpec) {
						return pa.RelocateSpec
					}
				}
			}
		}
	}
	return nil
}

func CloneVMRelocateSpec(
	ctx context.Context,
	cluster *object.ClusterComputeResource,
	vmRef vimtypes.ManagedObjectReference,
	cloneSpec *vimtypes.VirtualMachineCloneSpec) (*vimtypes.VirtualMachineRelocateSpec, error) {

	placementSpec := vimtypes.PlacementSpec{
		PlacementType: string(vimtypes.PlacementSpecPlacementTypeClone),
		CloneSpec:     cloneSpec,
		RelocateSpec:  &cloneSpec.Location,
		CloneName:     cloneSpec.Config.Name,
		Vm:            &vmRef,
	}

	resp, err := cluster.PlaceVm(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	rSpec := ParseRelocateVMResponse(resp)
	if rSpec == nil {
		return nil, fmt.Errorf("no valid placement action")
	}

	return rSpec, nil
}

// PlaceVMForCreate determines the suitable placement for the hosts in the cluster.
func PlaceVMForCreate(
	ctx context.Context,
	cluster *object.ClusterComputeResource,
	configSpec *vimtypes.VirtualMachineConfigSpec) ([]vimtypes.ManagedObjectReference, error) {

	placementSpec := vimtypes.PlacementSpec{
		PlacementType: string(vimtypes.PlacementSpecPlacementTypeCreate),
		ConfigSpec:    configSpec,
	}

	resp, err := cluster.PlaceVm(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	var hostMoIDs []vimtypes.ManagedObjectReference

	for _, r := range resp.Recommendations {
		if r.Reason != string(vimtypes.RecommendationReasonCodeXvmotionPlacement) {
			continue
		}

		for _, a := range r.Action {
			if pa, ok := a.(*vimtypes.PlacementAction); ok {
				// TBD: What response do we actually expect? govc create uses RelocateSpec.
				if pa.RelocateSpec != nil && pa.RelocateSpec.Host != nil {
					hostMoIDs = append(hostMoIDs, *pa.RelocateSpec.Host)
				} else if pa.TargetHost != nil {
					hostMoIDs = append(hostMoIDs, *pa.TargetHost)
				}
			}
		}
	}

	return hostMoIDs, nil
}
