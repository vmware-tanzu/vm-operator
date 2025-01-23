// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// ErrImageNotSynced is returned from GetImageInfo when the underlying content
// library item is not synced.
var ErrImageNotSynced = errors.New("image not synced")

// ImageDiskInfo contains information about a VM image that is used to create a VM.
type ImageDiskInfo struct {
	ItemID             string
	ItemContentVersion string
	DiskURIs           []string
}

// GetImageDiskInfo returns the information about a VM image's disks, used to
// create a VM.
// This method returns an error for images that are not OVFs.
func GetImageDiskInfo(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	imgRef vmopv1.VirtualMachineImageRef,
	namespace string) (ImageDiskInfo, error) {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(k8sClient) {
		panic("k8sClient is nil")
	}

	img, err := GetImage(ctx, k8sClient, imgRef, namespace)
	if err != nil {
		return ImageDiskInfo{}, err
	}

	if err := IsImageOVF(img); err != nil {
		return ImageDiskInfo{}, err
	}
	if err := IsImageReady(img); err != nil {
		return ImageDiskInfo{}, err
	}
	if err := IsImageProviderReady(img); err != nil {
		return ImageDiskInfo{}, err
	}

	item, err := GetContentLibraryItemForImage(ctx, k8sClient, img)
	if err != nil {
		return ImageDiskInfo{}, err
	}
	if err := IsLibraryItemSynced(item); err != nil {
		return ImageDiskInfo{}, err
	}

	diskURIs, err := GetStorageURIsForLibraryItemDisks(item)
	if err != nil {
		return ImageDiskInfo{}, err
	}

	return ImageDiskInfo{
		DiskURIs:           diskURIs,
		ItemID:             string(item.Spec.UUID),
		ItemContentVersion: item.Status.ContentVersion,
	}, nil
}

// GetImage returns the VirtualMachineImage or ClusterVirtualMachineImage for
// the provided image reference.
func GetImage(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	imgRef vmopv1.VirtualMachineImageRef,
	namespace string) (vmopv1.VirtualMachineImage, error) {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(k8sClient) {
		panic("k8sClient is nil")
	}

	var obj vmopv1.VirtualMachineImage

	switch imgRef.Kind {
	case vmiKind:
		// Namespace scope image.
		if err := k8sClient.Get(
			ctx,
			ctrlclient.ObjectKey{
				Name:      imgRef.Name,
				Namespace: namespace,
			},
			&obj); err != nil {

			return vmopv1.VirtualMachineImage{}, err
		}
	case cvmiKind:
		// Cluster scope image.
		var obj2 vmopv1.ClusterVirtualMachineImage
		if err := k8sClient.Get(
			ctx,
			ctrlclient.ObjectKey{
				Name: imgRef.Name,
			}, &obj2); err != nil {

			return vmopv1.VirtualMachineImage{}, err
		}
		obj = vmopv1.VirtualMachineImage(obj2)
	default:
		return vmopv1.VirtualMachineImage{},
			fmt.Errorf("unsupported image kind: %q", imgRef.Kind)
	}

	return obj, nil
}

func IsImageReady(img vmopv1.VirtualMachineImage) error {
	if !pkgcnd.IsTrue(&img, vmopv1.ReadyConditionType) {
		return fmt.Errorf(
			"image condition is not ready: %v",
			pkgcnd.Get(&img, vmopv1.ReadyConditionType))
	}
	if img.Spec.ProviderRef == nil || img.Spec.ProviderRef.Name == "" {
		return errors.New("image provider ref is empty")
	}
	return nil
}

func IsImageOVF(img vmopv1.VirtualMachineImage) error {
	if img.Status.Type != string(imgregv1a1.ContentLibraryItemTypeOvf) {
		return fmt.Errorf(
			"image type %q is not OVF", img.Status.Type)
	}
	return nil
}

func IsImageProviderReady(img vmopv1.VirtualMachineImage) error {
	if img.Spec.ProviderRef == nil {
		return errors.New("image provider ref is nil")
	}
	if img.Spec.ProviderRef.Name == "" {
		return errors.New("image provider ref name is empty")
	}
	return nil
}

func IsLibraryItemSynced(item imgregv1a1.ContentLibraryItem) error {
	if !item.Status.Cached || item.Status.SizeInBytes.IsZero() {
		return ErrImageNotSynced
	}
	return nil
}

