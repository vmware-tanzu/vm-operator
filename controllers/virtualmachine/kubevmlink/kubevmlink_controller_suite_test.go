// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kubevmlink_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/kubevmlink"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// The KubeVMProvider gate must be on in the initial config, not merely set
// later via pkgcfg.SetContext in the InitializeProviders callback below:
// pkg/manager/manager.go registers the generic scheme conditionally on this
// gate before InitializeProviders runs, so setting it any later leaves the
// scheme without the generic VirtualMachine type.
var suiteConfig = func() pkgcfg.Config {
	config := pkgcfg.Default()
	config.Features.KubeVMProvider = true
	return config
}()

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.WithConfig(suiteConfig),
	kubevmlink.AddToManager,
	func(_ *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
		return nil
	})

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

func TestKubeVMLink(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubeVM Link Controller Test Suite")
}
