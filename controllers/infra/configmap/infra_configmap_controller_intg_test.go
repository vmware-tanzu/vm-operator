// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package configmap_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/configmap"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Reconcile", Label("controller", "envtest", "v1alpha2", "vcsim"), intgTestsReconcile)
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
		provider.Reset()
	})

	Context("WcpClusterConfigMap", func() {
		var (
			obj       *corev1.ConfigMap
			savedPnid string
			savedPort string
		)

		BeforeEach(func() {
			var err error
			obj, err = configmap.NewWcpClusterConfigMap(configmap.WcpClusterConfig{
				VcPNID: "dummy-pnid",
				VcPort: "dummy-port",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).ToNot(BeNil())
		})

		JustBeforeEach(func() {
			provider.Lock()
			defer provider.Unlock()
			provider.UpdateVcPNIDFn = func(_ context.Context, pnid, port string) error {
				savedPnid = pnid
				savedPort = port
				return nil
			}
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			savedPnid, savedPort = "", ""
			err := ctx.Client.Delete(ctx, obj)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("updated", func() {
			JustBeforeEach(func() {
				data, err := yaml.Marshal(configmap.WcpClusterConfig{
					VcPNID: "new-pnid",
					VcPort: "new-port",
				})
				Expect(err).ToNot(HaveOccurred())
				obj.Data[configmap.WcpClusterConfigFileName] = string(data)
				Expect(ctx.Client.Update(ctx, obj)).To(Succeed())
			})

			When("in expected namespace", func() {
				It("should be reconciled", func() {
					Eventually(func() string {
						provider.Lock()
						defer provider.Unlock()
						return savedPnid + "::" + savedPort
					}).Should(Equal("new-pnid::new-port"))
				})
			})

			When("in unexpected namespace", func() {
				BeforeEach(func() {
					obj.Namespace = ctx.Namespace
				})
				It("should not be reconciled", func() {
					Consistently(func() string {
						provider.Lock()
						defer provider.Unlock()
						return savedPnid + "::" + savedPort
					}).ShouldNot(Equal("new-pnid::new-port"))
				})
			})
		})
	})
}
