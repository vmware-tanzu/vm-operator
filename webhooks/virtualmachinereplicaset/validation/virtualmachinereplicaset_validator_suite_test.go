// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinereplicaset/validation"
)

// suite is used for unit and integration testing this webhook.
var suite = builder.NewTestSuiteForValidatingWebhookWithContext(
	pkgcfg.NewContext(),
	validation.AddToManager,
	validation.NewValidator,
	"default.validating.virtualmachinereplicaset.v1alpha3.vmoperator.vmware.com")

func TestWebhook(t *testing.T) {
	suite.Register(t, "VirtualMachineReplicaSet webhook suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
