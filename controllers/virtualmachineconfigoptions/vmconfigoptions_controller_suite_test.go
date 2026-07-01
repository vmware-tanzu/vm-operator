// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigoptions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	vmconfigoptions "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineconfigoptions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	cource.WithContext(
		pkgcfg.UpdateContext(
			pkgcfg.NewContextWithDefaultConfig(),
			func(config *pkgcfg.Config) {
				config.Features.VirtualMachineConfigPolicy = true
			},
		),
	),
	vmconfigoptions.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVirtualMachineConfigOptions(t *testing.T) {
	suite.Register(t, "VirtualMachineConfigOptions controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(func() {
	vmconfigoptions.SkipNameValidation = ptr.To(true)
	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
