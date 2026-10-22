// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package coverage exports cumulative snapshots from an instrumented E2E
// manager. Production binaries do not import this package.
package coverage

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	runtimecoverage "runtime/coverage"
	"sync"
	"time"
)

// Revision identifies the source used to build the manager and its reports.
// The coverage build sets it with -ldflags.
var Revision = "unknown"

// SocketName is the private socket used by the exporter inside the container.
const SocketName = "export.sock"

// Start serves coverage snapshots on a Unix socket in dir. The caller must
// provide an existing, writable directory and an atomic coverage build.
// Closing the returned server stops the listener.
func Start(ctx context.Context, dir string) (*http.Server, error) {
	if dir == "" {
		return nil, fmt.Errorf("GOCOVERDIR must name a writable coverage volume")
	}
	// Validate instrumentation immediately, before accepting any requests.
	if err := runtimecoverage.WriteMetaDir(dir); err != nil {
		return nil, fmt.Errorf("write coverage metadata: %w", err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("create coverage process identity: %w", err)
	}
	socket := filepath.Join(dir, SocketName)
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("coverage socket path is not a socket: %s", socket)
		}
		if err := os.Remove(socket); err != nil {
			return nil, fmt.Errorf("remove stale coverage socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect coverage socket: %w", err)
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("listen on coverage socket: %w", err)
	}
	if err := os.Chmod(socket, 0600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("restrict coverage socket: %w", err)
	}
	h := &handler{dir: dir, processID: hex.EncodeToString(id)}
	server := &http.Server{
		Handler: h, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	return server, nil
}

type handler struct {
	dir       string
	processID string
	mu        sync.Mutex
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/snapshot" {
		http.Error(w, "use GET /snapshot", http.StatusNotFound)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := h.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	_, _ = w.Write(data)
}

func (h *handler) snapshot() ([]byte, error) {
	dir, err := os.MkdirTemp(h.dir, "snapshot-")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := runtimecoverage.WriteMetaDir(dir); err != nil {
		return nil, fmt.Errorf("snapshot metadata: %w", err)
	}
	if err := runtimecoverage.WriteCountersDir(dir); err != nil {
		return nil, fmt.Errorf("snapshot counters (requires -covermode=atomic): %w", err)
	}
	manifest, err := json.Marshal(map[string]string{
		"revision": Revision, "process_id": h.processID,
		"go_version": runtime.Version(), "collected_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("encode snapshot manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), manifest, 0600); err != nil {
		return nil, fmt.Errorf("write snapshot manifest: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list snapshot files: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open snapshot directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		data, err := root.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read snapshot file: %w", err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: entry.Name(), Mode: 0600, Size: int64(len(data))}); err != nil {
			return nil, fmt.Errorf("archive snapshot header: %w", err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("archive snapshot content: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("finish snapshot archive: %w", err)
	}
	return buf.Bytes(), nil
}

// Export streams a snapshot of the manager process to w through its private
// Unix socket. The helper itself is built without coverage instrumentation.
func Export(ctx context.Context, dir string, w io.Writer) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", filepath.Join(dir, SocketName))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/snapshot", nil)
	if err != nil {
		return fmt.Errorf("create coverage request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request manager coverage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("manager coverage returned %s: %s", resp.Status, body)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("copy manager coverage: %w", err)
	}
	return nil
}
