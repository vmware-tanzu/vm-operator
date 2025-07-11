// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

	if isNil(e.ObjectOld) {
		return false
	}
	if isNil(e.ObjectNew) {
		return false
	}
	return e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
}

func isNil(arg any) bool {
	if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
		v.Kind() == reflect.Interface ||
		v.Kind() == reflect.Slice ||
		v.Kind() == reflect.Map ||
		v.Kind() == reflect.Chan ||
		v.Kind() == reflect.Func) && v.IsNil()) {
		return true
	}
	return false
}
