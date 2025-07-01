// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("Cluster", Label(testlabels.VCSim), clusterTests)
	Describe("Folder", Label(testlabels.VCSim), folderTests)
	Describe("GetVM", Label(testlabels.VCSim), getVMTests)
	Describe("Host", Label(testlabels.VCSim), hostTests)
	Describe("ResourcePool", Label(testlabels.VCSim), resourcePoolTests)
}

func TestVCenter(t *testing.T) {
	suite.Register(t, "VMProvider VCenter Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
