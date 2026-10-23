//go:build !linux

// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Local non-Linux development uses filesystem permissions because abstract
// Unix sockets are Linux-specific. No coverage data is written here.
func socketPath() (string, error) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("vmoperator-coverage-%d", os.Geteuid()))
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("create private coverage socket directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect coverage socket directory: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0077 != 0 || int64(stat.Uid) != int64(os.Geteuid()) {
		return "", fmt.Errorf("coverage socket directory must be private and owned by this UID")
	}
	return filepath.Join(dir, "export.sock"), nil
}

func listen(ctx context.Context) (net.Listener, error) {
	path, err := socketPath()
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("coverage socket path is not a socket: %s", path)
		}
		dialer := net.Dialer{Timeout: time.Second}
		conn, err := dialer.DialContext(ctx, "unix", path)
		if err == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("coverage socket is already in use")
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("check previous coverage listener: %w", err)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale coverage socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect coverage socket: %w", err)
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("bind coverage socket: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("restrict coverage socket: %w", err)
	}
	return listener, nil
}

func dial(ctx context.Context) (net.Conn, error) {
	path, err := socketPath()
	if err != nil {
		return nil, err
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("connect to coverage socket: %w", err)
	}
	return conn, nil
}
