// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx           *builder.IntegrationTestContext
		vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest
	)

	getVMGroupPubReq := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1.VirtualMachineGroupPublishRequest {
		req := &vmopv1.VirtualMachineGroupPublishRequest{}
		if err := ctx.Client.Get(ctx, objKey, req); err != nil {
			return nil
		}
		return req
	}

	getVMPubReqs := func(ctx *builder.IntegrationTestContext, ns string) []vmopv1.VirtualMachinePublishRequest {
		reqs := &vmopv1.VirtualMachinePublishRequestList{}
		Expect(ctx.Client.List(ctx, reqs, client.InNamespace(ns), client.MatchingLabels{
			vmopv1.VirtualMachinePublishRequestManagedByLabelKey: vmGroupPubReq.Name,
		})).NotTo(HaveOccurred())
		return reqs.Items
	}

	setVMPubReqRandomCondition := func(vmPubReq *vmopv1.VirtualMachinePublishRequest) {
		_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubReq, func() error {
			vmPubReqConditions := []string{
				vmopv1.VirtualMachinePublishRequestConditionSourceValid,
				vmopv1.VirtualMachinePublishRequestConditionTargetValid,
				vmopv1.VirtualMachinePublishRequestConditionUploaded,
				vmopv1.VirtualMachinePublishRequestConditionImageAvailable,
				vmopv1.VirtualMachinePublishRequestConditionComplete,
			}
			vmPubReqReasons := []string{
				vmopv1.SourceVirtualMachineNotExistReason,
				vmopv1.SourceVirtualMachineNotCreatedReason,
				vmopv1.TargetContentLibraryNotExistReason,
				vmopv1.TargetContentLibraryNotReadyReason,
				vmopv1.TargetContentLibraryNotWritableReason,
			}
			// mark condition to true does not impact reconciliation since the controller checks for status.ready
			updatedCondition := vmPubReqConditions[rand.Intn(len(vmPubReqConditions))]
			updatedReason := vmPubReqReasons[rand.Intn(len(vmPubReqReasons))]
			if rand.Int()%2 == 0 {
				conditions.MarkFalse(vmPubReq, updatedCondition, updatedReason, "")
			} else {
				conditions.MarkTrue(vmPubReq, updatedCondition)
			}

			return nil
		})
		Expect(err).ToNot(HaveOccurred())
	}

	setVMPubReqCompleted := func(vmPubReq *vmopv1.VirtualMachinePublishRequest) {
		_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, vmPubReq, func() error {
			vmPubReq.Status.Ready = true
			return nil
		})
		Expect(err).ToNot(HaveOccurred())
	}

	verifyDeletionEventually := func() {
		Eventually(func() []string {
			if req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq)); req != nil {
				return req.GetFinalizers()
			}
			return nil
		}).ShouldNot(ContainElement(finalizerName), "deletes finalizer")

		Eventually(func(g Gomega) {
			reqs := getVMPubReqs(ctx, vmGroupPubReq.Namespace)
			g.Expect(len(reqs)).To(Equal(0))
			req := &vmopv1.VirtualMachineGroupPublishRequest{}
			err := ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmGroupPubReq), req)
			g.Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
		}).Should(Succeed(), "deletes vm group publish request and its child vm publish requests")
	}

	verifyFalseConditionEventually := func(expectedPendingCnt int) {
		Eventually(func(g Gomega) {
			req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq))
			g.Expect(req).ToNot(BeNil())
			verifyFalseCondition(g, req, expectedPendingCnt)
		}).Should(Succeed(), fmt.Sprintf("sets completed condition to false with %d pending", expectedPendingCnt))
	}

	verifyImageStatusEventually := func(expectedImages []vmopv1.VirtualMachineGroupPublishRequestImageStatus) {
		Eventually(func(g Gomega) {
			req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq))
			g.Expect(req).ToNot(BeNil())
			g.Expect(reflect.DeepEqual(req.Status.Images, expectedImages)).To(BeTrue())
		}).Should(Succeed(), "reflects owned vm publish requests' conditions to group request status.images")
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		vmGroupPubReq = builder.VirtualMachineGroupPublishRequest(builder.DummyVMGroupPublishRequestName,
			ctx.Namespace, []string{builder.DummyVirtualMachine0Name, builder.DummyVirtualMachine1Name})
		Expect(ctx.Client.Create(ctx, vmGroupPubReq)).To(Succeed())

		Eventually(func() []string {
			if req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq)); req != nil {
				return req.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizerName), "adds finalizer")
	})
	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmGroupPubReq = nil
	})

	Context("ReconcileNormal", func() {
		It("simulates VirtualMachineGroupPublishRequest completion workflow", func() {

			Eventually(func(g Gomega) {
				req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq))
				Expect(req).ToNot(BeNil())
				g.Expect(req.Status.StartTime).ToNot(Equal(metav1.Time{}))
			}).Should(Succeed(), "sets start time")

			Eventually(func(g Gomega) {
				reqs := getVMPubReqs(ctx, vmGroupPubReq.Namespace)
				Expect(reqs).To(HaveLen(len(vmGroupPubReq.Spec.VirtualMachines)))
			}).Should(Succeed(), "creates vm publish requests for each of the vms")

			// reconcile re-queued since last vm publish requests creations returns requeue error
			verifyFalseConditionEventually(len(vmGroupPubReq.Spec.VirtualMachines))

			reqs := getVMPubReqs(ctx, vmGroupPubReq.Namespace)
			Expect(reqs).To(HaveLen(len(vmGroupPubReq.Spec.VirtualMachines)))

			// update vm publish request condition should reflect changes to vm group publish request status images[]
			setVMPubReqRandomCondition(&reqs[0])
			setVMPubReqRandomCondition(&reqs[1])
			Eventually(func(g Gomega) {
				reqs := getVMPubReqs(ctx, vmGroupPubReq.Namespace)
				Expect(reqs).To(HaveLen(len(vmGroupPubReq.Spec.VirtualMachines)))
				verifyImageStatusEventually(constructStatusImages(reqs))
			}).Should(Succeed(), "sets completed condition to true")

			// mark one of the vm publish requests to be true
			setVMPubReqCompleted(&reqs[0])
			verifyFalseConditionEventually(len(vmGroupPubReq.Spec.VirtualMachines) - 1)

			// mark last of the vm publish requests to be true
			setVMPubReqCompleted(&reqs[1])
			Eventually(func(g Gomega) {
				req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq))
				Expect(req).ToNot(BeNil())
				verifyTrueCondition(g, req)
			}).Should(Succeed(), "sets completed condition to true")

			// sets ttl to a future time
			req := getVMGroupPubReq(ctx, client.ObjectKeyFromObject(vmGroupPubReq))
			Expect(req).ToNot(BeNil())
			Expect(req.Spec.TTLSecondsAfterFinished).To(BeNil())
			_, err := controllerutil.CreateOrPatch(ctx, ctx.Client, req, func() error {
				newTTL := int64(time.Since(req.Status.CompletionTime.Time) + 10*time.Second)
				req.Spec.TTLSecondsAfterFinished = &newTTL
				return nil
			})
			Expect(err).ToNot(HaveOccurred())

			verifyDeletionEventually()
		})
	})

	Context("ReconcileDelete", func() {
		It("simulates VirtualMachineGroupPublishRequest deletion workflow", func() {
			// ensure there are still pending vm publish requests
			verifyFalseConditionEventually(len(vmGroupPubReq.Spec.VirtualMachines))
			Expect(ctx.Client.Delete(ctx, vmGroupPubReq)).To(Succeed())
			verifyDeletionEventually()
		})
	})
}

