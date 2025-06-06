// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
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
				Namespace: "default",
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

		It("Should create VirtualMachineClassInstance", func() {
			Eventually(func(g Gomega) {
				var list vmopv1.VirtualMachineClassInstanceList
				err := ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(list.Items).To(HaveLen(1))

				instance := list.Items[0]

				g.Expect(instance.Annotations).ToNot(BeEmpty())
				g.Expect(instance.Annotations[pkgconst.VirtualMachineClassHashAnnotationKey]).ToNot(BeEmpty())

				g.Expect(instance.Labels).ToNot(BeEmpty())
				g.Expect(instance.Labels[pkgconst.VirtualMachineClassNameLabelKey]).To(Equal(vmClass.Name))
				g.Expect(instance.Labels[pkgconst.VirtualMachineClassStateLabelKey]).To(Equal(pkgconst.VirtualMachineClassStateActive))
			}).Should(Succeed())
		})

		It("Should create new VirtualMachineClassInstance when config changes", func() {
			vmClass.Spec.Hardware.Cpus++
			setConfigSpec(vmClass)

			err := ctx.Client.Update(ctx, vmClass)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				var list vmopv1.VirtualMachineClassInstanceList
				err := ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(list.Items).To(HaveLen(2))
			}).Should(Succeed())

			vmClass.Spec.Hardware.Cpus++
			setConfigSpec(vmClass)

			err = ctx.Client.Update(ctx, vmClass)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				var list vmopv1.VirtualMachineClassInstanceList
				err := ctx.Client.List(ctx, &list, client.InNamespace(vmClass.Namespace))
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(list.Items).To(HaveLen(3))

				state := map[string]int{
					pkgconst.VirtualMachineClassStateActive:   0,
					pkgconst.VirtualMachineClassStateInactive: 0,
				}

				for _, item := range list.Items {
					val := item.Labels[pkgconst.VirtualMachineClassStateLabelKey]

					g.Expect(val).ToNot(BeEmpty())

					state[val]++
				}

				g.Expect(state).To(HaveLen(2))
				g.Expect(state[pkgconst.VirtualMachineClassStateActive]).To(Equal(1))
				g.Expect(state[pkgconst.VirtualMachineClassStateInactive]).To(Equal(2))
			}).Should(Succeed())
		})
	})
}
