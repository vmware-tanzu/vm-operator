// Copyright (c) 2026 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// newTestHostKey generates an ephemeral host key for the fake SSH server
// used by these tests; NewServerConn refuses to complete a handshake
// without one configured.
func newTestHostKey(t *testing.T) ssh.Signer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test host key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to create signer from test host key: %v", err)
	}

	return signer
}

// dialWithHardDeadline runs newSSHDialerWithRetries on its own goroutine and
// fails the test rather than hanging forever if it does not return within
// deadline — a hard backstop against a regression reintroducing a hang in
// the retry loop itself.
func dialWithHardDeadline(
	t *testing.T,
	hostname string, port int, config *ssh.ClientConfig, interval, timeout, deadline time.Duration,
) (*ssh.Client, error) {
	t.Helper()

	type result struct {
		client *ssh.Client
		err    error
	}
	done := make(chan result, 1)

	go func() {
		client, err := newSSHDialerWithRetries(hostname, port, config, interval, timeout)
		done <- result{client: client, err: err}
	}()

	select {
	case r := <-done:
		return r.client, r.err
	case <-time.After(deadline):
		t.Fatalf("newSSHDialerWithRetries did not return within %s; retry budget was %s", deadline, timeout)
		return nil, nil
	}
}

// TestNewSSHDialerWithRetries_RetriesTransientDialErrors guards against a
// regression where a dial error aborted the retry loop on the first
// attempt instead of retrying until the timeout elapsed (the loop's
// condition func must return (false, nil) on a transient error, not
// (false, err), or wait.PollUntilContextTimeout stops immediately).
func TestNewSSHDialerWithRetries_RetriesTransientDialErrors(t *testing.T) {
	// Keep one stable listener for the whole test (reopening the same
	// ephemeral port mid-test is racy) and simulate a transient failure by
	// dropping the first couple of connections before completing the SSH
	// handshake; only the third+ connection completes it successfully.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	defer func() { _ = l.Close() }()

	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(newTestHostKey(t))

	go func() {
		attempt := 0
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			attempt++

			if attempt < 3 {
				_ = conn.Close() // Simulate a dropped/failed connection.
				continue
			}

			_, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
			if err != nil {
				_ = conn.Close()
				return
			}
			go ssh.DiscardRequests(reqs)
			for range chans {
			}
		}
	}()

	host, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host/port %q: %v", l.Addr().String(), err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port %q: %v", portStr, err)
	}

	config := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("unused")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test-only dial, no real secrets.
		Timeout:         time.Second,
	}

	client, err := dialWithHardDeadline(t, host, port, config, 50*time.Millisecond, 5*time.Second, 10*time.Second)
	if err != nil {
		t.Fatalf("expected the dial to eventually succeed once the listener came up, got err: %v", err)
	}
	if client == nil {
		t.Fatal("expected a non-nil client on success")
	}
	_ = client.Close()
}

// TestNewSSHDialerWithRetries_ReturnsLastErrorAfterTimeout ensures a
// persistent failure still surfaces a real error once the retry budget is
// exhausted, rather than retrying forever or silently succeeding.
func TestNewSSHDialerWithRetries_ReturnsLastErrorAfterTimeout(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("failed to split host/port %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port %q: %v", portStr, err)
	}

	config := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("unused")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test-only dial, no real secrets.
		Timeout:         time.Second,
	}

	start := time.Now()
	_, err = dialWithHardDeadline(t, host, port, config, 50*time.Millisecond, 300*time.Millisecond, 10*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error since nothing ever listens on the port")
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("expected the dialer to retry for close to the full timeout budget, only elapsed %s", elapsed)
	}
}
