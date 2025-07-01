// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()
var _ = BeforeSuite(suite.BeforeSuite)
var _ = AfterSuite(suite.AfterSuite)

func TestNetwork(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Network Suite")
}
