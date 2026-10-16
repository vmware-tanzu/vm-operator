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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

func webhookConfigurationFailureTests() {
	DescribeTable("retries failures through normal quota reconciliation",
		func(stage string) {
			var injectedError error
			patchCalls := 0
			funcs := interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
					obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*admissionv1.ValidatingWebhookConfiguration); ok && injectedError != nil && stage == "get" {
						return injectedError
					}
					return c.Get(ctx, key, obj, opts...)
				},
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList,
					opts ...client.ListOption) error {
					if _, ok := list.(*storagev1.StorageClassList); ok && injectedError != nil && stage == "list" {
						return injectedError
					}
					return c.List(ctx, list, opts...)
				},
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
					patch client.Patch, opts ...client.PatchOption) error {
					patchCalls++
					if injectedError != nil && stage == "patch" {
						return injectedError
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
			By("Creating usages with the original CA")
			_, err := r.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("Rotating the CA and injecting a transient API failure")
			webhook.Webhooks[0].ClientConfig.CABundle = []byte("new-ca")
			Expect(ctx.Client.Update(ctx, webhook)).To(Succeed())
			injectedError = errors.New("transient API failure")
			requests := spqutil.ValidatingWebhookConfigurationToStoragePolicyQuotaMapper(ctx.Client)(ctx, webhook)
			Expect(requests).To(Equal([]reconcile.Request{req}))
			_, err = r.Reconcile(ctx, requests[0])
			Expect(err).To(MatchError(injectedError))

			By("Recovering on retry without another webhook event")
			injectedError = nil
			_, err = r.Reconcile(ctx, requests[0])
			Expect(err).NotTo(HaveOccurred())
			for _, name := range []string{spqutil.StoragePolicyUsageNameForVM("class"), spqutil.StoragePolicyUsageNameForVMSnapshot("class")} {
				var usage spqv1.StoragePolicyUsage
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Namespace: quota.Namespace, Name: name}, &usage)).To(Succeed())
				Expect(usage.Spec.CABundle).To(Equal([]byte("new-ca")))
			}

			By("Avoiding usage writes once reconciliation has converged")
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
