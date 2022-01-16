// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package hostvalidation_test

import (
	"testing"

	. "github.com/onsi/ginkgo"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

func TestHostValidation(t *testing.T) {
	suite.Register(t, "Host Validation Suite", nil, vcSimTests)
}

func vcSimTests() {
	Describe("Host Validation Tests with VC Simulator", hostValidationTests)
}
