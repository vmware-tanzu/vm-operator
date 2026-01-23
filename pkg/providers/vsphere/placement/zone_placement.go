// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
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
	InstanceStoragePlacement bool
	ZoneName                 string
	HostMoRef                *vimtypes.ManagedObjectReference
	PoolMoRef                vimtypes.ManagedObjectReference
	Datastores               []DatastoreResult

	needInstanceStoragePlacement bool
	needDatastorePlacement       bool
}

type DatastoreResult struct {
	Name                             string
	MoRef                            vimtypes.ManagedObjectReference
	URL                              string
	DiskFormats                      []string
	TopLevelDirectoryCreateSupported bool

	// ForDisk is false if the recommendation is for the VM's home directory and
	// true if for a disk. DiskKey is only valid if ForDisk is true.
	ForDisk bool
	DiskKey int32
}

var (
	ErrNoPlacementCandidates      = errors.New("no placement candidates")
	ErrNoPlacementRecommendations = errors.New("no placement recommendations")
)

func doesVMNeedPlacement(vmCtx pkgctx.VirtualMachineContext) (res Result) {
	if zoneName := vmCtx.VM.Labels[corev1.LabelTopologyZone]; zoneName != "" {
		// Zone has already been selected.
		res.ZoneName = zoneName
	}

	f := pkgcfg.FromContext(vmCtx).Features
	res.needDatastorePlacement = f.FastDeploy

	if f.InstanceStorage {
		if vmopv1util.IsInstanceStoragePresent(vmCtx.VM) {
			// Note instance storage is not compatible with fast deploy, so the fast
			// deploy feature is disabled within the context of this VM.
			res.InstanceStoragePlacement = true

			if hostMoID := vmCtx.VM.Annotations[constants.InstanceStorageSelectedNodeMOIDAnnotationKey]; hostMoID != "" {
				// Host has already been selected.
				res.HostMoRef = &vimtypes.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
			} else {
				// VM has InstanceStorage volumes so we need to select a host.
				res.needInstanceStoragePlacement = true
			}
		}
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
			Type:  string(vimtypes.ManagedObjectTypeResourcePool),
			Value: rpMoID,
		})

		childRP, err := vcenter.GetChildResourcePool(ctx, rp, childRPName)
		if err != nil {
			pkglog.FromContextOrDefault(ctx).Error(err, "Skipping this resource pool since failed to get child ResourcePool",
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
	childRPName string) (map[string][]string, error) {

	candidates := map[string][]string{}

	// When FSS_WCP_WORKLOAD_DOMAIN_ISOLATION is enabled, use namespaced Zone CR to get candidate resource pools.
	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		var zones []topologyv1.Zone
		if preAssignedZoneName == "" {
			z, err := topology.GetZones(ctx, client, namespace)
			if err != nil {
				return nil, err
			}
			zones = z
		} else {
			zone, err := topology.GetZone(ctx, client, preAssignedZoneName, namespace)
			if err != nil {
				return nil, err
			}
			zones = append(zones, zone)
		}

		for _, zone := range zones {
			// Filter out zone that pending delete, so we don't use it as a candidate.
			if preAssignedZoneName == "" && !zone.DeletionTimestamp.IsZero() {
				continue
			}

			rpMoIDs := zone.Spec.ManagedVMs.PoolMoIDs
			if len(rpMoIDs) == 0 {
				pkglog.FromContextOrDefault(ctx).Info(
					"Skipping candidate zone with no ResourcePool MoIDs", "zone", zone.Name)
				continue
			}

			if childRPName != "" {
				childRPMoIDs := lookupChildRPs(ctx, vcClient, rpMoIDs, zone.Name, childRPName)
				if len(childRPMoIDs) == 0 {
					pkglog.FromContextOrDefault(ctx).Info(
						"Zone had no candidates after looking up children ResourcePools",
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
	if preAssignedZoneName == "" {
		az, err := topology.GetAvailabilityZones(ctx, client)
		if err != nil {
			return nil, err
		}

		azs = az
	} else {
		// Consider candidates only within the already assigned zone.
		// NOTE: GetAvailabilityZone() will return a "default" AZ when WCP_FaultDomains FSS is not enabled.
		az, err := topology.GetAvailabilityZone(ctx, client, preAssignedZoneName)
		if err != nil {
			return nil, err
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
				pkglog.FromContextOrDefault(ctx).Info("AvailabilityZone had no candidates after looking up children ResourcePools",
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

// getPlaceVMRecommendation calls DRS PlaceVM to determine clusters suitable for placement.
func getPlaceVMRecommendation(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	candidates map[string][]string,
	configSpec vimtypes.VirtualMachineConfigSpec) (Recommendation, error) {

	var recommendations []Recommendation
	var errs []error

	for zoneName, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeResourcePool),
				Value: rpMoID,
			}

			cluster, err := rpMoIDToCluster(vmCtx, vcClient, rpMoRef)
			if err != nil {
				errs = append(errs, fmt.Errorf("error getting cluster for zone %s RP %s: %w",
					zoneName, rpMoID, err))
				continue
			}

			rec, err := PlaceVMForCreate(vmCtx, cluster, configSpec)
			if err != nil {
				errs = append(errs, fmt.Errorf("PlaceVM failed for zone %s CCR %s: %w",
					zoneName, cluster.Reference().Value, err))
				continue
			}

			if rec == nil {
				vmCtx.Logger.Info("No placement recommendation", "zone", zoneName,
					"clusterMoID", cluster.Reference().Value, "rpMoID", rpMoID)
				continue
			}

			// Replace the resource pool returned by PlaceVM - that is the cluster's root RP - with
			// our more specific namespace or namespace child RP since this VM needs to be under the
			// more specific RP. This makes the recommendation returned here the same as what zonal
			// would return.
			rec.PoolMoRef = rpMoRef
			recommendations = append(recommendations, *rec)
		}
	}

	err := errors.Join(errs...)
	if err != nil {
		vmCtx.Logger.Error(err, "PlaceVM recommendation errors",
			"recommendations", recommendations, "candidates", candidates)
	}

	if len(recommendations) == 0 {
		if err == nil {
			err = ErrNoPlacementRecommendations
		}
		return Recommendation{}, err
	}

	return recommendations[rand.Intn(len(recommendations))], nil // nolint:gosec
}

func getPlacementRecommendation(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vim25.Client,
	finder *find.Finder,
	candidates map[string][]string,
	configSpec vimtypes.VirtualMachineConfigSpec,
	needsInstanceStoragePlacement, needDatastorePlacement bool) (Recommendation, error) {

	candidateRPMoRefs := make([]vimtypes.ManagedObjectReference, 0, len(candidates))
	for _, rpMoIDs := range candidates {
		for _, rpMoID := range rpMoIDs {
			rpMoRef := vimtypes.ManagedObjectReference{
				Type:  string(vimtypes.ManagedObjectTypeResourcePool),
				Value: rpMoID,
			}
			candidateRPMoRefs = append(candidateRPMoRefs, rpMoRef)
		}
	}

	var recommendation Recommendation

	switch {
	case needsInstanceStoragePlacement:
		// PlaceVmsXCluster does not support the magic instance storage disk ID, so
		// fallback to PlaceVm. Note PlaceVm will always return a host recommendation
		// which is required for instance storage.
		rec, err := getPlaceVMRecommendation(vmCtx, vcClient, candidates, configSpec)
		if err != nil {
			return Recommendation{}, fmt.Errorf("PlaceVM failed: %w", err)
		}
		recommendation = rec

	case len(candidates) > 1 || needDatastorePlacement:
		recs, err := getClusterPlacementRecommendations(
			vmCtx,
			vcClient,
			finder,
			candidateRPMoRefs,
			[]vimtypes.VirtualMachineConfigSpec{configSpec},
			needDatastorePlacement)
		if err != nil {
			return Recommendation{}, fmt.Errorf("PlaceVmsXCluster failed: %w", err)
		}

		rec, ok := recs[configSpec.Name]
		if !ok {
			// This should never happen: PlaceVmsXCluster should return an error.
			return Recommendation{}, fmt.Errorf("no placement recommendation for VM returned")
		}

		recommendation = rec

	default:
		vmCtx.Logger.V(5).Info("Doing implied placement since there is only one candidate")
		recommendation = Recommendation{PoolMoRef: candidateRPMoRefs[0]}
	}

	vmCtx.Logger.V(5).Info("Got placement recommendation", "rec", recommendation)
	return recommendation, nil
}

// Placement determines if the VM needs placement, and if so, calls DRS to determine the
// best placement location.
func Placement(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	finder *find.Finder,
	configSpec vimtypes.VirtualMachineConfigSpec,
	constraints Constraints) (*Result, error) {

	curResult := doesVMNeedPlacement(vmCtx)
	if curResult.ZoneName != "" &&
		!curResult.needDatastorePlacement &&
		!curResult.needInstanceStoragePlacement {
		// VM does not require any type of placement, so we can return early.
		return &curResult, nil
	}

	candidates, err := getPlacementCandidates(
		vmCtx,
		client,
		vcClient,
		curResult.ZoneName,
		vmCtx.VM.Namespace,
		constraints.ChildRPName)
	if err != nil {
		return nil, fmt.Errorf("failed to get placement candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrNoPlacementCandidates
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
			vmCtx.Logger.V(4).Info("Removed candidate zones due to constraints",
				"candidateZones", maps.Keys(candidates), "disallowedZones", disallowedZones)
		}

		if len(allowedCandidates) == 0 {
			return nil, fmt.Errorf("no candidates remaining after applying zone constraints %s: %w",
				strings.Join(constraints.Zones.UnsortedList(), ","), ErrNoPlacementCandidates)
		}

		candidates = allowedCandidates
	}

	recommendation, err := getPlacementRecommendation(
		vmCtx,
		vcClient,
		finder,
		candidates,
		configSpec,
		curResult.needInstanceStoragePlacement,
		curResult.needDatastorePlacement)
	if err != nil {
		return nil, err
	}

	var zoneName string
	for z, rpMoIDs := range candidates {
		if slices.Contains(rpMoIDs, recommendation.PoolMoRef.Value) {
			zoneName = z
			break
		}
	}
	if zoneName == "" {
		// This should never happen: placement returned a non-candidate RP.
		return nil, fmt.Errorf("no zone assignment for ResourcePool %s",
			recommendation.PoolMoRef.Value)
	}

	if curResult.ZoneName != "" && curResult.ZoneName != zoneName {
		return nil, fmt.Errorf("preassigned zone %s was different than recommended zone %s",
			curResult.ZoneName, zoneName)
	}

	if curResult.needDatastorePlacement {
		// Get the name and type of the datastores.
		if err := getDatastoreProperties(vmCtx, vcClient, &recommendation); err != nil {
			return nil, err
		}
	}

	result := Result{
		InstanceStoragePlacement: curResult.InstanceStoragePlacement,
		ZoneName:                 zoneName,
		PoolMoRef:                recommendation.PoolMoRef,
		HostMoRef:                recommendation.HostMoRef,
		Datastores:               recommendation.Datastores,
	}

	vmCtx.Logger.Info("Placement result", "result", result)
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
			"capability.topLevelDirectoryCreateSupported",
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
				if v := moDS.Capability.TopLevelDirectoryCreateSupported; v != nil {
					ds.TopLevelDirectoryCreateSupported = *v
				}
			}
		}
	}

	return nil
}
