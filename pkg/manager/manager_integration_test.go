// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopcontext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var suite = builder.NewTestSuiteForPackage(noOpAddToManagerFunc)
var _ = BeforeSuite(suite.BeforeSuite)

func TestManager(t *testing.T) {
	suite.Register(t, "Service Discovery controller suite", intgTests, nil)
}

func intgTests() {
	Describe("ControllerManager", func() {
		var (
			opts *manager.Options
			m    manager.Manager
			c    client.Client
			err  error
		)

		JustBeforeEach(func() {
			Expect(opts).ToNot(BeNil(), "Be sure to set the options you want to test in your BeforeEach!")
			m, err = manager.New(testingManagerOpts(*opts))
			Expect(err).NotTo(HaveOccurred())
			Expect(m).ToNot(BeNil())
			c = m.GetClient()
			Expect(c).ToNot(BeNil())
		})

		AfterEach(func() {
			opts, m, c = nil, nil, nil
		})

		When("no options are specified", func() {
			BeforeEach(func() {
				opts = &manager.Options{}
			})

			It("a manager is created with testingManagerOpts", func() {
				// nothing to do here: just checking a manager can be created with default opts
			})
		})

		When("leader election is enabled in manager.Options", func() {
			leaderElectionConfigMap := fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
			ns := integration.DefaultNamespace

			BeforeEach(func() {
				opts = &manager.Options{
					LeaderElectionEnabled: true,
					LeaderElectionID:      leaderElectionConfigMap,
					PodNamespace:          ns,
				}
			})

			It("enables leader-election in the underlying controller-runtime Manager", func() {
				ler := make(leaderElectionRunnable)
				err = m.Add(ler)
				Expect(err).ToNot(HaveOccurred(), "should successfully add a leaderElectionRunnable")
				stopMgr, mgrStopped := integration.StartTestManager(m)
				<-ler
				// At this point, runnables that need leader election have started.
				// If leader election has been enabled, leaderElectionConfigMap should exist.
				close(stopMgr)
				mgrStopped.Wait()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      leaderElectionConfigMap,
					},
				}
				err = c.Delete(context.Background(), configMap)
				Expect(err).NotTo(HaveOccurred(), "if leader election was on, the ConfigMap would have existed")
			})
		})
	})
}

type leaderElectionRunnable chan struct{}

func (n leaderElectionRunnable) Start(<-chan struct{}) error {
	close(n)
	return nil
}

func (n leaderElectionRunnable) NeedLeaderElection() bool {
	return true
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
func noOpAddToManagerFunc(_ *vmopcontext.ControllerManagerContext, _ ctrlmgr.Manager) error {
	return nil
}
