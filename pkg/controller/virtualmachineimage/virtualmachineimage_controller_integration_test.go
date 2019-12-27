// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineimage

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

const timeout = time.Second * 30

var _ = Describe("VirtualMachineImageDiscoverer", func() {

	var (
		c                       client.Client
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		err                     error
		leaderElectionConfigMap string
		ns                      = integration.DefaultNamespace
		ctx                     context.Context
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.

		syncPeriod := 5 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(restConfig, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		err = AddWithOptions(mgr, VirtualMachineImageDiscovererOptions{
			initialDiscoveryFrequency:    1 * time.Second,
			continuousDiscoveryFrequency: 5 * time.Second})
		Expect(err).NotTo(HaveOccurred())

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

	Describe("with VM images in inventory", func() {

		Context("with an initial image in the CL", func() {

			It("should list the VM images", func() {

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.Background(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return len(imageList.Items) == 1 && imageList.Items[0].Name == integration.IntegrationContentLibraryItemName
				}, timeout).Should(BeTrue())

			})
		})

		Context("with a Content Library configured", func() {
			var err error

			BeforeEach(func() {
				ctx = context.Background()
			})

			imageExistsFunc := func(imageList *vmoperatorv1alpha1.VirtualMachineImageList, name string) bool {
				found := false
				for _, image := range imageList.Items {
					if image.Name == name {
						found = true
					}
				}

				return found
			}

			It("should add a VirtualMachineImage to if an item is added to the library", func() {
				var imageToAddName = "imageToAdd"

				err = integration.CreateLibraryItem(ctx, session, imageToAddName, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())
					return imageExistsFunc(imageList, imageToAddName)
				}, timeout).Should(BeTrue())
			})

			// TODO: Need Jira for CL item removal
			XIt("should remove a VirtualMachineImage if one is removed from the library", func() {
				var imageToRemoveName = "imageToRemove"

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())
					return imageExistsFunc(imageList, imageToRemoveName)
				}, timeout).Should(BeFalse())
			})

			It("should remove an extra image if one was added to the control plane", func() {
				var strayImageName = "strayImage"

				strayImage := vmoperatorv1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: strayImageName,
					},
					Spec: vmoperatorv1alpha1.VirtualMachineImageSpec{
						Type:            "ovf",
						ImageSourceType: "Content Library",
					},
				}

				err = c.Create(ctx, &strayImage)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, strayImageName)
				}, timeout).Should(BeFalse())
			})

			It("should re-create if image is deleted", func() {
				var imageToRecreate = "imageToRecreate"

				err = integration.CreateLibraryItem(ctx, session, imageToRecreate, "ovf", integration.ContentSourceID)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, imageToRecreate)
				}, timeout).Should(BeTrue())

				existingImage := vmoperatorv1alpha1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: imageToRecreate,
					},
				}
				err = c.Delete(ctx, &existingImage)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func() bool {
					imageList := &vmoperatorv1alpha1.VirtualMachineImageList{}
					err = c.List(context.TODO(), &client.ListOptions{}, imageList)
					Expect(err).ShouldNot(HaveOccurred())

					return imageExistsFunc(imageList, imageToRecreate)
				}, timeout).Should(BeTrue())
			})
		})
	})
})

var _ = Describe("ReconcileVirtualMachineImage", func() {

	var (
		c                       client.Client
		stopMgr                 chan struct{}
		mgrStopped              *sync.WaitGroup
		mgr                     manager.Manager
		expectedRequest         reconcile.Request
		recFn                   reconcile.Reconciler
		requests                chan reconcile.Request
		err                     error
		leaderElectionConfigMap string
		ns                      = integration.DefaultNamespace
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
		// channel when it is finished.

		syncPeriod := 5 * time.Second
		leaderElectionConfigMap = fmt.Sprintf("vmoperator-controller-manager-runtime-%s", uuid.New())
		mgr, err = manager.New(restConfig, manager.Options{SyncPeriod: &syncPeriod,
			LeaderElection:          true,
			LeaderElectionID:        leaderElectionConfigMap,
			LeaderElectionNamespace: ns})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()

		recFn, requests, _ = integration.SetupTestReconcile(newReconciler(mgr, VirtualMachineImageDiscovererOptions{
			initialDiscoveryFrequency:    1 * time.Second,
			continuousDiscoveryFrequency: 5 * time.Second}))
		Expect(add(mgr, recFn)).To(Succeed())

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

	Describe("Reconcile", func() {
		It("Should receive reconcile event and do nothing", func() {
			// Create the VM Image Object then expect Reconcile
			image := vmoperatorv1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "imageToReconcile",
				},
			}
			expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "imageToReconcile"}}

			err := c.Create(context.TODO(), &image)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})

})
