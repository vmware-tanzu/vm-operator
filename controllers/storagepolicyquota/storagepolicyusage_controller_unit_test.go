// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/storagepolicyquota"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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
	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler *storagepolicyquota.Reconciler
		src        *spqv1.StoragePolicyQuota
	)

	BeforeEach(func() {
		initObjects = nil
		src = &spqv1.StoragePolicyQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-quota",
				Namespace: "dummy-namespace",
			},
		}
	})

	JustBeforeEach(func() {
		initObjects = append(initObjects, src)
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = storagepolicyquota.NewReconciler(
			ctx,
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
			name = src.Name
			namespace = src.Namespace
		})

		JustBeforeEach(func() {
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}})
		})

		// Please note it is not possible to validate garbage collection with
		// the fake client or with envtest, because neither of them implement
		// the Kubernetes garbage collector.
		When("Deleted", func() {
			BeforeEach(func() {
				src.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				src.Finalizers = append(src.Finalizers, "fake.com/finalizer")
			})
			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Normal", func() {
			assertStoragePolicyUsage := func(err error) {
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				var dst spqv1.StoragePolicyUsage
				ExpectWithOffset(1, ctx.Client.Get(
					ctx,
					types.NamespacedName{
						Namespace: namespace,
						Name:      kubeutil.StoragePolicyUsageNameFromQuotaName(name),
					},
					&dst)).To(Succeed())
				ExpectWithOffset(1, dst.Spec.ResourceAPIgroup).To(Equal(ptr.To(spqv1.GroupVersion.Group)))
				ExpectWithOffset(1, dst.Spec.ResourceExtensionName).To(Equal("vmservice.cns.vsphere.vmware.com"))
				ExpectWithOffset(1, dst.Spec.ResourceKind).To(Equal("StoragePolicyQuota"))
				ExpectWithOffset(1, dst.Spec.StorageClassName).To(Equal(src.Name))
				ExpectWithOffset(1, dst.Spec.StoragePolicyId).To(Equal(src.Spec.StoragePolicyId))
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
							Name:      kubeutil.StoragePolicyUsageNameFromQuotaName(name),
						},
						Spec: spqv1.StoragePolicyUsageSpec{
							StoragePolicyId: src.Spec.StoragePolicyId,
						},
					}
					initObjects = append(initObjects, dst)
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
									Kind:               "StoragePolicyQuota",
									Name:               src.Name,
									UID:                src.UID,
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
