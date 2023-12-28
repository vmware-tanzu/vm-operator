// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vcSimTests() {
	Describe("Config", configTests)
}

var suite = builder.NewTestSuite()

func TestConfig(t *testing.T) {
	suite.Register(t, "vSphere Provider Config Suite", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
