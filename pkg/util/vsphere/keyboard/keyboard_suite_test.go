// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Command", commandTests)
	Describe("Send", sendTests)
	Describe("Send Commands", Label(testlabels.VCSim), sendVCSimTests)
}

var suite = builder.NewTestSuite()

func TestKeyboard(t *testing.T) {
	suite.Register(t, "vSphere Keyboard Suite", nil, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)
var _ = AfterSuite(suite.AfterSuite)
