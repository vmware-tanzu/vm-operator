// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/utils"
	virtualmachinepublishrequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking VirtualMachinePublishRequest controller tests", virtualMachinePublishRequestReconcile)
}

func virtualMachinePublishRequestReconcile() {
	var (
		ctx   *builder.IntegrationTestContext
		vmpub *vmopv1.VirtualMachinePublishRequest
		vm    *vmopv1.VirtualMachine
		cl    *imgregv1a1.ContentLibrary
	)

	getVirtualMachinePublishRequest := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1.VirtualMachinePublishRequest {
		vmpubObj := &vmopv1.VirtualMachinePublishRequest{}
		if err := ctx.Client.Get(ctx, objKey, vmpubObj); err != nil {
			return nil
		}
		return vmpubObj
	}

	waitForVirtualMachinePublishRequestFinalizer := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) {
		Eventually(func() []string {
			if vmpub := getVirtualMachinePublishRequest(ctx, objKey); vmpub != nil {
				return vmpub.GetFinalizers()
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
				PowerState: vmopv1.VirtualMachinePoweredOn,
			},
		}

		vmpub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", ctx.Namespace, vm.Name,
			"dummy-item", "dummy-cl")
		cl = builder.DummyContentLibrary("dummy-cl", ctx.Namespace, "dummy-cl")
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		var (
			itemID string
		)

		Context("Successfully reconcile a VirtualMachinepublishRequest", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				vmObj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), vmObj)).To(Succeed())
				vmObj.Status = vmopv1.VirtualMachineStatus{
					Phase:    vmopv1.Created,
					UniqueID: "dummy-unique-id",
				}
				Expect(ctx.Client.Status().Update(ctx, vmObj)).To(Succeed())
				Expect(ctx.Client.Create(ctx, cl)).To(Succeed())

				cl.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionTrue,
					},
				}
				Expect(ctx.Client.Status().Update(ctx, cl)).To(Succeed())

				Expect(ctx.Client.Create(ctx, vmpub)).To(Succeed())

				itemID = uuid.New().String()
				go func() {
					// create ContentLibraryItem and VirtualMachineImage once the task succeeds.
					for {
						obj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
						if state := intgFakeVMProvider.GetVMPublishRequestResult(obj); state == types.TaskInfoStateSuccess {
							clitem := &imgregv1a1.ContentLibraryItem{}
							err := ctx.Client.Get(ctx, client.ObjectKey{Name: "dummy-clitem", Namespace: ctx.Namespace}, clitem)
							if k8serrors.IsNotFound(err) {
								clitem = utils.DummyContentLibraryItem("dummy-clitem", ctx.Namespace)
								Expect(ctx.Client.Create(ctx, clitem)).To(Succeed())

								vmi := builder.DummyVirtualMachineImage("dummy-image")
								vmi.Namespace = ctx.Namespace
								vmi.Spec.ImageID = itemID
								Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
								vmi.Status.ImageName = vmpub.Spec.Target.Item.Name
								Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())

								return
							}
						}
						time.Sleep(time.Second)
					}
				}()
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, vmpub)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, vm)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = ctx.Client.Delete(ctx, cl)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

				intgFakeVMProvider.Reset()
			})

			It("VirtualMachinePublishRequest completed", func() {
				// Wait for initial reconcile.
				waitForVirtualMachinePublishRequestFinalizer(ctx, client.ObjectKeyFromObject(vmpub))

				By("VM publish task is queued", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateQueued,
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					// Force trigger a reconcile.
					vmPubObj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubObj, func() error {
						vmPubObj.Annotations = map[string]string{"dummy": "dummy-1"}
						return nil
					})
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func() bool {
						vmPubObj = getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
						for _, condition := range vmPubObj.Status.Conditions {
							if condition.Type == vmopv1.VirtualMachinePublishRequestConditionUploaded {
								return condition.Status == corev1.ConditionFalse &&
									condition.Reason == vmopv1.UploadTaskQueuedReason &&
									vmPubObj.Status.Attempts == 1
							}
						}
						return false
					}).Should(BeTrue())
				})

				By("VM publish task is running", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateRunning,
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					// Force trigger a reconcile.
					vmPubObj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubObj, func() error {
						vmPubObj.Annotations = map[string]string{"dummy": "dummy-2"}
						return nil
					})
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func() bool {
						vmPubObj = getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
						for _, condition := range vmPubObj.Status.Conditions {
							if condition.Type == vmopv1.VirtualMachinePublishRequestConditionUploaded {
								return condition.Status == corev1.ConditionFalse &&
									condition.Reason == vmopv1.UploadingReason &&
									vmPubObj.Status.Attempts == 1
							}
						}
						return false
					}).Should(BeTrue())
				})

				By("VM publish task succeeded", func() {
					intgFakeVMProvider.Lock()
					intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateSuccess,
							Result:        types.ManagedObjectReference{Type: "ContentLibraryItem", Value: fmt.Sprintf("clibitem-%s", itemID)},
						}
						return []types.TaskInfo{task}, nil
					}
					intgFakeVMProvider.Unlock()

					// Force trigger a reconcile.
					vmPubObj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
					_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubObj, func() error {
						vmPubObj.Annotations = map[string]string{"dummy": "dummy-3"}
						return nil
					})
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func(g Gomega) {
						obj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
						g.Expect(obj).ToNot(BeNil())
						g.Expect(conditions.IsTrue(obj, vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeTrue())
						g.Expect(obj.Status.Ready).To(BeTrue())
						g.Expect(obj.Status.ImageName).To(Equal("dummy-image"))
						g.Expect(obj.Status.CompletionTime).NotTo(BeZero())
					}).Should(Succeed())
				})
			})
		})

		It("Reconciles after VirtualMachinePublishRequest deletion", func() {
			Expect(ctx.Client.Create(ctx, vmpub)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachinePublishRequestFinalizer(ctx, client.ObjectKeyFromObject(vmpub))

			Expect(ctx.Client.Delete(ctx, vmpub)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if vmpub := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub)); vmpub != nil {
						return vmpub.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizerName))
			})
		})
	})
}
