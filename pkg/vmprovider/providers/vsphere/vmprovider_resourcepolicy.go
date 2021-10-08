// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/clustermodules"
)

// IsVirtualMachineSetResourcePolicyReady checks if the VirtualMachineSetResourcePolicy for the AZ is ready.
func (vs *vSphereVMProvider) IsVirtualMachineSetResourcePolicyReady(
	ctx context.Context,
	availabilityZoneName string,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {

	client, err := vs.GetClient(ctx)
	if err != nil {
		return false, err
	}

	// If the FSS is not enabled and no zone was specified then assume the default zone.
	if !lib.IsWcpFaultDomainsFSSEnabled() {
		if availabilityZoneName == "" {
			availabilityZoneName = topology.DefaultAvailabilityZoneName
		}
	}

	az, err := topology.GetAvailabilityZone(ctx, vs.sessions.KubeClient(), availabilityZoneName)
	if err != nil {
		return false, err
	}

	ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
	if err != nil {
		return false, err
	}

	rpExists, err := ses.DoesResourcePoolExist(ctx, resourcePolicy.Spec.ResourcePool.Name)
	if err != nil {
		return false, err
	}

	folderExists, err := ses.DoesFolderExist(ctx, resourcePolicy.Spec.Folder.Name)
	if err != nil {
		return false, err
	}

	modulesExist, err := vs.doClusterModulesExist(ctx, client.ClusterModuleClient(), ses.Cluster(), resourcePolicy)
	if err != nil {
		return false, err
	}

	if !rpExists || !folderExists || !modulesExist {
		log.V(4).Info("Resource policy is not ready",
			"az", az.Name, "resourcePool", rpExists, "folder", folderExists, "modules", modulesExist)
		return false, nil
	}

	return true, nil
}

// CreateOrUpdateVirtualMachineSetResourcePolicy creates if a VirtualMachineSetResourcePolicy doesn't exist, updates otherwise.
func (vs *vSphereVMProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(
	ctx context.Context,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	client, err := vs.GetClient(ctx)
	if err != nil {
		return err
	}

	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return err
	}

	for _, az := range availabilityZones {
		ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
		if err != nil {
			return err
		}

		rpExists, err := ses.DoesResourcePoolExist(ctx, resourcePolicy.Spec.ResourcePool.Name)
		if err != nil {
			return err
		}

		if !rpExists {
			if _, err = ses.CreateResourcePool(ctx, &resourcePolicy.Spec.ResourcePool); err != nil {
				return err
			}
		} else {
			if err = ses.UpdateResourcePool(ctx, &resourcePolicy.Spec.ResourcePool); err != nil {
				return err
			}
		}

		folderExists, err := ses.DoesFolderExist(ctx, resourcePolicy.Spec.Folder.Name)
		if err != nil {
			return err
		}

		if !folderExists {
			if _, err = ses.CreateFolder(ctx, &resourcePolicy.Spec.Folder); err != nil {
				return err
			}
		}

		err = vs.createClusterModules(ctx, client.ClusterModuleClient(), ses.Cluster(), resourcePolicy)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteVirtualMachineSetResourcePolicy deletes the VirtualMachineSetPolicy.
func (vs *vSphereVMProvider) DeleteVirtualMachineSetResourcePolicy(
	ctx context.Context,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return err
	}

	for _, az := range availabilityZones {
		ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
		if err != nil {
			return err
		}

		if err = ses.DeleteResourcePool(ctx, resourcePolicy.Spec.ResourcePool.Name); err != nil {
			return err
		}

		if err = ses.DeleteFolder(ctx, resourcePolicy.Spec.Folder.Name); err != nil {
			return err
		}
	}

	return vs.deleteClusterModules(ctx, resourcePolicy)
}

// doClusterModulesExist checks whether all the ClusterModules for the given VirtualMachineSetResourcePolicy
// have been created and exist in VC for the Session's Cluster.
func (vs *vSphereVMProvider) doClusterModulesExist(
	ctx context.Context,
	clusterModProvider clustermodules.Provider,
	cluster *object.ClusterComputeResource,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {

	if cluster == nil {
		return false, fmt.Errorf("cluster does not exist")
	}

	clusterRef := cluster.Reference()

	for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
		_, moduleID := clustermodules.FindClusterModuleUUID(moduleSpec.GroupName, clusterRef, resourcePolicy)
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
	cluster *object.ClusterComputeResource,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	if cluster == nil {
		return fmt.Errorf("cluster does not exist")
	}

	clusterRef := cluster.Reference()

	// There is no way to give a name when creating a VC cluster module, so we have to
	// resort to using the status as the source of truth. This can result in orphaned
	// modules if, for instance, we fail to update the resource policy k8s object.
	for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
		idx, moduleID := clustermodules.FindClusterModuleUUID(moduleSpec.GroupName, clusterRef, resourcePolicy)

		if moduleID != "" {
			// Verify this cluster module exists on VC for this cluster.
			exists, err := clusterModProvider.DoesModuleExist(ctx, moduleID, clusterRef)
			if err != nil {
				return err
			}
			if !exists {
				// Status entry is stale. Create below.
				moduleID = ""
			}
		}

		if moduleID == "" {
			var err error
			moduleID, err = clusterModProvider.CreateModule(ctx, clusterRef)
			if err != nil {
				return err
			}
		}

		if idx >= 0 {
			resourcePolicy.Status.ClusterModules[idx].ModuleUuid = moduleID
			resourcePolicy.Status.ClusterModules[idx].ClusterMoID = clusterRef.Value
		} else {
			status := v1alpha1.ClusterModuleStatus{
				GroupName:   moduleSpec.GroupName,
				ModuleUuid:  moduleID,
				ClusterMoID: clusterRef.Value,
			}
			resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules, status)
		}
	}

	return nil
}

// deleteClusterModules deletes all the ClusterModules associated with a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVMProvider) deleteClusterModules(
	ctx context.Context,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	client, err := vs.GetClient(ctx)
	if err != nil {
		return err
	}

	var errModStatus []v1alpha1.ClusterModuleStatus
	var errs []error

	for _, moduleStatus := range resourcePolicy.Status.ClusterModules {
		err := client.ClusterModuleClient().DeleteModule(ctx, moduleStatus.ModuleUuid)
		if err != nil {
			errModStatus = append(errModStatus, moduleStatus)
			errs = append(errs, err)
		}
	}

	resourcePolicy.Status.ClusterModules = errModStatus
	return k8serrors.NewAggregate(errs)
}
