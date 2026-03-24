// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumeattributesclass_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/storage/volumeattributesclass"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(config *pkgcfg.Config) {
			config.Features.BringYourOwnEncryptionKey = true
		},
	),
	volumeattributesclass.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		return nil
	})

func TestVolumeAttributesClassController(t *testing.T) {
	suite.Register(t, "VolumeAttributesClass controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
