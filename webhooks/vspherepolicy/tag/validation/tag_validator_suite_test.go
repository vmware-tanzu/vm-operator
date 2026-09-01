// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/vspherepolicy/tag/validation"
)

// suite is used for unit testing this webhook.
var suite = builder.NewTestSuiteForValidatingWebhook(
	validation.AddToManager,
	validation.NewValidator,
	"default.validating.tag.v1alpha1.vsphere.policy.vmware.com")

func TestWebhook(t *testing.T) {
	suite.Register(t, "Tag validation webhook suite", nil, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
