// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/lifecycle"
	proberfake "github.com/vmware-tanzu/vm-operator/pkg/prober/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizer           = "vmoperator.vmware.com/virtualmachine"
	stateFinalizer      = "vmoperator.vmware.com/lifecyclestate"
	otherStateFinalizer = "example.com/other"
)

// countingReader counts the Get calls made through it.
type countingReader struct {
	client.Reader
	gets int
}

func (c *countingReader) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption) error {

	c.gets++
	return c.Reader.Get(ctx, key, obj, opts...)
}

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
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

		// ReconcileDelete looks up PVCs by VM OwnerRef via a field index that
		// is normally registered by AddToManager. The fake client used here
		// does not go through AddToManager, so register the same index
		// directly.
		ctx.Client = fake.NewClientBuilder().
			WithScheme(ctx.Client.Scheme()).
			WithObjects(initObjects...).
			WithStatusSubresource(builder.KnownObjectTypes()...).
			WithIndex(
				&corev1.PersistentVolumeClaim{},
				kubeutil.PVCOwnerRefVMUIDIndexKey,
				kubeutil.PVCOwnerRefVMUIDIndexerFunc).
			Build()

		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 16
		})

		fakeProbeManagerIf := proberfake.NewFakeProberManager()

		plainContext := cource.WithContext(
			pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.AsyncCreateEnabled = false
					config.AsyncSignalEnabled = false
					config.Features.PodVMOnStretchedSupervisor = true
				},
			),
		)
		plainContext = ctxop.WithContext(plainContext)
		plainContext = ovfcache.WithContext(plainContext)

		reconciler = virtualmachine.NewReconciler(
			plainContext,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.Client,
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

	Context("Reconcile", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vm)
		})

		When("reconcile fails with ErrReconcileInProgress error", func() {
			It("should succeed but with requeue delay", func() {
				var createAttempts int32

				By("CreateOrUpdate func returns duplicate creates", func() {
					providerfake.SetCreateOrUpdateFunction(
						ctx,
						fakeVMProvider,
						func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
							atomic.AddInt32(&createAttempts, 1)
							return providers.ErrReconcileInProgress
						},
					)
				})

				req := ctrl.Request{}
				req.Namespace = vm.Namespace
				req.Name = vm.Name

				res, err := reconciler.Reconcile(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
				Expect(res.RequeueAfter).To(Equal(pkgcfg.FromContext(ctx).CreateVMRequeueDelay))
			})
		})
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
					config.AsyncCreateEnabled = false
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

			It("Should emit a CreatePending event if ReconcileNormal returns a NoRequeueErr during create", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return pkgerr.NoRequeueError{}
					},
				)
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
				expectEvents(ctx, "CreatePending")
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
					config.AsyncCreateEnabled = true
					config.AsyncSignalEnabled = true
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

			It("Should emit a CreatePending event if ReconcileNormal returns a NoRequeueErr during create", func() {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkCreate(ctx)
						return pkgerr.NoRequeueError{}
					},
				)
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
				expectEvents(ctx, "CreatePending")
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

		DescribeTable("Should emit UpdateSuccess event if ReconcileNormal causes a successful VM update that returns a NoRequeueErr",
			func(noRequeueErr error) {
				providerfake.SetCreateOrUpdateFunction(
					vmCtx,
					fakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						ctxop.MarkUpdate(ctx)
						return noRequeueErr
					},
				)

				err := reconciler.ReconcileNormal(vmCtx)
				Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
				expectEvents(ctx, "UpdateSuccess")
			},
			Entry("pkgerr.NoRequeueErr", pkgerr.NoRequeueError{}),
			Entry("vsphere.ErrBackup", vsphere.ErrBackup),
			Entry("vsphere.ErrBootstrapCustomize", vsphere.ErrBootstrapCustomize),
			Entry("vsphere.ErrBootstrapReconfigure", vsphere.ErrBootstrapReconfigure),
			Entry("vsphere.ErrReconfigure", vsphere.ErrReconfigure),
			Entry("vsphere.ErrRestart", vsphere.ErrRestart),
			Entry("vsphere.ErrSetPowerState", vsphere.ErrSetPowerState),
			Entry("vsphere.ErrUpgradeHardwareVersion", vsphere.ErrUpgradeHardwareVersion),
		)

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

		It("reports that the image cache is not ready while waiting for create prerequisites", func() {
			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.Features.FastDeploy = true
			})
			providerfake.SetCreateOrUpdateFunction(vmCtx, fakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					ctxop.MarkCreate(ctx)
					return pkgerr.VMICacheNotReadyError{Name: "pending-image-cache"}
				})

			const message = "disk image cache is not ready"
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(MatchError(message))
			Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
			Expect(vm.Labels).To(HaveKeyWithValue(pkgconst.VMICacheLabelKey, "pending-image-cache"))
			Eventually(ctx.Events).Should(Receive(Equal("Warning CreatePending " + message)))

			// Subsequent reconciles wait for the cache before calling the provider.
			fakeVMProvider.CreateOrUpdateVirtualMachineFn = func(context.Context, *vmopv1.VirtualMachine) error {
				Fail("provider called before image cache became ready")
				return nil
			}
			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(MatchError(message))
			Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
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

		It("will not delete the created VM when VM is paused by devops", func() {
			vm.Annotations[vmopv1.PauseAnnotation] = "enabled"
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
		})

		It("will not delete the created VM when VM has delete.check annotation", func() {
			vm.Annotations[vmopv1.CheckAnnotationDelete+"/app1"] = "reason"
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

		When("the Delete stage is blocked on hooks", func() {
			var blocked bool

			BeforeEach(func() {
				blocked = true
			})

			JustBeforeEach(func() {
				fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					if blocked {
						return lifecycle.ErrHooksBlocked
					}
					return nil
				}
			})

			It("emits no event and keeps the finalizers", func() {
				err := reconciler.ReconcileDelete(vmCtx)
				Expect(lifecycle.IsHooksBlocked(err)).To(BeTrue())
				Expect(vm.GetFinalizers()).To(ContainElement(finalizer))

				Consistently(ctx.Events).ShouldNot(Receive())
			})

			It("emits exactly one DeleteSuccess across the sequence", func() {
				for range 3 {
					Expect(lifecycle.IsHooksBlocked(
						reconciler.ReconcileDelete(vmCtx))).To(BeTrue())
				}

				blocked = false
				Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

				expectEvents(ctx, "DeleteSuccess")
			})
		})

		It("emits DeleteFailure for an unsupported connection state", func() {
			fakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
				return fmt.Errorf("failed to delete vm: %w", pkgerr.NoRequeueError{
					Message: "unsupported connection state: disconnected",
				})
			}

			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(lifecycle.IsHooksBlocked(err)).To(BeFalse())

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

		When("the VM owns PVCs", func() {
			var (
				attachedPVC     *corev1.PersistentVolumeClaim
				detachedPVC     *corev1.PersistentVolumeClaim
				detachedKeptPVC *corev1.PersistentVolumeClaim
			)

			ownerRefFor := func(vm *vmopv1.VirtualMachine) metav1.OwnerReference {
				return metav1.OwnerReference{
					APIVersion: vmopv1.GroupVersion.String(),
					Kind:       "VirtualMachine",
					Name:       vm.Name,
					UID:        vm.UID,
				}
			}

			BeforeEach(func() {
				vm.UID = "dummy-vm-uid"
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					builder.DummyPVCVolume("disk-1", "attached-pvc"),
				}

				attachedPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "attached-pvc",
						Namespace: vm.Namespace,
					},
				}
				detachedPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "detached-pvc",
						Namespace: vm.Namespace,
					},
				}
				detachedKeptPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "detached-kept-pvc",
						Namespace: vm.Namespace,
						Annotations: map[string]string{
							pkgconst.KeepOwnerRefAnnotationKey: "",
						},
					},
				}
			})

			JustBeforeEach(func() {
				attachedPVC.OwnerReferences = []metav1.OwnerReference{ownerRefFor(vm)}
				detachedPVC.OwnerReferences = []metav1.OwnerReference{ownerRefFor(vm)}
				detachedKeptPVC.OwnerReferences = []metav1.OwnerReference{ownerRefFor(vm)}
				Expect(ctx.Client.Create(ctx, attachedPVC)).To(Succeed())
				Expect(ctx.Client.Create(ctx, detachedPVC)).To(Succeed())
				Expect(ctx.Client.Create(ctx, detachedKeptPVC)).To(Succeed())
			})

			It("removes the VM's OwnerRef only from the detached PVC without the keep-owner-ref annotation", func() {
				Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

				gotAttached := &corev1.PersistentVolumeClaim{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(attachedPVC), gotAttached)).To(Succeed())
				Expect(gotAttached.OwnerReferences).To(ConsistOf(ownerRefFor(vm)))

				gotDetached := &corev1.PersistentVolumeClaim{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(detachedPVC), gotDetached)).To(Succeed())
				Expect(gotDetached.OwnerReferences).To(BeEmpty())

				gotDetachedKept := &corev1.PersistentVolumeClaim{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(detachedKeptPVC), gotDetachedKept)).To(Succeed())
				Expect(gotDetachedKept.OwnerReferences).To(ConsistOf(ownerRefFor(vm)))
			})

			When("the VM is being unregistered rather than deleted", func() {
				JustBeforeEach(func() {
					vm.Annotations[pkgconst.SkipDeletePlatformResourceKey] = ""
				})

				It("still removes the OwnerRef from the detached PVC", func() {
					Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

					gotDetached := &corev1.PersistentVolumeClaim{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(detachedPVC), gotDetached)).To(Succeed())
					Expect(gotDetached.OwnerReferences).To(BeEmpty())
				})
			})
		})

		When("VM has skip-platform-delete annotation", func() {
			JustBeforeEach(func() {
				vm.Annotations = map[string]string{
					pkgconst.SkipDeletePlatformResourceKey: "",
				}
			})
			It("will delete the created VM and will not emit corresponding event", func() {
				Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
				doNotExpectEvent(ctx, "DeleteSuccess")
			})
		})
	})

	Context("LifecycleState", func() {
		var (
			state     *lifecyclev1.LifecycleState
			apiReader *countingReader
			capOn     bool
		)

		getState := func() *lifecyclev1.LifecycleState {
			got := &lifecyclev1.LifecycleState{}
			ExpectWithOffset(1, ctx.Client.Get(
				ctx, client.ObjectKeyFromObject(state), got)).To(Succeed())
			return got
		}

		BeforeEach(func() {
			capOn = true
			vm.UID = "dummy-vm-uid"

			isController := true
			state = &lifecyclev1.LifecycleState{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, vm.UID),
					Labels: map[string]string{
						lifecycle.TargetGroupLabel: lifecycle.TargetGroup,
						lifecycle.TargetKindLabel:  lifecycle.TargetKind,
					},
					Finalizers: []string{otherStateFinalizer},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       lifecycle.TargetKind,
						Name:       vm.Name,
						UID:        vm.UID,
						Controller: &isController,
					}},
				},
				Spec: lifecyclev1.LifecycleStateSpec{
					Stages: []lifecyclev1.StageSpec{{Name: lifecycle.StageCreate}},
				},
			}
			initObjects = append(initObjects, vm, state)
		})

		JustBeforeEach(func() {
			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.Features.LifecycleHooks = capOn
			})
			apiReader = &countingReader{Reader: ctx.Client}
			reconciler.APIReader = apiReader
		})

		When("the capability is on", func() {
			It("adopts the LifecycleState on ReconcileNormal", func() {
				Expect(reconciler.ReconcileNormal(vmCtx)).To(Succeed())
				Expect(getState().Finalizers).To(ConsistOf(otherStateFinalizer, stateFinalizer))
			})

			It("removes only VM Operator's finalizer on ReconcileDelete", func() {
				Expect(reconciler.ReconcileNormal(vmCtx)).To(Succeed())
				Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

				Expect(getState().Finalizers).To(ConsistOf(otherStateFinalizer))
				Expect(vm.Finalizers).ToNot(ContainElement(finalizer))
			})

			When("the LifecycleState is not bound to the VM", func() {
				BeforeEach(func() {
					state.OwnerReferences = nil
				})

				It("keeps the VM finalizers on ReconcileDelete", func() {
					Expect(reconciler.ReconcileDelete(vmCtx)).To(MatchError(
						ContainSubstring("is not bound to VirtualMachine")))
					Expect(vm.Finalizers).To(ContainElement(finalizer))
				})
			})

			When("the ResourceDelete stage is hooked", func() {
				const stage = lifecycle.StageResourceDelete

				const otherVMFinalizer = "example.com/vm-other"

				var stateWrites atomic.Int32

				stageSpec := func() lifecyclev1.StageSpec {
					for _, s := range getState().Spec.Stages {
						if s.Name == stage {
							return s
						}
					}
					Fail("stage " + stage + " not found")
					return lifecyclev1.StageSpec{}
				}

				// setHooksReady simulates the Lifecycle Operator. A nil status
				// clears the stage's entry, as it does once a stage resumed.
				setHooksReady := func(status *metav1.ConditionStatus) {
					s := getState()
					s.Status.Stages = nil
					if status != nil {
						s.Status.Stages = []lifecyclev1.StageStatus{{
							Name: stage,
							Conditions: []metav1.Condition{{
								Type:               lifecycle.HooksReadyConditionType,
								Status:             *status,
								Reason:             "AllHooksDone",
								LastTransitionTime: metav1.Now(),
							}},
						}}
					}
					ExpectWithOffset(1, ctx.Client.Status().Update(ctx, s)).To(Succeed())
				}

				BeforeEach(func() {
					state.Spec.Stages = []lifecyclev1.StageSpec{
						{Name: lifecycle.StageDelete},
						{Name: stage},
					}
					vm.Finalizers = append(vm.Finalizers, otherVMFinalizer)
					stateWrites.Store(0)
				})

				JustBeforeEach(func() {
					// Count the writes to the LifecycleState.
					reconciler.Client = interceptor.NewClient(
						ctx.Client.(client.WithWatch),
						interceptor.Funcs{
							Patch: func(
								ctx context.Context,
								c client.WithWatch,
								obj client.Object,
								patch client.Patch,
								opts ...client.PatchOption) error {

								if _, ok := obj.(*lifecyclev1.LifecycleState); ok {
									stateWrites.Add(1)
								}
								return c.Patch(ctx, obj, patch, opts...)
							},
						})
				})

				It("keeps the VM finalizers and emits no event while blocked", func() {
					Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

					Expect(vm.Finalizers).To(ContainElements(finalizer, otherVMFinalizer))
					Expect(stageSpec().WorkflowPaused).To(BeTrue())
					Expect(getState().Finalizers).To(ContainElement(stateFinalizer))

					c := conditions.Get(
						vm, vmopv1.VirtualMachineConditionLifecycleHookReady)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(
						vmopv1.VirtualMachineLifecycleHookReadyResourceDeleteStageReason))

					Consistently(ctx.Events).ShouldNot(Receive())
				})

				It("evaluates the stage only after Delete resolves", func() {
					deleteBlocked := true
					fakeVMProvider.DeleteVirtualMachineFn = func(
						context.Context, *vmopv1.VirtualMachine) error {

						if deleteBlocked {
							return lifecycle.ErrHooksBlocked
						}
						return nil
					}

					By("Delete being held", func() {
						Expect(lifecycle.IsHooksBlocked(
							reconciler.ReconcileDelete(vmCtx))).To(BeTrue())
						Expect(stageSpec()).To(Equal(lifecyclev1.StageSpec{Name: stage}))
						Expect(stateWrites.Load()).To(BeZero())
						Expect(vm.Finalizers).To(ContainElement(finalizer))
					})

					By("Delete resolving", func() {
						deleteBlocked = false
						Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
						Expect(stageSpec().WorkflowPaused).To(BeTrue())
						Expect(vm.Finalizers).To(ContainElement(finalizer))
					})
				})

				It("resumes and releases in one write, keeps other finalizers and emits one DeleteSuccess", func() {
					By("holding across several passes", func() {
						for range 3 {
							Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
							Expect(vm.Finalizers).To(ContainElement(finalizer))
						}
					})

					By("the hooks resolving", func() {
						ready := metav1.ConditionTrue
						setHooksReady(&ready)
						before := stateWrites.Load()

						Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())

						// The resume write also drops VM Operator's finalizer,
						// so the release that follows writes nothing more.
						Expect(stateWrites.Load() - before).To(Equal(int32(1)))
						Expect(stageSpec().WorkflowPaused).To(BeFalse())
						Expect(stageSpec().WorkflowResumed).To(BeTrue())
						Expect(getState().Finalizers).To(ConsistOf(otherStateFinalizer))
						Expect(vm.Finalizers).To(ConsistOf(otherVMFinalizer))
					})

					expectEvents(ctx, "DeleteSuccess")
				})

				It("pauses again when the VM finalizer patch fails after resume", func() {
					Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
					ready := metav1.ConditionTrue
					setHooksReady(&ready)
					Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
					Expect(vm.Finalizers).ToNot(ContainElement(finalizer))

					By("the deferred VM patch having failed", func() {
						vm.Finalizers = append(vm.Finalizers, finalizer)
						// The Lifecycle Operator clears the resumed entry.
						setHooksReady(nil)
					})

					Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
					Expect(stageSpec().WorkflowPaused).To(BeTrue())
					Expect(stageSpec().WorkflowResumed).To(BeFalse())
					Expect(vm.Finalizers).To(ContainElement(finalizer))
				})
			})
		})

		When("the capability is off", func() {
			BeforeEach(func() {
				capOn = false
				state.Finalizers = append(state.Finalizers, stateFinalizer)
			})

			It("does not touch the LifecycleState on ReconcileNormal", func() {
				Expect(reconciler.ReconcileNormal(vmCtx)).To(Succeed())
				Expect(apiReader.gets).To(BeZero())
				Expect(getState().Finalizers).To(ConsistOf(otherStateFinalizer, stateFinalizer))
			})

			It("releases through the APIReader on ReconcileDelete", func() {
				Expect(reconciler.ReconcileDelete(vmCtx)).To(Succeed())
				Expect(apiReader.gets).To(Equal(1))
				Expect(getState().Finalizers).To(ConsistOf(otherStateFinalizer))
				Expect(vm.Finalizers).ToNot(ContainElement(finalizer))
			})
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

func doNotExpectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	ConsistentlyWithOffset(1, func(g Gomega) {
		var event string
		ConsistentlyWithOffset(1, ctx.Events).ShouldNot(Receive(&event), "receive expected event: "+eventStr)
	}).Should(Succeed())
}
