// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmconfpolicy "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
)

type vmGroupPlacementArgs struct {
	configSpecs           []vimtypes.VirtualMachineConfigSpec
	childResourcePoolName string
}

const (
	vmKind = "VirtualMachine"
)

var (
	// ErrVMGroupPlacementConfigSpec is the error returned when not all VMs in the group
	// placement were able to get their placement ConfigSpec, so group placement could not
	// be done.
	ErrVMGroupPlacementConfigSpec = errors.New("failed to get all VM placement ConfigSpecs")
)

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
	var errs []error

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
				err := fmt.Errorf("failed to get VM placement prereqs: %w", err)
				setVMPlacementReadyCondErr(grpPlacement.VMGroup, vm, err)
				errs = append(errs, err)
				continue
			}

			if firstVM {
				placementArgs.childResourcePoolName = createArgs.ChildResourcePoolName
				firstVM = false
			} else if placementArgs.childResourcePoolName != createArgs.ChildResourcePoolName {
				// Since PlaceVmsXCluster only takes a single list of candidate RPs, all the VMs
				// must have the same child name.
				err := errors.New("all VMs being placed as group must belong to same child ResourcePool")
				setVMPlacementReadyCondErr(grpPlacement.VMGroup, vm, err)
				errs = append(errs, err)
				continue
			}

			configSpec, err := vs.vmGroupGetVMPlacementConfigSpec(vmCtx, vcClient, createArgs)
			if err != nil {
				err := fmt.Errorf("failed to get VM placement ConfigSpec: %w", err)
				setVMPlacementReadyCondErr(grpPlacement.VMGroup, vm, err)
				errs = append(errs, err)
				continue
			}

			memberStatus := getOrAddVMMemberStatus(grpPlacement.VMGroup, vm)
			pkgcond.MarkFalse(
				memberStatus,
				vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
				"PendingPlacement",
				"")

			placementArgs.configSpecs = append(placementArgs.configSpecs, *configSpec)
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %w", ErrVMGroupPlacementConfigSpec, errors.Join(errs...))
	}

	return placementArgs, nil
}

func setVMPlacementReadyCondErr(
	vmGroup *vmopv1.VirtualMachineGroup,
	vm *vmopv1.VirtualMachine,
	err error) {

	memberStatus := getOrAddVMMemberStatus(vmGroup, vm)
	pkgcond.MarkError(
		memberStatus,
		vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
		"NotReady",
		err)
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
	vcClient *vcclient.Client,
	createArgs *VMCreateArgs) (*vimtypes.VirtualMachineConfigSpec, error) {

	if err := vs.vmCreateGenConfigSpec(vmCtx, createArgs); err != nil {
		return nil, err
	}

	{
		// Partial vmCreateDoPlacement():

		if pkgcfg.FromContext(vmCtx).Features.VSpherePolicies {
			if err := vmconfpolicy.Reconcile(
				pkgctx.WithRestClient(vmCtx, vcClient.RestClient()),
				vs.k8sClient,
				vcClient.Client.VimClient(),
				vmCtx.VM,
				vmCtx.MoVM,
				&createArgs.ConfigSpec); err != nil {

				return nil, fmt.Errorf(
					"failed to reconcile vSphere policies for placement: %w", err)
			}
		}

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

			memberStatus := getOrAddVMMemberStatus(vmGroup, vm)
			memberStatus.Placement = placeResultToGroupMemberPlacement(&result)
			// TODO: Clear this on failure for the root group
			pkgcond.MarkTrue(memberStatus, vmopv1.VirtualMachineGroupMemberConditionPlacementReady)
		}
	}

	return nil
}

func getOrAddVMMemberStatus(
	vmGroup *vmopv1.VirtualMachineGroup,
	vm *vmopv1.VirtualMachine) *vmopv1.VirtualMachineGroupMemberStatus {

	for i, m := range vmGroup.Status.Members {
		if m.Kind == vmKind && m.Name == vm.Name && m.UID == vm.UID {
			return &vmGroup.Status.Members[i]
		}
	}

	vmGroup.Status.Members = append(vmGroup.Status.Members,
		vmopv1.VirtualMachineGroupMemberStatus{
			Name: vm.Name,
			Kind: vmKind,
			UID:  vm.UID,
		},
	)

	return &vmGroup.Status.Members[len(vmGroup.Status.Members)-1]
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
