// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vmClass    *vmopv1.VirtualMachineClass
	oldVMClass *vmopv1.VirtualMachineClass
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmClass := builder.DummyVirtualMachineClass()
	obj, err := builder.ToUnstructured(vmClass)
	Expect(err).ToNot(HaveOccurred())

	var oldVMClass *vmopv1.VirtualMachineClass
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMClass = vmClass.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMClass)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmClass:                             vmClass,
		oldVMClass:                          oldVMClass,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidCpuRequest    bool
		invalidMemoryRequest bool
		noCpuLimit           bool
		noMemoryLimit        bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidCpuRequest {
			ctx.vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("2Gi")
			ctx.vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("1Gi")
		}
		if args.invalidMemoryRequest {
			ctx.vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("2Gi")
			ctx.vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("1Gi")
		}
		if args.noCpuLimit {
			ctx.vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("0")
		}
		if args.noMemoryLimit {
			ctx.vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("0")
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmClass)
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
		Entry("should deny invalid cpu request", createArgs{invalidCpuRequest: true}, false, "CPU request must not be larger than the CPU limit", nil),
		Entry("should deny invalid memory request", createArgs{invalidMemoryRequest: true}, false, "memory request must not be larger than the memory limit", nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response

		// represents the VM Service FSS. This should be manipulated atomically to avoid races where
		// the controller is trying to read this _while_ the tests are updating it.
		vmServiceFSS uint32
	)

	type updateArgs struct {
		changeHwCpu    bool
		changeHwMemory bool
		changeCpu      bool
		changeMemory   bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.changeHwCpu {
			ctx.vmClass.Spec.Hardware.Cpus = 64
		}
		if args.changeHwMemory {
			ctx.vmClass.Spec.Hardware.Memory = resource.MustParse("5Gi")
		}
		if args.changeCpu {
			ctx.vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("5Gi")
			ctx.vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("10Gi")
		}
		if args.changeMemory {
			ctx.vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("5Gi")
			ctx.vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("10Gi")
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmClass)
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
		// Modify the helper function to return the custom value of the FSS
		lib.IsVMServiceFSSEnabled = func() bool {
			return atomic.LoadUint32(&vmServiceFSS) != 0
		}

		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	Context("WCP_VMService FSS is set", func() {
		var oldVMServiceFSSState uint32

		BeforeEach(func() {
			oldVMServiceFSSState = vmServiceFSS
			atomic.StoreUint32(&vmServiceFSS, 1)
		})
		AfterEach(func() {
			atomic.StoreUint32(&vmServiceFSS, oldVMServiceFSSState)
		})

		DescribeTable("update table with VMService FSS", validateUpdate,
			Entry("should allow", updateArgs{}, true, nil, nil),
			Entry("should allow hw cpu change", updateArgs{changeHwCpu: true}, true, nil, nil),
			Entry("should allow hw memory change", updateArgs{changeHwMemory: true}, true, nil, nil),
			Entry("should allow policy cpu change", updateArgs{changeCpu: true}, true, nil, nil),
			Entry("should allow policy memory change", updateArgs{changeMemory: true}, true, nil, nil),
		)
	})

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny hw cpu change", updateArgs{changeHwCpu: true}, false, "updates to immutable fields are not allowed", nil),
		Entry("should deny hw memory change", updateArgs{changeHwMemory: true}, false, "updates to immutable fields are not allowed", nil),
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
