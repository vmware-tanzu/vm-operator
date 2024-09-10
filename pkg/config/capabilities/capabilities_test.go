// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capabilities_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
)

var _ = Describe("UpdateCapabilitiesFeatures", func() {

	var (
		ctx  context.Context
		data map[string]string
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		data = map[string]string{}
	})

	JustBeforeEach(func() {
		capabilities.UpdateCapabilitiesFeatures(ctx, data)
	})

	Context("MultipleCL_For_TKG_Supported", func() {
		BeforeEach(func() {
			Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
			data[capabilities.TKGMultipleCLCapabilityKey] = "true"
		})

		It("Enabled", func() {
			Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
		})
	})

	Context("SVAsyncUpgrade is enabled", func() {

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.SVAsyncUpgrade = true
			})
		})

		Context("Workload_Domain_Isolation_Supported", func() {
			BeforeEach(func() {
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeFalse())
				data[capabilities.WorkloadIsolationCapabilityKey] = "true"
			})

			It("Enabled", func() {
				Expect(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation).To(BeTrue())
			})
		})
	})
})
