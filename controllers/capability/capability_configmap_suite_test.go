// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	"testing"

	"github.com/vmware-tanzu/vm-operator/controllers/capability"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var provider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForController(
	capability.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = provider
		return nil
	},
)

func TestCapabilityConfigMapController(t *testing.T) {
	suite.Register(t, "Capability ConfigMap Controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
