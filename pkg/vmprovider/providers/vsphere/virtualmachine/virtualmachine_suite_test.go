// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("ClusterComputeResource", ccrTests)
	Describe("Delete", deleteTests)
	Describe("Publish", publishTests)
	Describe("Backup", backupTests)
}

var suite = builder.NewTestSuite()

func TestClusterModules(t *testing.T) {
	suite.Register(t, "vSphere Provider VirtualMachine Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
