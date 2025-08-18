// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"fmt"
	"math/rand/v2"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
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
	vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmGroupPubReq = builder.DummyVirtualMachineGroupPublishRequest(
		builder.DummyVMGroupPublishRequestName,
		ctx.Namespace,
		[]string{builder.DummyVirtualMachineName + "-0", builder.DummyVirtualMachineName + "-1"})

	return ctx
}

func intgTestsMutating() {
	var (
		ctx               *intgMutatingWebhookContext
		mutatedReq        *vmopv1.VirtualMachineGroupPublishRequest
		expectedCreateErr *errors.StatusError
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		mutatedReq = &vmopv1.VirtualMachineGroupPublishRequest{}
		expectedCreateErr = nil
	})

	AfterEach(func() {
		if expectedCreateErr == nil {
			Expect(ctx.Client.Delete(ctx.Context, ctx.vmGroupPubReq)).To(Succeed())
		}
		ctx.AfterEach()
		ctx = nil
		mutatedReq = nil
		expectedCreateErr = nil
	})

	JustBeforeEach(func() {
		actualCreateErr := ctx.Client.Create(ctx.Context, ctx.vmGroupPubReq)
		if expectedCreateErr == nil {
			Expect(actualCreateErr).NotTo(HaveOccurred())
			Expect(ctx.Client.Get(ctx.Context, ctrlclient.ObjectKeyFromObject(ctx.vmGroupPubReq), mutatedReq)).To(Succeed())
		} else {
			Expect(actualCreateErr).To(MatchError(expectedCreateErr))
		}
	})

	When("all optional spec fields with default values are filled", func() {
		It("should allow without mutating any fields", func() {
			Expect(mutatedReq).To(Equal(ctx.vmGroupPubReq))
		})
	})

	When("all spec fields are filled", func() {
		var (
			randTTL int64
		)
		BeforeEach(func() {
			randTTL = rand.Int64()
			ctx.vmGroupPubReq.Spec.TTLSecondsAfterFinished = &randTTL
		})
		It("should allow without mutating any fields", func() {
			Expect(mutatedReq).To(Equal(ctx.vmGroupPubReq))
			Expect(mutatedReq.Spec.TTLSecondsAfterFinished).To(Equal(&randTTL))
		})
	})

	When("spec.source is omitted", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Source = ""
		})
		It("should allow with spec.source set to the same name", func() {
			Expect(mutatedReq.Spec.Source).To(Equal(ctx.vmGroupPubReq.Name))
		})
	})

	When("spec.target is omitted without default content library", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Target = ""
			expectedCreateErr = &errors.StatusError{
				ErrStatus: metav1.Status{
					Code:   http.StatusNotFound,
					Status: metav1.StatusFailure,
					Message: fmt.Sprintf("admission webhook %q denied the request: cannot find a default content library with the %q label",
						WebhookName, common.DefaultImagePublishContentLibraryLabelKey),
				},
			}
		})
		It("should deny with CL not found error", func() {})
	})

	When("spec.target is omitted with default content library", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Target = ""
			Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
				builder.DummyContentLibraryName, ctx.vmGroupPubReq.Namespace, ""))).To(Succeed())
		})
		It("should allow with spec.target set to default CL's name", func() {})
	})

	When("spec.target is omitted with multiple default content libraries", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.Target = ""
			Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
				builder.DummyContentLibraryName, ctx.vmGroupPubReq.Namespace, ""))).To(Succeed())
			Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
				builder.DummyContentLibraryName+"-0", ctx.vmGroupPubReq.Namespace, ""))).To(Succeed())
			expectedCreateErr = &errors.StatusError{
				ErrStatus: metav1.Status{
					Code:   http.StatusBadRequest,
					Status: metav1.StatusFailure,
					Message: fmt.Sprintf("admission webhook %q denied the request: "+
						"more than one default ContentLibrary found: dummy-cl, dummy-cl-0", WebhookName),
				},
			}
		})
		It("should deny with spec.target with duplicated CLs error", func() {})
	})

	When("spec.virtualMachines is omitted without the VirtualMachineGroup created", func() {
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.VirtualMachines = nil
			expectedCreateErr = &errors.StatusError{
				ErrStatus: metav1.Status{
					Code:   http.StatusNotFound,
					Status: metav1.StatusFailure,
					Message: fmt.Sprintf("admission webhook %q denied the request: %s %q not found",
						WebhookName, "VirtualMachineGroup.vmoperator.vmware.com", ctx.vmGroupPubReq.Spec.Source),
				},
			}
		})
		It("should deny with the VirtualMachineGroup not found error", func() {})
	})

	When("spec.virtualMachines is omitted with the VirtualMachineGroup created", func() {
		var (
			expectedVMs []string
		)
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.VirtualMachines = nil
			Eventually(func(g Gomega) {
				expectedVMs = setupVMGroups(g, &ctx.IntegrationTestContext, ctx.vmGroupPubReq.Spec.Source, ctx.Namespace)
			}).Should(Succeed())
		})
		It("should allow with the spec.virtualMachines set to all group members", func() {
			Expect(mutatedReq.Spec.VirtualMachines).To(Equal(expectedVMs))
		})
	})

	When("spec.virtualMachines and spec.source are omitted with the VirtualMachineGroup created", func() {
		var (
			expectedVMs []string
		)
		BeforeEach(func() {
			ctx.vmGroupPubReq.Spec.VirtualMachines = nil
			ctx.vmGroupPubReq.Spec.Source = ""
			Eventually(func(g Gomega) {
				expectedVMs = setupVMGroups(g, &ctx.IntegrationTestContext, ctx.vmGroupPubReq.Name, ctx.vmGroupPubReq.Namespace)
			}).Should(Succeed())
		})
		It("should allow with the spec.virtualMachines set to all group members and source set to the name", func() {
			Expect(mutatedReq.Spec.Source).To(Equal(ctx.vmGroupPubReq.Name))
			Expect(mutatedReq.Spec.VirtualMachines).To(Equal(expectedVMs))
		})
	})
}

func setupVMGroups(g Gomega, ctx *builder.IntegrationTestContext, name, namespace string) []string {
	vmNames := []string{
		builder.DummyVirtualMachineName + "-0",
		builder.DummyVirtualMachineName + "-1",
		builder.DummyVirtualMachineName + "-2",
		builder.DummyVirtualMachineName + "-3"}
	parentVMGroup := &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
	childVMGrop := &vmopv1.VirtualMachineGroup{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name + "-child"},
	}

	g.Expect(ctx.Client.Create(ctx, parentVMGroup)).To(Succeed())
	g.Expect(ctx.Client.Create(ctx, childVMGrop)).To(Succeed())

	childVMGrop.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
		{Name: vmNames[2], Kind: "VirtualMachine"},
		{Name: vmNames[0], Kind: "VirtualMachine"}}
	conditions.MarkTrue(&childVMGrop.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
	conditions.MarkTrue(&childVMGrop.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
	g.Expect(ctx.Client.Status().Update(ctx, childVMGrop)).To(Succeed())
	parentVMGroup.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
		{Name: vmNames[1], Kind: "VirtualMachine"},
		{Name: vmNames[3], Kind: "VirtualMachine"},
		{Name: childVMGrop.Name, Kind: "VirtualMachineGroup"}}
	conditions.MarkTrue(&parentVMGroup.Status.Members[0], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
	conditions.MarkTrue(&parentVMGroup.Status.Members[1], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
	conditions.MarkTrue(&parentVMGroup.Status.Members[2], vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
	g.Expect(ctx.Client.Status().Update(ctx, parentVMGroup)).To(Succeed())

	return vmNames
}
