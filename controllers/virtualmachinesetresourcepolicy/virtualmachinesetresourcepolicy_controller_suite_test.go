// +build !integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForController(
	virtualmachinesetresourcepolicy.AddToManager,
	pkgmgr.InitializeProvidersNoopFn,
)

func TestVirtualMachineSetResourcePolicy(t *testing.T) {
	suite.Register(t, "VirtualMachineSetResourcePolicy controller suite", nil, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
