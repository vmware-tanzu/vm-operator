// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Reconcile", Label(testlabels.Controller, testlabels.EnvTest), intgTestsReconcile)
}

func intgTestsReconcile() {

	const (
		storageQuotaName = "my-storage-quota"
		storageClassName = "my-storage-class"
		storagePolicyID  = "my-storage-policy"
	)

	var (
		ctx             *builder.IntegrationTestContext
		storageQuotaUID types.UID
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		Expect(ctx.Client.DeleteAllOf(ctx, &spqv1.StoragePolicyQuota{}, client.InNamespace(ctx.Namespace))).To(Succeed())
		Eventually(func(g Gomega) {
			var quotas spqv1.StoragePolicyQuotaList
			g.Expect(suite.GetManager().GetClient().List(ctx, &quotas, client.InNamespace(ctx.Namespace))).To(Succeed())
			g.Expect(quotas.Items).To(BeEmpty())
		}).Should(Succeed())
		ctx.AfterEach()
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			validatingWebhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{
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
								Path:      ptr.To("/default-validate-vmoperator-vmware-com-v1alpha6-virtualmachine"),
							},
							CABundle: []byte("fake-ca-bundle"),
						},
						FailurePolicy: ptr.To(admissionv1.Fail),
						Name:          "default.validating.virtualmachine.v1alpha6.vmoperator.vmware.com",
						SideEffects:   ptr.To(admissionv1.SideEffectClassNone),
					},
				},
			}
			Expect(ctx.Client.Create(ctx, validatingWebhookConfiguration)).To(Succeed())
			DeferCleanup(func() {
				Expect(ctx.Client.Delete(ctx, validatingWebhookConfiguration)).To(Succeed())
			})

			storageClass := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: storageClassName},
				Provisioner: "fake",
				Parameters:  map[string]string{"storagePolicyID": storagePolicyID},
			}
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
			DeferCleanup(func() {
				Expect(ctx.Client.Delete(ctx, storageClass)).To(Succeed())
			})
			Eventually(func() error {
				return suite.GetManager().GetClient().Get(ctx,
					client.ObjectKeyFromObject(storageClass), &storagev1.StorageClass{})
			}).Should(Succeed())

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

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				var usage spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: ctx.Namespace, Name: spqutil.StoragePolicyUsageNameForVM(storageClassName),
				}, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("fake-ca-bundle")))
			}).Should(Succeed())
		})

		It("should only create StoragePolicyUsage resources for VirtualMachine", func() {
			Eventually(func(g Gomega) {
				var obj spqv1.StoragePolicyUsage
				dstKey := client.ObjectKey{
					Namespace: ctx.Namespace,
					Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
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
				g.Expect(obj.Spec.ResourceKind).To(Equal(spqutil.VirtualMachineKind))
				g.Expect(obj.Spec.ResourceExtensionName).To(Equal(spqutil.StoragePolicyQuotaVMExtensionName))
				g.Expect(obj.Spec.ResourceExtensionNamespace).To(Equal(ctx.PodNamespace))
				g.Expect(obj.Spec.CABundle).To(Equal([]byte("fake-ca-bundle")))
			}).Should(Succeed())

			var obj spqv1.StoragePolicyUsage
			dstKey := client.ObjectKey{
				Namespace: ctx.Namespace,
				Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
			}
			Expect(ctx.Client.Get(ctx, dstKey, &obj)).NotTo(Succeed())
		})

		It("rotates the CA on an owned usage without changing its quota status", func() {
			var usage spqv1.StoragePolicyUsage
			key := client.ObjectKey{Namespace: ctx.Namespace, Name: spqutil.StoragePolicyUsageNameForVM(storageClassName)}
			Eventually(func(g Gomega) {
				g.Expect(ctx.Client.Get(ctx, key, &usage)).To(Succeed())
			}).Should(Succeed())
			usage.Status.ResourceTypeLevelQuotaUsage = &spqv1.QuotaUsageDetails{Used: ptr.To(resource.MustParse("5Gi"))}
			Expect(ctx.Client.Status().Update(ctx, &usage)).To(Succeed())
			before := usage.DeepCopy()

			var webhook admissionv1.ValidatingWebhookConfiguration
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}, &webhook)).To(Succeed())
			webhook.Webhooks[0].ClientConfig.CABundle = []byte("rotated-ca")
			Expect(ctx.Client.Update(ctx, &webhook)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(ctx.Client.Get(ctx, key, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))
			}).Should(Succeed())
			before.Spec.CABundle = []byte("rotated-ca")
			Expect(usage.Spec).To(Equal(before.Spec))
			Expect(usage.Status).To(Equal(before.Status))
			Expect(usage.OwnerReferences).To(Equal(before.OwnerReferences))
		})

		Context("with snapshots enabled and quotas in multiple namespaces", func() {
			var otherNamespace *corev1.Namespace

			BeforeEach(func() {
				pkgcfg.SetContext(suite, func(config *pkgcfg.Config) { config.Features.VMSnapshots = true })
				DeferCleanup(func() {
					pkgcfg.SetContext(suite, func(config *pkgcfg.Config) { config.Features.VMSnapshots = false })
				})
				otherNamespace = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{GenerateName: "storage-quota-"},
				}
				Expect(ctx.Client.Create(ctx, otherNamespace)).To(Succeed())
				DeferCleanup(func() {
					Expect(ctx.Client.DeleteAllOf(ctx, &spqv1.StoragePolicyQuota{}, client.InNamespace(otherNamespace.Name))).To(Succeed())
					Eventually(func(g Gomega) {
						var quotas spqv1.StoragePolicyQuotaList
						g.Expect(suite.GetManager().GetClient().List(ctx, &quotas, client.InNamespace(otherNamespace.Name))).To(Succeed())
						g.Expect(quotas.Items).To(BeEmpty())
					}).Should(Succeed())
					Expect(ctx.Client.Delete(ctx, otherNamespace)).To(Succeed())
				})
				quota := &spqv1.StoragePolicyQuota{
					ObjectMeta: metav1.ObjectMeta{Name: storageQuotaName, Namespace: otherNamespace.Name},
					Spec:       spqv1.StoragePolicyQuotaSpec{StoragePolicyId: storagePolicyID},
				}
				Expect(ctx.Client.Create(ctx, quota)).To(Succeed())
				Eventually(func(g Gomega) {
					var usage spqv1.StoragePolicyUsage
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: otherNamespace.Name, Name: spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
					}, &usage)).To(Succeed())
					g.Expect(usage.Spec.CABundle).To(Equal([]byte("fake-ca-bundle")))
				}).Should(Succeed())
			})

			It("fans out CA rotation to VM and snapshot usages across namespaces", func() {
				updateQuotaWebhookCA(ctx, []byte("rotated-ca"))
				Eventually(func(g Gomega) {
					var usage spqv1.StoragePolicyUsage
					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: ctx.Namespace,
						Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
					}, &usage)).To(Succeed())
					g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))

					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: ctx.Namespace,
						Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
					}, &usage)).To(Succeed())
					g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))

					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: otherNamespace.Name,
						Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
					}, &usage)).To(Succeed())
					g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))

					g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Namespace: otherNamespace.Name,
						Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
					}, &usage)).To(Succeed())
					g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))
				}).Should(Succeed())
			})
		})

		It("leaves disabled snapshot and unmanaged usages unchanged", func() {
			snapshotUsage := &spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{Name: spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName), Namespace: ctx.Namespace},
				Spec: spqv1.StoragePolicyUsageSpec{
					StorageClassName: storageClassName, StoragePolicyId: storagePolicyID,
					ResourceKind:          spqutil.VirtualMachineSnapshotKind,
					ResourceExtensionName: spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
					CABundle:              []byte("unchanged-ca"),
				},
			}
			Expect(ctx.Client.Create(ctx, snapshotUsage)).To(Succeed())

			unmanagedUsage := &spqv1.StoragePolicyUsage{
				ObjectMeta: metav1.ObjectMeta{Name: "unmanaged-usage", Namespace: ctx.Namespace},
				Spec: spqv1.StoragePolicyUsageSpec{
					StorageClassName: "missing-class", StoragePolicyId: storagePolicyID,
					ResourceKind:          spqutil.VirtualMachineKind,
					ResourceExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
					CABundle:              []byte("unchanged-ca"),
				},
			}
			Expect(ctx.Client.Create(ctx, unmanagedUsage)).To(Succeed())
			updateQuotaWebhookCA(ctx, []byte("rotated-ca"))
			Eventually(func(g Gomega) {
				var usage spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: ctx.Namespace, Name: spqutil.StoragePolicyUsageNameForVM(storageClassName),
				}, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))
			}).Should(Succeed())
			for _, name := range []string{spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName), "unmanaged-usage"} {
				var usage spqv1.StoragePolicyUsage
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: ctx.Namespace, Name: name}, &usage)).To(Succeed())
				Expect(usage.Spec.CABundle).To(Equal([]byte("unchanged-ca")))
			}
		})

		It("uses the fallback Secret CA after fan-out", func() {
			var webhook admissionv1.ValidatingWebhookConfiguration
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}, &webhook)).To(Succeed())
			webhook.Webhooks = nil
			webhook.Annotations = map[string]string{spqutil.WebhookConfigAnnotationKey: ctx.Namespace + "/ca-secret"}
			Expect(ctx.Client.Update(ctx, &webhook)).To(Succeed())
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "ca-secret", Namespace: ctx.Namespace},
				Data:       map[string][]byte{spqutil.SecretCertKey: []byte("secret-ca")},
			}
			Expect(ctx.Client.Create(ctx, secret)).To(Succeed())
			Eventually(func(g Gomega) {
				var usage spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: ctx.Namespace, Name: spqutil.StoragePolicyUsageNameForVM(storageClassName),
				}, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("secret-ca")))
			}).Should(Succeed())
		})

		It("fans out when the webhook configuration is recreated", func() {
			var webhook admissionv1.ValidatingWebhookConfiguration
			key := client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}
			Expect(ctx.Client.Get(ctx, key, &webhook)).To(Succeed())
			Expect(ctx.Client.Delete(ctx, &webhook)).To(Succeed())
			Eventually(func() bool {
				return apierrors.IsNotFound(suite.GetManager().GetClient().Get(ctx, key, &admissionv1.ValidatingWebhookConfiguration{}))
			}).Should(BeTrue())
			webhook.ResourceVersion = ""
			webhook.UID = ""
			webhook.Webhooks[0].ClientConfig.CABundle = []byte("recreated-ca")
			Expect(ctx.Client.Create(ctx, &webhook)).To(Succeed())
			Eventually(func(g Gomega) {
				var usage spqv1.StoragePolicyUsage
				g.Expect(ctx.Client.Get(ctx, client.ObjectKey{
					Namespace: ctx.Namespace, Name: spqutil.StoragePolicyUsageNameForVM(storageClassName),
				}, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("recreated-ca")))
			}).Should(Succeed())
		})
	})
}

func updateQuotaWebhookCA(ctx *builder.IntegrationTestContext, ca []byte) {
	var webhook admissionv1.ValidatingWebhookConfiguration
	ExpectWithOffset(1, ctx.Client.Get(ctx, client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}, &webhook)).To(Succeed())
	webhook.Webhooks[0].ClientConfig.CABundle = ca
	ExpectWithOffset(1, ctx.Client.Update(ctx, &webhook)).To(Succeed())
}
