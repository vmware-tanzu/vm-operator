// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
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
		"Update with VM Group Publish Request",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdateWithVMGroupPub,
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
	vmPub *vmopv1.VirtualMachinePublishRequest
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmPub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", ctx.Namespace, "dummy-vm",
		"dummy-item", "dummy-cl")
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

	It("should allow the request", func() {
		Eventually(func() error {
			return ctx.Client.Create(ctx, ctx.vmPub)
		}).Should(Succeed())
	})

	When("create with managed by vm group publish request label", func() {
		var (
			vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest
		)
		BeforeEach(func() {
			vmGroupPubReq = builder.DummyVirtualMachineGroupPublishRequest(
				builder.DummyVMGroupPublishRequestName, ctx.vmPub.Namespace, []string{})
			vmGroupPubReq.SetUID("dummy-uid")
			ctx.vmPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: vmGroupPubReq.Name}
		})
		AfterEach(func() {
			vmGroupPubReq = nil
		})

		It("should deny the request if owner reference is not set", func() {
			Eventually(func(g Gomega) {
				err := ctx.Client.Create(ctx, ctx.vmPub)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(
					fmt.Sprintf("%q label is reserved", vmopv1.VirtualMachinePublishRequestManagedByLabelKey)))
			}).Should(Succeed())
		})

		It("should allow the request if owner reference is set", func() {
			Expect(ctrlutil.SetOwnerReference(vmGroupPubReq, ctx.vmPub, ctx.Client.Scheme())).Should(Succeed())
			Eventually(func() error {
				return ctx.Client.Create(ctx, ctx.vmPub)
			}).Should(Succeed())
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmPub)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmPub)
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmPub)).To(Succeed())
		err = nil
		ctx = nil
	})

	When("update is performed with changed source name", func() {
		BeforeEach(func() {
			ctx.vmPub.Spec.Source.Name = "alternate-vm-name"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
		})
	})

	When("update is performed with changed target info", func() {
		BeforeEach(func() {
			ctx.vmPub.Spec.Target.Location.Name = "alternate-cl"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
		})
	})

	When("update is performed with added managed by label", func() {
		BeforeEach(func() {
			ctx.vmPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(
				fmt.Sprintf("%q label is reserved", vmopv1.VirtualMachinePublishRequestManagedByLabelKey)))
		})
	})
}

func intgTestsValidateUpdateWithVMGroupPub() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		vmGroupPubReq := builder.DummyVirtualMachineGroupPublishRequest(
			builder.DummyVMGroupPublishRequestName, ctx.vmPub.Namespace, []string{})
		vmGroupPubReq.SetUID("dummy-uid")
		ctx.vmPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: vmGroupPubReq.Name}
		Expect(ctrlutil.SetOwnerReference(vmGroupPubReq, ctx.vmPub, ctx.Client.Scheme())).Should(Succeed())
		Expect(ctx.Client.Create(ctx, ctx.vmPub)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmPub)
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmPub)).To(Succeed())
		err = nil
		ctx = nil
	})

	When("update is performed while request is created with managed by label", func() {
		BeforeEach(func() {
			ctx.vmPub.Labels = nil
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot update VirtualMachineGroupPublishRequest owned VirtualMachinePublishRequest"))
		})
	})
}

func intgTestsValidateDelete() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmPub)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vmPub)
	})

	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
