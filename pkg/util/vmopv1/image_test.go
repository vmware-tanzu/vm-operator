// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = DescribeTable("IsImageReady",
	func(obj vmopv1.VirtualMachineImage, expErr string) {
		err := vmopv1util.IsImageReady(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
	Entry(
		"condition is missing",
		vmopv1.VirtualMachineImage{},
		"image condition is not ready: nil",
	),
	Entry(
		"condition is not ready",
		vmopv1.VirtualMachineImage{
			Status: vmopv1.VirtualMachineImageStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.ReadyConditionType,
						Status: metav1.ConditionFalse,
					},
				},
			},
		},
		fmt.Sprintf("image condition is not ready: %v", &metav1.Condition{
			Type:   vmopv1.ReadyConditionType,
			Status: metav1.ConditionFalse,
		}),
	),
	Entry(
		"provider ref is nil",
		vmopv1.VirtualMachineImage{
			Status: vmopv1.VirtualMachineImageStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.ReadyConditionType,
						Status: metav1.ConditionTrue,
					},
				},
			},
		},
		"image provider ref is empty",
	),
	Entry(
		"provider ref name is empty",
		vmopv1.VirtualMachineImage{
			Spec: vmopv1.VirtualMachineImageSpec{
				ProviderRef: &vmopv1common.LocalObjectRef{},
			},
			Status: vmopv1.VirtualMachineImageStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.ReadyConditionType,
						Status: metav1.ConditionTrue,
					},
				},
			},
		},
		"image provider ref is empty",
	),
	Entry(
		"ready",
		vmopv1.VirtualMachineImage{
			Spec: vmopv1.VirtualMachineImageSpec{
				ProviderRef: &vmopv1common.LocalObjectRef{
					Name: "my-provider-ref-name",
				},
			},
			Status: vmopv1.VirtualMachineImageStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.ReadyConditionType,
						Status: metav1.ConditionTrue,
					},
				},
			},
		},
		"",
	),
)

