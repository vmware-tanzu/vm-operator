// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateDelete,
	)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vmSnapshot *vmopv1.VirtualMachineSnapshot
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmSnapshot = builder.DummyVirtualMachineSnapshot(ctx.Namespace, "dummy-vm-snapshot", "dummy-vm")
	// Set the label on the snapshot manually since the mutation webhook is not running.
	metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, ctx.vmSnapshot.Spec.VMRef.Name)

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	AfterEach(func() {
		ctx = nil
	})

	When("the VMSnapshot is created", func() {
		It("should allow the request", func() {
			Expect(ctx.Client.Create(ctx, ctx.vmSnapshot)).To(Succeed())
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx                 *intgValidatingWebhookContext
		originalDescription string
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmSnapshot)).To(Succeed())
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmSnapshot)).To(Succeed())
		ctx = nil
	})

	When("the VMSnapshot with a new description", func() {
		BeforeEach(func() {
			originalDescription = ctx.vmSnapshot.Spec.Description
			ctx.vmSnapshot.Spec.Description = "new-description"
		})

		AfterEach(func() {
			ctx.vmSnapshot.Spec.Description = originalDescription
		})

		It("should should allow the request", func() {
			Expect(ctx.Client.Update(ctx, ctx.vmSnapshot)).To(Succeed())
		})
	})

	When("trying to update VM name label", func() {
		It("should reject attempt to change VM name label", func() {
			ctx.vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel] = "different-vm-name"
			err := ctx.Client.Update(ctx, ctx.vmSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
		})

		It("should reject attempt to remove VM name label", func() {
			delete(ctx.vmSnapshot.Labels, vmopv1.VMNameForSnapshotLabel)
			err := ctx.Client.Update(ctx, ctx.vmSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
		})
	})
}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	When("delete is performed", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ctx.vmSnapshot)).To(Succeed())
		})

		It("should allow the request", func() {
			Expect(ctx.Client.Delete(ctx, ctx.vmSnapshot)).To(Succeed())
		})
	})
}
