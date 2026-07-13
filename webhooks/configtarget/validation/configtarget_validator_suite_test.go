// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/configtarget/validation"
)

// suite is used for unit and integration testing this webhook.
var suite = builder.NewTestSuiteForValidatingWebhookWithContext(
	pkgcfg.WithConfig(
		pkgcfg.Config{
			Features: pkgcfg.FeatureStates{
				VirtualMachineConfigPolicy: true,
			},
		}),
	validation.AddToManager,
	validation.NewValidator,
	"default.validating.configtarget.v1alpha1.vim.vmware.com")

func TestWebhook(t *testing.T) {
	suite.Register(t, "Validation webhook suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
