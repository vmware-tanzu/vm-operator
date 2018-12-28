package v1beta1

import (
	"k8s.io/apimachinery/pkg/watch"
	"log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
)

// +k8s:deepcopy-gen=false
type VirtualMachineImagesREST struct {
}

// Provide a Read only interface for now
var _ rest.Getter = &VirtualMachineImagesREST{}
var _ rest.Lister = &VirtualMachineImagesREST{}
var _ rest.Watcher = &VirtualMachineImagesREST{}

func (r *VirtualMachineImagesREST) Create(ctx request.Context, obj runtime.Object) (runtime.Object, error) {
	return nil, nil
}

func (r *VirtualMachineImagesREST) NewList() runtime.Object {
	return &VirtualMachineImage{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *VirtualMachineImagesREST) List(ctx genericapirequest.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	return NewVirtualMachineImageFake(), nil
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *VirtualMachineImagesREST) Get(ctx request.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	log.Printf("Validating fields for VirtualMachineImage")
	return nil, nil
}

func (r *VirtualMachineImagesREST) Watch(ctx genericapirequest.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

// Update alters the status subset of an object.
func (r *VirtualMachineImagesREST) Update(ctx request.Context, name string, objInfo rest.UpdatedObjectInfo) (runtime.Object, bool, error) {
	return nil, false, nil
}

func (r *VirtualMachineImagesREST) New() runtime.Object {
	return &VirtualMachineImage{}
}

func (r *VirtualMachineImagesREST) NamespaceScoped() bool {
	return true
}