func GetContentLibraryItemForImage(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	img vmopv1.VirtualMachineImage) (imgregv1a1.ContentLibraryItem, error) {

	if pkgutil.IsNil(ctx) {
		panic("context is nil")
	}
	if pkgutil.IsNil(k8sClient) {
		panic("k8sClient is nil")
	}

	var obj imgregv1a1.ContentLibraryItem

	if img.Namespace != "" {
		// Namespace scope ContentLibraryItem.
		if err := k8sClient.Get(
			ctx,
			ctrlclient.ObjectKey{
				Name:      img.Spec.ProviderRef.Name,
				Namespace: img.Namespace,
			},
			&obj); err != nil {

			return imgregv1a1.ContentLibraryItem{}, err
		}
	} else {
		// Cluster scope ClusterContentLibraryItem.
		var obj2 imgregv1a1.ClusterContentLibraryItem
		if err := k8sClient.Get(
			ctx,
			ctrlclient.ObjectKey{Name: img.Spec.ProviderRef.Name},
			&obj2); err != nil {

			return imgregv1a1.ContentLibraryItem{}, err
		}
		obj = imgregv1a1.ContentLibraryItem(obj2)
	}

	return obj, nil
}

// GetStorageURIsForLibraryItemDisks returns the paths to the VMDK files from
// the provided library item.
func GetStorageURIsForLibraryItemDisks(
	item imgregv1a1.ContentLibraryItem) ([]string, error) {

	var storageURIs []string
	for i := range item.Status.FileInfo {
		fi := item.Status.FileInfo[i]
		if fi.StorageURI != "" {
			if strings.EqualFold(path.Ext(fi.StorageURI), ".vmdk") {
				storageURIs = append(storageURIs, fi.StorageURI)
			}
		}
	}
	if len(storageURIs) == 0 {
		return nil, fmt.Errorf(
			"no vmdk files found in the content library item status: %v",
			item.Status)
	}
	return storageURIs, nil
}

// VirtualMachineImageCacheToItemMapper returns a mapper function used to
// enqueue reconcile requests for resources in response to an event on
// a VirtualMachineImageCache resource.
func VirtualMachineImageCacheToItemMapper(
	ctx context.Context,
	logger logr.Logger,
	k8sClient ctrlclient.Client,
	groupVersion schema.GroupVersion,
	kind string) handler.MapFunc {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if groupVersion.Empty() {
		panic("groupVersion is empty")
	}
	if kind == "" {
		panic("kind is empty")
	}

	gvkString := groupVersion.WithKind(kind).String()
	listGVK := groupVersion.WithKind(kind + "List")

	// For a given VirtualMachineImageCache, return reconcile requests for
	// resources that have the label pkgconst.VMICacheLabelKey with a value set
	// to the name of a VMI cache resource.
	return func(ctx context.Context, o ctrlclient.Object) []reconcile.Request {
		if ctx == nil {
			panic("context is nil")
		}
		if o == nil {
			panic("object is nil")
		}
		obj, ok := o.(*vmopv1.VirtualMachineImageCache)
		if !ok {
			panic(fmt.Sprintf("object is %T", o))
		}

		// Do not reconcile anything if the referred object is being deleted.
		if !obj.DeletionTimestamp.IsZero() {
			return nil
		}

		// Do not reconcile anything unless the OVF for this VMI Cache resource
		// is ready.
		if !pkgcnd.IsTrue(obj, vmopv1.VirtualMachineImageCacheConditionOVFReady) {
			return nil
		}

		logger := logger.WithValues(
			"name", o.GetName(),
			"namespace", o.GetNamespace())
		logger.V(4).Info(
			"Reconciling all resources referencing an VirtualMachineImageCache",
			"resourceGVK", gvkString)

		list := unstructured.UnstructuredList{
			Object: map[string]interface{}{},
		}
		list.SetGroupVersionKind(listGVK)

		// Find all the referrers.
		if err := k8sClient.List(
			ctx,
			&list,
			ctrlclient.MatchingLabels(map[string]string{
				pkgconst.VMICacheLabelKey: obj.Name,
			})); err != nil {

			if !apierrors.IsNotFound(err) {
				logger.Error(
					err,
					"Failed to list resources due to VirtualMachineImageCache watch",
					"resourceGVK", gvkString)
			}
			return nil
		}

		// Populate reconcile requests for referrers.
		var requests []reconcile.Request
		for i := range list.Items {
			requests = append(
				requests,
				reconcile.Request{
					NamespacedName: ctrlclient.ObjectKey{
						Namespace: list.Items[i].GetNamespace(),
						Name:      list.Items[i].GetName(),
					},
				})
		}

		if len(requests) > 0 {
			logger.V(4).Info(
				"Reconciling resources due to VirtualMachineImageCache watch",
				"resourceGVK", gvkString,
				"requests", requests)
		}

		return requests
	}
}
