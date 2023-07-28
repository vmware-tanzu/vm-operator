// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	virtualmachinepublishrequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest/v1alpha2"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking VirtualMachinePublishRequest controller tests", virtualMachinePublishRequestReconcile)
}

func virtualMachinePublishRequestReconcile() {
	var (
		ctx      *builder.IntegrationTestContext
		vmPubReq *vmopv1.VirtualMachinePublishRequest
		vm       *vmopv1.VirtualMachine
		cl       *imgregv1a1.ContentLibrary
	)

	getVirtualMachinePublishRequest := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1.VirtualMachinePublishRequest {
		req := &vmopv1.VirtualMachinePublishRequest{}
		if err := ctx.Client.Get(ctx, objKey, req); err != nil {
			return nil
		}
		return req
	}

	waitForVirtualMachinePublishRequestFinalizer := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) {
		Eventually(func() []string {
			if req := getVirtualMachinePublishRequest(ctx, objKey); req != nil {
				return req.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizerName), "waiting for VirtualMachinePublishRequest finalizer")
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				ClassName:  "dummy-class",
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
		}

		cl = builder.DummyContentLibrary("dummy-cl", ctx.Namespace, "dummy-cl-uuid")

		vmPubReq = builder.DummyVirtualMachinePublishRequestA2(
			"dummy-vmpub", ctx.Namespace,
			vm.Name,
			"dummy-item",
			cl.Name)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {

		Context("Successfully reconcile a VirtualMachinePublishRequest", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				vm.Status.UniqueID = "dummy-vm-unique-id"
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				Expect(ctx.Client.Create(ctx, cl)).To(Succeed())
				cl.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionTrue,
					},
				}
				Expect(ctx.Client.Status().Update(ctx, cl)).To(Succeed())

				Expect(ctx.Client.Create(ctx, vmPubReq)).To(Succeed())
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, vmPubReq)
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
				err = ctx.Client.Delete(ctx, vm)
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
				err = ctx.Client.Delete(ctx, cl)
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

				intgFakeVMProvider.Reset()
			})

			It("VirtualMachinePublishRequest completed", func() {
				// Wait for initial reconcile.
				waitForVirtualMachinePublishRequestFinalizer(ctx, client.ObjectKeyFromObject(vmPubReq))

				By("PublishVM should be called", func() {
					Eventually(intgFakeVMProvider.IsPublishVMCalled).Should(BeTrue())
				})

				By("VM publish task is queued", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) ([]types.TaskInfo, error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateQueued,
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					// Force trigger a reconcile.
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubReq, func() error {
						vmPubReq.Annotations = map[string]string{"dummy": "dummy-1"}
						return nil
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						req := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmPubReq))
						g.Expect(req).ToNot(BeNil())
						g.Expect(req.Status.Attempts).To(BeEquivalentTo(1))

						condition := conditions.Get(req, vmopv1.VirtualMachinePublishRequestConditionUploaded)
						g.Expect(condition).ToNot(BeNil())
						g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						g.Expect(condition.Reason).To(Equal(vmopv1.UploadTaskQueuedReason))
					}).Should(Succeed())
				})

				By("VM publish task is running", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) ([]types.TaskInfo, error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateRunning,
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					// Force trigger a reconcile.
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubReq, func() error {
						vmPubReq.Annotations = map[string]string{"dummy": "dummy-2"}
						return nil
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(func(g Gomega) {
						req := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmPubReq))
						g.Expect(req).ToNot(BeNil())
						g.Expect(req.Status.Attempts).To(BeEquivalentTo(1))

						condition := conditions.Get(req, vmopv1.VirtualMachinePublishRequestConditionUploaded)
						g.Expect(condition).ToNot(BeNil())
						g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
						g.Expect(condition.Reason).To(Equal(vmopv1.UploadingReason))
					}).Should(Succeed())
				})

				itemID := uuid.New().String()

				By("VM publish task succeeded", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) ([]types.TaskInfo, error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateSuccess,
							Result:        types.ManagedObjectReference{Type: "ContentLibraryItem", Value: fmt.Sprintf("clibitem-%s", itemID)},
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					By("Simulate ContentLibrary and VM Image reconcile", func() {
						clItem := utils.DummyContentLibraryItem("dummy-clitem", ctx.Namespace)
						Expect(ctx.Client.Create(ctx, clItem)).To(Succeed())

						vmi := builder.DummyVirtualMachineImageA2("dummy-image")
						vmi.Namespace = ctx.Namespace
						Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
						targetName := vmPubReq.Spec.Target.Item.Name
						Expect(targetName).ToNot(BeEmpty())
						vmi.Status.Name = targetName
						vmi.Status.ProviderItemID = itemID
						Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())
					})

					// Force trigger a reconcile.
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubReq, func() error {
						vmPubReq.Annotations = map[string]string{"dummy": "dummy-3"}
						return nil
					})
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func(g Gomega) {
						req := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmPubReq))
						g.Expect(req).ToNot(BeNil())

						g.Expect(conditions.IsTrue(req, vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeTrue())
						g.Expect(req.Status.Ready).To(BeTrue())
						g.Expect(req.Status.ImageName).To(Equal("dummy-image"))
						g.Expect(req.Status.CompletionTime).NotTo(BeZero())
					}).Should(Succeed())
				})
			})
		})

		It("Reconciles after VirtualMachinePublishRequest deletion", func() {
			Expect(ctx.Client.Create(ctx, vmPubReq)).To(Succeed())

			// Wait for initial reconcile.
			waitForVirtualMachinePublishRequestFinalizer(ctx, client.ObjectKeyFromObject(vmPubReq))

			Expect(ctx.Client.Delete(ctx, vmPubReq)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if req := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmPubReq)); req != nil {
						return req.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizerName))
			})
		})
	})
}
