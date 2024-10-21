// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validatingwebhookconfiguration_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {

	const (
		secretName = "vmware-system-vmop-serving-cert"
		vmSPUName  = "vm-spu"
		pvcSPUName = "pvc-spu"
	)

	var (
		ctx                            *builder.IntegrationTestContext
		caBundle                       []byte
		validatingWebhookConfiguration *admissionv1.ValidatingWebhookConfiguration
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		caBundle = []byte("fake-ca-bundle")

		validatingWebhookConfiguration = &admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: spqutil.ValidatingWebhookConfigName,
				Annotations: map[string]string{
					"cert-manager.io/inject-ca-from": fmt.Sprintf("%s/vmware-system-vmop-serving-cert", ctx.Namespace),
				},
			},
			Webhooks: []admissionv1.ValidatingWebhook{
				{
					AdmissionReviewVersions: []string{"v1beta1", "v1"},
					ClientConfig: admissionv1.WebhookClientConfig{
						Service: &admissionv1.ServiceReference{
							Name:      "vmware-system-vmop-webhook-service",
							Namespace: "vmware-system-vmop",
							Path:      ptr.To("/default-validate-vmoperator-vmware-com-v1alpha3-virtualmachine"),
						},
						CABundle: caBundle,
					},
					FailurePolicy: ptr.To(admissionv1.Fail),
					Name:          "default.validating.virtualmachine.v1alpha3.vmoperator.vmware.com",
					SideEffects:   ptr.To(admissionv1.SideEffectClassNone),
				},
			},
		}
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, validatingWebhookConfiguration)).To(Succeed())

		caBundle = nil
		validatingWebhookConfiguration = nil

		ctx.AfterEach()
	})

	JustBeforeEach(func() {
		// Different resourceExtensionName
		pvcSPU := &spqv1.StoragePolicyUsage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcSPUName,
				Namespace: ctx.Namespace,
			},
			Spec: spqv1.StoragePolicyUsageSpec{
				StorageClassName:      builder.DummyStorageClassName,
				StoragePolicyId:       "dummy-storage-policy-id",
				ResourceExtensionName: "fake.cns.vsphere.vmware.com",
				CABundle:              []byte("invalid-resource-ca-bundle"),
			},
		}
		Expect(ctx.Client.Create(ctx, pvcSPU)).To(Succeed())

		// Same resourceExtensionName
		vmSPU := &spqv1.StoragePolicyUsage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmSPUName,
				Namespace: ctx.Namespace,
			},
			Spec: spqv1.StoragePolicyUsageSpec{
				StorageClassName:      builder.DummyStorageClassName,
				StoragePolicyId:       "dummy-storage-policy-id",
				ResourceExtensionName: spqutil.StoragePolicyQuotaExtensionName,
				CABundle:              []byte("initial-ca-bundle"),
			},
		}
		Expect(ctx.Client.Create(ctx, vmSPU)).To(Succeed())
	})

	Context("CABundle from ValidatingWebhookConfig", func() {

		When("ValidatingWebhookConfiguration is created", func() {

			It("should update the correct resources and leave others untouched", func() {
				// Create ValidatingWebhookConfiguration
				Expect(ctx.Client.Create(ctx, validatingWebhookConfiguration)).To(Succeed())
				// SPU for vm should be updated with new CABundle, while that for pvc should be left alone
				Eventually(func(g Gomega) {
					pvcSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: pvcSPUName, Namespace: ctx.Namespace}, pvcSPU)).To(Succeed())

					g.Expect(pvcSPU.Spec.CABundle).NotTo(Equal(validatingWebhookConfiguration.Webhooks[0].ClientConfig.CABundle))
					g.Expect(pvcSPU.Spec.CABundle).To(Equal([]byte("invalid-resource-ca-bundle")))

					vmSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vmSPUName, Namespace: ctx.Namespace}, vmSPU)).To(Succeed())

					g.Expect(vmSPU.Spec.CABundle).NotTo(Equal([]byte("initial-ca-bundle")))
					g.Expect(vmSPU.Spec.CABundle).To(Equal(caBundle))
				}).Should(Succeed())
			})
		})

		When("ValidatingWebhookConfiguration is updated", FlakeAttempts(5), func() {

			BeforeEach(func() {
				// Create ValidatingWebhookConfiguration
				Expect(ctx.Client.Create(ctx, validatingWebhookConfiguration)).To(Succeed())
			})

			It("should update the correct resources and leave others untouched", func() {
				// Update ValidatingWebhookConfiguration with new CABundle
				webhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}, webhookConfiguration)).To(Succeed())

				webhookConfiguration.Webhooks[0].ClientConfig.CABundle = []byte("updated-ca-bundle")
				Expect(ctx.Client.Update(ctx, webhookConfiguration)).To(Succeed())

				// SPU for vm should be updated with new CABundle, while that for pvc should be left alone
				Eventually(func(g Gomega) {
					pvcSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: pvcSPUName, Namespace: ctx.Namespace}, pvcSPU)).To(Succeed())

					g.Expect(pvcSPU.Spec.CABundle).NotTo(Equal(webhookConfiguration.Webhooks[0].ClientConfig.CABundle))
					g.Expect(pvcSPU.Spec.CABundle).To(Equal([]byte("invalid-resource-ca-bundle")))

					vmSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vmSPUName, Namespace: ctx.Namespace}, vmSPU)).To(Succeed())

					g.Expect(vmSPU.Spec.CABundle).NotTo(Equal(caBundle))
					g.Expect(vmSPU.Spec.CABundle).To(Equal(webhookConfiguration.Webhooks[0].ClientConfig.CABundle))
				}).Should(Succeed())
			})
		})
	})

	Context("CABundle from Secret", func() {
		var certSecret *corev1.Secret

		BeforeEach(func() {
			validatingWebhookConfiguration.Webhooks = []admissionv1.ValidatingWebhook{}

			certSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ctx.Namespace,
				},
				Data: map[string][]byte{
					"ca.crt": caBundle,
				},
			}

			Expect(ctx.Client.Create(ctx, certSecret)).To(Succeed())
		})

		When("ValidatingWebhookConfiguration is created", func() {

			It("should update the correct resources and leave others untouched", func() {
				// Create ValidatingWebhookConfiguration
				Expect(ctx.Client.Create(ctx, validatingWebhookConfiguration)).To(Succeed())
				// SPU for vm should be updated with new CABundle, while that for pvc should be left alone
				Eventually(func(g Gomega) {
					pvcSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: pvcSPUName, Namespace: ctx.Namespace}, pvcSPU)).To(Succeed())

					g.Expect(pvcSPU.Spec.CABundle).NotTo(Equal(certSecret.Data["ca.crt"]))
					g.Expect(pvcSPU.Spec.CABundle).To(Equal([]byte("invalid-resource-ca-bundle")))

					vmSPU := &spqv1.StoragePolicyUsage{}
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vmSPUName, Namespace: ctx.Namespace}, vmSPU)).To(Succeed())

					g.Expect(vmSPU.Spec.CABundle).NotTo(Equal([]byte("initial-ca-bundle")))
					g.Expect(vmSPU.Spec.CABundle).To(Equal(caBundle))
				}).Should(Succeed())
			})
		})
	})
}
