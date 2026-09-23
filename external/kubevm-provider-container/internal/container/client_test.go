// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package container_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/internal/container"
)

// fakeRunner answers a fixed script of CLI invocations without shelling out
// to a real docker/podman binary — the seam that lets this package's tests,
// and the containermachine controller's tests, run with no container engine
// installed at all.
type fakeRunner struct {
	// calls records every invocation, joined args first.
	calls [][]string

	// outputs is popped in order, one per call; err is returned instead when
	// set for that call.
	outputs []string
	errs    []error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	i := len(f.calls) - 1
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	var out string
	if i < len(f.outputs) {
		out = f.outputs[i]
	}
	return out, err
}

func TestInspectNotFound(t *testing.T) {
	f := &fakeRunner{errs: []error{errors.New("Error: No such object: missing")}}
	c := container.Client{Runtime: "docker", Runner: f}

	ins, found, err := c.Inspect(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Inspect returned an error for a not-found container: %v", err)
	}
	if found || ins != nil {
		t.Fatalf("Inspect reported found=%v for a container the engine does not have", found)
	}
}

func TestInspectRunning(t *testing.T) {
	f := &fakeRunner{outputs: []string{
		`{"Id":"abc123","State":{"Status":"running","Running":true},` +
			`"NetworkSettings":{"IPAddress":"172.17.0.5"}}`,
	}}
	c := container.Client{Runtime: "docker", Runner: f}

	ins, found, err := c.Inspect(context.Background(), "kubevm-default-my-vm")
	if err != nil {
		t.Fatalf("Inspect returned an unexpected error: %v", err)
	}
	if !found {
		t.Fatal("Inspect reported not found for a container the fake says exists")
	}
	if ins.ID != "abc123" || !ins.State.Running ||
		ins.NetworkSettings.IPAddress != "172.17.0.5" {
		t.Fatalf("Inspect parsed the wrong values: %+v", ins)
	}
}

func TestCreateReadsIDFromFirstLine(t *testing.T) {
	f := &fakeRunner{outputs: []string{"deadbeef1234\n"}}
	c := container.Client{Runtime: "docker", Runner: f}

	id, err := c.Create(context.Background(), "kubevm-default-my-vm", "nginx:latest")
	if err != nil {
		t.Fatalf("Create returned an unexpected error: %v", err)
	}
	if id != "deadbeef1234" {
		t.Fatalf("Create returned id %q, want %q", id, "deadbeef1234")
	}
	if len(f.calls) != 1 || f.calls[0][0] != "docker" {
		t.Fatalf("Create did not invoke the docker binary: %v", f.calls)
	}
}

func TestIsNameConflict(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "podman refusal",
			err:  errors.New("Error: creating container storage: the container name \"kubevm-team-a-web-01\" is already in use by ... You have to remove that container to be able to reuse that name"),
			want: true,
		},
		{
			name: "docker refusal",
			err: errors.New("docker: Error response from daemon: Conflict. " +
				"The container name \"/kubevm-team-a-web-01\" is already in " +
				"use by container \"abc123\". You have to remove (or rename) " +
				"that container to be able to reuse that name."),
			want: true,
		},
		{
			name: "unrelated failure is not a conflict",
			err:  errors.New("Error: pull access denied for no-such-image"),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := container.IsNameConflict(tc.err); got != tc.want {
				t.Fatalf("IsNameConflict(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRemoveIgnoresNotFound(t *testing.T) {
	f := &fakeRunner{errs: []error{errors.New("Error: No such container: gone")}}
	c := container.Client{Runtime: "podman", Runner: f}

	if err := c.Remove(context.Background(), "gone"); err != nil {
		t.Fatalf("Remove should ignore a container the engine no longer has, got: %v", err)
	}
}
