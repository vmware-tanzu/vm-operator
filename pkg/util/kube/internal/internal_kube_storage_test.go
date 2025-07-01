// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package internal_test

import (
	"context"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/internal"
)

var _ = Describe("GetEncryptedStorageClassRefs", func() {

	const fakeString = "fake"

	var (
		ctx    context.Context
		client ctrlclient.Client
		obj    corev1.ConfigMap
		err    error
		funcs  interceptor.Funcs
		refs   []metav1.OwnerReference
	)

	BeforeEach(func() {
		ctx = pkgcfg.WithConfig(pkgcfg.Config{
			PodNamespace: fakeString,
		})
		funcs = interceptor.Funcs{}
		obj = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      internal.EncryptedStorageClassNamesConfigMapName,
			},
		}
	})

	JustBeforeEach(func() {
		client = fake.NewClientBuilder().
			WithInterceptorFuncs(funcs).
			WithObjects(&obj).
			Build()
		refs, err = internal.GetEncryptedStorageClassRefs(ctx, client)
	})

	When("there is a 404 error getting the ConfigMap", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				return apierrors.NewNotFound(
					corev1.Resource("configmaps"),
					obj.GetName())
			}
		})
		It("should return an empty slice", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(refs).To(BeEmpty())
		})
	})

	When("there is a non-404 error getting the ConfigMap", func() {
		BeforeEach(func() {
			funcs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				return apierrors.NewInternalError(errors.New(fakeString))
			}
		})
		It("should return an empty slice", func() {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(
				apierrors.NewInternalError(errors.New(fakeString)).Error()))
			Expect(refs).To(BeEmpty())
		})
	})

	When("there are no owner refs", func() {
		It("should return an empty slice", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(refs).To(BeEmpty())
		})
	})

	When("there are owner refs", func() {

		var storageClasses []storagev1.StorageClass

		BeforeEach(func() {
			storageClasses = []storagev1.StorageClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: uuid.NewString(),
						UID:  types.UID(uuid.NewString()),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: uuid.NewString(),
						UID:  types.UID(uuid.NewString()),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: uuid.NewString(),
						UID:  types.UID(uuid.NewString()),
					},
				},
			}

			obj.OwnerReferences = make(
				[]metav1.OwnerReference, len(storageClasses))

			for i := range storageClasses {
				obj.OwnerReferences[i] = internal.GetOwnerRefForStorageClass(
					storageClasses[i])
			}
		})
		It("should return the owner refs", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(refs).To(HaveLen(len(storageClasses)))

			expRefs := make([]any, len(storageClasses))
			for i := range storageClasses {
				expRefs[i] = internal.GetOwnerRefForStorageClass(
					storageClasses[i])
			}
			Expect(refs).To(HaveLen(len(expRefs)))
			Expect(refs).To(ContainElements(expRefs...))
		})
	})
})
