/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/klog"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimTypes "github.com/vmware/govmomi/vim25/types"
)

//GetResourcePoolOwner - Get owner (i.e. parent cluster) of the resource pool
func GetResourcePoolOwner(ctx context.Context, rp *object.ResourcePool) (*object.ClusterComputeResource, error) {
	var owner mo.ResourcePool
	err := rp.Properties(ctx, rp.Reference(), []string{"owner"}, &owner)
	if err != nil {
		klog.Errorf("Failed to retrieve owner of %v: %v", rp.Reference(), err)
		return nil, err
	}
	if owner.Owner.Type != "ClusterComputeResource" {
		return nil, fmt.Errorf("owner of the ResourcePool is not a cluster")
	}
	return object.NewClusterComputeResource(rp.Client(), owner.Owner), nil
}

func CheckPlacementRelocateSpec(spec *vimTypes.VirtualMachineRelocateSpec) bool {
	if spec == nil {
		klog.Warning("RelocateSpec is nil")
		return false
	}
	if spec.Host == nil {
		klog.Warningf("RelocateSpec does not have a host: %#v", spec)
		return false
	}
	if spec.Pool == nil {
		klog.Warningf("RelocateSpec does not have a resource pool: %#v", spec)
		return false
	}
	if spec.Datastore == nil {
		klog.Warningf("RelocateSpec does not have a datastore: %#v", spec)
		return false
	}
	return true
}

func ParsePlaceVmResponse(res *vimTypes.PlacementResult) *vimTypes.VirtualMachineRelocateSpec {
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

func computeVMPlacement(ctx context.Context, cls *object.ClusterComputeResource, vmRef *vimTypes.ManagedObjectReference,
	spec interface{}, placementType vimTypes.PlacementSpecPlacementType) (*vimTypes.VirtualMachineRelocateSpec, error) {
	ps := vimTypes.PlacementSpec{PlacementType: string(placementType)}
	switch placementType {
	case vimTypes.PlacementSpecPlacementTypeClone:
		cloneSpec := spec.(*vimTypes.VirtualMachineCloneSpec)
		ps.CloneSpec = cloneSpec
		ps.RelocateSpec = &cloneSpec.Location
		ps.CloneName = cloneSpec.Config.Name
		ps.Vm = vmRef
	case vimTypes.PlacementSpecPlacementTypeCreate:
		configSpec := spec.(*vimTypes.VirtualMachineConfigSpec)
		ps.ConfigSpec = configSpec
	default:
		return nil, fmt.Errorf("unsupported placement type: %s", string(placementType))
	}

	res, err := cls.PlaceVm(ctx, ps)
	if err != nil {
		return nil, err
	}
	rSpec := ParsePlaceVmResponse(res)
	if rSpec == nil {
		return nil, fmt.Errorf("no valid placement action")
	}

	return rSpec, nil
}

func isNilPtr(i interface{}) bool {
	if i != nil {
		return (reflect.ValueOf(i).Kind() == reflect.Ptr) && (reflect.ValueOf(i).IsNil())
	}
	return true
}
