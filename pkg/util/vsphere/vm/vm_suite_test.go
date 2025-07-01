// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const doesNotExist = "does-not-exist"

func vcSimTests() {
	Describe("Power State", Label(testlabels.VCSim), powerStateTests)
	Describe("Hardware Version", Label(testlabels.VCSim), hardwareVersionTests)
	Describe("Managed Object", managedObjectTests)
	Describe("Guest ID", guestIDTests)
}

var suite = builder.NewTestSuite()

func TestVSphereVirtualMachine(t *testing.T) {
	suite.Register(t, "vSphere VirtualMachine Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
