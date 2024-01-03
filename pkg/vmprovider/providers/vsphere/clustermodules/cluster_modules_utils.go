// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules

import (
	"context"

	"github.com/vmware/govmomi/vim25/types"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// FindClusterModuleUUID returns the index in the Status.ClusterModules and UUID of the
// VC cluster module for the given groupName and cluster reference.
func FindClusterModuleUUID(
	ctx context.Context,
	groupName string,
	clusterRef types.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (int, string) {

	// Prior to the stretched cluster work, the status did not contain the VC cluster the module was
	// created for, but we still need to return existing modules when the FSS is not enabled.
	matchCluster := pkgconfig.FromContext(ctx).Features.FaultDomains

	for i, modStatus := range resourcePolicy.Status.ClusterModules {
		if modStatus.GroupName == groupName && (modStatus.ClusterMoID == clusterRef.Value || !matchCluster) {
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
	clusterRef types.ManagedObjectReference,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (int, string, error) {

	var errs []error

	if pkgconfig.FromContext(ctx).Features.FaultDomains {
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
	}

	return -1, "", k8serrors.NewAggregate(errs)
}
