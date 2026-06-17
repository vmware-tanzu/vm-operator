// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

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
	vmConfigOptions    *vimv1.VirtualMachineConfigOptions
	oldVMConfigOptions *vimv1.VirtualMachineConfigOptions
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmConfigOptions := &vimv1.VirtualMachineConfigOptions{}
	vmConfigOptions.Name = "dummy-vmconfigoptions"
	vmConfigOptions.Spec.HardwareVersion = "vmx-19"

	obj, err := builder.ToUnstructured(vmConfigOptions)
	Expect(err).ToNot(HaveOccurred())

	var oldVMConfigOptions *vimv1.VirtualMachineConfigOptions
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMConfigOptions = vmConfigOptions.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMConfigOptions)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmConfigOptions:                     vmConfigOptions,
		oldVMConfigOptions:                  oldVMConfigOptions,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		hardwareVersion string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		ctx.vmConfigOptions.Spec.HardwareVersion = args.hardwareVersion

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmConfigOptions)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("Create VirtualMachineConfigOptions", validateCreate,
		Entry("should allow valid hardware version vmx-19",
			createArgs{hardwareVersion: "vmx-19"},
			true, ""),
		Entry("should allow valid hardware version vmx-22",
			createArgs{hardwareVersion: "vmx-22"},
			true, ""),
		Entry("should allow valid hardware version vmx-100",
			createArgs{hardwareVersion: "vmx-100"},
			true, ""),
		Entry("should deny empty hardwareVersion",
			createArgs{hardwareVersion: ""},
			false, "hardwareVersion must be provided"),
		Entry("should deny invalid format vmx-abc",
			createArgs{hardwareVersion: "vmx-abc"},
			false, "hardwareVersion must match pattern"),
		Entry("should deny invalid format vmx",
			createArgs{hardwareVersion: "vmx"},
			false, "hardwareVersion must match pattern"),
		Entry("should deny invalid format vmxxx-22",
			createArgs{hardwareVersion: "vmxxx-22"},
			false, "hardwareVersion must match pattern"),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		newHardwareVersion string
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		ctx.vmConfigOptions.Spec.HardwareVersion = args.newHardwareVersion

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmConfigOptions)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMConfigOptions)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})

	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("Update VirtualMachineConfigOptions", validateUpdate,
		Entry("should allow unchanged hardwareVersion",
			updateArgs{newHardwareVersion: "vmx-19"},
			true, ""),
		Entry("should deny changed hardwareVersion",
			updateArgs{newHardwareVersion: "vmx-21"},
			false, "field is immutable"),
	)
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("the delete is performed", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})
	})
}
