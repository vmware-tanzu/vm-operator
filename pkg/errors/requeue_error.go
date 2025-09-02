// // © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
	"fmt"
	"time"
)

// RequeueError may be returned from any part of a reconcile call stack and the
// controller should requeue the request. If After > 0 then the request is
// requeued with the provided value, otherwise the request is requeued
// immediately.
type RequeueError struct {
	After   time.Duration
	Message string
}

func (e RequeueError) Error() string {
	if e.Message == "" {
		if e.After == 0 {
			return "requeue immediately"
		}
		return fmt.Sprintf("requeue after %s", e.After)
	}
	if e.After == 0 {
		return "requeue immediately: " + e.Message
	}
	return fmt.Sprintf("requeue after %s: %s", e.After, e.Message)
}

// IsRequeueError returns true if the error or a nested error is a
// RequeueError.
func IsRequeueError(err error) bool {
	var requeue RequeueError
	return errors.As(err, &requeue)
}
