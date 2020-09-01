// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForController(virtualmachine.AddToManager, builder.AddToContextNoopFn)

func TestVirtualMachine(t *testing.T) {
	suite.Register(t, "Virtualmachine controller suite", nil, unitTests)
}

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}
