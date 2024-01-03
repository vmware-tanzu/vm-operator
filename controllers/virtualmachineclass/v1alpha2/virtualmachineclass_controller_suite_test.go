// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	virtualmachineclass "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass/v1alpha2"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgconfig.UpdateContext(
		pkgconfig.NewContextWithDefaultConfig(),
		func(config *pkgconfig.Config) {
			config.Features.VMOpV1Alpha2 = true
		}),
	virtualmachineclass.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVirtualMachineClass(t *testing.T) {
	suite.Register(t, "VirtualMachineClass controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
