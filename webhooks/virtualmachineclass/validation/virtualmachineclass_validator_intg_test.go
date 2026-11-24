// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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
	vmClass *vmopv1.VirtualMachineClass
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmClass = builder.DummyVirtualMachineClassGenName()
	ctx.vmClass.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	validateCreate := func(expectedAllowed bool, expectedReason string, expectedErr error) {
		vmClass := ctx.vmClass.DeepCopy()

		if !expectedAllowed {
			vmClass.Spec.Resources.Requests.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("2Gi"))}
			vmClass.Spec.Resources.Limits.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("1Gi"))}
		}

		err := ctx.Client.Create(ctx, vmClass)
		if expectedAllowed {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
		if expectedReason != "" {
			Expect(err.Error()).To(ContainSubstring(expectedReason))
		}
	}

	validateSchema := func(mutator func(*vmopv1.VirtualMachineClass), expectFailure bool) {
		vmClass := ctx.vmClass.DeepCopy()
		mutator(vmClass)
		err := ctx.Client.Create(ctx, vmClass)
		if expectFailure {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	}

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})
	AfterEach(func() {
		ctx = nil
	})

	fieldPath := field.NewPath("spec", "resources", "requests", "memory")
	invalidMemField := field.Invalid(fieldPath, "2Gi", "memory request must not be larger than the memory limit")
	DescribeTable("create table", validateCreate,
		Entry("should work", true, "", nil),
		Entry("should not work for invalid", false, invalidMemField.Error(), nil),
	)

	DescribeTable("class policy schema", validateSchema,
		Entry("accepts numeric and string quantities", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("1")), Max: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("2")),
			}
			class.Spec.Resources.Size.Memory = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("1Gi")), Max: ptr.To(resource.MustParse("8Gi")), Default: ptr.To(resource.MustParse("4Gi")),
			}
		}, false),
		Entry("rejects bound without default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{Min: ptr.To(resource.MustParse("1"))}
		}, true),
		Entry("rejects default below minimum", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("2")),
			}
		}, true),
		Entry("rejects default above maximum", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{
				Max: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("8")),
			}
		}, true),
		Entry("rejects fractional CPU size defaults", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("1500m"))}
		}, true),
		Entry("rejects CPU size defaults above int32", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Size.CPU = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("2147483648"))}
		}, true),
	)

	quantityRange := func(minimum, maximum, def string) *vmopv1.ResourceQuantityRange {
		r := &vmopv1.ResourceQuantityRange{}
		for _, b := range []struct {
			value string
			field **resource.Quantity
		}{{minimum, &r.Min}, {maximum, &r.Max}, {def, &r.Default}} {
			if b.value != "" {
				*b.field = ptr.To(resource.MustParse(b.value))
			}
		}
		return r
	}

	// An empty wantErr means the class is admitted.
	validateAllocation := func(mutator func(*vmopv1.VirtualMachineClass), wantErr string) {
		vmClass := ctx.vmClass.DeepCopy()
		mutator(vmClass)
		err := ctx.Client.Create(ctx, vmClass)
		if wantErr == "" {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(MatchError(ContainSubstring(wantErr)))
		}
	}

	DescribeTable("class requests and limits schema", validateAllocation,
		Entry("accepts an unlimited (-1) CPU limit with a request", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "-1")
		}, ""),
		Entry("accepts a fixed unlimited (-1) memory limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("-1", "-1", "-1")
		}, ""),
		Entry("accepts a zero limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "0")
		}, ""),
		Entry("accepts a bounded limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("4Gi", "16Gi", "8Gi")
		}, ""),
		Entry("rejects a negative CPU limit other than -1", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "-2")
		}, "limits must not be negative, except -1 for unlimited"),
		Entry("rejects a fractional negative memory limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("", "", "-500m")
		}, "limits must not be negative, except -1 for unlimited"),
		Entry("rejects an unlimited (-1) CPU default under a bounded max", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = quantityRange("", "4", "-1")
		}, "an unlimited (-1) CPU limit must be the default and every set bound"),
		Entry("rejects an unlimited (-1) memory min with a bounded default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("-1", "", "8Gi")
		}, "an unlimited (-1) memory limit must be the default and every set bound"),
		Entry("rejects a negative CPU request", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "-1")
		}, "requests must not be negative"),
		Entry("rejects a negative memory request bound", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.Memory = quantityRange("-1Gi", "", "2Gi")
		}, "requests must not be negative"),
	)
}

func intgTestsValidateDelete() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		err = ctx.Client.Create(ctx, ctx.vmClass)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vmClass)
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
