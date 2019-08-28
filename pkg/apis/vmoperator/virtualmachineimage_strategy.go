/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"

	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/klog/klogr"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func NewVirtualMachineImagesREST(getter generic.RESTOptionsGetter) rest.Storage {
	return GetRestProvider().ImagesProvider
}

// Validate checks that an instance of VirtualMachineImage is well formed
func (VirtualMachineImageStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	image := obj.(*VirtualMachineImage)

	log := klogr.New()
	log.V(4).Info("Validating fields for VirtualMachineImage", "namespace", image.Namespace, "name", image.Name)
	errors := field.ErrorList{}
	return errors
}
