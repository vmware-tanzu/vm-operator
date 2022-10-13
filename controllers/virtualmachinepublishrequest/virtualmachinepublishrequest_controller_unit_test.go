// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest_test

import (
	goctx "context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vim25/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking VirtualMachinePublishRequest Reconcile", unitTestsReconcile)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *virtualmachinepublishrequest.Reconciler
		fakeVMProvider *providerfake.VMProvider

		vm       *vmopv1alpha1.VirtualMachine
		vmpub    *vmopv1alpha1.VirtualMachinePublishRequest
		cl       *imgregv1a1.ContentLibrary
		vmpubCtx *vmopContext.VirtualMachinePublishRequestContext
	)

	BeforeEach(func() {
		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
		}

		vmpub = &vmopv1alpha1.VirtualMachinePublishRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vmpub",
				Namespace: vm.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachinePublishRequestSpec{
				Source: vmopv1alpha1.VirtualMachinePublishRequestSource{
					Name: vm.Name,
				},
				Target: vmopv1alpha1.VirtualMachinePublishRequestTarget{
					Item: vmopv1alpha1.VirtualMachinePublishRequestTargetItem{
						Name: "dummy-item",
					},
					Location: vmopv1alpha1.VirtualMachinePublishRequestTargetLocation{
						Name: "dummy-cl",
					},
				},
			},
		}

		cl = &imgregv1a1.ContentLibrary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-cl",
				Namespace: vm.Namespace,
			},
			Spec: imgregv1a1.ContentLibrarySpec{
				UUID: "dummy-id",
			},
			Status: imgregv1a1.ContentLibraryStatus{
				Name: "dummy-name",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachinepublishrequest.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.Reset()

		vmpubCtx = &vmopContext.VirtualMachinePublishRequestContext{
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

		When("VMPubRequest update failed", func() {
			JustBeforeEach(func() {
				// mock update failure
				obj := vmpub.DeepCopy()
				obj.Annotations = map[string]string{"dummy": "dummy"}
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			})

			It("Should not publish VM", func() {
				_, err := reconciler.ReconcileNormal(vmpubCtx)
				Expect(err).To(HaveOccurred())

				// vmpub result should be empty because it never starts.
				Consistently(func() types.TaskInfoState {
					return fakeVMProvider.GetVMPublishRequestResult(vmpub)
				}).Should(BeEmpty())
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

					Eventually(func() types.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(types.TaskInfoStateSuccess))

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

						Eventually(func() types.TaskInfoState {
							return fakeVMProvider.GetVMPublishRequestResult(vmpub)
						}).Should(Equal(types.TaskInfoStateSuccess))

						By("Should set sourceRef/targetRef")
						Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
						Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-vmpub-image"))
						Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
					})
				})
			})

			When("Publish VM fails", func() {
				JustBeforeEach(func() {
					fakeVMProvider.PublishVirtualMachineFn = func(ctx goctx.Context, vm *vmopv1alpha1.VirtualMachine,
						vmPub *vmopv1alpha1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
						fakeVMProvider.AddToVMPublishMap(actID, types.TaskInfoStateError)
						return "", fmt.Errorf("dummy error")
					}
				})

				It("requeue and sets VM Publish status to error", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).ToNot(HaveOccurred())

					// Eventually vmpub result should be updated to error.
					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					Eventually(func() types.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(types.TaskInfoStateError))

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
					fakeVMProvider.GetTasksByActIDFn = func(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						return nil, nil
					}
				})

				It("Should send a second publish VM request and return error", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).To(HaveOccurred())

					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					// vmpub result should eventually be success.
					Eventually(func() types.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(types.TaskInfoStateSuccess))
				})
			})

			When("Previous request succeeded", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateSuccess,
							QueueTime:     time.Now().Add(time.Minute),
							Result:        "dummy-item",
						}
						return []types.TaskInfo{task}, nil
					}
				})

				It("Should not send a second publish VM request and return success", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Consistently(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeFalse())
				})
			})

			When("Previous request failed", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						currentTime := time.Now()
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateError,
							StartTime:     &currentTime,
						}
						return []types.TaskInfo{task}, nil
					}
				})

				It("Should send a second publish VM request and return error", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).To(HaveOccurred())

					Eventually(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeTrue())

					// vmpub result should eventually be success.
					Eventually(func() types.TaskInfoState {
						return fakeVMProvider.GetVMPublishRequestResult(vmpub)
					}).Should(Equal(types.TaskInfoStateSuccess))
				})
			})

			When("Previous request is queued", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateQueued,
							QueueTime:     time.Now().Add(time.Minute),
						}
						return []types.TaskInfo{task}, nil
					}
				})

				It("Should return early and requeue", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Consistently(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeFalse())
				})
			})

			When("Previous request is in progress", func() {
				JustBeforeEach(func() {
					fakeVMProvider.GetTasksByActIDFn = func(ctx goctx.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
						task := types.TaskInfo{
							DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
							State:         types.TaskInfoStateRunning,
							QueueTime:     time.Now(),
						}
						return []types.TaskInfo{task}, nil
					}
				})

				It("Should return early and requeue", func() {
					_, err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).NotTo(HaveOccurred())

					Consistently(func() bool {
						return fakeVMProvider.IsPublishVMCalled()
					}).Should(BeFalse())
				})
			})
		})
	})
}
