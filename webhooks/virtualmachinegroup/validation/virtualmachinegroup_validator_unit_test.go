// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
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
	vmGroup, oldVMGroup *vmopv1.VirtualMachineGroup
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmGroup := &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-vmgroup",
			Namespace: dummyNamespaceName,
		},
		Spec: vmopv1.VirtualMachineGroupSpec{
			Members: []vmopv1.GroupMember{
				{
					Kind: "VirtualMachine",
					Name: "vm-1",
				},
				{
					Kind: "VirtualMachine",
					Name: "vm-2",
				},
				{
					Kind: "VirtualMachineGroup",
					Name: "vmgroup-1",
				},
			},
		},
	}

	obj, err := builder.ToUnstructured(vmGroup)
	Expect(err).ToNot(HaveOccurred())

	var oldVMGroup *vmopv1.VirtualMachineGroup
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMGroup = vmGroup.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMGroup)
		Expect(err).ToNot(HaveOccurred())
	}

	initObjects := []client.Object{}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, initObjects...),
		vmGroup:                             vmGroup,
		oldVMGroup:                          oldVMGroup,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		isServiceUser         bool
		powerState            vmopv1.VirtualMachinePowerState
		lastUpdatedPowerState string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		ctx = newUnitTestContextForValidatingWebhook(false)

		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}

		ctx.vmGroup.Spec.PowerState = args.powerState

		if args.lastUpdatedPowerState != "" {
			if ctx.vmGroup.Annotations == nil {
				ctx.vmGroup.Annotations = make(map[string]string)
			}
			ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = args.lastUpdatedPowerState
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmGroup)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if !expectedAllowed && expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	DescribeTable("create table", validateCreate,
		Entry("should work with no power state", createArgs{}, true, ""),
		Entry("should work with power state on", createArgs{powerState: vmopv1.VirtualMachinePowerStateOn}, true, ""),
		Entry("should work with power state off", createArgs{powerState: vmopv1.VirtualMachinePowerStateOff}, true, ""),
		Entry("should not work with invalid time format in last-updated-power-state annotation",
			createArgs{isServiceUser: true, lastUpdatedPowerState: invalidTime}, false, invalidTimeFormatMsg),
		Entry("should work with valid time format in last-updated-power-state annotation",
			createArgs{isServiceUser: true, lastUpdatedPowerState: time.Now().Format(time.RFC3339Nano)}, true, ""),
		Entry("should not work with non-admin modifying last-updated-power-state annotation",
			createArgs{isServiceUser: false, lastUpdatedPowerState: time.Now().Format(time.RFC3339Nano)}, false, modifyAnnotationNotAllowedForNonAdminMsg),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		isServiceUser                bool
		oldPowerState                vmopv1.VirtualMachinePowerState
		newPowerState                vmopv1.VirtualMachinePowerState
		modifyLastUpdatedPowerState  bool
		invalidLastUpdatedPowerState bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		ctx = newUnitTestContextForValidatingWebhook(true)

		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}

		// Setup old VMGroup
		ctx.oldVMGroup.Spec.PowerState = args.oldPowerState

		// Setup new VMGroup
		ctx.vmGroup.Spec.PowerState = args.newPowerState

		if args.modifyLastUpdatedPowerState {
			if ctx.vmGroup.Annotations == nil {
				ctx.vmGroup.Annotations = make(map[string]string)
			}
			if ctx.oldVMGroup.Annotations == nil {
				ctx.oldVMGroup.Annotations = make(map[string]string)
			}

			ctx.oldVMGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = "2023-01-01T00:00:00Z"

			if args.invalidLastUpdatedPowerState {
				ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = invalidTime
			} else {
				ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = time.Now().Format(time.RFC3339Nano)
			}
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmGroup)
		Expect(err).ToNot(HaveOccurred())

		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMGroup)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if !expectedAllowed && expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	DescribeTable("update table", validateUpdate,
		Entry("should work with no power state change",
			updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateOn, newPowerState: vmopv1.VirtualMachinePowerStateOn}, true, ""),
		Entry("should work with power state change",
			updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateOn, newPowerState: vmopv1.VirtualMachinePowerStateOff}, true, ""),
		Entry("should not work with empty power state after it's been set",
			updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateOn, newPowerState: ""}, false, emptyPowerStateNotAllowedAfterSetMsg),
		Entry("should not work with non-admin modifying last-updated-power-state annotation",
			updateArgs{modifyLastUpdatedPowerState: true, isServiceUser: false}, false, modifyAnnotationNotAllowedForNonAdminMsg),
		Entry("should work with admin modifying last-updated-power-state annotation",
			updateArgs{modifyLastUpdatedPowerState: true, isServiceUser: true}, true, ""),
		Entry("should not work with invalid time format in last-updated-power-state annotation",
			updateArgs{modifyLastUpdatedPowerState: true, isServiceUser: true, invalidLastUpdatedPowerState: true}, false, invalidTimeFormatMsg),
	)
}

func unitTestsValidateDelete() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	It("should allow delete", func() {
		response := ctx.ValidateDelete(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(BeTrue())
	})
}
