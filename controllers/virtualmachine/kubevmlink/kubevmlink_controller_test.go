// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kubevmlink_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	func() {
		var (
			ctx        *builder.IntegrationTestContext
			vmKey      types.NamespacedName
			genericKey types.NamespacedName
			vm         *vmopv1.VirtualMachine
			generic    *kubevmv1a1.VirtualMachine
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()

			vmKey = types.NamespacedName{
				Name:      "vm-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}
			genericKey = types.NamespacedName{
				Name:      "generic-vm-" + uuid.NewString(),
				Namespace: ctx.Namespace,
			}

			generic = &kubevmv1a1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      genericKey.Name,
					Namespace: genericKey.Namespace,
				},
				Spec: kubevmv1a1.VirtualMachineSpec{
					InfrastructureRef: kubevmv1a1.ObjectReference{
						APIGroup: vmopv1.GroupName,
						Kind:     "VirtualMachine",
						Name:     vmKey.Name,
					},
					PowerState: kubevmv1a1.PowerStateOn,
				},
			}

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmKey.Name,
					Namespace: vmKey.Namespace,
					Annotations: map[string]string{
						kubevm.AnnotationKey: genericKey.Name,
					},
				},
			}
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		When("the feature gate is off", func() {
			BeforeEach(func() {
				pkgcfg.UpdateContext(suite, func(config *pkgcfg.Config) {
					config.Features.KubeVMProvider = false
				})
			})

			AfterEach(func() {
				pkgcfg.UpdateContext(suite, func(config *pkgcfg.Config) {
					config.Features.KubeVMProvider = true
				})
			})

			It("does nothing, even with the annotation and a mutually linked generic object present", func() {
				Expect(ctx.Client.Create(ctx, generic)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Consistently(func(g Gomega) {
					obj := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					g.Expect(obj.Spec.PowerState).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the reference is one-sided: no generic object names this VM back", func() {
			It("does not reassert the power state", func() {
				generic.Spec.InfrastructureRef.Name = "some-other-vm"
				Expect(ctx.Client.Create(ctx, generic)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

				Consistently(func(g Gomega) {
					obj := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					g.Expect(obj.Spec.PowerState).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the objects mutually name each other", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, generic)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			})

			It("reasserts the desired power state from the generic object", func() {
				Eventually(func(g Gomega) {
					obj := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					g.Expect(obj.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				}).Should(Succeed())
			})

			It("reverts a hand-patched power state on the provider object", func() {
				Eventually(func(g Gomega) {
					obj := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					g.Expect(obj.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				}).Should(Succeed())

				obj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
				obj.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Update(ctx, obj)).To(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					g.Expect(obj.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				}).Should(Succeed())
			})

			When("the generic object's instance type no longer matches the persisted value", func() {
				BeforeEach(func() {
					vmObj := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, vmKey, vmObj)).To(Succeed())
					vmObj.Spec.ClassName = "best-effort-2xlarge"
					Expect(ctx.Client.Update(ctx, vmObj)).To(Succeed())

					genericObj := &kubevmv1a1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, genericKey, genericObj)).To(Succeed())
					genericObj.Spec.InstanceType = &kubevmv1a1.InstanceTypeSpec{Name: "best-effort-4xlarge"}
					Expect(ctx.Client.Update(ctx, genericObj)).To(Succeed())
				})

				It("sets UpToDate=False with reason UnsupportedByProvider", func() {
					Eventually(func(g Gomega) {
						obj := &vmopv1.VirtualMachine{}
						g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
						c := conditions.Get(obj, vmopv1.VirtualMachineConditionUpToDate)
						g.Expect(c).ToNot(BeNil())
						g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
						g.Expect(c.Reason).To(Equal("UnsupportedByProvider"))
					}).Should(Succeed())
				})
			})
		})

		When("the VM is both a group member and kubevm-owned", func() {
			BeforeEach(func() {
				vm.Spec.GroupName = "some-group"
				Expect(ctx.Client.Create(ctx, generic)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			})

			It("refuses to engage and reports it", func() {
				Eventually(func(g Gomega) {
					obj := &vmopv1.VirtualMachine{}
					g.Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
					c := conditions.Get(obj, vmopv1.VirtualMachineConditionUpToDate)
					g.Expect(c).ToNot(BeNil())
					g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(c.Reason).To(Equal("GroupMemberConflict"))
				}).Should(Succeed())

				obj := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, vmKey, obj)).To(Succeed())
				Expect(obj.Spec.PowerState).To(BeEmpty())
			})
		})
	},
)
