// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func integTests() {
	Describe("Cache", Ordered, Label(testlabels.EnvTest), cacheTests)
}

var suite = builder.NewTestSuite()

func TestManager(t *testing.T) {
	suite.SetManagerNewCacheFunc(newCacheProxy)
	suite.Register(t, "Manager Suite", integTests, nil)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
