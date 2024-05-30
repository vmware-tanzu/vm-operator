// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const finalizerName = "vmoperator.vmware.com/virtualmachinepublishrequest"

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
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *virtualmachinepublishrequest.Reconciler
		fakeVMProvider *providerfake.VMProvider

		vm       *vmopv1.VirtualMachine
		vmpub    *vmopv1.VirtualMachinePublishRequest
		cl       *imgregv1a1.ContentLibrary
		vmpubCtx *pkgctx.VirtualMachinePublishRequestContext
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Status: vmopv1.VirtualMachineStatus{
				UniqueID: "dummy-id",
			},
		}

		vmpub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", vm.Namespace, vm.Name, "dummy-item", "dummy-cl")
		cl = builder.DummyContentLibrary("dummy-cl", vm.Namespace, "dummy-id")
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachinepublishrequest.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.Reset()

		vmpubCtx = &pkgctx.VirtualMachinePublishRequestContext{
			Context:          ctx,
			Logger:           ctx.Logger.WithName(vmpub.Name),
			VMPublishRequest: vmpub,
			VM:               vm,
			ContentLibrary:   cl,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, cl, vm, vmpub)
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vmpub.Finalizers = nil
			})

			It("will set finalizer", func() {
				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmpubCtx.VMPublishRequest.GetFinalizers()).To(ContainElement(finalizerName))
			})
		})

		It("will have finalizer set upon successful reconciliation and can be called multiple times", func() {
			_, err := reconciler.ReconcileNormal(vmpubCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmpubCtx.VMPublishRequest.GetFinalizers()).To(ContainElement(finalizerName))

			_, err = reconciler.ReconcileNormal(vmpubCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmpubCtx.VMPublishRequest.GetFinalizers()).To(ContainElement(finalizerName))
		})

		When("VMPubRequest update failed", func() {
			JustBeforeEach(func() {
				// mock update failure
				obj := vmpub.DeepCopy()
				obj.Annotations = map[string]string{"dummy": "dummy"}
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			})
			// This is relying on incorrect behavior in controller-runtime's fake client
			// which has been fixed by: https://github.com/kubernetes-sigs/controller-runtime/pull/2259
			// where a Status would change the entire object and the next Update will
			// fail due to conflict.  Comment out the test while we figure out a way to
			// mimic Update failure.
			XIt("Should not publish VM", func() {
				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// vmpub result should be empty because it never starts.
				Consistently(func() vimtypes.TaskInfoState {
					return fakeVMProvider.GetVMPublishRequestResult(vmpub)
				}).Should(BeEmpty())
			})
		})

		When("Source VM isn't valid", func() {
			BeforeEach(func() {
				initObjects = []client.Object{cl, vmpub}
			})

			It("returns error if source VM doesn't exist", func() {
				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// Update SourceValid condition.
				Expect(conditions.IsTrue(vmpub,
					vmopv1.VirtualMachinePublishRequestConditionSourceValid)).To(BeFalse())
			})

			When("Source VM has empty uniqueID", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = ""
					initObjects = append(initObjects, vm)
				})

				It("should return error and retry", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).To(HaveOccurred())

					// Update SourceValid condition.
					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionSourceValid)).To(BeFalse())
				})
			})
		})

		When("Target isn't valid", func() {
			BeforeEach(func() {
				initObjects = []client.Object{vm, vmpub}
			})

			It("returns error to retry if content library doesn't exist", func() {
				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// Update TargetValid condition.
				Expect(conditions.IsTrue(vmpub,
					vmopv1.VirtualMachinePublishRequestConditionTargetValid)).To(BeFalse())
			})

			It("returns error if content library is not writable", func() {
				cl.Spec.Writable = false
				Expect(ctx.Client.Create(ctx, cl)).To(Succeed())

				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// Update TargetValid condition.
				Expect(conditions.IsTrue(vmpub,
					vmopv1.VirtualMachinePublishRequestConditionTargetValid)).To(BeFalse())
			})

			It("returns error if content library is not ready", func() {
				cl.Status.Conditions = []imgregv1a1.Condition{
					{
						Type:   imgregv1a1.ReadyCondition,
						Status: corev1.ConditionFalse,
					},
				}
				Expect(ctx.Client.Create(ctx, cl)).To(Succeed())

				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// Update TargetValid condition.
				Expect(conditions.IsTrue(vmpub,
					vmopv1.VirtualMachinePublishRequestConditionTargetValid)).To(BeFalse())
			})

			When("item with same name already exists in the content library", func() {
				JustBeforeEach(func() {
					Expect(ctx.Client.Create(ctx, cl)).To(Succeed())
					fakeVMProvider.GetItemFromLibraryByNameFn = func(ctx context.Context,
						contentLibrary, itemName string) (*library.Item, error) {
						return &library.Item{ID: "dummy-id"}, nil
					}
				})

				It("doesn't return error to skip requeue", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					// Update TargetValid condition.
					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionTargetValid)).To(BeFalse())
				})
			})
		})

		When("Source and target are both valid", func() {
			When("Publish VM succeeds", func() {
				It("requeue and sets VM Publish request status to Success", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).ToNot(HaveOccurred())

					// Eventually vmpub result should be updated to success.
					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					Eventually(func() vimtypes.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(vimtypes.TaskInfoStateSuccess))

					By("Should set sourceRef/targetRef")
					Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
					Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-item"))
					Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
				})

				When(".spec.source.name is empty", func() {
					BeforeEach(func() {
						vm.Name = vmpub.Name
						vmpub.Spec.Source.Name = ""
					})

					It("requeue and sets VM Publish request status to Success", func() {
						_, err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).ToNot(HaveOccurred())

						// Eventually vmpub result should be updated to success.
						Eventually(func() bool {
							return fakeVMProvider.IsPublishVMCalled()
						}).Should(BeTrue())

						By("Should set sourceRef/targetRef")
						Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
						Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-item"))
						Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
					})
				})

				When(".spec.target.item.name is empty", func() {
					BeforeEach(func() {
						vmpub.Spec.Target.Item.Name = ""
					})

					It("requeue and sets VM Publish request status to Success", func() {
						_, err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).ToNot(HaveOccurred())

						// Eventually vmpub result should be updated to success.
						Eventually(func() bool {
							return fakeVMProvider.IsPublishVMCalled()
						}).Should(BeTrue())

						Eventually(func() vimtypes.TaskInfoState {
							return fakeVMProvider.GetVMPublishRequestResult(vmpub)
						}).Should(Equal(vimtypes.TaskInfoStateSuccess))

						By("Should set sourceRef/targetRef")
						Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
						Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-vm-image"))
						Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
					})
				})
			})

			When("Publish VM fails", func() {
				JustBeforeEach(func() {
					fakeVMProvider.PublishVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine,
						vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
						fakeVMProvider.AddToVMPublishMap(actID, vimtypes.TaskInfoStateError)
						return "", fmt.Errorf("dummy error")
					}
				})

				It("returns success but sets VM Publish status to error", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					// Eventually vmpub result should be updated to error.
					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					Eventually(func() vimtypes.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(vimtypes.TaskInfoStateError))

					By("Should set sourceRef/targetRef")
					Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
					Expect(vmpub.Status.TargetRef.Item.Name).To(Equal(vmpub.Spec.Target.Item.Name))
					Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
				})
			})
		})

		Context("A previous publish request has been sent", func() {
			BeforeEach(func() {
				vmpub.Status.Attempts = 1
				lastAttemptTime := time.Now().Add(-time.Minute)
				vmpub.Status.LastAttemptTime = metav1.NewTime(lastAttemptTime)
			})

			When("Previous task doesn't exist", func() {
				JustBeforeEach(func() {
					fakeVMProvider.Lock()
					fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
						return nil, nil
					}
					fakeVMProvider.Unlock()
				})

				It("Should send a second publish VM request", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeFalse())
					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionImageAvailable)).To(BeFalse())

					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					// vmpub result should eventually be success.
					Eventually(func() vimtypes.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(vimtypes.TaskInfoStateSuccess))
				})
			})

			When("Previous request succeeded", func() {
				var (
					itemID = uuid.New().String()
					task   *vimtypes.TaskInfo
				)

				When("task succeeded but failed to parse item ID", func() {
					JustBeforeEach(func() {
						task = &vimtypes.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         vimtypes.TaskInfoStateSuccess,
							QueueTime:     time.Now().Add(time.Minute),
						}
						fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
							return []vimtypes.TaskInfo{*task}, nil
						}
					})

					It("result object invalid, Upload condition is false, not send a second publish VM request and return success", func() {
						_, err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).NotTo(HaveOccurred())

						Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())

						uploadCondition := conditions.Get(vmpub, vmopv1.VirtualMachinePublishRequestConditionUploaded)
						Expect(uploadCondition).ToNot(BeNil())
						Expect(uploadCondition.Status).To(Equal(metav1.ConditionFalse))
						Expect(uploadCondition.Reason).To(Equal(vmopv1.UploadItemIDInvalidReason))
					})

					It("result type invalid, Upload condition is false, not send a second publish VM request and return success", func() {
						task.Result = vimtypes.ManagedObjectReference{Type: "ContentLibrary",
							Value: fmt.Sprintf("clib-%s", itemID)}
						_, err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).NotTo(HaveOccurred())

						Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())

						uploadCondition := conditions.Get(vmpub, vmopv1.VirtualMachinePublishRequestConditionUploaded)
						Expect(uploadCondition.Status).To(Equal(metav1.ConditionFalse))
						Expect(uploadCondition.Reason).To(Equal(vmopv1.UploadItemIDInvalidReason))
					})

					It("result value invalid, Upload condition is false, not send a second publish VM request and return success", func() {
						task.Result = vimtypes.ManagedObjectReference{Type: "ContentLibraryItem", Value: itemID}
						_, err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).NotTo(HaveOccurred())

						Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())

						uploadCondition := conditions.Get(vmpub, vmopv1.VirtualMachinePublishRequestConditionUploaded)
						Expect(uploadCondition).ToNot(BeNil())
						Expect(uploadCondition.Status).To(Equal(metav1.ConditionFalse))
						Expect(uploadCondition.Reason).To(Equal(vmopv1.UploadItemIDInvalidReason))
					})
				})

				When("Uploaded item id is valid", func() {
					JustBeforeEach(func() {
						fakeVMProvider.Lock()
						fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
							task := vimtypes.TaskInfo{
								DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
								State:         vimtypes.TaskInfoStateSuccess,
								QueueTime:     time.Now().Add(time.Minute),
								Result: vimtypes.ManagedObjectReference{Type: "ContentLibraryItem",
									Value: fmt.Sprintf("clibitem-%s", itemID)},
							}
							return []vimtypes.TaskInfo{task}, nil
						}
						fakeVMProvider.Unlock()
					})

					When("task succeeded but failed to parse item ID", func() {
						JustBeforeEach(func() {
							fakeVMProvider.Lock()
							fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
								task := vimtypes.TaskInfo{
									DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
									State:         vimtypes.TaskInfoStateSuccess,
									QueueTime:     time.Now().Add(time.Minute),
									Result: vimtypes.ManagedObjectReference{Type: "ContentLibrary",
										Value: fmt.Sprintf("item-%s", itemID)},
								}
								return []vimtypes.TaskInfo{task}, nil
							}
							fakeVMProvider.Unlock()
						})
					})

					When("VirtualMachineImage is available", func() {
						JustBeforeEach(func() {
							vmi := builder.DummyVirtualMachineImage("dummy-image")
							vmi.Namespace = vmpub.Namespace
							Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
							vmi.Status.ProviderItemID = itemID
							Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())
						})

						It("Complete condition is true, not send a second publish VM request and return success", func() {
							_, err := reconciler.ReconcileNormal(vmpubCtx)
							Expect(err).NotTo(HaveOccurred())

							Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())

							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeTrue())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionImageAvailable)).To(BeTrue())
							Expect(vmpub.Status.ImageName).To(Equal("dummy-image"))
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeTrue())
							Expect(vmpub.Status.Ready).To(BeTrue())
						})

						It("Update item description failed once", func() {
							fakeVMProvider.Lock()
							fakeVMProvider.UpdateContentLibraryItemFn = func(ctx context.Context,
								itemID, newName string, newDescription *string) error {
								return fmt.Errorf("dummy error")
							}
							fakeVMProvider.Unlock()

							_, err := reconciler.ReconcileNormal(vmpubCtx)
							Expect(err).To(HaveOccurred())

							By("ImageAvailable is true and Complete is false")
							Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeTrue())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionImageAvailable)).To(BeTrue())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeFalse())
							Expect(vmpub.Status.Ready).To(BeFalse())

							// requeue reconcile and update item description succeeded.
							fakeVMProvider.Lock()
							fakeVMProvider.UpdateContentLibraryItemFn = nil
							fakeVMProvider.Unlock()

							_, err = reconciler.ReconcileNormal(vmpubCtx)
							Expect(err).NotTo(HaveOccurred())

							By("Complete is true")
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeTrue())
							Expect(vmpub.Status.Ready).To(BeTrue())
						})

						When("TTLSecondsAfterFinished is set", func() {
							JustBeforeEach(func() {
								ttl := int64(0)
								vmpub.Spec.TTLSecondsAfterFinished = &ttl
							})

							It("Should delete VirtualMachinePublishRequest resource", func() {
								_, err := reconciler.ReconcileNormal(vmpubCtx)
								Expect(err).NotTo(HaveOccurred())

								Eventually(func() bool {
									newVMPub := &vmopv1.VirtualMachinePublishRequest{}
									err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmpub), newVMPub)
									return apierrors.IsNotFound(err) || !newVMPub.DeletionTimestamp.IsZero()
								}).Should(BeTrue())
							})
						})
					})

					When("VirtualMachineImage is unavailable", func() {
						It("ImageAvailable condition is false, not send a second publish VM request and return success", func() {
							_, err := reconciler.ReconcileNormal(vmpubCtx)
							Expect(err).NotTo(HaveOccurred())

							Expect(fakeVMProvider.IsPublishVMCalled()).To(BeFalse())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeTrue())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionImageAvailable)).To(BeFalse())
							Expect(conditions.IsTrue(vmpub,
								vmopv1.VirtualMachinePublishRequestConditionComplete)).To(BeFalse())
							Expect(vmpub.Status.Ready).To(BeFalse())
						})
					})
				})
			})

			When("Previous request failed", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
						currentTime := time.Now()
						task := vimtypes.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         vimtypes.TaskInfoStateError,
							StartTime:     &currentTime,
						}
						return []vimtypes.TaskInfo{task}, nil
					}
				})

				It("Should send a second publish VM request", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeFalse())

					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					// vmpub result should eventually be success.
					Eventually(func() vimtypes.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(vimtypes.TaskInfoStateSuccess))
				})
			})

			When("Previous request is queued", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
						task := vimtypes.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         vimtypes.TaskInfoStateQueued,
							QueueTime:     time.Now().Add(time.Minute),
						}
						return []vimtypes.TaskInfo{task}, nil
					}
				})

				It("Should return early and requeue", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeFalse())

					Consistently(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeFalse())
				})
			})

			When("Previous request is in progress", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
						task := vimtypes.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         vimtypes.TaskInfoStateRunning,
							QueueTime:     time.Now(),
						}
						return []vimtypes.TaskInfo{task}, nil
					}
				})

				It("Should return early and requeue", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeFalse())

					Consistently(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeFalse())
				})
			})

			When("Prior task succeeded but lost track of this task", func() {
				JustBeforeEach(func() {
					fakeVMProvider.Lock()
					fakeVMProvider.GetTasksByActIDFn = func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
						return nil, nil
					}
					vmpub.UID = "123"
					description := fmt.Sprintf("virtualmachinepublishrequest.vmoperator.vmware.com: %s\n",
						vmpub.UID)
					fakeVMProvider.GetItemFromLibraryByNameFn = func(ctx context.Context,
						contentLibrary, itemName string) (*library.Item, error) {
						return &library.Item{ID: "dummy-id", Description: &description}, nil
					}
					fakeVMProvider.Unlock()
				})

				It("target valid and mark Upload to true", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					// Update TargetValid condition.
					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionTargetValid)).To(BeTrue())
					Expect(conditions.IsTrue(vmpub,
						vmopv1.VirtualMachinePublishRequestConditionUploaded)).To(BeTrue())
				})
			})
		})
	})
}
