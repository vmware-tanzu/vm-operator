// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibraryitem_test

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha2/contentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha2/utils"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ContentLibraryItem controller unit tests", unitTestsReconcile)
}

func unitTestsReconcile() {
	const firmwareValue = "my-firmware"

	var (
		ctx *builder.UnitTestContextForController

		reconciler     *contentlibraryitem.Reconciler
		fakeVMProvider *providerfake.VMProviderA2

		clItem    *imgregv1a1.ContentLibraryItem
		clItemCtx *context.ContentLibraryItemContextA2
	)

	BeforeEach(func() {
		ctx = suite.NewUnitTestContextForController()

		reconciler = contentlibraryitem.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProviderA2,
		)

		fakeVMProvider = ctx.VMProviderA2.(*providerfake.VMProviderA2)

		fakeVMProvider.SyncVirtualMachineImageFn = func(_ goctx.Context, _, vmiObj client.Object) error {
			vmi := vmiObj.(*vmopv1.VirtualMachineImage)

			// Use Firmware field to verify the provider function is called.
			vmi.Status.Firmware = firmwareValue
			return nil
		}

		clItem = utils.DummyContentLibraryItem(utils.ItemFieldNamePrefix+"-dummy", "dummy-ns")
		clItem.Finalizers = []string{utils.ContentLibraryItemVmopFinalizer}
	})

	JustBeforeEach(func() {
		Expect(ctx.Client.Create(ctx, clItem)).To(Succeed())

		imageName, err := utils.GetImageFieldNameFromItem(clItem.Name)
		Expect(err).ToNot(HaveOccurred())

		clItemCtx = &context.ContentLibraryItemContextA2{
			Context:      ctx,
			Logger:       ctx.Logger,
			CLItem:       clItem,
			ImageObjName: imageName,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		clItem = nil
		reconciler = nil
		fakeVMProvider.Reset()
	})

	Context("ReconcileNormal", func() {

		When("ContentLibraryItem doesn't have the VMOP finalizer", func() {
			BeforeEach(func() {
				clItem.Finalizers = nil
			})

			It("should add the finalizer", func() {
				Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

				Expect(clItem.Finalizers).To(ContainElement(utils.ContentLibraryItemVmopFinalizer))
			})
		})

		When("ContentLibraryItem is Not Ready", func() {
			BeforeEach(func() {
				clItem.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionFalse,
					},
				}
			})

			It("should mark VirtualMachineImage condition as provider not ready", func() {
				Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

				vmi := getVMI(ctx, clItemCtx)
				condition := conditions.Get(vmi, vmopv1.VirtualMachineImageProviderReadyCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderNotReadyReason))
			})
		})

		When("ContentLibraryItem is not security compliant", func() {

			BeforeEach(func() {
				clItem.Status.SecurityCompliance = pointer.Bool(false)
			})

			It("should mark VirtualMachineImage condition as provider security not compliant", func() {
				Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

				vmi := getVMI(ctx, clItemCtx)
				condition := conditions.Get(vmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
			})
		})

		When("SyncVirtualMachineImage returns an error", func() {

			BeforeEach(func() {
				fakeVMProvider.SyncVirtualMachineImageFn = func(_ goctx.Context, _, _ client.Object) error {
					return fmt.Errorf("sync-error")
				}
			})

			It("should mark VirtualMachineImage condition synced failed", func() {
				err := reconciler.ReconcileNormal(clItemCtx)
				Expect(err).To(MatchError("sync-error"))

				vmi := getVMI(ctx, clItemCtx)
				condition := conditions.Get(vmi, vmopv1.VirtualMachineImageSyncedCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageNotSyncedReason))
			})
		})

		When("ContentLibraryItem is ready and security complaint", func() {

			JustBeforeEach(func() {
				// The DummyContentLibraryItem() should meet these requirements.
				var readyCond *imgregv1a1.Condition
				for _, c := range clItemCtx.CLItem.Status.Conditions {
					if c.Type == imgregv1a1.ReadyCondition {
						c := c
						readyCond = &c
						break
					}
				}
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(corev1.ConditionTrue))

				// BMV: We don't use this field - only the Condition.
				// Expect(clItemCtx.CLItem.Status.Ready).To(BeTrue())

				Expect(clItemCtx.CLItem.Status.SecurityCompliance).To(Equal(pointer.Bool(true)))
			})

			When("VirtualMachineImage resource has not been created yet", func() {

				It("should create a new VirtualMachineImage syncing up with ContentLibraryItem", func() {
					Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

					vmi := getVMI(ctx, clItemCtx)
					assertVMImageFromCLItem(vmi, clItemCtx.CLItem)
					Expect(vmi.Status.Firmware).To(Equal(firmwareValue))
				})
			})

			When("VirtualMachineImage resource is exists but not up-to-date", func() {

				JustBeforeEach(func() {
					vmi := &vmopv1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clItemCtx.ImageObjName,
							Namespace: clItem.Namespace,
						},
						Spec: vmopv1.VirtualMachineImageSpec{
							ProviderRef: common.LocalObjectRef{
								Name: "bogus",
							},
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ProviderContentVersion: "stale",
							Firmware:               "should-be-updated",
						},
					}
					Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
				})

				It("should update the existing VirtualMachineImage with ContentLibraryItem", func() {
					clItemCtx.CLItem.Status.ContentVersion += "-updated"
					Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

					vmi := getVMI(ctx, clItemCtx)
					assertVMImageFromCLItem(vmi, clItemCtx.CLItem)
					Expect(vmi.Status.Firmware).To(Equal(firmwareValue))
				})
			})

			When("VirtualMachineImage resource is created and already up-to-date", func() {

				JustBeforeEach(func() {
					vmi := &vmopv1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      clItemCtx.ImageObjName,
							Namespace: clItemCtx.CLItem.Namespace,
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ProviderContentVersion: clItemCtx.CLItem.Status.ContentVersion,
							Firmware:               "should-not-be-updated",
						},
					}
					Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
				})

				It("should skip updating the VirtualMachineImage with library item", func() {
					fakeVMProvider.SyncVirtualMachineImageFn = func(_ goctx.Context, _, _ client.Object) error {
						// Should not be called since the content versions match.
						return fmt.Errorf("sync-error")
					}

					Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())

					vmi := getVMI(ctx, clItemCtx)
					Expect(vmi.Status.Firmware).To(Equal("should-not-be-updated"))
				})
			})
		})
	})

	Context("ReconcileDelete", func() {

		It("should remove the finalizer from ContentLibraryItem resource", func() {
			Expect(clItem.Finalizers).To(ContainElement(utils.ContentLibraryItemVmopFinalizer))

			Expect(reconciler.ReconcileDelete(clItemCtx)).To(Succeed())
			Expect(clItem.Finalizers).ToNot(ContainElement(utils.ContentLibraryItemVmopFinalizer))
		})
	})
}

