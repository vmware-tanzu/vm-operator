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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
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

var _ = Describe("GetContentLibraryItemForImage", func() {
	const (
		vmiName       = builder.DummyVMIName
		clItem        = "fake-content-library-item"
		cclItem       = "fake-cluster-content-library-item"
		namespaceName = "fake"
	)

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		img       vmopv1.VirtualMachineImage
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
		cli       *imgregv1a1.ContentLibraryItem
		ccli      *imgregv1a1.ClusterContentLibraryItem
		expOut    imgregv1a1.ContentLibraryItem
		expErr    error
	)

	BeforeEach(func() {
		withObjs = nil
		withFuncs = interceptor.Funcs{}

		ctx = context.Background()

		img = *builder.DummyVirtualMachineImage(vmiName)

		cli = &imgregv1a1.ContentLibraryItem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clItem,
				Namespace: namespaceName,
			},
			Spec: imgregv1a1.ContentLibraryItemSpec{
				UUID: clItem,
			},
		}

		ccli = &imgregv1a1.ClusterContentLibraryItem{
			ObjectMeta: metav1.ObjectMeta{
				Name: cclItem,
			},
			Spec: imgregv1a1.ContentLibraryItemSpec{
				UUID: cclItem,
			},
		}

		img.Spec.ProviderRef = &vmopv1common.LocalObjectRef{
			Name: clItem,
		}
	})

	JustBeforeEach(func() {
		withObjs = append(withObjs, cli, ccli)
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
					_, _ = vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
				}).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
	})

	When("panic is not expected", func() {
		When("image is namespace-scoped", func() {
			BeforeEach(func() {
				img.Namespace = namespaceName
			})
			It("should return the correct object", func() {
				var obj imgregv1a1.ContentLibraryItem
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: cli.Name, Namespace: cli.Namespace}, &obj)).To(Succeed())
				expOut = obj

				item, err := vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(item).To(Equal(expOut))
			})
			When("there is an error getting library item", func() {
				BeforeEach(func() {
					expErr = errors.New("fake error")
					withFuncs.Get = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						key ctrlclient.ObjectKey,
						obj ctrlclient.Object,
						opts ...ctrlclient.GetOption) error {

						return expErr
					}
				})
				It("should return an error", func() {
					img, err := vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(Equal(expErr.Error()))
					Expect(img).To(Equal(imgregv1a1.ContentLibraryItem{}))
				})
			})
		})

		When("image is cluster-scoped", func() {
			BeforeEach(func() {
				img.Spec.ProviderRef.Name = cclItem
			})
			It("should return the correct object", func() {
				var obj imgregv1a1.ClusterContentLibraryItem
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: ccli.Name, Namespace: ccli.Namespace}, &obj)).To(Succeed())
				expOut = imgregv1a1.ContentLibraryItem(obj)

				item, err := vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(item).To(Equal(expOut))
			})
			When("there is an error getting library item", func() {
				BeforeEach(func() {
					expErr = errors.New("fake error")
					withFuncs.Get = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						key ctrlclient.ObjectKey,
						obj ctrlclient.Object,
						opts ...ctrlclient.GetOption) error {

						return expErr
					}
				})
				It("should return an error", func() {
					img, err := vmopv1util.GetContentLibraryItemForImage(ctx, k8sClient, img)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(Equal(expErr.Error()))
					Expect(img).To(Equal(imgregv1a1.ContentLibraryItem{}))
				})
			})
		})
	})
})

