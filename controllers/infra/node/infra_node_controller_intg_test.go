// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package node_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		obj *corev1.Node
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		obj = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-node",
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile", func() {
		var isCalled int32

		BeforeEach(func() {
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.ComputeCPUMinFrequencyFn = func(ctx context.Context) error {
				atomic.AddInt32(&isCalled, 1)
				return nil
			}
			intgFakeVMProvider.Unlock()

			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, obj)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("Verify that provider is called to update CPU frequency", func() {
			Eventually(func() int32 {
				return atomic.LoadInt32(&isCalled)
			}).Should(Equal(int32(1)))
		})
	})
}
