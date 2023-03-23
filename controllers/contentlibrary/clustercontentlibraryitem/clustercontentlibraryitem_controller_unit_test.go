// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ClusterContentLibraryItem controller unit tests", unitTestsReconcile)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *clustercontentlibraryitem.Reconciler
		fakeVMProvider *providerfake.VMProvider

		cclItem *imgregv1a1.ClusterContentLibraryItem
	)

	BeforeEach(func() {
		// The name for cclItem must have the expected prefix to be parsed by the controller.
		cclItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, "dummy")
		cclItem = utils.DummyClusterContentLibraryItem(cclItemName)
		initObjects = []client.Object{cclItem}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = clustercontentlibraryitem.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.SyncVirtualMachineImageFn = func(
			_ context.Context, _, cvmiObj client.Object) error {
			cvmi := cvmiObj.(*vmopv1.ClusterVirtualMachineImage)
			// Change a random spec and status field to verify the provider function is called.
			cvmi.Spec.HardwareVersion = 123
			cvmi.Status.ImageSupported = &[]bool{true}[0]
			return nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		cclItem = nil
		reconciler = nil
		fakeVMProvider.Reset()
	})

	Context("ReconcileNormal", func() {

		When("ClusterContentLibraryItem does not have the `securityCompliance` field to be true", func() {

			BeforeEach(func() {
				cclItem.Status.SecurityCompliance = &[]bool{false}[0]
			})

			When("No existing ClusterVirtualMachineImage found", func() {

				It("should skip delete ClusterVirtualMachineImage", func() {
					err := reconciler.ReconcileNormal(ctx, cclItem)
					Expect(err).ToNot(HaveOccurred())

					cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
					Expect(cvmiList.Items).To(HaveLen(0))
				})
			})

			When("Existing ClusterVirtualMachineImage found", func() {

				BeforeEach(func() {
					cvmiName := utils.GetTestVMINameFrom(cclItem.Name)
					existingCVMI := &vmopv1.ClusterVirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: cvmiName,
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ContentVersion: "dummy-old",
						},
					}
					initObjects = append(initObjects, existingCVMI)
				})

				It("should delete the ClusterVirtualMachineImage", func() {
					err := reconciler.ReconcileNormal(ctx, cclItem)
					Expect(err).ToNot(HaveOccurred())

					cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
					Expect(cvmiList.Items).To(HaveLen(0))
				})
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
				err := reconciler.ReconcileNormal(ctx, cclItem)
				Expect(err).ToNot(HaveOccurred())

				cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
				Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
				Expect(cvmiList.Items).To(HaveLen(1))
				unreadyCVMI := cvmiList.Items[0]
				providerCondition := conditions.Get(&unreadyCVMI, vmopv1.VirtualMachineImageProviderReadyCondition)
				Expect(providerCondition).ToNot(BeNil())
				Expect(providerCondition.Status).To(Equal(corev1.ConditionFalse))

			})
		})

		When("ClusterContentLibraryItem is in Ready condition", func() {
			BeforeEach(func() {
				cclItem.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionTrue,
					},
				}
			})

			When("ClusterVirtualMachineImage resource has not been created yet", func() {

				It("should create a new ClusterVirtualMachineImage syncing up with ClusterContentLibraryItem", func() {
					err := reconciler.ReconcileNormal(ctx, cclItem)
					Expect(err).ToNot(HaveOccurred())
					cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
					Expect(cvmiList.Items).To(HaveLen(1))
					createdCVMI := cvmiList.Items[0]
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, &createdCVMI)

					Expect(createdCVMI.Name).To(Equal(expectedCVMI.Name))
					Expect(createdCVMI.Labels).To(Equal(expectedCVMI.Labels))
					Expect(createdCVMI.OwnerReferences).To(Equal(expectedCVMI.OwnerReferences))
					Expect(createdCVMI.Spec).To(Equal(expectedCVMI.Spec))
					Expect(createdCVMI.Status).To(Equal(expectedCVMI.Status))
				})
			})

			When("ClusterVirtualMachineImage resource is created but not up-to-date", func() {

				BeforeEach(func() {
					cvmiName := utils.GetTestVMINameFrom(cclItem.Name)
					existingCVMI := &vmopv1.ClusterVirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: cvmiName,
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ContentVersion: "dummy-old",
						},
					}
					initObjects = append(initObjects, existingCVMI)
				})

				It("should update the existing ClusterVirtualMachineImage with ClusterContentLibraryItem", func() {
					err := reconciler.ReconcileNormal(ctx, cclItem)
					Expect(err).ToNot(HaveOccurred())

					cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
					Expect(cvmiList.Items).To(HaveLen(1))
					updatedCVMI := cvmiList.Items[0]
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, &updatedCVMI)

					Expect(updatedCVMI.Name).To(Equal(expectedCVMI.Name))
					Expect(updatedCVMI.Labels).To(Equal(expectedCVMI.Labels))
					Expect(updatedCVMI.OwnerReferences).To(Equal(expectedCVMI.OwnerReferences))
					Expect(updatedCVMI.Spec).To(Equal(expectedCVMI.Spec))
					Expect(updatedCVMI.Status).To(Equal(expectedCVMI.Status))
				})
			})

			When("ClusterVirtualMachineImage resource is created and already up-to-date", func() {

				BeforeEach(func() {
					// The following fields are set from the VMProvider by downloading the library item.
					// These fields should remain as is if the VMProvider update is skipped below.
					upToDateCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					upToDateCVMI.Spec.HardwareVersion = 0
					upToDateCVMI.Status.ImageSupported = nil
					initObjects = append(initObjects, upToDateCVMI)
				})

				It("should skip updating the ClusterVirtualMachineImage with library item", func() {
					err := reconciler.ReconcileNormal(ctx, cclItem)
					Expect(err).ToNot(HaveOccurred())

					cvmiList := &vmopv1.ClusterVirtualMachineImageList{}
					Expect(ctx.Client.List(ctx, cvmiList)).To(Succeed())
					Expect(cvmiList.Items).To(HaveLen(1))
					currentCVMI := cvmiList.Items[0]
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, &currentCVMI)

					Expect(currentCVMI.Name).To(Equal(expectedCVMI.Name))
					Expect(currentCVMI.Labels).To(Equal(expectedCVMI.Labels))
					Expect(currentCVMI.OwnerReferences).To(Equal(expectedCVMI.OwnerReferences))
					Expect(currentCVMI.Spec.HardwareVersion).To(BeZero())
					Expect(currentCVMI.Status.ImageSupported).To(BeNil())
				})
			})
		})
	})
}
