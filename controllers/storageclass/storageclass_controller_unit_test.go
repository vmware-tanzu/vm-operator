// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
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

	"github.com/vmware-tanzu/vm-operator/controllers/storageclass"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.V1Alpha3,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		initObjects []ctrlclient.Object
		ctx         *builder.UnitTestContextForController

		reconciler     *storageclass.Reconciler
		obj            *storagev1.StorageClass
		fakeVMProvider *providerfake.VMProvider

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

		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)
		fakeVMProvider.DoesProfileSupportEncryptionFn = func(
			ctx context.Context,
			profileID string) (bool, error) {

			return profileID == myEncryptedStoragePolicy, nil
		}

		reconciler = storageclass.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			fakeVMProvider)

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
				// StorageClass update will cause a reconcile so don't need to return an error.
				Expect(err).ToNot(HaveOccurred())
			})
		})
		When("not encrypted", func() {
			BeforeEach(func() {
				obj.Parameters = map[string]string{
					"storagePolicyID": "my-storage-policy",
				}
			})
			It("marks item as encrypted and returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				ok, _, err := kubeutil.IsEncryptedStorageClass(ctx, ctx.Client, obj.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
		When("encrypted", func() {
			BeforeEach(func() {
				obj.Parameters = map[string]string{
					"storagePolicyID": myEncryptedStoragePolicy,
				}
			})
			It("marks item as encrypted and returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				ok, _, err := kubeutil.IsEncryptedStorageClass(ctx, ctx.Client, obj.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
	})
}
