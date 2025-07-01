// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
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

		resourcePolicy    *vmopv1.VirtualMachineSetResourcePolicy
		resourcePolicyKey client.ObjectKey
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		resourcePolicy = &vmopv1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm-policy",
			},
			Spec: vmopv1.VirtualMachineSetResourcePolicySpec{},
		}

		resourcePolicyKey = client.ObjectKey{Namespace: resourcePolicy.Namespace, Name: resourcePolicy.Name}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getResourcePolicy := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1.VirtualMachineSetResourcePolicy {
		rp := &vmopv1.VirtualMachineSetResourcePolicy{}
		if err := ctx.Client.Get(ctx, objKey, rp); err != nil {
			return nil
		}
		return rp
	}

	waitForResourcePolicyFinalizer := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) {
		Eventually(func() []string {
			if rp := getResourcePolicy(ctx, objKey); rp != nil {
				return rp.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for VirtualMachineSetResourcePolicy finalizer")
	}

	Context("Reconcile", func() {
		var called atomic.Bool

		BeforeEach(func() {
			called.Store(false)

			intgFakeVMProvider.Lock()
			intgFakeVMProvider.CreateOrUpdateVirtualMachineSetResourcePolicyFn = func(_ context.Context, _ *vmopv1.VirtualMachineSetResourcePolicy) error {
				called.Store(true)
				return nil
			}
			intgFakeVMProvider.Unlock()
		})

		It("Reconciles after VirtualMachineSetResourcePolicy creation", func() {
			Expect(ctx.Client.Create(ctx, resourcePolicy)).To(Succeed())

			By("VirtualMachineSetResourcePolicy should have finalizer added", func() {
				waitForResourcePolicyFinalizer(ctx, resourcePolicyKey)
			})

			By("Create policy should be called", func() {
				Eventually(called.Load).Should(BeTrue())
			})

			By("Reconcile when Zone is added", func() {
				called.Store(false)

				zone := &topologyv1.Zone{}
				zone.Name = "my-new-zone"
				zone.Namespace = ctx.Namespace
				Expect(ctx.Client.Create(ctx, zone)).To(Succeed())

				By("Reconcile again because of Zone create", func() {
					Eventually(called.Load).Should(BeTrue())
				})
			})

			By("Deleting the VirtualMachineSetResourcePolicy", func() {
				err := ctx.Client.Delete(ctx, resourcePolicy)
				Expect(err).ToNot(HaveOccurred())
			})

			By("VirtualMachineSetResourcePolicy should have finalizer removed", func() {
				Eventually(func() []string {
					if rp := getResourcePolicy(ctx, resourcePolicyKey); rp != nil {
						return rp.GetFinalizers()
					}
					return nil
				}).ShouldNot(ContainElement(finalizer))
			})
		})
	})
}
