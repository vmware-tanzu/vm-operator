// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validatingwebhookconfiguration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/validatingwebhookconfiguration"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const (
		secretName      = "vmware-system-vmop-serving-cert"
		secretNamespace = "vmware-system-vmop"
	)
	var (
		ctx         *builder.UnitTestContextForController
		withObjects []client.Object

		reconciler *validatingwebhookconfiguration.Reconciler

		certSecret                     *corev1.Secret
		validatingWebhookConfiguration *admissionv1.ValidatingWebhookConfiguration
		spu                            *spqv1.StoragePolicyUsage
	)

	BeforeEach(func() {
		withObjects = []client.Object{}

		certSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
			Data: map[string][]byte{
				"ca.crt": []byte("fake-ca-bundle"),
			},
		}

		spu = &spqv1.StoragePolicyUsage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-spu-name",
				Namespace: "fake-spu-namespace",
			},
			Spec: spqv1.StoragePolicyUsageSpec{
				ResourceExtensionName: spqutil.StoragePolicyQuotaExtensionName,
			},
		}

		validatingWebhookConfiguration = &admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: spqutil.ValidatingWebhookConfigName,
				Annotations: map[string]string{
					"cert-manager.io/inject-ca-from": "vmware-system-vmop/vmware-system-vmop-serving-cert",
				},
			},
			Webhooks: []admissionv1.ValidatingWebhook{},
		}
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, certSecret, validatingWebhookConfiguration, spu)
		ctx = suite.NewUnitTestContextForController(withObjects...)
		reconciler = validatingwebhookconfiguration.NewReconciler(ctx, ctx.Client, ctx.Logger, ctx.Recorder)
	})

	Context("Reconcile", func() {
		var (
			err  error
			name string
		)

		BeforeEach(func() {
			err = nil
			name = spqutil.ValidatingWebhookConfigName
		})

		JustBeforeEach(func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: name,
				}})
		})

		When("Reconcile is called with invalid name", func() {
			BeforeEach(func() {
				name = "fakeName"
			})

			It("should return nil error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("ValidatingWebhookConfiguration is not found", func() {
			errMsg := `validatingwebhookconfigurations.admissionregistration.k8s.io "vmware-system-vmop-validating-webhook-configuration" not found`

			BeforeEach(func() {
				validatingWebhookConfiguration = &admissionv1.ValidatingWebhookConfiguration{}
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errMsg))
			})
		})

		When("certificate annotation is not present", func() {
			BeforeEach(func() {
				delete(validatingWebhookConfiguration.Annotations, "cert-manager.io/inject-ca-from")
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get annotation for key"))
			})
		})

		When("certificate annotation is present with empty value", func() {
			BeforeEach(func() {
				validatingWebhookConfiguration.Annotations["cert-manager.io/inject-ca-from"] = ""
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get annotation for key"))
			})
		})

		When("certificate annotation is present and missing namespace", func() {
			BeforeEach(func() {
				validatingWebhookConfiguration.Annotations["cert-manager.io/inject-ca-from"] = "vmware-system-vmop-serving-cert"
			})

			It("should not return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get namespace and name for key"))
			})
		})

		When("certificate Secret is not found", func() {
			BeforeEach(func() {
				certSecret.Name = "fake"
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`secrets "vmware-system-vmop-serving-cert" not found`))
			})
		})

		When("secret is missing ca.crt", func() {
			BeforeEach(func() {
				delete(certSecret.Data, "ca.crt")
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get CA bundle from secret"))
			})
		})

		When("ca.crt is empty", func() {
			BeforeEach(func() {
				certSecret.Data["ca.crt"] = []byte("")
			})

			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get CA bundle from secret"))
			})
		})

		Context("update SPU docs", func() {
			JustBeforeEach(func() {
				Expect(err).To(BeNil())
				err = ctx.Client.Get(ctx, client.ObjectKeyFromObject(spu), spu)

				Expect(err).To(BeNil())
			})

			When("no SPU docs exist for vmservice", func() {
				BeforeEach(func() {
					spu.Spec.ResourceExtensionName = "fake.cns.vsphere.vmware.com"
					spu.Spec.CABundle = []byte("fake.cns.vsphere.vmware.com")
				})

				It("should not update caBundle", func() {
					Expect(spu.Spec.CABundle).To(Equal([]byte("fake.cns.vsphere.vmware.com")))
				})
			})

			When("SPU caBundle matches contents of secret", func() {
				BeforeEach(func() {
					spu.Spec.CABundle = certSecret.Data["ca.crt"]
				})

				It("should not update caBundle", func() {
					Expect(spu.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
				})
			})

			When("SPU caBundle does not match contents of secret", func() {
				BeforeEach(func() {
					spu.Spec.CABundle = []byte("outdated-fake-ca-bundle")
				})

				It("should update caBundle", func() {
					Expect(spu.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
					Expect(spu.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
				})
			})

			When("webhooks slice is non-empty", func() {
				BeforeEach(func() {
					validatingWebhookConfiguration.Webhooks = []admissionv1.ValidatingWebhook{
						{
							ClientConfig: admissionv1.WebhookClientConfig{
								CABundle: []byte("fake-ca-bundle"),
							},
						},
					}
					spu.Spec.CABundle = []byte("outdated-fake-ca-bundle")
				})

				It("should get caBundle from first non-empty entry in slice and update caBundle", func() {
					Expect(spu.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
					Expect(spu.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
				})
			})
		})
	})
}
