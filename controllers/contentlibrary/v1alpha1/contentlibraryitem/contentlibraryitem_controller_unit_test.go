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
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/contentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ContentLibraryItem controller unit tests", unitTestsReconcile)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *contentlibraryitem.Reconciler
		fakeVMProvider *providerfake.VMProvider

		clItem *imgregv1a1.ContentLibraryItem
	)

	BeforeEach(func() {
		// The name for clItem must have the expected prefix to be parsed by the controller.
		clItemName := fmt.Sprintf("%s-%s", utils.ItemFieldNamePrefix, "dummy")
		clItem = utils.DummyContentLibraryItem(clItemName, "dummy-ns")
		// Adding the finalizer here to avoid ReconcileNormal returning early without resolving image resource.
		clItem.Finalizers = []string{utils.ContentLibraryItemVmopFinalizer}
		initObjects = []client.Object{clItem}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = contentlibraryitem.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.SyncVirtualMachineImageFn = func(_ goctx.Context, _, vmi client.Object) error {
			vmiObj := vmi.(*vmopv1.VirtualMachineImage)
			// Change a random spec and status field to verify the provider function is called.
			vmiObj.Spec.HardwareVersion = 123
			vmiObj.Status.ImageSupported = &[]bool{true}[0]
			return nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		clItem = nil
		reconciler = nil
		fakeVMProvider.Reset()
	})

	Context("ReconcileNormal", func() {

		JustBeforeEach(func() {
			clItemCtx := &context.ContentLibraryItemContext{
				Context:      ctx,
				Logger:       ctx.Logger,
				CLItem:       clItem,
				ImageObjName: utils.GetTestVMINameFrom(clItem.Name),
			}
			Expect(reconciler.ReconcileNormal(clItemCtx)).To(Succeed())
		})

		When("ContentLibraryItem doesn't have the VMOP finalizer", func() {

			BeforeEach(func() {
				clItem.Finalizers = []string{}
			})

			It("should add the finalizer", func() {
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
				unreadyVMI := getVMIFromCLItem(*ctx, clItem)
				providerCondition := conditions.Get(unreadyVMI, vmopv1.VirtualMachineImageProviderReadyCondition)
				Expect(providerCondition).ToNot(BeNil())
				Expect(providerCondition.Status).To(Equal(corev1.ConditionFalse))
				Expect(providerCondition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderNotReadyReason))
			})
		})

		When("ContentLibraryItem is not security compliant", func() {

			BeforeEach(func() {
				clItem.Status.SecurityCompliance = &[]bool{false}[0]
			})

			It("should mark VirtualMachineImage condition as provider security not compliant", func() {
				vmi := getVMIFromCLItem(*ctx, clItem)
				condition := conditions.Get(vmi, vmopv1.VirtualMachineImageProviderSecurityComplianceCondition)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				Expect(condition.Reason).To(Equal(vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason))
			})
		})

		// ContentLibraryItem clItem, which is created from DummyContentLibraryItem() already has all conditions satisfied.
		When("ContentLibraryItem is ready and security compliant", func() {
			When("VirtualMachineImage resource has not been created yet", func() {
				It("should create a new VirtualMachineImage syncing up with ContentLibraryItem", func() {
					createdVMI := getVMIFromCLItem(*ctx, clItem)
					expectedVMI := utils.GetExpectedVMIFrom(*clItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedVMI, createdVMI)

					Expect(createdVMI.Name).To(Equal(expectedVMI.Name))
					Expect(createdVMI.OwnerReferences).To(Equal(expectedVMI.OwnerReferences))
					Expect(createdVMI.Spec).To(Equal(expectedVMI.Spec))
					Expect(createdVMI.Status).To(Equal(expectedVMI.Status))
				})
			})

			When("VirtualMachineImage resource is created but not up-to-date", func() {
				BeforeEach(func() {
					vmiName := utils.GetTestVMINameFrom(clItem.Name)
					existingVMI := &vmopv1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      vmiName,
							Namespace: clItem.Namespace,
						},
						Status: vmopv1.VirtualMachineImageStatus{
							ContentVersion: "dummy-old",
						},
					}
					initObjects = append(initObjects, existingVMI)
				})

				It("should update the existing VirtualMachineImage with ContentLibraryItem", func() {
					updatedVMI := getVMIFromCLItem(*ctx, clItem)
					expectedVMI := utils.GetExpectedVMIFrom(*clItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedVMI, updatedVMI)

					Expect(updatedVMI.Name).To(Equal(expectedVMI.Name))
					Expect(updatedVMI.OwnerReferences).To(Equal(expectedVMI.OwnerReferences))
					Expect(updatedVMI.Spec).To(Equal(expectedVMI.Spec))
					Expect(updatedVMI.Status).To(Equal(expectedVMI.Status))
				})
			})

			When("VirtualMachineImage resource is created and already up-to-date", func() {
				BeforeEach(func() {
					// The following fields are set from the VMProvider by downloading the library item.
					// These fields should remain as is if the VMProvider update is skipped below.
					upToDateVMI := utils.GetExpectedVMIFrom(*clItem, fakeVMProvider.SyncVirtualMachineImageFn)
					upToDateVMI.Spec.HardwareVersion = 0
					upToDateVMI.Status.ImageSupported = nil
					initObjects = append(initObjects, upToDateVMI)
				})

				It("should skip updating the VirtualMachineImage with library item", func() {
					currentVMI := getVMIFromCLItem(*ctx, clItem)
					expectedVMI := utils.GetExpectedVMIFrom(*clItem, fakeVMProvider.SyncVirtualMachineImageFn)
					utils.PopulateRuntimeFieldsTo(expectedVMI, currentVMI)

					Expect(currentVMI.Name).To(Equal(expectedVMI.Name))
					Expect(currentVMI.OwnerReferences).To(Equal(expectedVMI.OwnerReferences))
					Expect(currentVMI.Spec.HardwareVersion).To(BeZero())
					Expect(currentVMI.Status.ImageSupported).To(BeNil())
				})
			})
		})
	})

	Context("ReconcileDelete", func() {

		JustBeforeEach(func() {
			clItemCtx := &context.ContentLibraryItemContext{
				Context: ctx,
				Logger:  ctx.Logger,
				CLItem:  clItem,
			}
			Expect(reconciler.ReconcileDelete(clItemCtx)).To(Succeed())
		})

		It("should remove the finalizer from ContentLibraryItem resource", func() {
			Expect(clItem.Finalizers).ToNot(ContainElement(utils.ContentLibraryItemVmopFinalizer))
		})
	})
}

func getVMIFromCLItem(
	ctx builder.UnitTestContextForController,
	clItem *imgregv1a1.ContentLibraryItem) *vmopv1.VirtualMachineImage {

	vmiName := utils.GetTestVMINameFrom(clItem.Name)
	vmi := &vmopv1.VirtualMachineImage{}
	Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vmiName, Namespace: clItem.Namespace}, vmi)).To(Succeed())

	return vmi
}
