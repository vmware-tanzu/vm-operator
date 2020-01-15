// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// VirtualMachineImages are cluster-scoped
func (VirtualMachineImageStrategy) NamespaceScoped() bool {
	return false
}

// VirtualMachineImagesStatus are cluster-scoped
func (VirtualMachineImageStatusStrategy) NamespaceScoped() bool {
	return false
}

// Validate checks that an instance of VirtualMachineImage is well formed
func (VirtualMachineImageStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	image := obj.(*VirtualMachineImage)

	log := logf.Log.WithName("virtual-machine-image-strategy")
	log.V(4).Info("Validating fields for VirtualMachineImage", "namespace", image.Namespace, "name", image.Name)
	errors := field.ErrorList{}
	return errors
}
