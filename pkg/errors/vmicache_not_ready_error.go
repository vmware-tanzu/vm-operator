// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

// VMICacheNotReadyError is returned from a method that cannot proceed until a
// VirtualMachineImageCache object is not ready.
type VMICacheNotReadyError struct {
	// Name of the VirtualMachineImageCache object.
	Name string
}

func (e VMICacheNotReadyError) Error() string {
	return "cache not ready"
}

// WatchVMICacheIfNotReady adds a label to the provided object that allows it
// to be reconciled when a VMI Cache object is updated. This occurs when the
// provided error is or contains an ErrVMICacheNotReady error.
// This function returns true if a label was added to the object, otherwise
// false is returned.
func WatchVMICacheIfNotReady(err error, obj metav1.Object) bool {
	if err == nil || obj == nil {
		return false
	}
	var e VMICacheNotReadyError
	if !errors.As(err, &e) {
		return false
	}
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[pkgconst.VMICacheLabelKey] = e.Name
	obj.SetLabels(labels)
	return true
}
