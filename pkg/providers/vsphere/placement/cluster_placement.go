// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// Recommendation is the info about a placement recommendation.
type Recommendation struct {
	PoolMoRef  vimtypes.ManagedObjectReference
	HostMoRef  *vimtypes.ManagedObjectReference
	Datastores []DatastoreResult
}

func relocateSpecToRecommendation(
	ctx context.Context,
	relocateSpec *vimtypes.VirtualMachineRelocateSpec) *Recommendation {

	// Instance Storage requires the host.
	if relocateSpec == nil || relocateSpec.Pool == nil || relocateSpec.Host == nil {
		return nil
	}

	r := Recommendation{
		PoolMoRef: *relocateSpec.Pool,
		HostMoRef: relocateSpec.Host,
	}

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		if ds := relocateSpec.Datastore; ds != nil {
			r.Datastores = append(r.Datastores, DatastoreResult{
				MoRef: *ds,
			})
		}
		for i := range relocateSpec.Disk {
			d := relocateSpec.Disk[i]
			r.Datastores = append(r.Datastores, DatastoreResult{
				MoRef:   d.Datastore,
				ForDisk: true,
				DiskKey: d.DiskId,
			})
		}
	}

	return &r
}

func clusterPlacementActionToRecommendation(
	ctx context.Context,
	finder *find.Finder,
	action vimtypes.ClusterClusterInitialPlacementAction) (*Recommendation, error) {

	r := Recommendation{
		PoolMoRef: action.Pool,
		HostMoRef: action.TargetHost,
	}

	if pkgcfg.FromContext(ctx).Features.FastDeploy {
		if cs := action.ConfigSpec; cs != nil {
			//
			// Get the recommended datastore for the VM.
			//
			if cs.Files != nil {
				if dsn := util.DatastoreNameFromStorageURI(cs.Files.VmPathName); dsn != "" {
					ds, err := finder.Datastore(ctx, dsn)
					if err != nil {
						return nil, fmt.Errorf("failed to get datastore for %q: %w", dsn, err)
					}
					if ds != nil {
						r.Datastores = append(r.Datastores, DatastoreResult{
							Name:  dsn,
							MoRef: ds.Reference(),
						})
					}
				}
			}

			//
			// Get the recommended datastores for each disk.
			//
			for i := range cs.DeviceChange {
				dcs := cs.DeviceChange[i].GetVirtualDeviceConfigSpec()
				if disk, ok := dcs.Device.(*vimtypes.VirtualDisk); ok {
					if bbi, ok := disk.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo); ok {
						if bi := bbi.GetVirtualDeviceFileBackingInfo(); bi.Datastore != nil {
							r.Datastores = append(r.Datastores, DatastoreResult{
								MoRef:   *bi.Datastore,
								ForDisk: true,
								DiskKey: disk.Key,
							})
						}
					}
				}
			}
		}
	}

	return &r, nil
}

func CheckPlacementRelocateSpec(spec *vimtypes.VirtualMachineRelocateSpec) error {
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
	vmCtx pkgctx.VirtualMachineContext,
	res *vimtypes.PlacementResult) *vimtypes.VirtualMachineRelocateSpec {

	for _, r := range res.Recommendations {
		if r.Reason == string(vimtypes.RecommendationReasonCodeXvmotionPlacement) {
			for _, a := range r.Action {
				if pa, ok := a.(*vimtypes.PlacementAction); ok {
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
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
	cluster *object.ClusterComputeResource,
	configSpec vimtypes.VirtualMachineConfigSpec) ([]Recommendation, error) {

	placementSpec := vimtypes.PlacementSpec{
		PlacementType: string(vimtypes.PlacementSpecPlacementTypeCreate),
		ConfigSpec:    &configSpec,
	}

	vmCtx.Logger.V(4).Info("PlaceVMForCreate request", "placementSpec", vimtypes.ToString(placementSpec))

	resp, err := cluster.PlaceVm(vmCtx, placementSpec)
	if err != nil {
		return nil, err
	}

	vmCtx.Logger.V(6).Info("PlaceVMForCreate response", "resp", vimtypes.ToString(resp))

	var recommendations []Recommendation

	for _, r := range resp.Recommendations {
		if r.Reason != string(vimtypes.RecommendationReasonCodeXvmotionPlacement) {
			continue
		}

		for _, a := range r.Action {
			if pa, ok := a.(*vimtypes.PlacementAction); ok {
				if r := relocateSpecToRecommendation(vmCtx, pa.RelocateSpec); r != nil {
					recommendations = append(recommendations, *r)
				}
			}
		}
	}

	return recommendations, nil
}

// ClusterPlaceVMForCreate determines the suitable cluster placement among the specified ResourcePools.
func ClusterPlaceVMForCreate(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	finder *find.Finder,
	resourcePoolsMoRefs []vimtypes.ManagedObjectReference,
	configSpec vimtypes.VirtualMachineConfigSpec,
	needHostPlacement, needDatastorePlacement bool) ([]Recommendation, error) {

	// Work around PlaceVmsXCluster bug that crashes vpxd when ConfigSpec.Files is nil.
	configSpec.Files = new(vimtypes.VirtualMachineFileInfo)

	placementSpec := vimtypes.PlaceVmsXClusterSpec{
		ResourcePools: resourcePoolsMoRefs,
		VmPlacementSpecs: []vimtypes.PlaceVmsXClusterSpecVmPlacementSpec{
			{
				ConfigSpec: configSpec,
			},
		},
		HostRecommRequired:      &needHostPlacement,
		DatastoreRecommRequired: &needDatastorePlacement,
	}

	vmCtx.Logger.V(4).Info("PlaceVmsXCluster request", "placementSpec", vimtypes.ToString(placementSpec))

	resp, err := object.NewRootFolder(vcClient).PlaceVmsXCluster(vmCtx, placementSpec)
	if err != nil {
		return nil, err
	}

	vmCtx.Logger.V(6).Info("PlaceVmsXCluster response", "resp", vimtypes.ToString(resp))

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
		if info.Recommendation.Reason != string(vimtypes.RecommendationReasonCodeXClusterPlacement) {
			continue
		}

		for _, a := range info.Recommendation.Action {
			if ca, ok := a.(*vimtypes.ClusterClusterInitialPlacementAction); ok {
				r, err := clusterPlacementActionToRecommendation(vmCtx, finder, *ca)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to translate placement action to recommendation: %w", err)
				}
				if r != nil {
					recommendations = append(recommendations, *r)
				}
			}
		}
	}

	return recommendations, nil
}
