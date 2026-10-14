// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	faultutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/fault"
)

// Recommendation is the info about a placement recommendation.
type Recommendation struct {
	PoolMoRef  vimtypes.ManagedObjectReference
	HostMoRef  *vimtypes.ManagedObjectReference
	Datastores []DatastoreResult
}

// PlaceVMsXClusterErrors preserves the per-VM faults returned by PlaceVmsXCluster.
type PlaceVMsXClusterErrors struct {
	FaultsByVM map[string][]vimtypes.PlaceVmsXClusterResultPlacementFaults
}

// NewPlaceVMsXClusterErrors groups placement faults by VM name.
func NewPlaceVMsXClusterErrors(faults []vimtypes.PlaceVmsXClusterResultPlacementFaults) *PlaceVMsXClusterErrors {
	e := &PlaceVMsXClusterErrors{
		FaultsByVM: make(map[string][]vimtypes.PlaceVmsXClusterResultPlacementFaults),
	}
	for _, pf := range faults {
		e.FaultsByVM[pf.VmName] = append(e.FaultsByVM[pf.VmName], pf)
	}
	return e
}

// Error returns all placement fault details grouped by VM for logging.
func (e *PlaceVMsXClusterErrors) Error() string {
	messages := make([]string, 0, len(e.FaultsByVM))
	for vmName, faults := range e.FaultsByVM {
		poolMessages := make([]string, 0, len(faults))
		for _, pf := range faults {
			poolMessages = append(poolMessages, placementFaultMessage(pf))
		}
		slices.Sort(poolMessages)
		messages = append(messages, fmt.Sprintf("VM %s: [%s]",
			vmName, strings.Join(poolMessages, "; ")))
	}
	slices.Sort(messages)
	return fmt.Sprintf("%v", messages)
}

// HasFaultForVM reports whether any placement faults were reported for vmName.
func (e *PlaceVMsXClusterErrors) HasFaultForVM(vmName string) bool {
	return len(e.FaultsByVM[vmName]) > 0
}

// ErrorForVM returns only the faults for vmName, or nil if none were reported.
// The message omits the VM name because the member condition identifies the VM.
func (e *PlaceVMsXClusterErrors) ErrorForVM(vmName string) error {
	faults := e.FaultsByVM[vmName]
	if len(faults) == 0 {
		return nil
	}
	messages := make([]string, 0, len(faults))
	for _, pf := range faults {
		messages = append(messages, placementFaultMessage(pf))
	}
	slices.Sort(messages)
	return errors.New(strings.Join(messages, "; "))
}

