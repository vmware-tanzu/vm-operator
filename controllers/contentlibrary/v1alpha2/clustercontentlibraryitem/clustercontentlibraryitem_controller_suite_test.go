// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha2/clustercontentlibraryitem"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProviderA2()

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgconfig.UpdateContext(
		pkgconfig.NewContextWithDefaultConfig(),
		func(config *pkgconfig.Config) {
			config.Features.VMOpV1Alpha2 = true
		}),
	clustercontentlibraryitem.AddToManager,
	func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProviderA2 = intgFakeVMProvider
		return nil
	})

func TestClusterContentLibraryItem(t *testing.T) {
	suite.Register(t, "ClusterContentLibraryItem controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
