// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	virtualmachineclass "github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForController(
	virtualmachineclass.AddToManager,
	manager.InitializeProvidersNoopFn,
)

func TestVirtualMachineClass(t *testing.T) {
	suite.Register(t, "VirtualMachineClass controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
