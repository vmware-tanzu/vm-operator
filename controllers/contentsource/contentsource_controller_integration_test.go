// +build integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

// newContentsource returns a new ContentSource object
func newContentSource(name, namespace string) *vmopv1alpha1.ContentSource {
	return &vmopv1alpha1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				APIVersion: "vmoperator.vmware.com/v1alpha1",
				Kind:       "ContentLibraryProvider",
				Name:       name,
				Namespace:  namespace,
			},
		},
	}
}

var _ = Describe("ContentSource controller", func() {
	var (
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
		mgr        manager.Manager
		err        error
		k8sClient  client.Client
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod})
		Expect(err).NotTo(HaveOccurred())
		k8sClient = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Context("create a content source", func() {
		It("will create the resource the API server", func() {
			contentSourceName := "test-contentsource"
			testns := "test-ns"
			ctrlContext := &context.ControllerManagerContext{}
			recFn, requests, _, _ := integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
			Expect(add(mgr, recFn)).To(Succeed())

			obj := newContentSource(contentSourceName, testns)
			key := client.ObjectKey{
				Name: obj.GetName(),
			}

			expectedRequest := reconcile.Request{key}

			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			Eventually(requests, 10*time.Second).Should(Receive(Equal(expectedRequest)))

			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
			Eventually(requests, 10*time.Second).Should(Receive(Equal(expectedRequest)))
		})
	})
})
