// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinereplicaset"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "virtualmachinereplicaset.vmoperator.vmware.com"
)

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

const unitTestNamespace = "dummy-ns"

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		interceptFn interceptor.Funcs

		// failCreates/createErr and failDeletes/deleteErr are read by the
		// Create/Delete interceptors registered below; setting them from
		// within an It (after JustBeforeEach has already run) still takes
		// effect because the interceptor closures read them at call time,
		// not registration time.
		failCreates bool
		createErr   error
		failDeletes bool
		deleteErr   error

		ctx        *builder.UnitTestContextForController
		reconciler *virtualmachinereplicaset.Reconciler

		rs    *vmopv1.VirtualMachineReplicaSet
		rsKey types.NamespacedName
	)

	BeforeEach(func() {
		initObjects = nil
		failCreates = false
		createErr = nil
		failDeletes = false
		deleteErr = nil
		interceptFn = interceptor.Funcs{
			Create: func(
				ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*vmopv1.VirtualMachine); ok && failCreates {
					return createErr
				}
				return c.Create(ctx, obj, opts...)
			},
			Delete: func(
				ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*vmopv1.VirtualMachine); ok && failDeletes {
					return deleteErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}

		rs = builder.DummyVirtualMachineReplicaSet()
		rs.Namespace = unitTestNamespace
		rs.Name = "dummy-rs"
		rs.Spec.Replicas = ptr.To(int32(3))
		rs.Spec.Selector.MatchLabels = map[string]string{"appname": "dummy"}
		rs.Spec.Template.Labels = map[string]string{"appname": "dummy"}
	})

	JustBeforeEach(func() {
		initObjects = append(initObjects, rs)
		ctx = suite.NewUnitTestContextForControllerWithFuncs(interceptFn, initObjects...)
		reconciler = virtualmachinereplicaset.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)
		rsKey = types.NamespacedName{Namespace: rs.Namespace, Name: rs.Name}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
	})

	// reconcileOnce performs a single Reconcile call against rsKey.
	reconcileOnce := func() (ctrl.Result, error) {
		return reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: rsKey})
	}

	// reconcileN performs n sequential Reconcile calls, asserting each
	// succeeds. The first call after creation only adds the finalizer and
	// returns early, so callers typically need at least 2 calls to reach a
	// converged state.
	reconcileN := func(n int) {
		for i := 0; i < n; i++ {
			_, err := reconcileOnce()
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
		}
	}

	getRS := func() *vmopv1.VirtualMachineReplicaSet {
		obj := &vmopv1.VirtualMachineReplicaSet{}
		ExpectWithOffset(1, ctx.Client.Get(ctx, rsKey, obj)).To(Succeed())
		return obj
	}

	listVMsByLabels := func(labels map[string]string) []vmopv1.VirtualMachine {
		vmList := &vmopv1.VirtualMachineList{}
		ExpectWithOffset(1, ctx.Client.List(
			ctx, vmList, client.InNamespace(unitTestNamespace), client.MatchingLabels(labels),
		)).To(Succeed())
		return vmList.Items
	}

	// markAllVMsReady marks every VM owned by rs Ready=True, simulating what
	// the (not-running-in-this-suite) VirtualMachine controller would
	// eventually do. Needed for any assertion depending on
	// status.readyReplicas/the Resized/VirtualMachinesReady conditions,
	// since nothing else in this test sets a VM's Ready condition.
	markAllVMsReady := func() {
		for _, vm := range listVMsByLabels(rs.Spec.Selector.MatchLabels) {
			conditions.MarkTrue(&vm, vmopv1.ReadyConditionType)
			ExpectWithOffset(1, ctx.Client.Status().Update(ctx, &vm)).To(Succeed())
		}
	}

	listAllVMs := func() []vmopv1.VirtualMachine {
		vmList := &vmopv1.VirtualMachineList{}
		ExpectWithOffset(1, ctx.Client.List(ctx, vmList, client.InNamespace(unitTestNamespace))).To(Succeed())
		return vmList.Items
	}

	vmNames := func(vms []vmopv1.VirtualMachine) []string {
		names := make([]string, 0, len(vms))
		for _, vm := range vms {
			names = append(names, vm.Name)
		}
		return names
	}

	Context("Reconcile", func() {

		Context("basic reconciliation", func() {

			// SC1: create with N replicas from scratch.
			It("creates exactly N owned VirtualMachines matching the template", func() {
				// 3 calls: add finalizer, create the VMs, then let status
				// catch up to the just-created VMs (status reflects the
				// pre-sync snapshot from the reconcile that did the create).
				reconcileN(3)

				vms := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(vms).To(HaveLen(3))

				seenNames := map[string]bool{}
				for _, vm := range vms {
					Expect(seenNames[vm.Name]).To(BeFalse(), "expected unique generated VM name")
					seenNames[vm.Name] = true

					ownerRef := metav1.GetControllerOfNoCopy(&vm)
					Expect(ownerRef).ToNot(BeNil())
					Expect(ownerRef.Name).To(Equal(rs.Name))
					Expect(ownerRef.Kind).To(Equal("VirtualMachineReplicaSet"))

					for k, v := range rs.Spec.Template.Labels {
						Expect(vm.Labels).To(HaveKeyWithValue(k, v))
					}
					Expect(vm.Labels).To(HaveKeyWithValue(vmopv1.VirtualMachineReplicaSetNameLabel, rs.Name))
					Expect(vm.Spec).To(Equal(rs.Spec.Template.Spec))
				}

				updated := getRS()
				Expect(updated.Status.Replicas).To(Equal(int32(3)))
				Expect(updated.Status.FullyLabeledReplicas).To(Equal(int32(3)))
			})

			// SC2: replicas defaulting. Fake clients don't run CRD defaulting
			// (that's an API-server concern, not this controller's), so this
			// only asserts the controller's behavior given the *default
			// value* (1) has already been applied -- not that defaulting
			// itself occurs.
			When("spec.replicas is 1 (the CRD default value)", func() {
				BeforeEach(func() {
					rs.Spec.Replicas = ptr.To(int32(1))
				})

				It("creates exactly one VirtualMachine", func() {
					reconcileN(3)
					Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(HaveLen(1))
					Expect(getRS().Status.Replicas).To(Equal(int32(1)))
				})
			})

			// SC3: explicit zero replicas.
			When("spec.replicas is explicitly 0", func() {
				BeforeEach(func() {
					rs.Spec.Replicas = ptr.To(int32(0))
				})

				It("creates no VirtualMachine objects", func() {
					reconcileN(2)
					Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(BeEmpty())
					Expect(getRS().Status.Replicas).To(Equal(int32(0)))
				})
			})

			When("scaled down to 0 from a higher replica count", func() {
				It("deletes all existing owned VirtualMachines", func() {
					reconcileN(2)
					Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(HaveLen(3))

					rs = getRS()
					rs.Spec.Replicas = ptr.To(int32(0))
					Expect(ctx.Client.Update(ctx, rs)).To(Succeed())

					// 2 calls: delete the VMs, then let status catch up.
					reconcileN(2)
					Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(BeEmpty())
					Expect(getRS().Status.Replicas).To(Equal(int32(0)))
				})
			})

			// SC4 & SC36 (idempotent / no auto-scaling side effects).
			When("already converged", func() {
				It("is a no-op on subsequent reconciles", func() {
					reconcileN(3)
					before := getRS()
					beforeVMs := listAllVMs()

					_, err := reconcileOnce()
					Expect(err).ToNot(HaveOccurred())

					after := getRS()
					afterVMs := listAllVMs()

					Expect(after.Status.Replicas).To(Equal(before.Status.Replicas))
					Expect(after.Status.ObservedGeneration).To(Equal(after.Generation))
					Expect(vmNames(afterVMs)).To(ConsistOf(vmNames(beforeVMs)))
				})
			})

			// SC5: a fresh Reconciler instance (simulating a controller
			// restart / cold cache) against the same steady state produces
			// the same result.
			When("reconciled by a fresh Reconciler instance", func() {
				It("produces the same converged result", func() {
					reconcileN(3)
					before := getRS()
					beforeVMs := listVMsByLabels(rs.Spec.Selector.MatchLabels)

					freshReconciler := virtualmachinereplicaset.NewReconciler(
						ctx, ctx.Client, ctx.Logger, ctx.Recorder)
					_, err := freshReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: rsKey})
					Expect(err).ToNot(HaveOccurred())

					after := getRS()
					afterVMs := listVMsByLabels(rs.Spec.Selector.MatchLabels)
					Expect(after.Status.Replicas).To(Equal(before.Status.Replicas))
					Expect(vmNames(afterVMs)).To(ConsistOf(vmNames(beforeVMs)))
				})
			})

			// SC9: scale to zero then back up never reuses names.
			It("creates fresh VirtualMachines with new names when scaled back up from zero", func() {
				reconcileN(2)
				original := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(original).To(HaveLen(3))
				originalNames := vmNames(original)

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(0))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)
				Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(BeEmpty())

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(3))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)

				newVMs := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(newVMs).To(HaveLen(3))
				for _, name := range vmNames(newVMs) {
					Expect(originalNames).ToNot(ContainElement(name))
				}
			})

			// SC10: rapid successive scale edits converge on the final value.
			It("converges to the final replica count after rapid successive edits", func() {
				rs.Spec.Replicas = ptr.To(int32(1))
				Expect(ctx.Client.Update(ctx, rs)).ToNot(HaveOccurred())
				// The above Update happens before the first Get in
				// JustBeforeEach's key computation isn't affected; use the
				// key directly.
				reconcileN(2)
				Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(HaveLen(1))

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(5))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)
				Expect(listVMsByLabels(rs.Spec.Selector.MatchLabels)).To(HaveLen(5))

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(2))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				// 2 calls: delete down to 2, then let status catch up.
				reconcileN(2)

				finalVMs := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(finalVMs).To(HaveLen(2))
				Expect(getRS().Status.Replicas).To(Equal(int32(2)))
			})
		})

		Context("status and conditions", func() {

			// SC23: fullyLabeledReplicas can diverge from replicas.
			It("decrements fullyLabeledReplicas when an owned VM's template labels drift, while replicas stays put", func() {
				rs.Spec.Template.Labels = map[string]string{"appname": "dummy", "tier": "web"}
				Expect(ctx.Client.Update(ctx, rs)).ToNot(HaveOccurred())

				reconcileN(2)
				vms := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(vms).To(HaveLen(3))

				drifted := vms[0]
				delete(drifted.Labels, "tier")
				Expect(ctx.Client.Update(ctx, &drifted)).To(Succeed())

				reconcileN(1)
				updated := getRS()
				Expect(updated.Status.Replicas).To(Equal(int32(3)))
				Expect(updated.Status.FullyLabeledReplicas).To(Equal(int32(2)))
			})

			// SC25: VirtualMachinesCreated reflects create-path failures and
			// self-heals once the underlying error is resolved.
			It("marks VirtualMachinesCreated False on create failure, then True once creates succeed", func() {
				createErr = errors.New("no VirtualMachineClass named dummy-class")
				failCreates = true

				reconcileN(1) // adds finalizer only
				_, err := reconcileOnce()
				Expect(err).To(HaveOccurred())

				afterFailure := getRS()
				Expect(conditions.IsFalse(afterFailure, vmopv1.VirtualMachinesCreatedCondition)).To(BeTrue())
				Expect(conditions.GetReason(afterFailure, vmopv1.VirtualMachinesCreatedCondition)).
					To(Equal(vmopv1.VirtualMachineCreationFailedReason))
				Expect(conditions.GetMessage(afterFailure, vmopv1.VirtualMachinesCreatedCondition)).
					To(ContainSubstring(createErr.Error()))
				Expect(afterFailure.Status.Replicas).To(Equal(int32(0)))

				failCreates = false
				// 2 calls: create the VMs, then let status catch up.
				_, err = reconcileOnce()
				Expect(err).ToNot(HaveOccurred())
				_, err = reconcileOnce()
				Expect(err).ToNot(HaveOccurred())

				healed := getRS()
				Expect(conditions.IsTrue(healed, vmopv1.VirtualMachinesCreatedCondition)).To(BeTrue())
				Expect(healed.Status.Replicas).To(Equal(int32(3)))
			})

			// SC26: sustained failure to converge surfaces ReplicaFailure,
			// which clears again once creation starts succeeding.
			It("sets a ReplicaFailure condition when replicas persistently fail to be created, clearing once healed", func() {
				createErr = errors.New("insufficient quota")
				failCreates = true

				reconcileN(1) // adds finalizer only
				_, _ = reconcileOnce()
				_, _ = reconcileOnce()

				failing := getRS()
				Expect(conditions.IsTrue(failing, vmopv1.VirtualMachineReplicaSetReplicaFailure)).To(BeTrue())
				Expect(conditions.GetReason(failing, vmopv1.VirtualMachineReplicaSetReplicaFailure)).
					To(Equal(vmopv1.VirtualMachineCreationFailedReason))

				failCreates = false
				_, err := reconcileOnce()
				Expect(err).ToNot(HaveOccurred())

				Expect(conditions.Has(getRS(), vmopv1.VirtualMachineReplicaSetReplicaFailure)).To(BeFalse())
			})

			// SC26 (delete-path variant): the same ReplicaFailure signal must
			// also fire when a scale-down's delete attempt fails (e.g. a
			// competing finalizer or API error), not just on the create path.
			It("sets a ReplicaFailure condition when a scale-down delete fails, clearing once healed", func() {
				reconcileN(3) // finalizer, create all 3, then let status catch up
				Expect(getRS().Status.Replicas).To(Equal(int32(3)))

				deleteErr = errors.New("delete blocked")
				failDeletes = true

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(2))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())

				_, err := reconcileOnce()
				Expect(err).To(HaveOccurred())

				failing := getRS()
				Expect(conditions.IsTrue(failing, vmopv1.VirtualMachineReplicaSetReplicaFailure)).To(BeTrue())
				Expect(conditions.GetReason(failing, vmopv1.VirtualMachineReplicaSetReplicaFailure)).
					To(Equal(vmopv1.VirtualMachineDeletionFailedReason))
				// The delete attempt failed, so all 3 VMs must still exist.
				Expect(failing.Status.Replicas).To(Equal(int32(3)))

				failDeletes = false
				reconcileN(2) // delete down to 2, then let status catch up.

				healed := getRS()
				Expect(conditions.Has(healed, vmopv1.VirtualMachineReplicaSetReplicaFailure)).To(BeFalse())
				Expect(healed.Status.Replicas).To(Equal(int32(2)))
			})

			// SC27: VirtualMachinesReady aggregates readiness. For
			// spec.replicas == 0, the TDS's `[NEEDS CLARIFICATION]` is
			// resolved as True/vacuously ready -- codified by the second It
			// below.
			It("marks VirtualMachinesReady True once readyReplicas matches replicas", func() {
				reconcileN(2) // finalizer, then create all 3
				markAllVMsReady()
				reconcileN(1) // let status catch up with the now-ready VMs

				updated := getRS()
				Expect(updated.Status.ReadyReplicas).To(Equal(updated.Status.Replicas))
				Expect(conditions.IsTrue(updated, vmopv1.VirtualMachinesReadyCondition)).To(BeTrue())
			})

			When("spec.replicas is 0", func() {
				BeforeEach(func() {
					rs.Spec.Replicas = ptr.To(int32(0))
				})

				It("marks VirtualMachinesReady True vacuously", func() {
					reconcileN(2)
					Expect(conditions.IsTrue(getRS(), vmopv1.VirtualMachinesReadyCondition)).To(BeTrue())
				})
			})

			// SC28: observedGeneration only advances once the generation's
			// spec has actually been processed. The fake client (unlike a
			// real API server) does not auto-increment metadata.generation
			// on spec edits, so this test bumps it explicitly to simulate
			// that API-server behavior.
			It("advances observedGeneration only after the new generation's spec is processed", func() {
				reconcileN(3)
				initialGeneration := getRS().Generation

				rs = getRS()
				rs.Generation = initialGeneration + 1
				rs.Spec.Replicas = ptr.To(int32(4))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())

				// 2 calls: create the extra VM, then let status catch up.
				reconcileN(2)
				updated := getRS()
				Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
				Expect(updated.Status.ObservedGeneration).To(BeNumerically(">", initialGeneration))
				Expect(updated.Status.Replicas).To(Equal(int32(4)))
			})

			// SC29: Resized condition clears once converged.
			It("marks Resized True once a scale operation has fully converged", func() {
				reconcileN(2) // finalizer, then create all 3
				markAllVMsReady()
				reconcileN(1) // let status catch up with the now-ready VMs

				converged := getRS()
				Expect(conditions.IsTrue(converged, vmopv1.ResizedCondition)).To(BeTrue())

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(5))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)
				scalingUp := getRS()
				Expect(conditions.GetReason(scalingUp, vmopv1.ResizedCondition)).To(Equal(vmopv1.ScalingUpReason))

				markAllVMsReady() // mark the 2 new replicas ready too
				reconcileN(1)
				Expect(conditions.IsTrue(getRS(), vmopv1.ResizedCondition)).To(BeTrue())
			})
		})

		Context("template, selector, and label semantics", func() {

			// SC14: changing the selector doesn't retroactively relabel or
			// delete existing VMs; VMs that fall out of the new selector are
			// left untouched, and new replicas are created to satisfy the
			// desired count under the new selector.
			It("does not relabel or delete existing VMs when the selector (and template) change", func() {
				reconcileN(2)
				original := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(original).To(HaveLen(3))
				originalLabels := make([]map[string]string, len(original))
				for i, vm := range original {
					originalLabels[i] = vm.Labels
				}

				rs = getRS()
				rs.Spec.Selector.MatchLabels = map[string]string{"appname": "dummy-b"}
				rs.Spec.Template.Labels = map[string]string{"appname": "dummy-b"}
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())

				reconcileN(1)

				staleVMs := listVMsByLabels(map[string]string{"appname": "dummy"})
				Expect(staleVMs).To(HaveLen(3), "old VMs must not be deleted")
				for _, vm := range staleVMs {
					Expect(vm.Labels).To(Equal(originalLabels[0]), "old VMs must not be relabeled")
				}

				newVMs := listVMsByLabels(map[string]string{"appname": "dummy-b"})
				Expect(newVMs).To(HaveLen(3))
			})

			// SC15: template.spec edits don't mutate/recreate existing
			// replicas; a subsequent scale-up uses the new template.
			It("does not mutate existing replicas when template.spec changes, but new replicas use it", func() {
				reconcileN(2)
				original := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(original).To(HaveLen(3))
				originalNames := vmNames(original)

				originalClassName := original[0].Spec.ClassName

				rs = getRS()
				rs.Spec.Template.Spec.ClassName = "dummy-class-v2"
				rs.Spec.Replicas = ptr.To(int32(4))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)

				allVMs := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(allVMs).To(HaveLen(4))

				for _, vm := range allVMs {
					if slices.Contains(originalNames, vm.Name) {
						Expect(vm.Spec.ClassName).To(Equal(originalClassName))
					} else {
						Expect(vm.Spec.ClassName).To(Equal("dummy-class-v2"))
					}
				}
			})

			// SC16 (NEEDS CLARIFICATION resolved: option (a), Pod ReplicaSet
			// parity -- see tds.md section 2.3 scenario 16): template
			// metadata-label edits do not propagate to existing replicas.
			It("does not propagate template metadata-label edits to existing replicas", func() {
				reconcileN(2)
				original := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(original).To(HaveLen(3))

				rs = getRS()
				rs.Spec.Template.Labels = map[string]string{"appname": "dummy", "tier": "web"}
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)

				for _, vm := range listVMsByLabels(rs.Spec.Selector.MatchLabels) {
					Expect(vm.Labels).ToNot(HaveKey("tier"))
				}
			})

			// SC30 & SC34.
			When("template.spec.powerState is PoweredOff", func() {
				BeforeEach(func() {
					rs.Spec.Template.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				})

				It("stamps the replicaset-name label and passes through template.spec verbatim", func() {
					reconcileN(3)

					vms := listVMsByLabels(rs.Spec.Selector.MatchLabels)
					Expect(vms).To(HaveLen(3))
					for _, vm := range vms {
						Expect(vm.Labels[vmopv1.VirtualMachineReplicaSetNameLabel]).To(Equal(rs.Name))
						Expect(vm.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					}

					byLabel := listVMsByLabels(map[string]string{vmopv1.VirtualMachineReplicaSetNameLabel: rs.Name})
					Expect(vmNames(byLabel)).To(ConsistOf(vmNames(vms)))
				})
			})

			// SC31: removing the replicaset-name label breaks label-based
			// discovery but not owner-reference-based ownership/counting.
			It("keeps status.replicas correct via owner references even if the replicaset-name label is removed", func() {
				reconcileN(3)
				vms := listVMsByLabels(rs.Spec.Selector.MatchLabels)
				Expect(vms).To(HaveLen(3))

				stripped := vms[0]
				delete(stripped.Labels, vmopv1.VirtualMachineReplicaSetNameLabel)
				Expect(ctx.Client.Update(ctx, &stripped)).To(Succeed())

				byLabel := listVMsByLabels(map[string]string{vmopv1.VirtualMachineReplicaSetNameLabel: rs.Name})
				Expect(byLabel).To(HaveLen(2), "label-based discovery is expected to miss the stripped VM")

				Expect(getRS().Status.Replicas).To(Equal(int32(3)),
					"status.replicas is driven by owner references via the selector, not the label")
			})
		})

		Context("non-goal absence checks", func() {

			// SC35: no rollout/revision-history-style fields exist.
			It("has no ControllerRevision-equivalent or rollout-status field", func() {
				fields := reflect.VisibleFields(reflect.TypeOf(vmopv1.VirtualMachineReplicaSetStatus{}))
				names := make([]string, 0, len(fields))
				for _, f := range fields {
					names = append(names, f.Name)
				}
				Expect(names).To(ConsistOf(
					"Replicas", "FullyLabeledReplicas", "ReadyReplicas", "ObservedGeneration", "Conditions"))
			})

			// SC36: status.replicas never changes except in response to an
			// external spec.replicas edit.
			It("never changes status.replicas without an external spec.replicas edit", func() {
				reconcileN(3)
				before := getRS().Status.Replicas

				for i := 0; i < 3; i++ {
					_, err := reconcileOnce()
					Expect(err).ToNot(HaveOccurred())
				}

				Expect(getRS().Status.Replicas).To(Equal(before))
			})
		})
	})
}

