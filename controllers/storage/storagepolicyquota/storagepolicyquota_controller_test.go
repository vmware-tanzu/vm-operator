// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/storage/storagepolicyquota"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Quota CA failure recovery", Label(testlabels.Controller), webhookConfigurationFailureTests)
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
		namespaceName    = "default"
		storageQuotaName = "my-storage-quota"
		storageClassName = "my-storage-class"
		storagePolicyID  = "my-storage-policy"
		caBundle         = "fake-ca-bundle"
	)

	var (
		ctx         *builder.UnitTestContextForController
		withObjects []client.Object

		reconciler                     *storagepolicyquota.Reconciler
		storagePolicyQuota             *spqv1.StoragePolicyQuota
		validatingWebhookConfiguration *admissionv1.ValidatingWebhookConfiguration
		// enableFeatureFunc is used to enable the VMSnapshots feature flag
		enableFeatureFunc = func(ctx *builder.UnitTestContextForController) {}
	)

	BeforeEach(func() {
		withObjects = []client.Object{
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: storageClassName,
				},
				Provisioner: "fake",
				Parameters: map[string]string{
					"storagePolicyID": storagePolicyID,
				},
			},
		}

		validatingWebhookConfiguration = &admissionv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: spqutil.ValidatingWebhookConfigName,
			},
			Webhooks: []admissionv1.ValidatingWebhook{
				{
					ClientConfig: admissionv1.WebhookClientConfig{
						CABundle: []byte(caBundle),
					},
				},
			},
		}

		storagePolicyQuota = &spqv1.StoragePolicyQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storageQuotaName,
				Namespace: namespaceName,
			},
			Spec: spqv1.StoragePolicyQuotaSpec{
				StoragePolicyId: storagePolicyID,
			},
			Status: spqv1.StoragePolicyQuotaStatus{
				SCLevelQuotaStatuses: spqv1.SCLevelQuotaStatusList{
					{
						StorageClassName: storageClassName,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		withObjects = append(withObjects, storagePolicyQuota, validatingWebhookConfiguration)

		ctx = suite.NewUnitTestContextForController(withObjects...)
		if enableFeatureFunc != nil {
			enableFeatureFunc(ctx)
		}
		reconciler = storagepolicyquota.NewReconciler(
			ctx.Context,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.Namespace,
		)
	})

	AfterEach(func() {
		enableFeatureFunc = nil
	})

	Context("Reconcile", func() {
		var (
			err       error
			name      string
			namespace string
		)

		BeforeEach(func() {
			err = nil
			name = storageQuotaName
			namespace = namespaceName
		})

		JustBeforeEach(func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}})
			Expect(ctx.Client.Get(
				ctx,
				client.ObjectKeyFromObject(storagePolicyQuota),
				storagePolicyQuota)).To(Succeed())
		})

		// Please note it is not possible to validate garbage collection with
		// the fake client or with envtest, because neither of them implement
		// the Kubernetes garbage collector.
		When("Deleted", func() {
			BeforeEach(func() {
				storagePolicyQuota.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				storagePolicyQuota.Finalizers = append(storagePolicyQuota.Finalizers, "fake.com/finalizer")
			})
			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Normal", func() {
			assertStoragePolicyUsage := func(err error) {
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				By("Checking StoragePolicyUsage for VirtualMachine and VirtualMachineSnapshot")
				resourceKinds := []struct {
					kind               string
					nameFunc           func(string) string
					quotaExtensionName string
					enabled            bool
				}{
					{
						kind:               spqutil.VirtualMachineKind,
						nameFunc:           spqutil.StoragePolicyUsageNameForVM,
						quotaExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
						enabled:            true,
					},
					{
						kind:               spqutil.VirtualMachineSnapshotKind,
						nameFunc:           spqutil.StoragePolicyUsageNameForVMSnapshot,
						quotaExtensionName: spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
						enabled:            enableFeatureFunc != nil,
					},
				}
				for _, resourceKind := range resourceKinds {
					var obj spqv1.StoragePolicyUsage
					if !resourceKind.enabled {
						ExpectWithOffset(1, ctx.Client.Get(
							ctx,
							types.NamespacedName{
								Namespace: namespace,
								Name:      resourceKind.nameFunc(storageClassName),
							},
							&obj)).NotTo(Succeed())
						continue
					}
					Eventually(func(g Gomega) {
						ExpectWithOffset(1, ctx.Client.Get(
							ctx,
							types.NamespacedName{
								Namespace: namespace,
								Name:      resourceKind.nameFunc(storageClassName),
							},
							&obj)).To(Succeed())
						ExpectWithOffset(1, obj.Spec.ResourceAPIgroup).To(Equal(ptr.To(vmopv1.GroupVersion.Group)))
						ExpectWithOffset(1, obj.Spec.ResourceExtensionName).To(Equal(resourceKind.quotaExtensionName))
						ExpectWithOffset(1, obj.Spec.ResourceExtensionNamespace).To(Equal(ctx.Namespace))
						ExpectWithOffset(1, obj.Spec.ResourceKind).To(Equal(resourceKind.kind))
						ExpectWithOffset(1, obj.Spec.StorageClassName).To(Equal(storageClassName))
						ExpectWithOffset(1, obj.Spec.StoragePolicyId).To(Equal(storagePolicyID))
						ExpectWithOffset(1, obj.Spec.CABundle).To(Equal([]byte(caBundle)))
					}).Should(Succeed())
				}
			}

			When("StoragePolicyUsage resources do not exist", func() {
				It("creates resource for VirtualMachine by default", func() {
					assertStoragePolicyUsage(err)
				})
				When("VMSnapshot feature flag is enabled", func() {
					BeforeEach(func() {
						enableFeatureFunc = func(ctx *builder.UnitTestContextForController) {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.VMSnapshots = true
							})
						}
					})
					JustBeforeEach(func() {
						Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
					})
					It("creates resources for VirtualMachine and VirtualMachineSnapshot", func() {
						assertStoragePolicyUsage(err)
					})
				})
			})

			When("a StoragePolicyUsage resource does exist", func() {
				var dst *spqv1.StoragePolicyUsage

				BeforeEach(func() {
					dst = &spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageNameForVM(storageClassName),
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							StoragePolicyId: storagePolicyID,
						},
					}
					withObjects = append(withObjects, dst)
				})

				When("the resource does not have a controller reference", func() {
					It("patches the resource", func() {
						assertStoragePolicyUsage(err)
					})
				})
				When("the resource does have a controller reference", func() {
					When("it is not the StoragePolicyQuota", func() {
						BeforeEach(func() {
							dst.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
								{
									APIVersion:         vmopv1.GroupName,
									Kind:               spqutil.VirtualMachineKind,
									Name:               "my-vm",
									UID:                types.UID("1234"),
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							}
						})
						It("returns an error", func() {
							Expect(err).To(HaveOccurred())
							Expect(err).To(MatchError(fmt.Sprintf(
								"failed to CreateOrPatch SPU %s: Object %s/%s is already owned by another %s controller %s",
								dst.Name,
								dst.Namespace,
								dst.Name,
								spqutil.VirtualMachineKind,
								"my-vm")))
						})
					})
					When("it is the StoragePolicyQuota", func() {
						BeforeEach(func() {
							dst.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
								{
									APIVersion:         spqv1.GroupVersion.String(),
									Kind:               spqutil.StoragePolicyQuotaKind,
									Name:               storageQuotaName,
									UID:                storagePolicyQuota.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							}
						})
						It("patches the resource", func() {
							assertStoragePolicyUsage(err)
						})
					})
				})
			})

			When("a VMSnapshot StoragePolicyUsage resource does exist but with wrong extension name", func() {
				var dst *spqv1.StoragePolicyUsage
				BeforeEach(func() {
					By("VMSnapshot feature flag is enabled")
					enableFeatureFunc = func(ctx *builder.UnitTestContextForController) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMSnapshots = true
						})
					}

					By("Create SPU for snapshot with wrong extension name")
					dst = &spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         spqv1.GroupVersion.String(),
									Kind:               spqutil.StoragePolicyQuotaKind,
									Name:               storageQuotaName,
									UID:                storagePolicyQuota.UID,
									Controller:         ptr.To(true),
									BlockOwnerDeletion: ptr.To(true),
								},
							},
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							StoragePolicyId:       storagePolicyID,
							ResourceExtensionName: spqutil.StoragePolicyQuotaVMExtensionName, // Wrong extension name.
						},
					}
					withObjects = append(withObjects, dst)
				})

				JustBeforeEach(func() {
					Expect(pkgcfg.FromContext(ctx).Features.VMSnapshots).To(BeTrue())
				})

				It("recreates VirtualMachineSnapshot with correct resourceExtensionName", func() {
					assertStoragePolicyUsage(err)
				})
			})
		})

		When("Object not found", func() {
			Context("name is invalid", func() {
				BeforeEach(func() {
					name = "invalid"
				})
				It("ignores the error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})
			Context("namespace is invalid", func() {
				BeforeEach(func() {
					namespace = "invalid"
				})
				It("ignores the error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		When("Unable to get CABundle; annotation is missing", func() {
			BeforeEach(func() {
				validatingWebhookConfiguration.Webhooks = []admissionv1.ValidatingWebhook{}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unable to get annotation"))
			})
		})
	})
}

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

		It("fans out CA rotation to VM and snapshot usages across namespaces", func() {
			pkgcfg.SetContext(suite, func(config *pkgcfg.Config) { config.Features.VMSnapshots = true })
			DeferCleanup(func() {
				pkgcfg.SetContext(suite, func(config *pkgcfg.Config) { config.Features.VMSnapshots = false })
			})
			other := suite.NewIntegrationTestContext()
			DeferCleanup(func() {
				Expect(other.Client.DeleteAllOf(other, &spqv1.StoragePolicyQuota{}, client.InNamespace(other.Namespace))).To(Succeed())
				Eventually(func(g Gomega) {
					var quotas spqv1.StoragePolicyQuotaList
					g.Expect(suite.GetManager().GetClient().List(other, &quotas, client.InNamespace(other.Namespace))).To(Succeed())
					g.Expect(quotas.Items).To(BeEmpty())
				}).Should(Succeed())
				other.AfterEach()
			})
			quota := &spqv1.StoragePolicyQuota{
				ObjectMeta: metav1.ObjectMeta{Name: storageQuotaName, Namespace: other.Namespace},
				Spec:       spqv1.StoragePolicyQuotaSpec{StoragePolicyId: storagePolicyID},
			}
			Expect(other.Client.Create(other, quota)).To(Succeed())
			Eventually(func(g Gomega) {
				var usage spqv1.StoragePolicyUsage
				g.Expect(other.Client.Get(other, client.ObjectKey{
					Namespace: other.Namespace, Name: spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
				}, &usage)).To(Succeed())
				g.Expect(usage.Spec.CABundle).To(Equal([]byte("fake-ca-bundle")))
			}).Should(Succeed())

			updateQuotaWebhookCA(ctx, []byte("rotated-ca"))
			Eventually(func(g Gomega) {
				for _, namespace := range []string{ctx.Namespace, other.Namespace} {
					for _, name := range []string{
						spqutil.StoragePolicyUsageNameForVM(storageClassName),
						spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName),
					} {
						var usage spqv1.StoragePolicyUsage
						g.Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &usage)).To(Succeed())
						g.Expect(usage.Spec.CABundle).To(Equal([]byte("rotated-ca")))
					}
				}
			}).Should(Succeed())
		})

		It("leaves disabled snapshot and unmanaged usages unchanged", func() {
			for _, usage := range []*spqv1.StoragePolicyUsage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: spqutil.StoragePolicyUsageNameForVMSnapshot(storageClassName), Namespace: ctx.Namespace},
					Spec: spqv1.StoragePolicyUsageSpec{
						StorageClassName: storageClassName, StoragePolicyId: storagePolicyID,
						ResourceKind:          spqutil.VirtualMachineSnapshotKind,
						ResourceExtensionName: spqutil.StoragePolicyQuotaVMSnapshotExtensionName,
						CABundle:              []byte("unchanged-ca"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "unmanaged-usage", Namespace: ctx.Namespace},
					Spec: spqv1.StoragePolicyUsageSpec{
						StorageClassName: "missing-class", StoragePolicyId: storagePolicyID,
						ResourceKind:          spqutil.VirtualMachineKind,
						ResourceExtensionName: spqutil.StoragePolicyQuotaVMExtensionName,
						CABundle:              []byte("unchanged-ca"),
					},
				},
			} {
				Expect(ctx.Client.Create(ctx, usage)).To(Succeed())
			}
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

		It("retries an unavailable fallback Secret after fan-out without another webhook event", func() {
			var webhook admissionv1.ValidatingWebhookConfiguration
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: spqutil.ValidatingWebhookConfigName}, &webhook)).To(Succeed())
			webhook.Webhooks = nil
			webhook.Annotations = map[string]string{spqutil.WebhookConfigAnnotationKey: ctx.Namespace + "/ca-secret"}
			errorsBefore := storageQuotaReconcileErrors()
			Expect(ctx.Client.Update(ctx, &webhook)).To(Succeed())
			Eventually(storageQuotaReconcileErrors).Should(BeNumerically(">", errorsBefore))
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

