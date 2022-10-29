// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
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

	waitForClusterVirtualMachineImageReady := func(ctx *builder.IntegrationTestContext) {
		curCVMI := vmopv1a1.ClusterVirtualMachineImage{}
		expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, intgFakeVMProvider.SyncVirtualMachineImageFn)
		Eventually(func() bool {
			cvmiName := utils.GetTestVMINameFrom(cclItem.Name)
			if err := ctx.Client.Get(ctx, client.ObjectKey{Name: cvmiName}, &curCVMI); err != nil {
				return false
			}
			utils.PopulateRuntimeFieldsTo(expectedCVMI, &curCVMI)
			return reflect.DeepEqual(curCVMI.OwnerReferences, expectedCVMI.OwnerReferences) &&
				reflect.DeepEqual(curCVMI.Spec, expectedCVMI.Spec) &&
				reflect.DeepEqual(curCVMI.Status, expectedCVMI.Status)
		}).Should(BeTrue(), "waiting for ClusterVirtualMachineImage to be created and synced")
	}

	BeforeEach(func() {
		// The name for cclItem must have the expected prefix to be parsed by the controller.
		cclItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, "dummy")
		cclItem = utils.DummyClusterContentLibraryItem(cclItemName)

		ctx = suite.NewIntegrationTestContext()
		intgFakeVMProvider.Lock()
		defer intgFakeVMProvider.Unlock()
		intgFakeVMProvider.SyncVirtualMachineImageFn = func(ctx context.Context,
			itemID string, cvmiObj client.Object) error {
			// Change a random spec and status field to verify the provider function is called.
			cvmi := cvmiObj.(*vmopv1a1.ClusterVirtualMachineImage)
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
			// TypeMeta and Status will be reset after the Create call below.
			// Save and add them back later to avoid the update or comparison failure.
			savedTypeMeta := cclItem.TypeMeta
			savedStatus := *cclItem.Status.DeepCopy()
			Expect(ctx.Client.Create(ctx, cclItem)).To(Succeed())
			cclItem.TypeMeta = savedTypeMeta
			cclItem.Status = savedStatus
			Expect(ctx.Client.Status().Update(ctx, cclItem)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctx.Client.DeleteAllOf(ctx, &imgregv1a1.ClusterContentLibraryItem{})).Should(Succeed())
			Eventually(func() bool {
				cclItemList := &imgregv1a1.ClusterContentLibraryItemList{}
				if err := ctx.Client.List(ctx, cclItemList); err != nil {
					return false
				}
				return len(cclItemList.Items) == 0
			}).Should(BeTrue())

			// Manually delete CVMI as envTest doesn't delete it even if the OwnerReference is set up.
			Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1a1.ClusterVirtualMachineImage{})).Should(Succeed())
			Eventually(func() bool {
				cvmiList := &vmopv1a1.ClusterVirtualMachineImageList{}
				if err := ctx.Client.List(ctx, cvmiList); err != nil {
					return false
				}
				return len(cvmiList.Items) == 0
			}).Should(BeTrue())
		})

		When("ClusterVirtualMachineImage doesn't exist", func() {

			It("should create a new ClusterVirtualMachineImage", func() {
				waitForClusterVirtualMachineImageReady(ctx)
			})
		})

		When("ClusterVirtualMachineImage already exists", func() {

			JustBeforeEach(func() {
				// Wait until the CVMI gets created from BeforeEach() executed in Context block above.
				waitForClusterVirtualMachineImageReady(ctx)
				// Update CCLItem tro trigger another reconciliation on the existing CVMI resource.
				cclItem.Status.ContentVersion = "dummy-content-version-new"
				Expect(ctx.Client.Status().Update(ctx, cclItem)).To(Succeed())
			})

			It("should sync and update the existing ClusterVirtualMachineImage", func() {
				waitForClusterVirtualMachineImageReady(ctx)
			})
		})
	})
}
