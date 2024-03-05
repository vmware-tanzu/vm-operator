// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/secret"
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

	Context("VcCredsSecret", func() {
		var (
			obj    *corev1.Secret
			called int32
		)

		BeforeEach(func() {
			obj = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.PodNamespace,
					Name:      secret.VcCredsSecretName,
				},
			}
		})

		JustBeforeEach(func() {
			provider.Lock()
			provider.ResetVcClientFn = func(_ context.Context) {
				atomic.AddInt32(&called, 1)
			}
			provider.Unlock()
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			called = 0
			err := ctx.Client.Delete(ctx, obj)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("created", func() {
			When("in expected namespace", func() {
				It("should be reconciled once", func() {
					Eventually(func() int32 {
						return atomic.LoadInt32(&called)
					}).Should(Equal(int32(1)))
				})
			})

			When("in unexpected namespace", func() {
				BeforeEach(func() {
					obj.Namespace = ctx.Namespace
				})
				It("should not be reconciled", func() {
					Consistently(func() int32 {
						return atomic.LoadInt32(&called)
					}).Should(Equal(int32(0)))
				})
			})
		})

		When("updated", func() {
			JustBeforeEach(func() {
				obj.StringData = map[string]string{"foo": "vmware-bar"}
				Expect(ctx.Client.Update(ctx, obj)).To(Succeed())
			})

			When("in expected namespace", func() {
				It("should be reconciled", func() {
					Eventually(func() int32 {
						return atomic.LoadInt32(&called)
					}).Should(BeNumerically(">=", int32(2)))
				})
			})

			When("in unexpected namespace", func() {
				BeforeEach(func() {
					obj.Namespace = ctx.Namespace
				})
				It("should not be reconciled", func() {
					Consistently(func() int32 {
						return atomic.LoadInt32(&called)
					}).Should(Equal(int32(0)))
				})
			})
		})
	})

}
