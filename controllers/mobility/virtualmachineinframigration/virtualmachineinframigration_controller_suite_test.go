// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineinframigration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/controllers/mobility/virtualmachineinframigration"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(config *pkgcfg.Config) {
			config.Features.StoragePolicyMutability = true
		},
	),
	virtualmachineinframigration.AddToManager,
	pkgmgr.InitializeProvidersNoopFn)

func TestVirtualMachineInfraMigrationController(t *testing.T) {
	suite.Register(
		t,
		"VirtualMachineInfraMigration controller suite",
		intgTests,
		unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
