// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
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

	waitForVirtualMachineImageReady := func(ctx *builder.IntegrationTestContext) {
		curVMI := vmopv1.VirtualMachineImage{}
		expectedVMI := utils.GetExpectedVMIFrom(*clItem, intgFakeVMProvider.SyncVirtualMachineImageFn)
		Eventually(func() bool {
			if err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(expectedVMI), &curVMI); err != nil {
				return false
			}
			utils.PopulateRuntimeFieldsTo(expectedVMI, &curVMI)
			return reflect.DeepEqual(curVMI.OwnerReferences, expectedVMI.OwnerReferences) &&
				reflect.DeepEqual(curVMI.Spec, expectedVMI.Spec) &&
				reflect.DeepEqual(curVMI.Status, expectedVMI.Status)
		}).Should(BeTrue(), "waiting for VirtualMachineImage to be created and synced")
	}

	waitForVirtualMachineImageDeleted := func(ctx *builder.IntegrationTestContext) {
		curVMI := vmopv1.VirtualMachineImage{}
		Eventually(func() bool {
			vmiName := utils.GetTestVMINameFrom(clItem.Name)
			if err := ctx.Client.Get(ctx, client.ObjectKey{Name: vmiName}, &curVMI); err != nil {
				return true
			}
			return false
		}).Should(BeTrue())
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
		clItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, "dummy")
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
			// TypeMeta and Status will be reset after the Create call below.
			// Save and add them back later to avoid the update or comparison failure.
			savedTypeMeta := clItem.TypeMeta
			savedStatus := *clItem.Status.DeepCopy()
			Expect(ctx.Client.Create(ctx, clItem)).To(Succeed())
			clItem.TypeMeta = savedTypeMeta
			clItem.Status = savedStatus
			Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctx.Client.DeleteAllOf(ctx, &imgregv1a1.ContentLibraryItem{},
				client.InNamespace(clItem.Namespace))).Should(Succeed())
			Eventually(func() bool {
				clItemList := &imgregv1a1.ContentLibraryItemList{}
				if err := ctx.Client.List(ctx, clItemList, client.InNamespace(clItem.Namespace)); err != nil {
					return false
				}
				return len(clItemList.Items) == 0
			}).Should(BeTrue())

			// Manually delete VMI as envTest doesn't delete it even if the OwnerReference is set up.
			Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineImage{},
				client.InNamespace(clItem.Namespace))).Should(Succeed())
			Eventually(func() bool {
				vmiList := &vmopv1.VirtualMachineImageList{}
				if err := ctx.Client.List(ctx, vmiList); err != nil {
					return false
				}
				return len(vmiList.Items) == 0
			}).Should(BeTrue())
		})

		When("VirtualMachineImage doesn't exist", func() {
			It("should create a new VirtualMachineImage", func() {
				waitForVirtualMachineImageReady(ctx)
			})
		})

		When("VirtualMachineImage already exists", func() {
			JustBeforeEach(func() {
				// Wait until the VMI gets created from BeforeEach() executed in Context block above.
				waitForVirtualMachineImageReady(ctx)
				// Update CLItem to trigger another reconciliation on the existing VMI resource.
				clItem.Status.ContentVersion = "dummy-content-version-new"
				Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
			})

			It("should sync and update the existing VirtualMachineImage", func() {
				waitForVirtualMachineImageReady(ctx)
			})
		})

		When("ContentLibraryItem's security compliance is not true", func() {

			JustBeforeEach(func() {
				waitForVirtualMachineImageReady(ctx)
				clItem.Status.SecurityCompliance = &[]bool{false}[0]
				Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
			})

			It("should delete VirtualMachineImage", func() {
				waitForVirtualMachineImageDeleted(ctx)
			})
		})
	})
}
