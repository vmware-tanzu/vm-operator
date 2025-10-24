// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	pkgnil "github.com/vmware-tanzu/vm-operator/pkg/util/nil"
)

// TypedResourceVersionChangedPredicate implements a default update predicate
// function on resource version change.
type TypedResourceVersionChangedPredicate[T metav1.Object] struct {
	predicate.TypedFuncs[T]
}

// Update implements default UpdateEvent filter for validating resource version
// change.
func (TypedResourceVersionChangedPredicate[T]) Update(
	e event.TypedUpdateEvent[T]) bool {

	if pkgnil.IsNil(e.ObjectOld) {
		return false
	}
	if pkgnil.IsNil(e.ObjectNew) {
		return false
	}
	return e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
}
