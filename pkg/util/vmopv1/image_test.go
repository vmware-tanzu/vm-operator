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

	When("there is an error gettting disk URIs", func() {
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
		expOut    vmopv1.VirtualMachineImage
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
		When("image kind is VirtualMachineImage", func() {
			It("should return the correct object", func() {
				var obj vmopv1.VirtualMachineImage
				Expect(k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: vmi.Name, Namespace: vmi.Namespace}, &obj)).To(Succeed())
				expOut = obj

				img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(img).To(Equal(expOut))
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
				expOut = vmopv1.VirtualMachineImage(obj)

				img, err := vmopv1util.GetImage(ctx, k8sClient, imgRef, namespace)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(img).To(Equal(expOut))
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
