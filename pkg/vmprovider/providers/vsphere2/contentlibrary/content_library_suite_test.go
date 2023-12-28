// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("ContentLibrary Provider", clTests)
}

var suite = builder.NewTestSuite()

func TestContentLibrary(t *testing.T) {
	suite.Register(t, "vSphere Provider ContentLibrary Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
