// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegrouppublishrequest_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinegrouppublishrequest"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizerName = "vmoperator.vmware.com/virtualmachinegrouppublishrequest"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		), unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects       []client.Object
		ctx               *builder.UnitTestContextForController
		reconciler        *virtualmachinegrouppublishrequest.Reconciler
		vmGroupPubReq     *vmopv1.VirtualMachineGroupPublishRequest
		vmpGroupPubReqCtx *pkgctx.VirtualMachineGroupPublishRequestContext
	)

	BeforeEach(func() {
		vmGroupPubReq = builder.DummyVirtualMachineGroupPublishRequest(
			builder.DummyVMGroupPublishRequestName,
			builder.DummyNamespaceName,
			[]string{builder.DummyVirtualMachineName + "-0", builder.DummyVirtualMachineName + "-1"})
		controllerutil.AddFinalizer(vmGroupPubReq, finalizerName)
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachinegrouppublishrequest.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		vmpGroupPubReqCtx = &pkgctx.VirtualMachineGroupPublishRequestContext{
			Context:               ctx,
			Logger:                ctx.Logger.WithName(vmGroupPubReq.Name),
			VMGroupPublishRequest: vmGroupPubReq,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		vmGroupPubReq = nil
		vmpGroupPubReqCtx = nil
	})

	Context("ReconcileDelete", func() {
		When("object does have finalizer set", func() {
			It("should delete finalizer", func() {
				_, err := reconciler.ReconcileDelete(vmpGroupPubReqCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmGroupPubReq.GetFinalizers()).ToNot(ContainElement(finalizerName))
				getVMPublishRequests(vmGroupPubReq, reconciler, 0)
			})
		})
	})

	Context("ReconcileNormal", func() {

		When("object does not have finalizer set", func() {
			It("should set finalizer", func() {
				controllerutil.RemoveFinalizer(vmGroupPubReq, finalizerName)
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).NotTo(HaveOccurred())
				Expect(vmGroupPubReq.GetFinalizers()).To(ContainElement(finalizerName))
			})
		})

		When("object does not have spec optional fields with default values set properly", func() {
			var (
				specErr error
			)
			AfterEach(func() {
				Expect(vmGroupPubReq.Status.Conditions).To(HaveLen(1))
				Expect(vmGroupPubReq.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
				Expect(specErr).To(Equal(pkgerr.NoRequeueError{
					Message: "webhooks failed to mutate/validate spec. please delete and create again.",
				}))
			})
			BeforeEach(func() {
				specErr = nil
			})

			It("should set complete condition false when spec source is missing", func() {
				vmGroupPubReq.Spec.Source = ""
				specErr = reconciler.ReconcileNormal(vmpGroupPubReqCtx)
				Expect(vmGroupPubReq.Status.Conditions[0].Message).To(ContainSubstring("spec.source is undefined"))
			})

			It("should set complete condition false when spec target is missing", func() {
				vmGroupPubReq.Spec.Target = ""
				specErr = reconciler.ReconcileNormal(vmpGroupPubReqCtx)
				Expect(vmGroupPubReq.Status.Conditions[0].Message).To(ContainSubstring("spec.target is undefined"))
			})

			It("should set complete condition false when spec virtualMachines is missing", func() {
				vmGroupPubReq.Spec.VirtualMachines = nil
				specErr = reconciler.ReconcileNormal(vmpGroupPubReqCtx)
				Expect(vmGroupPubReq.Status.Conditions[0].Message).To(ContainSubstring("spec.virtualMachines is undefined"))
			})

			It("should set complete condition false when spec source and spec target is missing", func() {
				vmGroupPubReq.Spec.Source = ""
				vmGroupPubReq.Spec.Target = ""
				specErr = reconciler.ReconcileNormal(vmpGroupPubReqCtx)
				Expect(vmGroupPubReq.Status.Conditions[0].Message).To(ContainSubstring("spec.source is undefined"))
				Expect(vmGroupPubReq.Status.Conditions[0].Message).To(ContainSubstring("spec.target is undefined"))
			})
		})

		When("there is no vm publish requests exist", func() {
			It("should create two dummy vm publish requests", func() {
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(Succeed())
				Expect(vmGroupPubReq.Status.StartTime).ToNot(Equal(metav1.Time{}))
				reqs := getVMPublishRequests(vmGroupPubReq, reconciler, len(vmGroupPubReq.Spec.VirtualMachines))
				verifyRequest(reqs[0], vmGroupPubReq)
				verifyRequest(reqs[1], vmGroupPubReq)
			})
		})

		When("there is a pending vm publish request", func() {
			It("should create other vm publish requests", func() {
				// first reconcile normal creates the vm requests
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(Succeed())
				reqs := getVMPublishRequests(vmGroupPubReq, reconciler, len(vmGroupPubReq.Spec.VirtualMachines))
				Expect(reconciler.Delete(vmpGroupPubReqCtx, &reqs[0])).To(Succeed())
				getVMPublishRequests(vmGroupPubReq, reconciler, len(vmGroupPubReq.Spec.VirtualMachines)-1)
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(Succeed())
				getVMPublishRequests(vmGroupPubReq, reconciler, len(vmGroupPubReq.Spec.VirtualMachines))
			})
		})

		When("all vm publish requests are created", func() {
			It("should update the completed condition to False with waiting for completion message", func() {
				// first reconcile normal creates the vm requests
				// second reconcile enters reconcileStatusCompletedCondition()
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(Succeed())
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(Succeed())
				Eventually(func(g Gomega) {
					verifyFalseCondition(g, vmGroupPubReq, len(vmGroupPubReq.Spec.VirtualMachines))
				}).Should(Succeed(), fmt.Sprintf("sets completed condition to false with %d pending",
					len(vmGroupPubReq.Spec.VirtualMachines)))
			})
		})

		When("vm group publish request is completed", func() {
			BeforeEach(func() {
				conditions.MarkTrue(vmGroupPubReq, vmopv1.VirtualMachineGroupPublishRequestConditionComplete)
			})
			It("should return nil when ttl is nil", func() {
				Expect(reconciler.ReconcileNormal(vmpGroupPubReqCtx)).To(BeNil())
			})
		})
	})
}

