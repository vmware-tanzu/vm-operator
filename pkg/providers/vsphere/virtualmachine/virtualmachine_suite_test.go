// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("ClusterComputeResource", Label(testlabels.VCSim), ccrTests)
	Describe("Delete", Label(testlabels.VCSim), deleteTests)
	Describe("Publish", Label(testlabels.VCSim), publishTests)
	Describe("Backup", Label(testlabels.VCSim), backupTests)
	Describe("GuestInfo", Label(testlabels.VCSim), guestInfoTests)
	Describe("CD-ROM", Label(testlabels.VCSim), cdromTests)
	Describe("Snapshot", Label(testlabels.VCSim), snapShotTests)
	Describe("ExtraConfig", Label(testlabels.VCSim), extraConfigTests)
}

var suite = builder.NewTestSuite()

func TestVirtualMachine(t *testing.T) {
	suite.Register(t, "vSphere Provider VirtualMachine Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
