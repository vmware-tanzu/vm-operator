// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForPackage(noOpAddToManagerFunc)
var _ = BeforeSuite(suite.BeforeSuite)

func TestManager(t *testing.T) {
	suite.Register(t, "Service Discovery controller suite", intgTests, nil)
}

func intgTests() {
	Describe("ControllerManager", func() {
		var opts manager.Options
		var m manager.Manager
		var err error

		JustBeforeEach(func() {
			Expect(opts).ToNot(BeNil(), "Be sure to set the options you want to test in your BeforeEach!")
			m, err = manager.New(testingManagerOpts(opts))
		})

		AfterEach(func() {
			m, err = nil, nil
		})

		Context("When no options are specified", func() {
			BeforeEach(func() {
				opts = manager.Options{}
			})

			JustBeforeEach(func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(m).ToNot(BeNil())
			})
		})
	})
}

// testingManagerOpts returns adds necessary testing-related options to the supplied values
func testingManagerOpts(opts manager.Options) manager.Options {
	opts.KubeConfig = suite.GetEnvTestConfig()
	opts.Scheme = runtime.NewScheme()
	opts.MetricsAddr = "0"
	opts.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		syncPeriod := 1 * time.Second
		opts.Resync = &syncPeriod
		return cache.New(config, opts)
	}
	opts.AddToManager = noOpAddToManagerFunc

	return opts
}

// noOpAddToManagerFunc does nothing because we don't need to add anything to the manager for this testing.
func noOpAddToManagerFunc(_ *context.ControllerManagerContext, _ ctrlmgr.Manager) error {
	return nil
}
