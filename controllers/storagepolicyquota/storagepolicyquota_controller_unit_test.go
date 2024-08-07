// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/storagepolicyquota"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
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
	)

	var (
		ctx         *builder.UnitTestContextForController
		withObjects []client.Object

		reconciler         *storagepolicyquota.Reconciler
		storagePolicyQuota *spqv1.StoragePolicyQuota
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
		withObjects = append(withObjects, storagePolicyQuota)

		ctx = suite.NewUnitTestContextForController(withObjects...)

		reconciler = storagepolicyquota.NewReconciler(
			pkgcfg.UpdateContext(
				ctx,
				func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				},
			),
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)
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
				var obj spqv1.StoragePolicyUsage
				ExpectWithOffset(1, ctx.Client.Get(
					ctx,
					types.NamespacedName{
						Namespace: namespace,
						Name:      spqutil.StoragePolicyUsageName(storageClassName),
					},
					&obj)).To(Succeed())
				ExpectWithOffset(1, obj.Spec.ResourceAPIgroup).To(Equal(ptr.To(vmopv1.GroupVersion.Group)))
				ExpectWithOffset(1, obj.Spec.ResourceExtensionName).To(Equal(spqutil.StoragePolicyQuotaExtensionName))
				ExpectWithOffset(1, obj.Spec.ResourceKind).To(Equal("VirtualMachine"))
				ExpectWithOffset(1, obj.Spec.StorageClassName).To(Equal(storageClassName))
				ExpectWithOffset(1, obj.Spec.StoragePolicyId).To(Equal(storagePolicyID))
			}

			When("a StoragePolicyUsage resource does not exist", func() {
				It("creates the resource", func() {
					assertStoragePolicyUsage(err)
				})
			})
			When("a StoragePolicyUsage resource does exist", func() {
				var dst *spqv1.StoragePolicyUsage

				BeforeEach(func() {
					dst = &spqv1.StoragePolicyUsage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      spqutil.StoragePolicyUsageName(storageClassName),
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
									Kind:               "VirtualMachine",
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
								"VirtualMachine",
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
	})

}
