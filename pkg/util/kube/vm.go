// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube

const (
	// CAPWClusterRoleLabelKey is the key for the label applied to a VM that was
	// created by CAPW.
	CAPWClusterRoleLabelKey = "capw.vmware.com/cluster.role" //nolint:gosec

	// CAPVClusterRoleLabelKey is the key for the label applied to a VM that was
	// created by CAPV.
	CAPVClusterRoleLabelKey = "capv.vmware.com/cluster.role"
)

func HasTKGLabels(vmLabels map[string]string) bool {
	_, ok := vmLabels[CAPWClusterRoleLabelKey]
	if !ok {
		_, ok = vmLabels[CAPVClusterRoleLabelKey]
	}
	return ok
}