func verifyFalseCondition(g Gomega, req *vmopv1.VirtualMachineGroupPublishRequest, expectedPendingCnt int) {
	g.Expect(req.Status.Conditions).To(HaveLen(1))
	g.Expect(req.Status.Conditions[0].Type).To(Equal(vmopv1.VirtualMachinePublishRequestConditionComplete))
	g.Expect(req.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(req.Status.Conditions[0].Reason).To(Equal(vmopv1.VirtualMachineGroupPublishRequestConditionReasonPending))
	g.Expect(req.Status.Conditions[0].Message).To(
		Equal(fmt.Sprintf("waiting %d more vm publish requests to be completed", expectedPendingCnt)))
}

func verifyTrueCondition(g Gomega, req *vmopv1.VirtualMachineGroupPublishRequest) {
	g.Expect(req.Status.Conditions).To(HaveLen(1))
	g.Expect(req.Status.Conditions[0].Type).To(Equal(vmopv1.VirtualMachinePublishRequestConditionComplete))
	g.Expect(req.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(req.Status.CompletionTime).ToNot(Equal(metav1.Time{}))
}

func constructStatusImages(
	vmPubReqs []vmopv1.VirtualMachinePublishRequest) []vmopv1.VirtualMachineGroupPublishRequestImageStatus {
	imageStatuses := make([]vmopv1.VirtualMachineGroupPublishRequestImageStatus, 0, len(vmPubReqs))
	for _, req := range vmPubReqs {
		imageStatuses = append(imageStatuses, vmopv1.VirtualMachineGroupPublishRequestImageStatus{
			Source:             req.Spec.Source.Name,
			PublishRequestName: req.Name,
			ImageName:          req.Status.ImageName,
			Conditions:         req.Status.Conditions,
		})
	}
	return slices.SortedFunc(slices.Values(imageStatuses),
		func(a, b vmopv1.VirtualMachineGroupPublishRequestImageStatus) int {
			return strings.Compare(a.Source, b.Source)
		})
}
