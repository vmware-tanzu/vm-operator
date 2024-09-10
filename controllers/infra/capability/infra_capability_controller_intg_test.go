// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
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
		var configMap *corev1.ConfigMap

		BeforeEach(func() {
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      capabilities.WCPClusterCapabilitiesConfigMapName,
					Namespace: capabilities.WCPClusterCapabilitiesNamespace,
				},
				Data: map[string]string{
					capabilities.TKGMultipleCLCapabilityKey: "false",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, configMap)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		When("updated", func() {
			JustBeforeEach(func() {
				configMap.Data = map[string]string{
					capabilities.TKGMultipleCLCapabilityKey: "true",
				}
				Expect(ctx.Client.Update(ctx, configMap)).To(Succeed())
			})

			It("should be reconciled", func() {
				Eventually(func(g Gomega) {
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
}
