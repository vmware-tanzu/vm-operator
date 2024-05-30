// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.V1Alpha3,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const firmwareValue = "my-firmware"

	var (
		ctx *builder.UnitTestContextForController

		reconciler     *clustercontentlibraryitem.Reconciler
		fakeVMProvider *providerfake.VMProvider

		cclItem    *imgregv1a1.ClusterContentLibraryItem
		cclItemCtx *pkgctx.ClusterContentLibraryItemContext
	)

	BeforeEach(func() {
		ctx = suite.NewUnitTestContextForController()

		reconciler = clustercontentlibraryitem.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)

		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, cvmiObj client.Object) error {
			cvmi := cvmiObj.(*vmopv1.ClusterVirtualMachineImage)
			// Use Firmware field to verify the provider function is called.
			cvmi.Status.Firmware = firmwareValue
			return nil
		}

		cclItem = utils.DummyClusterContentLibraryItem(utils.ItemFieldNamePrefix + "-dummy")
		// Add our finalizer so ReconcileNormal() does not return early.
		cclItem.Finalizers = []string{utils.CCLItemFinalizer}
	})

	JustBeforeEach(func() {
		Expect(ctx.Client.Create(ctx, cclItem)).To(Succeed())

		imageName, err := utils.GetImageFieldNameFromItem(cclItem.Name)
		Expect(err).ToNot(HaveOccurred())

		cclItemCtx = &pkgctx.ClusterContentLibraryItemContext{
			Context:      ctx,
			Logger:       ctx.Logger,
			CCLItem:      cclItem,
			ImageObjName: imageName,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		cclItem = nil
		reconciler = nil
		fakeVMProvider.Reset()
	})

	Context("ReconcileNormal", func() {

		When("ClusterContentLibraryItem doesn't have the VMOP finalizer", func() {

			BeforeEach(func() {
				cclItem.Finalizers = nil
			})

			It("should add the finalizer", func() {
				Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

				Expect(cclItem.Finalizers).To(ContainElement(utils.CCLItemFinalizer))
			})
		})

		When("ClusterContentLibraryItem is Not Ready", func() {

			BeforeEach(func() {
				cclItem.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionFalse,
					},
				}
			})

			It("should mark ClusterVirtualMachineImage condition as provider not ready", func() {
				Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

				cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
				condition := conditions.Get(cvmi, vmopv1.ReadyConditionType)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderNotReadyReason))
			})
		})

		When("ClusterContentLibraryItem is not security compliant", func() {

			BeforeEach(func() {
				cclItem.Status.SecurityCompliance = ptr.To(false)
			})

			It("should mark ClusterVirtualMachineImage condition as provider security not compliant", func() {
				Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

				cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
				condition := conditions.Get(cvmi, vmopv1.ReadyConditionType)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
			})

			When("ClusterContentLibraryItem is Not Ready", func() {

				BeforeEach(func() {
					cclItem.Status.Conditions = []imgregv1a1.Condition{
						{
							Type:   imgregv1a1.ReadyCondition,
							Status: corev1.ConditionFalse,
						},
					}
				})

				It("should mark the Ready condition as false", func() {
					Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

					cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
					readyCondition := conditions.Get(cvmi, vmopv1.ReadyConditionType)
					Expect(readyCondition).ToNot(BeNil())
					Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
					Expect(readyCondition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
				})

			})
		})

		When("SyncVirtualMachineImage returns an error", func() {

			BeforeEach(func() {
				fakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, _ client.Object) error {
					return fmt.Errorf("sync-error")
				}
			})

			It("should mark ClusterVirtualMachineImage Ready condition as failed", func() {
				err := reconciler.ReconcileNormal(cclItemCtx)
				Expect(err).To(MatchError("sync-error"))

				cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
				condition := conditions.Get(cvmi, vmopv1.ReadyConditionType)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageNotSyncedReason))
			})
		})

		When("ClusterContentLibraryItem is ready and security complaint", func() {

			JustBeforeEach(func() {
				// The DummyClusterContentLibraryItem() should meet these requirements.
				var readyCond *imgregv1a1.Condition
				for _, c := range cclItemCtx.CCLItem.Status.Conditions {
					if c.Type == imgregv1a1.ReadyCondition {
						c := c
						readyCond = &c
						break
					}
				}
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(corev1.ConditionTrue))

				Expect(cclItemCtx.CCLItem.Status.SecurityCompliance).To(Equal(ptr.To(true)))
			})

			When("ClusterVirtualMachineImage resource has not been created yet", func() {

				It("should create a new ClusterVirtualMachineImage syncing up with ClusterContentLibraryItem", func() {
					Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

					cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
					assertCVMImageFromCCLItem(cvmi, cclItemCtx.CCLItem)
					Expect(cvmi.Status.Firmware).To(Equal(firmwareValue))
				})
			})

			When("ClusterVirtualMachineImage resource is exists but not up-to-date", func() {

				JustBeforeEach(func() {
					cvmi := &vmopv1.ClusterVirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: cclItemCtx.ImageObjName,
						},
						Spec: vmopv1.VirtualMachineImageSpec{
							ProviderRef: &common.LocalObjectRef{
								Name: "bogus",
							},
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ProviderContentVersion: "stale",
							Firmware:               "should-be-updated",
						},
					}
					Expect(ctx.Client.Create(ctx, cvmi)).To(Succeed())
				})

				It("should update the existing ClusterVirtualMachineImage with ClusterContentLibraryItem", func() {
					cclItemCtx.CCLItem.Status.ContentVersion += "-updated"
					Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

					cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
					assertCVMImageFromCCLItem(cvmi, cclItemCtx.CCLItem)
					Expect(cvmi.Status.Firmware).To(Equal(firmwareValue))
				})
			})

			When("ClusterVirtualMachineImage resource is created and already up-to-date", func() {

				JustBeforeEach(func() {
					cvmi := &vmopv1.ClusterVirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: cclItemCtx.ImageObjName,
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ProviderContentVersion: cclItemCtx.CCLItem.Status.ContentVersion,
							Firmware:               "should-not-be-updated",
						},
					}
					Expect(ctx.Client.Create(ctx, cvmi)).To(Succeed())
				})

				It("should skip updating the ClusterVirtualMachineImage with library item", func() {
					fakeVMProvider.SyncVirtualMachineImageFn = func(_ context.Context, _, _ client.Object) error {
						// Should not be called since the content versions match.
						return fmt.Errorf("sync-error")
					}

					Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())

					cvmi := getClusterVMI(ctx, cclItemCtx.ImageObjName)
					Expect(cvmi.Status.Firmware).To(Equal("should-not-be-updated"))
				})
			})
		})
	})

	Context("ReconcileDelete", func() {

		It("should remove the finalizer from ClusterContentLibraryItem resource", func() {
			Expect(cclItem.Finalizers).To(ContainElement(utils.CCLItemFinalizer))

			Expect(reconciler.ReconcileDelete(cclItemCtx)).To(Succeed())
			Expect(cclItem.Finalizers).ToNot(ContainElement(utils.CCLItemFinalizer))
		})
	})
}

