// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = DescribeTable("StoragePolicyUsageNameFromQuotaName",
	func(s string, expected string) {
		Ω(kubeutil.StoragePolicyUsageNameFromQuotaName(s)).Should(Equal(expected))
	},
	Entry("empty input", "", "-vm-usage"),
	Entry("non-empty-input", "my-quota", "my-quota-vm-usage"),
)