func getVMPublishRequests(
	vmGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest,
	reconciler *virtualmachinegrouppublishrequest.Reconciler,
	expectedPendingCnt int) []vmopv1.VirtualMachinePublishRequest {
	reqs := &vmopv1.VirtualMachinePublishRequestList{}
	Expect(reconciler.List(reconciler.Context,
		reqs,
		client.InNamespace(vmGroupPubReq.Namespace),
		client.MatchingLabels{
			vmopv1.VirtualMachinePublishRequestManagedByLabelKey: vmGroupPubReq.Name,
		})).NotTo(HaveOccurred())
	Expect(reqs.Items).To(HaveLen(expectedPendingCnt))
	return reqs.Items
}

func verifyRequest(req vmopv1.VirtualMachinePublishRequest, vmGroupPub *vmopv1.VirtualMachineGroupPublishRequest) {
	Expect(req.Name).To(ContainSubstring(vmGroupPub.Name))
	Expect(metav1.HasLabel(req.ObjectMeta, vmopv1.VirtualMachinePublishRequestManagedByLabelKey)).To(BeTrue())
	Expect(req.Labels).To(HaveKeyWithValue(vmopv1.VirtualMachinePublishRequestManagedByLabelKey, vmGroupPub.Name))
	Expect(metav1.IsControlledBy(&req, vmGroupPub)).To(BeTrue())
	Expect(req.Spec.Source.Kind).To(Equal("VirtualMachine"))
	Expect(req.Spec.Source.Name).To(ContainSubstring("dummy-vm"))
	Expect(req.Spec.Target.Location.Name).To(Equal(vmGroupPub.Spec.Target))
	Expect(req.Spec.Target.Location.Kind).To(Equal("ContentLibrary"))
}
