// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	virtualmachine "github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/v1alpha1"
	ctrlContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	mutationv1a1 "github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1/mutation"
	validationv1a1 "github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1/validation"
)

var intgFakeVMProvider = providerfake.NewVMProvider()

var suite = builder.NewTestSuiteWithOptions(
	builder.TestSuiteOptions{
		InitProviderFn: func(ctx *ctrlContext.ControllerManagerContext, _ ctrlmgr.Manager) error {
			ctx.VMProvider = intgFakeVMProvider
			return nil
		},
		Controllers: []pkgmgr.AddToManagerFunc{virtualmachine.AddToManager},
		MutationWebhooks: []builder.TestSuiteMutationWebhookOptions{
			{
				Name:           "default.mutating.virtualmachine.v1alpha1.vmoperator.vmware.com",
				AddToManagerFn: mutationv1a1.AddToManager,
			},
		},
		ValidationWebhooks: []builder.TestSuiteValidationWebhookOptions{
			{
				Name:           "default.validating.virtualmachine.v1alpha1.vmoperator.vmware.com",
				AddToManagerFn: validationv1a1.AddToManager,
			},
		},
	})

func TestVirtualMachine(t *testing.T) {
	suite.Register(t, "VirtualMachine controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
