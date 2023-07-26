// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	var (
		ctx     *builder.IntegrationTestContext
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vmClass = &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "small",
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
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("NoOp", func() {
		})
	})
}
