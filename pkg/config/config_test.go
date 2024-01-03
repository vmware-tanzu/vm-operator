// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
)

var _ = Describe("Config", func() {
	Describe("GetMaxDeployThreadsOnProvider", func() {
		When("MaxDeployThreadsOnProvider == 0", func() {
			It("Should return MaxDeployThreadsOnProvider", func() {
				config := pkgconfig.Config{MaxDeployThreadsOnProvider: 100}
				Expect(config.GetMaxDeployThreadsOnProvider()).To(Equal(100))
			})
		})
		When("MaxDeployThreadsOnProvider > 0", func() {
			It("Should return percentage of MaxConcurrentReconciles / MaxCreateVMsOnProvider", func() {
				config := pkgconfig.Config{
					MaxDeployThreadsOnProvider: 0,
					MaxConcurrentReconciles:    20,
					MaxCreateVMsOnProvider:     80,
				}
				Expect(config.GetMaxDeployThreadsOnProvider()).To(Equal(16))
			})
		})
	})
})
