// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgctxfake "github.com/vmware-tanzu/vm-operator/pkg/context/fake"
)

type requestContextTestKey struct{}

var _ = Describe("NewWebhookRequestContext", Label(testlabels.Webhook), func() {
	var (
		parentCtx    context.Context
		cancelParent context.CancelFunc
		httpCtx      context.Context
		cancelHTTP   context.CancelCauseFunc
		webhookCtx   *pkgctx.WebhookContext
		req          admission.Request
		obj          *unstructured.Unstructured
		oldObj       *unstructured.Unstructured

		reqCtx    *pkgctx.WebhookRequestContext
		cancelReq context.CancelFunc
	)

	BeforeEach(func() {
		webhookCtx = pkgctxfake.NewWebhookContext(
			pkgctxfake.NewControllerManagerContext())

		// Use a cancelable parent that carries a manager-scoped value so the
		// tests can observe both value and cancellation propagation.
		parentCtx, cancelParent = context.WithCancel(
			context.WithValue(
				webhookCtx.Context,
				requestContextTestKey{},
				"manager-value"))
		webhookCtx.Context = parentCtx

		httpCtx, cancelHTTP = context.WithCancelCause(context.Background())

		obj = &unstructured.Unstructured{}
		obj.SetNamespace("my-namespace")
		obj.SetName("my-obj")
		oldObj = nil

		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: []byte(`{"a":"b"}`)},
				UserInfo:  authv1.UserInfo{Username: "my-user"},
			},
		}
	})

	JustBeforeEach(func() {
		reqCtx, cancelReq = builder.NewWebhookRequestContext(
			httpCtx,
			webhookCtx,
			req,
			obj,
			oldObj)
	})

	AfterEach(func() {
		cancelReq()
		cancelHTTP(nil)
		cancelParent()
	})

	It("populates the request fields", func() {
		Expect(reqCtx.WebhookContext).To(BeIdenticalTo(webhookCtx))
		Expect(reqCtx.Op).To(Equal(admissionv1.Update))
		Expect(reqCtx.Obj).To(BeIdenticalTo(obj))
		Expect(reqCtx.OldObj).To(BeNil())
		Expect(reqCtx.RawObj).To(Equal(req.Object.Raw))
		Expect(reqCtx.UserInfo).To(Equal(req.UserInfo))
		Expect(reqCtx.IsPrivilegedAccount).To(BeFalse())
		Expect(reqCtx.IsVMOperatorAccount).To(BeFalse())
	})

	When("only the old object is set", func() {
		BeforeEach(func() {
			oldObj, obj = obj, nil
		})
		It("does not panic and keeps the old object", func() {
			Expect(reqCtx.Obj).To(BeNil())
			Expect(reqCtx.OldObj).To(BeIdenticalTo(oldObj))
		})
	})

	It("carries the values of the webhook's long-lived context", func() {
		Expect(reqCtx.Value(requestContextTestKey{})).To(Equal("manager-value"))
		Expect(func() { _ = pkgcfg.FromContext(reqCtx) }).ToNot(Panic())
	})

	It("does not change the webhook context when the request context is replaced", func() {
		reqCtx.Context = context.WithValue(
			reqCtx.Context, requestContextTestKey{}, "request-value")

		Expect(reqCtx.Value(requestContextTestKey{})).To(Equal("request-value"))
		Expect(webhookCtx.Context).To(BeIdenticalTo(parentCtx))
		Expect(webhookCtx.Value(requestContextTestKey{})).To(Equal("manager-value"))
	})

	It("is canceled with the HTTP cause when the HTTP request is done", func() {
		Expect(reqCtx.Err()).ToNot(HaveOccurred())

		cause := errors.New("client went away")
		cancelHTTP(cause)

		Eventually(reqCtx.Done()).Should(BeClosed())
		Expect(context.Cause(reqCtx)).To(MatchError(cause))
		Expect(parentCtx.Err()).ToNot(HaveOccurred())
	})

	When("the HTTP request has a deadline", func() {
		var deadline time.Time

		BeforeEach(func() {
			deadline = time.Now().Add(time.Hour)
			var cancel context.CancelFunc
			httpCtx, cancel = context.WithDeadline(httpCtx, deadline)
			DeferCleanup(cancel)
		})

		It("applies the deadline", func() {
			d, ok := reqCtx.Deadline()
			Expect(ok).To(BeTrue())
			Expect(d).To(Equal(deadline))
		})
	})

	It("is canceled when the webhook's long-lived context is canceled", func() {
		cancelParent()

		Eventually(reqCtx.Done()).Should(BeClosed())
		Expect(httpCtx.Err()).ToNot(HaveOccurred())
	})

	It("is canceled by the returned cancel function without canceling its parents", func() {
		cancelReq()

		Expect(reqCtx.Done()).To(BeClosed())
		Expect(reqCtx.Err()).To(MatchError(context.Canceled))
		Expect(parentCtx.Err()).ToNot(HaveOccurred())
		Expect(httpCtx.Err()).ToNot(HaveOccurred())
	})
})
