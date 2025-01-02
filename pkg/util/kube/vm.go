// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

// HasCAPILabels returns true if the VM has a label indicating it was created by
// Cluster API such as CAPW or CAPV.
func HasCAPILabels(vmLabels map[string]string) bool {
	_, hasCAPWLabel := vmLabels[CAPWClusterRoleLabelKey]
	_, hasCAPVLabel := vmLabels[CAPVClusterRoleLabelKey]

	return hasCAPWLabel || hasCAPVLabel
}
