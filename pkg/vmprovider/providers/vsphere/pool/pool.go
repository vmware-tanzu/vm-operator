// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package pool

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("vsphere").WithName("pool")

// GetResourcePoolOwner - Get owner (i.e. parent cluster) of the resource pool.
func GetResourcePoolOwner(ctx context.Context, rp *object.ResourcePool) (*object.ClusterComputeResource, error) {
	var owner mo.ResourcePool
	err := rp.Properties(ctx, rp.Reference(), []string{"owner"}, &owner)
	if err != nil {
		log.Error(err, "Failed to retrieve owner of", "resourcePool", rp.Reference())
		return nil, err
	}
	if owner.Owner.Type != "ClusterComputeResource" {
		return nil, fmt.Errorf("owner of the ResourcePool is not a cluster")
	}
	return object.NewClusterComputeResource(rp.Client(), owner.Owner), nil
}

func CheckPlacementRelocateSpec(spec *vimTypes.VirtualMachineRelocateSpec) bool {
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

func ParsePlaceVMResponse(res *vimTypes.PlacementResult) *vimTypes.VirtualMachineRelocateSpec {
	for _, r := range res.Recommendations {
		if r.Reason == string(vimTypes.RecommendationReasonCodeXvmotionPlacement) {
			for _, a := range r.Action {
				if pa, ok := a.(*vimTypes.PlacementAction); ok {
					if CheckPlacementRelocateSpec(pa.RelocateSpec) {
						return pa.RelocateSpec
					}
				}
			}
		}
	}
	return nil
}

func placeVM(
	ctx context.Context,
	cluster *object.ClusterComputeResource,
	placementSpec vimTypes.PlacementSpec) (*vimTypes.VirtualMachineRelocateSpec, error) {

	res, err := cluster.PlaceVm(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	rSpec := ParsePlaceVMResponse(res)
	if rSpec == nil {
		return nil, fmt.Errorf("no valid placement action")
	}

	return rSpec, nil
}

func CloneVMRelocateSpec(
	ctx context.Context,
	cluster *object.ClusterComputeResource,
	vmRef vimTypes.ManagedObjectReference,
	cloneSpec *vimTypes.VirtualMachineCloneSpec) (*vimTypes.VirtualMachineRelocateSpec, error) {

	placementSpec := vimTypes.PlacementSpec{
		PlacementType: string(vimTypes.PlacementSpecPlacementTypeClone),
		CloneSpec:     cloneSpec,
		RelocateSpec:  &cloneSpec.Location,
		CloneName:     cloneSpec.Config.Name,
		Vm:            &vmRef,
	}

	return placeVM(ctx, cluster, placementSpec)
}
