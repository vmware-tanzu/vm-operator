// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules

import (
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

// FindClusterModuleUUID returns the index in the Status.ClusterModules and UUID of the
// VC cluster module for the given groupName and cluster reference.
func FindClusterModuleUUID(
	groupName string,
	clusterRef types.ManagedObjectReference,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (int, string) {

	// Prior to the stretched cluster work, the status did not contain the VC cluster the module was
	// created for, but we still need to return existing modules when the FSS is not enabled. Not
	// being able to provide a name or identifier when creating a cluster module will make this a lot
	// more complicated if we ever have to support stretching an existing setup.
	// TODO: Might just have to plumb default AZ knowledge all the way here.
	matchCluster := lib.IsWcpFaultDomainsFSSEnabled()

	for i, modStatus := range resourcePolicy.Status.ClusterModules {
		if modStatus.GroupName == groupName && (modStatus.ClusterMoID == clusterRef.Value || !matchCluster) {
			return i, modStatus.ModuleUuid
		}
	}

	return -1, ""
}
