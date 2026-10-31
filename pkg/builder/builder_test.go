// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgctxfake "github.com/vmware-tanzu/vm-operator/pkg/context/fake"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// contextWebhook exercises the same context override used by VM validation.
type contextWebhook struct {
	callback func(*pkgctx.WebhookRequestContext) admission.Response
}

func (w contextWebhook) For() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("ConfigMap")
}

func (w contextWebhook) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return w.callback(ctx)
}

func (w contextWebhook) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return w.callback(ctx)
}

func (w contextWebhook) ValidateDelete(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return w.callback(ctx)
}

func (w contextWebhook) Mutate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return w.callback(ctx)
}

func newContextWebhook(ctx *pkgctx.ControllerManagerContext, mutating bool,
	callback func(*pkgctx.WebhookRequestContext) admission.Response) *admission.Webhook {

	mgr := fakeManager{scheme: runtime.NewScheme()}
	w := contextWebhook{callback: callback}
	if mutating {
		wh, err := pkgbuilder.NewMutatingWebhook(ctx, mgr, "context", w)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return &wh.Webhook
	}
	wh, err := pkgbuilder.NewValidatingWebhook(ctx, mgr, "context", w)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return &wh.Webhook
}

func overrideWorkloadIsolation(ctx *pkgctx.WebhookRequestContext) {
	vm := &vmopv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Labels: ctx.Obj.GetLabels()}}
	ctx.Context = vmopv1util.GetContextWithWorkloadDomainIsolation(ctx.Context, vm)
}

type cancellationClient struct {
	ctrlclient.Client
	started chan struct{}
}

func (c cancellationClient) Get(ctx context.Context, _ ctrlclient.ObjectKey,
	_ ctrlclient.Object, _ ...ctrlclient.GetOption) error {

	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}

var _ = Describe("Webhook request context", Label(testlabels.Webhook), func() {
	for _, mutating := range []bool{false, true} {
		name := "validating"
		if mutating {
			name = "mutating"
		}
		Context(name, func() {
			DescribeTable("isolates sequential requests, including rejected requests",
				func(op admissionv1.Operation) {
					manager := pkgctxfake.NewControllerManagerContext()
					wh := newContextWebhook(manager, mutating, func(ctx *pkgctx.WebhookRequestContext) admission.Response {
						overrideWorkloadIsolation(ctx)
						return admission.ValidationResponse(!pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation, "")
					})
					obj := getConfigMap()
					Expect(wh.Handle(context.Background(), getAdmissionRequest(obj, op)).Allowed).To(BeFalse())
					obj.Labels = map[string]string{vmopv1util.KubernetesNodeLabelKey: "cluster"}
					Expect(wh.Handle(context.Background(), getAdmissionRequest(obj, op)).Allowed).To(BeTrue())
					Expect(pkgcfg.FromContext(manager).Features.WorkloadDomainIsolation).To(BeFalse())

					// Requests must continue seeing changes to the manager configuration.
					pkgcfg.SetContext(manager, func(cfg *pkgcfg.Config) { cfg.Features.WorkloadDomainIsolation = true })
					Expect(wh.Handle(context.Background(), getAdmissionRequest(obj, op)).Allowed).To(BeFalse())
				},
				Entry("create", admissionv1.Create),
				Entry("update", admissionv1.Update),
			)

			It("isolates concurrent Kubernetes and non-Kubernetes requests", func() {
				const count = 32
				manager := pkgctxfake.NewControllerManagerContext()
				ready := make(chan struct{}, count)
				release := make(chan struct{})
				var once sync.Once
				DeferCleanup(func() { once.Do(func() { close(release) }) })
				wh := newContextWebhook(manager, mutating, func(ctx *pkgctx.WebhookRequestContext) admission.Response {
					overrideWorkloadIsolation(ctx)
					ready <- struct{}{}
					<-release
					_, node := ctx.Obj.GetLabels()[vmopv1util.KubernetesNodeLabelKey]
					return admission.ValidationResponse(pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation == !node, "")
				})
				results := make(chan bool, count)
				for i := range count {
					obj := getConfigMap()
					if i%2 == 0 {
						obj.Labels = map[string]string{vmopv1util.KubernetesNodeLabelKey: "cluster"}
					}
					req := getAdmissionRequest(obj, admissionv1.Create)
					go func() { results <- wh.Handle(context.Background(), req).Allowed }()
				}
				for range count {
					Eventually(ready).Should(Receive())
				}
				once.Do(func() { close(release) })
				for range count {
					Eventually(results).Should(Receive(BeTrue()))
				}
				Expect(pkgcfg.FromContext(manager).Features.WorkloadDomainIsolation).To(BeFalse())
			})

			It("preserves HTTP values and deadlines and cancels a blocked client call", func() {
				manager := pkgctxfake.NewControllerManagerContext()
				client := cancellationClient{started: make(chan struct{})}
				errs := make(chan error, 1)
				type requestKey struct{}
				deadline := time.Now().Add(time.Minute)
				parent := context.WithValue(context.Background(), requestKey{}, "request-value")
				httpCtx, cancel := context.WithDeadline(parent, deadline)
				defer cancel()
				wh := newContextWebhook(manager, mutating, func(ctx *pkgctx.WebhookRequestContext) admission.Response {
					defer GinkgoRecover()
					overrideWorkloadIsolation(ctx)
					Expect(ctx.Value(requestKey{})).To(Equal("request-value"))
					actual, ok := ctx.Deadline()
					Expect(ok).To(BeTrue())
					Expect(actual).To(Equal(deadline))
					errs <- client.Get(ctx, ctrlclient.ObjectKey{}, &corev1.ConfigMap{})
					return admission.Allowed("")
				})
				admissionReq := getAdmissionRequest(getConfigMap(), admissionv1.Create)
				body, err := json.Marshal(admissionv1.AdmissionReview{
					TypeMeta: metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "AdmissionReview"},
					Request:  &admissionReq.AdmissionRequest,
				})
				Expect(err).NotTo(HaveOccurred())
				req := httptest.NewRequestWithContext(httpCtx, http.MethodPost, "/", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				done := make(chan struct{})
				go func() {
					defer close(done)
					wh.ServeHTTP(httptest.NewRecorder(), req)
				}()
				Eventually(client.started).Should(BeClosed())
				cancel()
				Eventually(errs).Should(Receive(MatchError(context.Canceled)))
				Eventually(done).Should(BeClosed())
				Expect(manager.Err()).NotTo(HaveOccurred())
			})
		})
	}
})
