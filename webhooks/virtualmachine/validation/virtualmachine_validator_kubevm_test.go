// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func kubevmAnnotationTests() {
	Describe(
		"kube-vm.io/virtual-machine annotation immutability",
		Label(testlabels.Update, testlabels.API, testlabels.Validation, testlabels.Webhook),
		unitTestsKubeVMAnnotationImmutability,
	)
}

func unitTestsKubeVMAnnotationImmutability() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
		ctx.oldVM.Annotations[kubevm.AnnotationKey] = "my-generic-vm"
		ctx.vm.Annotations[kubevm.AnnotationKey] = "my-generic-vm"

		oldObj, err := builder.ToUnstructured(ctx.oldVM)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj = oldObj
	})

	When("the annotation is unchanged", func() {
		It("allows the update", func() {
			obj, err := builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			ctx.WebhookRequestContext.Obj = obj

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})

	When("a non-privileged caller changes the annotation to name a different generic object", func() {
		It("denies the update", func() {
			ctx.vm.Annotations[kubevm.AnnotationKey] = "some-other-generic-vm"
			obj, err := builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			ctx.WebhookRequestContext.Obj = obj

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(
				field.Forbidden(
					field.NewPath("metadata", "annotations").Key(kubevm.AnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error()))
		})
	})

	When("a non-privileged caller clears the annotation", func() {
		It("denies the update", func() {
			delete(ctx.vm.Annotations, kubevm.AnnotationKey)
			obj, err := builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			ctx.WebhookRequestContext.Obj = obj

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
		})
	})

	When("a privileged caller changes the annotation", func() {
		It("allows the update", func() {
			ctx.IsPrivilegedAccount = true
			ctx.vm.Annotations[kubevm.AnnotationKey] = "some-other-generic-vm"
			obj, err := builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			ctx.WebhookRequestContext.Obj = obj

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})
}
