// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const (
	// TransientRetryInterval and TransientRetryTimeout bound how long any e2e
	// retry path -- the controller-runtime client retry below, and the
	// kubectl-based retry in vmservice/common/vmservice_clusterproxy.go --
	// keeps retrying a call that fails with a transient connectivity error,
	// e.g. the conversion webhook being briefly unreachable during a
	// vm-operator pod rollout, rather than a real admission/validation
	// failure. Shared so both paths behave the same way against the same
	// class of outage instead of drifting apart.
	TransientRetryInterval = 15 * time.Second
	TransientRetryTimeout  = 4 * time.Minute
)

// RetryableTransientErrSubstrings are lower-cased substrings of arbitrary
// error/output text that indicate a transient failure talking to the
// supervisor API server, or a webhook it calls out to, rather than a real
// error that would just fail again.
//
// This is the single shared list for every e2e retry path that has to tell
// "transient connectivity blip" apart from "real failure" -- both the
// controller-runtime client retry below and the kubectl-based retry in
// vmservice/common/vmservice_clusterproxy.go use it, so a new signature only
// needs to be added once. The kubectl path matches it against raw
// stdout/stderr text; IsRetryableTransientErrString below matches it against
// a Go error's message for the client path.
var RetryableTransientErrSubstrings = []string{
	"dial tcp",
	"connection refused",
	"connection reset by peer",
	"i/o timeout",
	"tls handshake timeout",
	"unexpected eof",
	"broken pipe",
	"http2: server sent goaway",
	"http2: client connection lost",
	"context deadline exceeded",
	"no endpoints available for service",
	"unable to connect to the server",
	"error trying to reach service",
	"the server is currently unable to handle the request",
}

// IsRetryableTransientErrString reports whether msg contains one of
// RetryableTransientErrSubstrings, case-insensitively.
func IsRetryableTransientErrString(msg string) bool {
	msg = strings.ToLower(msg)
	for _, substr := range RetryableTransientErrSubstrings {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}

// isRetryableClientError reports whether err from a controller-runtime client
// call looks like a transient connectivity failure -- as opposed to a real
// error from the API server or an admission/validation webhook.
//
// A conversion webhook that is briefly unreachable surfaces as a typed
// apierrors.IsInternalError (the API server wraps the dial failure in
// "Internal error occurred: ..."), which is why that check comes first; the
// substring list is the fallback for raw transport errors that never reach a
// typed apierrors.APIStatus, e.g. the apiserver connection itself timing out.
func isRetryableClientError(err error) bool {
	if err == nil {
		return false
	}

	if apierrors.IsInternalError(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) {
		return true
	}

	if IsRetryableTransientErrString(err.Error()) {
		return true
	}

	return false
}

// alreadySucceededFunc reports whether a retried write actually landed on a
// previous attempt before its connection dropped, e.g. Create sees
// AlreadyExists, Delete sees NotFound.
type alreadySucceededFunc func(err error) bool

func createAlreadySucceeded(err error) bool { return apierrors.IsAlreadyExists(err) }
func deleteAlreadySucceeded(err error) bool { return apierrors.IsNotFound(err) }

// retryOnTransientError runs fn, retrying every TransientRetryInterval for up
// to TransientRetryTimeout, but only while fn's error is classified as a transient
// connectivity failure by isRetryableClientError. Any other error is
// returned immediately.
//
// succeeded, when non-nil, is checked on attempts after the first: if fn
// fails but succeeded reports the write already landed (e.g. AlreadyExists on
// a retried Create, NotFound on a retried Delete), the earlier failure is
// treated as success rather than surfaced as a hard error -- the object may
// have been persisted before the connection reporting the failure dropped.
// On the first attempt, that same signal is a real failure (a name
// collision, or a delete target that never existed) and is not suppressed.
//
// Update and Patch intentionally pass a nil succeeded: a Conflict on a
// retried Update/Patch is ambiguous -- it can mean the earlier attempt
// landed, or that someone else wrote to the object -- so it is surfaced as a
// real error rather than guessed at. Call sites that mutate through
// Update/Patch already re-fetch and retry at a higher level (see
// lib/vmoperator), which self-heals a spurious conflict from this case.
func retryOnTransientError(ctx context.Context, op string, succeeded alreadySucceededFunc, fn func() error) error {
	var (
		err     error
		attempt int
	)

	start := time.Now()

	pollErr := wait.PollUntilContextTimeout(ctx, TransientRetryInterval, TransientRetryTimeout, true,
		func(pollCtx context.Context) (bool, error) {
			attempt++

			if pollCtx.Err() != nil {
				err = pollCtx.Err()
				return false, err
			}

			err = fn()
			if err == nil {
				return true, nil
			}

			if attempt > 1 && succeeded != nil && succeeded(err) {
				e2eframework.Logf("controller-runtime %s: attempt %d landed before a transient error was reported, treating as success: %v",
					op, attempt, err)
				err = nil
				return true, nil
			}

			if !isRetryableClientError(err) {
				return false, err
			}

			e2eframework.Logf("controller-runtime %s: attempt %d failed with a transient error, retrying in %s (elapsed %s): %v",
				op, attempt, TransientRetryInterval, time.Since(start).Round(time.Second), err)

			return false, nil
		})

	if err != nil {
		return err
	}

	return pollErr
}

// retryableClient wraps a controller-runtime client.Client so that Get,
// Create, Update, Patch, Delete, and DeleteAllOf retry on transient
// connectivity errors instead of failing an entire e2e spec on a single
// blip. All other methods (List, Status, SubResource, Scheme, RESTMapper,
// ...) pass through to the embedded client unmodified.
type retryableClient struct {
	client.Client
}

// NewRetryableClient wraps c with the retry behavior documented on
// retryableClient.
func NewRetryableClient(c client.Client) client.Client {
	return &retryableClient{Client: c}
}

func (r *retryableClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return retryOnTransientError(ctx, "get", nil, func() error {
		return r.Client.Get(ctx, key, obj, opts...)
	})
}

func (r *retryableClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return retryOnTransientError(ctx, "create", createAlreadySucceeded, func() error {
		return r.Client.Create(ctx, obj, opts...)
	})
}

func (r *retryableClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return retryOnTransientError(ctx, "update", nil, func() error {
		return r.Client.Update(ctx, obj, opts...)
	})
}

func (r *retryableClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return retryOnTransientError(ctx, "patch", nil, func() error {
		return r.Client.Patch(ctx, obj, patch, opts...)
	})
}

func (r *retryableClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return retryOnTransientError(ctx, "delete", deleteAlreadySucceeded, func() error {
		return r.Client.Delete(ctx, obj, opts...)
	})
}

func (r *retryableClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return retryOnTransientError(ctx, "deleteAllOf", nil, func() error {
		return r.Client.DeleteAllOf(ctx, obj, opts...)
	})
}
