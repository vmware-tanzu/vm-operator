// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"golang.org/x/net/context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/integration"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var c client.Client

const timeout = time.Second * 5

var _ = Describe("VirtualMachineService controller", func() {
	ns := integration.DefaultNamespace
	name := "fooVm"

	var (
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
		mgr        manager.Manager
		err        error
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		mgr, err = manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		stopMgr, mgrStopped = StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Describe("when creating/deleting a VM Service", func() {
		It("invoke the reconcile method", func() {
			port := vmoperatorv1alpha1.VirtualMachineServicePort{
				Name:       "foo",
				Protocol:   "TCP",
				Port:       42,
				TargetPort: 42,
			}

			instance := vmoperatorv1alpha1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
				},
				Spec: vmoperatorv1alpha1.VirtualMachineServiceSpec{
					Type:     "ClusterIP",
					Ports:    []vmoperatorv1alpha1.VirtualMachineServicePort{port},
					Selector: map[string]string{"foo": "bar"},
				},
			}

			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
			recFn, requests := SetupTestReconcile(newReconciler(mgr))
			Expect(add(mgr, recFn)).To(Succeed())
			// Create the VM Service object and expect the Reconcile
			err := c.Create(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
			//Delete the VM Service object and expect the Reconcile
			err = c.Delete(context.TODO(), &instance)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})
	/*
		deploy := &appsv1.Deployment{}
		g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
			Should(gomega.Succeed())

		// Delete the Deployment and expect Reconcile to be called for Deployment deletion
		g.Expect(c.Delete(context.TODO(), deploy)).NotTo(gomega.HaveOccurred())
		g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
		g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
			Should(gomega.Succeed())

		// Manually delete Deployment since GC isn't enabled in the test control plane
		g.Eventually(func() error { return c.Delete(context.TODO(), deploy) }, timeout).
			Should(gomega.MatchError("deployments.apps \"foo-deployment\" not found"))
	*/
})
