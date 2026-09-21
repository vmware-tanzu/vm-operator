// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// unitTests establishes a regression baseline for the current no-op mutator
// (see the vmop-1827 TODOs in virtualmachinereplicaset_mutator.go): today
// Mutate never patches the object on create or update. Once vmop-1827 adds
// real mutation logic, these baseline assertions should be updated alongside
// it rather than left to silently rot.
func unitTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		unitTestsMutating,
	)
}

func intgTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		intgTestsMutating,
	)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	rs *vmopv1.VirtualMachineReplicaSet
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	rs := builder.DummyVirtualMachineReplicaSet()
	obj, err := builder.ToUnstructured(rs)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		rs:                                rs,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})

	AfterEach(func() {
		ctx = nil
	})

	When("the object is under deletion", func() {
		It("should admit the update without mutating", func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			ctx.WebhookRequestContext.Op = admissionv1.Update

			response := ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Patches).To(BeEmpty())
		})
	})

	When("a VirtualMachineReplicaSet is created", func() {
		It("should admit the request without patching it (current no-op baseline)", func() {
			ctx.WebhookRequestContext.Op = admissionv1.Create

			response := ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Patches).To(BeEmpty())
		})
	})

	When("a VirtualMachineReplicaSet is updated", func() {
		It("should admit the request without patching it (current no-op baseline)", func() {
			ctx.WebhookRequestContext.Op = admissionv1.Update
			ctx.rs.Spec.Template.Spec = vmopv1.VirtualMachineSpec{
				ImageName: "some-other-image",
			}

			var err error
			ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.rs)
			Expect(err).ToNot(HaveOccurred())

			response := ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Patches).To(BeEmpty())
		})
	})
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	rs *vmopv1.VirtualMachineReplicaSet
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.rs = builder.DummyVirtualMachineReplicaSet()
	ctx.rs.Name = "dummy-rs-for-webhook-mutation"
	ctx.rs.Namespace = ctx.Namespace
	ctx.rs.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"foo": "bar"},
	}
	ctx.rs.Spec.Template.Labels = map[string]string{"foo": "bar"}

	return ctx
}

func intgTestsMutating() {
	var (
		ctx *intgMutatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("current no-op mutator baseline", func() {
		It("creates the VirtualMachineReplicaSet unchanged", func() {
			Expect(ctx.Client.Create(ctx, ctx.rs)).To(Succeed())

			created := &vmopv1.VirtualMachineReplicaSet{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.rs), created)).To(Succeed())
			Expect(created.Annotations).To(BeEmpty())
			Expect(created.Spec.Template.Labels).To(Equal(ctx.rs.Spec.Template.Labels))
		})

		It("updates the VirtualMachineReplicaSet without any additional mutation", func() {
			Expect(ctx.Client.Create(ctx, ctx.rs)).To(Succeed())

			created := &vmopv1.VirtualMachineReplicaSet{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.rs), created)).To(Succeed())

			n := int32(2)
			created.Spec.Replicas = &n
			Expect(ctx.Client.Update(ctx, created)).To(Succeed())

			updated := &vmopv1.VirtualMachineReplicaSet{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.rs), updated)).To(Succeed())
			Expect(*updated.Spec.Replicas).To(Equal(n))
			Expect(updated.Annotations).To(BeEmpty())
		})
	})
}
