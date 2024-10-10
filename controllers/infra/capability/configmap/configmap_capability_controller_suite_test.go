// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"

	capability "github.com/vmware-tanzu/vm-operator/controllers/infra/capability/configmap"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/capability/exit"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var numExits int32

func init() {
	exit.Exit = func() {
		atomic.AddInt32(&numExits, 1)
	}
}

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.UpdateContext(
		pkgcfg.NewContext(),
		func(config *pkgcfg.Config) {
			config.Features.SVAsyncUpgrade = true
		},
	),
	capability.AddToManager,
	manager.InitializeProvidersNoopFn,
)

func TestConfigMapCapabilityController(t *testing.T) {
	suite.Register(t, "ConfigMap Capability Controller suite", nil, nil)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
