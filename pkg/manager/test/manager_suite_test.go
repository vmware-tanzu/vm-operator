// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func integTests() {
	Describe("Cache", Ordered, Label("envtest"), cacheTests)
}

var suite = builder.NewTestSuite()

func TestManager(t *testing.T) {
	suite.SetManagerNewCacheFunc(newCacheProxy)
	suite.Register(t, "Manager Suite", integTests, nil)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
