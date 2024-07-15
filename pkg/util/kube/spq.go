// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube

// StoragePolicyUsageNameFromQuotaName returns the name of the
// StoragePolicyUsage resource for a given StoragePolicyQuota resource name.
func StoragePolicyUsageNameFromQuotaName(s string) string {
	return s + "-vm-usage"
}
