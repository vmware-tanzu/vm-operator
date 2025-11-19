// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type vmGroupPlacementArgs struct {
	configSpecs           []vimtypes.VirtualMachineConfigSpec
	childResourcePoolName string
}

func (vs *vSphereVMProvider) PlaceVirtualMachineGroup(
	ctx context.Context,
	group *vmopv1.VirtualMachineGroup,
	groupPlacements []providers.VMGroupPlacement) error {

	ctx = context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, group, "groupPlacement"))

	vcClient, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	placementArgs, err := vs.vmGroupGetVMPlacementArgs(ctx, vcClient, groupPlacements)
	if err != nil {
		return err
	}

	if len(placementArgs.configSpecs) == 0 {
		return nil
	}

	results, err := vs.vmGroupDoPlacement(ctx, vcClient, group.Namespace, placementArgs)
	if err != nil {
		return err
	}

	if err := applyPlacementResultsToGroups(results, groupPlacements); err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVMProvider) vmGroupGetVMPlacementArgs(
	ctx context.Context,
	vcClient *vcclient.Client,
	groupPlacements []providers.VMGroupPlacement) (*vmGroupPlacementArgs, error) {

	placementArgs := &vmGroupPlacementArgs{}
	firstVM := true

	for _, grpPlacement := range groupPlacements {
		for _, vm := range grpPlacement.VMMembers {
			logger := pkglog.FromContextOrDefault(ctx).WithValues(
				"childGroupName", grpPlacement.VMGroup.Name,
				"vm", vm.Name,
			)

			vmCtx := pkgctx.VirtualMachineContext{
				Context: ctx,
				Logger:  logger,
				VM:      vm,
			}

			createArgs, err := vs.vmGroupGetVMCreatePrereqs(vmCtx, vcClient)
			if err != nil {
				return nil, err
			}

			if firstVM {
				placementArgs.childResourcePoolName = createArgs.ChildResourcePoolName
				firstVM = false
			} else if placementArgs.childResourcePoolName != createArgs.ChildResourcePoolName {
				return nil, fmt.Errorf("all VMs being placed as group must belong to same child ResourcePool")
			}

			configSpec, err := vs.vmGroupGetVMPlacementConfigSpec(vmCtx, createArgs)
			if err != nil {
				return nil, err
			}

			placementArgs.configSpecs = append(placementArgs.configSpecs, *configSpec)
		}
	}

	return placementArgs, nil
}

func (vs *vSphereVMProvider) vmGroupGetVMCreatePrereqs(
	vmCtx pkgctx.VirtualMachineContext,
	vcClient *vcclient.Client) (*VMCreateArgs, error) {

	// This reuses parts of the VM controller driven create VM path to
	// generate the VM's placement ConfigSpec. Later, we should work on
	// reducing the duplication here.

	createArgs := &VMCreateArgs{}

	{
		// Partial vmCreateGetPrereqs():

		if err := vs.vmCreateGetVirtualMachineClass(vmCtx, createArgs); err != nil {
			return nil, err
		}

		if err := vs.vmCreateGetVirtualMachineImage(vmCtx, createArgs); err != nil {
			return nil, err
		}

		if err := vs.vmCreateGetSetResourcePolicy(vmCtx, createArgs); err != nil {
			return nil, err
		}

		if err := vs.vmCreateGetStoragePrereqs(vmCtx, vcClient, createArgs); err != nil {
			return nil, err
		}
	}

	// TODO: Networking, what else?

	return createArgs, nil
}

func (vs *vSphereVMProvider) vmGroupGetVMPlacementConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *VMCreateArgs) (*vimtypes.VirtualMachineConfigSpec, error) {

	if err := vs.vmCreateGenConfigSpec(vmCtx, createArgs); err != nil {
		return nil, err
	}

	{
		// Partial vmCreateDoPlacement():

		placementConfigSpec, err := virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			createArgs.ConfigSpec,
			createArgs.Storage.StorageClassToPolicyID)
		if err != nil {
			return nil, err
		}

		return &placementConfigSpec, nil
	}
}

func (vs *vSphereVMProvider) vmGroupDoPlacement(
	ctx context.Context,
	vcClient *vcclient.Client,
	namespace string,
	placementArgs *vmGroupPlacementArgs) (map[string]placement.Result, error) {

	return placement.GroupPlacement(
		ctx,
		vs.k8sClient,
		vcClient.VimClient(),
		vcClient.Finder(),
		namespace,
		placementArgs.childResourcePoolName,
		placementArgs.configSpecs,
	)
}

func applyPlacementResultsToGroups(
	results map[string]placement.Result,
	groupPlacements []providers.VMGroupPlacement) error {

	for _, grpPlacement := range groupPlacements {
		vmGroup := grpPlacement.VMGroup

		for _, vm := range grpPlacement.VMMembers {
			result, ok := results[vm.Name]
			if !ok {
				return fmt.Errorf("no placement result for VM %s in group %s", vm.Name, vmGroup.Name)
			}

			idx := findVMMemberStatus(vm.Name, vmGroup.Status.Members)
			if idx < 0 {
				m := vmopv1.VirtualMachineGroupMemberStatus{
					Name: vm.Name,
					Kind: "VirtualMachine",
				}
				vmGroup.Status.Members = append(vmGroup.Status.Members, m)
				idx = len(vmGroup.Status.Members) - 1
			}

			vmGroup.Status.Members[idx].Placement = placeResultToGroupMemberPlacement(&result)
			// TODO: Clear this on failure for the root group
			pkgcond.MarkTrue(&vmGroup.Status.Members[idx], vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
		}
	}

	return nil
}

func findVMMemberStatus(vmName string, members []vmopv1.VirtualMachineGroupMemberStatus) int {
	for i := range members {
		if members[i].Name == vmName && members[i].Kind == "VirtualMachine" {
			return i
		}
	}
	return -1
}

func placeResultToGroupMemberPlacement(
	result *placement.Result) *vmopv1.VirtualMachinePlacementStatus {

	placementStatus := &vmopv1.VirtualMachinePlacementStatus{}
	placementStatus.Zone = result.ZoneName
	placementStatus.Pool = result.PoolMoRef.Value

	if result.HostMoRef != nil {
		placementStatus.Node = result.HostMoRef.Value
	}

	for _, ds := range result.Datastores {
		status := vmopv1.VirtualMachineGroupPlacementDatastoreStatus{
			Name:                             ds.Name,
			ID:                               ds.MoRef.Value,
			URL:                              ds.URL,
			SupportedDiskFormats:             ds.DiskFormats,
			TopLevelDirectoryCreateSupported: ds.TopLevelDirectoryCreateSupported,
		}
		if ds.ForDisk {
			status.DiskKey = ptr.To(ds.DiskKey)
		}

		placementStatus.Datastores = append(placementStatus.Datastores, status)
	}

	return placementStatus
}
