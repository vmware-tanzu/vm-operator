// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
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
	Datastores               []DatastoreResult

	needZonePlacement      bool
	needHostPlacement      bool
	needDatastorePlacement bool
}

type DatastoreResult struct {
	Name        string
	MoRef       vimtypes.ManagedObjectReference
	URL         string
	DiskFormats []string

	// ForDisk is false if the recommendation is for the VM's home directory and
	// true if for a disk. DiskKey is only valid if ForDisk is true.
	ForDisk bool
	DiskKey int32
}

func doesVMNeedPlacement(vmCtx pkgctx.VirtualMachineContext) (res Result) {
	res.ZonePlacement = true

	if zoneName := vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey]; zoneName != "" {
		// Zone has already been selected.
		res.ZoneName = zoneName
	} else {
		// VM does not have a zone already assigned so we need to select one.
		res.needZonePlacement = true
	}

	if pkgcfg.FromContext(vmCtx).Features.InstanceStorage {
		if vmopv1util.IsInstanceStoragePresent(vmCtx.VM) {
			res.InstanceStoragePlacement = true

			if hostMoID := vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]; hostMoID != "" {
				// Host has already been selected.
				res.HostMoRef = &vimtypes.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
			} else {
				// VM has InstanceStorage volumes so we need to select a host.
				res.needHostPlacement = true
			}
		}
	}

	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
		res.needDatastorePlacement = true
	}

	return
}

// lookupChildRPs lookups the child ResourcePool under each parent ResourcePool. A VM with a ResourcePolicy
// may specify a child ResourcePool that the VM will be created under.
func lookupChildRPs(
	ctx context.Context,
	vcClient *vim25.Client,
	rpMoIDs []string,
	zoneName, childRPName string) []string {

	childRPMoIDs := make([]string, 0, len(rpMoIDs))

	for _, rpMoID := range rpMoIDs {
		rp := object.NewResourcePool(vcClient, vimtypes.ManagedObjectReference{
			Type:  string(vimtypes.ManagedObjectTypesResourcePool),
			Value: rpMoID,
		})

		childRP, err := vcenter.GetChildResourcePool(ctx, rp, childRPName)
		if err != nil {
			pkgutil.FromContextOrDefault(ctx).Error(err, "Skipping this resource pool since failed to get child ResourcePool",
				"zone", zoneName, "parentRPMoID", rpMoID, "childRPName", childRPName)
			continue
		}

		childRPMoIDs = append(childRPMoIDs, childRP.Reference().Value)
	}

	return childRPMoIDs
}

