// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking ContentLibraryItem controller integration tests", clItemReconcile)
}

func clItemReconcile() {
	var (
		ctx    *builder.IntegrationTestContext
		clItem *imgregv1a1.ContentLibraryItem
	)

	waitForContentLibraryItemCreation := func(ctx *builder.IntegrationTestContext) {
		// TypeMeta and Status will be reset after the Create call below.
		// Save and add them back later to avoid the update or comparison failure.
		savedTypeMeta := clItem.TypeMeta
		savedStatus := *clItem.Status.DeepCopy()
		Expect(ctx.Client.Create(ctx, clItem)).To(Succeed())

		// Get the latest version of CLItem to avoid status update conflict.
		updatedCLItem := imgregv1a1.ContentLibraryItem{}
		Eventually(func(g Gomega) {
			g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(clItem), &updatedCLItem)).To(Succeed())
			g.Expect(updatedCLItem.Finalizers).To(ContainElement(utils.ContentLibraryItemVmopFinalizer))
		}).Should(Succeed(), "waiting for ContentLibraryItem to be created and synced")
		updatedCLItem.Status = savedStatus
		Expect(ctx.Client.Status().Update(ctx, &updatedCLItem)).To(Succeed())

		clItem = &updatedCLItem
		clItem.TypeMeta = savedTypeMeta
	}

	waitForVirtualMachineImageReady := func(ctx *builder.IntegrationTestContext) {
		curVMI := vmopv1.VirtualMachineImage{}
		expectedVMI := utils.GetExpectedVMIFrom(*clItem, intgFakeVMProvider.SyncVirtualMachineImageFn)
		Eventually(func(g Gomega) {
			g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(expectedVMI), &curVMI)).To(Succeed())
			utils.PopulateRuntimeFieldsTo(expectedVMI, &curVMI)
			g.Expect(curVMI.OwnerReferences).To(Equal(expectedVMI.OwnerReferences))
			g.Expect(curVMI.Finalizers).To(Equal(expectedVMI.Finalizers))
			g.Expect(curVMI.Spec).To(Equal(expectedVMI.Spec))
			g.Expect(curVMI.Status).To(Equal(expectedVMI.Status))
		}).Should(Succeed(), "waiting for VirtualMachineImage to be created and synced")
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		intgFakeVMProvider.Lock()
		defer intgFakeVMProvider.Unlock()
		intgFakeVMProvider.SyncVirtualMachineImageFn = func(
			_ context.Context, _, vmiObj client.Object) error {
			vmi := vmiObj.(*vmopv1.VirtualMachineImage)
			// Change a random spec and status field to verify the provider function is called.
			vmi.Spec.HardwareVersion = 123
			vmi.Status.ImageSupported = &[]bool{true}[0]
			return nil
		}

		// The name for clItem must have the expected prefix to be parsed by the controller.
		// Using uuid here to avoid name conflict as envtest doesn't clean up the resources by ownerReference.
		clItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, uuid.NewString())
		clItem = utils.DummyContentLibraryItem(clItemName, ctx.Namespace)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		clItem = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile ContentLibraryItem", func() {

		BeforeEach(func() {
			waitForContentLibraryItemCreation(ctx)
		})

		When("VirtualMachineImage doesn't exist", func() {
			It("should create a new VirtualMachineImage", func() {
				waitForVirtualMachineImageReady(ctx)
			})
		})

		When("VirtualMachineImage already exists", func() {
			JustBeforeEach(func() {
				// Wait until the VMI gets created before triggering another reconciliation.
				waitForVirtualMachineImageReady(ctx)
				clItem.Status.ContentVersion = "dummy-content-version-new"
				Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
			})

			It("should sync and update the existing VirtualMachineImage", func() {
				waitForVirtualMachineImageReady(ctx)
			})
		})
	})
}
