// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package veeam_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/veeam"
)

// fakeVBR is a minimal in-memory VBR REST server.
type fakeVBR struct {
	t *testing.T

	mu           sync.Mutex
	versions     map[string]bool
	password     string
	logins       int
	expireTokens bool
	requests     []string
	bodies       map[string]map[string]any

	// sessionStates is the sequence of states GET /sessions/{id} returns;
	// the last entry repeats.
	sessionStates []veeam.Session
	sessionPolls  int
}

func newFakeVBR(t *testing.T, versions ...string) (*fakeVBR, *httptest.Server) {
	t.Helper()

	f := &fakeVBR{
		t:        t,
		versions: map[string]bool{},
		password: "pw",
		bodies:   map[string]map[string]any{},
		sessionStates: []veeam.Session{
			{ID: "s1", State: veeam.SessionStateStopped, Result: veeam.SessionResult{Result: veeam.SessionResultSuccess}},
		},
	}
	for _, v := range versions {
		f.versions[v] = true
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)

	return f, srv
}

func (f *fakeVBR) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.Path
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())

	if strings.HasPrefix(path, "/swagger/v") {
		v := strings.TrimSuffix(strings.TrimPrefix(path, "/swagger/v"), "/swagger.json")
		if f.versions[v] {
			_, _ = io.WriteString(w, "{}")
			return
		}

		w.WriteHeader(http.StatusNotFound)

		return
	}

	if path == "/api/oauth2/token" {
		_ = r.ParseForm()
		if r.PostForm.Get("password") != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errorCode":"AccessDenied"}`)

			return
		}

		f.logins++
		writeJSON(w, http.StatusOK, map[string]any{"access_token": "tok" + string(rune('0'+f.logins)), "expires_in": 900})

		return
	}

	if r.Header.Get("x-api-version") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if f.expireTokens {
		// Reject the first token once to exercise the re-login path.
		f.expireTokens = false

		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	if r.Body != nil {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			f.bodies[r.Method+" "+path] = body
		}
	}

	switch {
	case path == "/api/v1/inventory/vmware/hosts/vc.example.com":
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
			{"type": "VirtualMachine", "name": "vm1", "objectId": "vm-1", "hostName": "vc.example.com"},
			{"type": "VirtualMachine", "name": "vm1", "objectId": "vm-2", "hostName": "vc.example.com", "platform": "VSphere"},
		}})
	case path == "/api/v1/backupInfrastructure/repositories":
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
			{"id": "repo-1", "name": "Default Backup Repository"},
			{"id": "repo-2", "name": "Other"},
		}})
	case path == "/api/v1/jobs" && r.Method == http.MethodPost:
		writeJSON(w, http.StatusCreated, map[string]any{"id": "job-1", "name": "n"})
	case path == "/api/v1/jobs/job-1/start":
		writeJSON(w, http.StatusCreated, map[string]any{"id": "s1", "state": "Starting"})
	case strings.HasPrefix(path, "/api/v1/sessions/") && strings.HasSuffix(path, "/logs"):
		writeJSON(w, http.StatusOK, map[string]any{"records": []map[string]any{
			{"status": "Failed", "title": "Processing vm1 Error: boom"},
		}})
	case strings.HasPrefix(path, "/api/v1/sessions/"):
		i := min(f.sessionPolls, len(f.sessionStates)-1)
		f.sessionPolls++
		writeJSON(w, http.StatusOK, f.sessionStates[i])
	case path == "/api/v1/backups" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
			{"id": "b1", "jobId": r.URL.Query().Get("jobIdFilter")},
			{"id": "b-other", "jobId": "someone-elses-job"},
		}})
	case path == "/api/v1/restorePoints":
		if r.URL.Query().Get("backupIdFilter") != "b1" {
			writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
			{"id": "rp-old", "creationTime": "2026-09-25T10:00:00Z"},
			{"id": "rp-new", "creationTime": "2026-09-25T11:00:00Z"},
		}})
	case path == "/api/v1/restore/vmRestore/vSphere" || path == "/api/v1/restore/vmRestore/vmware/":
		writeJSON(w, http.StatusCreated, map[string]any{"id": "s1", "state": "Starting"})
	case path == "/api/v1/backups/b1" && r.Method == http.MethodDelete:
		writeJSON(w, http.StatusCreated, map[string]any{"id": "s1", "state": "Starting"})
	case path == "/api/v1/jobs/job-1" && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeVBR) saw(req string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, r := range f.requests {
		if r == req {
			return true
		}
	}

	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) //nolint:errchkjson // Test fake; encode errors surface as client decode failures.
}

