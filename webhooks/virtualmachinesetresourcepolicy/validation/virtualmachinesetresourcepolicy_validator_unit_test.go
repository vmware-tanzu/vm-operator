// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vmRP    *vmopv1.VirtualMachineSetResourcePolicy
	oldVMRP *vmopv1.VirtualMachineSetResourcePolicy
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmRP := builder.DummyVirtualMachineSetResourcePolicy()
	obj, err := builder.ToUnstructured(vmRP)
	Expect(err).ToNot(HaveOccurred())

	var oldVMRP *vmopv1.VirtualMachineSetResourcePolicy
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMRP = vmRP.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMRP)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmRP:                                vmRP,
		oldVMRP:                             oldVMRP,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		noCpuLimit           bool
		noMemoryLimit        bool
		invalidCpuRequest    bool
		invalidMemoryRequest bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.noCpuLimit {
			ctx.vmRP.Spec.ResourcePool.Limits.Cpu = resource.MustParse("0")
		}
		if args.noMemoryLimit {
			ctx.vmRP.Spec.ResourcePool.Limits.Memory = resource.MustParse("0")
		}
		if args.invalidCpuRequest {
			ctx.vmRP.Spec.ResourcePool.Reservations.Cpu = resource.MustParse("2Gi")
			ctx.vmRP.Spec.ResourcePool.Limits.Cpu = resource.MustParse("1Gi")
		}
		if args.invalidMemoryRequest {
			ctx.vmRP.Spec.ResourcePool.Reservations.Memory = resource.MustParse("2Gi")
			ctx.vmRP.Spec.ResourcePool.Limits.Memory = resource.MustParse("1Gi")
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmRP)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should allow no cpu limit", createArgs{noCpuLimit: true}, true, nil, nil),
		Entry("should allow no memory limit", createArgs{noMemoryLimit: true}, true, nil, nil),
		Entry("should deny invalid cpu reservation", createArgs{invalidCpuRequest: true}, false, "CPU reservation must not be larger than the CPU limit", nil),
		Entry("should deny invalid memory reservation", createArgs{invalidMemoryRequest: true}, false, "memory reservation must not be larger than the memory limit", nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		changeCpu    bool
		changeMemory bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.changeCpu {
			ctx.vmRP.Spec.ResourcePool.Reservations.Cpu = resource.MustParse("5Gi")
			ctx.vmRP.Spec.ResourcePool.Limits.Cpu = resource.MustParse("10Gi")
		}
		if args.changeMemory {
			ctx.vmRP.Spec.ResourcePool.Reservations.Memory = resource.MustParse("5Gi")
			ctx.vmRP.Spec.ResourcePool.Limits.Memory = resource.MustParse("10Gi")
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmRP)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny policy cpu change", updateArgs{changeCpu: true}, false, "updates to immutable fields are not allowed", nil),
		Entry("should deny policy memory change", updateArgs{changeMemory: true}, false, "updates to immutable fields are not allowed", nil),
	)

	When("the update is performed while object deletion", func() {
		JustBeforeEach(func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
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
			// BMV: Is this set at this point for Delete?
			//t := metav1.Now()
			//ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
