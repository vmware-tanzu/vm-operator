// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.NewContextWithDefaultConfig(),
	addToMgr,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = intgFakeVMProvider
		return nil
	})

func addToMgr(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	// TODO(BMV): Here until we centralize all indexer func registration. In production,
	// this is done in the VirtualMachine controller.
	if err := kubeutil.IndexVMSpecVolumesPVCs(ctx, mgr); err != nil {
		return err
	}

	return volumebatch.AddToManager(ctx, mgr)
}

func TestBatchVolume(t *testing.T) {
	suite.Register(t, "Volume batch controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
