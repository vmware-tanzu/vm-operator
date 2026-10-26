// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("TagOwnerReferencesUIDIndexerFunc", func() {
	When("the object is not a Tag", func() {
		It("should return nil", func() {
			Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(&corev1.Pod{})).To(BeNil())
		})
	})

	When("the object is a Tag", func() {
		When("there are no owner references", func() {
			It("should return nil", func() {
				tag := &vspherepolv1.Tag{}
				Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(tag)).To(BeNil())
			})
		})

		When("there is a single owner reference", func() {
			It("should return the owner's UID", func() {
				tag := &vspherepolv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID("vm-a-uid")},
						},
					},
				}
				Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(tag)).To(
					ConsistOf("vm-a-uid"))
			})
		})

		When("there are several owner references", func() {
			It("should return one entry per owner UID", func() {
				tag := &vspherepolv1.Tag{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID("vm-a-uid")},
							{UID: types.UID("vm-b-uid")},
							{UID: types.UID("vm-c-uid")},
						},
					},
				}
				Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(tag)).To(
					ConsistOf("vm-a-uid", "vm-b-uid", "vm-c-uid"))
			})
		})
	})
})

// recordingFieldIndexer is a minimal ctrlclient.FieldIndexer that records the
// object/field pairs it was asked to index, so RegisterVMTagsIndexes
// can be verified without a running manager or API server.
type recordingFieldIndexer struct {
	registered []recordedIndex
}

type recordedIndex struct {
	obj   ctrlclient.Object
	field string
}

func (r *recordingFieldIndexer) IndexField(
	_ context.Context,
	obj ctrlclient.Object,
	field string,
	_ ctrlclient.IndexerFunc) error {

	r.registered = append(r.registered, recordedIndex{obj: obj, field: field})
	return nil
}

var _ = Describe("RegisterVMTagsIndexes", func() {
	It("registers the Tag owner-UID index", func() {
		indexer := &recordingFieldIndexer{}
		Expect(kubeutil.RegisterVMTagsIndexes(context.Background(), indexer)).To(Succeed())

		Expect(indexer.registered).To(ConsistOf(
			recordedIndex{obj: &vspherepolv1.Tag{}, field: kubeutil.TagOwnerReferencesUIDIndexKey},
		))
	})

	When("an index registration fails", func() {
		It("propagates the error", func() {
			expectedErr := errors.New("fake")
			indexer := &erroringFieldIndexer{err: expectedErr}
			Expect(kubeutil.RegisterVMTagsIndexes(context.Background(), indexer)).To(
				MatchError(expectedErr))
		})
	})
})

// erroringFieldIndexer always fails on IndexField, to exercise
// RegisterVMTagsIndexes' error propagation.
type erroringFieldIndexer struct {
	err error
}

func (e *erroringFieldIndexer) IndexField(
	context.Context, ctrlclient.Object, string, ctrlclient.IndexerFunc) error {

	return e.err
}
