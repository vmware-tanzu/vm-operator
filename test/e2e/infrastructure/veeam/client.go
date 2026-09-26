// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package veeam is a minimal, test-only client for the Veeam Backup &
// Replication (VBR) REST API. It covers only what the backup/restore E2E suite
// needs: create a job for one VM, run it, find the restore point, restore the
// VM, and clean up. See .sdd/specs/006-veeam-e2e-backup-restore/research.md
// for the validated request/response shapes.
package veeam

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultPort is the default VBR REST API port.
	DefaultPort = "9419"

	// tokenMaxAge is how long an access token is reused before the client
	// logs in again. VBR issues tokens that expire after 900 seconds.
	tokenMaxAge = 10 * time.Minute
)

// SupportedAPIVersions lists the x-api-version values this client knows how
// to speak, newest first. The first one the server accepts is used.
var SupportedAPIVersions = []string{
	"1.3-rev2",
	"1.3-rev1",
	"1.3-rev0",
	"1.2-rev1",
	"1.2-rev0",
	"1.1-rev2",
	"1.1-rev1",
	"1.1-rev0",
}

// ErrorKind classifies connection-time failures so a caller can decide
// whether to skip or fail a test.
type ErrorKind string

const (
	// ErrorKindNotConfigured means no server address was provided.
	ErrorKindNotConfigured ErrorKind = "NotConfigured"
	// ErrorKindUnreachable means the server could not be contacted.
	ErrorKindUnreachable ErrorKind = "Unreachable"
	// ErrorKindUnsupportedVersion means the server supports none of
	// SupportedAPIVersions.
	ErrorKindUnsupportedVersion ErrorKind = "UnsupportedVersion"
	// ErrorKindAuth means the server rejected the credentials.
	ErrorKindAuth ErrorKind = "AuthFailure"
)

// ConnectError is returned by New when a client cannot be established.
type ConnectError struct {
	Kind   ErrorKind
	Server string
	Err    error
}

func (e *ConnectError) Error() string {
	return fmt.Sprintf("veeam %s (server %q): %v", e.Kind, e.Server, e.Err)
}

func (e *ConnectError) Unwrap() error {
	return e.Err
}

// Config holds the connection parameters for a VBR server.
type Config struct {
	// Server is "host", "host:port", or a full "https://host:port" URL.
	Server   string
	Username string
	Password string
}

// Client is a VBR REST API client pinned to a single API version.
type Client struct {
	baseURL    string
	username   string
	password   string
	apiVersion string
	httpClient *http.Client

	mu      sync.Mutex
	token   string
	tokenAt time.Time
}

// New detects the API version the server supports, logs in, and returns a
// ready client. Connection-time failures are returned as *ConnectError.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Server == "" {
		return nil, &ConnectError{Kind: ErrorKindNotConfigured, Err: errors.New("no Veeam server address configured")}
	}

	c := &Client{
		baseURL:  baseURL(cfg.Server),
		username: cfg.Username,
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
			Transport: &http.Transport{
				// Never route through HTTP(S)_PROXY. The E2E environment
				// points those at the testbed gateway, which cannot reach
				// the Veeam appliance.
				Proxy: nil,
				// The appliance uses a self-signed certificate.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Test-only client.
			},
		},
	}

	v, err := c.detectAPIVersion(ctx)
	if err != nil {
		return nil, err
	}

	c.apiVersion = v

	if err := c.login(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// APIVersion returns the x-api-version the client negotiated.
func (c *Client) APIVersion() string {
	return c.apiVersion
}

func baseURL(server string) string {
	if strings.Contains(server, "://") {
		return strings.TrimSuffix(server, "/")
	}

	if !strings.Contains(server, ":") {
		server += ":" + DefaultPort
	}

	return "https://" + server
}

// detectAPIVersion probes the unauthenticated per-version Swagger document,
// newest first. VBR returns 200 for a version it serves and 404 otherwise.
func (c *Client) detectAPIVersion(ctx context.Context) (string, error) {
	var lastStatus int

	for _, v := range SupportedAPIVersions {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/swagger/v"+v+"/swagger.json", nil)
		if err != nil {
			return "", err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", &ConnectError{Kind: ErrorKindUnreachable, Server: c.baseURL, Err: err}
		}

		// The documents are several MB; only the status code matters.
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return v, nil
		}

		lastStatus = resp.StatusCode
	}

	return "", &ConnectError{
		Kind:   ErrorKindUnsupportedVersion,
		Server: c.baseURL,
		Err:    fmt.Errorf("none of %v is served (last HTTP status %d)", SupportedAPIVersions, lastStatus),
	}
}

func (c *Client) login(ctx context.Context) error {
	form := url.Values{
		"grant_type": {"password"},
		"username":   {c.username},
		"password":   {c.password},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Api-Version", c.apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &ConnectError{Kind: ErrorKindUnreachable, Server: c.baseURL, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return &ConnectError{Kind: ErrorKindAuth, Server: c.baseURL, Err: fmt.Errorf("login as %q rejected: %s", c.username, body)}
	case resp.StatusCode != http.StatusOK:
		return &ConnectError{Kind: ErrorKindUnreachable, Server: c.baseURL, Err: fmt.Errorf("login returned HTTP %d: %s", resp.StatusCode, body)}
	}

	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return &ConnectError{Kind: ErrorKindAuth, Server: c.baseURL, Err: fmt.Errorf("login response has no access token: %s", body)}
	}

	c.token = tok.AccessToken
	c.tokenAt = time.Now()

	return nil
}

// bearer returns a valid access token, logging in again once the current
// token is older than tokenMaxAge.
func (c *Client) bearer(ctx context.Context, forceLogin bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if forceLogin || c.token == "" || time.Since(c.tokenAt) > tokenMaxAge {
		if err := c.login(ctx); err != nil {
			return "", err
		}
	}

	return c.token, nil
}

// APIError is returned when the server answers with an unexpected status.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("veeam %s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// IsNotFound reports whether err is an APIError with HTTP status 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// do sends an authenticated request and decodes a JSON response into out
// when out is non-nil. Any 2xx status is treated as success. A 401 triggers
// one re-login and retry.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte

	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return fmt.Errorf("failed to marshal request for %s %s: %w", method, path, err)
		}
	}

	for attempt := 0; ; attempt++ {
		token, err := c.bearer(ctx, attempt > 0)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}

		req.Header.Set("X-Api-Version", c.apiVersion)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("veeam %s %s: %w", method, path, err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			return fmt.Errorf("veeam %s %s: failed to read response: %w", method, path, err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &APIError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: string(body)}
		}

		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("veeam %s %s: failed to decode response: %w", method, path, err)
			}
		}

		return nil
	}
}

// legacyAPI reports whether the negotiated version predates 1.2-rev0, which
// renamed the vSphere job type and restore path.
func (c *Client) legacyAPI() bool {
	return strings.HasPrefix(c.apiVersion, "1.0-") || strings.HasPrefix(c.apiVersion, "1.1-")
}
