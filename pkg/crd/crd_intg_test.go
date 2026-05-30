// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgcrd "github.com/vmware-tanzu/vm-operator/pkg/crd"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

// These tests apply mutated CRD schemas to a real kube-apiserver and verify
// acceptance. The fake-client unit tests cannot catch CEL compilation errors
// since the fake client skips schema validation — only a real apiserver runs
// the CEL type-checker at CRD apply time.
var _ = Describe(
	"Install against real API server",
	Label(testlabels.EnvTest),
	func() {

		It("should accept CRDs with all capabilities disabled", func() {
			ctx := pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features:          pkgcfg.FeatureStates{},
			})
			// The kube-apiserver will reject the CRD if any x-kubernetes-validations
			// entry references a field that was removed from the schema.
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
		})

		It("should accept CRDs with all capabilities enabled", func() {
			ctx := pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features: pkgcfg.FeatureStates{
					WorkloadIPv6:      true,
					TelcoVMServiceAPI: true,
				},
			})
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())
		})
	},
)
