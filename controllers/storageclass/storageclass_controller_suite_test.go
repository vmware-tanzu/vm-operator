// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storageclass_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/storageclass"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const myEncryptedStoragePolicy = "my-encrypted-storage-policy"

var intgFakeVMProvider = providerfake.NewVMProvider()

func init() {
	intgFakeVMProvider.DoesProfileSupportEncryptionFn = func(
		ctx context.Context,
		profileID string) (bool, error) {

		return profileID == "my-encrypted-storage-policy", nil
	}
}

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(config *pkgcfg.Config) {
			config.Features.BringYourOwnEncryptionKey = true
		},
	),
	storageclass.AddToManager,
	func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = intgFakeVMProvider
		return nil
	})

func TestStorageClassController(t *testing.T) {
	suite.Register(t, "StorageClass controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
