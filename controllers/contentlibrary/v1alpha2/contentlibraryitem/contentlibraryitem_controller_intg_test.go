// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem_test

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
	Describe("Invoking ContentLibraryItem controller integration tests", clItemReconcile)
}

func clItemReconcile() {
	const firmwareValue = "my-firmware"

	var (
		ctx *builder.IntegrationTestContext
	)

	waitForContentLibraryItemFinalizer := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			item := &imgregv1a1.ContentLibraryItem{}
			g.Expect(ctx.Client.Get(ctx, objKey, item)).To(Succeed())
			g.Expect(item.Finalizers).To(ContainElement(utils.ContentLibraryItemVmopFinalizer))
		}).Should(Succeed(), "waiting for ContentLibraryItem finalizer")
	}

	waitForVirtualMachineImageNotReady := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			image := &vmopv1.VirtualMachineImage{}
			g.Expect(ctx.Client.Get(ctx, objKey, image)).To(Succeed())

			readyCond := conditions.Get(image, vmopv1.VirtualMachineImageProviderReadyCondition)
			g.Expect(readyCond).ToNot(BeNil())
			g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		}).Should(Succeed(), "waiting for VirtualMachineImage to be not ready")
	}

	waitForVirtualMachineImageReady := func(objKey client.ObjectKey) {
		Eventually(func(g Gomega) {
			image := &vmopv1.VirtualMachineImage{}
			g.Expect(ctx.Client.Get(ctx, objKey, image)).To(Succeed())
			g.Expect(conditions.IsTrue(image, vmopv1.VirtualMachineImageProviderReadyCondition))
			// Assert that SyncVirtualMachineImage() has been called too.
			g.Expect(image.Status.Firmware).To(Equal(firmwareValue))
		}).Should(Succeed(), "waiting for VirtualMachineImage to be sync'd and ready")
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		intgFakeVMProvider.Lock()
		intgFakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, vmiObj client.Object) error {
			vmi := vmiObj.(*vmopv1.VirtualMachineImage)
			// Use Firmware field to verify the provider function is called.
			vmi.Status.Firmware = firmwareValue
			return nil
		}
		intgFakeVMProvider.Unlock()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile ContentLibraryItem", func() {

		It("Workflow", func() {
			origCLItem := utils.DummyContentLibraryItem(utils.ItemFieldNamePrefix+"-"+uuid.NewString(), ctx.Namespace)
			clItemKey := client.ObjectKeyFromObject(origCLItem)
			Expect(ctx.Client.Create(ctx, origCLItem.DeepCopy())).To(Succeed())

			imageName, err := utils.GetImageFieldNameFromItem(clItemKey.Name)
			Expect(err).ToNot(HaveOccurred())
			vmiKey := client.ObjectKey{Name: imageName, Namespace: ctx.Namespace}

			By("Finalizer should be added to ContentLibraryItem", func() {
				waitForContentLibraryItemFinalizer(clItemKey)
			})

			By("VirtualMachineImage is created but not ready", func() {
				waitForVirtualMachineImageNotReady(vmiKey)
			})

			clItem := &imgregv1a1.ContentLibraryItem{}
			By("Update ContentLibraryItem to populate Status and be Ready", func() {
				Expect(ctx.Client.Get(ctx, clItemKey, clItem)).To(Succeed())
				clItem.Status = origCLItem.Status
				Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
			})

			By("VirtualMachineImage becomes ready", func() {
				waitForVirtualMachineImageReady(vmiKey)
			})

			/* The non-caching ctx.Client.Get() won't populate these fields. */
			gvk, err := apiutil.GVKForObject(clItem, ctx.Client.Scheme())
			Expect(err).ToNot(HaveOccurred())
			clItem.APIVersion, clItem.Kind = gvk.ToAPIVersionAndKind()

			image := &vmopv1.VirtualMachineImage{}
			Expect(ctx.Client.Get(ctx, vmiKey, image)).To(Succeed())
			assertVMImageFromCLItem(image, clItem)

			By("ContentLibraryItem has new content version", func() {
				Expect(ctx.Client.Get(ctx, clItemKey, clItem)).To(Succeed())
				clItem.Status.ContentVersion += "-new-version"
				Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
			})

			By("VirtualMachineImage should be updated with new content version", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, vmiKey, image)).To(Succeed())
					g.Expect(image.Status.ProviderContentVersion).To(Equal(clItem.Status.ContentVersion))
				}).Should(Succeed())

				clItem.APIVersion, clItem.Kind = gvk.ToAPIVersionAndKind()
				assertVMImageFromCLItem(image, clItem)
			})
		})
	})
}
