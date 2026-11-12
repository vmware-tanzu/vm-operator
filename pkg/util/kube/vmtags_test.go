// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("TagOwnerReferencesUIDIndexerFunc", Label(testlabels.Controller), func() {
	It("returns nil for non-Tags and one UID per owner of a Tag", func() {
		Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(&corev1.Pod{})).To(BeNil())
		Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(&vspherepolv1.Tag{})).To(BeNil())
		tag := &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{
					{UID: types.UID("vm-a-uid")},
					{UID: types.UID("vm-b-uid")},
				},
			},
		}
		Expect(kubeutil.TagOwnerReferencesUIDIndexerFunc(tag)).To(
			ConsistOf("vm-a-uid", "vm-b-uid"))
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

var _ = Describe("RegisterVMTagsIndexes", Label(testlabels.Controller), func() {
	It("registers all three indexes", func() {
		indexer := &recordingFieldIndexer{}
		Expect(kubeutil.RegisterVMTagsIndexes(context.Background(), indexer)).To(Succeed())

		Expect(indexer.registered).To(HaveLen(3))
		Expect(indexer.registered).To(ConsistOf(
			recordedIndex{obj: &vspherepolv1.Tag{}, field: kubeutil.TagSourceLabelIndexKey},
			recordedIndex{obj: &vmopv1.VirtualMachine{}, field: kubeutil.VMLabelKeyValueIndexKey},
			recordedIndex{obj: &vspherepolv1.Tag{}, field: kubeutil.TagOwnerReferencesUIDIndexKey},
		))
	})

})

var _ = Describe("VMLabelKeyValueIndexerFunc", Label(testlabels.Controller), func() {
	It("returns nil for objects that are not VirtualMachines", func() {
		Expect(kubeutil.VMLabelKeyValueIndexerFunc(&corev1.Pod{})).To(BeNil())
	})

	It("returns one key/value entry per label, excluding reserved labels", func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app":                              "nginx",
					"example.com/app":                  "",
					"vmoperator.vmware.com/some-label": "value",
				},
			},
		}
		Expect(kubeutil.VMLabelKeyValueIndexerFunc(vm)).To(
			ConsistOf("app:nginx", "example.com/app:"))
	})
})

var _ = Describe("TagToVirtualMachineMapper", Label(testlabels.Controller), func() {
	const namespaceName = "fake-ns"

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
		tag       *vspherepolv1.Tag
		mapFn     handler.MapFunc
		mapFnCtx  context.Context
		mapFnObj  ctrlclient.Object
		reqs      []reconcile.Request
	)

	BeforeEach(func() {
		reqs = nil
		withObjs = nil
		withFuncs = interceptor.Funcs{}

		ctx = context.Background()
		mapFnCtx = ctx

		tag = &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      "tag-1",
			},
			Spec: vspherepolv1.TagSpec{
				Key:        "app",
				Value:      "nginx",
				Name:       "app:nginx",
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
			},
		}
		mapFnObj = tag
	})

	JustBeforeEach(func() {
		k8sClient = fake.NewClientBuilder().
			WithScheme(builder.NewScheme()).
			WithInterceptorFuncs(withFuncs).
			WithObjects(withObjs...).
			WithIndex(
				&vmopv1.VirtualMachine{},
				kubeutil.VMLabelKeyValueIndexKey,
				kubeutil.VMLabelKeyValueIndexerFunc).
			Build()
	})

	When("panic is expected", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = kubeutil.TagToVirtualMachineMapper(ctx, k8sClient)
				}).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = kubeutil.TagToVirtualMachineMapper(ctx, k8sClient)
				}).To(PanicWith("k8sClient is nil"))
			})
		})

		Context("mapFn", func() {
			JustBeforeEach(func() {
				mapFn = kubeutil.TagToVirtualMachineMapper(ctx, k8sClient)
				Expect(mapFn).ToNot(BeNil())
			})

			When("ctx is nil", func() {
				BeforeEach(func() {
					mapFnCtx = nil
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith("context is nil"))
				})
			})

			When("object is nil", func() {
				BeforeEach(func() {
					mapFnObj = nil
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith("object is nil"))
				})
			})

			When("object is not a Tag", func() {
				It("should panic", func() {
					obj := &vmopv1.VirtualMachine{}
					Expect(func() {
						_ = mapFn(mapFnCtx, obj)
					}).To(PanicWith(fmt.Sprintf("object is %T", obj)))
				})
			})
		})
	})

	When("panic is not expected", func() {
		JustBeforeEach(func() {
			mapFn = kubeutil.TagToVirtualMachineMapper(ctx, k8sClient)
			Expect(mapFn).ToNot(BeNil())
			reqs = mapFn(mapFnCtx, mapFnObj)
		})

		When("there is an error listing VMs", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					c ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*vmopv1.VirtualMachineList); ok {
						return errors.New("fake")
					}
					return c.List(ctx, list, opts...)
				}
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})

		When("a single VM carries the Tag's label", func() {
			BeforeEach(func() {
				withObjs = append(withObjs, &vmopv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceName,
						Name:      "vm-match",
						Labels:    map[string]string{"app": "nginx"},
					},
				})
			})
			Specify("one reconcile request should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-match",
						},
					},
				))
			})
		})

		When("multiple VMs carry the Tag's label, including an owner", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-owner",
							Labels:    map[string]string{"app": "nginx"},
							OwnerReferences: []metav1.OwnerReference{
								{UID: types.UID("vm-owner-uid")},
							},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-label-only",
							Labels:    map[string]string{"app": "nginx"},
						},
					},
					&vmopv1.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      "vm-unrelated",
						},
					},
				)
			})
			Specify("one reconcile request per carrying VM should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-owner",
						},
					},
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: namespaceName,
							Name:      "vm-label-only",
						},
					},
				))
			})
		})
	})
})