func intgTestsReconcile() {

	var (
		ctx *builder.IntegrationTestContext

		rs    *vmopv1.VirtualMachineReplicaSet
		rsKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		rs = &vmopv1.VirtualMachineReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-replicaset",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineReplicaSetSpec{
				Replicas: ptr.To(int32(2)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"appname": "db",
					},
				},
				Template: vmopv1.VirtualMachineTemplateSpec{
					ObjectMeta: vmopv1common.ObjectMeta{
						Labels: map[string]string{
							"appname": "db",
						},
						Annotations: make(map[string]string),
					},
					Spec: vmopv1.VirtualMachineSpec{
						ImageName:  "dummy-image",
						ClassName:  "dummy-class",
						PowerState: vmopv1.VirtualMachinePowerStateOn,
						Network: &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						},
					},
				},
			},
		}
		rsKey = types.NamespacedName{Name: rs.Name, Namespace: rs.Namespace}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getVirtualMachineReplicaSet := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachineReplicaSet {
		rs := &vmopv1.VirtualMachineReplicaSet{}
		if err := ctx.Client.Get(ctx, objKey, rs); err != nil {
			return nil
		}
		return rs
	}

	waitForReplicaSetFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		EventuallyWithOffset(1, func() []string {
			if rs := getVirtualMachineReplicaSet(ctx, objKey); rs != nil {
				return rs.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizerName), "waiting for VirtualMachineReplicaSet finalizer")
	}

	ensureReplicas := func(ctx *builder.IntegrationTestContext, labels map[string]string, desiredReplicas int) {
		Eventually(func(g Gomega) int {
			vmList := &vmopv1.VirtualMachineList{}
			err := ctx.Client.List(ctx, vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(labels))
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(vmList).ToNot(BeNil())
			return len(vmList.Items)
		}, 10*time.Second, 1*time.Second).Should(Equal(desiredReplicas))
	}

	// markVMsReady marks every VM matching labels Ready=True, simulating what
	// the (not-running-in-this-suite) VirtualMachine controller would
	// eventually do. Needed for any assertion depending on
	// status.readyReplicas/the Resized/VirtualMachinesReady conditions.
	markVMsReady := func(ctx *builder.IntegrationTestContext, labels map[string]string) {
		var vmList vmopv1.VirtualMachineList
		Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(labels))).To(Succeed())
		for i := range vmList.Items {
			vm := &vmList.Items[i]
			conditions.MarkTrue(vm, vmopv1.ReadyConditionType)
			Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())
		}
	}

	Context("Reconcile", func() {
		dummyInstanceUUID := "instanceUUID1234"

		BeforeEach(func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					// Used below just to check for something in the Status is
					// updated.
					vm.Status.InstanceUUID = dummyInstanceUUID
					return nil
				})
		})

		AfterEach(func() {
			By("Delete VirtualMachineReplicaSet", func() {
				if err := ctx.Client.Delete(ctx, rs); err == nil {
					rs := &vmopv1.VirtualMachineReplicaSet{}
					// If ReplicaSet is still around because of finalizer, try to cleanup for next test.
					if err := ctx.Client.Get(ctx, rsKey, rs); err == nil && len(rs.Finalizers) > 0 {
						rs.Finalizers = nil
						_ = ctx.Client.Update(ctx, rs)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})

			// Integration tests use env-test which only starts API server.
			// Since garbage collection is handled by kube-controller-manager, we
			// can't really test that all replicas have been garbage collected
			// once the replica set has been deleted.
		})

		It("Reconciles after VirtualMachineReplicaSet creation", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})
		})

		It("Scale up VirtualMachineReplicaSet", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("Modifying the spec.replicas", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					numReplicas := *rs.Spec.Replicas + 1
					rs.Spec.Replicas = &numReplicas
					return nil
				})
				Expect(err).ToNot(HaveOccurred())
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("The Resized condition converges to True once the scale-up settles (TDS SC6)", func() {
				markVMsReady(ctx, rs.Spec.Selector.MatchLabels)
				Eventually(func(g Gomega) bool {
					got := getVirtualMachineReplicaSet(ctx, rsKey)
					g.Expect(got).ToNot(BeNil())
					return conditions.IsTrue(got, vmopv1.ResizedCondition)
				}, 10*time.Second, 1*time.Second).Should(BeTrue())
			})
		})

		It("Scale down VirtualMachineReplicaSet", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())

			By("VirtualMachineReplicaSet should have finalizer added", func() {
				waitForReplicaSetFinalizer(ctx, rsKey)
			})

			By("Sufficient replicas must be created", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("Modifying the spec.replicas", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					numReplicas := *rs.Spec.Replicas - 1
					rs.Spec.Replicas = &numReplicas
					return nil
				})
				Expect(err).ToNot(HaveOccurred())

				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))
			})

			By("The Resized condition converges to True once the scale-down settles (TDS SC7)", func() {
				markVMsReady(ctx, rs.Spec.Selector.MatchLabels)
				Eventually(func(g Gomega) bool {
					got := getVirtualMachineReplicaSet(ctx, rsKey)
					g.Expect(got).ToNot(BeNil())
					return conditions.IsTrue(got, vmopv1.ResizedCondition)
				}, 10*time.Second, 1*time.Second).Should(BeTrue())
			})
		})

		It("Scaling down with deletePolicy Random deletes exactly one owned VM and leaves the rest untouched", func() {
			rs.Spec.Replicas = ptr.To(int32(5))
			rs.Spec.DeletePolicy = "Random"

			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 5)

			var before vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &before, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(before.Items).To(HaveLen(5))
			ownedUIDs := make(map[types.UID]struct{}, len(before.Items))
			for _, vm := range before.Items {
				ownedUIDs[vm.UID] = struct{}{}
			}

			By("Scaling down by one", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					n := *rs.Spec.Replicas - 1
					rs.Spec.Replicas = &n
					return nil
				})
				Expect(err).ToNot(HaveOccurred())
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 4)
			})

			// The implementation is free to pick any replica when deletePolicy is
			// Random, so only the invariants that must hold regardless of which
			// one was chosen are asserted here (TDS SC8): exactly one fewer VM,
			// and every remaining VM was actually owned by this ReplicaSet before
			// the scale-down (never mutated/recreated).
			var after vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &after, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(after.Items).To(HaveLen(4))
			for _, vm := range after.Items {
				Expect(ownedUIDs).To(HaveKey(vm.UID), "remaining VM %q was not one of the originally owned replicas", vm.Name)
			}
		})

		It("Scaling via the /scale subresource has the same effect as editing spec.replicas directly", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))

			By("Updating the /scale subresource to 4 replicas", func() {
				scale := &autoscalingv1.Scale{
					ObjectMeta: metav1.ObjectMeta{Name: rs.Name, Namespace: rs.Namespace},
					Spec:       autoscalingv1.ScaleSpec{Replicas: 4},
				}
				Expect(ctx.Client.SubResource("scale").Update(ctx, rs, client.WithSubResourceBody(scale))).To(Succeed())
			})

			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 4)

			Eventually(func(g Gomega) int32 {
				got := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got).ToNot(BeNil())
				return *got.Spec.Replicas
			}, 10*time.Second, 1*time.Second).Should(Equal(int32(4)),
				"spec.replicas should reflect the /scale subresource edit")

			Eventually(func(g Gomega) int32 {
				got := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got).ToNot(BeNil())
				return got.Status.Replicas
			}, 10*time.Second, 1*time.Second).Should(Equal(int32(4)),
				"status.replicas (the scale subresource's status path) should converge same as a direct spec.replicas edit")
		})

		It("Reconciles after VirtualMachineReplicaSet deletion", func() {
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			// Wait for initial reconcile.
			waitForReplicaSetFinalizer(ctx, rsKey)

			Expect(ctx.Client.Delete(ctx, rs)).To(Succeed())
			By("Finalizer should be removed after deletion", func() {
				Eventually(func() []string {
					if vm := getVirtualMachineReplicaSet(ctx, rsKey); vm != nil {
						return vm.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizerName))
			})
		})
	})

	Context("Ownership, orphans, and readiness", func() {
		dummyInstanceUUID := "instanceUUID1234"

		BeforeEach(func() {
			providerfake.SetCreateOrUpdateFunction(
				ctx,
				intgFakeVMProvider,
				func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
					vm.Status.InstanceUUID = dummyInstanceUUID
					return nil
				})
		})

		AfterEach(func() {
			By("Delete VirtualMachineReplicaSet", func() {
				if err := ctx.Client.Delete(ctx, rs); err == nil {
					rs := &vmopv1.VirtualMachineReplicaSet{}
					if err := ctx.Client.Get(ctx, rsKey, rs); err == nil && len(rs.Finalizers) > 0 {
						rs.Finalizers = nil
						_ = ctx.Client.Update(ctx, rs)
					}
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			})
		})

		It("adopts a pre-existing standalone VirtualMachine matching the selector", func() {
			// TDS SC19, revised: this matches vanilla Kubernetes ReplicaSet
			// adoption parity intentionally -- a silent owner-ref claim plus
			// a transient SuccessfulAdopt Event, with no persistent
			// Condition (upstream ReplicaSetStatus only ever has
			// ReplicaFailure). A persistent "Adopted" Condition would be a
			// deliberate deviation from that parity and isn't part of the
			// approved design (see the WCP one-pager, which frames adoption
			// itself as future work gated on relocation support, let alone a
			// Condition for it) -- so this test only asserts adoption
			// occurs, not a Condition recording it.
			standalone := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-vm",
					Namespace: ctx.Namespace,
					Labels:    rs.Spec.Selector.MatchLabels,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:  "dummy-image",
					ClassName:  "dummy-class",
					PowerState: vmopv1.VirtualMachinePowerStateOn,
				},
			}
			Expect(ctx.Client.Create(ctx, standalone)).To(Succeed())

			rs.Spec.Replicas = ptr.To(int32(1))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)

			By("The standalone VirtualMachine is adopted rather than duplicated", func() {
				Eventually(func(g Gomega) []metav1.OwnerReference {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: standalone.Name, Namespace: ctx.Namespace}, vm)).To(Succeed())
					return vm.OwnerReferences
				}, 10*time.Second, 1*time.Second).Should(ContainElement(
					HaveField("Name", rs.Name),
				), "expected the standalone VM to be adopted (controller owner reference set)")

				var vmList vmopv1.VirtualMachineList
				Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
				Expect(vmList.Items).To(HaveLen(1), "the adopted VM alone should satisfy spec.replicas=1, no redundant VM created")
			})
		})

		It("creates exactly one replacement, with a fresh identity, when an owned VM is deleted directly", func() {
			// TDS SC18 (replacement), SC33 (deletion by something other than the
			// controller is treated identically), and SC38 (the replacement
			// never reuses the deleted replica's name/UID).
			rs.Spec.Replicas = ptr.To(int32(3))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 3)

			// Wait for status.replicas to also converge, not just the raw VM
			// count: the reconcile that creates these VMs polls to confirm
			// each one is gettable *after* issuing the Create calls, so
			// deleting one before that reconcile settles would race with its
			// own confirmation poll and stall it for the full timeout.
			Eventually(func(g Gomega) int32 {
				got := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got).ToNot(BeNil())
				return got.Status.Replicas
			}, 10*time.Second, 1*time.Second).Should(Equal(int32(3)))

			var before vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &before, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(before.Items).To(HaveLen(3))
			victim := before.Items[0]

			By("Deleting one owned VM directly, bypassing the ReplicaSet", func() {
				Expect(ctx.Client.Delete(ctx, &victim)).To(Succeed())
			})

			By("Exactly one new VirtualMachine replaces it", func() {
				Eventually(func(g Gomega) {
					var after vmopv1.VirtualMachineList
					g.Expect(ctx.Client.List(ctx, &after, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
					g.Expect(after.Items).To(HaveLen(3))
					for _, vm := range after.Items {
						g.Expect(vm.UID).ToNot(Equal(victim.UID), "the replacement must never reuse the deleted replica's identity")
						g.Expect(vm.Name).ToNot(Equal(victim.Name), "the replacement must have a freshly generated name")
					}
				}, 10*time.Second, 1*time.Second).Should(Succeed())
			})
		})

		It("never lets two ReplicaSets with overlapping selectors cross-delete each other's owned VMs", func() {
			// TDS SC20. This is a user-error scenario: the system must not
			// corrupt state, and each ReplicaSet may only create/delete VMs it
			// actually owns (via owner reference), never one owned by the
			// other ReplicaSet sharing the same selector.
			rs.Spec.Replicas = ptr.To(int32(2))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 2)

			rs2 := rs.DeepCopy()
			rs2.ObjectMeta = metav1.ObjectMeta{
				Name:      "dummy-replicaset-2",
				Namespace: ctx.Namespace,
			}
			rs2Key := types.NamespacedName{Name: rs2.Name, Namespace: rs2.Namespace}
			Expect(ctx.Client.Create(ctx, rs2)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rs2Key)
			defer func() {
				if err := ctx.Client.Delete(ctx, rs2); err == nil {
					got := &vmopv1.VirtualMachineReplicaSet{}
					if err := ctx.Client.Get(ctx, rs2Key, got); err == nil && len(got.Finalizers) > 0 {
						got.Finalizers = nil
						_ = ctx.Client.Update(ctx, got)
					}
				}
			}()

			// Both ReplicaSets converge to their own desired replica count
			// (each owning a disjoint set of VMs) rather than fighting over,
			// or cross-deleting, VMs owned by the other.
			Eventually(func(g Gomega) {
				got1 := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got1).ToNot(BeNil())
				g.Expect(got1.Status.Replicas).To(Equal(int32(2)))

				got2 := &vmopv1.VirtualMachineReplicaSet{}
				g.Expect(ctx.Client.Get(ctx, rs2Key, got2)).To(Succeed())
				g.Expect(got2.Status.Replicas).To(Equal(int32(2)))
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			var vmList vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			for _, vm := range vmList.Items {
				owner := metav1.GetControllerOfNoCopy(&vm)
				Expect(owner).ToNot(BeNil())
				Expect(owner.Name).To(Or(Equal(rs.Name), Equal(rs2.Name)),
					"every VM matching the shared selector must be owned by exactly one of the two ReplicaSets")
			}
		})

		It("never adopts, mutates, or deletes a VirtualMachine already controlled by an unrelated, non-ReplicaSet owner", func() {
			// Cross-cutting invariant (TDS section 3): "No VM outside the
			// owned set is ever mutated or deleted by a
			// VirtualMachineReplicaSet reconcile, full stop." SC20 only
			// proves this against a same-kind foreign owner (another
			// ReplicaSet); this generalizes it to an owner of a completely
			// different kind, which exercises the same
			// metav1.IsControlledBy skip-path in ReconcileNormal from a
			// different angle.
			foreign := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foreign-owned-vm",
					Namespace: ctx.Namespace,
					Labels:    rs.Spec.Selector.MatchLabels,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "unrelated.example.com/v1",
							Kind:       "SomeOtherController",
							Name:       "unrelated-owner",
							UID:        types.UID("11111111-1111-1111-1111-111111111111"),
							Controller: ptr.To(true),
						},
					},
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:  "dummy-image",
					ClassName:  "dummy-class",
					PowerState: vmopv1.VirtualMachinePowerStateOn,
				},
			}
			Expect(ctx.Client.Create(ctx, foreign)).To(Succeed())
			foreignResourceVersion := foreign.ResourceVersion

			rs.Spec.Replicas = ptr.To(int32(2))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)

			// The ReplicaSet must not get stuck below its desired count just
			// because a foreign-owned VM happens to match its selector: it
			// creates its own 2 replicas rather than counting or adopting
			// the foreign one.
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 3)

			Consistently(func(g Gomega) *vmopv1.VirtualMachine {
				vm := &vmopv1.VirtualMachine{}
				g.Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: foreign.Name, Namespace: ctx.Namespace}, vm)).To(Succeed())
				return vm
			}, 3*time.Second, 500*time.Millisecond).Should(SatisfyAll(
				HaveField("ResourceVersion", foreignResourceVersion),
				HaveField("OwnerReferences", ConsistOf(foreign.OwnerReferences)),
			), "the foreign-owned VM must never be mutated (relabeled, adopted, or otherwise touched)")

			got := getVirtualMachineReplicaSet(ctx, rsKey)
			Expect(got).ToNot(BeNil())
			Expect(got.Status.Replicas).To(Equal(int32(2)),
				"the foreign-owned VM must never count toward this ReplicaSet's status.replicas")
		})

		It("never considers, adopts, or deletes a VirtualMachine in a different namespace", func() {
			// TDS SC21. Selector matching is namespace-scoped, consistent with
			// both VirtualMachine and VirtualMachineReplicaSet being namespaced.
			otherNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "rs-sc21-" + rs.Namespace},
			}
			Expect(ctx.Client.Create(ctx, otherNS)).To(Succeed())

			foreign := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foreign-vm",
					Namespace: otherNS.Name,
					Labels:    rs.Spec.Selector.MatchLabels,
				},
				Spec: vmopv1.VirtualMachineSpec{
					ImageName:  "dummy-image",
					ClassName:  "dummy-class",
					PowerState: vmopv1.VirtualMachinePowerStateOn,
				},
			}
			Expect(ctx.Client.Create(ctx, foreign)).To(Succeed())

			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, int(*rs.Spec.Replicas))

			By("The foreign-namespace VM is never adopted", func() {
				Consistently(func(g Gomega) []metav1.OwnerReference {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: foreign.Name, Namespace: otherNS.Name}, vm)).To(Succeed())
					return vm.OwnerReferences
				}, 3*time.Second, 500*time.Millisecond).Should(BeEmpty())
			})

			Expect(ctx.Client.Delete(ctx, otherNS)).To(Succeed())
		})

		It("marks status.readyReplicas from each owned VM's Ready condition, not a blanket count", func() {
			// TDS SC24. An explicitly set Ready condition (True or False) is
			// always authoritative, regardless of whether the VM has a
			// readiness probe configured.
			rs.Spec.Replicas = ptr.To(int32(2))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 2)

			var vmList vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(vmList.Items).To(HaveLen(2))

			By("Marking only one of the two owned VMs Ready", func() {
				ready := &vmList.Items[0]
				conditions.MarkTrue(ready, vmopv1.ReadyConditionType)
				Expect(ctx.Client.Status().Update(ctx, ready)).To(Succeed())

				notReady := &vmList.Items[1]
				conditions.MarkFalse(notReady, vmopv1.ReadyConditionType, "NotReady", "not ready")
				Expect(ctx.Client.Status().Update(ctx, notReady)).To(Succeed())
			})

			Eventually(func(g Gomega) int32 {
				got := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got).ToNot(BeNil())
				return got.Status.ReadyReplicas
			}, 10*time.Second, 1*time.Second).Should(Equal(int32(1)),
				"status.readyReplicas should reflect only the VM whose Ready condition is actually true")
		})

		It("counts a VM without a configured readiness probe as ready, but not one with a probe and no Ready condition yet", func() {
			// TDS SC24: the Ready condition is only ever populated by the
			// readiness prober, and the prober only watches VMs with a
			// configured spec.readinessProbe. A VM without one never gets a
			// Ready condition, so it must be treated as implicitly ready;
			// otherwise readyReplicas could never converge for any
			// VirtualMachineReplicaSet whose template omits readinessProbe.
			rs.Spec.Replicas = ptr.To(int32(2))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 2)

			var vmList vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(vmList.Items).To(HaveLen(2))

			By("Giving one of the two VMs a readiness probe, and leaving the other without one", func() {
				probed := &vmList.Items[0]
				probed.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
					GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
				}
				Expect(ctx.Client.Update(ctx, probed)).To(Succeed())
			})

			Eventually(func(g Gomega) int32 {
				got := getVirtualMachineReplicaSet(ctx, rsKey)
				g.Expect(got).ToNot(BeNil())
				return got.Status.ReadyReplicas
			}, 10*time.Second, 1*time.Second).Should(Equal(int32(1)),
				"only the VM without a readiness probe should count as ready; the probed VM has no Ready condition yet")
		})

		It("does not overshoot spec.replicas while a scaled-down replica's finalizer drains", func() {
			// TDS SC32. A finalizer on the replica chosen for deletion must not
			// cause the controller to prematurely create a replacement while
			// that replica is still terminating.
			const testFinalizer = "test.vmoperator.vmware.com/block-delete"

			rs.Spec.Replicas = ptr.To(int32(2))
			Expect(ctx.Client.Create(ctx, rs)).To(Succeed())
			waitForReplicaSetFinalizer(ctx, rsKey)
			ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 2)

			var vmList vmopv1.VirtualMachineList
			Expect(ctx.Client.List(ctx, &vmList, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
			Expect(vmList.Items).To(HaveLen(2))

			// The random delete-priority tie-break sorts by ascending name, so
			// the lexicographically-smallest name is the one chosen for
			// deletion first; block that one's removal with a finalizer.
			victim := &vmList.Items[0]
			survivor := &vmList.Items[1]
			if survivor.Name < victim.Name {
				victim, survivor = survivor, victim
			}

			By("Adding a finalizer to the VM that will be chosen for deletion", func() {
				controllerutil.AddFinalizer(victim, testFinalizer)
				Expect(ctx.Client.Update(ctx, victim)).To(Succeed())
			})

			By("Scaling down by one", func() {
				_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, rs, func() error {
					n := *rs.Spec.Replicas - 1
					rs.Spec.Replicas = &n
					return nil
				})
				Expect(err).ToNot(HaveOccurred())
			})

			By("The victim gets a deletion timestamp but is retained by its finalizer", func() {
				Eventually(func(g Gomega) *metav1.Time {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: victim.Name, Namespace: ctx.Namespace}, vm)).To(Succeed())
					return vm.DeletionTimestamp
				}, 10*time.Second, 1*time.Second).ShouldNot(BeNil())
			})

			By("No premature replacement is created while the victim drains", func() {
				Consistently(func(g Gomega) int {
					var all vmopv1.VirtualMachineList
					g.Expect(ctx.Client.List(ctx, &all, client.InNamespace(ctx.Namespace), client.MatchingLabels(rs.Spec.Selector.MatchLabels))).To(Succeed())
					return len(all.Items)
				}, 3*time.Second, 500*time.Millisecond).Should(Equal(2),
					"the controller must not overshoot by creating a replacement before the draining VM is actually gone")
			})

			By("Removing the finalizer lets the victim actually terminate", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: victim.Name, Namespace: ctx.Namespace}, vm)).To(Succeed())
				controllerutil.RemoveFinalizer(vm, testFinalizer)
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			By("The ReplicaSet converges back to spec.replicas with the survivor untouched", func() {
				ensureReplicas(ctx, rs.Spec.Selector.MatchLabels, 1)

				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, types.NamespacedName{Name: survivor.Name, Namespace: ctx.Namespace}, vm)).To(Succeed())
				Expect(vm.UID).To(Equal(survivor.UID))
			})
		})
	})
}
