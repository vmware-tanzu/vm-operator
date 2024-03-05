// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("Cluster", Label("vcsim"), clusterTests)
	Describe("Folder", Label("vcsim"), folderTests)
	Describe("GetVM", Label("vcsim"), getVMTests)
	Describe("Host", Label("vcsim"), hostTests)
	Describe("ResourcePool", Label("vcsim"), resourcePoolTests)
}

func TestVCenter(t *testing.T) {
	suite.Register(t, "VMProvider VCenter Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
