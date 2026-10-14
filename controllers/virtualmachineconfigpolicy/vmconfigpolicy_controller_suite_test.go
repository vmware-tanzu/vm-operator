// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineconfigpolicy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineconfigpolicy"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func TestVirtualMachineConfigPolicyController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualMachineConfigPolicy Controller Test Suite")
}

var _ = BeforeSuite(func() {
	virtualmachineconfigpolicy.SkipNameValidation = ptr.To(true)

	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
