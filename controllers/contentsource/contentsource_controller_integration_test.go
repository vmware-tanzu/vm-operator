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
func newContentSource(name string) *vmopv1alpha1.ContentSource {
	return &vmopv1alpha1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: vmopv1alpha1.ContentSourceSpec{
			ProviderRef: vmopv1alpha1.ContentProviderReference{
				APIVersion: "vmoperator.vmware.com/v1alpha1",
				Kind:       "ContentLibraryProvider",
				Name:       name,
			},
		},
	}
}

// newContentLibraryProvider returns a new ContentLibraryProvider object
func newContentLibraryProvider(name string, libID string) *vmopv1alpha1.ContentLibraryProvider {
	return &vmopv1alpha1.ContentLibraryProvider{
		TypeMeta: metav1.TypeMeta{
			Kind: "ContentLibraryProvider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: vmopv1alpha1.ContentLibraryProviderSpec{
			UUID: libID,
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
		timeout    time.Duration
		recFn      reconcile.Reconciler
		requests   chan reconcile.Request
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.
		syncPeriod := 10 * time.Second
		mgr, err = manager.New(cfg, manager.Options{SyncPeriod: &syncPeriod, MetricsBindAddress: "0"})
		Expect(err).NotTo(HaveOccurred())
		k8sClient = mgr.GetClient()

		stopMgr, mgrStopped = integration.StartTestManager(mgr)

		ctrlContext := &context.ControllerManagerContext{
			VmProvider: vmProvider,
		}
		recFn, requests, _, _ = integration.SetupTestReconcile(newReconciler(ctrlContext, mgr))
		Expect(add(mgr, recFn)).To(Succeed())

		timeout = 30 * time.Second
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()
	})

	Context("create a content source", func() {
		It("will create the resource the API server", func() {
			contentSourceName := "test-contentsource"
			obj := newContentSource(contentSourceName)
			key := client.ObjectKey{
				Name: obj.GetName(),
			}

			expectedRequest := reconcile.Request{key}

			By("Creating the ContentLibraryProvider")
			clProvider := newContentLibraryProvider(contentSourceName, "fake-cl-uuid")
			Expect(k8sClient.Create(ctx, clProvider)).To(Succeed())

			// Wait for the clProvider to show up on the API server
			Eventually(func() bool {
				clProvider := vmopv1alpha1.ContentLibraryProvider{}
				err := k8sClient.Get(ctx, key, &clProvider)
				return err == nil
			}, timeout).Should(BeTrue())

			By("Creating the ContentSource with a ProviderRef")
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Wait for the object to show up on the API server
			Eventually(func() bool {
				contentSource := vmopv1alpha1.ContentSource{}
				err := k8sClient.Get(ctx, key, &contentSource)
				return err == nil
			}, timeout).Should(BeTrue())

			By("Deleting the ContentSource object")
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			// Wait for the object to be deleted
			Eventually(func() bool {
				contentSource := vmopv1alpha1.ContentSource{}
				err := k8sClient.Get(ctx, key, &contentSource)
				return err != nil
			}, timeout).Should(BeTrue())

		})
	})
})
