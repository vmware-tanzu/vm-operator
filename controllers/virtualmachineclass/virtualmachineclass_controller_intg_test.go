// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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
		ctx     *builder.IntegrationTestContext
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vmClass = &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "small",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineClassSpec{
				Hardware: vmopv1.VirtualMachineClassHardware{
					Cpus:   4,
					Memory: resource.MustParse("1Mi"),
				},
				Policies: vmopv1.VirtualMachineClassPolicies{
					Resources: vmopv1.VirtualMachineClassResources{
						Requests: vmopv1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("1000Mi"),
							Memory: resource.MustParse("100Mi"),
						},
						Limits: vmopv1.VirtualMachineResourceSpec{
							Cpu:    resource.MustParse("2000Mi"),
							Memory: resource.MustParse("200Mi"),
						},
					},
				},
			},
		}

		setConfigSpec(vmClass)

		pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
			config.Features.ImmutableClasses = true
		})
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmClass)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("Should reconcile VirtualMachineClassInstance", func() {
			By("validating that the instance was created")
			Eventually(func(g Gomega) {
				var list vmopv1.VirtualMachineClassInstanceList
				g.Expect(ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))

				instance := list.Items[0]

				g.Expect(instance.Annotations).ToNot(BeEmpty())
				g.Expect(instance.Annotations[pkgconst.VirtualMachineClassHashAnnotationKey]).ToNot(BeEmpty())

				g.Expect(instance.Labels).ToNot(BeEmpty())
				g.Expect(instance.Labels[vmopv1.VMClassInstanceActiveLabelKey]).To(Equal(""))

				// Ensure owner reference is set
				g.Expect(instance.OwnerReferences).To(HaveLen(1))
				g.Expect(instance.OwnerReferences[0].Name).To(Equal(vmClass.Name))
			}).Should(Succeed())
		})

		When("class config is updated", func() {
			It("Should create new VirtualMachineClassInstance", func() {
				Eventually(func(g Gomega) {
					var list vmopv1.VirtualMachineClassInstanceList
					g.Expect(ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))).To(Succeed())
					g.Expect(list.Items).To(HaveLen(1))
				}).Should(Succeed())

				By("Modify the class config")
				vmClass.Spec.Hardware.Cpus++
				setConfigSpec(vmClass)

				Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())

				By("Validating that only one instance is active")
				Eventually(func(g Gomega) {
					var list vmopv1.VirtualMachineClassInstanceList
					g.Expect(ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))).To(Succeed())
					g.Expect(list.Items).To(HaveLen(2))

					activeCount := 0
					inactiveCount := 0
					for _, item := range list.Items {
						if _, hasActiveLabel := item.Labels[vmopv1.VMClassInstanceActiveLabelKey]; hasActiveLabel {
							activeCount++
						} else {
							inactiveCount++
						}
					}

					g.Expect(activeCount).To(Equal(1))
					g.Expect(inactiveCount).To(Equal(1))
				}).Should(Succeed())
			})
		})
	})
}
