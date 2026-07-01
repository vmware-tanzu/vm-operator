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

	vmco    *vimv1.VirtualMachineConfigOptions
	oldVMCO *vimv1.VirtualMachineConfigOptions
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmco := builder.DummyVirtualMachineConfigOptions("vmx-21", "vmx-21")
	obj, err := builder.ToUnstructured(vmco)
	Expect(err).ToNot(HaveOccurred())

	var (
		oldVMCO *vimv1.VirtualMachineConfigOptions
		oldObj  *unstructured.Unstructured
	)

	if isUpdate {
		oldVMCO = vmco.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMCO)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmco:    vmco,
		oldVMCO: oldVMCO,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		name            string
		hardwareVersion string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		vmco := builder.DummyVirtualMachineConfigOptions(args.name, args.hardwareVersion)
		obj, err := builder.ToUnstructured(vmco)
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

	When("the hardwareVersion is valid and the name matches", func() {
		It("should allow creation with vmx-N format", func() {
			validateCreate(createArgs{name: "vmx-21", hardwareVersion: "vmx-21"}, true, "")
		})

		It("should allow creation with a large version number", func() {
			validateCreate(createArgs{name: "vmx-1000", hardwareVersion: "vmx-1000"}, true, "")
		})

		It("should allow creation with version 0", func() {
			validateCreate(createArgs{name: "vmx-0", hardwareVersion: "vmx-0"}, true, "")
		})
	})

	When("the hardwareVersion is empty", func() {
		It("should deny creation", func() {
			validateCreate(createArgs{name: "", hardwareVersion: ""}, false, "hardwareVersion")
		})
	})

	When("the hardwareVersion does not match the required pattern", func() {
		It("should deny creation when the numeric suffix is missing", func() {
			validateCreate(createArgs{name: "vmx-", hardwareVersion: "vmx-"}, false, "hardwareVersion")
		})

		It("should deny creation without the vmx- prefix", func() {
			validateCreate(createArgs{name: "21", hardwareVersion: "21"}, false, "hardwareVersion")
		})

		It("should deny creation with a non-numeric suffix", func() {
			validateCreate(createArgs{name: "vmx-abc", hardwareVersion: "vmx-abc"}, false, "hardwareVersion")
		})

		It("should deny creation with extra characters after the digits", func() {
			validateCreate(createArgs{name: "vmx-21a", hardwareVersion: "vmx-21a"}, false, "hardwareVersion")
		})

		It("should deny creation with an uppercase prefix", func() {
			validateCreate(createArgs{name: "VMX-21", hardwareVersion: "VMX-21"}, false, "hardwareVersion")
		})
	})

	When("metadata.name does not equal spec.hardwareVersion", func() {
		It("should deny creation", func() {
			validateCreate(createArgs{name: "vmx-22", hardwareVersion: "vmx-21"}, false, "metadata.name must equal spec.hardwareVersion")
		})
	})
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		newHardwareVersion string
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		vmco := builder.DummyVirtualMachineConfigOptions("test-vmco", args.newHardwareVersion)
		obj, err := builder.ToUnstructured(vmco)
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

	When("the hardwareVersion is unchanged", func() {
		It("should allow the update", func() {
			validateUpdate(updateArgs{newHardwareVersion: "vmx-21"}, true, "")
		})
	})

	When("the hardwareVersion is changed", func() {
		It("should deny the update", func() {
			validateUpdate(updateArgs{newHardwareVersion: "vmx-22"}, false, "field is immutable")
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
