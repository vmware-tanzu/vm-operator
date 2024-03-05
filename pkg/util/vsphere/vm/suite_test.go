// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("Power State", Label("vcsim"), powerStateTests)
	Describe("Hardware Version", Label("vcsim"), hardwareVersionTests)
	Describe("Managed Object", managedObjectTests)
}

var suite = builder.NewTestSuite()

func TestVSphereVirtualMachine(t *testing.T) {
	suite.Register(t, "vSphere VirtualMachine Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
