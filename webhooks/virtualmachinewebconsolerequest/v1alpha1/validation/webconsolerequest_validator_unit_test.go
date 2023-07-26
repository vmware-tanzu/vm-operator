// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"crypto/rsa"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	wcr        *vmopv1.WebConsoleRequest
	oldWcr     *vmopv1.WebConsoleRequest
	privateKey *rsa.PrivateKey
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	privateKey, publicKeyPem := builder.WebConsoleRequestKeyPair()

	wcr := builder.DummyWebConsoleRequest("some-namespace", "some-name", "some-vm-name", publicKeyPem)
	wcr.Labels = map[string]string{
		v1alpha1.UUIDLabelKey: "some-uuid",
	}
	obj, err := builder.ToUnstructured(wcr)
	Expect(err).ToNot(HaveOccurred())

	var oldWcr *vmopv1.WebConsoleRequest
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldWcr = wcr.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldWcr)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		wcr:                                 wcr,
		oldWcr:                              oldWcr,
		privateKey:                          privateKey,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		emptyVirtualMachineName bool
		emptyPublicKey          bool
		invalidPublicKey        bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.emptyVirtualMachineName {
			ctx.wcr.Spec.VirtualMachineName = ""
		}
		if args.emptyPublicKey {
			ctx.wcr.Spec.PublicKey = ""
		}
		if args.invalidPublicKey {
			ctx.wcr.Spec.PublicKey = "invalid-public-key"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.wcr)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
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
		Entry("should deny empty virtualmachinename", createArgs{emptyVirtualMachineName: true}, false, "spec.virtualMachineName: Required value", nil),
		Entry("should deny empty publickey", createArgs{emptyPublicKey: true}, false, "spec.publicKey: Required value", nil),
		Entry("should deny invalid publickey", createArgs{invalidPublicKey: true}, false, "spec.publicKey: Invalid value: \"\": invalid public key format", nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		updateVirtualMachineName bool
		updatePublicKey          bool
		updateUUIDLabel          bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.updateVirtualMachineName {
			ctx.wcr.Spec.VirtualMachineName = "new-vm-name"
		}

		if args.updatePublicKey {
			ctx.wcr.Spec.PublicKey = "new-public-key"
		}

		if args.updateUUIDLabel {
			ctx.wcr.Labels[v1alpha1.UUIDLabelKey] = "new-uuid"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured((ctx.wcr))
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
		Entry("should deny VirtualmachineName change", updateArgs{updateVirtualMachineName: true}, false, "spec.virtualMachineName: Invalid value: \"new-vm-name\": field is immutable", nil),
		Entry("should deny PublicKey change", updateArgs{updatePublicKey: true}, false, "spec.publicKey: Invalid value: \"new-public-key\": field is immutable", nil),
		Entry("should deny UUID label change", updateArgs{updateUUIDLabel: true}, false, "metadata.labels[vmoperator.vmware.com/webconsolerequest-uuid]: Invalid value: \"new-uuid\": field is immutable", nil),
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
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
