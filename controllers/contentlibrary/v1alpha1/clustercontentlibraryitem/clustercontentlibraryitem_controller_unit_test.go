// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem_test

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
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
		// Adding the finalizer here to avoid ReconcileNormal returning early without resolving image resource.
		cclItem.Finalizers = []string{utils.ClusterContentLibraryItemVmopFinalizer}
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
			_ goctx.Context, _, cvmiObj client.Object) error {
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

		JustBeforeEach(func() {
			cclItemCtx := &context.ClusterContentLibraryItemContext{
				Context:      ctx,
				Logger:       ctx.Logger,
				CCLItem:      cclItem,
				ImageObjName: utils.GetTestVMINameFrom(cclItem.Name),
			}
			Expect(reconciler.ReconcileNormal(cclItemCtx)).To(Succeed())
		})

		When("ClusterContentLibraryItem doesn't have the VMOP finalizer", func() {

			BeforeEach(func() {
				cclItem.Finalizers = []string{}
			})

			It("should add the finalizer", func() {
				Expect(cclItem.Finalizers).To(ContainElement(utils.ClusterContentLibraryItemVmopFinalizer))
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
				cvmi := getCVMIFromCCLItem(*ctx, cclItem)
				condition := conditions.Get(cvmi, vmopv1.VirtualMachineImageProviderReadyCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderNotReadyReason))

			})
		})

		When("ClusterContentLibraryItem is not security compliant", func() {

			BeforeEach(func() {
				cclItem.Status.SecurityCompliance = &[]bool{false}[0]
			})

			It("should mark ClusterVirtualMachineImage condition as provider security not compliant", func() {
				cvmi := getCVMIFromCCLItem(*ctx, cclItem)
				condition := conditions.Get(cvmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
			})

		})

		// ClusterContentLibraryItem cclItem, which is created from DummyClusterContentLibraryItem() already has all conditions satisfied.
		When("ClusterContentLibraryItem is ready and security complaint", func() {

			When("ClusterVirtualMachineImage resource has not been created yet", func() {

				It("should create a new ClusterVirtualMachineImage syncing up with ClusterContentLibraryItem", func() {
					createdCVMI := getCVMIFromCCLItem(*ctx, cclItem)
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, createdCVMI)

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
					updatedCVMI := getCVMIFromCCLItem(*ctx, cclItem)
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, updatedCVMI)

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
					currentCVMI := getCVMIFromCCLItem(*ctx, cclItem)
					expectedCVMI := utils.GetExpectedCVMIFrom(*cclItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedCVMI, currentCVMI)

					Expect(currentCVMI.Name).To(Equal(expectedCVMI.Name))
					Expect(currentCVMI.Labels).To(Equal(expectedCVMI.Labels))
					Expect(currentCVMI.OwnerReferences).To(Equal(expectedCVMI.OwnerReferences))
					Expect(currentCVMI.Spec.HardwareVersion).To(BeZero())
					Expect(currentCVMI.Status.ImageSupported).To(BeNil())
				})
			})
		})
	})

	Context("ReconcileDelete", func() {

		JustBeforeEach(func() {
			cclItemCtx := &context.ClusterContentLibraryItemContext{
				Context: ctx,
				Logger:  ctx.Logger,
				CCLItem: cclItem,
			}
			Expect(reconciler.ReconcileDelete(cclItemCtx)).To(Succeed())
		})

		It("should remove the finalizer from ClusterContentLibraryItem resource", func() {
			Expect(cclItem.Finalizers).ToNot(ContainElement(utils.ClusterContentLibraryItemVmopFinalizer))
		})
	})
}

func getCVMIFromCCLItem(
	ctx builder.UnitTestContextForController,
	cclItem *imgregv1a1.ClusterContentLibraryItem) *vmopv1.ClusterVirtualMachineImage {

	cvmiName := utils.GetTestVMINameFrom(cclItem.Name)
	cvmi := &vmopv1.ClusterVirtualMachineImage{}
	Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: cvmiName}, cvmi)).To(Succeed())
	return cvmi
}