func getClusterVMI(ctx *builder.UnitTestContextForController, name string) *vmopv1.ClusterVirtualMachineImage {
	cvmi := &vmopv1.ClusterVirtualMachineImage{}
	Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: name}, cvmi)).To(Succeed())
	return cvmi
}

func assertCVMImageFromCCLItem(
	cvmi *vmopv1.ClusterVirtualMachineImage,
	cclItem *imgregv1a1.ClusterContentLibraryItem) {

	Expect(metav1.IsControlledBy(cvmi, cclItem)).To(BeTrue())
	for k := range utils.FilterServicesTypeLabels(cclItem.Labels) {
		Expect(cvmi.Labels).To(HaveKey(k))
	}

	By("Expected ClusterVMImage Spec", func() {
		Expect(cvmi.Spec.ProviderRef.Name).To(Equal(cclItem.Name))
		Expect(cvmi.Spec.ProviderRef.APIVersion).To(Equal(cclItem.APIVersion))
		Expect(cvmi.Spec.ProviderRef.Kind).To(Equal(cclItem.Kind))
	})

	By("Expected ClusterVMImage Status", func() {
		Expect(cvmi.Status.Name).To(Equal(cclItem.Status.Name))
		Expect(cvmi.Status.ProviderItemID).To(BeEquivalentTo(cclItem.Spec.UUID))
		Expect(cvmi.Status.ProviderContentVersion).To(Equal(cclItem.Status.ContentVersion))

		Expect(conditions.IsTrue(cvmi, vmopv1.ReadyConditionType)).To(BeTrue())
	})
}