func getVMI(
	ctx *builder.UnitTestContextForController,
	clItemCtx *context.ContentLibraryItemContextA2) *vmopv1.VirtualMachineImage {

	vmi := &vmopv1.VirtualMachineImage{}
	Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: clItemCtx.ImageObjName, Namespace: clItemCtx.CLItem.Namespace}, vmi)).To(Succeed())
	return vmi
}

func assertVMImageFromCLItem(
	vmi *vmopv1.VirtualMachineImage,
	clItem *imgregv1a1.ContentLibraryItem) {

	Expect(metav1.IsControlledBy(vmi, clItem)).To(BeTrue())

	By("Expected VMImage Spec", func() {
		Expect(vmi.Spec.ProviderRef.Name).To(Equal(clItem.Name))
		Expect(vmi.Spec.ProviderRef.APIVersion).To(Equal(clItem.APIVersion))
		Expect(vmi.Spec.ProviderRef.Kind).To(Equal(clItem.Kind))
	})

	By("Expected VMImage Status", func() {
		Expect(vmi.Status.Name).To(Equal(clItem.Status.Name))
		Expect(vmi.Status.ProviderItemID).To(BeEquivalentTo(clItem.Spec.UUID))
		Expect(vmi.Status.ProviderContentVersion).To(Equal(clItem.Status.ContentVersion))

		Expect(conditions.IsTrue(vmi, vmopv1.VirtualMachineImageProviderReadyCondition)).To(BeTrue())
		Expect(conditions.IsTrue(vmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)).To(BeTrue())
		Expect(conditions.IsTrue(vmi, vmopv1.VirtualMachineImageSyncedCondition)).To(BeTrue())
	})
}
