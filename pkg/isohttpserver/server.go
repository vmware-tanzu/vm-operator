// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package isohttpserver implements a minimal, static-file HTTP server used
// to serve ISO bootstrap assets (kickstart files, autoinstall configs,
// Windows answer files, etc.) to a VM during automated OS installation.
//
// This package intentionally has no dependency on Kubernetes client
// libraries -- it only serves files from a local directory.
package isohttpserver

import (
	"errors"
	"log"
	"net/http"
	"time"
)

// Server is a minimal HTTP server that serves every file under Root as
// application/octet-stream, logging each request.
type Server struct {
	Addr string
	Root string
}

// NewServer returns a Server that listens on addr and serves files from
// root.
func NewServer(addr, root string) (*Server, error) {
	if addr == "" || root == "" {
		return nil, errors.New("server addr and root cannot be empty")
	}

	return &Server{Addr: addr, Root: root}, nil
}

// Run starts the server. It blocks until the server stops, returning the
// error from http.Server.ListenAndServe (http.ErrServerClosed on a normal
// shutdown).
func (s *Server) Run() error {
	server := &http.Server{
		Addr:              s.Addr,
		Handler:           s.loggingHandler(s.octetStreamHandler(http.FileServer(http.Dir(s.Root)))),
		ReadHeaderTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}

// loggingHandler logs the method and path of every request before
// delegating to next.
func (s *Server) loggingHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// octetStreamHandler forces the Content-Type header to
// application/octet-stream on every response, since served files are
// arbitrary bootstrap assets rather than content this server can infer a
// meaningful MIME type for.
func (s *Server) octetStreamHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		next.ServeHTTP(w, r)
	})
}
