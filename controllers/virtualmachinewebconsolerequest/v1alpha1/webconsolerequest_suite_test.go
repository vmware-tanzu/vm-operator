// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProviderA2 = providerfake.NewVMProviderA2()

var suite = builder.NewTestSuiteForController(
	v1alpha1.AddToManager,
	func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProviderA2 = intgFakeVMProviderA2
		return nil
	},
)

func TestWebConsoleRequest(t *testing.T) {
	suite.Register(t, "WebConsoleRequest controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
