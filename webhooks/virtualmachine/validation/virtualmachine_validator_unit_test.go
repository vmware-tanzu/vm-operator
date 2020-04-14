// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
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
	vm    *vmopv1.VirtualMachine
	oldVM *vmopv1.VirtualMachine
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachine()
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	var oldVM *vmopv1.VirtualMachine
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVM = vm.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vm:                                  vm,
		oldVM:                               oldVM,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidImageName   bool
		invalidClassName   bool
		invalidNetworkName bool
		invalidNetworkType bool
		invalidVolumeName  bool
		dupVolumeName      bool
		invalidPVC         bool
		invalidPVCName     bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidClassName {
			ctx.vm.Spec.ClassName = ""
		}
		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
		}
		if args.invalidNetworkName {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = ""
		}
		if args.invalidNetworkType {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = "bogusNetworkType"
		}
		if args.invalidVolumeName {
			ctx.vm.Spec.Volumes[0].Name = ""
		}
		if args.dupVolumeName {
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, ctx.vm.Spec.Volumes[0])
		}
		if args.invalidPVC {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = nil
		}
		if args.invalidPVCName {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = ""
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
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
		Entry("should deny invalid class name", createArgs{invalidClassName: true}, false, "spec.className must be specified", nil),
		Entry("should deny invalid image name", createArgs{invalidImageName: true}, false, "spec.imageName must be specified", nil),
		Entry("should deny invalid network name", createArgs{invalidNetworkName: true}, false, "spec.networkInterfaces[0].networkName must be specified", nil),
		Entry("should deny invalid network type", createArgs{invalidNetworkType: true}, false, "spec.networkInterfaces[0].networkType is not supported", nil),
		Entry("should deny invalid volume name", createArgs{invalidVolumeName: true}, false, "spec.volumes[0].name must be specified", nil),
		Entry("should deny duplicated volume names", createArgs{dupVolumeName: true}, false, "spec.volumes[1].name must be unique", nil),
		Entry("should deny invalid PVC", createArgs{invalidPVC: true}, false, "spec.volumes[0].persistentVolumeClaim must be specified", nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false, "spec.volumes[0].persistentVolumeClaim.claimName must be specified", nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		changeClassName bool
		changeImageName bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.changeClassName {
			ctx.vm.Spec.ClassName += "-updated"
		}
		if args.changeImageName {
			ctx.vm.Spec.ImageName += "-updated"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
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
		Entry("should deny class name change", updateArgs{changeClassName: true}, false, "updates to immutable fields are not allowed", nil),
		Entry("should deny image name change", updateArgs{changeImageName: true}, false, "updates to immutable fields are not allowed", nil),
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
