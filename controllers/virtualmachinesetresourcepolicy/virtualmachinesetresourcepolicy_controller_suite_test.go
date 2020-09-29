// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForController(virtualmachinesetresourcepolicy.AddToManager, builder.AddToContextNoopFn)

func TestVirtualMachine(t *testing.T) {
	suite.Register(t, "VirtualMachineSetResourcePolicy controller suite", nil, unitTests)
}

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}
