// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package testutil

// Contains common asserts made by unit tests for Reconcile() methods.

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/gomega"
)

// ExpectReconcilerToNotRequeueAndNotError is a helper method that asserts that the given Reconciler does not
// experience an error or result in a requeue when reconciling the given object.
func ExpectReconcilerToNotRequeueAndNotError(r reconcile.Reconciler, namespacedName types.NamespacedName) {
	reconcileResult, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: namespacedName})
	Expect(err).NotTo(HaveOccurred())
	Expect(reconcileResult).To(Equal(reconcile.Result{}))
}

// ExpectReconcileToRequeueWithError is a helper method that asserts that the given Reconciler returns an error
// when reconciling the given object.
// Takes an optional third argument (represented by the variadic argument) - if present, it attempts to match that specific error using the custom matcher that checks for substring presence in the error.
func ExpectReconcileToRequeueWithError(r reconcile.Reconciler, namespacedName types.NamespacedName, expectedErr ...interface{}) {
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: namespacedName})
	if len(expectedErr) < 1 {
		Expect(err).To(HaveOccurred())
		return
	}
	Expect(err).To(WrapError(expectedErr[0]))
}