// getPlacementCandidates determines the candidate resource pools for VM placement.
func getPlacementCandidates(
	ctx context.Context,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	preAssignedZoneName string,
	namespace string,
	childRPName string) (map[string][]string, map[string]string, error) {

	candidates := map[string][]string{}

	// When FSS_WCP_WORKLOAD_DOMAIN_ISOLATION is enabled, use namespaced Zone CR to get candidate resource pools.
	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		var zones []topologyv1.Zone
		if preAssignedZoneName == "" {
			z, err := topology.GetZones(ctx, client, namespace)
			if err != nil {
				return nil, nil, err
			}
			zones = z
		} else {
			zone, err := topology.GetZone(ctx, client, preAssignedZoneName, namespace)
			if err != nil {
				return nil, nil, err
			}
			zones = append(zones, zone)
		}

		for _, zone := range zones {
			// Filter out the zone that is to be deleted, so we don't have it as a candidate when doing placement.
			if preAssignedZoneName == "" && !zone.DeletionTimestamp.IsZero() {
				continue
			}
			rpMoIDs := zone.Spec.ManagedVMs.PoolMoIDs
			if childRPName != "" {
				childRPMoIDs := lookupChildRPs(ctx, vcClient, rpMoIDs, zone.Name, childRPName)
				if len(childRPMoIDs) == 0 {
					pkgutil.FromContextOrDefault(ctx).Info("Zone had no candidates after looking up children ResourcePools",
						"zone", zone.Name, "rpMoIDs", rpMoIDs, "childRPName", childRPName)
					continue
				}
				rpMoIDs = childRPMoIDs
			}

			candidates[zone.Name] = rpMoIDs
		}

		resourcePoolToZoneName := map[string]string{}
		for zoneName, rpMoIDs := range candidates {
			for _, rpMoID := range rpMoIDs {
				resourcePoolToZoneName[rpMoID] = zoneName
			}
		}

		return candidates, resourcePoolToZoneName, nil
	}

	// When FSS_WCP_WORKLOAD_DOMAIN_ISOLATION is disabled, use cluster scoped AvailabilityZone CR to get candidate resource pools.
	var azs []topologyv1.AvailabilityZone
	if preAssignedZoneName == "" {
		az, err := topology.GetAvailabilityZones(ctx, client)
		if err != nil {
			return nil, nil, err
		}

		azs = az
	} else {
		// Consider candidates only within the already assigned zone.
		// NOTE: GetAvailabilityZone() will return a "default" AZ when WCP_FaultDomains FSS is not enabled.
		az, err := topology.GetAvailabilityZone(ctx, client, preAssignedZoneName)
		if err != nil {
			return nil, nil, err
		}

		azs = append(azs, az)
	}

	for _, az := range azs {
		nsInfo, ok := az.Spec.Namespaces[namespace]
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
			childRPMoIDs := lookupChildRPs(ctx, vcClient, rpMoIDs, az.Name, childRPName)
			if len(childRPMoIDs) == 0 {
				pkgutil.FromContextOrDefault(ctx).Info("AvailabilityZone had no candidates after looking up children ResourcePools",
					"az", az.Name, "rpMoIDs", rpMoIDs, "childRPName", childRPName)
				continue
			}
			rpMoIDs = childRPMoIDs
		}

		candidates[az.Name] = rpMoIDs
	}

	resourcePoolToZoneName := map[string]string{}
	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			resourcePoolToZoneName[rpMoID] = zoneName
		}
	}

	return candidates, resourcePoolToZoneName, nil
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
	configSpec vimtypes.VirtualMachineConfigSpec) []Recommendation {

	var recommendations []Recommendation

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypesResourcePool),
				Value: rpMoID,
			}

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

			recommendations = append(recommendations, recs...)
		}
	}

	vmCtx.Logger.V(5).Info("Non zonal placement recommendations", "recommendations", recommendations)

	return recommendations
}

// getZonalPlacementRecommendations calls DRS PlaceVmsXCluster to determine clusters suitable for placement.
func getZonalPlacementRecommendations(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	finder *find.Finder,
	candidates map[string][]string,
	configSpec vimtypes.VirtualMachineConfigSpec,
	needHostPlacement, needDatastorePlacement bool) ([]Recommendation, error) {

	var candidateRPMoRefs []vimtypes.ManagedObjectReference

	for _, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypesResourcePool),
				Value: rpMoID,
			}
			candidateRPMoRefs = append(candidateRPMoRefs, rpMoRef)
		}
	}

	var recommendations []Recommendation

	if len(candidateRPMoRefs) == 1 {
		// If there is only one candidate, we might be able to skip some work.

		if needHostPlacement || needDatastorePlacement {
			// This is a hack until PlaceVmsXCluster() supports instance storage disks.
			vmCtx.Logger.Info("Falling back into non-zonal placement since the only candidate needs host selected",
				"rpMoID", candidateRPMoRefs[0].Value)
			return getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec), nil
		}

		vmCtx.Logger.V(5).Info("Doing implied placement since there is only one candidate")
		recommendations = append(recommendations, Recommendation{PoolMoRef: candidateRPMoRefs[0]})

	} else {
		clusterRecommendations, err := ClusterPlaceVMForCreate(
			vmCtx,
			vcClient,
			finder,
			candidateRPMoRefs,
			[]vimtypes.VirtualMachineConfigSpec{configSpec},
			needHostPlacement,
			needDatastorePlacement)
		if err != nil {
			vmCtx.Logger.Error(err, "PlaceVmsXCluster failed")
			return nil, err
		}

		recommendations = clusterRecommendations[configSpec.Name]
	}

	vmCtx.Logger.V(5).Info("Placement recommendations", "recommendations", recommendations)
	return recommendations, nil
}

