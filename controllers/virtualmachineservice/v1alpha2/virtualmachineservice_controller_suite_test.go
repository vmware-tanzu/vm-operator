// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	virtualmachineservice "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForController(
	virtualmachineservice.AddToManager,
	manager.InitializeProvidersNoopFn,
)

func TestVirtualMachineService(t *testing.T) {
	_ = intgTests
	suite.Register(t, "VirtualMachineService controller suite", nil /*intgTests*/, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
