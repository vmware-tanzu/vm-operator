/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package rest

import (
	"context"

	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/vmprovider/iface"
)

// +k8s:deepcopy-gen=false
type VirtualMachineImagesREST struct {
	provider iface.VirtualMachineProviderInterface
}

func NewVirtualMachineImagesREST(provider iface.VirtualMachineProviderInterface) v1alpha1.RestProvider {
	return v1alpha1.RestProvider{
		ImagesProvider: &VirtualMachineImagesREST{provider},
	}
}

// Provide a Read only interface for now.
var _ rest.Getter = &VirtualMachineImagesREST{}
var _ rest.Lister = &VirtualMachineImagesREST{}
var _ rest.Watcher = &VirtualMachineImagesREST{}

func (r *VirtualMachineImagesREST) NewList() runtime.Object {
	return &v1alpha1.VirtualMachineImageList{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
// TODO(bryanv) Honor ListOptions?
func (r *VirtualMachineImagesREST) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	glog.Infof("Listing VirtualMachineImages options: %v", options)

	// Get the namespace from the context (populated from the URL). The namespace in the object can
	// be empty until StandardStorage.Create()->BeforeCreate() populates it from the context.
	namespace, ok := genericapirequest.NamespaceFrom(ctx)
	if !ok {
		return nil, errors.NewBadRequest("namespace is required")
	}

	// TODO: Determine how we can pass a user from our integration tests
	// TODO: Until then, just ignore the user
	user, _ := genericapirequest.UserFrom(ctx)
	//if !ok {
	//return nil, errors.NewBadRequest("user is required")
	//}

	glog.Infof("Listing VirtualMachineImage ns=%s, user=%s", namespace, user)

	images, err := r.provider.ListVirtualMachineImages(ctx, namespace)
	if err != nil {
		glog.Errorf("Failed to list images: %s", err)
		return nil, errors.NewInternalError(err)
	}

	var items []v1alpha1.VirtualMachineImage
	for _, item := range images {
		items = append(items, *item)
	}

	return &v1alpha1.VirtualMachineImageList{
		Items: items,
	}, nil
}

func (r *VirtualMachineImagesREST) New() runtime.Object {
	return &v1alpha1.VirtualMachineImage{}
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

	// TODO: Determine how we can pass a user from our integration tests
	// TODO: Until then, just ignore the user
	user, _ := genericapirequest.UserFrom(ctx)
	//if !ok {
	//	return nil, errors.NewBadRequest("user is required")
	//}

	glog.Infof("Getting VirtualMachineImage name=%s, ns=%s, user=%s", name, namespace, user)

	image, err := r.provider.GetVirtualMachineImage(ctx, name)
	if err != nil {
		glog.Errorf("Failed to list images: %s", err)
		return nil, errors.NewInternalError(err) // TODO(bryanv) Do not convert NotFound errors?
	}

	return image, nil
}

func (r *VirtualMachineImagesREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

func (r *VirtualMachineImagesREST) NamespaceScoped() bool {
	return true
}
