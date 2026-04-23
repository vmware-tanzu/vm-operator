/*
Copyright 2020 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// https://github.com/kubernetes-sigs/cluster-api/blob/a260cb4c3b2cf2f32379f6ae5d8c6312780e9ca5/test/framework/exec/kubectl.go

package framework

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func KubectlRawWithArgs(ctx context.Context, kubeconfigPath, oper string, resources []byte, args ...string) (
	[]byte, []byte, error) {
	aargs := append([]string{oper, "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)

	return applyCmd.Run(ctx)
}

// TODO: Remove this usage of kubectl and replace with a function from apply.go using the controller-runtime client.
func KubectlApply(ctx context.Context, kubeconfigPath string, resources []byte) error {
	return KubectlApplyWithArgs(ctx, kubeconfigPath, resources)
}

// Example:
// ApplyWithArgs(ctx, workloadClusterTemplate, "--selector", "kcp-adoption.step1").
func KubectlApplyWithArgs(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	stdout, stderr, err := KubectlApplyRawWithArgs(ctx, kubeconfigPath, resources, args...)
	if err != nil {
		// Check if stderr is not empty and append it to the error message
		if len(stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, string(stderr))
		}

		fmt.Println(string(stderr))

		return err
	}

	fmt.Println(string(stdout))

	return nil
}

// KubectlApplyRawWithArgs applies the given yaml in bytes and arguments and return the kubectl command stdout, stderr and the error if there is any.
func KubectlApplyRawWithArgs(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) ([]byte, []byte, error) {
	return KubectlRawWithArgs(ctx, kubeconfigPath, "apply", resources, args...)
}

func KubectlCreate(ctx context.Context, kubeconfigPath string, resources []byte) error {
	return KubectlCreateWithArgs(ctx, kubeconfigPath, resources)
}

func KubectlCreateWithArgs(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	aargs := append([]string{"create", "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}

	fmt.Println(string(stdout))

	return nil
}

func KubectlCreateRawWithArgs(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) ([]byte, []byte, error) {
	return KubectlRawWithArgs(ctx, kubeconfigPath, "create", resources, args...)
}

func KubectlDelete(ctx context.Context, kubeconfigPath string, resources []byte) error {
	return KubectlDeleteWithArgs(ctx, kubeconfigPath, resources)
}

func KubectlDeleteWithNamespacedName(ctx context.Context, kubeconfigPath, resource, ns, name string) error {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "delete"}, resource, "-n", ns, name)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	_, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}

	return nil
}

func KubectlDeleteWithArgs(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	aargs := append([]string{"delete", "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}

	fmt.Println(string(stdout))

	return nil
}

func KubectlWait(ctx context.Context, kubeconfigPath string, args ...string) error {
	wargs := append([]string{"wait", "--kubeconfig", kubeconfigPath}, args...)
	wait := NewCommand(
		WithCommand("kubectl"),
		WithArgs(wargs...),
	)

	_, stderr, err := wait.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}

	return nil
}

func KubectlGet(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "get"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Printf("stderr: %s", string(stderr))
		fmt.Printf("err: %v", err)

		return []byte(""), err
	}

	return stdout, nil
}

func KubectlExec(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "exec"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return []byte(""), err
	}

	return stdout, nil
}

func KubectlCreateWithoutFile(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "create"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return []byte(""), err
	}

	return stdout, nil
}

func KubectlConfig(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "config"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return []byte(""), err
	}

	return stdout, nil
}

func KubectlAuth(ctx context.Context, kubeconfigPath string, args ...string) ([]byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "auth"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)
	fmt.Printf("Running command: %s %s\n", applyCmd.Cmd, strings.Join(applyCmd.Args, " "))
	stdout, stderr, err := applyCmd.Run(ctx)
	fmt.Printf("stdout: %s\n", string(stdout))

	if err != nil {
		fmt.Println(string(stderr))
		return stdout, err
	}

	return stdout, nil
}

func KubectlLabel(ctx context.Context, kubeconfigPath string, args ...string) error {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "label"}, args...)

	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}

	fmt.Println(string(stdout))

	return nil
}

func KubectlDescribeWithNamespacedName(ctx context.Context, kubeconfigPath, resource, ns, name string) ([]byte, []byte, error) {
	aargs := append([]string{"--kubeconfig", kubeconfigPath, "describe"}, resource, "-n", ns, name)
	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
	)

	return applyCmd.Run(ctx)
}

// Command wraps Command with specific functionality.
// This differentiates itself from the standard library by always collecting stdout and stderr.
// Command improves the UX of Command for our specific use case.
type Command struct {
	Cmd   string
	Args  []string
	Stdin io.Reader
}

// Option is a functional option type that modifies a Command.
type Option func(*Command)

// NewCommand returns a configured Command.
func NewCommand(opts ...Option) *Command {
	cmd := &Command{
		Stdin: nil,
	}
	for _, option := range opts {
		option(cmd)
	}

	return cmd
}

// WithStdin sets up the command to read from this io.Reader.
func WithStdin(stdin io.Reader) Option {
	return func(cmd *Command) {
		cmd.Stdin = stdin
	}
}

// WithCommand defines the command to run such as `kubectl` or `kind`.
func WithCommand(command string) Option {
	return func(cmd *Command) {
		cmd.Cmd = command
	}
}

// WithArgs sets the arguments for the command such as `get pods -n kube-system` to the command `kubectl`.
func WithArgs(args ...string) Option {
	return func(cmd *Command) {
		cmd.Args = args
	}
}

// Run executes the command and returns stdout, stderr and the error if there is any.
func (c *Command) Run(ctx context.Context) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...) //nolint:gosec
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, nil, err
	}

	errout, err := io.ReadAll(stderr)
	if err != nil {
		return nil, nil, err
	}

	if err := cmd.Wait(); err != nil {
		return output, errout, err
	}

	return output, errout, nil
}
