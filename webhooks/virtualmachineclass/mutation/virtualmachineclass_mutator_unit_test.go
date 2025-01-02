// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass/mutation"
)

func uniTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.V1Alpha3,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		unitTestsMutating,
	)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vmClass *vmopv1.VirtualMachineClass
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vmClass := builder.DummyVirtualMachineClassGenName()
	obj, err := builder.ToUnstructured(vmClass)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vmClass:                           vmClass,
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

	Describe("VirtualMachineClassMutator should admit updates when object is under deletion", func() {
		Context("when update request comes in while deletion in progress ", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Describe("SetControllerName", func() {
		When("an update attempts to set an empty spec.controllerName field to empty", func() {
			It("should not indicate anything was mutated", func() {
				oldObj, newObj := ctx.vmClass.DeepCopy(), ctx.vmClass.DeepCopy()
				oldObj.Spec.ControllerName = ""
				newObj.Spec.ControllerName = ""
				Expect(mutation.SetControllerName(newObj, oldObj)).To(BeFalse())
				Expect(newObj.Spec.ControllerName).To(BeEmpty())
			})
		})
		When("an update attempts to set a non-empty spec.controllerName field to empty", func() {
			It("should preserve the original value", func() {
				oldObj, newObj := ctx.vmClass.DeepCopy(), ctx.vmClass.DeepCopy()
				oldObj.Spec.ControllerName = "hello"
				newObj.Spec.ControllerName = ""
				Expect(mutation.SetControllerName(newObj, oldObj)).To(BeTrue())
				Expect(newObj.Spec.ControllerName).To(Equal("hello"))
			})
		})
		When("an update attempts to set a non-empty spec.controllerName field to a new, non-empty value", func() {
			It("should not mutate the request", func() {
				oldObj, newObj := ctx.vmClass.DeepCopy(), ctx.vmClass.DeepCopy()
				oldObj.Spec.ControllerName = "hello"
				newObj.Spec.ControllerName = "world"
				Expect(mutation.SetControllerName(newObj, oldObj)).To(BeFalse())
				Expect(newObj.Spec.ControllerName).To(Equal("world"))
			})
		})
	})
}
