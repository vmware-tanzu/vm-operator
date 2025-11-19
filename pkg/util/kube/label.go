// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"strings"
)

const (
	// CAPWClusterRoleLabelKey is the key for the label applied to a VM that was
	// created by CAPW.
	CAPWClusterRoleLabelKey = "capw.vmware.com/cluster.role" //nolint:gosec

	// CAPVClusterRoleLabelKey is the key for the label applied to a VM that was
	// created by CAPV.
	CAPVClusterRoleLabelKey = "capv.vmware.com/cluster.role"

	// VMOperatorLabelDomain is the domain for VM Operator managed labels.
	VMOperatorLabelDomain = "vmoperator.vmware.com"
)

// HasCAPILabels returns true if the VM has a label indicating it was created by
// Cluster API such as CAPW or CAPV.
func HasCAPILabels(labels map[string]string) bool {
	_, hasCAPWLabel := labels[CAPWClusterRoleLabelKey]
	_, hasCAPVLabel := labels[CAPVClusterRoleLabelKey]

	return hasCAPWLabel || hasCAPVLabel
}

// HasVMOperatorLabels returns true if the given labels map has a label key
// containing the "vmoperator.vmware.com" domain.
func HasVMOperatorLabels(labels map[string]string) bool {
	for k := range labels {
		if strings.Contains(k, VMOperatorLabelDomain) {
			return true
		}
	}

	return false
}

// RemoveVMOperatorLabels returns a copy of the labels map with all VM Operator
// managed labels removed. VM Operator managed labels are identified with the
// "vmoperator.vmware.com" domain in their key.
func RemoveVMOperatorLabels(labels map[string]string) map[string]string {
	filteredLabels := make(map[string]string)

	for k, v := range labels {
		if !strings.Contains(k, VMOperatorLabelDomain) {
			filteredLabels[k] = v
		}
	}

	return filteredLabels
}