func connect(t *testing.T, srv *httptest.Server, password string) (*veeam.Client, error) {
	t.Helper()

	return veeam.New(t.Context(), veeam.Config{Server: srv.URL, Username: "u", Password: password})
}

func mustConnect(t *testing.T, srv *httptest.Server) *veeam.Client {
	t.Helper()

	c, err := connect(t, srv, "pw")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	return c
}

func connectErrorKind(t *testing.T, err error) veeam.ErrorKind {
	t.Helper()

	var ce *veeam.ConnectError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConnectError, got %T: %v", err, err)
	}

	return ce.Kind
}

var fastWait = veeam.WaitOptions{Timeout: 2 * time.Second, StartTimeout: time.Second, Interval: time.Millisecond}

func TestNewPicksNewestSupportedVersion(t *testing.T) {
	_, srv := newFakeVBR(t, "1.1-rev0", "1.3-rev1")

	if v := mustConnect(t, srv).APIVersion(); v != "1.3-rev1" {
		t.Errorf("expected 1.3-rev1, got %s", v)
	}
}

func TestNewErrors(t *testing.T) {
	if _, err := veeam.New(t.Context(), veeam.Config{}); connectErrorKind(t, err) != veeam.ErrorKindNotConfigured {
		t.Errorf("expected NotConfigured, got %v", err)
	}

	_, srv := newFakeVBR(t, "9.9-rev0")
	if _, err := connect(t, srv, "pw"); connectErrorKind(t, err) != veeam.ErrorKindUnsupportedVersion {
		t.Errorf("expected UnsupportedVersion, got %v", err)
	}

	_, srv = newFakeVBR(t, "1.3-rev2")
	if _, err := connect(t, srv, "wrong"); connectErrorKind(t, err) != veeam.ErrorKindAuth {
		t.Errorf("expected AuthFailure, got %v", err)
	}

	srv.Close()

	if _, err := connect(t, srv, "pw"); connectErrorKind(t, err) != veeam.ErrorKindUnreachable {
		t.Errorf("expected Unreachable, got %v", err)
	}
}

func TestReloginOn401(t *testing.T) {
	f, srv := newFakeVBR(t, "1.3-rev2")
	c := mustConnect(t, srv)

	f.expireTokens = true

	if _, err := c.RepositoryID(t.Context(), ""); err != nil {
		t.Fatalf("RepositoryID: %v", err)
	}

	if f.logins != 2 {
		t.Errorf("expected 2 logins, got %d", f.logins)
	}
}

func TestFindVMMatchesMoref(t *testing.T) {
	_, srv := newFakeVBR(t, "1.3-rev2")
	c := mustConnect(t, srv)

	vm, err := c.FindVM(t.Context(), "vc.example.com", "vm1", "vm-2")
	if err != nil {
		t.Fatalf("FindVM: %v", err)
	}

	if vm.ObjectID != "vm-2" {
		t.Errorf("expected vm-2, got %s", vm.ObjectID)
	}

	if _, err := c.FindVM(t.Context(), "vc.example.com", "vm1", "vm-3"); err == nil {
		t.Error("expected an error for an unknown moref")
	}
}

func TestRepositoryID(t *testing.T) {
	_, srv := newFakeVBR(t, "1.3-rev2")
	c := mustConnect(t, srv)

	for name, want := range map[string]string{"": "repo-1", "Other": "repo-2"} {
		got, err := c.RepositoryID(t.Context(), name)
		if err != nil || got != want {
			t.Errorf("RepositoryID(%q) = %q, %v; want %q", name, got, err, want)
		}
	}

	if _, err := c.RepositoryID(t.Context(), "missing"); err == nil {
		t.Error("expected an error for a missing repository")
	}
}

func TestCreateJobUsesVersionSpecificType(t *testing.T) {
	for version, wantType := range map[string]string{"1.3-rev2": "VSphereBackup", "1.1-rev0": "Backup"} {
		f, srv := newFakeVBR(t, version)
		c := mustConnect(t, srv)

		job, err := c.CreateJob(t.Context(), "job", "repo-1", veeam.InventoryObject{Type: "VirtualMachine", Name: "vm1", ObjectID: "vm-1"})
		if err != nil {
			t.Fatalf("CreateJob: %v", err)
		}

		if job.ID != "job-1" {
			t.Errorf("expected job-1, got %s", job.ID)
		}

		body := f.bodies["POST /api/v1/jobs"]
		if body["type"] != wantType {
			t.Errorf("%s: expected job type %s, got %v", version, wantType, body["type"])
		}

		if sched, _ := body["schedule"].(map[string]any); sched["runAutomatically"] != false {
			t.Errorf("job must not run automatically: %v", body["schedule"])
		}
	}
}