var _ = Describe("GetImageDiskInfo", func() {
	const (
		vmiName       = builder.DummyVMIName
		cliName       = "fake-content-library-item"
		namespaceName = "fake"
	)

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		imgRef    vmopv1.VirtualMachineImageRef
		namespace string
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
		vmi       *vmopv1.VirtualMachineImage
		cli       *imgregv1a1.ContentLibraryItem
		expErr    error
	)

	BeforeEach(func() {
		withObjs = nil
		withFuncs = interceptor.Funcs{}
		namespace = namespaceName

		ctx = context.Background()
		imgRef = vmopv1.VirtualMachineImageRef{
			Kind: "VirtualMachineImage",
			Name: vmiName,
		}

		vmi = builder.DummyVirtualMachineImage(vmiName)
		vmi.Namespace = namespaceName
		vmi.Spec.ProviderRef = &vmopv1common.LocalObjectRef{
			Name: cliName,
		}
		vmi.Status.Type = string(imgregv1a1.ContentLibraryItemTypeOvf)
		vmi.Status.Conditions = []metav1.Condition{
			{
				Type:   vmopv1.ReadyConditionType,
				Status: metav1.ConditionTrue,
			},
		}

		cli = &imgregv1a1.ContentLibraryItem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cliName,
				Namespace: namespaceName,
			},
			Spec: imgregv1a1.ContentLibraryItemSpec{
				UUID: cliName,
			},
			Status: imgregv1a1.ContentLibraryItemStatus{
				Cached:      true,
				SizeInBytes: *resource.NewQuantity(1024, resource.BinarySI),
				FileInfo: []imgregv1a1.FileInfo{
					{
						StorageURI: "ds://vmfs/volumes/123/my-image-disk-1.vmdk",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		withObjs = append(withObjs, vmi, cli)
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
					_, _ = vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				}).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
	})

	When("there is an error getting image", func() {
		BeforeEach(func() {
			expErr = errors.New("fake error")
			withFuncs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*vmopv1.VirtualMachineImage); ok {
					return expErr
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should return an error", func() {
			_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal(expErr.Error()))
		})
	})

	When("image fails checks", func() {
		When("image is not an OVF", func() {
			BeforeEach(func() {
				vmi.Status.Type = string(imgregv1a1.ContentLibraryItemTypeIso)
			})
			It("should return an error", func() {
				_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("is not OVF"))
			})
		})
		When("image is not ready", func() {
			BeforeEach(func() {
				vmi.Status.Conditions = []metav1.Condition{
					{
						Type:   vmopv1.ReadyConditionType,
						Status: metav1.ConditionFalse,
					},
				}
			})
			It("should return an error", func() {
				_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("image condition is not ready"))
			})
		})
		When("image provider is not ready", func() {
			When("provider ref is nil", func() {
				BeforeEach(func() {
					vmi.Spec.ProviderRef = nil
				})
				It("should return an error", func() {
					_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("image provider ref is empty"))
				})
			})
			When("provider ref name is empty", func() {
				BeforeEach(func() {
					vmi.Spec.ProviderRef.Name = ""
				})
				It("should return an error", func() {
					_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("image provider ref is empty"))
				})
			})
		})
	})

	When("there is an error getting content library item", func() {
		BeforeEach(func() {
			expErr = errors.New("fake error")
			withFuncs.Get = func(
				ctx context.Context,
				client ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if _, ok := obj.(*imgregv1a1.ContentLibraryItem); ok {
					return expErr
				}

				return client.Get(ctx, key, obj, opts...)
			}
		})
		It("should return an error", func() {
			_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal(expErr.Error()))
		})
	})

	When("library item is not synced", func() {
		When("library item is not cached", func() {
			BeforeEach(func() {
				cli.Status.Cached = false
			})
			It("should return an error", func() {
				_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("image not synced"))
			})
		})
		When("library item size is zero", func() {
			BeforeEach(func() {
				cli.Status.SizeInBytes = *resource.NewQuantity(0, resource.BinarySI)
			})
			It("should return an error", func() {
				_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("image not synced"))
			})
		})
	})

	When("there is an error getting disk URIs", func() {
		BeforeEach(func() {
			cli.Status.FileInfo = nil
		})
		It("should return an error", func() {
			_, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no vmdk files found in the content library item status"))
		})
	})

	When("no error is expected", func() {
		It("should return the correct disk info", func() {
			diskInfo, err := vmopv1util.GetImageDiskInfo(ctx, k8sClient, imgRef, namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(diskInfo.DiskURIs).To(HaveLen(1))
		})
	})

})

