// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"

	vmmetrics "github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/metrics"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	cource.WithContext(
		pkgcfg.NewContextWithDefaultConfig(),
	),
	vmmetrics.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVMMetrics(t *testing.T) {
	suite.Register(t, "VM Metrics controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(func() {
	vmmetrics.SkipNameValidation = ptr.To(true)
	suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
