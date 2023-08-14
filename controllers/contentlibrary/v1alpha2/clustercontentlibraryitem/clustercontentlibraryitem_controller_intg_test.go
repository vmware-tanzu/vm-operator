// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha2/utils"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking ClusterContentLibraryItem controller integration tests", cclItemReconcile)
}

func cclItemReconcile() {
	const firmwareValue = "my-firmware"

	var (
		ctx *builder.IntegrationTestContext
	)

	waitForClusterContentLibraryItemFinalizer := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			item := &imgregv1a1.ClusterContentLibraryItem{}
			g.Expect(ctx.Client.Get(ctx, objKey, item)).To(Succeed())
			g.Expect(item.Finalizers).To(ContainElement(utils.ClusterContentLibraryItemVmopFinalizer))
		}).Should(Succeed(), "waiting for ClusterContentLibraryItem finalizer")
	}

	waitForClusterVirtualMachineImageNotReady := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			image := &vmopv1.ClusterVirtualMachineImage{}
			g.Expect(ctx.Client.Get(ctx, objKey, image)).To(Succeed())

			readyCond := conditions.Get(image, vmopv1.VirtualMachineImageProviderReadyCondition)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		}).Should(Succeed(), "waiting for ClusterVirtualMachineImage to be not ready")
	}

	waitForClusterVirtualMachineImageReady := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			image := &vmopv1.ClusterVirtualMachineImage{}
			g.Expect(ctx.Client.Get(ctx, objKey, image)).To(Succeed())
			g.Expect(conditions.IsTrue(image, vmopv1.VirtualMachineImageProviderReadyCondition))
			// Assert that SyncVirtualMachineImage() has been called too.
			g.Expect(image.Status.Firmware).To(Equal(firmwareValue))
		}).Should(Succeed(), "waiting for ClusterVirtualMachineImage to be sync'd and ready")
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		intgFakeVMProvider.Lock()
		intgFakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, cvmiObj client.Object) error {
			cvmi := cvmiObj.(*vmopv1.ClusterVirtualMachineImage)
			// Use Firmware field to verify the provider function is called.
			cvmi.Status.Firmware = firmwareValue
			return nil
		}
		intgFakeVMProvider.Unlock()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile ClusterContentLibraryItem", func() {

		It("Workflow", func() {
			origCCLItem := utils.DummyClusterContentLibraryItem(utils.ItemFieldNamePrefix + "-" + uuid.NewString())
			cclItemKey := client.ObjectKeyFromObject(origCCLItem)
			Expect(ctx.Client.Create(ctx, origCCLItem.DeepCopy())).To(Succeed())

			imageName, err := utils.GetImageFieldNameFromItem(cclItemKey.Name)
			Expect(err).ToNot(HaveOccurred())
			cvmiKey := client.ObjectKey{Name: imageName}

			By("Finalizer should be added to ClusterContentLibraryItem", func() {
				waitForClusterContentLibraryItemFinalizer(cclItemKey)
			})

			By("ClusterVirtualMachineImage is created but not ready", func() {
				waitForClusterVirtualMachineImageNotReady(cvmiKey)
			})

			cclItem := &imgregv1a1.ClusterContentLibraryItem{}
			By("Update ClusterContentLibraryItem to populate Status and be Ready", func() {
				Expect(ctx.Client.Get(ctx, cclItemKey, cclItem)).To(Succeed())
				cclItem.Status = origCCLItem.Status
				Expect(ctx.Client.Status().Update(ctx, cclItem)).To(Succeed())
			})

			By("ClusterVirtualMachineImage becomes ready", func() {
				waitForClusterVirtualMachineImageReady(cvmiKey)
			})

			/* The non-caching ctx.Client.Get() won't populate these fields. */
			gvk, err := apiutil.GVKForObject(cclItem, ctx.Client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			cclItem.APIVersion, cclItem.Kind = gvk.ToAPIVersionAndKind()

			image := &vmopv1.ClusterVirtualMachineImage{}
			Expect(ctx.Client.Get(ctx, cvmiKey, image)).To(Succeed())
			assertCVMImageFromCCLItem(image, cclItem)

			By("ClusterContentLibraryItem has new content version", func() {
				Expect(ctx.Client.Get(ctx, cclItemKey, cclItem)).To(Succeed())
				cclItem.Status.ContentVersion += "-new-version"
				Expect(ctx.Client.Status().Update(ctx, cclItem)).To(Succeed())
			})

			By("ClusterVirtualMachineImage should be updated with new content version", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, cvmiKey, image)).To(Succeed())
					g.Expect(image.Status.ProviderContentVersion).To(Equal(cclItem.Status.ContentVersion))
				}).Should(Succeed())

				cclItem.APIVersion, cclItem.Kind = gvk.ToAPIVersionAndKind()
				assertCVMImageFromCCLItem(image, cclItem)
			})
		})
	})
}
