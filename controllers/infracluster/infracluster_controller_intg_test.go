// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infracluster_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/controllers/infracluster"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	Context("VcCredsSecret", func() {
		var secret *corev1.Secret

		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.PodNamespace,
					Name:      infracluster.VcCredsSecretName,
				},
			}
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, secret)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("Secret is updated", func() {
			var called int32

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.ResetVcClientFn = func(_ context.Context) {
					atomic.AddInt32(&called, 1)
				}
				intgFakeVMProvider.Unlock()

				Expect(ctx.Client.Create(ctx, secret)).To(Succeed())
			})

			It("Resets Vc client", func() {
				// Wait for initial reconcile.
				Eventually(func() int32 { return atomic.LoadInt32(&called) }).Should(Equal(int32(1)))

				secret.StringData = map[string]string{"foo": "vmware-bar"}
				Expect(ctx.Client.Update(ctx, secret)).To(Succeed())

				Eventually(func() int32 { return atomic.LoadInt32(&called) }).Should(BeNumerically(">=", int32(2)))
			})
		})
	})

	Context("WcpClusterConfigMap", func() {
		var configMap *corev1.ConfigMap

		BeforeEach(func() {
			var err error
			configMap, err = infracluster.NewWcpClusterConfigMap(infracluster.WcpClusterConfig{
				VcPNID: "dummy-pnid",
				VcPort: "dummy-port",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(configMap).ToNot(BeNil())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, configMap)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("ConfigMap is updated", func() {
			var savedPnid, savedPort string

			BeforeEach(func() {
				intgFakeVMProvider.Lock()
				intgFakeVMProvider.UpdateVcPNIDFn = func(_ context.Context, pnid, port string) error {
					savedPnid = pnid
					savedPort = port
					return nil
				}
				intgFakeVMProvider.Unlock()
				Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())
			})

			It("Updates provider", func() {
				var err error
				configMap, err = infracluster.NewWcpClusterConfigMap(infracluster.WcpClusterConfig{
					VcPNID: "new-pnid",
					VcPort: "new-port",
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.Client.Update(ctx, configMap)).To(Succeed())

				Eventually(func() string {
					intgFakeVMProvider.Lock()
					defer intgFakeVMProvider.Unlock()
					return savedPnid + "::" + savedPort
				}).Should(Equal("new-pnid::new-port"))
			})
		})
	})
}
