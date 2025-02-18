// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capability_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	capability "github.com/vmware-tanzu/vm-operator/controllers/infra/capability/configmap"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(config *pkgcfg.Config) {
			config.Features.SVAsyncUpgrade = false
		},
	),
	capability.AddToManager,
	manager.InitializeProvidersNoopFn,
)

func TestConfigMapCapabilityController(t *testing.T) {
	suite.Register(t, "ConfigMap Capability Controller suite", nil, nil)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
