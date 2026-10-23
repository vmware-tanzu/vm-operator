// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
)

// socketAddress scopes the abstract socket to this UID and PID namespace.
// Supervisor pods may share the host network but have separate PID namespaces.
func socketAddress() (string, error) {
	namespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return "", fmt.Errorf("read coverage PID namespace: %w", err)
	}
	return fmt.Sprintf("@vmoperator-coverage.%d.%s", os.Geteuid(), namespace), nil
}

func listen(ctx context.Context) (net.Listener, error) {
	address, err := socketAddress()
	if err != nil {
		return nil, err
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "unix", address)
	if err != nil {
		return nil, fmt.Errorf("bind abstract coverage socket: %w", err)
	}
	return &peerListener{Listener: listener}, nil
}

type peerListener struct {
	net.Listener
}

// Accept rejects unauthorized peers before the HTTP server reads their request.
func (l *peerListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, fmt.Errorf("accept coverage peer: %w", err)
		}
		if err := checkPeer(conn); err != nil {
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}

func dial(ctx context.Context) (net.Conn, error) {
	address, err := socketAddress()
	if err != nil {
		return nil, err
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", address)
	if err != nil {
		return nil, fmt.Errorf("connect to abstract coverage socket: %w", err)
	}
	if err := checkPeer(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("authenticate coverage manager: %w", err)
	}
	return conn, nil
}

// Abstract sockets ignore filesystem permissions. Check credentials in both
// directions, and reject peers outside our PID namespace even with the same UID.
func checkPeer(conn net.Conn) error {
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("coverage peer is not a Unix connection")
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return fmt.Errorf("access coverage peer socket: %w", err)
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return fmt.Errorf("read coverage peer credentials: %w", err)
	}
	if credErr != nil {
		return fmt.Errorf("read coverage peer credentials: %w", credErr)
	}
	if int64(cred.Uid) != int64(os.Geteuid()) || cred.Pid <= 0 {
		return fmt.Errorf("coverage peer must have the same effective UID and PID namespace")
	}
	own, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return fmt.Errorf("read own PID namespace: %w", err)
	}
	peer, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/pid", cred.Pid))
	if err != nil {
		return fmt.Errorf("read coverage peer PID namespace: %w", err)
	}
	if own != peer {
		return fmt.Errorf("coverage peer has a different PID namespace")
	}
	return nil
}
