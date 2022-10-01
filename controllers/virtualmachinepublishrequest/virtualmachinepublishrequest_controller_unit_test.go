// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest_test

import (
	goctx "context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/vmpublish"
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
		vmPubCatalog := vmpublish.NewCatalog()
		reconciler = virtualmachinepublishrequest.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
			vmPubCatalog,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)

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

		When("Source and target are both valid", func() {
			When("Publish VM succeeds", func() {
				JustBeforeEach(func() {
					fakeVMProvider.PublishVirtualMachineFn = func(ctx goctx.Context, vm *vmopv1alpha1.VirtualMachine,
						vmPub *vmopv1alpha1.VirtualMachinePublishRequest,
						cl *imgregv1a1.ContentLibrary) (string, error) {
						return "dummy-item", nil
					}
				})

				It("requeue and sets VM Publish request status to Success", func() {
					err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).ToNot(HaveOccurred())

					// Eventually vmpub result should be updated to success.
					Eventually(func() string {
						exist, result := reconciler.VMPubCatalog.GetPubRequestState(string(vmpub.UID))
						if !exist {
							return ""
						}

						return result.State
					}).Should(Equal(vmpublish.StateSuccess))

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
						err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).ToNot(HaveOccurred())

						// Eventually vmpub result should be updated to success.
						Eventually(func() string {
							exist, result := reconciler.VMPubCatalog.GetPubRequestState(string(vmpub.UID))
							if !exist {
								return ""
							}

							return result.State
						}).Should(Equal(vmpublish.StateSuccess))

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
						err := reconciler.ReconcileNormal(vmpubCtx)
						Expect(err).ToNot(HaveOccurred())

						// Eventually vmpub result should be updated to success.
						Eventually(func() string {
							exist, result := reconciler.VMPubCatalog.GetPubRequestState(string(vmpub.UID))
							if !exist {
								return ""
							}

							return result.State
						}).Should(Equal(vmpublish.StateSuccess))

						Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
						Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-vmpub-image"))
						Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
					})
				})
			})

			When("Publish VM fails", func() {
				JustBeforeEach(func() {
					fakeVMProvider.PublishVirtualMachineFn = func(ctx goctx.Context, vm *vmopv1alpha1.VirtualMachine,
						vmPub *vmopv1alpha1.VirtualMachinePublishRequest,
						cl *imgregv1a1.ContentLibrary) (string, error) {
						return "", fmt.Errorf("dummy error")
					}
				})

				It("requeue and sets VM Publish status to error", func() {
					err := reconciler.ReconcileNormal(vmpubCtx)
					Expect(err).ToNot(HaveOccurred())

					// Eventually vmpub result should be updated to error.
					Eventually(func() string {
						exist, result := reconciler.VMPubCatalog.GetPubRequestState(string(vmpub.UID))
						if !exist {
							return ""
						}
						return result.State
					}).Should(Equal(vmpublish.StateError))

					Expect(vmpub.Status.SourceRef.Name).To(Equal(vm.Name))
					Expect(vmpub.Status.TargetRef.Item.Name).To(Equal("dummy-item"))
					Expect(vmpub.Status.TargetRef.Location).To(Equal(vmpub.Spec.Target.Location))
				})
			})
		})
	})
}