var _ = DescribeTable("IsImageOVF",
	func(obj vmopv1.VirtualMachineImage, expErr string) {
		err := vmopv1util.IsImageOVF(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
	Entry(
		"no type",
		vmopv1.VirtualMachineImage{},
		"image type \"\" is not OVF",
	),
	Entry(
		"ISO",
		vmopv1.VirtualMachineImage{
			Status: vmopv1.VirtualMachineImageStatus{
				Type: "ISO",
			},
		},
		"image type \"ISO\" is not OVF",
	),
	Entry(
		"OVF",
		vmopv1.VirtualMachineImage{
			Status: vmopv1.VirtualMachineImageStatus{
				Type: "OVF",
			},
		},
		"",
	),
)

var _ = DescribeTable("IsImageProviderReady",
	func(obj vmopv1.VirtualMachineImage, expErr string) {
		err := vmopv1util.IsImageProviderReady(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
	Entry(
		"ref is nil",
		vmopv1.VirtualMachineImage{},
		"image provider ref is nil",
	),
	Entry(
		"name is empty",
		vmopv1.VirtualMachineImage{
			Spec: vmopv1.VirtualMachineImageSpec{
				ProviderRef: &vmopv1common.LocalObjectRef{},
			},
		},
		"image provider ref name is empty",
	),
	Entry(
		"ready",
		vmopv1.VirtualMachineImage{
			Spec: vmopv1.VirtualMachineImageSpec{
				ProviderRef: &vmopv1common.LocalObjectRef{
					Name: "my-provider-ref",
				},
			},
		},
		"",
	),
)

var _ = DescribeTable("IsLibraryItemSynced",
	func(obj imgregv1a1.ContentLibraryItem, expErr string) {
		err := vmopv1util.IsLibraryItemSynced(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
	Entry(
		"not cached",
		imgregv1a1.ContentLibraryItem{},
		vmopv1util.ErrImageNotSynced.Error(),
	),
	Entry(
		"zero size",
		imgregv1a1.ContentLibraryItem{
			Status: imgregv1a1.ContentLibraryItemStatus{
				Cached: true,
			},
		},
		vmopv1util.ErrImageNotSynced.Error(),
	),
	Entry(
		"synced",
		imgregv1a1.ContentLibraryItem{
			Status: imgregv1a1.ContentLibraryItemStatus{
				Cached:      true,
				SizeInBytes: resource.MustParse("10Gi"),
			},
		},
		"",
	),
)

var _ = DescribeTable("GetStorageURIsForLibraryItemDisks",
	func(obj imgregv1a1.ContentLibraryItem, expOut []string, expErr string) {
		out, err := vmopv1util.GetStorageURIsForLibraryItemDisks(obj)
		if expErr != "" {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(Equal(expOut))
		}
	},
	Entry(
		"no disks",
		imgregv1a1.ContentLibraryItem{},
		nil,
		fmt.Sprintf(
			"no vmdk files found in the content library item status: %v",
			imgregv1a1.ContentLibraryItemStatus{}),
	),

	Entry(
		"has disks and other files",
		imgregv1a1.ContentLibraryItem{
			Status: imgregv1a1.ContentLibraryItemStatus{
				FileInfo: []imgregv1a1.FileInfo{
					{
						StorageURI: "ds://vmfs/volumes/123/my-image.ovf",
					},
					{
						StorageURI: "ds://vmfs/volumes/123/my-image-disk-1.vmdk",
					},
					{
						StorageURI: "ds://vmfs/volumes/123/my-image-disk-2.VMDK",
					},
					{
						StorageURI: "ds://vmfs/volumes/123/my-image.mf",
					},
				},
			},
		},
		[]string{
			"ds://vmfs/volumes/123/my-image-disk-1.vmdk",
			"ds://vmfs/volumes/123/my-image-disk-2.VMDK",
		},
		"",
	),
)

var _ = XDescribe("GetContentLibraryItemForImage", func() {

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		img       vmopv1.VirtualMachineImage
		expOut    imgregv1a1.ContentLibraryItem
		expErr    error
	)

	_, _, _, _, _ = ctx, k8sClient, img, expOut, expErr

	// TODO(akutz) Implement test
})

var _ = XDescribe("GetImageDiskInfo", func() {
	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		imgRef    vmopv1.VirtualMachineImageRef
		namespace string
		expOut    vmopv1util.ImageDiskInfo
		expErr    error
	)

	_, _, _, _, _, _ = ctx, k8sClient, imgRef, namespace, expOut, expErr

	// TODO(akutz) Implement test
})

var _ = XDescribe("GetImage", func() {
	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		imgRef    vmopv1.VirtualMachineImageRef
		namespace string
		expOut    vmopv1.VirtualMachineImage
		expErr    error
	)

	_, _, _, _, _, _ = ctx, k8sClient, imgRef, namespace, expOut, expErr

	// TODO(akutz) Implement test
})

var _ = Describe("VirtualMachineImageCacheToItemMapper", func() {
	const (
		vmiCacheName  = "my-vmi"
		namespaceName = "fake"
	)

	var (
		ctx          context.Context
		logger       logr.Logger
		k8sClient    ctrlclient.Client
		groupVersion schema.GroupVersion
		kind         string
		withObjs     []ctrlclient.Object
		withFuncs    interceptor.Funcs
		obj          *vmopv1.VirtualMachineImageCache
		mapFn        handler.MapFunc
		mapFnCtx     context.Context
		mapFnObj     ctrlclient.Object
		reqs         []reconcile.Request
	)
	BeforeEach(func() {
		reqs = nil
		withObjs = nil
		withFuncs = interceptor.Funcs{}

		ctx = context.Background()
		logger = logr.Discard()
		mapFnCtx = ctx
		groupVersion = schema.GroupVersion{
			Group:   "",
			Version: "v1",
		}
		kind = "ConfigMap"

		obj = &vmopv1.VirtualMachineImageCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmiCacheName,
				Namespace: namespaceName,
			},
			Status: vmopv1.VirtualMachineImageCacheStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.VirtualMachineImageCacheConditionOVFReady,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
		mapFnObj = obj
	})
	JustBeforeEach(func() {
		withObjs = append(withObjs, obj)
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
		Expect(k8sClient).ToNot(BeNil())
	})
	When("panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.VirtualMachineImageCacheToItemMapper(
						ctx,
						logger,
						k8sClient,
						groupVersion,
						kind)
				}).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.VirtualMachineImageCacheToItemMapper(
						ctx,
						logger,
						k8sClient,
						groupVersion,
						kind)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
		When("groupVersion is empty", func() {
			JustBeforeEach(func() {
				groupVersion = schema.GroupVersion{}
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.VirtualMachineImageCacheToItemMapper(
						ctx,
						logger,
						k8sClient,
						groupVersion,
						kind)
				}).To(PanicWith("groupVersion is empty"))
			})
		})
		When("kind is empty", func() {
			JustBeforeEach(func() {
				kind = ""
			})
			It("should panic", func() {
				Expect(func() {
					_ = vmopv1util.VirtualMachineImageCacheToItemMapper(
						ctx,
						logger,
						k8sClient,
						groupVersion,
						kind)
				}).To(PanicWith("kind is empty"))
			})
		})
		Context("mapFn", func() {
			JustBeforeEach(func() {
				mapFn = vmopv1util.VirtualMachineImageCacheToItemMapper(
					ctx,
					logger,
					k8sClient,
					groupVersion,
					kind)
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
			When("object is invalid", func() {
				BeforeEach(func() {
					mapFnObj = &vmopv1.VirtualMachine{}
				})
				It("should panic", func() {
					Expect(func() {
						_ = mapFn(mapFnCtx, mapFnObj)
					}).To(PanicWith(fmt.Sprintf("object is %T", mapFnObj)))
				})
			})
		})
	})
	When("panic is not expected", func() {
		JustBeforeEach(func() {
			mapFn = vmopv1util.VirtualMachineImageCacheToItemMapper(
				ctx,
				logger,
				k8sClient,
				groupVersion,
				kind)
			Expect(mapFn).ToNot(BeNil())
			reqs = mapFn(mapFnCtx, mapFnObj)
		})
		When("there is an error listing resources", func() {
			BeforeEach(func() {
				withFuncs.List = func(
					ctx context.Context,
					client ctrlclient.WithWatch,
					list ctrlclient.ObjectList,
					opts ...ctrlclient.ListOption) error {

					if _, ok := list.(*corev1.ConfigMapList); ok {
						return errors.New("fake")
					}
					return client.List(ctx, list, opts...)
				}
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("there are no matching resources", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "configmap-1",
						},
					},
				)
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("there is a single matching resource but ovf is not ready", func() {
			BeforeEach(func() {
				obj.Status.Conditions = nil

				withObjs = append(withObjs,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "configmap-1",
							Labels: map[string]string{
								pkgconst.VMICacheLabelKey: vmiCacheName,
							},
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "configmap-2",
						},
					},
				)
			})
			Specify("no reconcile requests should be returned", func() {
				Expect(reqs).To(BeEmpty())
			})
		})
		When("there is a single matching resource", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "configmap-1",
							Labels: map[string]string{
								pkgconst.VMICacheLabelKey: vmiCacheName,
							},
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "configmap-2",
						},
					},
				)
			})
			Specify("one reconcile request should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "ns-1",
							Name:      "configmap-1",
						},
					},
				))
			})
		})
		When("there are multiple matching resources across multiple namespaces", func() {
			BeforeEach(func() {
				withObjs = append(withObjs,
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-1",
							Name:      "configmap-1",
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-2",
							Name:      "configmap-2",
							Labels: map[string]string{
								pkgconst.VMICacheLabelKey: vmiCacheName,
							},
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-3",
							Name:      "configmap-3",
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-4",
							Name:      "configmap-4",
							Labels: map[string]string{
								pkgconst.VMICacheLabelKey: vmiCacheName,
							},
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-5",
							Name:      "configmap-5",
							Labels: map[string]string{
								pkgconst.VMICacheLabelKey: vmiCacheName,
							},
						},
					},
					&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns-6",
							Name:      "configmap-6",
						},
					},
				)
			})
			Specify("an equal number of requests should be returned", func() {
				Expect(reqs).To(ConsistOf(
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "ns-2",
							Name:      "configmap-2",
						},
					},
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "ns-4",
							Name:      "configmap-4",
						},
					},
					reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "ns-5",
							Name:      "configmap-5",
						},
					},
				))
			})
		})
	})
})
