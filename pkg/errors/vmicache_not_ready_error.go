// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

// VMICacheNotReadyError is returned from a method that cannot proceed until a
// VirtualMachineImageCache object is not ready.
type VMICacheNotReadyError struct {
	// Message is returned by the Error function. If empty, the Error function
	// returns "cache not ready".
	Message string

	// Name of the VirtualMachineImageCache object.
	Name string

	// DatacenterID describes the ID of the datacenter that does not have the
	// cached disks.
	// This field is only non-empty when the error is returned when waiting on
	// disks to be cached.
	DatacenterID string

	// DatastoreID describes the ID of the datastore that does not have the
	// cached disks.
	// This field is only non-empty when the error is returned when waiting on
	// disks to be cached.
	DatastoreID string
}

func (e VMICacheNotReadyError) Error() string {
	if e.Message == "" {
		return "cache not ready"
	}
	return e.Message
}

// WatchVMICacheIfNotReady checks if the provided error contains an
// ErrVMICacheNotReady error. If so, a label, and optionally two annotations,
// are added to the provided object that allow it to be reconciled when the
// VMI Cache object is updated.
// This function returns true if the specified error contains an
// ErrVMICacheNotReady error, otherwise false is returned.
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

	if dcID, dsID := e.DatacenterID, e.DatastoreID; dcID != "" || dsID != "" {
		annos := obj.GetAnnotations()
		if annos == nil {
			annos = map[string]string{}
		}
		annos[pkgconst.VMICacheLocationAnnotationKey] = fmt.Sprintf(
			"%s,%s", dcID, dsID)
		obj.SetAnnotations(annos)
	}

	return true
}
