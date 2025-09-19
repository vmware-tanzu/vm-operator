// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Serial,
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
					Name:      pkgcfg.FromContext(suite.Context).VCCredsSecretName,
				},
			}
		})

		JustBeforeEach(func() {
			provider.Lock()
			provider.UpdateVcCredsFn = func(_ context.Context, _ map[string][]byte) error {
				atomic.AddInt32(&called, 1)
				return nil
			}
			provider.Unlock()
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, obj)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			atomic.StoreInt32(&called, 0)
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
						// NOTE: ResetVcClient() won't be called during the reconcile because the
						// obj namespace won't match the pod's namespace. It is bad news if you see
						// "Reconciling unexpected object" in the logs.
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
						// NOTE: UpdateVcCreds() won't be called during the reconcile because the
						// obj namespace won't match the pod's namespace. It is bad news if you see
						// "Reconciling unexpected object" in the logs.
						return atomic.LoadInt32(&called)
					}).Should(Equal(int32(0)))
				})
			})
		})
	})
}
