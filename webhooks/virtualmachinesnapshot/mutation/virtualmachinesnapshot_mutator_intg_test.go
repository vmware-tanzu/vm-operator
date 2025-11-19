// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		intgTestsMutating,
	)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vmSnapshot *vmopv1.VirtualMachineSnapshot
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmSnapshot = builder.DummyVirtualMachineSnapshot(ctx.Namespace, "dummy-vm-snapshot", "dummy-vm")

	return ctx
}

func intgTestsMutating() {
	var (
		ctx        *intgMutatingWebhookContext
		vmSnapshot *vmopv1.VirtualMachineSnapshot
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		vmSnapshot = ctx.vmSnapshot.DeepCopy()
	})
	AfterEach(func() {
		ctx = nil
	})

	Describe("mutate", func() {
		Context("basic webhook functionality", func() {
			It("should allow creating snapshots", func() {
				err := ctx.Client.Create(ctx, vmSnapshot)
				Expect(err).ToNot(HaveOccurred())

				By("verifying the VM name label")
				snap := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmSnapshot), snap)).To(Succeed())
				Expect(snap.Labels).To(HaveKeyWithValue(vmopv1.VMNameForSnapshotLabel, vmSnapshot.Spec.VMName))
			})

			It("should allow updating snapshots", func() {
				err := ctx.Client.Create(ctx, vmSnapshot)
				Expect(err).ToNot(HaveOccurred())

				// Update description
				vmSnapshot.Spec.Description = "Updated description"
				err = ctx.Client.Update(ctx, vmSnapshot)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
