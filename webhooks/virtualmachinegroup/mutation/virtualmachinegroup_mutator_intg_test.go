// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		intgTestsMutating,
	)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vmGroup *vmopv1.VirtualMachineGroup
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmGroup = &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-vm-group",
			Namespace: ctx.Namespace,
		},
	}

	return ctx
}

func intgTestsMutating() {
	var (
		ctx *intgMutatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
	})

	AfterEach(func() {
		ctx = nil
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			AfterEach(func() {
				Expect(ctx.Client.Delete(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, ctx.vmGroup)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("PowerState Mutation", func() {
		AfterEach(func() {
			if ctx.vmGroup.ResourceVersion != "" {
				Expect(ctx.Client.Delete(ctx, ctx.vmGroup)).To(Succeed())
			}
		})

		When("Creating VirtualMachineGroup with non-empty power state", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			It("Should set last-updated-power-state annotation", func() {
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())

				modified := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), modified)).To(Succeed())
				Expect(modified.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
				timestamp := modified.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
				_, err := time.Parse(time.RFC3339, timestamp)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Updating VirtualMachineGroup with power state change", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("Should update last-updated-power-state annotation", func() {
				modified := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), modified)).To(Succeed())

				// Record the original timestamp
				originalTimestamp := modified.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
				Expect(originalTimestamp).ToNot(BeEmpty())

				// Change power state
				modified.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Update(ctx, modified)).To(Succeed())

				// Get the updated object
				updated := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), updated)).To(Succeed())

				// Check that the annotation was updated
				Expect(updated.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
				newTimestamp := updated.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
				Expect(newTimestamp).NotTo(Equal(originalTimestamp))
				_, err := time.Parse(time.RFC3339Nano, newTimestamp)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Updating VirtualMachineGroup with power state change and pre-existing apply power state change time annotation", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				if ctx.vmGroup.Annotations == nil {
					ctx.vmGroup.Annotations = make(map[string]string)
				}
				ctx.vmGroup.Annotations[constants.ApplyPowerStateTimeAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("Should remove the apply power state change time annotation", func() {
				modified := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), modified)).To(Succeed())

				// Verify all the expected annotations are present
				Expect(modified.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
				Expect(modified.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))

				// Change power state
				modified.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Update(ctx, modified)).To(Succeed())

				// Get the updated object
				updated := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), updated)).To(Succeed())

				// Verify the apply power state change time annotation is removed.
				Expect(updated.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
				Expect(updated.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
			})
		})

		When("Updating VirtualMachineGroup without power state change", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("Should not update last-updated-power-state annotation", func() {
				modified := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), modified)).To(Succeed())

				// Record the original timestamp
				originalTimestamp := modified.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]

				// Update without changing power state
				modified.Labels = map[string]string{"new-label": "value"}
				Expect(ctx.Client.Update(ctx, modified)).To(Succeed())

				// Get the updated object
				updated := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), updated)).To(Succeed())

				// Check that the annotation was not updated
				Expect(updated.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
				newTimestamp := updated.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
				Expect(newTimestamp).To(Equal(originalTimestamp))
			})
		})

		When("Updating VirtualMachineGroup with nextForcePowerStateSyncTime set to 'now'", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("Should update last-updated-power-state annotation", func() {
				modified := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), modified)).To(Succeed())

				// Record the original timestamp
				originalTimestamp := modified.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
				Expect(originalTimestamp).ToNot(BeEmpty())

				// Update with nextForcePowerStateSyncTime set to 'now'
				modified.Spec.NextForcePowerStateSyncTime = "now"
				Expect(ctx.Client.Update(ctx, modified)).To(Succeed())

				// Get the updated object
				updated := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), updated)).To(Succeed())

				// Check that the nextForcePowerStateSyncTime is set to a timestamp
				Expect(updated.Spec.NextForcePowerStateSyncTime).ToNot(BeEmpty())
				nextSyncTimestamp, err := time.Parse(time.RFC3339Nano, updated.Spec.NextForcePowerStateSyncTime)
				Expect(err).ToNot(HaveOccurred())
				Expect(nextSyncTimestamp).NotTo(Equal(originalTimestamp))

				// Check that the annotation was updated to the same timestamp as the nextForcePowerStateSyncTime
				Expect(updated.Annotations).To(HaveKey(constants.LastUpdatedPowerStateTimeAnnotation))
				lastUpdatedAnnoTimestamp, err := time.Parse(time.RFC3339Nano, updated.Annotations[constants.LastUpdatedPowerStateTimeAnnotation])
				Expect(err).ToNot(HaveOccurred())
				Expect(lastUpdatedAnnoTimestamp).To(Equal(nextSyncTimestamp))
			})
		})
	})

	Context("Group Name", func() {
		AfterEach(func() {
			if ctx.vmGroup.ResourceVersion != "" {
				Expect(ctx.Client.Delete(ctx, ctx.vmGroup)).To(Succeed())
			}
		})

		When("VMGroup has an owner reference to a group", func() {
			BeforeEach(func() {
				ctx.vmGroup.Spec.GroupName = "my-group"
				ctx.vmGroup.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: vmopv1.GroupVersion.String(),
						Kind:       "VirtualMachineGroup",
						Name:       "my-group",
						UID:        "my-group-uid",
					},
				}
				Expect(ctx.Client.Create(ctx, ctx.vmGroup)).To(Succeed())
			})

			It("should remove the group owner reference if the VMGroup.Spec.GroupName is changed", func() {
				vmGroup := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), vmGroup)).To(Succeed())
				vmGroup.Spec.GroupName = ""
				Expect(ctx.Client.Update(ctx, vmGroup)).To(Succeed())
				updated := &vmopv1.VirtualMachineGroup{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vmGroup), updated)).To(Succeed())
				Expect(updated.OwnerReferences).To(BeEmpty())
			})
		})
	})
}
