// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package veeam

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Session states and results reported by VBR.
const (
	SessionStateStarting = "Starting"
	SessionStateStopped  = "Stopped"

	SessionResultSuccess = "Success"
	SessionResultWarning = "Warning"
	SessionResultFailed  = "Failed"
)

// InventoryObject is a vSphere object as seen by VBR.
type InventoryObject struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	ObjectID string `json:"objectId"`
	HostName string `json:"hostName"`
	Platform string `json:"platform,omitempty"`
}

// Job is a VBR backup job.
type Job struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SessionResult is the outcome of a VBR session.
type SessionResult struct {
	Result     string `json:"result"`
	Message    string `json:"message"`
	IsCanceled bool   `json:"isCanceled"`
}

// Session is an asynchronous VBR operation (backup run, restore, delete).
type Session struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	SessionType     string        `json:"sessionType"`
	State           string        `json:"state"`
	ProgressPercent int           `json:"progressPercent"`
	Result          SessionResult `json:"result"`
}

// Succeeded reports whether a stopped session ended in Success or Warning.
// VBR reports Warning for benign conditions such as a vCenter newer than the
// build was tested against.
func (s Session) Succeeded() bool {
	return s.State == SessionStateStopped &&
		(s.Result.Result == SessionResultSuccess || s.Result.Result == SessionResultWarning)
}

// RestorePoint is a point-in-time backup of one VM.
type RestorePoint struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	BackupID     string    `json:"backupId"`
	CreationTime time.Time `json:"creationTime"`
}

type backup struct {
	ID    string `json:"id"`
	JobID string `json:"jobId"`
	Name  string `json:"name"`
}

type repository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type page[T any] struct {
	Data []T `json:"data"`
}

// SessionError is returned when a session fails, times out, or never starts.
// It carries VBR's own identifiers and log so a CI failure is diagnosable
// without access to the appliance.
type SessionError struct {
	Reason  string
	Session Session
	Log     []string
}

func (e *SessionError) Error() string {
	return fmt.Sprintf("veeam session %s (%s %q) %s: state=%s result=%s message=%q log=[%s]",
		e.Session.ID, e.Session.SessionType, e.Session.Name, e.Reason,
		e.Session.State, e.Session.Result.Result, e.Session.Result.Message,
		strings.Join(e.Log, "; "))
}

// FindVM returns the VBR inventory entry for the VM with the given name and
// managed object ID on the vCenter that VBR knows as vcName (its PNID).
// Matching on the moref as well as the name avoids picking a same-named VM
// from another namespace.
func (c *Client) FindVM(ctx context.Context, vcName, vmName, moID string) (InventoryObject, error) {
	var resp page[InventoryObject]

	path := fmt.Sprintf("/api/v1/inventory/vmware/hosts/%s?nameFilter=%s",
		url.PathEscape(vcName), url.QueryEscape(vmName))
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return InventoryObject{}, err
	}

	for _, o := range resp.Data {
		if o.Type == "VirtualMachine" && o.Name == vmName && o.ObjectID == moID {
			return o, nil
		}
	}

	return InventoryObject{}, fmt.Errorf("veeam inventory of %q has no VM %q with objectId %q (%d candidates)",
		vcName, vmName, moID, len(resp.Data))
}

// RepositoryID returns the ID of the backup repository with the given name,
// or of the first repository when name is empty.
func (c *Client) RepositoryID(ctx context.Context, name string) (string, error) {
	var resp page[repository]
	if err := c.do(ctx, "GET", "/api/v1/backupInfrastructure/repositories", nil, &resp); err != nil {
		return "", err
	}

	for _, r := range resp.Data {
		if name == "" || r.Name == name {
			return r.ID, nil
		}
	}

	return "", fmt.Errorf("veeam has no backup repository named %q", name)
}