func placementFaultMessage(pf vimtypes.PlaceVmsXClusterResultPlacementFaults) string {
	leafMessages := faultutil.LocalizedMessagesFromFaults(pf.Faults)
	if len(leafMessages) == 0 {
		leafMessages = []string{"placement failed without fault details"}
	}
	return fmt.Sprintf("resource pool (%s) has faults: %s",
		pf.ResourcePool.Value, strings.Join(leafMessages, " "))
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
	action *vimtypes.ClusterClusterInitialPlacementAction) (Recommendation, error) {

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
				if dsn := pkgutil.DatastoreNameFromStorageURI(cs.Files.VmPathName); dsn != "" {
					ds, err := finder.Datastore(ctx, dsn)
					if err != nil {
						return Recommendation{}, fmt.Errorf("failed to get datastore for %q: %w", dsn, err)
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

	return r, nil
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

// PlaceVMForCreate determines the suitable placement candidate in the cluster.
func PlaceVMForCreate(
	vmCtx pkgctx.VirtualMachineContext,
	cluster *object.ClusterComputeResource,
	configSpec vimtypes.VirtualMachineConfigSpec) (*Recommendation, error) {

	placementSpec := vimtypes.PlacementSpec{
		PlacementType: string(vimtypes.PlacementSpecPlacementTypeCreate),
		ConfigSpec:    &configSpec,
	}

	vmCtx.Logger.V(4).Info("PlaceVM request", "placementSpec", vimtypes.ToString(placementSpec))

	resp, err := cluster.PlaceVm(vmCtx, placementSpec)
	if err != nil {
		return nil, err
	}

	vmCtx.Logger.V(4).Info("PlaceVM response", "placementResult", vimtypes.ToString(resp))

	for _, r := range resp.Recommendations {
		if r.Reason != string(vimtypes.RecommendationReasonCodeXvmotionPlacement) {
			continue
		}

		for _, a := range r.Action {
			if pa, ok := a.(*vimtypes.PlacementAction); ok {
				if rec := relocateSpecToRecommendation(vmCtx, pa.RelocateSpec); rec != nil {
					return rec, nil
				}
			}
		}
	}

	return nil, nil
}

// VMPlacementInput maps a VM's ConfigSpec to the zone it is to be placed in.
// An empty ZoneName means the VM does not have pinned zone.
type VMPlacementInput struct {
	ConfigSpec vimtypes.VirtualMachineConfigSpec
	ZoneName   string
}

// getClusterPlacementRecommendations calls PlaceVmsXCluster to get the placement
// recommendations for the given VMs. Each entry's ZoneName is sent as the
// VM's CandidateVsphereZone in the PlaceVmsXCluster call only when per-VM
// zones are enabled.
func getClusterPlacementRecommendations(
	ctx context.Context,
	vcClient *vim25.Client,
	finder *find.Finder,
	resourcePoolsMoRefs []vimtypes.ManagedObjectReference,
	vmPlacementInputs []VMPlacementInput,
	needHostPlacement, needDatastorePlacement bool) (map[string]Recommendation, error) {

	logger := pkglog.FromContextOrDefault(ctx)
	placementSpec := vimtypes.PlaceVmsXClusterSpec{
		PlacementType:           string(vimtypes.PlaceVmsXClusterSpecPlacementTypeCreateAndPowerOn),
		ResourcePools:           resourcePoolsMoRefs,
		VmPlacementSpecs:        make([]vimtypes.PlaceVmsXClusterSpecVmPlacementSpec, len(vmPlacementInputs)),
		HostRecommRequired:      &needHostPlacement,
		DatastoreRecommRequired: &needDatastorePlacement,
	}

	perVMZones := pkgcfg.FromContext(ctx).Features.VMHardAffinityDuringExecution
	for i, m := range vmPlacementInputs {
		cs := m.ConfigSpec
		// Work around PlaceVmsXCluster bug that crashes vpxd when ConfigSpec.Files is nil (still needed?)
		cs.Files = new(vimtypes.VirtualMachineFileInfo)

		placementSpec.VmPlacementSpecs[i].ConfigSpec = cs
		if perVMZones {
			placementSpec.VmPlacementSpecs[i].CandidateVsphereZone = m.ZoneName
		}
	}

	logger.V(4).Info("PlaceVmsXCluster request", "spec", vimtypes.ToString(placementSpec))

	results, err := object.NewRootFolder(vcClient).PlaceVmsXCluster(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	logger.V(6).Info("PlaceVmsXCluster response", "results", vimtypes.ToString(results))

	if len(results.Faults) != 0 {
		return nil, NewPlaceVMsXClusterErrors(results.Faults)
	}

	recommendations := make(map[string]Recommendation, len(results.PlacementInfos))

	for _, info := range results.PlacementInfos {
		if info.Recommendation.Reason != string(vimtypes.RecommendationReasonCodeXClusterPlacement) {
			continue
		}

		for _, a := range info.Recommendation.Action {
			// There should be only one InitialPlacementAction per VM.
			if bca, ok := a.(vimtypes.BaseClusterClusterInitialPlacementAction); ok {
				ca := bca.GetClusterClusterInitialPlacementAction()
				r, err := clusterPlacementActionToRecommendation(ctx, finder, ca)
				if err != nil {
					return nil, fmt.Errorf("failed to translate placement action to recommendation: %w", err)
				}
				recommendations[info.VmName] = r
				break
			}
		}
	}

	return recommendations, nil
}
