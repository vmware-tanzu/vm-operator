// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesnapshot/mutation"
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
	vmSnapshot *vmopv1.VirtualMachineSnapshot
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vmSnapshot := builder.DummyVirtualMachineSnapshot("dummy-ns", "dummy-vm-snapshot", "dummy-vm")
	// Clear out the label that's set by default so we can test a clean object mutation
	vmSnapshot.Labels = nil

	obj, err := builder.ToUnstructured(vmSnapshot)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vmSnapshot:                        vmSnapshot,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})
	AfterEach(func() {
		ctx = nil
	})

	Describe("VirtualMachineSnapshotMutator should admit create operations", func() {
		Context("when creating snapshot with vmRef", func() {
			It("should set VM name label", func() {
				// Ensure the initial snapshot doesn't have the label
				Expect(ctx.vmSnapshot.Labels).To(BeNil())

				wasMutated := mutation.SetVMNameLabel(ctx.vmSnapshot)
				Expect(wasMutated).To(BeTrue())

				// Verify the label was set correctly
				Expect(ctx.vmSnapshot.Labels).ToNot(BeNil())
				Expect(ctx.vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel]).To(Equal("dummy-vm"))
			})

			It("should not mutate when label already exists (even with a wrong value)", func() {
				// Pre-set the label with the correct value
				ctx.vmSnapshot.Labels = map[string]string{
					vmopv1.VMNameForSnapshotLabel: "not-the-right-vm",
				}

				wasMutated := mutation.SetVMNameLabel(ctx.vmSnapshot)
				Expect(wasMutated).To(BeFalse())

				// Verify the label remained unchanged
				Expect(ctx.vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel]).To(Equal("not-the-right-vm"))
			})
		})

		Context("when creating snapshot without vmName", func() {
			It("should not set VM name label", func() {
				// Remove the vmName
				ctx.vmSnapshot.Spec.VMName = ""

				wasMutated := mutation.SetVMNameLabel(ctx.vmSnapshot)
				Expect(wasMutated).To(BeFalse())

				// Verify no labels were set
				Expect(ctx.vmSnapshot.Labels).To(BeNil())
			})
		})

		Context("when creating snapshot with empty vmName", func() {
			It("should not set VM name label", func() {
				// Set empty vmName
				ctx.vmSnapshot.Spec.VMName = ""

				wasMutated := mutation.SetVMNameLabel(ctx.vmSnapshot)
				Expect(wasMutated).To(BeFalse())

				// Verify no labels were set
				Expect(ctx.vmSnapshot.Labels).To(BeNil())
			})
		})
	})

	Describe("VirtualMachineSnapshotMutator should admit update operations", func() {
		Context("when updating a snapshot that is missing the VM name label", func() {
			It("should backfill the VM name label", func() {
				// Simulate a snapshot that predates this label.
				ctx.vmSnapshot.Labels = map[string]string{
					"app": "test",
				}
				obj, err := builder.ToUnstructured(ctx.vmSnapshot)
				Expect(err).ToNot(HaveOccurred())
				ctx.WebhookRequestContext.Obj = obj

				rawObj, err := json.Marshal(ctx.vmSnapshot)
				Expect(err).ToNot(HaveOccurred())
				ctx.WebhookRequestContext.RawObj = rawObj

				ctx.WebhookRequestContext.Op = admissionv1.Update
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())

				Expect(response.Patches).ToNot(BeEmpty())
			})
		})

		Context("when updating a snapshot that already has the VM name label", func() {
			It("should not mutate", func() {
				ctx.vmSnapshot.Labels = map[string]string{
					vmopv1.VMNameForSnapshotLabel: "dummy-vm",
				}
				obj, err := builder.ToUnstructured(ctx.vmSnapshot)
				Expect(err).ToNot(HaveOccurred())
				ctx.WebhookRequestContext.Obj = obj

				ctx.WebhookRequestContext.Op = admissionv1.Update
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())

				// Already labeled, so there is nothing to backfill.
				Expect(response.Patches).To(BeEmpty())
			})
		})
	})

	Describe("VirtualMachineSnapshotMutator should admit delete operations", func() {
		Context("when deleting snapshot", func() {
			It("should allow deletion without mutations", func() {
				ctx.WebhookRequestContext.Op = admissionv1.Delete
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())

				// Should not have any patches for delete operations
				Expect(response.Patches).To(BeEmpty())
			})
		})
	})

	Describe("VirtualMachineSnapshotMutator should handle object under deletion", func() {
		Context("when update request comes in while deletion in progress", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})
}
