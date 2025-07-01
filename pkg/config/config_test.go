// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

var _ = Describe("Config", func() {
	Describe("GetMaxDeployThreadsOnProvider", func() {
		When("MaxDeployThreadsOnProvider == 0", func() {
			It("Should return MaxDeployThreadsOnProvider", func() {
				config := pkgcfg.Config{MaxDeployThreadsOnProvider: 100}
				Expect(config.GetMaxDeployThreadsOnProvider()).To(Equal(100))
			})
		})
		When("MaxDeployThreadsOnProvider > 0", func() {
			It("Should return percentage of MaxConcurrentReconciles / MaxCreateVMsOnProvider", func() {
				config := pkgcfg.Config{
					MaxDeployThreadsOnProvider: 0,
					MaxConcurrentReconciles:    20,
					MaxCreateVMsOnProvider:     80,
				}
				Expect(config.GetMaxDeployThreadsOnProvider()).To(Equal(16))
			})
		})
	})
})
