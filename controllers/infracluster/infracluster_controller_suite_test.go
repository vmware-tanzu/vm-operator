// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infracluster_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/infracluster"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProviderA2()

var suite = builder.NewTestSuiteForController(
	infracluster.AddToManager,
	func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProviderA2 = intgFakeVMProvider
		return nil
	},
)

var unitTests = func() {
	Describe("WCP ConfigMap", unitTestsWcpConfig)
}

func TestInfraclusterController(t *testing.T) {
	suite.Register(t, "Infracluster controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
