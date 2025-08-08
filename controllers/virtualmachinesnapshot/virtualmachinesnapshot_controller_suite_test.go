// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesnapshot_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesnapshot"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	cource.WithContext(
		pkgcfg.NewContextWithDefaultConfig(),
	),
	virtualmachinesnapshot.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVirtualMachineSnapShot(t *testing.T) {
	suite.Register(t, "VirtualMachineSnapshot controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(func() {
	virtualmachinesnapshot.SkipNameValidation = ptr.To(true)
	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
