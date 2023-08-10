// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	volume "github.com/vmware-tanzu/vm-operator/controllers/volume/v1alpha2"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var intgFakeVMProvider = providerfake.NewVMProviderA2()

var suite = builder.NewTestSuiteForControllerWithFSS(
	volume.AddToManager,
	func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
		ctx.VMProviderA2 = intgFakeVMProvider
		return nil
	},
	map[string]bool{lib.VMServiceV1Alpha2FSS: true})

func TestVolume(t *testing.T) {

	suite.Register(t, "Volume controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
