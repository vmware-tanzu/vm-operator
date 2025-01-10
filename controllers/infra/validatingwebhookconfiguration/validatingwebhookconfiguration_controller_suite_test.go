// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validatingwebhookconfiguration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/validatingwebhookconfiguration"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	cource.WithContext(
		pkgcfg.UpdateContext(
			pkgcfg.NewContextWithDefaultConfig(),
			func(config *pkgcfg.Config) {
				config.Features.PodVMOnStretchedSupervisor = true
			},
		),
	),
	validatingwebhookconfiguration.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestWebhookConfiguration(t *testing.T) {
	suite.Register(t, "ValidatingWebhookConfiguration controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
