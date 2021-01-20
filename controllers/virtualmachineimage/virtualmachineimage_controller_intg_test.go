// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking VirtualMachineImage controller tests", virtualMachineImageReconcile)
}

func virtualMachineImageReconcile() {
	var (
		ctx     *builder.IntegrationTestContext
		vmImage *vmopv1alpha1.VirtualMachineImage
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vmImage = &vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-image",
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {

		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmImage)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmImage)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("resource successfully created", func() {
			// Tested in BeforeEach/AfterEach
		})
	})
}
