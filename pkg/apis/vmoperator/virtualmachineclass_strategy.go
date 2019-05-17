/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"context"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Validate checks that an instance of VirtualMachineClass is well formed
func (VirtualMachineClassStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	class := obj.(*VirtualMachineClass)
	klog.V(4).Infof("Validating fields for VirtualMachineClass %s/%s", class.Namespace, class.Name)
	errors := field.ErrorList{}
	return errors
}
