// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package chaos injects network-like faults (latency, jitter, stalls, resets
// and 429/503 responses) into the HTTP clients used by the e2e tests, so that
// flakiness caused by unstable infrastructure can be reproduced on demand even
// when the infrastructure is healthy.
//
// Kernel-level tools such as tc/netem are not usable from the unprivileged e2e
// runner container, so the faults are injected in-process by wrapping the
// http.RoundTripper of the Kubernetes REST config and the govmomi SOAP client.
//
// Chaos is off unless E2E_CHAOS_PROFILE is set to light, moderate or brutal.
// E2E_CHAOS_SEED makes a run reproducible; it is chosen and logged when unset.
//
// Not covered: anything that shells out (kubectl, govc, ssh, ...).
package chaos

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// ProfileEnv selects the fault profile: off (default), light, moderate or
	// brutal.
	ProfileEnv = "E2E_CHAOS_PROFILE"
	// SeedEnv seeds the fault RNG so a failing run can be replayed.
	SeedEnv = "E2E_CHAOS_SEED"
)

// Profile describes the faults applied to each request.
type Profile struct {
	Name string
	// Latency and Jitter delay every request by Latency plus up to Jitter.
	Latency time.Duration
	Jitter  time.Duration
	// StallRate is the probability of an extra Stall delay, which approximates
	// packet loss followed by a TCP retransmission timeout.
	StallRate float64
	Stall     time.Duration
	// ResetRate is the probability the request fails with a connection reset
	// before it reaches the server.
	ResetRate float64
	// ThrottleRate is the probability a request is answered with a synthetic
	// 429 or 503 before it reaches the server.
	ThrottleRate float64
}

var profiles = map[string]Profile{
	"light": {
		Name: "light", Latency: 50 * time.Millisecond, Jitter: 100 * time.Millisecond,
		StallRate: 0.005, Stall: 1 * time.Second, ResetRate: 0.002, ThrottleRate: 0.002,
	},
	"moderate": {
		Name: "moderate", Latency: 150 * time.Millisecond, Jitter: 300 * time.Millisecond,
		StallRate: 0.02, Stall: 3 * time.Second, ResetRate: 0.01, ThrottleRate: 0.01,
	},
	"brutal": {
		Name: "brutal", Latency: 400 * time.Millisecond, Jitter: 800 * time.Millisecond,
		StallRate: 0.05, Stall: 8 * time.Second, ResetRate: 0.03, ThrottleRate: 0.03,
	},
}

type injector struct {
	profile Profile
	mu      sync.Mutex
	rng     *rand.Rand
}

var (
	once sync.Once
	inj  *injector
)

// Enabled reports whether a chaos profile is active.
func Enabled() bool { return get() != nil }

func get() *injector {
	once.Do(func() {
		name := strings.ToLower(strings.TrimSpace(os.Getenv(ProfileEnv)))
		if name == "" || name == "off" {
			return
		}

		p, ok := profiles[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "chaos: unknown %s=%q, chaos disabled\n", ProfileEnv, name)
			return
		}

		seed := time.Now().UnixNano()
		if s := os.Getenv(SeedEnv); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				seed = v
			}
		}

		fmt.Fprintf(os.Stderr, "chaos: ENABLED profile=%s seed=%d (replay with %s=%s %s=%d) %+v\n",
			p.Name, seed, ProfileEnv, p.Name, SeedEnv, seed, p)

		inj = newInjector(p, seed)
	})

	return inj
}

// WrapTransport wraps rt with fault injection when chaos is enabled and
// otherwise returns rt unchanged. It matches rest.Config.WrapTransport.
func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	i := get()
	if i == nil {
		return rt
	}

	if rt == nil {
		rt = http.DefaultTransport
	}

	return &transport{next: rt, inj: i}
}

type transport struct {
	next http.RoundTripper
	inj  *injector
}

// exempt reports whether the request must not be faulted: long-lived log and
// event streams (faulting them only loses the artifacts used for triage) and
// protocol upgrades such as exec and port-forward.
func exempt(req *http.Request) bool {
	q := req.URL.Query()
	return q.Get("watch") == "true" || q.Get("follow") == "true" ||
		strings.HasSuffix(req.URL.Path, "/log") ||
		req.Header.Get("Upgrade") != ""
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if exempt(req) {
		return t.next.RoundTrip(req)
	}

	delay, reset, throttle := t.inj.roll()

	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}

	if reset {
		return nil, newInjectedReset()
	}

	if throttle != 0 {
		return &http.Response{
			Status:     fmt.Sprintf("%d %s", throttle, http.StatusText(throttle)),
			StatusCode: throttle,
			Proto:      "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header:  http.Header{"Retry-After": []string{"1"}, "Content-Type": []string{"text/plain"}},
			Body:    http.NoBody,
			Request: req,
		}, nil
	}

	return t.next.RoundTrip(req)
}

func newInjector(p Profile, seed int64) *injector {
	return &injector{profile: p, rng: rand.New(rand.NewSource(seed))} //nolint:gosec
}

func (i *injector) roll() (delay time.Duration, reset bool, throttle int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	p := i.profile

	delay = p.Latency
	if p.Jitter > 0 {
		delay += time.Duration(i.rng.Int63n(int64(p.Jitter)))
	}

	if i.rng.Float64() < p.StallRate {
		delay += p.Stall
	}

	reset = i.rng.Float64() < p.ResetRate

	if i.rng.Float64() < p.ThrottleRate {
		throttle = http.StatusTooManyRequests
		if i.rng.Intn(2) == 0 {
			throttle = http.StatusServiceUnavailable
		}
	}

	return delay, reset, throttle
}

// injectedReset is what a real TCP reset looks like to callers (a *net.OpError
// wrapping ECONNRESET, so Temporary() is true and generic retry logic treats
// it like the real thing), with a "chaos:" prefix so it is recognisable in
// logs.
type injectedReset struct {
	*net.OpError
}

func (e injectedReset) Error() string {
	return "chaos: injected " + e.OpError.Error()
}

func newInjectedReset() error {
	return injectedReset{&net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: os.NewSyscallError("read", syscall.ECONNRESET),
	}}
}