// Placement determines if the VM needs placement, and if so, determines where to place the VM
// and updates the Labels and Annotations with the placement decision.
func Placement(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	configSpec vimtypes.VirtualMachineConfigSpec,
	constraints Constraints) (*Result, error) {

	curResult := doesVMNeedPlacement(vmCtx)
	if !curResult.needZonePlacement &&
		!curResult.needHostPlacement &&
		!curResult.needDatastorePlacement {

		// VM does not require any type of placement, so we can return early.
		return &curResult, nil
	}

	candidates, resourcePoolToZoneName, err := getPlacementCandidates(
		vmCtx,
		client,
		vcClient,
		vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey],
		vmCtx.VM.Namespace,
		constraints.ChildRPName)
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

	var recommendations []Recommendation
	if curResult.needZonePlacement {
		recommendations, err = getZonalPlacementRecommendations(
			vmCtx,
			vcClient,
			finder,
			candidates,
			configSpec,
			curResult.needHostPlacement,
			curResult.needDatastorePlacement)
		if err != nil {
			return nil, fmt.Errorf("failed to get zonal placement recommendations: %w", err)
		}
	} else {
		recommendations = getPlacementRecommendations(vmCtx, vcClient, candidates, configSpec)
	}

	if len(recommendations) == 0 {
		return nil, fmt.Errorf("no placement recommendations available")
	}

	selectedRecommendation := recommendations[rand.Intn(len(recommendations))] // nolint:gosec
	zoneName := resourcePoolToZoneName[selectedRecommendation.PoolMoRef.Value]
	vmCtx.Logger.V(4).Info("Placement recommendation", "zone", zoneName, "recommendation", selectedRecommendation)

	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
		// Get the name and type of the datastores.
		if err := getDatastoreProperties(vmCtx, vcClient, &selectedRecommendation); err != nil {
			return nil, err
		}
	}

	result := Result{
		ZonePlacement:            curResult.needZonePlacement,
		InstanceStoragePlacement: curResult.needHostPlacement,
		ZoneName:                 zoneName,
		PoolMoRef:                selectedRecommendation.PoolMoRef,
		HostMoRef:                selectedRecommendation.HostMoRef,
		Datastores:               selectedRecommendation.Datastores,
	}

	vmCtx.Logger.V(4).Info("Placement result", "result", result)

	return &result, nil
}

func getDatastoreProperties(
	ctx context.Context,
	vcClient *vim25.Client,
	rec *Recommendation) error {

	if len(rec.Datastores) == 0 {
		return nil
	}

	dsRefs := make([]vimtypes.ManagedObjectReference, len(rec.Datastores))
	for i := range rec.Datastores {
		dsRefs[i] = rec.Datastores[i].MoRef
	}

	var moDSs []mo.Datastore
	pc := property.DefaultCollector(vcClient)
	if err := pc.Retrieve(
		ctx,
		dsRefs,
		[]string{
			"info.supportedVDiskFormats",
			"info.url",
			"name",
		},
		&moDSs); err != nil {

		return fmt.Errorf("failed to get datastore properties: %w", err)
	}

	for i := range moDSs {
		moDS := moDSs[i]
		for j := range rec.Datastores {
			ds := &rec.Datastores[j]
			if moDS.Reference() == rec.Datastores[j].MoRef {
				ds.Name = moDS.Name
				dsInfo := moDS.Info.GetDatastoreInfo()
				ds.DiskFormats = dsInfo.SupportedVDiskFormats
				ds.URL = dsInfo.Url
			}
		}
	}

	return nil
}
