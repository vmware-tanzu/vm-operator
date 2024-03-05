// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustermodules_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("ClusterModules Provider", Label("vcsim"), cmTests)
}

var suite = builder.NewTestSuite()

func TestClusterModules(t *testing.T) {
	suite.Register(t, "vSphere Provider Cluster Modules Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
