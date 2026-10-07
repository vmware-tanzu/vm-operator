// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

const (
	// transientRetryAttempts and transientRetryDelay bound how long a single
	// vCenter call keeps retrying a transient failure. They are deliberately
	// short: the goal is to ride out a network blip or a briefly overloaded
	// vCenter, not to mask an outage.
	transientRetryAttempts = 5
	transientRetryDelay    = time.Second
)

// IsTransientError reports whether err from a vCenter call looks like a
// transient connectivity or load problem (connection reset, EOF, timeout,
// HTTP 429/502/503/504) rather than a real failure that would just fail again.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Go does not classify ECONNRESET/ECONNABORTED/EPIPE as Temporary()
	// outside of accept, so vim25.IsTemporaryNetworkError misses real
	// connection resets; check for them explicitly.
	if vim25.IsTemporaryNetworkError(err) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() || urlErr.Temporary() {
			return true
		}

		for _, code := range []int{
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		} {
			// govmomi's HTTP status error is unexported; its message is the
			// response status, e.g. "429 Too Many Requests".
			if strings.HasPrefix(urlErr.Err.Error(), strconv.Itoa(code)+" ") {
				return true
			}
		}
	}

	return false
}

// WithTransientRetry wraps rt so every SOAP call made through it is retried
// when it fails with a transient error (see IsTransientError). Use it for any
// vCenter client the e2e tests create instead of calling vCenter bare.
func WithTransientRetry(rt soap.RoundTripper) soap.RoundTripper {
	return vim25.Retry(rt, func(err error) (bool, time.Duration) {
		return IsTransientError(err), transientRetryDelay
	}, transientRetryAttempts)
}

// RetryTransient runs fn, retrying it while it returns a transient error (see
// IsTransientError), for vCenter work that isn't a single SOAP call -- e.g.
// REST/HTTP requests or a multi-step setup that has to be redone as a unit.
func RetryTransient(ctx context.Context, fn func() error) error {
	var err error

	for attempt := 0; attempt < transientRetryAttempts; attempt++ {
		if err = fn(); err == nil || !IsTransientError(err) {
			return err
		}

		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-time.After(transientRetryDelay):
		}
	}

	return err
}
