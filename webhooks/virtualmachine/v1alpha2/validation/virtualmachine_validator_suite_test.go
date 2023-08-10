// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha2/validation"
)

// suite is used for unit and integration testing this webhook.
var suite = builder.NewTestSuiteForValidatingWebhookwithFSS(
	validation.AddToManager,
	validation.NewValidator,
	"default.validating.virtualmachine.v1alpha2.vmoperator.vmware.com",
	map[string]bool{lib.VMServiceV1Alpha2FSS: true})

func TestWebhook(t *testing.T) {
	suite.Register(t, "Validation webhook suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
