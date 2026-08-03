// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configtarget_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/configtarget"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// The manager only registers the ConfigTarget Go type with its runtime
// scheme when this feature is enabled (pkg/manager/manager.go), so the
// suite's integration test client needs it on to (de)serialize
// ConfigTarget at all -- independent of the CRD itself, which envtest
// installs from config/crd/external-crds regardless of this flag.
var suite = builder.NewTestSuiteWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(config *pkgcfg.Config) {
			config.Features.VirtualMachineConfigPolicy = true
		},
	))

func TestConfigTargetController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ConfigTarget Controller Test Suite")
}

var _ = BeforeSuite(func() {
	configtarget.SkipNameValidation = ptr.To(true)

	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
