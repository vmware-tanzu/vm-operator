// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/secret"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var provider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.NewContextWithDefaultConfig(),
	secret.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = provider
		return nil
	},
)

func TestVMOpCredentialsSecret(t *testing.T) {
	suite.Register(t, "Infra Secret Controller suite", intgTests, nil)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
