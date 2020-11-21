// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infraprovider_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {

	var (
		ctx  *builder.IntegrationTestContext
		node *v1.Node
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		node = &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-node",
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVmProvider.Reset()
	})

	Context("Reconcile", func() {

		var isCalled int32

		BeforeEach(func() {
			intgFakeVmProvider.Lock()
			intgFakeVmProvider.ComputeClusterCpuMinFrequencyFn = func(ctx context.Context) error {
				atomic.AddInt32(&isCalled, 1)
				return nil
			}
			intgFakeVmProvider.Unlock()

			Expect(ctx.Client.Create(ctx, node)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, node)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("Verify that provider is called to update CPU frequncy", func() {
			Eventually(func() int32 {
				return atomic.LoadInt32(&isCalled)
			}).Should(Equal(int32(1)))
		})
	})
}
