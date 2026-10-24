// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"io"
	"net/http"
)

const (
	// restSessionHeader is the canonical form of vapi/internal.SessionCookieName,
	// which is in an internal package and cannot be imported from this repository.
	restSessionHeader = "Vmware-Api-Session-Id"

	// restSessionPath mirrors vapi/rest.Path + vapi/internal.SessionPath.
	// Login (POST), logout (DELETE) and the session probe
	// (POST ?~action=get) all share this path, so a single path check
	// excludes all three from the retry logic.
	restSessionPath = "/rest/com/vmware/cis/session"
)

// reloginREST is an http.RoundTripper wrapper that re-authenticates the REST
// session and replays the request in place when the first attempt is answered
// with a 401. It must be installed innermost, below the keepalive handler.
type reloginREST struct {
	rt     http.RoundTripper
	keeper *sessionKeeper
}

// newReloginREST returns the inline re-login round tripper for the given
// underlying round tripper.
func newReloginREST(
	rt http.RoundTripper,
	keeper *sessionKeeper) *reloginREST {

	return &reloginREST{
		rt:     rt,
		keeper: keeper,
	}
}

// RoundTrip implements http.RoundTripper. On a 401 it re-authenticates the
// session and, when the request has a safely repeatable body, replays it
// exactly once with a rewritten vmware-api-session-id header. A request whose
// body cannot be repeated still triggers the re-login, so the session is
// healed for the next caller; only its own 401 is surfaced unretried.
// Requests to the session path -- login, logout and the session probe -- are
// never retried, which is the re-entrancy guard.
func (r *reloginREST) RoundTrip(req *http.Request) (*http.Response, error) {
	// Exclude the session path first, before anything else. This preserves
	// rest.Client.Session's documented contract of returning (nil, nil) on
	// a 401, which the wrapper below must never turn into a retry.
	if isSessionPath(req.URL.Path) {
		return r.rt.RoundTrip(req)
	}

	// Decide repeatability before the first attempt. replayBody returns nil
	// when the body cannot be re-read, in which case a 401 is surfaced to
	// the caller instead of being retried. The re-login below still runs.
	replay := replayBody(req)

	// Read the generation before the first attempt, not after the 401: a
	// login that lands while this call was in flight must still be
	// observed.
	gen := r.keeper.restGeneration()
	// Do stamps the header before entering this transport, so it can name
	// an older session even if gen already reflects a completed re-login.
	sessionID := req.Header.Get(restSessionHeader)

	res, err := r.rt.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		return res, err
	}

	// Drain and close the 401 response so the connection is reusable. The
	// body is restored from the drained bytes because res is handed back
	// to the caller on every path below that does not replay.
	drainResponse(res)

	// Re-login before considering the replay, so that a request whose body
	// cannot be repeated -- a streaming upload, say -- still heals the
	// session instead of leaving it dead until another call happens to
	// fault with a replayable body or the keepalive ticks.
	trigger := req.Method + " " + req.URL.Path
	if lerr := r.keeper.reloginREST(req.Context(), gen, sessionID, trigger); lerr != nil {
		// Re-login failed. Surface the original 401; there is nothing
		// better to return.
		return res, nil //nolint:nilerr // HTTP failures are returned in the response, per http.RoundTripper.
	}

	if replay == nil {
		// The body cannot be safely repeated, so this request is not
		// retried. The session is authenticated again for the next caller.
		return res, nil
	}

	// http.RoundTripper's contract forbids mutating the request it was
	// given, so clone before modifying. The header rewrite is not
	// optional: rest.Client.Do stamps the old session id before the
	// transport sees the request.
	req2 := req.Clone(req.Context())
	req2.Body = replay()
	if req.Body != nil && req2.Body == nil {
		// GetBody failed; do not replay with a missing body.
		return res, nil
	}
	if req2.Body == http.NoBody {
		// Rules 2 and 3 replay a bodiless request as http.NoBody; the stale
		// ContentLength from the original request must go with it.
		req2.ContentLength = 0
	}
	req2.Header.Set(restSessionHeader, r.keeper.restClient().SessionID())
	return r.rt.RoundTrip(req2)
}

// isSessionPath reports whether the request targets the vAPI session path.
func isSessionPath(path string) bool {
	return path == restSessionPath
}

// replayBody returns a func that produces a fresh body for a replayed request,
// or nil when the body cannot be safely repeated. The ordered rule:
//
//  1. GetBody != nil means the body is repeatable via GetBody; http.NewRequest
//     populates it for the *bytes.Buffer that rest.Resource.Request's encode
//     builds for JSON POST/PATCH/PUT bodies.
//
//  2. A provably empty body -- nil, http.NoBody, or ContentLength == 0 --
//     replays as http.NoBody.
//
//  3. GET, HEAD, OPTIONS and DELETE replay with http.NoBody: govmomi's
//     vapi/rest only ever puts the empty io.MultiReader() body on these verbs
//     -- req.Body is a non-nil NopCloser with GetBody == nil -- and servers
//     ignore request bodies on them, so dropping it on replay is semantically
//     lossless. Without this rule a plain REST GET whose body arrived with a
//     non-zero ContentLength would 401 unretried.
//
//  4. Anything else -- a streaming POST/PATCH/PUT with ContentLength > 0,
//     i.e. rest.Client.Upload of a content-library item via soap.Client.Upload
//     -- is not replayable. The body must not be buffered into memory, and the
//     request is not retried.
//
// Note on action-style POSTs (?~action=): rest.Resource.Request gives them the
// same empty io.MultiReader body with ContentLength == 0, so rule 2 replays
// them. Only a streaming action POST with ContentLength > 0 falls to rule 4,
// which is the desired upload behavior.
func replayBody(req *http.Request) func() io.ReadCloser {
	switch {
	case req.GetBody != nil:
		return func() io.ReadCloser {
			b, err := req.GetBody()
			if err != nil {
				return nil
			}
			return b
		}
	case req.Body == nil ||
		req.Body == http.NoBody ||
		req.ContentLength == 0:
		return func() io.ReadCloser { return http.NoBody }
	case req.Method == http.MethodGet ||
		req.Method == http.MethodHead ||
		req.Method == http.MethodOptions ||
		req.Method == http.MethodDelete:
		return func() io.ReadCloser { return http.NoBody }
	}
	return nil
}

// drainResponse reads and closes the response body so the underlying
// connection can be reused, then replaces it with an equivalent reader over
// the bytes just read. Restoring the body matters because the caller may be
// handed this response when the request is not replayed, and a consumed,
// closed body would give it no error detail to report.
func drainResponse(res *http.Response) {
	if res.Body == nil {
		return
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, res.Body)
	_ = res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
}
