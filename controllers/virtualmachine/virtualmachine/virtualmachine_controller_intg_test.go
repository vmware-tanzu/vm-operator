// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/lifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
	vmwatcher "github.com/vmware-tanzu/vm-operator/services/vm-watcher"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	var (
		ctx *builder.IntegrationTestContext

		vm    *vmopv1.VirtualMachine
		vmKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    "dummy-image",
				ClassName:    "dummy-class",
				StorageClass: "my-storage-class",
				PowerState:   vmopv1.VirtualMachinePowerStateOn,
			},
		}
		vmKey = types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachine := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachine {
		vm := &vmopv1.VirtualMachine{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return vm
	}

	waitForVirtualMachineFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func(g Gomega) {
			vm := getVirtualMachine(ctx, objKey)
			g.Expect(vm).ToNot(BeNil())
			g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
		}).Should(Succeed(), "waiting for VirtualMachine finalizer")
	}

	Context("Reconcile", func() {
		const dummyIPAddress = "1.2.3.4"
		dummyInstanceUUID := uuid.NewString()

		BeforeEach(func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					return nil
				},
			)
		})

		AfterEach(func() {
			By("Delete VirtualMachine", func() {
				if err := ctx.Client.Delete(ctx, vm); err == nil {
					vm := &vmopv1.VirtualMachine{}
					// If VM is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
						vm.Finalizers = nil
						_ = ctx.Client.Update(ctx, vm)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})
		})

		Context("LifecycleState", func() {
			It("adopts a LifecycleState and reconciles on its status changes only", func() {
				var reconciles int32
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						atomic.AddInt32(&reconciles, 1)
						// Without Created, the VM requeues itself and the
						// reconcile counts below are unreliable.
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
						return nil
					},
				)

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				waitForVirtualMachineFinalizer(ctx, vmKey)

				createdVM := getVirtualMachine(ctx, vmKey)
				Expect(createdVM).ToNot(BeNil())

				state := &lifecyclev1.LifecycleState{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vm.Namespace,
						Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, createdVM.UID),
						Labels: map[string]string{
							lifecycle.TargetGroupLabel: lifecycle.TargetGroup,
							lifecycle.TargetKindLabel:  lifecycle.TargetKind,
						},
					},
					Spec: lifecyclev1.LifecycleStateSpec{
						Stages: []lifecyclev1.StageSpec{{Name: lifecycle.StageCreate}},
					},
				}
				Expect(controllerutil.SetControllerReference(
					createdVM,
					state,
					ctx.Client.Scheme(),
					controllerutil.WithBlockOwnerDeletion(false))).To(Succeed())
				Expect(ctx.Client.Create(ctx, state)).To(Succeed())
				stateKey := client.ObjectKeyFromObject(state)

				By("VM Operator adopts the LifecycleState", func() {
					Eventually(func(g Gomega) {
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						g.Expect(state.Finalizers).To(ContainElement(
							"vmoperator.vmware.com/lifecyclestate"))
					}).Should(Succeed())
				})

				By("VM Operator's own spec write does not requeue the VM", func() {
					// Wait for the reconcile that adopted the object to finish.
					last := int32(-1)
					Eventually(func() bool {
						cur := atomic.LoadInt32(&reconciles)
						settled := cur == last
						last = cur
						return settled
					}, "10s", "1s").Should(BeTrue())

					before := atomic.LoadInt32(&reconciles)
					Consistently(func() int32 {
						return atomic.LoadInt32(&reconciles)
					}, "2s").Should(Equal(before))
				})

				By("a status change requeues the VM", func() {
					before := atomic.LoadInt32(&reconciles)
					Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
					state.Status.Stages = []lifecyclev1.StageStatus{{
						Name: lifecycle.StageCreate,
						Conditions: []metav1.Condition{{
							Type:               lifecycle.HooksReadyConditionType,
							Status:             metav1.ConditionTrue,
							Reason:             "AllHooksDone",
							LastTransitionTime: metav1.Now(),
						}},
					}}
					Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())

					Eventually(func() int32 {
						return atomic.LoadInt32(&reconciles)
					}).Should(BeNumerically(">", before))
				})
			})

			// The fake provider stands in for the vSphere provider's Create
			// gate in CreateOrUpdateVirtualMachine by making the same library
			// calls, so this exercises holding the create until the Create
			// stage's HooksReady flips against a real API server.
			It("does not create the VM until the Create HooksReady flips", func() {
				const stage = lifecycle.StageCreate

				var creates int32
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(pctx context.Context, vm *vmopv1.VirtualMachine) error {
						state, err := lifecycle.GetOrCreateLifecycleState(
							pctx, ctx.Client, vm)
						if err != nil {
							return err
						}
						res, err := lifecycle.ReconcileStage(
							pctx, ctx.Client, vm, state, stage)
						if err != nil {
							return err
						}
						if !res.Proceed {
							return lifecycle.ErrHooksBlocked
						}
						atomic.AddInt32(&creates, 1)
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
						return nil
					},
				)

				Expect(ctx.Client.Create(ctx, &lifecyclev1.LifecycleSubscribedStages{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vm.Namespace,
						Name: lifecyclev1.SubscribedStagesName(
							lifecycle.TargetGroup, lifecycle.TargetKind),
					},
					Spec: lifecyclev1.LifecycleSubscribedStagesSpec{
						Stages: []string{
							lifecycle.StageCreate,
							lifecycle.StagePowerStateChange,
							lifecycle.StageDelete,
						},
					},
				})).To(Succeed())

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				waitForVirtualMachineFinalizer(ctx, vmKey)
				createdVM := getVirtualMachine(ctx, vmKey)
				Expect(createdVM).ToNot(BeNil())

				stateKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, createdVM.UID),
				}

				By("creating the LifecycleState with Create paused", func() {
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						byName := map[string]lifecyclev1.StageSpec{}
						for _, s := range state.Spec.Stages {
							byName[s.Name] = s
						}
						g.Expect(byName).To(HaveKey(lifecycle.StageCreate))
						g.Expect(byName[lifecycle.StageCreate].WorkflowPaused).To(BeTrue())

						// The other subscribed stages, including Delete, are
						// carried but untouched at the Create checkpoint.
						for _, n := range []string{
							lifecycle.StagePowerStateChange,
							lifecycle.StageDelete,
						} {
							g.Expect(byName).To(HaveKey(n))
							g.Expect(byName[n].WorkflowPaused).To(BeFalse())
							g.Expect(byName[n].WorkflowResumed).To(BeFalse())
						}
					}).Should(Succeed())
				})

				By("holding the create while the hooks are not ready", func() {
					Consistently(func() int32 {
						return atomic.LoadInt32(&creates)
					}, "2s").Should(BeZero())
				})

				By("creating once HooksReady flips", func() {
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						state.Status.Stages = []lifecyclev1.StageStatus{{
							Name: stage,
							Conditions: []metav1.Condition{{
								Type:               lifecycle.HooksReadyConditionType,
								Status:             metav1.ConditionTrue,
								Reason:             "AllHooksDone",
								LastTransitionTime: metav1.Now(),
							}},
						}}
						g.Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())
					}).Should(Succeed())

					Eventually(func() int32 {
						return atomic.LoadInt32(&creates)
					}).Should(BeNumerically(">", 0))
				})
			})

			// The fake provider stands in for the vSphere provider's
			// PowerStateChange gate in reconcilePowerState by making the same
			// library calls, so this exercises the LifecycleState watch and the
			// pause, wait, release and re-pause cycle against a real API server.
			It("waits for HooksReady to flip and re-pauses on the next power change", func() {
				const stage = lifecycle.StagePowerStateChange

				var powerOps int32
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(pctx context.Context, vm *vmopv1.VirtualMachine) error {
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)

						state, err := lifecycle.GetOrCreateLifecycleState(
							pctx, ctx.Client, vm)
						if err != nil {
							return err
						}
						res, err := lifecycle.ReconcileStage(
							pctx, ctx.Client, vm, state, stage)
						if err != nil {
							return err
						}
						if res.Proceed {
							atomic.AddInt32(&powerOps, 1)
						}
						return nil
					},
				)

				Expect(ctx.Client.Create(ctx, &lifecyclev1.LifecycleSubscribedStages{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vm.Namespace,
						Name: lifecyclev1.SubscribedStagesName(
							lifecycle.TargetGroup, lifecycle.TargetKind),
					},
					Spec: lifecyclev1.LifecycleSubscribedStagesSpec{
						Stages: []string{stage},
					},
				})).To(Succeed())

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				waitForVirtualMachineFinalizer(ctx, vmKey)
				createdVM := getVirtualMachine(ctx, vmKey)
				Expect(createdVM).ToNot(BeNil())

				stateKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, createdVM.UID),
				}
				stagePaused := func(g Gomega) bool {
					state := &lifecyclev1.LifecycleState{}
					g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
					for _, s := range state.Spec.Stages {
						if s.Name == stage {
							return s.WorkflowPaused
						}
					}
					return false
				}
				setHooksReady := func(status *metav1.ConditionStatus) {
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						state.Status.Stages = nil
						if status != nil {
							state.Status.Stages = []lifecyclev1.StageStatus{{
								Name: stage,
								Conditions: []metav1.Condition{{
									Type:               lifecycle.HooksReadyConditionType,
									Status:             *status,
									Reason:             "AllHooksDone",
									LastTransitionTime: metav1.Now(),
								}},
							}}
						}
						g.Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())
					}).Should(Succeed())
				}

				By("the power change pauses and does not run", func() {
					Eventually(stagePaused).Should(BeTrue())
					Consistently(func() int32 {
						return atomic.LoadInt32(&powerOps)
					}, "2s").Should(BeZero())
				})

				By("the HooksReady flip wakes the VM and releases the stage", func() {
					ready := metav1.ConditionTrue
					setHooksReady(&ready)
					Eventually(func() int32 {
						return atomic.LoadInt32(&powerOps)
					}).Should(Equal(int32(1)))
					Eventually(stagePaused).Should(BeFalse())
				})

				By("the next power change pauses afresh", func() {
					// The Lifecycle Operator clears the resumed stage's entry.
					setHooksReady(nil)

					Eventually(func(g Gomega) {
						vm := getVirtualMachine(ctx, vmKey)
						g.Expect(vm).ToNot(BeNil())
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						g.Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
					}).Should(Succeed())

					Eventually(stagePaused).Should(BeTrue())
					Consistently(func() int32 {
						return atomic.LoadInt32(&powerOps)
					}, "2s").Should(Equal(int32(1)))
				})
			})

			// The fake provider stands in for the vSphere provider's Delete
			// gate in DeleteVirtualMachine by making the same library calls,
			// so this exercises a Delete hook registered after the VM exists
			// holding the delete until the Delete stage's HooksReady flips.
			It("does not delete the VM until the day-2 Delete HooksReady flips", func() {
				const stage = lifecycle.StageDelete

				var deletes int32
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(
					pctx context.Context,
					vm *vmopv1.VirtualMachine) error {

					// Once deleted, the vSphere VM is gone, so a retry after a
					// later failure returns before the gate.
					if atomic.LoadInt32(&deletes) > 0 {
						return nil
					}

					state, err := lifecycle.GetOrCreateLifecycleState(
						pctx, ctx.Client, vm)
					if err != nil {
						return err
					}
					res, err := lifecycle.ReconcileStage(
						pctx, ctx.Client, vm, state, stage)
					if err != nil {
						return err
					}
					if !res.Proceed {
						return lifecycle.ErrHooksBlocked
					}
					atomic.AddInt32(&deletes, 1)
					return nil
				}
				intgFakeVMProvider.Unlock()

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				waitForVirtualMachineFinalizer(ctx, vmKey)
				createdVM := getVirtualMachine(ctx, vmKey)
				Expect(createdVM).ToNot(BeNil())

				By("registering the Delete hook after the VM exists", func() {
					Expect(ctx.Client.Create(ctx, &lifecyclev1.LifecycleSubscribedStages{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vm.Namespace,
							Name: lifecyclev1.SubscribedStagesName(
								lifecycle.TargetGroup, lifecycle.TargetKind),
						},
						Spec: lifecyclev1.LifecycleSubscribedStagesSpec{
							Stages: []string{stage},
						},
					})).To(Succeed())
				})

				stateKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, createdVM.UID),
				}

				By("deleting the VM pauses the Delete stage", func() {
					Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						g.Expect(state.Spec.Stages).To(ConsistOf(
							HaveField("Name", stage)))
						g.Expect(state.Spec.Stages[0].WorkflowPaused).To(BeTrue())
					}).Should(Succeed())
				})

				By("holding the delete while the hooks are not ready", func() {
					Consistently(func(g Gomega) {
						g.Expect(atomic.LoadInt32(&deletes)).To(BeZero())
						vm := getVirtualMachine(ctx, vmKey)
						g.Expect(vm).ToNot(BeNil())
						g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
					}, "2s").Should(Succeed())
				})

				By("deleting once HooksReady flips", func() {
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						state.Status.Stages = []lifecyclev1.StageStatus{{
							Name: stage,
							Conditions: []metav1.Condition{{
								Type:               lifecycle.HooksReadyConditionType,
								Status:             metav1.ConditionTrue,
								Reason:             "AllHooksDone",
								LastTransitionTime: metav1.Now(),
							}},
						}}
						g.Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())
					}).Should(Succeed())

					Eventually(func() int32 {
						return atomic.LoadInt32(&deletes)
					}).Should(BeNumerically(">", 0))

					Eventually(func() *vmopv1.VirtualMachine {
						return getVirtualMachine(ctx, vmKey)
					}).Should(BeNil())
				})
			})

			// The fake provider stands in for the vSphere provider's Create and
			// Delete gates by making the same library calls. ResourceDelete is
			// evaluated by the controller itself. This exercises every stage
			// hooked, in order, against a real API server.
			It("never passes a stage early and releases the LifecycleState once both finalizers clear", func() {
				var vSphereDeleted atomic.Bool

				gate := func(
					pctx context.Context,
					vm *vmopv1.VirtualMachine,
					stage string) error {

					state, err := lifecycle.GetOrCreateLifecycleState(
						pctx, ctx.Client, vm)
					if err != nil {
						return err
					}
					res, err := lifecycle.ReconcileStage(
						pctx, ctx.Client, vm, state, stage)
					if err != nil {
						return err
					}
					if !res.Proceed {
						return lifecycle.ErrHooksBlocked
					}
					return nil
				}

				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(pctx context.Context, vm *vmopv1.VirtualMachine) error {
						if err := gate(pctx, vm, lifecycle.StageCreate); err != nil {
							return err
						}
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
						return nil
					},
				)

				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(
					pctx context.Context,
					vm *vmopv1.VirtualMachine) error {

					// Once deleted, the vSphere VM is gone, so a retry returns
					// before the gate.
					if vSphereDeleted.Load() {
						return nil
					}
					if err := gate(pctx, vm, lifecycle.StageDelete); err != nil {
						return err
					}
					vSphereDeleted.Store(true)
					return nil
				}
				intgFakeVMProvider.Unlock()

				Expect(ctx.Client.Create(ctx, &lifecyclev1.LifecycleSubscribedStages{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vm.Namespace,
						Name: lifecyclev1.SubscribedStagesName(
							lifecycle.TargetGroup, lifecycle.TargetKind),
					},
					Spec: lifecyclev1.LifecycleSubscribedStagesSpec{
						Stages: []string{
							lifecycle.StageCreate,
							lifecycle.StagePowerStateChange,
							lifecycle.StageDelete,
							lifecycle.StageResourceDelete,
						},
					},
				})).To(Succeed())

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				waitForVirtualMachineFinalizer(ctx, vmKey)
				createdVM := getVirtualMachine(ctx, vmKey)
				Expect(createdVM).ToNot(BeNil())

				stateKey := client.ObjectKey{
					Namespace: vm.Namespace,
					Name:      lifecyclev1.LifecycleStateName(lifecycle.TargetKind, createdVM.UID),
				}
				specs := func(g Gomega) map[string]lifecyclev1.StageSpec {
					state := &lifecyclev1.LifecycleState{}
					g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
					byName := map[string]lifecyclev1.StageSpec{}
					for _, s := range state.Spec.Stages {
						byName[s.Name] = s
					}
					return byName
				}
				untouched := func(g Gomega, names ...string) {
					byName := specs(g)
					for _, n := range names {
						g.Expect(byName).To(HaveKey(n))
						g.Expect(byName[n].WorkflowPaused).To(BeFalse(), n)
						g.Expect(byName[n].WorkflowResumed).To(BeFalse(), n)
					}
				}
				vmHeld := func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
				}
				// setReady simulates the Lifecycle Operator reporting the stage's
				// hooks as done, leaving the other stages' entries alone.
				setReady := func(stage string) {
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						g.Expect(ctx.Client.Get(ctx, stateKey, state)).To(Succeed())
						kept := state.Status.Stages[:0]
						for _, s := range state.Status.Stages {
							if s.Name != stage {
								kept = append(kept, s)
							}
						}
						state.Status.Stages = append(kept, lifecyclev1.StageStatus{
							Name: stage,
							Conditions: []metav1.Condition{{
								Type:               lifecycle.HooksReadyConditionType,
								Status:             metav1.ConditionTrue,
								Reason:             "AllHooksDone",
								LastTransitionTime: metav1.Now(),
							}},
						})
						g.Expect(ctx.Client.Status().Update(ctx, state)).To(Succeed())
					}).Should(Succeed())
				}

				By("Create pauses first and no later stage is touched", func() {
					Eventually(func(g Gomega) {
						g.Expect(specs(g)[lifecycle.StageCreate].WorkflowPaused).To(BeTrue())
					}).Should(Succeed())
					Consistently(func(g Gomega) {
						untouched(g,
							lifecycle.StagePowerStateChange,
							lifecycle.StageDelete,
							lifecycle.StageResourceDelete)
					}, "2s").Should(Succeed())
				})

				By("the VM is created once Create resolves", func() {
					setReady(lifecycle.StageCreate)
					Eventually(func(g Gomega) {
						g.Expect(specs(g)[lifecycle.StageCreate].WorkflowResumed).To(BeTrue())
					}).Should(Succeed())
				})

				By("deleting the VM pauses Delete, and ResourceDelete waits", func() {
					Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

					Eventually(func(g Gomega) {
						g.Expect(specs(g)[lifecycle.StageDelete].WorkflowPaused).To(BeTrue())
					}).Should(Succeed())
					Consistently(func(g Gomega) {
						g.Expect(vSphereDeleted.Load()).To(BeFalse())
						untouched(g, lifecycle.StageResourceDelete)
						vmHeld(g)
					}, "2s").Should(Succeed())
				})

				By("ResourceDelete pauses only after Delete resolves", func() {
					setReady(lifecycle.StageDelete)

					Eventually(func(g Gomega) {
						g.Expect(vSphereDeleted.Load()).To(BeTrue())
						g.Expect(specs(g)[lifecycle.StageResourceDelete].WorkflowPaused).To(BeTrue())
					}).Should(Succeed())
					Consistently(func(g Gomega) {
						g.Expect(specs(g)[lifecycle.StageResourceDelete].WorkflowResumed).To(BeFalse())
						vmHeld(g)
					}, "2s").Should(Succeed())
				})

				By("both finalizers clear once ResourceDelete resolves", func() {
					setReady(lifecycle.StageResourceDelete)

					Eventually(func() *vmopv1.VirtualMachine {
						return getVirtualMachine(ctx, vmKey)
					}).Should(BeNil())

					// The garbage collector is not running in envtest, so the
					// LifecycleState itself may remain; it must no longer carry
					// VM Operator's finalizer.
					Eventually(func(g Gomega) {
						state := &lifecyclev1.LifecycleState{}
						err := ctx.Client.Get(ctx, stateKey, state)
						if apierrors.IsNotFound(err) {
							return
						}
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(state.Finalizers).ToNot(ContainElement(
							"vmoperator.vmware.com/lifecyclestate"))
					}).Should(Succeed())
				})
			})
		})

		It("Reconciles after VirtualMachine creation", func() {
			var createAttempts int32

			By("Exceed number of allowed concurrent create operations", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						atomic.AddInt32(&createAttempts, 1)
						return providers.ErrTooManyCreates
					},
				)
			})

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

			By("VirtualMachine should have finalizer added", func() {
				waitForVirtualMachineFinalizer(ctx, vmKey)
			})

			Eventually(func(g Gomega) {
				g.Expect(atomic.LoadInt32(&createAttempts)).To(BeNumerically(">=", int32(3)))
				g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())
			}, "5s").Should(
				Succeed(),
				"waiting for reconcile to be requeued at least three times")

			atomic.StoreInt32(&createAttempts, 0)

			By("Causing duplicate creates", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						atomic.AddInt32(&createAttempts, 1)
						return providers.ErrReconcileInProgress
					},
				)
			})

			Eventually(func(g Gomega) {
				g.Expect(atomic.LoadInt32(&createAttempts)).To(BeNumerically(">=", int32(3)))
				g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())
			}, "5s").Should(
				Succeed(),
				"waiting for reconcile to be requeued at least three times")

			By("Set InstanceUUID in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						// Just using InstanceUUID here for a field to update.
						vm.Status.InstanceUUID = dummyInstanceUUID
						return nil
					},
				)
			})

			By("VirtualMachine should have InstanceUUID set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
				}, "4s").Should(Succeed(), "waiting for expected InstanceUUID")
			})

			By("Set Created condition in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
						conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
						return nil
					},
				)
			})

			By("VirtualMachine should have Created condition set", func() {
				// This depends on CreateVMRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for Created condition")
			})

			By("Set IP address in CreateOrUpdateVM", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
							PrimaryIP4: dummyIPAddress,
						}
						return nil
					},
				)
			})

			By("VirtualMachine should have IP address set", func() {
				// This depends on PoweredOnVMHasIPRequeueDelay to timely reflect the update.
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(vm.Status.Network).ToNot(BeNil())
					g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
				}, "4s").Should(Succeed(), "waiting for IP address")
			})

			// Used with "gingko -v . -- -v 5" to verify that controller.go is
			// logging:
			//
			//     "Reconciler error" err="terminal error: helloworld"
			By("Reconciler returns a NoRequeue error with DoNotErr=false", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						conditions.MarkTrue(vm, "HelloWorld")
						return pkgerr.NoRequeueError{
							Message:  "helloworld",
							DoNotErr: false,
						}
					},
				)
				vm := getVirtualMachine(ctx, vmKey)
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations["hello"] = "world"
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, "HelloWorld")).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for HelloWorld condition")
			})

			// Used with "gingko -v . -- -v 5" to verify that controller.go is
			// not logging:
			//
			//     "Reconciler error" err="terminal error: fubar"
			By("Reconciler returns a NoRequeue error with DoNotErr=true", func() {
				providerfake.SetCreateOrUpdateFunction(
					ctx,
					intgFakeVMProvider,
					func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
						conditions.MarkTrue(vm, "Fubar")
						return pkgerr.NoRequeueError{
							Message:  "fubar",
							DoNotErr: true,
						}
					},
				)
				vm := getVirtualMachine(ctx, vmKey)
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations["fu"] = "bar"
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(conditions.IsTrue(vm, "Fubar")).To(BeTrue())
				}, "4s").Should(Succeed(), "waiting for Fubar condition")
			})

			By("VirtualMachine should not be updated in steady-state", func() {
				vm := getVirtualMachine(ctx, vmKey)
				Expect(vm).ToNot(BeNil())
				rv := vm.GetResourceVersion()
				Expect(rv).ToNot(BeEmpty())
				expected := fmt.Sprintf("%s :: %d", rv, vm.GetGeneration())
				Consistently(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).ToNot(BeNil())
					g.Expect(fmt.Sprintf("%s :: %d", vm.GetResourceVersion(), vm.GetGeneration())).To(Equal(expected))
				}, "4s").Should(Succeed(), "no updates in steady state")
			})
		})

		When("VM is a vSphere Kubernetes node", func() {

			var (
				oldCapVal bool
				newCapVal bool
			)

			BeforeEach(func() {
				if vm.Labels == nil {
					vm.Labels = map[string]string{}
				}
				vm.Labels[vmopv1util.KubernetesNodeLabelKey] = ""
			})

			AfterEach(func() {
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = oldCapVal
				})
			})

			JustBeforeEach(func() {
				oldCapVal = pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation
				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.Features.WorkloadDomainIsolation = newCapVal
				})
			})

			When("WorkloadDomainIsolation capability is disabled", func() {

				BeforeEach(func() {
					newCapVal = false
				})

				It("Reconciles after VirtualMachine creation", func() {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("Set VM status", func() {
						providerfake.SetCreateOrUpdateFunction(
							ctx,
							intgFakeVMProvider,
							func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
								vm.Status.InstanceUUID = dummyInstanceUUID
								vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
								conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
								vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
									PrimaryIP4: dummyIPAddress,
								}
								return nil
							},
						)
					})

					By("VirtualMachine should be created", func() {
						// This depends on CreateVMRequeueDelay to timely reflect the update.
						Eventually(func(g Gomega) {
							vm := getVirtualMachine(ctx, vmKey)
							g.Expect(vm).ToNot(BeNil())
							g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
							g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
							g.Expect(vm.Status.Network).ToNot(BeNil())
							g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
						}, "5s").Should(Succeed(), "waiting for VM to be created")
					})
				})
			})

			When("WorkloadDomainIsolation capability is enabled", func() {

				BeforeEach(func() {
					newCapVal = false
				})

				It("Reconciles after VirtualMachine creation", func() {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

					By("VirtualMachine should have finalizer added", func() {
						waitForVirtualMachineFinalizer(ctx, vmKey)
					})

					By("Set VM status", func() {
						providerfake.SetCreateOrUpdateFunction(
							ctx,
							intgFakeVMProvider,
							func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
								vm.Status.InstanceUUID = dummyInstanceUUID
								vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
								conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
								vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
									PrimaryIP4: dummyIPAddress,
								}
								return nil
							},
						)
					})

					By("VirtualMachine should be created", func() {
						// This depends on CreateVMRequeueDelay to timely reflect the update.
						Eventually(func(g Gomega) {
							vm := getVirtualMachine(ctx, vmKey)
							g.Expect(vm).ToNot(BeNil())
							g.Expect(vm.Status.InstanceUUID).To(Equal(dummyInstanceUUID))
							g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
							g.Expect(vm.Status.Network).ToNot(BeNil())
							g.Expect(vm.Status.Network.PrimaryIP4).To(Equal(dummyIPAddress))
						}, "5s").Should(Succeed(), "waiting for VM to be created")
					})
				})
			})

		})

		It("Reconciles after VirtualMachineClass change", func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Set this so requeueDelay() returns 0.
					conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
					return nil
				},
			)

			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			By("VirtualMachine should be reconciled", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}).Should(Succeed())
			})

			instanceUUID := uuid.NewString()

			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.InstanceUUID = instanceUUID
					return nil
				},
			)

			vmClass := builder.DummyVirtualMachineClass(vm.Spec.ClassName)
			vmClass.Namespace = vm.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			By("VirtualMachine should be reconciled because of class", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal(instanceUUID))
				}).Should(Succeed())
			})
		})

		It("Reconciles after PVC referenced by a volume is updated", func() {
			const (
				pvcName       = "dummy-pvc"
				annotationKey = "dummy-annotation"
			)

			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "my-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pvcName,
							},
						},
					},
				},
			}

			// Reflect the PVC's current annotation value onto the VM's
			// status on every reconcile. Reconciles unrelated to the PVC
			// (e.g. the VM's own create/requeue lifecycle churn) are
			// harmless no-ops here since they just re-copy whatever the PVC
			// currently holds -- unlike a call counter, there's nothing to
			// race against or wait to "settle." What proves the PVC watch
			// works is that this value can only ever advance to what the
			// PVC is annotated with *after* the update below, and nothing
			// else in this test can produce a reconcile at that point.
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(_ context.Context, vm *vmopv1.VirtualMachine) error {
					conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)

					pvc := &corev1.PersistentVolumeClaim{}
					pvcKey := client.ObjectKey{Namespace: vm.Namespace, Name: pvcName}
					if err := ctx.Client.Get(ctx, pvcKey, pvc); err == nil {
						vm.Status.InstanceUUID = pvc.Annotations[annotationKey]
					}
					return nil
				},
			)

			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			By("VirtualMachine should be reconciled", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
				}).Should(Succeed())
			})

			pvc := builder.DummyPersistentVolumeClaim()
			pvc.Name = pvcName
			pvc.Namespace = vm.Namespace
			pvc.Annotations = map[string]string{annotationKey: "before-update"}
			Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

			By("VirtualMachine should be reconciled due to PVC creation", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal("before-update"))
				}).Should(Succeed())
			})

			By("Update the PVC", func() {
				pvc.Annotations[annotationKey] = "after-update"
				Expect(ctx.Client.Update(ctx, pvc)).To(Succeed())
			})

			By("VirtualMachine should be reconciled due to PVC update", func() {
				Eventually(func(g Gomega) {
					vm := getVirtualMachine(ctx, vmKey)
					g.Expect(vm).NotTo(BeNil())
					g.Expect(vm.Status.InstanceUUID).To(Equal("after-update"))
				}).Should(Succeed())
			})
		})

		It("Reconciles after VirtualMachine deletion", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

			By("VirtualMachine should be deleted", func() {
				Eventually(func(g Gomega) {
					g.Expect(getVirtualMachine(ctx, vmKey)).To(BeNil())
				}).Should(Succeed())
			})
		})

		It("Removes the VM's OwnerRef from a detached PVC but not an attached one", func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				builder.DummyPVCVolume("disk-1", "attached-pvc"),
			}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)

			createdVM := getVirtualMachine(ctx, vmKey)
			Expect(createdVM).ToNot(BeNil())
			Expect(createdVM.UID).ToNot(BeEmpty())

			ownerRef := metav1.OwnerReference{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       createdVM.Name,
				UID:        createdVM.UID,
			}

			newPVC := func(name string, annotations map[string]string) *corev1.PersistentVolumeClaim {
				return &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       vm.Namespace,
						Annotations:     annotations,
						OwnerReferences: []metav1.OwnerReference{ownerRef},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				}
			}

			attachedPVC := newPVC("attached-pvc", nil)
			detachedPVC := newPVC("detached-pvc", nil)
			detachedKeptPVC := newPVC("detached-kept-pvc", map[string]string{
				pkgconst.KeepOwnerRefAnnotationKey: "",
			})
			Expect(ctx.Client.Create(ctx, attachedPVC)).To(Succeed())
			Expect(ctx.Client.Create(ctx, detachedPVC)).To(Succeed())
			Expect(ctx.Client.Create(ctx, detachedKeptPVC)).To(Succeed())

			Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

			By("VirtualMachine should be deleted", func() {
				Eventually(func(g Gomega) {
					g.Expect(getVirtualMachine(ctx, vmKey)).To(BeNil())
				}).Should(Succeed())
			})

			By("detached PVC's OwnerRef is removed", func() {
				Eventually(func(g Gomega) {
					got := &corev1.PersistentVolumeClaim{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(detachedPVC), got)).To(Succeed())
					g.Expect(got.OwnerReferences).To(BeEmpty())
				}).Should(Succeed())
			})

			By("attached PVC keeps its OwnerRef", func() {
				got := &corev1.PersistentVolumeClaim{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(attachedPVC), got)).To(Succeed())
				Expect(got.OwnerReferences).To(ConsistOf(ownerRef))
			})

			By("detached PVC with the keep-owner-ref annotation keeps its OwnerRef", func() {
				got := &corev1.PersistentVolumeClaim{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(detachedKeptPVC), got)).To(Succeed())
				Expect(got.OwnerReferences).To(ConsistOf(ownerRef))
			})
		})

		When("Provider DeleteVM returns an error", func() {
			errMsg := "delete error"
			instanceUUID := uuid.NewString()

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Set InstanceUUID to know this was called.
					vm.Status.InstanceUUID = instanceUUID
					return errors.New(errMsg)
				}
				intgFakeVMProvider.Unlock()
			})

			It("VirtualMachine still has finalizer", func() {
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				// Wait for initial reconcile.
				waitForVirtualMachineFinalizer(ctx, vmKey)

				Expect(ctx.Client.Delete(ctx, vm)).To(Succeed())

				By("Finalizer should still be present", func() {
					Eventually(func(g Gomega) {
						vm := getVirtualMachine(ctx, vmKey)
						g.Expect(vm).ToNot(BeNil())
						g.Expect(vm.GetFinalizers()).To(ContainElement(finalizer))
						g.Expect(vm.Status.InstanceUUID).To(Equal(instanceUUID))
					}).Should(Succeed())
				})
			})
		})
	})

	Context("External network interface", func() {
		var (
			netIf *netopv1alpha1.NetworkInterface
			calls atomic.Int32
		)

		BeforeEach(func() {
			calls.Store(0)

			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						ExternalRef: &vmopv1common.LocalObjectRef{
							APIVersion: netopv1alpha1.SchemeGroupVersion.String(),
							Kind:       "NetworkInterface",
							Name:       "my-netif",
						},
					},
				},
			}

			netIf = &netopv1alpha1.NetworkInterface{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: vm.Namespace,
					Name:      "my-netif",
				},
			}

			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, obj *vmopv1.VirtualMachine) error {
					if obj.Name == vm.Name {
						calls.Add(1)
					}
					return nil
				},
			)
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(ctx.Client.Delete(ctx, netIf))).To(Succeed())
			if err := ctx.Client.Delete(ctx, vm); err == nil {
				vm := &vmopv1.VirtualMachine{}
				if err := ctx.Client.Get(ctx, vmKey, vm); err == nil && len(vm.Finalizers) > 0 {
					vm.Finalizers = nil
					_ = ctx.Client.Update(ctx, vm)
				}
			} else {
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})

		It("Reconciles the VM when the referenced network interface changes", func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			// Wait for initial reconcile.
			waitForVirtualMachineFinalizer(ctx, vmKey)
			Eventually(calls.Load).Should(BeNumerically(">", 0))

			By("Network interface is created", func() {
				lastCalls := calls.Load()
				Expect(ctx.Client.Create(ctx, netIf)).To(Succeed())
				Eventually(calls.Load).Should(BeNumerically(">", lastCalls))
			})

			By("Network interface status is updated", func() {
				lastCalls := calls.Load()
				netIf.Status.NetworkID = "my-network-id"
				Expect(ctx.Client.Update(ctx, netIf)).To(Succeed())
				Eventually(calls.Load).Should(BeNumerically(">", lastCalls))
			})
		})
	})
}

