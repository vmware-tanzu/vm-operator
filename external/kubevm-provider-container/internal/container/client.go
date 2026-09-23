// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package container drives a local Docker or Podman engine through its CLI.
//
// A CLI, not a client library: docker and podman both ship a CLI that already
// speaks to whichever engine is actually installed (including a remote
// DOCKER_HOST), while their Go client libraries pull in a much larger
// dependency tree and, for podman, an HTTP API that is not always enabled.
// For a teaching example the CLI is also the thing a reader can run by hand
// on the same command line the tests fake — see FakeRunner in this package's
// tests for how that keeps "no test needs a real container engine" provable
// by construction, the same guarantee external/kubevm-provider-aws gets from
// its narrow ec2.Client interface.
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes one CLI invocation and returns its stdout.
//
// The seam every test substitutes. A production Runner is exec.Command; a
// test Runner is a map from expected args to canned output, so unit tests for
// the reconcile logic in controllers/containermachine never shell out.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, err error)
}

// ExecRunner runs a real binary on the host.
type ExecRunner struct{}

// Run implements Runner by invoking exec.CommandContext.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w: %s", name, args, err, out)
	}
	return string(out), nil
}

// Client drives one container engine (docker or podman) through Runner.
type Client struct {
	// Runtime is the binary to invoke: "docker" or "podman".
	Runtime string

	// Runner executes the invocation. Nil means ExecRunner.
	Runner Runner
}

func (c Client) run(ctx context.Context, args ...string) (string, error) {
	r := c.Runner
	if r == nil {
		r = ExecRunner{}
	}
	return r.Run(ctx, c.Runtime, args...)
}

// Inspect is the subset of `docker/podman inspect` this provider reads.
type Inspect struct {
	ID    string `json:"Id"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
	NetworkSettings struct {
		IPAddress string `json:"IPAddress"`
	} `json:"NetworkSettings"`
}

// Inspect reads the named container's state, or (nil, false, nil) if the
// engine has no container by that name.
//
// `--format '{{json .}}'` and a Go JSON unmarshal, not a hand-assembled
// Go-template field list: the shape below is a small, stable subset of a much
// larger struct that both engines already serialize correctly, and asking the
// template engine to assemble that JSON itself is the same amount of code with
// far more ways to get a field name wrong.
func (c Client) Inspect(ctx context.Context, name string) (*Inspect, bool, error) {
	out, err := c.run(ctx, "inspect", "--format", "{{json .}}", name)
	if err != nil {
		// Both engines exit non-zero and print "No such object" on a name
		// they do not have; neither offers a distinct exit code for it, so
		// the message is what this provider has to go on.
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var ins Inspect
	if err := json.Unmarshal([]byte(out), &ins); err != nil {
		return nil, false, fmt.Errorf("parsing %q inspect output: %w", name, err)
	}
	return &ins, true, nil
}

// Create starts a new detached container under the given name.
//
// Idempotency here is by NAME, not by a client token: unlike EC2, a
// docker/podman container name is a strongly consistent, caller-chosen unique
// key the engine itself enforces (a second `run` under the same name fails
// outright), so a lookup immediately after a failed or interrupted create
// tells the truth. See docs/findings.md for why that removes the whole
// class of problem external/kubevm-provider-aws's ClientToken/propagation-grace
// machinery exists to solve.
func (c Client) Create(ctx context.Context, name, image string) (string, error) {
	out, err := c.run(ctx, "run", "-d", "--name", name, image)
	if err != nil {
		return "", err
	}
	// A successful `run -d` prints the new container's full ID on stdout.
	id := firstLine(out)
	if id == "" {
		return "", fmt.Errorf("creating container %q: no id in output", name)
	}
	return id, nil
}

// Start starts an existing, stopped container.
func (c Client) Start(ctx context.Context, name string) error {
	_, err := c.run(ctx, "start", name)
	return err
}

// Stop stops a running container.
func (c Client) Stop(ctx context.Context, name string) error {
	_, err := c.run(ctx, "stop", name)
	return err
}

// Remove force-removes a container, ignoring one the engine no longer has.
func (c Client) Remove(ctx context.Context, name string) error {
	_, err := c.run(ctx, "rm", "-f", name)
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func isNotFound(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "No such object") ||
		strings.Contains(msg, "No such container") ||
		strings.Contains(msg, "no such container")
}

// IsNameConflict reports whether err is a Create failure because a
// container by that name already exists -- the recoverable case described
// on Create's own doc comment, distinct from every other Create failure
// (bad image, engine unreachable), which is genuinely terminal.
func IsNameConflict(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "already in use") ||
		strings.Contains(msg, "already exists")
}
