// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/vmware-tanzu/vm-operator/controllers/capability"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.V1Alpha3,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("WcpClusterCapabilitiesConfigMap", func() {
		var obj *corev1.ConfigMap

		BeforeEach(func() {
			var err error
			obj, err = capability.NewWcpClusterCapabilitiesConfigMap(
				map[string]string{
					capability.TKGMultipleCLCapabilityKey: "false",
				})
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).NotTo(BeNil())
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, obj)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		When("updated", func() {
			JustBeforeEach(func() {
				data := map[string]string{
					capability.TKGMultipleCLCapabilityKey: "true",
				}
				obj.Data = data
				Expect(ctx.Client.Update(ctx, obj)).To(Succeed())
			})

			It("should be reconciled", func() {
				Eventually(func() bool {
					return pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL
				}).Should(BeTrue())
			})
		})
	})
}
