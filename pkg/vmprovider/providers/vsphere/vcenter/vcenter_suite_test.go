// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("Cluster", clusterTests)
	Describe("Host", hostTests)
	Describe("Folder", folderTests)
	Describe("GetVM", getVMTests)
}

func TestVCenter(t *testing.T) {
	suite.Register(t, "VMProvider VCenter Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
