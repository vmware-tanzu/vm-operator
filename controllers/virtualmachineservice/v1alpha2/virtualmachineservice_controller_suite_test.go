// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	virtualmachineservice "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2"
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
	virtualmachineservice.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVirtualMachineService(t *testing.T) {
	suite.Register(t, "VirtualMachineService controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
