// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgctxfake "github.com/vmware-tanzu/vm-operator/pkg/context/fake"
)

var _ = Describe("NewValidatingWebhook", func() {

	var (
		err        error
		wh         *pkgbuilder.ValidatingWebhook
		whName     string
		whCallback pkgbuilder.Validator
		ctx        *pkgctx.ControllerManagerContext
		obj        metaRuntimeObj
		res        admission.Response // used in webhook callback response
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = pkgctxfake.NewControllerManagerContext()
		obj = getConfigMap()
		scheme = runtime.NewScheme()
		whName = "fake"
		whCallback = &fakeValidator{}
	})

	AfterEach(func() {
		err = nil
		wh = nil
		res = admission.Response{}
	})

	JustBeforeEach(func() {
		if whCallback != nil {
			whCallback.(*fakeValidator).gvk = obj.GetObjectKind().GroupVersionKind()
			whCallback.(*fakeValidator).res = res
		}
		wh, err = pkgbuilder.NewValidatingWebhook(
			ctx,
			fakeManager{scheme: scheme},
			whName,
			whCallback)
	})

	When("args are invalid", func() {
		When("webhookName is empty", func() {
			BeforeEach(func() {
				whName = ""
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("webhookName arg is empty"))
				Expect(wh).To(BeNil())
			})
		})
		When("validator is nil", func() {
			BeforeEach(func() {
				whCallback = nil
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("validator arg is nil"))
				Expect(wh).To(BeNil())
			})
		})
	})

	When("args are valid", func() {

		assertOkay := func(req admission.Request, msg string) {
			r := wh.Handle(ctx, req)
			ExpectWithOffset(1, r.Result).ToNot(BeNil())
			ExpectWithOffset(1, r.Result.Message).To(Equal(msg))
			ExpectWithOffset(1, r.Result.Code).To(Equal(int32(200)))
			ExpectWithOffset(1, r.Allowed).To(BeTrue())
		}

		assertInvalidChar := func(req admission.Request) {
			r := wh.Handle(ctx, req)
			ExpectWithOffset(1, r.Result).ToNot(BeNil())
			ExpectWithOffset(1, r.Result.Message).To(Equal(`invalid character '\x00' looking for beginning of value`))
			ExpectWithOffset(1, r.Result.Code).To(Equal(int32(400)))
			ExpectWithOffset(1, r.Allowed).To(BeFalse())
		}

		It("should return a new webhook handler", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(wh).ToNot(BeNil())
		})

		Context("Handle", func() {
			Context("unknown op", func() {
				When("input is valid", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should return true", func() {
						req := getAdmissionRequest(obj, admissionv1.Create)
						req.Operation = admissionv1.Operation("unknown")
						assertOkay(req, "unknown")
					})
				})

				When("input obj is invalid", func() {
					It("should return an error", func() {
						req := getAdmissionRequest(obj, admissionv1.Create)
						req.AdmissionRequest.Object.Raw = []byte{0}
						assertInvalidChar(req)
					})
				})
			})

			Context("create", func() {
				When("input is valid", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should return true", func() {
						assertOkay(getAdmissionRequest(obj, admissionv1.Create), "")
					})
				})

				When("input obj is invalid", func() {
					It("should return an error", func() {
						req := getAdmissionRequest(obj, admissionv1.Create)
						req.AdmissionRequest.Object.Raw = []byte{0}
						assertInvalidChar(req)
					})
				})
			})

			Context("update", func() {
				When("input is valid", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should return true", func() {
						assertOkay(getAdmissionRequest(obj, admissionv1.Update), "")
					})
				})

				When("input is valid with finalizer", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should return true", func() {
						obj.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
						req := getAdmissionRequest(obj, admissionv1.Update)
						assertOkay(req, "Update is allowed during deletion in order to remove the finalizers.")
					})
				})

				When("input obj is invalid", func() {
					It("should return an error", func() {
						req := getAdmissionRequest(obj, admissionv1.Update)
						req.AdmissionRequest.Object.Raw = []byte{0}
						assertInvalidChar(req)
					})
				})

				When("input oldObj is invalid", func() {
					It("should return an error", func() {
						req := getAdmissionRequest(obj, admissionv1.Update)
						req.AdmissionRequest.OldObject.Raw = []byte{0}
						assertInvalidChar(req)
					})
				})
			})

			Context("delete", func() {
				When("input is valid", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should return true", func() {
						assertOkay(getAdmissionRequest(obj, admissionv1.Delete), "")
					})
				})

				When("input oldObj is invalid", func() {
					It("should return an error", func() {
						req := getAdmissionRequest(obj, admissionv1.Delete)
						req.AdmissionRequest.OldObject.Raw = []byte{0}
						assertInvalidChar(req)
					})
				})
			})
		})
	})

})

type fakeValidator struct {
	gvk schema.GroupVersionKind
	res admission.Response
}

func (f fakeValidator) For() schema.GroupVersionKind {
	return f.gvk
}

func (f fakeValidator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return f.res
}

func (f fakeValidator) ValidateDelete(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return f.res
}

func (f fakeValidator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return f.res
}
