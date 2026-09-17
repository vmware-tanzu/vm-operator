// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset_test

import (
	"context"
	"errors"
	"reflect"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinereplicaset"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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

const unitTestNamespace = "dummy-ns"

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		interceptFn interceptor.Funcs

		// failCreates and createErr are read by the Create interceptor
		// registered below; setting them from within an It (after
		// JustBeforeEach has already run) still takes effect because the
		// interceptor closure reads them at call time, not registration
		// time.
		failCreates bool
		createErr   error

		ctx        *builder.UnitTestContextForController
		reconciler *virtualmachinereplicaset.Reconciler

		rs    *vmopv1.VirtualMachineReplicaSet
		rsKey types.NamespacedName
	)

	BeforeEach(func() {
		initObjects = nil
		failCreates = false
		createErr = nil
		interceptFn = interceptor.Funcs{
			Create: func(
				ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*vmopv1.VirtualMachine); ok && failCreates {
					return createErr
				}
				return c.Create(ctx, obj, opts...)
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

			// SC27: VirtualMachinesReady aggregates readiness. For
			// spec.replicas == 0, the TDS's `[NEEDS CLARIFICATION]` is
			// resolved as True/vacuously ready -- codified by the second It
			// below.
			It("marks VirtualMachinesReady True once readyReplicas matches replicas", func() {
				reconcileN(3)
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
				reconcileN(3)
				converged := getRS()
				Expect(conditions.IsTrue(converged, vmopv1.ResizedCondition)).To(BeTrue())

				rs = getRS()
				rs.Spec.Replicas = ptr.To(int32(5))
				Expect(ctx.Client.Update(ctx, rs)).To(Succeed())
				reconcileN(1)
				scalingUp := getRS()
				Expect(conditions.GetReason(scalingUp, vmopv1.ResizedCondition)).To(Equal(vmopv1.ScalingUpReason))

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
