// +build integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// BMV This doesn't belong here
package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/test/integration"
)

// The purpose of this test is to start up a vmop controller and verify leader election functionality on that controller
var _ = Describe("VM Operator Controller leader election tests", func() {

	ns := integration.DefaultNamespace
	var (
		stopMgr                 chan struct{}
		c                       client.Client
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
	)
	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      leaderElectionConfigMap,
			},
		}
		err := c.Delete(context.Background(), configMap)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("When Leader election is enabled", func() {
		It("The vmoperator-controller-manager-runtime configmap should be created in the gcm-system namespace", func() {
			leConfigMap := &corev1.ConfigMap{}
			err := c.Get(context.Background(), types.NamespacedName{
				Namespace: ns,
				Name:      leaderElectionConfigMap,
			}, leConfigMap)
			Expect(err).NotTo(HaveOccurred())
		})
		It("The vmoperator-controller-manager-runtime configmap specifies one leader", func() {
			leConfigMap := &corev1.ConfigMap{}
			err := c.Get(context.Background(), types.NamespacedName{
				Namespace: ns,
				Name:      leaderElectionConfigMap,
			}, leConfigMap)
			Expect(err).NotTo(HaveOccurred())
			leaderId, err := os.Hostname()
			Expect(err).NotTo(HaveOccurred())
			Expect(leConfigMap.Annotations["control-plane.alpha.kubernetes.io/leader"]).To(ContainSubstring(leaderId))
		})
	})
})
