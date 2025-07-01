// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
)

// CreateOrUpdateVirtualMachineSetResourcePolicy creates if a VirtualMachineSetResourcePolicy doesn't exist, updates otherwise.
func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(
	ctx context.Context,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {

	folderMoID, rpMoIDs, err := topology.GetNamespaceFolderAndRPMoIDs(ctx, vs.k8sClient, resourcePolicy.Namespace)
	if err != nil {
		return err
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	vimClient := client.VimClient()
	var errs []error

	if folderName := resourcePolicy.Spec.Folder; folderName != "" {
		if folderMoID != "" {
			_, err = vcenter.CreateFolder(ctx, vimClient, folderMoID, folderName)
			if err != nil {
				errs = append(errs, err)
			}
		} else {
			errs = append(errs, fmt.Errorf("cannot create child folder because Namespace folder not found"))
		}
	}

	for _, rpMoID := range rpMoIDs {
		if rpSpec := &resourcePolicy.Spec.ResourcePool; rpSpec.Name != "" {
			_, err := vcenter.CreateOrUpdateChildResourcePool(ctx, vimClient, rpMoID, rpSpec)
			if err != nil {
				errs = append(errs, err)
			}
		}

		if len(resourcePolicy.Spec.ClusterModuleGroups) > 0 {
			clusterRef, err := vcenter.GetResourcePoolOwnerMoRef(ctx, vimClient, rpMoID)
			if err == nil {
				clusterModuleProvider := clustermodules.NewProvider(client.RestClient())
				err = vs.createClusterModules(ctx, clusterModuleProvider, clusterRef.Reference(), resourcePolicy)
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

// DeleteVirtualMachineSetResourcePolicy deletes the VirtualMachineSetPolicy.
func (vs *vSphereVMProvider) DeleteVirtualMachineSetResourcePolicy(
	ctx context.Context,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {

	folderMoID, rpMoIDs, err := topology.GetNamespaceFolderAndRPMoIDs(ctx, vs.k8sClient, resourcePolicy.Namespace)
	if err != nil {
		return err
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	vimClient := client.VimClient()
	var errs []error

	for _, rpMoID := range rpMoIDs {
		err := vcenter.DeleteChildResourcePool(ctx, vimClient, rpMoID, resourcePolicy.Spec.ResourcePool.Name)
		if err != nil {
			errs = append(errs, err)
		}
	}

	clusterModuleProvider := clustermodules.NewProvider(client.RestClient())
	errs = append(errs, vs.deleteClusterModules(ctx, clusterModuleProvider, resourcePolicy)...)

	if folderName := resourcePolicy.Spec.Folder; folderMoID != "" && folderName != "" {
		if err := vcenter.DeleteChildFolder(ctx, vimClient, folderMoID, folderName); err != nil {
			errs = append(errs, err)
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

// doClusterModulesExist checks whether all the ClusterModules for the given VirtualMachineSetResourcePolicy
// have been created and exist in VC for the Session's Cluster.
func (vs *vSphereVMProvider) doClusterModulesExist(
	ctx context.Context,
	clusterModProvider clustermodules.Provider,
	clusterRef vimtypes.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (bool, error) {

	for _, groupName := range resourcePolicy.Spec.ClusterModuleGroups {
		_, moduleID := clustermodules.FindClusterModuleUUID(ctx, groupName, clusterRef, resourcePolicy)
		if moduleID == "" {
			return false, nil
		}

		exists, err := clusterModProvider.DoesModuleExist(ctx, moduleID, clusterRef)
		if !exists || err != nil {
			return false, err
		}
	}

	return true, nil
}

// createClusterModules creates all the ClusterModules that has not created yet for a
// given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVMProvider) createClusterModules(
	ctx context.Context,
	clusterModProvider clustermodules.Provider,
	clusterRef vimtypes.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {

	var errs []error

	// There is no way to give a name when creating a VC cluster module, so we have to
	// resort to using the status as the source of truth. This can result in orphaned
	// modules if, for instance, we fail to update the resource policy k8s object.
	for _, groupName := range resourcePolicy.Spec.ClusterModuleGroups {
		idx, moduleID := clustermodules.FindClusterModuleUUID(ctx, groupName, clusterRef, resourcePolicy)

		if moduleID != "" {
			// Verify this cluster module exists on VC for this cluster.
			exists, err := clusterModProvider.DoesModuleExist(ctx, moduleID, clusterRef)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if !exists {
				// Status entry is stale. Create below.
				moduleID = ""
			}
		} else {
			var err error
			// See if there is already a module for this cluster but without the ClusterMoID field
			// set that we can claim.
			idx, moduleID, err = clustermodules.ClaimClusterModuleUUID(ctx, clusterModProvider,
				groupName, clusterRef, resourcePolicy)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}

		if moduleID == "" {
			var err error
			moduleID, err = clusterModProvider.CreateModule(ctx, clusterRef)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}

		if idx >= 0 {
			resourcePolicy.Status.ClusterModules[idx].ModuleUuid = moduleID
			resourcePolicy.Status.ClusterModules[idx].ClusterMoID = clusterRef.Value
		} else {
			status := vmopv1.VSphereClusterModuleStatus{
				GroupName:   groupName,
				ModuleUuid:  moduleID,
				ClusterMoID: clusterRef.Value,
			}
			resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules, status)
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

// deleteClusterModules deletes all the ClusterModules associated with a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVMProvider) deleteClusterModules(
	ctx context.Context,
	clusterModProvider clustermodules.Provider,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) []error {

	var errModStatus []vmopv1.VSphereClusterModuleStatus
	var errs []error

	for _, moduleStatus := range resourcePolicy.Status.ClusterModules {
		err := clusterModProvider.DeleteModule(ctx, moduleStatus.ModuleUuid)
		if err != nil {
			errModStatus = append(errModStatus, moduleStatus)
			errs = append(errs, err)
		}
	}

	resourcePolicy.Status.ClusterModules = errModStatus
	return errs
}
