// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinegrouppublishrequest"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.NewContextWithDefaultConfig(),
	virtualmachinegrouppublishrequest.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = intgFakeVMProvider
		return nil
	})

func TestVirtualMachineGroupPublishRequest(t *testing.T) {
	// TODO: Add intg tests and unit tests
	// suite.Register(t, "VirtualMachineGroupPublishRequest controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
