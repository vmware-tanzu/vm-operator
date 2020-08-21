// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package infraprovider

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controllerContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

const timeout = time.Second * 5

var _ = Describe("InfraProvider controller", func() {
	var (
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
		mgr        manager.Manager
		c          client.Client
	)

	BeforeEach(func() {
		var err error
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/deleting a Node", func() {
		It("invoke the reconcile method to compute cpu min frequency", func() {
			name := "node-02"
			instance := v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			}

			ctrlContext := &controllerContext.ControllerManagerContext{
				VmProvider: vmProvider,
			}

			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}
			recFn, requests, _, _ := integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
			Expect(add(mgr, recFn)).To(Succeed())
			// Create the Node object and expect the Reconcile to compute cpuMinFreq
			err := c.Create(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			// Delete the Node object and expect the Reconcile to compute cpuMinFreq
			err = c.Delete(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})
})