var _ = Describe("GetImage", func() {
	const (
		vmiName       = builder.DummyVMIName
		cvmiName      = builder.DummyCVMIName
		namespaceName = "fake"
	)

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		imgRef    vmopv1.VirtualMachineImageRef
		namespace string
		withObjs  []ctrlclient.Object
		withFuncs interceptor.Funcs
		vmi       *vmopv1.VirtualMachineImage
		cvmi      *vmopv1.ClusterVirtualMachineImage
		expErr    error
	)

	BeforeEach(func() {
		withObjs = nil
		withFuncs = interceptor.Funcs{}
		namespace = namespaceName

		ctx = context.Background()
		imgRef = vmopv1.VirtualMachineImageRef{
			Kind: "VirtualMachineImage",
			Name: vmiName,
		}

		vmi = builder.DummyVirtualMachineImage(vmiName)
		vmi.Namespace = namespaceName

		cvmi = builder.DummyClusterVirtualMachineImage(cvmiName)
	})

	JustBeforeEach(func() {
		withObjs = append(withObjs, vmi, cvmi)
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
					_, _ = vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				}).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				Expect(func() {
					_, _ = vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				}).To(PanicWith("k8sClient is nil"))
			})
		})
	})

	When("panic is not expected", func() {
		When("image kind is unknown", func() {
			BeforeEach(func() {
				imgRef = vmopv1.VirtualMachineImageRef{
					Kind: "bogus",
					Name: cvmiName,
				}
			})

			It("should return an error", func() {
				_, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).To(MatchError(`unsupported image kind: "bogus"`))
			})
		})

		When("image kind is VirtualMachineImage", func() {
			It("should return the correct object", func() {
				var obj vmopv1.VirtualMachineImage
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: vmi.Name, Namespace: vmi.Namespace}, &obj)).To(Succeed())
				expOut := obj

				img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(img).To(Equal(expOut))
			})
			When("image does not exist", func() {
				BeforeEach(func() {
					imgRef.Name = "vmi-bogus"
				})
				It("should return error", func() {
					_, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})
			})
			When("there is an error getting image", func() {
				BeforeEach(func() {
					expErr = errors.New("fake error")
					withFuncs.Get = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						key ctrlclient.ObjectKey,
						obj ctrlclient.Object,
						opts ...ctrlclient.GetOption) error {

						return expErr
					}
				})
				It("should return an error", func() {
					img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(Equal(expErr.Error()))
					Expect(img).To(Equal(vmopv1.VirtualMachineImage{}))
				})
			})
		})

		When("image kind is ClusterVirtualMachineImage", func() {
			BeforeEach(func() {
				imgRef = vmopv1.VirtualMachineImageRef{
					Kind: "ClusterVirtualMachineImage",
					Name: cvmiName,
				}
			})
			It("should return the correct object", func() {
				var obj vmopv1.ClusterVirtualMachineImage
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: cvmi.Name, Namespace: cvmi.Namespace}, &obj)).To(Succeed())
				expOut := vmopv1.VirtualMachineImage(obj)

				img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(img).To(Equal(expOut))
			})

			When("image does not exist", func() {
				BeforeEach(func() {
					imgRef.Name = "vmi-bogus"
				})
				It("should return error", func() {
					_, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})
			})

			When("there is an error getting image", func() {
				BeforeEach(func() {
					expErr = errors.New("fake error")
					withFuncs.Get = func(
						ctx context.Context,
						client ctrlclient.WithWatch,
						key ctrlclient.ObjectKey,
						obj ctrlclient.Object,
						opts ...ctrlclient.GetOption) error {

						return expErr
					}
				})
				It("should return an error", func() {
					img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(Equal(expErr.Error()))
					Expect(img).To(Equal(vmopv1.VirtualMachineImage{}))
				})
			})
		})

		When("image kind is unset", func() {
			Context("image name resolves to ClusterVirtualMachineImage", func() {
				BeforeEach(func() {
					imgRef = vmopv1.VirtualMachineImageRef{
						Kind: "",
						Name: cvmiName,
					}
				})
				It("should return the correct object", func() {
					var obj vmopv1.ClusterVirtualMachineImage
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: imgRef.Name}, &obj)).To(Succeed())
					expOut := vmopv1.VirtualMachineImage(obj)

					img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(img).To(Equal(expOut))
				})
			})

			Context("image name resolves to VirtualMachineImage", func() {
				BeforeEach(func() {
					imgRef = vmopv1.VirtualMachineImageRef{
						Kind: "",
						Name: vmiName,
					}
				})
				It("should return the correct object", func() {
					var obj vmopv1.VirtualMachineImage
					Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: imgRef.Name, Namespace: vmi.Namespace}, &obj)).To(Succeed())
					expOut := obj

					img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(img).To(Equal(expOut))
				})
			})

			Context("image name does not resolve to an image", func() {
				BeforeEach(func() {
					imgRef = vmopv1.VirtualMachineImageRef{
						Kind: "",
						Name: "vmi-bogus",
					}
				})
				It("should return error", func() {
					_, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
					Expect(err).To(HaveOccurred())
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
				})
			})
		})

		When("image kind is not valid", func() {
			BeforeEach(func() {
				imgRef = vmopv1.VirtualMachineImageRef{
					Kind: "Facsimile",
					Name: vmiName,
				}
			})
			It("should return an error", func() {
				img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).Should(HaveOccurred())
				Expect(img).To(Equal(vmopv1.VirtualMachineImage{}))
			})
		})
	})
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
						Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
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