// CreateJob creates a backup job for a single VM that only runs when started
// explicitly.
func (c *Client) CreateJob(ctx context.Context, name, repositoryID string, vm InventoryObject) (Job, error) {
	jobType := "VSphereBackup"
	if c.legacyAPI() {
		jobType = "Backup"
	}

	if vm.Platform == "" {
		vm.Platform = "VSphere"
	}

	spec := map[string]any{
		"type":           jobType,
		"name":           name,
		"description":    "Created by the VM Operator E2E suite.",
		"isHighPriority": false,
		"virtualMachines": map[string]any{
			"includes": []InventoryObject{vm},
		},
		"storage": map[string]any{
			"backupRepositoryId": repositoryID,
			"backupProxies":      map[string]any{"autoSelectEnabled": true},
			"retentionPolicy":    map[string]any{"type": "RestorePoints", "quantity": 7},
		},
		"guestProcessing": map[string]any{
			"appAwareProcessing": map[string]any{"isEnabled": false},
			"guestFSIndexing":    map[string]any{"isEnabled": false},
		},
		"schedule": map[string]any{"runAutomatically": false},
	}

	var job Job
	if err := c.do(ctx, "POST", "/api/v1/jobs", spec, &job); err != nil {
		return Job{}, err
	}

	if job.ID == "" {
		return Job{}, fmt.Errorf("veeam created job %q but returned no id", name)
	}

	return job, nil
}

// StartJob starts an incremental run of a backup job.
func (c *Client) StartJob(ctx context.Context, jobID string) (Session, error) {
	var s Session
	err := c.do(ctx, "POST", "/api/v1/jobs/"+jobID+"/start", map[string]any{"performActiveFull": false}, &s)

	return s, err
}

// GetSession returns the current state of a session.
func (c *Client) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	err := c.do(ctx, "GET", "/api/v1/sessions/"+id, nil, &s)

	return s, err
}

// SessionLog returns the session's log records as "<status> <title>" lines.
func (c *Client) SessionLog(ctx context.Context, id string) ([]string, error) {
	var resp struct {
		Records []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
		} `json:"records"`
	}
	if err := c.do(ctx, "GET", "/api/v1/sessions/"+id+"/logs", nil, &resp); err != nil {
		return nil, err
	}

	lines := make([]string, 0, len(resp.Records))
	for _, r := range resp.Records {
		lines = append(lines, r.Status+" "+r.Title)
	}

	return lines, nil
}

// WaitOptions controls WaitForSession.
type WaitOptions struct {
	// Timeout bounds the whole wait.
	Timeout time.Duration
	// StartTimeout bounds how long the session may stay in the Starting
	// state, so a job that never starts fails fast.
	StartTimeout time.Duration
	// Interval is the poll interval.
	Interval time.Duration
}

// WaitForSession polls a session until it stops. It returns the final
// session when it ended in Success or Warning, and a *SessionError when it
// failed, timed out, or never left the Starting state.
func (c *Client) WaitForSession(ctx context.Context, id string, opts WaitOptions) (Session, error) {
	deadline := time.Now().Add(opts.Timeout)
	startDeadline := time.Now().Add(opts.StartTimeout)

	for {
		s, err := c.GetSession(ctx, id)
		if err != nil {
			return s, err
		}

		switch {
		case s.Succeeded():
			return s, nil
		case s.State == SessionStateStopped:
			return s, c.sessionError(ctx, "failed", s)
		case opts.StartTimeout > 0 && s.State == SessionStateStarting && time.Now().After(startDeadline):
			return s, c.sessionError(ctx, fmt.Sprintf("did not start within %s", opts.StartTimeout), s)
		case time.Now().After(deadline):
			return s, c.sessionError(ctx, fmt.Sprintf("did not finish within %s", opts.Timeout), s)
		}

		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-time.After(opts.Interval):
		}
	}
}

func (c *Client) sessionError(ctx context.Context, reason string, s Session) error {
	log, err := c.SessionLog(ctx, s.ID)
	if err != nil {
		log = []string{fmt.Sprintf("<failed to fetch session log: %v>", err)}
	}

	return &SessionError{Reason: reason, Session: s, Log: log}
}

// Backup runs the job and waits for the run to succeed.
func (c *Client) Backup(ctx context.Context, jobID string, opts WaitOptions) (Session, error) {
	s, err := c.StartJob(ctx, jobID)
	if err != nil {
		return s, fmt.Errorf("failed to start veeam job %s: %w", jobID, err)
	}

	return c.WaitForSession(ctx, s.ID, opts)
}

