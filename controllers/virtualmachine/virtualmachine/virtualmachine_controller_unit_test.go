// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	proberfake "github.com/vmware-tanzu/vm-operator/pkg/prober/fake"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const finalizer = "vmoperator.vmware.com/virtualmachine"

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
	const (
		providerError = "provider error"
	)

	var (
		initObjects      []client.Object
		ctx              *builder.UnitTestContextForController
		reconciler       *virtualmachine.Reconciler
		fakeProbeManager *proberfake.ProberManager
		fakeVMProvider   *providerfake.VMProvider

		vm    *vmopv1.VirtualMachine
		vmCtx *pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vm",
				Namespace:   "dummy-ns",
				Annotations: map[string]string{},
				Finalizers:  []string{finalizer},
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName:    "dummy-class",
				ImageName:    "dummy-image",
				StorageClass: "my-storage-class",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 16
			config.Features.WorkloadDomainIsolation = false
		})

		fakeProbeManagerIf := proberfake.NewFakeProberManager()

		plainContext := cource.WithContext(
			pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				},
			),
		)
		plainContext = ctxop.WithContext(plainContext)

		reconciler = virtualmachine.NewReconciler(
			plainContext,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
			fakeProbeManagerIf)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeProbeManager = fakeProbeManagerIf.(*proberfake.ProberManager)

		vmCtx = &pkgctx.VirtualMachineContext{
			Context: plainContext,
			Logger:  ctx.Logger.WithName(vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmCtx = nil
		reconciler = nil
		fakeVMProvider = nil
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vm)
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vm.Finalizers = nil
			})

			It("will not set the finalizer when the VM contains the pause annotation", func() {
				vm.Annotations[vmopv1.PauseAnnotation] = "true"
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vm.GetFinalizers()).ToNot(ContainElement(finalizer))
			})

			It("will set finalizer", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			})
		})

		It("will have finalizer set upon successful reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
		})

		It("can be called multiple times", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))

			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
		})

		Context("ProberManager", func() {

			It("Should call add to Prober Manager if ReconcileNormal fails", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						return errors.New(providerError)
					},
				)

				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(fakeProbeManager.IsAddToProberManagerCalled).Should(BeFalse())
			})

			It("Should call add to Prober Manager if ReconcileNormal succeeds", func() {
				fakeProbeManager.AddToProberManagerFn = func(vm *vmopv1.VirtualMachine) {
					fakeProbeManager.IsAddToProberManagerCalled = true
				}

				Expect(reconciler.ReconcileNormal(vmCtx)).Should(Succeed())
				Expect(fakeProbeManager.IsAddToProberManagerCalled).Should(BeTrue())
			})
		})

		When("blocking create", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.AsyncCreateDisabled = true
				})
			})
			It("Should emit a CreateSuccess event if ReconcileNormal causes a successful VM creation", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return nil
					},
				)
				Expect(reconciler.ReconcileNormal(vmCtx)).Should(Succeed())
				expectEvents(ctx, "CreateSuccess")
			})

			It("Should emit CreateFailure event if ReconcileNormal causes a failed VM create", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return errors.New("fake")
					},
				)

				Expect(reconciler.ReconcileNormal(vmCtx)).ShouldNot(Succeed())
				expectEvents(ctx, "CreateFailure")
			})
		})

		When("non-blocking create", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = true
					config.AsyncSignalDisabled = false
					config.AsyncCreateDisabled = false
				})
			})
			It("Should emit a CreateSuccess event if ReconcileNormal causes a successful VM creation", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return nil
					},
				)
				Expect(reconciler.ReconcileNormal(vmCtx)).Should(Succeed())
				expectEvents(ctx, "CreateSuccess")
			})

			It("Should emit CreateFailure event if ReconcileNormal causes a failed VM create", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return errors.New("fake")
					},
				)

				Expect(reconciler.ReconcileNormal(vmCtx)).ShouldNot(Succeed())
				expectEvents(ctx, "CreateFailure")
			})

			It("Should emit CreateFailure events if ReconcileNormal causes a failed VM create that reports multiple errors", func() {
				fakeVMProvider.CreateOrUpdateVirtualMachineAsyncFn = func(
					ctx context.Context,
					vm *vmopv1.VirtualMachine) (<-chan error, error) {

					ctxop.MarkCreate(ctx)
					chanErr := make(chan error, 2)
					chanErr <- errors.New("error1")
					chanErr <- errors.New("error2")
					return chanErr, nil
				}

				Expect(reconciler.ReconcileNormal(vmCtx)).To(Succeed())
				expectEvents(ctx, "CreateFailure", "CreateFailure")
			})
		})

		It("Should emit UpdateSuccess event if ReconcileNormal causes a successful VM update", func() {
			providerfake.SetCreateOrUpdateFunction(
				vmCtx,
				fakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					ctxop.MarkUpdate(ctx)
					return nil
				},
			)

			Expect(reconciler.ReconcileNormal(vmCtx)).Should(Succeed())
			expectEvents(ctx, "UpdateSuccess")
		})

		It("Should emit UpdateFailure event if ReconcileNormal causes a failed VM update", func() {
			providerfake.SetCreateOrUpdateFunction(
				vmCtx,
				fakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					ctxop.MarkUpdate(ctx)
					return errors.New("fake")
				},
			)

			Expect(reconciler.ReconcileNormal(vmCtx)).ShouldNot(Succeed())
			expectEvents(ctx, "UpdateFailure")
		})

		It("Should emit ReconcileNormalFailure if ReconcileNormal fails for neither create or update op", func() {
			providerfake.SetCreateOrUpdateFunction(
				vmCtx,
				fakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					return errors.New("fake")
				},
			)

			Expect(reconciler.ReconcileNormal(vmCtx)).ShouldNot(Succeed())
			expectEvents(ctx, "ReconcileNormalFailure")
		})
	})

	Context("ReconcileDelete", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vm)
		})

		JustBeforeEach(func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("will not delete the created VM when VM is paused by devops ", func() {
			vm.Annotations[vmopv1.PauseAnnotation] = "enabled"
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
		})

		It("will delete the created VM and emit corresponding event", func() {
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())

			expectEvents(ctx, "DeleteSuccess")
		})

		It("will emit corresponding event during delete failure", func() {
			// Simulate delete failure
			fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())

			expectEvents(ctx, "DeleteFailure")
		})

		It("Should not remove from Prober Manager if ReconcileDelete fails", func() {
			// Simulate delete failure
			fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(fakeProbeManager.IsRemoveFromProberManagerCalled).Should(BeFalse())
		})

		It("Should remove from Prober Manager if ReconcileDelete succeeds", func() {
			fakeProbeManager.RemoveFromProberManagerFn = func(vm *vmopv1.VirtualMachine) {
				fakeProbeManager.IsRemoveFromProberManagerCalled = true
			}

			Expect(reconciler.ReconcileDelete(vmCtx)).Should(Succeed())
			Expect(fakeProbeManager.IsRemoveFromProberManagerCalled).Should(BeTrue())
		})
	})
}

func expectEvents(ctx *builder.UnitTestContextForController, eventStrs ...string) {
	for _, s := range eventStrs {
		var event string
		EventuallyWithOffset(1, ctx.Events).Should(Receive(&event), "receive expected event: "+s)
		eventComponents := strings.Split(event, " ")
		ExpectWithOffset(1, eventComponents[1]).To(Equal(s))
	}
	ConsistentlyWithOffset(1, ctx.Events).ShouldNot(Receive())
}
