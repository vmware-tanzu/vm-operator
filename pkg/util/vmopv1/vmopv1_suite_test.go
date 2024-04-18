// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteWithContext(
	pkgcfg.WithConfig(
		pkgcfg.Config{
			BuildVersion: "v1",
		}))

func TestVmopv1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vmopv1 util suite")
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)
