// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	virtualmachinepublishrequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest/v1alpha1"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteForControllerWithFSS(
	virtualmachinepublishrequest.AddToManager,
	func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProvider = intgFakeVMProvider
		return nil
	},
	map[string]bool{lib.VMImageRegistryFSS: true},
)

func TestVirtualMachinePublishRequest(t *testing.T) {
	suite.Register(t, "VirtualMachinePublishRequest controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
