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
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
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
		spuForVM, spuForVMSnapshot     *spqv1.StoragePolicyUsage
		// enableFeatureFunc is used to enable the VMSnapshots feature flag
		enableFeatureFunc func(ctx *builder.UnitTestContextForController)
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

		spuForVM = &spqv1.StoragePolicyUsage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-spu-name-for-vm",
				Namespace: "fake-spu-namespace-for-vm",
			},
			Spec: spqv1.StoragePolicyUsageSpec{
				ResourceExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
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

	AfterEach(func() {
		enableFeatureFunc = nil
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, certSecret, validatingWebhookConfiguration, spuForVM)
		if spuForVMSnapshot != nil {
			withObjects = append(withObjects, spuForVMSnapshot)
		}
		ctx = suite.NewUnitTestContextForController(withObjects...)
		if enableFeatureFunc != nil {
			enableFeatureFunc(ctx)
		}
		reconciler = validatingwebhookconfiguration.NewReconciler(
			ctx.Context,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder)
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
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(spuForVM), spuForVM)).To(Succeed())
			})

			When("no SPU docs exist for vmservice", func() {
				BeforeEach(func() {
					spuForVM.Spec.ResourceExtensionName = "fake.cns.vsphere.vmware.com"
					spuForVM.Spec.CABundle = []byte("fake.cns.vsphere.vmware.com")
				})

				It("should not update caBundle", func() {
					Expect(spuForVM.Spec.CABundle).To(Equal([]byte("fake.cns.vsphere.vmware.com")))
				})
			})

			When("SPU caBundle matches contents of secret", func() {
				BeforeEach(func() {
					spuForVM.Spec.CABundle = certSecret.Data["ca.crt"]
				})

				It("should not update caBundle", func() {
					Expect(spuForVM.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
				})
			})

			When("SPU caBundle does not match contents of secret", func() {
				BeforeEach(func() {
					spuForVM.Spec.CABundle = []byte("outdated-fake-ca-bundle")
				})

				It("should update caBundle", func() {
					Expect(spuForVM.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
					Expect(spuForVM.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
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
					spuForVM.Spec.CABundle = []byte("outdated-fake-ca-bundle")
				})

				It("should get caBundle from first non-empty entry in slice and update caBundle", func() {
					Expect(spuForVM.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
					Expect(spuForVM.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
				})
			})

			When("VMSnapshots feature flag is enabled", func() {
				BeforeEach(func() {
					spuForVMSnapshot = &spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "fake-spu-name-for-vmsnapshot",
							Namespace: "fake-spu-namespace-for-vmsnapshot",
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							ResourceExtensionName: spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
						},
					}
					enableFeatureFunc = func(ctx *builder.UnitTestContextForController) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMSnapshots = true
						})
					}
				})
				JustBeforeEach(func() {
					Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(spuForVMSnapshot), spuForVMSnapshot)).To(Succeed())
				})

				When("SPU caBundle matches contents of secret", func() {
					BeforeEach(func() {
						spuForVMSnapshot.Spec.CABundle = certSecret.Data["ca.crt"]
					})

					It("should not update caBundle", func() {
						Expect(spuForVMSnapshot.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
					})
				})

				When("SPU caBundle does not match contents of secret", func() {
					BeforeEach(func() {
						spuForVMSnapshot.Spec.CABundle = []byte("outdated-fake-ca-bundle")
					})

					It("should update caBundle", func() {
						Expect(spuForVMSnapshot.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
						Expect(spuForVMSnapshot.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
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
						spuForVMSnapshot.Spec.CABundle = []byte("outdated-fake-ca-bundle")
					})

					It("should get caBundle from first non-empty entry in slice and update caBundle", func() {
						Expect(spuForVMSnapshot.Spec.CABundle).NotTo(Equal([]byte("outdated-fake-ca-bundle")))
						Expect(spuForVMSnapshot.Spec.CABundle).To(Equal(certSecret.Data["ca.crt"]))
					})
				})
			})
		})
	})
}
