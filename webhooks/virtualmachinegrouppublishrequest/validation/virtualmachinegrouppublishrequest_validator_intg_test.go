// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var (
	mutatingWebhookDownWarning = "The mutating webhook should have filled it if empty."
	sourceMissingMsg           = "spec.source: Required value: " + mutatingWebhookDownWarning
	sourceNotFoundMsg          = "spec.source: Not found"
	targetMissingMsg           = "spec.target: Required value: " + mutatingWebhookDownWarning
	targetNotFoundMsg          = "spec.target: Not found"
	vmsMissingMsg              = "spec.virtualMachines: Required value: " + mutatingWebhookDownWarning
	ttlInvalidValueMsg         = "spec.ttlSecondsAfterFinished: Invalid value"
)

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest
}

func newIntgValidatingWebhookContext(setDefaultSpec bool) *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}
	ctx.vmGroupPubReq = &vmopv1.VirtualMachineGroupPublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      builder.DummyVMGroupPublishRequestName,
			Namespace: ctx.Namespace,
		},
	}
	if setDefaultSpec {
		setDefaultSpecValues(ctx)
	}
	return ctx
}

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

func intgTestsValidateCreate() {
	var (
		ctx       *intgValidatingWebhookContext
		createErr error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext(false)
	})
	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		createErr = ctx.Client.Create(ctx.Context, ctx.vmGroupPubReq)
	})

	When("validate with an empty spec", func() {
		It("should return those fields should have filled by mutating webhook message", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).To(ContainSubstring(sourceMissingMsg))
			Expect(createErr.Error()).To(ContainSubstring(targetMissingMsg))
			Expect(createErr.Error()).To(ContainSubstring(vmsMissingMsg))
		})
	})

	When("spec.source is defined but it does not exist", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Source = ctx.vmGroupPubReq.Name
		})
		It("should return error message contains source not found sub-string", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).To(ContainSubstring(sourceNotFoundMsg))
		})
	})

	When("spec.source is defined and it exists", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Source = ctx.vmGroupPubReq.Name
			Expect(ctx.Client.Create(ctx, &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.vmGroupPubReq.Namespace,
					Name:      ctx.vmGroupPubReq.Spec.Source,
				},
			})).To(Succeed())
		})
		It("should not return any source related issues", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).ToNot(ContainSubstring(sourceMissingMsg))
			Expect(createErr.Error()).ToNot(ContainSubstring(sourceNotFoundMsg))
		})
	})

	When("spec.target is defined but it does not exist", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Target = builder.DummyContentLibraryName
		})
		It("should return error message contains source not found sub-string", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).To(ContainSubstring(targetNotFoundMsg))
		})
	})

	When("spec.target is defined and it exists", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Target = builder.DummyContentLibraryName
			Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
				builder.DummyContentLibraryName, ctx.vmGroupPubReq.Namespace, ""))).To(Succeed())
		})
		It("should not return any target related issues", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).ToNot(ContainSubstring(targetMissingMsg))
			Expect(createErr.Error()).ToNot(ContainSubstring(targetNotFoundMsg))
		})
	})

	When("spec.ttlSecondsAfterFinished is defined with negative number", func() {
		BeforeEach(func() {
			negativeTTL := int64(-1)
			ctx.vmGroupPubReq.Spec.TTLSecondsAfterFinished = &negativeTTL
		})
		It("should return error message contains TTL less than 0", func() {
			Expect(createErr).To(HaveOccurred())
			Expect(createErr.Error()).To(ContainSubstring(ttlInvalidValueMsg))
		})
	})

	When("spec is filled properly with resources created", func() {
		BeforeEach(func() {
			setDefaultSpecValues(ctx)
			Expect(setCreateRequiredResources(ctx)).To(Succeed())
		})
		It("should allow", func() {
			Expect(createErr).ToNot(HaveOccurred())
		})
	})

}

func intgTestsValidateUpdate() {
	var (
		ctx       *intgValidatingWebhookContext
		updateErr error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext(true)
		Expect(setCreateRequiredResources(ctx)).To(Succeed())
		Expect(ctx.Client.Create(ctx.Context, ctx.vmGroupPubReq)).To(Succeed())
	})
	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		updateErr = ctx.Client.Update(ctx.Context, ctx.vmGroupPubReq)
	})

	When("spec.source, spec.target, or spec.virtualMachines is updated", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Source = uuid.NewString()
			ctx.vmGroupPubReq.Spec.Target = uuid.NewString()
			ctx.vmGroupPubReq.Spec.VirtualMachines = nil
		})
		It("should return error with field is immutable message", func() {
			Expect(updateErr).To(HaveOccurred())
			Expect(updateErr.Error()).To(ContainSubstring(
				fmt.Sprintf("spec.source: Invalid value: %q: field is immutable", ctx.vmGroupPubReq.Spec.Source)))
			Expect(updateErr.Error()).To(ContainSubstring(
				fmt.Sprintf("spec.target: Invalid value: %q: field is immutable", ctx.vmGroupPubReq.Spec.Target)))
			Expect(updateErr.Error()).To(ContainSubstring("spec.virtualMachines: Invalid value: []string(nil): field is immutable"))
		})
	})

	When("spec.ttlSecondsAfterFinished is updated to a non-negative number", func() {
		BeforeEach(func() {
			ttl := int64(0)
			ctx.vmGroupPubReq.Spec.TTLSecondsAfterFinished = &ttl
		})
		It("should allow", func() {
			Expect(updateErr).ToNot(HaveOccurred())
		})
	})
}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext(true)
		Expect(setCreateRequiredResources(ctx)).To(Succeed())
		Expect(ctx.Client.Create(ctx, ctx.vmGroupPubReq)).To(Succeed())
	})
	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("should allow delete", func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmGroupPubReq)).To(Succeed())
	})
}

func setDefaultSpecValues(ctx *intgValidatingWebhookContext) {
	ctx.vmGroupPubReq.Spec = vmopv1.VirtualMachineGroupPublishRequestSpec{
		Source:          ctx.vmGroupPubReq.Name,
		Target:          builder.DummyContentLibraryName,
		VirtualMachines: []string{builder.DummyVirtualMachineName + "-0"},
	}
}

func setCreateRequiredResources(ctx *intgValidatingWebhookContext) error {
	if err := ctx.Client.Create(ctx.Context, &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ctx.vmGroupPubReq.Namespace,
			Name:      ctx.vmGroupPubReq.Spec.Source,
		},
	}); err != nil {
		return err
	}

	if err := ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
		builder.DummyContentLibraryName, ctx.vmGroupPubReq.Namespace, "")); err != nil {
		return err
	}

	return nil
}