func (c *Client) backupsForJob(ctx context.Context, jobID string) ([]backup, error) {
	var resp page[backup]
	if err := c.do(ctx, "GET", "/api/v1/backups?jobIdFilter="+url.QueryEscape(jobID), nil, &resp); err != nil {
		return nil, err
	}

	// Filter client-side as well, in case the server ignores the filter.
	var out []backup

	for _, b := range resp.Data {
		if b.JobID == "" || b.JobID == jobID {
			out = append(out, b)
		}
	}

	return out, nil
}

// LatestRestorePoint returns the newest restore point produced by the job.
func (c *Client) LatestRestorePoint(ctx context.Context, jobID string) (RestorePoint, error) {
	backups, err := c.backupsForJob(ctx, jobID)
	if err != nil {
		return RestorePoint{}, err
	}

	var points []RestorePoint

	for _, b := range backups {
		var resp page[RestorePoint]
		if err := c.do(ctx, "GET", "/api/v1/restorePoints?backupIdFilter="+url.QueryEscape(b.ID), nil, &resp); err != nil {
			return RestorePoint{}, err
		}

		points = append(points, resp.Data...)
	}

	if len(points) == 0 {
		return RestorePoint{}, fmt.Errorf("veeam job %s has no restore points (%d backups)", jobID, len(backups))
	}

	sort.Slice(points, func(i, j int) bool { return points[i].CreationTime.After(points[j].CreationTime) })

	return points[0], nil
}

// RestoreVM restores the entire VM from a restore point to its original
// location, powered off. With overwrite false the VM is recreated (restore
// to new, which yields a new moref); with overwrite true the existing VM is
// restored in place (its moref is kept).
func (c *Client) RestoreVM(ctx context.Context, restorePointID string, overwrite bool, reason string, opts WaitOptions) (Session, error) {
	path := "/api/v1/restore/vmRestore/vSphere"
	if c.legacyAPI() {
		path = "/api/v1/restore/vmRestore/vmware/"
	}

	spec := map[string]any{
		"type":           "OriginalLocation",
		"restorePointId": restorePointID,
		"powerUp":        false,
		"overwrite":      overwrite,
		"reason":         reason,
	}

	var s Session
	if err := c.do(ctx, "POST", path, spec, &s); err != nil {
		return s, fmt.Errorf("failed to start veeam restore of restore point %s: %w", restorePointID, err)
	}

	return c.WaitForSession(ctx, s.ID, opts)
}

// DeleteJob deletes the job's backups (including their files on the
// repository) and then the job itself. Missing objects are ignored, so it is
// safe to call from cleanup regardless of how far a test got.
func (c *Client) DeleteJob(ctx context.Context, jobID string, opts WaitOptions) error {
	backups, err := c.backupsForJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to list backups of veeam job %s: %w", jobID, err)
	}

	var errs []error

	for _, b := range backups {
		// fromDB=false deletes the backup files too; fromDB=true would only
		// forget them and leave the data on the repository.
		var s Session

		err := c.do(ctx, "DELETE", "/api/v1/backups/"+b.ID+"?fromDB=false&includeGFS=true", nil, &s)

		switch {
		case IsNotFound(err):
		case err != nil:
			errs = append(errs, fmt.Errorf("failed to delete veeam backup %s: %w", b.ID, err))
		case s.ID != "":
			if _, err := c.WaitForSession(ctx, s.ID, opts); err != nil {
				errs = append(errs, fmt.Errorf("failed to delete veeam backup %s: %w", b.ID, err))
			}
		}
	}

	if err := c.do(ctx, "DELETE", "/api/v1/jobs/"+jobID, nil, nil); err != nil && !IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete veeam job %s: %w", jobID, err))
	}

	return errors.Join(errs...)
}

// JobName returns the name of the backup job the E2E suite creates for a VM.
// The run ID keeps concurrent runs against a shared appliance apart and lets
// leaked jobs be traced back to the run that created them.
func JobName(runID, vmName string) string {
	return "vmop-e2e-" + runID + "-" + vmName
}
