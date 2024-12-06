// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForControllerWithContext(
	ovfcache.WithContext(
		cource.WithContext(
			pkgcfg.UpdateContext(pkgcfg.NewContextWithDefaultConfig(), func(config *pkgcfg.Config) {
				config.SyncPeriod = 60 * time.Minute
				config.CreateVMRequeueDelay = 1 * time.Second
				config.PoweredOnVMHasIPRequeueDelay = 1 * time.Second
				config.AsyncSignalDisabled = true
				config.AsyncCreateDisabled = true
			}))),
	virtualmachine.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = intgFakeVMProvider
		return nil
	})

func TestVirtualMachine(t *testing.T) {
	suite.Register(t, "VirtualMachine controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(func() {
	virtualmachine.SkipNameValidation = ptr.To(true)
	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