func TestBackupAndRestore(t *testing.T) {
	for version, restorePath := range map[string]string{
		"1.3-rev2": "/api/v1/restore/vmRestore/vSphere",
		"1.1-rev0": "/api/v1/restore/vmRestore/vmware/",
	} {
		f, srv := newFakeVBR(t, version)
		c := mustConnect(t, srv)
		ctx := t.Context()

		if _, err := c.Backup(ctx, "job-1", fastWait); err != nil {
			t.Fatalf("Backup: %v", err)
		}

		rp, err := c.LatestRestorePoint(ctx, "job-1")
		if err != nil {
			t.Fatalf("LatestRestorePoint: %v", err)
		}

		if rp.ID != "rp-new" {
			t.Errorf("expected rp-new, got %s", rp.ID)
		}

		if _, err := c.RestoreVM(ctx, rp.ID, true, "test", fastWait); err != nil {
			t.Fatalf("RestoreVM: %v", err)
		}

		body := f.bodies["POST "+restorePath]
		if body["overwrite"] != true || body["powerUp"] != false || body["restorePointId"] != "rp-new" {
			t.Errorf("%s: unexpected restore body %v", version, body)
		}
	}
}

func TestWaitForSession(t *testing.T) {
	stopped := func(result string) veeam.Session {
		return veeam.Session{ID: "s1", State: veeam.SessionStateStopped, Result: veeam.SessionResult{Result: result}}
	}

	testCases := []struct {
		name       string
		states     []veeam.Session
		wantErr    string
		wantResult string
	}{
		{
			name:       "success after working",
			states:     []veeam.Session{{ID: "s1", State: "Working"}, stopped(veeam.SessionResultSuccess)},
			wantResult: veeam.SessionResultSuccess,
		},
		{
			name:       "warning is success",
			states:     []veeam.Session{stopped(veeam.SessionResultWarning)},
			wantResult: veeam.SessionResultWarning,
		},
		{
			name:    "failed includes log",
			states:  []veeam.Session{stopped(veeam.SessionResultFailed)},
			wantErr: "Processing vm1 Error: boom",
		},
		{
			name:    "never starts",
			states:  []veeam.Session{{ID: "s1", State: veeam.SessionStateStarting}},
			wantErr: "did not start",
		},
		{
			name:    "never finishes",
			states:  []veeam.Session{{ID: "s1", State: "Working"}},
			wantErr: "did not finish",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, srv := newFakeVBR(t, "1.3-rev2")
			f.sessionStates = tc.states
			c := mustConnect(t, srv)

			opts := veeam.WaitOptions{Timeout: 50 * time.Millisecond, StartTimeout: 20 * time.Millisecond, Interval: time.Millisecond}
			if tc.name == "never finishes" {
				opts.StartTimeout = 0
			}

			s, err := c.WaitForSession(t.Context(), "s1", opts)

			if tc.wantErr != "" {
				var se *veeam.SessionError
				if !errors.As(err, &se) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected SessionError containing %q, got %v", tc.wantErr, err)
				}

				return
			}

			if err != nil || s.Result.Result != tc.wantResult {
				t.Fatalf("got %v, %v; want result %s", s.Result, err, tc.wantResult)
			}
		})
	}
}

func TestDeleteJob(t *testing.T) {
	f, srv := newFakeVBR(t, "1.3-rev2")
	c := mustConnect(t, srv)

	if err := c.DeleteJob(t.Context(), "job-1", fastWait); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	for _, req := range []string{
		"DELETE /api/v1/backups/b1?fromDB=false&includeGFS=true",
		"DELETE /api/v1/jobs/job-1",
	} {
		if !f.saw(req) {
			t.Errorf("expected request %q", req)
		}
	}

	if f.saw("DELETE /api/v1/backups/b-other?fromDB=false&includeGFS=true") {
		t.Error("deleted a backup that belongs to another job")
	}

	// A job that is already gone is not an error.
	if err := c.DeleteJob(t.Context(), "gone", fastWait); err != nil {
		t.Errorf("DeleteJob of a missing job: %v", err)
	}
}

func TestJobName(t *testing.T) {
	name := veeam.JobName("abc123", "my-vm")
	if name != "vmop-e2e-abc123-my-vm" {
		t.Errorf("unexpected job name %q", name)
	}
}
