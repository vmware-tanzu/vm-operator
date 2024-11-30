// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/controllers/storagepolicyquota"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	cource.WithContext(
		pkgcfg.UpdateContext(
			pkgcfg.NewContext(),
			func(config *pkgcfg.Config) {
				config.Features.PodVMOnStretchedSupervisor = true
				config.Features.UnifiedStorageQuota = true
			},
		),
	),
	storagepolicyquota.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestStoragePolicyQuota(t *testing.T) {
	suite.Register(t, "StoragePolicyQuota controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
