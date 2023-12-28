// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package placement_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("Placement", vcSimPlacement)
}

var suite = builder.NewTestSuite()

func TestPlacement(t *testing.T) {
	suite.Register(t, "VMProvider Placement", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
