// // © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ResultFromError returns a ReconcileResult based on the provided error. If
// the error contains an embedded RequeueError or NoRequeueError, then it is
// used to influence the result. An embedded RequeueError is favored in an
// error that also contains a NoRequeueError.
// An embedded NoRequeueError will return a controller-runtime TerminalError
// that will be logged and counted as an error but does not retry.
func ResultFromError(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeue RequeueError
	if errors.As(err, &requeue) {
		if requeue.After == 0 {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{RequeueAfter: requeue.After}, nil
	}

	var norequeue NoRequeueError
	if errors.As(err, &norequeue) {
		if norequeue.DoNotErr {
			return ctrl.Result{}, nil
		}

		// TerminalError is confusingly named: it won't cause an error retry to
		// be enqueued but later events from like a watch will still be queued.
		// Wrap the original error so any other wrapped errors are still there.
		return ctrl.Result{}, reconcile.TerminalError(err)
	}

	return ctrl.Result{}, err
}
