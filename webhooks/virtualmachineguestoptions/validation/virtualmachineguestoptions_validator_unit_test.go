// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateDelete,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook

	vmgo    *vimv1.VirtualMachineGuestOptions
	oldVMGO *vimv1.VirtualMachineGuestOptions
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmgo := builder.DummyVirtualMachineGuestOptions("otherlinux64guest", "otherLinux64Guest")
	obj, err := builder.ToUnstructured(vmgo)
	Expect(err).ToNot(HaveOccurred())

	var (
		oldVMGO *vimv1.VirtualMachineGuestOptions
		oldObj  *unstructured.Unstructured
	)

	if isUpdate {
		oldVMGO = vmgo.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMGO)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmgo:                                vmgo,
		oldVMGO:                             oldVMGO,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		name string
		id   string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		vmgo := builder.DummyVirtualMachineGuestOptions(args.name, args.id)
		obj, err := builder.ToUnstructured(vmgo)
		Expect(err).ToNot(HaveOccurred())

		ctx.Obj = obj

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if !expectedAllowed {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("spec.id is provided and metadata.name matches its DNS-safe transform", func() {
		It("should allow creation", func() {
			validateCreate(createArgs{name: "otherlinux64guest", id: "otherLinux64Guest"}, true, "")
		})

		It("should allow creation when the id needs no transformation", func() {
			validateCreate(createArgs{name: "dos", id: "dos"}, true, "")
		})
	})

	When("spec.id is empty", func() {
		It("should deny creation", func() {
			validateCreate(createArgs{name: "", id: ""}, false, "id must be provided")
		})
	})

	When("metadata.name does not equal the DNS-safe transform of spec.id", func() {
		It("should deny creation", func() {
			validateCreate(
				createArgs{name: "not-a-match", id: "otherLinux64Guest"},
				false,
				"metadata.name must equal the DNS-safe transform of spec.id",
			)
		})
	})
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		newID string
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		vmgo := builder.DummyVirtualMachineGuestOptions("otherlinux64guest", args.newID)
		obj, err := builder.ToUnstructured(vmgo)
		Expect(err).ToNot(HaveOccurred())

		ctx.Obj = obj

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if !expectedAllowed {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("spec.id is unchanged", func() {
		It("should allow the update", func() {
			validateUpdate(updateArgs{newID: "otherLinux64Guest"}, true, "")
		})
	})

	// spec.id immutability is enforced by the CRD's CEL transition rule, not
	// this webhook, so it cannot be exercised via a direct ValidateUpdate
	// call. See the intg suite for coverage of the CEL rejection through a
	// real API server. What remains testable here is the base
	// re-validation this webhook still performs on update: a changed id
	// whose DNS-safe transform no longer matches metadata.name is denied on
	// that basis.
	When("spec.id is changed", func() {
		It("should deny the update", func() {
			validateUpdate(updateArgs{newID: "dos"}, false, "metadata.name must equal the DNS-safe transform of spec.id")
		})
	})

	When("the object predates this webhook and was already invalid", func() {
		It("should deny an update that leaves an empty spec.id unchanged", func() {
			legacy := builder.DummyVirtualMachineGuestOptions("legacy-empty-id", "")
			legacyObj, err := builder.ToUnstructured(legacy)
			Expect(err).ToNot(HaveOccurred())

			ctx = &unitValidatingWebhookContext{
				UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(legacyObj, legacyObj),
			}

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring("id must be provided"))
		})
	})
}

func unitTestsValidateDelete() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("delete is performed", func() {
		It("should always allow", func() {
			response := ctx.ValidateDelete(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})
}
