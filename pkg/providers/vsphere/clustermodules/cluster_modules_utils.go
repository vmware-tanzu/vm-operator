// Copyright (c) 2021-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// FindClusterModuleUUID returns the index in the Status.ClusterModules and UUID of the
// VC cluster module for the given groupName and cluster reference.
func FindClusterModuleUUID(
	ctx context.Context,
	groupName string,
	clusterRef vimtypes.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (int, string) {

	for i, modStatus := range resourcePolicy.Status.ClusterModules {
		if modStatus.GroupName == groupName && (modStatus.ClusterMoID == clusterRef.Value) {
			return i, modStatus.ModuleUuid
		}
	}

	return -1, ""
}

// ClaimClusterModuleUUID tries to find an existing entry in the Status.ClusterModules that is for
// the given groupName and cluster reference. This is meant for after an upgrade where the FaultDomains
// FSS is now enabled but we had not previously set the ClusterMoID.
func ClaimClusterModuleUUID(
	ctx context.Context,
	clusterModProvider Provider,
	groupName string,
	clusterRef vimtypes.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (int, string, error) {

	var errs []error

	for i, modStatus := range resourcePolicy.Status.ClusterModules {
		if modStatus.GroupName == groupName && modStatus.ClusterMoID == "" {
			exists, err := clusterModProvider.DoesModuleExist(ctx, modStatus.ModuleUuid, clusterRef)
			if err != nil {
				errs = append(errs, err)
			} else if exists {
				return i, modStatus.ModuleUuid, nil
			}
		}
	}

	return -1, "", apierrorsutil.NewAggregate(errs)
}
