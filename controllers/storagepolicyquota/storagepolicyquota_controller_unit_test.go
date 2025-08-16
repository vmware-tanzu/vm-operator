// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/storagepolicyquota"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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
								"Object %s/%s is already owned by another %s controller %s",
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