var _ = Describe("EnsureImageCacheLocation", func() {
	const (
		namespace = "vmop-system"
		name      = "vmi-1"
		itemID    = "item-1"
		itemVer   = "version-1"
		dcID      = "datacenter-1"
		dsID      = "datastore-1"
		profileID = "profile-1"
	)

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		withFuncs interceptor.Funcs
		withObjs  []ctrlclient.Object
		key       ctrlclient.ObjectKey
		obj       vmopv1.VirtualMachineImageCache
		err       error
	)

	location := func(dsID string) vmopv1.VirtualMachineImageCacheLocationSpec {
		return vmopv1.VirtualMachineImageCacheLocationSpec{
			DatacenterID: dcID,
			DatastoreID:  dsID,
			ProfileID:    profileID,
		}
	}

	existingVMIC := func(locations ...vmopv1.VirtualMachineImageCacheLocationSpec) *vmopv1.VirtualMachineImageCache {
		return &vmopv1.VirtualMachineImageCache{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Spec: vmopv1.VirtualMachineImageCacheSpec{
				ProviderID:      itemID,
				ProviderVersion: itemVer,
				Locations:       locations,
			},
		}
	}

	// addLocationOutOfBand simulates another VM adding a location between
	// this call's read and its write.
	addLocationOutOfBand := func(
		ctx context.Context,
		c ctrlclient.Client,
		dsID string) {

		var other vmopv1.VirtualMachineImageCache
		ExpectWithOffset(1, c.Get(ctx, key, &other)).To(Succeed())
		other.AddLocation(dcID, dsID, profileID)
		ExpectWithOffset(1, c.Update(ctx, &other)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()
		withFuncs = interceptor.Funcs{}
		withObjs = nil
		key = ctrlclient.ObjectKey{Namespace: namespace, Name: name}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
		obj, err = vmopv1util.EnsureImageCacheLocation(
			ctx, k8sClient, key, itemID, itemVer, dcID, dsID, profileID)
	})

	getVMIC := func() vmopv1.VirtualMachineImageCache {
		var vmic vmopv1.VirtualMachineImageCache
		ExpectWithOffset(1, k8sClient.Get(ctx, key, &vmic)).To(Succeed())
		return vmic
	}

	When("the resource does not exist", func() {
		It("creates it with the location", func() {
			Expect(err).ToNot(HaveOccurred())
			vmic := getVMIC()
			Expect(vmic.Spec.ProviderID).To(Equal(itemID))
			Expect(vmic.Spec.ProviderVersion).To(Equal(itemVer))
			Expect(vmic.Spec.Locations).To(ConsistOf(location(dsID)))
			Expect(obj.Spec.Locations).To(ConsistOf(location(dsID)))
		})
	})

	When("the resource exists without the location", func() {
		BeforeEach(func() {
			withObjs = append(withObjs, existingVMIC(location("datastore-0")))
		})

		It("adds the location and keeps the existing ones", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(getVMIC().Spec.Locations).To(ConsistOf(
				location("datastore-0"),
				location(dsID)))
		})
	})

	When("the resource already has the location", func() {
		var patchCalls int

		BeforeEach(func() {
			patchCalls = 0
			withObjs = append(withObjs, existingVMIC(location(dsID)))
			withFuncs.Patch = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				patchCalls++
				return c.Patch(ctx, obj, patch, opts...)
			}
		})

		It("does not write the resource", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(patchCalls).To(BeZero())
			Expect(obj.Spec.Locations).To(ConsistOf(location(dsID)))
		})
	})

	When("another writer adds a location concurrently", func() {
		var patchCalls int

		BeforeEach(func() {
			patchCalls = 0
			withObjs = append(withObjs, existingVMIC(location("datastore-0")))
			withFuncs.Patch = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				obj ctrlclient.Object,
				patch ctrlclient.Patch,
				opts ...ctrlclient.PatchOption) error {

				patchCalls++
				if patchCalls == 1 {
					addLocationOutOfBand(ctx, c, "datastore-2")
				}
				return c.Patch(ctx, obj, patch, opts...)
			}
		})

		It("retries after the conflict and keeps both locations", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(patchCalls).To(Equal(2))
			Expect(getVMIC().Spec.Locations).To(ConsistOf(
				location("datastore-0"),
				location("datastore-2"),
				location(dsID)))
		})
	})

	When("another writer creates the resource concurrently", func() {
		var createCalls int

		BeforeEach(func() {
			createCalls = 0
			withFuncs.Create = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				obj ctrlclient.Object,
				opts ...ctrlclient.CreateOption) error {

				createCalls++
				if createCalls == 1 {
					ExpectWithOffset(1, c.Create(
						ctx,
						existingVMIC(location("datastore-2")))).To(Succeed())
				}
				return c.Create(ctx, obj, opts...)
			}
		})

		It("patches the created resource and keeps both locations", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(getVMIC().Spec.Locations).To(ConsistOf(
				location("datastore-2"),
				location(dsID)))
		})
	})

	When("getting the resource fails", func() {
		BeforeEach(func() {
			withFuncs.Get = func(
				ctx context.Context,
				c ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				return errors.New("fake get error")
			}
		})

		It("returns the error", func() {
			Expect(err).To(MatchError("fake get error"))
		})
	})

	DescribeTable("panics on invalid arguments",
		func(
			nilCtx, nilClient bool,
			key ctrlclient.ObjectKey,
			expPanic string) {

			callCtx, callClient := ctx, k8sClient
			if nilCtx {
				callCtx = nil
			}
			if nilClient {
				callClient = nil
			}
			Expect(func() {
				_, _ = vmopv1util.EnsureImageCacheLocation(
					callCtx, callClient, key, itemID, itemVer, dcID, dsID, profileID)
			}).To(PanicWith(expPanic))
		},
		Entry("nil context", true, false,
			ctrlclient.ObjectKey{Namespace: namespace, Name: name}, "context is nil"),
		Entry("nil client", false, true,
			ctrlclient.ObjectKey{Namespace: namespace, Name: name}, "k8sClient is nil"),
		Entry("empty namespace", false, false,
			ctrlclient.ObjectKey{Name: name}, "key.Namespace is empty"),
		Entry("empty name", false, false,
			ctrlclient.ObjectKey{Namespace: namespace}, "key.Name is empty"),
	)
})
