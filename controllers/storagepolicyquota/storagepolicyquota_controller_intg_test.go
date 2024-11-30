// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
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
		storageQuotaName = "my-storage-quota"
		storageClassName = "my-storage-class"
		storagePolicyID  = "my-storage-policy"

		caBundle        = "fake-ca-bundle"
		caBundleUpdated = "updated-ca-bundle"
	)

	var (
		ctx             *builder.IntegrationTestContext
		storageQuotaUID types.UID
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	Context("Reconcile", func() {
		var (
			validatingWebhookConfiguration *admissionv1.ValidatingWebhookConfiguration
			storageClass                   *storagev1.StorageClass
		)

		BeforeEach(func() {
			validatingWebhookConfiguration = &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: spqutil.ValidatingWebhookConfigName,
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
							CABundle: []byte(caBundle),
						},
						FailurePolicy: ptr.To(admissionv1.Fail),
						Name:          "default.validating.virtualmachine.v1alpha3.vmoperator.vmware.com",
						SideEffects:   ptr.To(admissionv1.SideEffectClassNone),
					},
				},
			}
			Expect(ctx.Client.Create(ctx, validatingWebhookConfiguration)).To(Succeed())

			storageClass = &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: storageClassName,
				},
				Provisioner: "fake",
				Parameters: map[string]string{
					"storagePolicyID": storagePolicyID,
				},
			}
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())

			obj := spqv1.StoragePolicyQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      storageQuotaName,
					Namespace: ctx.Namespace,
				},
				Spec: spqv1.StoragePolicyQuotaSpec{
					StoragePolicyId: storagePolicyID,
				},
			}
			Expect(ctx.Client.Create(ctx, &obj)).To(Succeed())
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(&obj), &obj)).To(Succeed())
			obj.Status = spqv1.StoragePolicyQuotaStatus{
				SCLevelQuotaStatuses: spqv1.SCLevelQuotaStatusList{
					{
						StorageClassName: storageClassName,
					},
				},
			}
			storageQuotaUID = obj.UID
			Expect(ctx.Client.Status().Update(ctx, &obj)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctx.Client.Delete(ctx, validatingWebhookConfiguration)).To(Succeed())
			Expect(ctx.Client.Delete(ctx, storageClass)).To(Succeed())
		})

		It("should result in the creation of a StoragePolicyUsage resource", func() {
			Eventually(func(g Gomega) {
				var obj spqv1.StoragePolicyUsage
				dstKey := client.ObjectKey{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageName(storageClassName),
				}
				g.Expect(ctx.Client.Get(ctx, dstKey, &obj)).To(Succeed())

				g.Expect(obj.OwnerReferences).To(Equal([]metav1.OwnerReference{
					{
						APIVersion:         spqv1.GroupVersion.String(),
						Kind:               spqutil.StoragePolicyQuotaKind,
						Name:               storageQuotaName,
						UID:                storageQuotaUID,
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					},
				}))
				g.Expect(obj.Spec.StoragePolicyId).To(Equal(storagePolicyID))
				g.Expect(obj.Spec.StorageClassName).To(Equal(storageClassName))
				g.Expect(obj.Spec.ResourceAPIgroup).To(Equal(ptr.To(vmopv1.GroupVersion.Group)))
				g.Expect(obj.Spec.ResourceKind).To(Equal("VirtualMachine"))
				g.Expect(obj.Spec.ResourceExtensionName).To(Equal(spqutil.StoragePolicyQuotaExtensionName))
				g.Expect(obj.Spec.ResourceExtensionNamespace).To(Equal(ctx.PodNamespace))
				g.Expect(obj.Spec.CABundle).To(Equal([]byte(caBundle)))
			}).Should(Succeed())
		})

		It("reconciles a change to ValidatingWebhookConfiguration", func() {
			Eventually(func(g Gomega) {
				var spu spqv1.StoragePolicyUsage
				dstKey := client.ObjectKey{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageName(storageClassName),
				}
				g.Expect(ctx.Client.Get(ctx, dstKey, &spu)).To(Succeed())

				g.Expect(spu.Spec.CABundle).To(Equal([]byte(caBundle)))
			}).Should(Succeed())

			var obj admissionv1.ValidatingWebhookConfiguration
			objKey := client.ObjectKey{
				Name: spqutil.ValidatingWebhookConfigName,
			}
			Expect(ctx.Client.Get(ctx, objKey, &obj)).To(Succeed())

			obj.Webhooks[0].ClientConfig.CABundle = []byte(caBundleUpdated)

			Expect(ctx.Client.Update(ctx, &obj)).To(Succeed())

			Eventually(func(g Gomega) {
				var spu spqv1.StoragePolicyUsage
				dstKey := client.ObjectKey{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageName(storageClassName),
				}
				g.Expect(ctx.Client.Get(ctx, dstKey, &spu)).To(Succeed())

				g.Expect(spu.Spec.CABundle).To(Equal([]byte(caBundleUpdated)))
			}).Should(Succeed())
		})
	})
}
