// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// RequeueError may be returned from any part of a reconcile call stack and the
// controller should requeue the request. If After > 0 then the request is
// requeued with the provided value, otherwise the request is requeued
// immediately.
type RequeueError struct {
	After time.Duration
}

func (e RequeueError) Error() string {
	if e.After == 0 {
		return "requeue immediately"
	}
	return fmt.Sprintf("requeue after %s", e.After)
}

// ResultFromError returns a ReconcileResult based on the provided error. If
// the error contains an embedded RequeueError, then it is used to influence
// the result.
func ResultFromError(err error) (ctrl.Result, error) {
	var dst RequeueError
	if err != nil && errors.As(err, &dst) {
		if dst.After == 0 {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{RequeueAfter: dst.After}, nil
	}
	return ctrl.Result{}, err
}
