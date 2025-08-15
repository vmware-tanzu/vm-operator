// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler *virtualmachineclass.Reconciler
		vmClass    *vmopv1.VirtualMachineClass
		vmClassCtx *pkgctx.VirtualMachineClassContext
	)

	BeforeEach(func() {
		vmClass = &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "dummy-vmclass",
				Namespace:   "dummy-ns",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: vmopv1.VirtualMachineClassSpec{
				ReservedProfileID: "dummy-profile",
			},
		}
		setConfigSpec(vmClass)

		vmClassCtx = &pkgctx.VirtualMachineClassContext{
			Context: pkgcfg.NewContextWithDefaultConfig(),
			Logger:  suite.GetLogger().WithValues("vmClassName", vmClass.Name),
			VMClass: vmClass,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachineclass.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmClassCtx = nil
		reconciler = nil
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, vmClass)
		})

		Context("when ImmutableClasses feature is disabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmClassCtx, func(config *pkgcfg.Config) {
					config.Features.ImmutableClasses = false
				})
			})

			It("should succeed without creating VirtualMachineClassInstances", func() {
				Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())

				// Verify no VirtualMachineClassInstances were created
				list := &vmopv1.VirtualMachineClassInstanceList{}
				Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())
				Expect(list.Items).To(HaveLen(0))
			})
		})

		Context("when ImmutableClasses feature is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmClassCtx, func(config *pkgcfg.Config) {
					config.Features.ImmutableClasses = true
				})
			})

			It("should succeed and call reconcileInstance", func() {
				Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())

				// Verify a VirtualMachineClassInstance was created
				list := &vmopv1.VirtualMachineClassInstanceList{}
				Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())
				Expect(list.Items).To(HaveLen(1))

				instance := list.Items[0]
				// Verify annotation and label on the object
				Expect(instance.Annotations).To(HaveKey(pkgconst.VirtualMachineClassHashAnnotationKey))
				Expect(instance.Labels).To(HaveKey(vmopv1.VMClassInstanceActiveLabelKey))
			})

			Context("when called multiple times", func() {
				It("should be idempotent", func() {
					// First reconcile
					Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())
					// Verify a VirtualMachineClassInstance was created
					list := &vmopv1.VirtualMachineClassInstanceList{}
					Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())
					Expect(list.Items).To(HaveLen(1))

					// Second reconcile should also succeed
					Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())
					// No new instances should be created.
					list = &vmopv1.VirtualMachineClassInstanceList{}
					Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())
					Expect(list.Items).To(HaveLen(1))
				})
			})

			Context("when VirtualMachineClass config changes", func() {
				It("should succeed with different config", func() {
					// First reconcile with original config
					Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())

					// Verify a VirtualMachineClassInstance was created
					list := &vmopv1.VirtualMachineClassInstanceList{}
					Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())
					Expect(list.Items).To(HaveLen(1))

					// Verify the instance has the correct annotations and labels
					instance := list.Items[0]
					Expect(instance.Annotations).To(HaveKey(pkgconst.VirtualMachineClassHashAnnotationKey))
					Expect(instance.Labels).To(HaveKey(vmopv1.VMClassInstanceActiveLabelKey))

					// Modify the VM class config
					vmClass.Spec.Hardware.Cpus = 8
					setConfigSpec(vmClass)
					Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())

					// Pass in the new class to the context during reconcile
					vmClassCtx.VMClass = vmClass

					// Second reconcile with new config should succeed
					Expect(reconciler.ReconcileNormal(vmClassCtx)).To(Succeed())

					// Verify that there's only one active instance.
					// Since the inactive instances may still be
					// around if there are VMs referencing it, only
					// validate that there's on active instance.
					list = &vmopv1.VirtualMachineClassInstanceList{}
					Expect(ctx.Client.List(ctx, list, client.InNamespace(vmClassCtx.VMClass.Namespace))).To(Succeed())

					active := 0
					for _, instance := range list.Items {
						if _, ok := instance.Labels[vmopv1.VMClassInstanceActiveLabelKey]; ok {
							active++
						}
					}
					Expect(active).To(Equal(1))
				})
			})

			Context("when VirtualMachineClass has invalid config", func() {
				BeforeEach(func() {
					vmClass.Spec.ConfigSpec = nil
				})

				It("should return an error in generating the hash", func() {
					// In unit tests, we primarily test that the method doesn't panic
					// Error handling validation is better tested in integration tests
					Expect(reconciler.ReconcileNormal(vmClassCtx)).NotTo(Succeed())
				})
			})
		})
	})
}
