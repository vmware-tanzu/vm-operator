// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"

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

	// The v1alpha6 schema rejects a request default above its limit default
	// before the webhook runs; the webhook check covers writes through older
	// API versions and is unit tested.
	DescribeTable("create table", validateCreate,
		Entry("should work", true, "", nil),
		Entry("should not work for invalid", false, "the memory request default must not exceed the memory limit default", nil),
	)

	DescribeTable("class policy schema", validateSchema,
		Entry("accepts numeric and string quantities", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("1")), Max: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("2")),
			}
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("1Gi")), Max: ptr.To(resource.MustParse("8Gi")), Default: ptr.To(resource.MustParse("4Gi")),
			}
		}, false),
		Entry("rejects bound without default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{Min: ptr.To(resource.MustParse("1"))}
		}, true),
		Entry("rejects default below minimum", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("2")),
			}
		}, true),
		Entry("rejects default above maximum", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{
				Max: ptr.To(resource.MustParse("4")), Default: ptr.To(resource.MustParse("8")),
			}
		}, true),
		Entry("rejects fractional CPU size defaults", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("1500m"))}
		}, true),
		Entry("rejects CPU size defaults above int32", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("2147483648"))}
		}, true),
		Entry("rejects a zero CPU size", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("0"))}
		}, true),
		Entry("rejects a zero CPU size min", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.CPU = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("0")), Default: ptr.To(resource.MustParse("2")),
			}
		}, true),
		Entry("rejects a zero memory size", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("0"))}
		}, true),
		Entry("accepts a memory size of exactly 4Mi", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("4Mi"))}
			class.Spec.Resources.Requests.Memory = nil
			class.Spec.Resources.Limits.Memory = nil
		}, false),
		Entry("rejects a memory size below 4Mi", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("2Mi"))}
		}, true),
		Entry("rejects a memory size that is not a multiple of 4Mi", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{Default: ptr.To(resource.MustParse("4097Mi"))}
		}, true),
		Entry("rejects a memory size max that is not a multiple of 4Mi", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("4Gi")), Max: ptr.To(resource.MustParse("8G")), Default: ptr.To(resource.MustParse("4Gi")),
			}
		}, true),
		Entry("rejects a negative memory size min", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Memory = &vmopv1.ResourceQuantityRange{
				Min: ptr.To(resource.MustParse("-1Gi")), Default: ptr.To(resource.MustParse("2Gi")),
			}
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
		Entry("accepts an unset limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = nil
		}, ""),
		Entry("accepts a limit with no default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = &vmopv1.ResourceQuantityRange{}
		}, ""),
		Entry("accepts a limit with no max", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "1")
			class.Spec.Resources.Limits.CPU = quantityRange("2", "", "4")
		}, ""),
		Entry("accepts a bounded limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("4Gi", "16Gi", "8Gi")
		}, ""),
		Entry("accepts a zero request", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("0", "2", "0")
		}, ""),
		Entry("accepts a request default equal to the limit default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "2")
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "2")
		}, ""),
		Entry("accepts a request default with an unlimited limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "8")
			class.Spec.Resources.Limits.CPU = &vmopv1.ResourceQuantityRange{}
		}, ""),
		Entry("rejects a zero limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = nil
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "0")
		}, "limits must be greater than 0"),
		Entry("rejects a zero limit min", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.CPU = quantityRange("0", "4", "4")
		}, "limits must be greater than 0"),
		Entry("rejects a -1 CPU limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = nil
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "-1")
		}, "limits must be greater than 0"),
		Entry("rejects a fixed -1 memory limit", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.Memory = nil
			class.Spec.Resources.Limits.Memory = quantityRange("-1", "-1", "-1")
		}, "limits must be greater than 0"),
		Entry("rejects a fractional negative memory limit bound", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Limits.Memory = quantityRange("-500m", "", "8Gi")
		}, "limits must be greater than 0"),
		Entry("rejects a negative CPU request", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "-1")
		}, "requests must not be negative"),
		Entry("rejects a negative memory request bound", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.Memory = quantityRange("-1Gi", "", "2Gi")
		}, "requests must not be negative"),
		Entry("rejects a CPU request default above the CPU limit default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.CPU = quantityRange("", "", "3")
			class.Spec.Resources.Limits.CPU = quantityRange("", "", "2")
		}, "the CPU request default must not exceed the CPU limit default"),
		Entry("rejects a memory request default above the memory limit default", func(class *vmopv1.VirtualMachineClass) {
			class.Spec.Resources.Requests.Memory = quantityRange("", "", "8Gi")
			class.Spec.Resources.Limits.Memory = quantityRange("", "", "4Gi")
		}, "the memory request default must not exceed the memory limit default"),
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
