// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

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
	Describe("Invoking ClusterContentLibraryItem controller integration tests", cclItemReconcile)
}

func cclItemReconcile() {
	var (
		ctx     *builder.IntegrationTestContext
		cclItem *imgregv1a1.ClusterContentLibraryItem
	)

	waitForClusterContentLibraryItemCreation := func(ctx *builder.IntegrationTestContext) {
		// TypeMeta and Status will be reset after the Create call below.
		// Save and add them back later to avoid the update or comparison failure.
		savedTypeMeta := cclItem.TypeMeta
		savedStatus := *cclItem.Status.DeepCopy()
		Expect(ctx.Client.Create(ctx, cclItem)).To(Succeed())

		// Get the latest version of CCLItem to avoid status update conflict.
		updatedCCLItem := imgregv1a1.ClusterContentLibraryItem{}
		Eventually(func(g Gomega) {
			g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(cclItem), &updatedCCLItem)).To(Succeed())
			g.Expect(updatedCCLItem.Finalizers).To(ContainElement(utils.ClusterContentLibraryItemVmopFinalizer))
		}).Should(Succeed(), "waiting for ClusterContentLibraryItem to be created and synced")
		updatedCCLItem.Status = savedStatus
		Expect(ctx.Client.Status().Update(ctx, &updatedCCLItem)).To(Succeed())

		cclItem = &updatedCCLItem
		cclItem.TypeMeta = savedTypeMeta
	}

	waitForClusterVirtualMachineImageReady := func(ctx *builder.IntegrationTestContext) {
		curCVMI := vmopv1.ClusterVirtualMachineImage{}
		expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, intgFakeVMProvider.SyncVirtualMachineImageFn)
		Eventually(func(g Gomega) {
			g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(expectedCVMI), &curCVMI)).To(Succeed())
			utils.PopulateRuntimeFieldsTo(expectedCVMI, &curCVMI)
			g.Expect(curCVMI.OwnerReferences).To(Equal(expectedCVMI.OwnerReferences))
			g.Expect(curCVMI.Spec).To(Equal(expectedCVMI.Spec))
			g.Expect(curCVMI.Status).To(Equal(expectedCVMI.Status))
		}).Should(Succeed(), "waiting for ClusterVirtualMachineImage to be created and synced")
	}

	BeforeEach(func() {
		// The name for cclItem must have the expected prefix to be parsed by the controller.
		// Using uuid here to avoid name conflict as envtest doesn't clean up the resources by ownerReference.
		cclItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, uuid.NewString())
		cclItem = utils.DummyClusterContentLibraryItem(cclItemName)

		ctx = suite.NewIntegrationTestContext()
		intgFakeVMProvider.Lock()
		defer intgFakeVMProvider.Unlock()
		intgFakeVMProvider.SyncVirtualMachineImageFn = func(
			_ context.Context, _, cvmiObj client.Object) error {
			// Change a random spec and status field to verify the provider function is called.
			cvmi := cvmiObj.(*vmopv1.ClusterVirtualMachineImage)
			cvmi.Spec.HardwareVersion = 123
			cvmi.Status.ImageSupported = &[]bool{true}[0]
			return nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		cclItem = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile ClusterContentLibraryItem", func() {

		BeforeEach(func() {
			waitForClusterContentLibraryItemCreation(ctx)
		})

		When("ClusterVirtualMachineImage doesn't exist", func() {

			It("should create a new ClusterVirtualMachineImage", func() {
				waitForClusterVirtualMachineImageReady(ctx)
			})
		})

		When("ClusterVirtualMachineImage already exists", func() {

			JustBeforeEach(func() {
				// Wait until the CVMI gets created before triggering another reconciliation.
				waitForClusterVirtualMachineImageReady(ctx)
				cclItem.Status.ContentVersion = "dummy-content-version-new"
				Expect(ctx.Client.Status().Update(ctx, cclItem)).To(Succeed())
			})

			It("should sync and update the existing ClusterVirtualMachineImage", func() {
				waitForClusterVirtualMachineImageReady(ctx)
			})
		})
	})
}
