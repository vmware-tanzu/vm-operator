// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("VirtualMachine", vmTests)
	Describe("ResourcePolicyTests", resourcePolicyTests)
	Describe("CPUFreq", cpuFreqTests)
}

func TestVSphereProvider(t *testing.T) {
	suite.Register(t, "VMProvider Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
