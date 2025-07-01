// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// SyncGVKToObject synchronizes the group, version, and kind for a given object
// back into the object by looking up the information from the provided scheme.
//
// For more information, please see Update 1 from the following issue:
// https://github.com/kubernetes-sigs/controller-runtime/issues/2382.
func SyncGVKToObject(obj runtime.Object, scheme *runtime.Scheme) error {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return nil
}

// MustSyncGVKToObject synchronizes the group, version, and kind for a given
// object back into the object by looking up the information from the provided
// scheme.
//
// For more information, please see Update 1 from the following issue:
// https://github.com/kubernetes-sigs/controller-runtime/issues/2382.
func MustSyncGVKToObject(obj runtime.Object, scheme *runtime.Scheme) {
	if err := SyncGVKToObject(obj, scheme); err != nil {
		panic(err)
	}
}
