// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storageclass_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/vm-operator/controllers/storage/storageclass"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.API,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []ctrlclient.Object
		ctx         *builder.UnitTestContextForController

		reconciler *storageclass.Reconciler
		obj        *storagev1.StorageClass

		err  error
		name string
	)

	BeforeEach(func() {
		err = nil
		initObjects = nil
		obj = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-storage-class",
				UID:  types.UID(uuid.NewString()),
			},
		}
		name = obj.Name
	})

	JustBeforeEach(func() {
		initObjects = append(initObjects, obj)
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = storageclass.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder)

		_, err = reconciler.Reconcile(
			context.Background(),
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: name,
				},
			})
	})

	When("NotFound", func() {
		BeforeEach(func() {
			name = "invalid"
		})
		It("ignores the error", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("Deleted", func() {
		BeforeEach(func() {
			obj.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			obj.Finalizers = append(obj.Finalizers, "fake.com/finalizer")
		})
		It("returns success", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("Normal", func() {
		When("no storage policy ID", func() {
			It("should not return an error", func() {
				// StorageClass update will cause a reconcile so don't need to
				// return an error.
				Expect(err).ToNot(HaveOccurred())
			})
		})
		When("has invalid storage policy ID", func() {
			BeforeEach(func() {
				obj.Parameters = map[string]string{
					"storagePolicyID": "invalid",
				}
			})
			It("should create a StoragePolicy object", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{
					Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					Name:      kubeutil.GetStoragePolicyObjectName("invalid"),
				}, &infrav1.StoragePolicy{})).To(Succeed())
			})
		})
		When("has valid storage policy ID", func() {
			var profileID string
			BeforeEach(func() {
				profileID = uuid.NewString()
				obj.Parameters = map[string]string{
					"storagePolicyID": profileID,
				}
			})
			It("should create a StoragePolicy object", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{
					Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					Name:      kubeutil.GetStoragePolicyObjectName(profileID),
				}, &infrav1.StoragePolicy{})).To(Succeed())
			})
		})
	})
}
