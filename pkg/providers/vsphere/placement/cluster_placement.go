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
			homeDS, err := recommendedHomeDatastore(ctx, finder, cs)
			if err != nil {
				return Recommendation{}, err
			}
			if homeDS != nil {
				r.Datastores = append(r.Datastores, *homeDS)
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

// recommendedHomeDatastore returns the recommended datastore for the VM's home:
// the datastore DRS recommended for the first disk in the ConfigSpec that DRS
// placed itself. Only if DRS placed none, because every disk already exists and
// stays where it is, is the datastore of ConfigSpec.Files.VmPathName used.
//
// That is where DRS means to put the VM's home too, which is why
// CreateConfigSpecForPlacement leads with a disk that carries the VM's own
// storage policy. However, DRS sets VmPathName from the first entry of its
// recommendation after regrouping the entries by datastore in an arbitrary
// order. When the VM's disks span several datastores, for example an image disk
// on a shared datastore and a PVC on a host-local one, VmPathName can therefore
// name a datastore that does not comply with the VM's storage policy.
func recommendedHomeDatastore(
	ctx context.Context,
	finder *find.Finder,
	cs *vimtypes.VirtualMachineConfigSpec) (*DatastoreResult, error) {

	var vmPathDSName string
	if cs.Files != nil {
		vmPathDSName = pkgutil.DatastoreNameFromStorageURI(cs.Files.VmPathName)
	}

	if ds := firstPlacedDiskDatastore(cs); ds != nil {
		if ds.Name != "" && ds.Name != vmPathDSName {
			pkglog.FromContextOrDefault(ctx).Info(
				"Using the datastore of the first disk placed by DRS for the VM's home",
				"datastore", ds.Name,
				"vmPathNameDatastore", vmPathDSName)
		}
		return ds, nil
	}

	if vmPathDSName == "" {
		return nil, nil
	}

	ds, err := finder.Datastore(ctx, vmPathDSName)
	if err != nil {
		return nil, fmt.Errorf("failed to get datastore for %q: %w", vmPathDSName, err)
	}

	return &DatastoreResult{
		Name:  vmPathDSName,
		MoRef: ds.Reference(),
	}, nil
}

// firstPlacedDiskDatastore returns the datastore DRS recommended for the first
// disk in the ConfigSpec that DRS placed itself, or nil if there is none.
func firstPlacedDiskDatastore(cs *vimtypes.VirtualMachineConfigSpec) *DatastoreResult {
	for i := range cs.DeviceChange {
		if ds := placedDiskDatastore(cs.DeviceChange[i]); ds != nil {
			return ds
		}
	}
	return nil
}

// placedDiskDatastore returns the datastore DRS recommended for the given
// device if it is a disk DRS placed itself, or nil otherwise. A disk DRS placed
// is still to be created, and its backing refers to the recommended datastore.
// An existing disk keeps its full path and has no file operation. The
// datastore's name is filled in later from its MoRef, along with its other
// properties, so the name here is only what the backing's file name gives.
func placedDiskDatastore(bdc vimtypes.BaseVirtualDeviceConfigSpec) *DatastoreResult {
	dc := bdc.GetVirtualDeviceConfigSpec()
	if dc.FileOperation != vimtypes.VirtualDeviceConfigSpecFileOperationCreate {
		return nil
	}

	disk, ok := dc.Device.(*vimtypes.VirtualDisk)
	if !ok {
		return nil
	}

	bbi, ok := disk.Backing.(vimtypes.BaseVirtualDeviceFileBackingInfo)
	if !ok {
		return nil
	}

	bi := bbi.GetVirtualDeviceFileBackingInfo()
	if bi.Datastore == nil {
		return nil
	}

	return &DatastoreResult{
		Name:  pkgutil.DatastoreNameFromStorageURI(bi.FileName),
		MoRef: *bi.Datastore,
	}
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
	for i := range placementSpec.VmPlacementSpecs {
		cs := placementSpec.VmPlacementSpecs[i].ConfigSpec
		logger.Info("PlaceVmsXCluster request tag specs and placement policies",
			"vmName", cs.Name,
			"tagSpecs", vimtypes.ToString(cs.TagSpecs),
			"vmPlacementPolicies", vimtypes.ToString(cs.VmPlacementPolicies))
	}

	results, err := object.NewRootFolder(vcClient).PlaceVmsXCluster(ctx, placementSpec)
	if err != nil {
		return nil, err
	}

	logger.V(6).Info("PlaceVmsXCluster response", "results", vimtypes.ToString(results))
	for _, info := range results.PlacementInfos {
		logger.Info("PlaceVmsXCluster response recommendation",
			"vmName", info.VmName,
			"recommendation", vimtypes.ToString(info.Recommendation))
	}
	if len(results.Faults) != 0 {
		logger.Info("PlaceVmsXCluster response faults",
			"faults", vimtypes.ToString(results.Faults))
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
