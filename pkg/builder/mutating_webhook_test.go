// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
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

var _ = Describe("NewMutatingWebhook", Serial, func() {

	var (
		err        error
		wh         *pkgbuilder.MutatingWebhook
		whName     string
		whCallback pkgbuilder.Mutator
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
		whCallback = &fakeMutator{}
	})

	AfterEach(func() {
		err = nil
		wh = nil
		res = admission.Response{}
	})

	JustBeforeEach(func() {
		if whCallback != nil {
			whCallback.(*fakeMutator).gvk = obj.GetObjectKind().GroupVersionKind()
			whCallback.(*fakeMutator).res = res
		}
		wh, err = pkgbuilder.NewMutatingWebhook(
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
		When("mutator is nil", func() {
			BeforeEach(func() {
				whCallback = nil
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("mutator arg is nil"))
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

		assertNotOkay := func(req admission.Request, code int32, msg string) {
			r := wh.Handle(ctx, req)
			ExpectWithOffset(1, r.Result).ToNot(BeNil())
			ExpectWithOffset(1, r.Result.Message).To(Equal(msg))
			ExpectWithOffset(1, r.Result.Code).To(Equal(code))
			ExpectWithOffset(1, r.Allowed).To(BeFalse())
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
			Context("certificate verification", func() {
				BeforeEach(func() {
					dir := filepath.Dir(caFilePath)
					err := os.MkdirAll(dir, 0755)
					Expect(err).NotTo(HaveOccurred())

					err = os.WriteFile(caFilePath, []byte(sampleCert), 0644)
					Expect(err).NotTo(HaveOccurred())

					ctx.EnableWebhookClientVerification = true
				})

				AfterEach(func() {
					err := os.RemoveAll(caFilePath)
					Expect(err).NotTo(HaveOccurred())

					err = os.RemoveAll("/tmp/k8s-webhook-server")
					Expect(err).NotTo(HaveOccurred())
				})

				When("request has invalid peer certs", func() {
					It("should return an error", func() {
						block, _ := pem.Decode([]byte(invalidClientCert))
						clientCert, err := x509.ParseCertificate(block.Bytes)
						Expect(err).NotTo(HaveOccurred())

						ctx.Context = context.WithValue(ctx.Context, pkgbuilder.RequestClientCertificateContextKey, &tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{clientCert},
						})
						assertNotOkay(getAdmissionRequest(obj, admissionv1.Create), http.StatusBadRequest, "unauthorized client CN: other-client")
					})
				})

				When("request has valid peer certs", func() {
					BeforeEach(func() {
						res = getAdmissionResponseOkay()
					})
					It("should not return an error", func() {
						block, _ := pem.Decode([]byte(sampleClientCert))
						clientCert, err := x509.ParseCertificate(block.Bytes)
						Expect(err).NotTo(HaveOccurred())

						ctx.Context = context.WithValue(ctx.Context, pkgbuilder.RequestClientCertificateContextKey, &tls.ConnectionState{
							PeerCertificates: []*x509.Certificate{clientCert},
						})
						assertOkay(getAdmissionRequest(obj, admissionv1.Create), "")
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

type fakeMutator struct {
	gvk schema.GroupVersionKind
	res admission.Response
}

func (f fakeMutator) For() schema.GroupVersionKind {
	return f.gvk
}

func (f fakeMutator) Mutate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	return f.res
}
