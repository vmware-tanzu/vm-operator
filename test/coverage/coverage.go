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
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	runtimecoverage "runtime/coverage"
	"sync"
	"time"
)

// Revision identifies the source used to build the manager and its reports.
// The coverage build sets it with -ldflags.
var Revision = "unknown"

// DefaultPort is the loopback HTTP port used by the coverage manager.
const DefaultPort uint16 = 18081

// Start serves in-memory snapshots from an atomic coverage build on IPv4
// loopback. Port zero selects an available port for local tests. The endpoint
// is intentionally unauthenticated and requires no writable filesystem or
// GOCOVERDIR. Closing the returned server stops the listener.
func Start(ctx context.Context, port uint16) (*http.Server, error) {
	// Validate instrumentation immediately, before accepting any requests.
	var meta bytes.Buffer
	if err := runtimecoverage.WriteMeta(&meta); err != nil {
		return nil, fmt.Errorf("write coverage metadata: %w", err)
	}
	if err := runtimecoverage.WriteCounters(io.Discard); err != nil {
		return nil, fmt.Errorf("validate atomic coverage counters: %w", err)
	}
	// Go's version-1 metadata header contains its 16-byte file hash at
	// offset 24. Use that hash in both filenames, as WriteMetaDir does.
	// Fail explicitly if a future toolchain changes the format; integration
	// tests consume these archives with this build's covdata and cover tools.
	data := meta.Bytes()
	if len(data) < 56 || string(data[:4]) != "\x00cvm" ||
		binary.LittleEndian.Uint32(data[4:8]) != 1 {
		return nil, fmt.Errorf("unsupported Go coverage metadata format")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("create coverage process identity: %w", err)
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on coverage TCP port: %w", err)
	}
	h := &handler{metadata: data, metaHash: hex.EncodeToString(data[24:40]), processID: hex.EncodeToString(id)}
	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: h, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
		MaxHeaderBytes: 4096,
	}
	go func() { _ = server.Serve(listener) }()
	return server, nil
}

type handler struct {
	metadata  []byte
	metaHash  string
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
	entries, err := h.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Cache-Control", "no-store")
	if err := writeArchive(w, entries); err != nil {
		// Abort the HTTP stream so the helper cannot accept a partial archive.
		panic(http.ErrAbortHandler)
	}
}

type entry struct {
	name string
	data []byte
}

func (h *handler) snapshot() ([]entry, error) {
	var counters bytes.Buffer
	if err := runtimecoverage.WriteCounters(&counters); err != nil {
		return nil, fmt.Errorf("snapshot counters (requires -covermode=atomic): %w", err)
	}
	now := time.Now().UTC()
	manifest, err := json.Marshal(map[string]string{
		"revision": Revision, "process_id": h.processID,
		"go_version": runtime.Version(), "collected_at": now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("encode snapshot manifest: %w", err)
	}
	return []entry{
		{name: "covmeta." + h.metaHash, data: h.metadata},
		{name: fmt.Sprintf("covcounters.%s.%d.%d", h.metaHash, os.Getpid(), now.UnixNano()), data: counters.Bytes()},
		{name: "snapshot.json", data: manifest},
	}, nil
}

func writeArchive(w io.Writer, entries []entry) error {
	tw := tar.NewWriter(w)
	for _, entry := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0600, Size: int64(len(entry.data))}); err != nil {
			return fmt.Errorf("archive snapshot header: %w", err)
		}
		if _, err := tw.Write(entry.data); err != nil {
			return fmt.Errorf("archive snapshot content: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("finish snapshot archive: %w", err)
	}
	return nil
}
