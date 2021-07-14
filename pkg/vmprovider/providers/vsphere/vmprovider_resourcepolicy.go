// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

// DoesVirtualMachineSetResourcePolicyExist checks if the entities of a VirtualMachineSetResourcePolicy exist on vSphere
func (vs *vSphereVmProvider) DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {

	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return false, err
	}

	for _, az := range availabilityZones {
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

		modulesExist, err := vs.DoClusterModulesExist(ctx, resourcePolicy)
		if err != nil {
			return false, err
		}

		if !rpExists || !folderExists || !modulesExist {
			return false, nil
		}
	}

	return true, nil
}

// CreateOrUpdateVirtualMachineSetResourcePolicy creates if a VirtualMachineSetResourcePolicy doesn't exist, updates otherwise.
func (vs *vSphereVmProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
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

		moduleExists, err := vs.DoClusterModulesExist(ctx, resourcePolicy)
		if err != nil {
			return err
		}

		if !moduleExists {
			if err = vs.CreateClusterModules(ctx, resourcePolicy); err != nil {
				return err
			}
		}
	}

	return nil
}

// DeleteVirtualMachineSetResourcePolicy deletes the VirtualMachineSetPolicy.
func (vs *vSphereVmProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
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

		if err = vs.DeleteClusterModules(ctx, resourcePolicy); err != nil {
			return err
		}

	}

	return nil
}

// A helper function to check whether a given clusterModule has been created, and exists in VC.
func isClusterModulePresent(ctx context.Context, session *session.Session, moduleSpec v1alpha1.ClusterModuleSpec, moduleStatuses []v1alpha1.ClusterModuleStatus) (bool, error) {
	for _, module := range moduleStatuses {
		if module.GroupName == moduleSpec.GroupName {
			// If we find a match then we need to see whether the corresponding object exists in VC.
			moduleExists, err := session.DoesClusterModuleExist(ctx, module.ModuleUuid)
			if err != nil {
				return false, err
			}
			return moduleExists, nil
		}
	}
	return false, nil
}

// DoClusterModulesExist checks whether all the ClusterModules for the given VirtualMachineSetResourcePolicy has been
// created and exist in VC.
func (vs *vSphereVmProvider) DoClusterModulesExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return false, err
	}

	for _, az := range availabilityZones {
		ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
		if err != nil {
			return false, err
		}
		for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
			exists, err := isClusterModulePresent(ctx, ses, moduleSpec, resourcePolicy.Status.ClusterModules)
			if err != nil {
				return false, err
			}
			if !exists {
				return false, nil
			}
		}
	}
	return true, nil
}

// A helper function which adds or updates the status of a clusterModule to a given values. If the module with the
// same group name exists, its UUID is updated. Otherwise new ClusterModuleStatus is appended.
func updateOrAddClusterModuleStatus(new v1alpha1.ClusterModuleStatus, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) {
	for idx := range resourcePolicy.Status.ClusterModules {
		if resourcePolicy.Status.ClusterModules[idx].GroupName == new.GroupName {
			resourcePolicy.Status.ClusterModules[idx].ModuleUuid = new.ModuleUuid
			return
		}
	}
	resourcePolicy.Status.ClusterModules = append(resourcePolicy.Status.ClusterModules, new)
}

// CreateClusterModules creates all the ClusterModules that has not created yet for a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVmProvider) CreateClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return err
	}

	for _, az := range availabilityZones {
		ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
		if err != nil {
			return err
		}

		for _, moduleSpec := range resourcePolicy.Spec.ClusterModules {
			exists, err := isClusterModulePresent(ctx, ses, moduleSpec, resourcePolicy.Status.ClusterModules)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			id, err := ses.CreateClusterModule(ctx)
			if err != nil {
				return err
			}
			updateOrAddClusterModuleStatus(v1alpha1.ClusterModuleStatus{GroupName: moduleSpec.GroupName, ModuleUuid: id}, resourcePolicy)
		}
	}
	return nil
}

// DeleteClusterModules deletes all the ClusterModules associated with a given VirtualMachineSetResourcePolicy in VC.
func (vs *vSphereVmProvider) DeleteClusterModules(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	availabilityZones, err := topology.GetAvailabilityZones(ctx, vs.sessions.KubeClient())
	if err != nil {
		return err
	}

	for _, az := range availabilityZones {
		ses, err := vs.sessions.GetSession(ctx, az.Name, resourcePolicy.Namespace)
		if err != nil {
			return err
		}
		i := 0
		for _, moduleStatus := range resourcePolicy.Status.ClusterModules {
			err = ses.DeleteClusterModule(ctx, moduleStatus.ModuleUuid)
			// If the clusterModule has already been deleted, we can ignore the error and proceed.
			if err != nil && !lib.IsNotFoundError(err) {
				break
			}
			i++
		}
		resourcePolicy.Status.ClusterModules = resourcePolicy.Status.ClusterModules[i:]
		if err != nil && !lib.IsNotFoundError(err) {
			return err
		}
	}
	return nil
}
