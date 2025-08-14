// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinegroup/mutation"
)

const (
	nextForcePowerStateSyncTimeNow = "now"
)

func uniTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		unitTestsMutating,
	)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vmGroup *vmopv1.VirtualMachineGroup
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vmGroup := &vmopv1.VirtualMachineGroup{}
	obj, err := builder.ToUnstructured(vmGroup)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vmGroup:                           vmGroup,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})

	Describe("VirtualMachineGroupMutator should admit updates when object is under deletion", func() {
		Context("when update request comes in while deletion in progress ", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Describe("ProcessPowerState", func() {
		Context("Creation", func() {
			When("Spec.PowerState is set", func() {
				It("Should add the last-updated-power-state annotation", func() {
					ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
					timestamp := ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
					updatedTime, err := time.Parse(time.RFC3339, timestamp)
					Expect(err).ToNot(HaveOccurred())
					Expect(updatedTime).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is set to 'now'", func() {
				It("Should add the last-updated-power-state annotation", func() {
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = nextForcePowerStateSyncTimeNow

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
					timestamp := ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
					updatedTime, err := time.Parse(time.RFC3339, timestamp)
					Expect(err).ToNot(HaveOccurred())
					Expect(updatedTime).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is set to something other than 'now'", func() {
				It("Should deny the request", func() {
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = "not-now"

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, nil)
					Expect(result).To(BeFalse())
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("may only be set to \"now\""))
				})
			})
		})

		Context("Update", func() {
			When("Spec.PowerState is updated", func() {
				It("Should add the last-updated-power-state annotation", func() {
					oldVMGroup := ctx.vmGroup.DeepCopy()
					oldVMGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, oldVMGroup)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
					timestamp := ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
					updatedTime, err := time.Parse(time.RFC3339, timestamp)
					Expect(err).ToNot(HaveOccurred())
					Expect(updatedTime).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is set to 'now'", func() {
				It("Should add the last-updated-power-state annotation", func() {
					oldVMGroup := ctx.vmGroup.DeepCopy()
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = nextForcePowerStateSyncTimeNow

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, oldVMGroup)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
					timestamp := ctx.vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
					updatedTime, err := time.Parse(time.RFC3339, timestamp)
					Expect(err).ToNot(HaveOccurred())
					Expect(updatedTime).To(BeTemporally("~", time.Now(), time.Second))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is set to something other than 'now'", func() {
				It("Should deny the request", func() {
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = "not-now"

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, nil)
					Expect(result).To(BeFalse())
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("may only be set to \"now\""))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is removed", func() {

				var now = time.Now().UTC().Format(time.RFC3339Nano)

				It("Should set it to the previous value", func() {
					oldVMGroup := ctx.vmGroup.DeepCopy()
					oldVMGroup.Spec.NextForcePowerStateSyncTime = now
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = ""

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, oldVMGroup)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Spec.NextForcePowerStateSyncTime).To(Equal(now))
				})
			})

			When("Spec.PowerState is updated with a pre-existing apply-power-state time annotation", func() {
				It("Should remove the apply-power-state time annotation", func() {
					oldVMGroup := ctx.vmGroup.DeepCopy()
					oldVMGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					if oldVMGroup.Annotations == nil {
						oldVMGroup.Annotations = make(map[string]string)
					}
					oldVMGroup.Annotations[constants.ApplyPowerStateTimeAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
					ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, oldVMGroup)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
				})
			})

			When("Spec.NextForcePowerStateSyncTime is set to 'now' with a pre-existing apply-power-state time annotation", func() {
				It("Should remove the apply-power-state time annotation", func() {
					oldVMGroup := ctx.vmGroup.DeepCopy()
					if oldVMGroup.Annotations == nil {
						oldVMGroup.Annotations = make(map[string]string)
					}
					oldVMGroup.Annotations[constants.ApplyPowerStateTimeAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
					ctx.vmGroup.Spec.NextForcePowerStateSyncTime = nextForcePowerStateSyncTimeNow

					result, err := mutation.ProcessPowerState(&ctx.WebhookRequestContext, ctx.vmGroup, oldVMGroup)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(BeTrue())
					Expect(ctx.vmGroup.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
				})
			})
		})
	})
}
