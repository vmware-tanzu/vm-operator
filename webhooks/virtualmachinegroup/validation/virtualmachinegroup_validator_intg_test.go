// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyNamespaceName                       = "dummy-namespace"
	invalidTime                              = "invalid-time"
	invalidTimeFormatMsg                     = "time must be in RFC3339Nano format"
	modifyAnnotationNotAllowedForNonAdminMsg = "modifying this annotation is not allowed for non-admin users"
	emptyPowerStateNotAllowedAfterSetMsg     = "cannot set powerState to empty once it's been set"
	selfRefMemberOrGroupMsg                  = "group cannot have itself as a member or group name"
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
		"Update",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdate,
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
	vmGroup *vmopv1.VirtualMachineGroup
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmGroup = &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vmgroup-root",
			Namespace: ctx.Namespace,
		},
		Spec: vmopv1.VirtualMachineGroupSpec{
			BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
				{
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
			},
		},
	}

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		invalidTimeFormat bool
		duplicateMember   bool
		selfReferenced    bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		if ctx.vmGroup.Annotations == nil {
			ctx.vmGroup.Annotations = make(map[string]string)
		}
		if args.invalidTimeFormat {
			ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = invalidTime
		} else {
			ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = time.Now().Format(time.RFC3339Nano)
		}

		if args.duplicateMember {
			ctx.vmGroup.Spec.BootOrder = append(ctx.vmGroup.Spec.BootOrder, ctx.vmGroup.Spec.BootOrder...)
		}

		if args.selfReferenced {
			ctx.vmGroup.Spec.BootOrder = append(ctx.vmGroup.Spec.BootOrder, vmopv1.VirtualMachineGroupBootOrderGroup{
				Members: []vmopv1.GroupMember{
					{
						Kind: "VirtualMachineGroup",
						Name: ctx.vmGroup.Name,
					},
				},
			})
			ctx.vmGroup.Spec.GroupName = ctx.vmGroup.Name
		}

		err := ctx.Client.Create(ctx, ctx.vmGroup)
		if expectedAllowed {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
			if expectedReason != "" {
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			}
		}
	}

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("should work", createArgs{}, true, ""),
		Entry("should not work with invalid last-updated-power-state annotation",
			createArgs{invalidTimeFormat: true}, false, invalidTimeFormatMsg),
		Entry("should not work with duplicate members",
			createArgs{duplicateMember: true}, false, "spec.bootOrder[1].members[0]: Duplicate value: \"VirtualMachine/vm-1\", spec.bootOrder[1].members[1]: Duplicate value: \"VirtualMachine/vm-2\", spec.bootOrder[1].members[2]: Duplicate value: \"VirtualMachineGroup/vmgroup-1\""),
		Entry("should not work with self referenced member or group name",
			createArgs{selfReferenced: true}, false, selfRefMemberOrGroupMsg),
	)
}

func intgTestsValidateUpdate() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmGroup)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("update is performed with empty power state after it's been set", func() {
		BeforeEach(func() {
			// First set a power state
			ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(ctx.Client.Update(suite, ctx.vmGroup)).To(Succeed())

			// Then try to set it to empty
			ctx.vmGroup.Spec.PowerState = ""
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			expectedPath := field.NewPath("spec", "powerState")
			Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			Expect(err.Error()).To(ContainSubstring(emptyPowerStateNotAllowedAfterSetMsg))
		})
	})

	When("update is performed with invalid time format in last-updated-power-state annotation", func() {
		BeforeEach(func() {
			if ctx.vmGroup.Annotations == nil {
				ctx.vmGroup.Annotations = make(map[string]string)
			}
			ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = invalidTime
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			expectedPath := field.NewPath("metadata", "annotations").Key(constants.LastUpdatedPowerStateTimeAnnotation)
			Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			Expect(err.Error()).To(ContainSubstring(invalidTimeFormatMsg))
		})
	})

	When("update is performed with valid time format in last-updated-power-state annotation", func() {
		BeforeEach(func() {
			// This would be rejected for non-admin users in a real environment,
			// but the integration test context doesn't fully simulate user permissions
			if ctx.vmGroup.Annotations == nil {
				ctx.vmGroup.Annotations = make(map[string]string)
			}
			ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation] = time.Now().Format(time.RFC3339Nano)
		})

		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("update is performed with duplicate members among different boot orders", func() {
		BeforeEach(func() {
			ctx.vmGroup.Spec.BootOrder = append(ctx.vmGroup.Spec.BootOrder, ctx.vmGroup.Spec.BootOrder...)
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			bootOrderPath := field.NewPath("spec", "bootOrder")
			for i := 1; i < len(ctx.vmGroup.Spec.BootOrder); i++ {
				for j, member := range ctx.vmGroup.Spec.BootOrder[i].Members {
					curPath := bootOrderPath.Index(i).Child("members").Index(j)
					Expect(err.Error()).To(ContainSubstring(curPath.String()))
					dupVal := fmt.Sprintf("Duplicate value: \"%s/%s\"", member.Kind, member.Name)
					Expect(err.Error()).To(ContainSubstring(dupVal))
				}
			}
		})
	})

	When("update is performed with self reference member", func() {
		BeforeEach(func() {
			ctx.vmGroup.Spec.BootOrder = append(ctx.vmGroup.Spec.BootOrder, vmopv1.VirtualMachineGroupBootOrderGroup{
				Members: []vmopv1.GroupMember{
					{
						Kind: "VirtualMachineGroup",
						Name: ctx.vmGroup.Name,
					},
				},
			})
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(selfRefMemberOrGroupMsg))
		})
	})

	When("update is performed with self reference group name", func() {
		BeforeEach(func() {
			ctx.vmGroup.Spec.GroupName = ctx.vmGroup.Name
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(selfRefMemberOrGroupMsg))
		})
	})
}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
	})

	It("should allow delete", func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmGroup)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})
}
