// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
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