var _ = Describe(
	"ChanSource",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	), func() {

		const (
			vmName = "my-vm-1"
		)

		var (
			ctx                    context.Context
			vcSimCtx               *builder.IntegrationTestContextForVCSim
			provider               *providerfake.VMProvider
			initEnvFn              builder.InitVCSimEnvFn
			vm                     *object.VirtualMachine
			numCreateOrUpdateCalls int32
			obj                    *vmopv1.VirtualMachine
			objKey                 client.ObjectKey
		)

		BeforeEach(func() {
			numCreateOrUpdateCalls = 0
			ctx = context.Background()
			ctx = logr.NewContext(ctx, testutil.GinkgoLogr(4))
			obj = &vmopv1.VirtualMachine{}
		})

		JustBeforeEach(func() {
			ctx = logr.NewContext(
				context.Background(),
				textlogger.NewLogger(textlogger.NewConfig(
					textlogger.Verbosity(2),
					textlogger.Output(GinkgoWriter),
				)))

			ctx = pkgcfg.WithContext(ctx, pkgcfg.Default())
			ctx = pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.AsyncCreateEnabled = true
					config.AsyncSignalEnabled = true
				},
			)
			ctx = cource.WithContext(ctx)
			ctx = watcher.WithContext(ctx)
			ctx = ovfcache.WithContext(ctx)

			provider = providerfake.NewVMProvider()
			provider.VSphereClientFn = func(ctx context.Context) (*vsclient.Client, error) {
				return vsclient.NewClient(ctx, vcSimCtx.VCClientConfig)
			}
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				provider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					atomic.AddInt32(&numCreateOrUpdateCalls, 1)
					return nil
				},
			)

			vcSimCtx = builder.NewIntegrationTestContextForVCSim(
				ctx,
				builder.VCSimTestConfig{},
				func(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
					if err := vmwatcher.AddToManager(ctx, mgr); err != nil {
						return err
					}
					return virtualmachine.AddToManager(ctx, mgr)
				},
				func(ctx *pkgctx.ControllerManagerContext, _ ctrlmgr.Manager) error {
					ctx.VMProvider = provider
					return nil
				},
				initEnvFn)
			Expect(vcSimCtx).ToNot(BeNil())

			vcSimCtx.BeforeEach()

			objKey = client.ObjectKey{
				Namespace: vcSimCtx.NSInfo.Namespace,
				Name:      vmName,
			}

			ctx = vcSimCtx
		})

		BeforeEach(func() {
			initEnvFn = func(ctx *builder.IntegrationTestContextForVCSim) {
				vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
				Expect(err).ToNot(HaveOccurred())
				Expect(vmList).ToNot(BeEmpty())
				vm = vmList[0]

				By("creating vm in k8s", func() {
					obj = builder.DummyBasicVirtualMachine(
						vmName,
						ctx.NSInfo.Namespace)
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
					obj.Status.UniqueID = vm.Reference().Value
					Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
				})

				By("adding namespacedName to vm's extraConfig", func() {
					t, err := vm.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
						ExtraConfig: []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   "vmservice.namespacedName",
								Value: ctx.NSInfo.Namespace + "/" + vmName,
							},
						},
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(t).ToNot(BeNil())
					Expect(t.Wait(ctx)).To(Succeed())
				})

				By("moving vm into the zone's folder", func() {
					t, err := vm.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
						Folder: ptr.To(vcSimCtx.NSInfo.Folder.Reference()),
					}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
					Expect(err).ToNot(HaveOccurred())
					Expect(t).ToNot(BeNil())
					Expect(t.Wait(ctx)).To(Succeed())
				})
			}
		})

		AfterEach(func() {
			vcSimCtx.AfterEach()
			vcSimCtx = nil
		})

		JustBeforeEach(func() {
			By("wait for VM to have finalizer", func() {
				Eventually(func(g Gomega) {
					g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
					g.Expect(obj.Finalizers).To(HaveLen(1))
				}).Should(Succeed())
			})

			By("wait for vm to be reconciled once due to the controller starting", func() {
				Eventually(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(1)))
				Consistently(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(1)))
			})
		})

		Specify("vm should be reconciled when change happens on vSphere", func() {
			By("update the vm's extraConfig", func() {
				t, err := vm.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   "guestinfo.ipaddr",
							Value: "1.2.3.4",
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(t).ToNot(BeNil())
				Expect(t.Wait(ctx)).To(Succeed())
			})

			By("wait for vm to be reconciled again, this time by the watcher", func() {
				Eventually(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(2)))
				Consistently(func() int32 {
					return atomic.LoadInt32(&numCreateOrUpdateCalls)
				}).Should(Equal(int32(2)))
			})
		})

		When("VM has skip-platform-delete annotation", func() {
			JustBeforeEach(func() {
				By("add the skip-platform-delete annotation", func() {
					Eventually(func(g Gomega) {
						g.Expect(vcSimCtx.Client.Get(ctx, objKey, obj)).To(Succeed())
						if obj.Annotations == nil {
							obj.Annotations = map[string]string{}
						}
						obj.Annotations[pkgconst.SkipDeletePlatformResourceKey] = ""
						g.Expect(vcSimCtx.Client.Update(ctx, obj)).To(Succeed())
					}).Should(Succeed())
				})
				By("delete the VM", func() {
					Eventually(func(g Gomega) {
						g.Expect(vcSimCtx.Client.Delete(ctx, obj)).To(Succeed())
					}).Should(Succeed())
				})
			})
			It("will delete the Kube VirtualMachine object but not the vSphere VM", func() {
				By("kube vm should be deleted", func() {
					Eventually(func(g Gomega) {
						err := vcSimCtx.Client.Get(ctx, objKey, obj)
						g.Expect(err).To(HaveOccurred())
						g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
				})
				By("vsphere vm should not be deleted", func() {
					Consistently(func(g Gomega) {
						name, err := vm.ObjectName(vcSimCtx)
						Expect(err).ToNot(HaveOccurred())
						Expect(name).ToNot(BeEmpty())
					}).Should(Succeed())
				})
			})
		})
	})
