// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
)

// NoRequeueError may be returned from any part of a reconcile call stack and the
// controller will not requeue the request. This can be used to return an error
// but not cause the back-off retry to occur.
type NoRequeueError struct {
	Message string

	// DoNotErr may be set to true in order to prevent a reconciler from
	// returning a TerminalError. This is helpful when the error is intended to
	// stop the reconciliation loop, but not intended to be logged as an error.
	DoNotErr bool
}

func (e NoRequeueError) Error() string {
	if e.Message == "" {
		return "no requeue"
	}
	return e.Message
}

// IsNoRequeueError returns true if the error or a nested error is a
// NoRequeueError.
func IsNoRequeueError(err error) bool {
	var noRequeue NoRequeueError
	return errors.As(err, &noRequeue)
}

// IsNoRequeueNoError returns true if the error or a nested error is a
// NoRequeueError with DoNotErr=true.
func IsNoRequeueNoError(err error) bool {
	var noRequeue NoRequeueError
	if !errors.As(err, &noRequeue) {
		return false
	}
	return noRequeue.DoNotErr
}

// NoRequeueNoErr returns a NoRequeueErr with DoNotErr=true.
func NoRequeueNoErr(msg string) error {
	return NoRequeueError{Message: msg, DoNotErr: true}
}
