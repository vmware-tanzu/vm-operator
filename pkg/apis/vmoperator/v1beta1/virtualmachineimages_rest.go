/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1beta1

import (
	"context"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"vmware.com/kubevsphere/pkg/vmprovider"
)

// +k8s:deepcopy-gen=false
type VirtualMachineImagesREST struct {}

// Provide a Read only interface for now
var _ rest.Getter = &VirtualMachineImagesREST{}
var _ rest.Lister = &VirtualMachineImagesREST{}
var _ rest.Watcher = &VirtualMachineImagesREST{}


func (r *VirtualMachineImagesREST) NewList() runtime.Object {
	return &VirtualMachineImageList{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *VirtualMachineImagesREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	glog.Info("Listing VirtualMachineImage")

	// Get the namespace from the context (populated from the URL).
	// The namespace in the object can be empty until StandardStorage.Create()->BeforeCreate() populates it from the context.
	namespace, ok := genericapirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	user, ok := genericapirequest.UserFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("user is required")
	}

	glog.Infof("Listing VirtualMachineImage ns=%s, user=%s", namespace, user)

	vmprovider, err := vmprovider.NewVmProvider(namespace)
	if err != nil {
		glog.Errorf("Failed to find vmprovider: %s", err)
		return nil, errors.NewBadRequest("namespace is invalid")
	}

	imagesProvider, supported := vmprovider.VirtualMachineImages()
	if !supported {
		glog.Error("Provider doesn't support images func")
		return nil, errors.NewMethodNotSupported(schema.GroupResource{"vmoperator", "VirtualMachineImages"}, "list")
	}

	images, err := imagesProvider.ListVirtualMachineImages(ctx, namespace)
	if err != nil {
		glog.Errorf("Failed to list images: %s", err)
		return nil, errors.NewInternalError(err)
	}

	var convertImages = func() *VirtualMachineImageList {
		newImages := []VirtualMachineImage{}
		for _, image := range images {
			ni := VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: image.Name,
					Namespace: namespace,
				},
				Spec: VirtualMachineImageSpec{},
				Status: VirtualMachineImageStatus{
					Uuid: image.Uuid,
					PowerState: image.PowerState,
					InternalId: image.InternalId,
				},
			}
			newImages = append(newImages, ni)
		}
		return &VirtualMachineImageList{Items: newImages}
	}

	converted := convertImages()
	glog.Infof("Images: %s", converted)
	return converted, nil
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *VirtualMachineImagesREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	glog.Infof("Getting VirtualMachineImage %s", name)

	// Get the namespace from the context (populated from the URL).
	// The namespace in the object can be empty until StandardStorage.Create()->BeforeCreate() populates it from the context.
	namespace, ok := genericapirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	user, ok := genericapirequest.UserFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("user is required")
	}

	glog.Infof("Getting VirtualMachineImage name=%s, ns=%s, user=%s", name, namespace, user)

	vmprovider, err := vmprovider.NewVmProvider(namespace)
	if err != nil {
		glog.Errorf("Failed to find vmprovider: %s", err)
		return nil, errors.NewBadRequest("namespace is invalid")
	}

	imagesProvider, supported := vmprovider.VirtualMachineImages()
	if !supported {
		glog.Error("Provider doesn't support images func")
		return nil, errors.NewMethodNotSupported(schema.GroupResource{"vmoperator", "VirtualMachineImages"}, "list")
	}

	image, err := imagesProvider.GetVirtualMachineImage(ctx, name)
	if err != nil {
		glog.Errorf("Failed to list images: %s", err)
		return nil, errors.NewInternalError(err)
	}

	var convertImage = func() *VirtualMachineImage {
		return &VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: image.Name,
				Namespace: namespace,
			},
			Spec: VirtualMachineImageSpec{},
			Status: VirtualMachineImageStatus{
				Uuid: image.Uuid,
				PowerState: image.PowerState,
				InternalId: image.InternalId,
			},
		}
	}

	converted := convertImage()
	glog.Infof("Image: %s", converted)
	return converted, nil
}

func (r *VirtualMachineImagesREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

func (r *VirtualMachineImagesREST) New() runtime.Object {
	return &VirtualMachineImage{}
}

func (r *VirtualMachineImagesREST) NamespaceScoped() bool {
	return true
}

