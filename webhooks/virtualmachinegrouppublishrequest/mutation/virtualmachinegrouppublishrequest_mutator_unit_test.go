// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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
	vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vmGroupPubReq := &vmopv1.VirtualMachineGroupPublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: builder.DummyVMGroupPublishRequestName,
		},
	}
	obj, err := builder.ToUnstructured(vmGroupPubReq)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vmGroupPubReq:                     vmGroupPubReq,
	}
}

func unitTestsMutating() {
	var (
		ctx     *unitMutationWebhookContext
		vmGroup *vmopv1.VirtualMachineGroup
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
		ctx.Op = admissionv1.Create
		rawObj, err := json.Marshal(ctx.vmGroupPubReq)
		Expect(err).ToNot(HaveOccurred())
		ctx.RawObj = rawObj
	})
	AfterEach(func() {
		ctx = nil
	})

	When("there is a default content library and a VM publish group with the same name", func() {
		BeforeEach(func() {
			vmGroup = &vmopv1.VirtualMachineGroup{
				ObjectMeta: metav1.ObjectMeta{Namespace: ctx.vmGroupPubReq.Namespace, Name: ctx.vmGroupPubReq.Name}}
			Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
				builder.DummyContentLibraryName, ctx.vmGroupPubReq.Namespace, ""))).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmGroup)).To(Succeed())
			vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
				{Name: builder.DummyVirtualMachineName, Kind: "VirtualMachine"}}
			conditions.MarkTrue(&vmGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
			Expect(ctx.Client.Status().Update(ctx, vmGroup)).To(Succeed())
		})

		It("should mutate VirtualMachineGroupPublishRequest successfully", func() {
			response := ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Patches).To(HaveLen(3))
			for _, patch := range response.Patches {
				Expect(patch.Operation).To(Equal("add"))
				switch patch.Path {
				case "/spec/source":
					Expect(patch.Value).To(Equal(ctx.vmGroupPubReq.Name))
				case "/spec/target":
					Expect(patch.Value).To(Equal(builder.DummyContentLibraryName))
				case "/spec/virtualMachines":
					Expect(patch.Json()).To(ContainSubstring(builder.DummyVirtualMachineName))
				default:
					Fail(fmt.Sprintf("Unexpected patch path: %s", patch.Path))
				}
			}
		})

		It("should mutate error", func() {
			vmGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
				{
					Kind: "Unknown",
				},
			}
			Expect(ctx.Client.Status().Update(ctx, vmGroup)).To(Succeed())
			Expect(ctx.Mutate(&ctx.WebhookRequestContext).Allowed).To(BeFalse())
		})
	})

	When("the operation is not CREATE", func() {
		nonCreateOperation := func(op admissionv1.Operation) bool {
			ctx.Op = op
			return ctx.Mutate(&ctx.WebhookRequestContext).Allowed
		}
		It("should mutate VirtualMachineGroupPublishRequest successfully", func() {
			Expect(nonCreateOperation(admissionv1.Update)).To(BeTrue())
			Expect(nonCreateOperation(admissionv1.Delete)).To(BeTrue())
			Expect(nonCreateOperation(admissionv1.Connect)).To(BeTrue())
		})
	})
}
