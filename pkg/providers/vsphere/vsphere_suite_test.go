// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("CPUFreq", cpuFreqTests)
	Describe("InitOvfCacheAndLockPool", initOvfCacheAndLockPoolTests)
	Describe("ResourcePolicyTests", resourcePolicyTests)
	Describe("VirtualMachine", vmTests)
	Describe("VirtualMachineE2E", vmE2ETests)
	Describe("VirtualMachineResize", vmResizeTests)
	Describe("VirtualMachineUtilsTest", vmUtilTests)
}

func TestVSphereProvider(t *testing.T) {
	suite.Register(t, "VMProvider Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
