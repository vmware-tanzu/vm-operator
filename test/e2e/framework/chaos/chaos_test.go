// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package chaos

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

func newReq(t *testing.T, url string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}

	return req
}

func TestExempt(t *testing.T) {
	for url, want := range map[string]bool{
		"https://h/api/v1/pods":                    false,
		"https://h/api/v1/pods?watch=true":         true,
		"https://h/api/v1/namespaces/n/pods/p/log": true,
		"https://h/api/v1/pods/p/log?follow=true":  true,
	} {
		if got := exempt(newReq(t, url)); got != want {
			t.Errorf("exempt(%q)=%v, want %v", url, got, want)
		}
	}
}

func TestTransportInjectsFaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	tr := &transport{next: http.DefaultTransport, inj: newInjector(Profile{ResetRate: 1}, 1)}
	_, err := tr.RoundTrip(newReq(t, srv.URL))
	if err == nil {
		t.Fatal("expected injected reset error")
	}

	if !errors.Is(err, syscall.ECONNRESET) {
		t.Errorf("injected reset should wrap ECONNRESET like a real one, got %T: %v", err, err)
	}

	tr = &transport{next: http.DefaultTransport, inj: newInjector(Profile{ThrottleRate: 1}, 1)}

	resp, err := tr.RoundTrip(newReq(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unexpected status %d", resp.StatusCode)
	}

	tr = &transport{next: http.DefaultTransport, inj: newInjector(Profile{Latency: 50 * time.Millisecond}, 1)}

	start := time.Now()
	if _, err := tr.RoundTrip(newReq(t, srv.URL)); err != nil {
		t.Fatal(err)
	}

	if time.Since(start) < 50*time.Millisecond {
		t.Error("expected latency to be injected")
	}
}

func TestSeedReproducible(t *testing.T) {
	a, b := newInjector(profiles["brutal"], 42), newInjector(profiles["brutal"], 42)
	for range 100 {
		d1, r1, t1 := a.roll()
		d2, r2, t2 := b.roll()

		if d1 != d2 || r1 != r2 || t1 != t2 {
			t.Fatal("same seed produced different faults")
		}
	}
}
