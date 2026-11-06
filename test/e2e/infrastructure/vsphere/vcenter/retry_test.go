// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"testing"
)

type fakeStatus string

func (e fakeStatus) Error() string { return string(e) }

func TestIsTransientError(t *testing.T) {
	reset := &url.Error{Op: "Post", URL: "https://vc/sdk", Err: &net.OpError{
		Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET),
	}}

	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"nil":            {nil, false},
		"conn reset":     {reset, true},
		"429":            {&url.Error{Op: "POST", URL: "/sdk", Err: fakeStatus("429 Too Many Requests")}, true},
		"503":            {&url.Error{Op: "POST", URL: "/sdk", Err: fakeStatus("503 Service Unavailable")}, true},
		"401":            {&url.Error{Op: "POST", URL: "/sdk", Err: fakeStatus("401 Unauthorized")}, false},
		"real failure":   {errors.New("ServerFaultCode: NoPermission"), false},
		"status in text": {errors.New(http.StatusText(http.StatusTooManyRequests)), false},
	} {
		if got := IsTransientError(tc.err); got != tc.want {
			t.Errorf("%s: IsTransientError = %v, want %v", name, got, tc.want)
		}
	}
}

func TestRetryTransient(t *testing.T) {
	calls := 0

	err := RetryTransient(context.Background(), func() error {
		if calls++; calls < 3 {
			return syscall.ECONNRESET
		}

		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("want success after 3 calls, got err=%v calls=%d", err, calls)
	}

	calls = 0
	permanent := errors.New("NoPermission")

	if err := RetryTransient(context.Background(), func() error { calls++; return permanent }); !errors.Is(err, permanent) || calls != 1 {
		t.Errorf("permanent error must not be retried, got err=%v calls=%d", err, calls)
	}
}
