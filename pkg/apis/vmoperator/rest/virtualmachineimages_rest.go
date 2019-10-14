/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package rest

import (
	"context"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/klogr"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
)

// +k8s:deepcopy-gen=false
type VirtualMachineImagesREST struct {
	provider vmprovider.VirtualMachineProviderInterface
}

var log = klogr.New().WithName("vm-image-rest")

func NewVirtualMachineImagesREST(provider vmprovider.VirtualMachineProviderInterface) vmoperator.RestProvider {
	return vmoperator.RestProvider{
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
	log.Info("Listing VirtualMachineImages", "namespace", namespace, "options", options, "user", user)

	// namespace == "" means vmimages for ALL namespaces are requested
	if namespace == "" {
		// listing for the default namespace only: it's the same list for any other namespace anyway
		namespace = "default"
	}

	images, err := r.provider.ListVirtualMachineImages(ctx, namespace)
	if err != nil {
		log.Error(err, "Failed to list images")
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
	log.Info("Getting VirtualMachineImage", "name", name, "options", options)

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

	log.Info("Getting VirtualMachineImage", "name", name, "namespace", namespace, "user", user)

	image, err := r.provider.GetVirtualMachineImage(ctx, namespace, name)
	if err != nil {
		log.Error(err, "Failed to get image")
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
