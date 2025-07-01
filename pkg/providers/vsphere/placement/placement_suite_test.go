// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("Placement", Label(testlabels.VCSim), vcSimPlacement)
}

var suite = builder.NewTestSuite()

func TestPlacement(t *testing.T) {
	suite.Register(t, "VMProvider Placement", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