func storageQuotaReconcileErrors() float64 {
	families, err := ctrlmetrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, family := range families {
		if family.GetName() != "controller_runtime_reconcile_errors_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "controller" && label.GetValue() == "storagepolicyquota" {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func webhookConfigurationFailureTests() {
	DescribeTable("retries failures through normal quota reconciliation",
		func(stage string) {
			failure := errors.New("transient API failure")
			fail := false
			patchCalls := 0
			funcs := interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
					obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*admissionv1.ValidatingWebhookConfiguration); ok && fail && stage == "get" {
						return failure
					}
					return c.Get(ctx, key, obj, opts...)
				},
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList,
					opts ...client.ListOption) error {
					if _, ok := list.(*storagev1.StorageClassList); ok && fail && stage == "list" {
						return failure
					}
					return c.List(ctx, list, opts...)
				},
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
					patch client.Patch, opts ...client.PatchOption) error {
					patchCalls++
					if fail && stage == "patch" {
						return failure
					}
					return c.Patch(ctx, obj, patch, opts...)
				},
			}
			webhook := &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: spqutil.ValidatingWebhookConfigName},
				Webhooks: []admissionv1.ValidatingWebhook{{
					ClientConfig: admissionv1.WebhookClientConfig{CABundle: []byte("old-ca")},
				}},
			}
			quota := &spqv1.StoragePolicyQuota{
				ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "namespace"},
				Spec:       spqv1.StoragePolicyQuotaSpec{StoragePolicyId: "policy"},
			}
			storageClass := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{Name: "class"},
				Parameters: map[string]string{"storagePolicyID": "policy"},
			}
			ctx := suite.NewUnitTestContextForControllerWithFuncs(funcs, webhook, quota, storageClass)
			DeferCleanup(ctx.AfterEach)
			// Configuration must come from the manager context during reconciliation.
			managerContext := pkgcfg.NewContextWithDefaultConfig()
			pkgcfg.SetContext(managerContext, func(config *pkgcfg.Config) { config.Features.VMSnapshots = true })
			r := storagepolicyquota.NewReconciler(managerContext, ctx.Client, ctx.Logger, ctx.Recorder, ctx.Namespace)
			req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(quota)}
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			webhook.Webhooks[0].ClientConfig.CABundle = []byte("new-ca")
			Expect(ctx.Client.Update(ctx, webhook)).To(Succeed())
			fail = true
			requests := spqutil.ValidatingWebhookConfigurationToStoragePolicyQuotaMapper(ctx.Client)(ctx, webhook)
			Expect(requests).To(Equal([]reconcile.Request{req}))
			_, err = r.Reconcile(ctx, requests[0])
			Expect(errors.Is(err, failure)).To(BeTrue())
			fail = false
			_, err = r.Reconcile(ctx, requests[0])
			Expect(err).NotTo(HaveOccurred())
			for _, name := range []string{spqutil.StoragePolicyUsageNameForVM("class"), spqutil.StoragePolicyUsageNameForVMSnapshot("class")} {
				var usage spqv1.StoragePolicyUsage
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: quota.Namespace, Name: name}, &usage)).To(Succeed())
				Expect(usage.Spec.CABundle).To(Equal([]byte("new-ca")))
			}
			writes := patchCalls
			_, err = r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(patchCalls).To(Equal(writes), "a converged quota reconcile must not write usages")
		},
		Entry("CA lookup", "get"),
		Entry("storage class lookup", "list"),
		Entry("usage patch", "patch"),
	)
}
