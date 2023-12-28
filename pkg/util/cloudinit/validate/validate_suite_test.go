// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validate_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()
var _ = BeforeSuite(suite.BeforeSuite)
var _ = AfterSuite(suite.AfterSuite)

func TestCloudInit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Cloud-Init Validation Suite")
}