var _ = Describe("TagFanOutPredicate", Label(testlabels.Controller), func() {
	// The Tag carries no finalizer, so a delete is atomic and every update is
	// the Tag controller's own status/label-mirror write or the VM path's
	// owner-reference patch, none of which can change any VM's desired tag set.
	It("admits only create and delete", func() {
		pred := kubeutil.TagFanOutPredicate()
		oldTag := &vspherepolv1.Tag{ObjectMeta: metav1.ObjectMeta{Namespace: "fake-ns", Name: "tag-1"}}
		newTag := oldTag.DeepCopy()
		newTag.OwnerReferences = []metav1.OwnerReference{{UID: types.UID("vm-a-uid")}}

		Expect(pred.Create(event.CreateEvent{Object: newTag})).To(BeTrue())
		Expect(pred.Delete(event.DeleteEvent{Object: oldTag})).To(BeTrue())
		Expect(pred.Generic(event.GenericEvent{Object: oldTag})).To(BeFalse())
		Expect(pred.Update(event.UpdateEvent{ObjectOld: oldTag, ObjectNew: newTag})).To(BeFalse())
	})
})

var _ = Describe("TagSourceLabelIndexerFunc", Label(testlabels.Controller), func() {
	It("ignores other objects and tags without a source label", func() {
		Expect(kubeutil.TagSourceLabelIndexerFunc(&corev1.Pod{})).To(BeNil())
		Expect(kubeutil.TagSourceLabelIndexerFunc(&vspherepolv1.Tag{})).To(BeNil())
	})

	It("preserves prefixed keys and empty values without mutating the object", func() {
		tag := &vspherepolv1.Tag{Spec: vspherepolv1.TagSpec{
			Key:        "example.com/app",
			Value:      "",
			Name:       "example.com/app:",
			Kind:       vspherepolv1.TagKindOriginal,
			Membership: vspherepolv1.TagMembershipSourceLabel,
		}}
		before := tag.DeepCopy()
		Expect(kubeutil.TagSourceLabelIndexerFunc(tag)).To(Equal([]string{"example.com/app:"}))
		Expect(tag).To(Equal(before))
	})

})

var _ = Describe("Self Tag event mapping", Label(testlabels.API), func() {
	It("maps valid unique VM owners without reading peer VMs", func() {
		ctx := context.Background()
		client := builder.NewFakeClientWithInterceptors(interceptor.Funcs{
			List: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectList, ...ctrlclient.ListOption) error {
				Fail("self mapping must not list objects")
				return nil
			},
		})
		tag := &vspherepolv1.Tag{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", OwnerReferences: []metav1.OwnerReference{
			{APIVersion: vmopv1.GroupVersion.String(), Kind: "VirtualMachine", Name: "vm", UID: "uid"},
			{APIVersion: vmopv1.GroupVersion.String(), Kind: "VirtualMachine", Name: "vm", UID: "old-uid"},
			{APIVersion: "other/v1", Kind: "VirtualMachine", Name: "foreign", UID: "uid"},
			{APIVersion: "v1", Kind: "Pod", Name: "pod", UID: "uid"},
			{APIVersion: vmopv1.GroupVersion.String(), Kind: "VirtualMachine", Name: "no-uid"},
		}}, Spec: vspherepolv1.TagSpec{
			Kind:       vspherepolv1.TagKindGroupedAntiAffinitySelf,
			Membership: vspherepolv1.TagMembershipPolicyRequesters,
		}}
		Expect(kubeutil.TagToVirtualMachineMapper(ctx, client)(ctx, tag)).To(Equal([]reconcile.Request{{NamespacedName: ctrlclient.ObjectKey{Namespace: "ns", Name: "vm"}}}))
	})
})
