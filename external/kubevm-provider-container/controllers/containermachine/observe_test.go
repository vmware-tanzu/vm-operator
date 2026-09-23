// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package containermachine

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	containerv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/internal/container"
)

// scriptedRunner answers a fixed sequence of CLI invocations, one entry per
// call, without shelling out to a real docker/podman binary.
type scriptedRunner struct {
	calls   int
	outputs []string
	errs    []error
}

func (s *scriptedRunner) Run(_ context.Context, _ string, _ ...string) (string, error) {
	i := s.calls
	s.calls++
	var out string
	var err error
	if i < len(s.outputs) {
		out = s.outputs[i]
	}
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return out, err
}

// TestReconcileContainerRecoversFromNameConflict exercises the case
// docs/findings.md's "A Create that succeeds can still look like a failure
// to the next reconcile" describes: a prior reconcile's Create actually
// succeeded but never got recorded, so this reconcile's retry sees the
// engine refuse to create a second container under the same name. Confirms
// reconcileContainer recovers by Inspect instead of reporting it as a
// terminal RuntimeError, the recovery internal/container.Client.Create's own
// doc comment promises.
func TestReconcileContainerRecoversFromNameConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := containerv1a1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	machine := &containerv1a1.ContainerMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "web-01", Namespace: "team-a"},
		Spec: containerv1a1.ContainerMachineSpec{
			Image:      "nginx",
			Runtime:    containerv1a1.ContainerRuntimeDocker,
			PowerState: "PoweredOn",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(machine).
		WithStatusSubresource(&containerv1a1.ContainerMachine{}).
		Build()

	runner := &scriptedRunner{
		// Call 0: Create ("run -d ...") refused as a name conflict.
		// Calls 1-2: Inspect, once in reconcileContainer's own recovery
		// lookup and once more in settle's unconditional re-inspect.
		errs: []error{
			errors.New("Error: response from daemon: Conflict. The container " +
				"name \"/kubevm-team-a-web-01\" is already in use by container " +
				"\"abc123\". You have to remove (or rename) that container to " +
				"be able to reuse that name."),
		},
		outputs: []string{
			"",
			`{"Id":"abc123","State":{"Status":"running","Running":true},` +
				`"NetworkSettings":{"IPAddress":"172.17.0.9"}}`,
			`{"Id":"abc123","State":{"Status":"running","Running":true},` +
				`"NetworkSettings":{"IPAddress":"172.17.0.9"}}`,
		},
	}

	r := &Reconciler{
		Client: c,
		Engine: func(containerv1a1.ContainerRuntime) container.Client {
			return container.Client{Runtime: "docker", Runner: runner}
		},
	}

	if _, err := r.reconcileContainer(context.Background(), machine); err != nil {
		t.Fatalf("reconcileContainer returned an error recovering from a "+
			"name conflict: %v", err)
	}

	if machine.Status.ContainerID != "abc123" {
		t.Fatalf("Status.ContainerID = %q, want the inspected container's id %q",
			machine.Status.ContainerID, "abc123")
	}

	found := false
	for _, cond := range machine.Status.Conditions {
		if cond.Type == containerv1a1.ConditionInfrastructureReady {
			found = true
			if cond.Status != metav1.ConditionTrue {
				t.Fatalf("InfrastructureReady = %v, want True: %s",
					cond.Status, cond.Message)
			}
		}
	}
	if !found {
		t.Fatal("no InfrastructureReady condition was set")
	}
}

// TestReconcileContainerCreateFailureIsTerminal confirms a Create failure
// that is NOT a name conflict (bad image, engine unreachable) is still
// reported as a terminal RuntimeError, not swallowed by the new recovery
// path.
func TestReconcileContainerCreateFailureIsTerminal(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := containerv1a1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	machine := &containerv1a1.ContainerMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "web-01", Namespace: "team-a"},
		Spec: containerv1a1.ContainerMachineSpec{
			Image:      "no-such-image",
			Runtime:    containerv1a1.ContainerRuntimeDocker,
			PowerState: "PoweredOn",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(machine).
		WithStatusSubresource(&containerv1a1.ContainerMachine{}).
		Build()

	runner := &scriptedRunner{
		errs: []error{errors.New("Error: pull access denied for no-such-image")},
	}

	r := &Reconciler{
		Client: c,
		Engine: func(containerv1a1.ContainerRuntime) container.Client {
			return container.Client{Runtime: "docker", Runner: runner}
		},
	}

	if _, err := r.reconcileContainer(context.Background(), machine); err == nil {
		t.Fatal("reconcileContainer returned no error for a genuine, " +
			"non-recoverable Create failure")
	}

	for _, cond := range machine.Status.Conditions {
		if cond.Type == containerv1a1.ConditionInfrastructureReady &&
			cond.Reason != containerv1a1.ReasonRuntimeError {
			t.Fatalf("InfrastructureReady reason = %q, want %q",
				cond.Reason, containerv1a1.ReasonRuntimeError)
		}
	}
}
