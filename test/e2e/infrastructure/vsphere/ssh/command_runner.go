// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/redact"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
)

const (
	sshRetryDuration = time.Minute
	sshRetryInterval = 5 * time.Second
)

// SSHCommandRunner knows how to run SSH commands on an arbitrary host. This is
// useful to directly run commands, e.g. on the VC or a supervisor control
// plane VM. Callers should Close the runner when done with it.
type SSHCommandRunner interface {
	// RunCommand runs the command on the host and returns its combined
	// STDOUT and STDERR.
	RunCommand(cmd string) ([]byte, error)
	// Close closes all connections held by the runner.
	Close() error
}

// sshCommandRunnerImpl implements the SSHCommandRunner interface, using the
// Golang ssh library.
type sshCommandRunnerImpl struct {
	client *ssh.Client
	// gatewayClient is the jump box connection, if any, that client is
	// tunneled through.
	gatewayClient *ssh.Client
}

type Gateway struct {
	Hostname    string
	Username    string
	Port        int
	AuthMethods []ssh.AuthMethod
}

// NewSSHCommandRunnerFromHTTPProxy returns a helper configured based on the
// HTTP_PROXY set in the env.
func NewSSHCommandRunnerFromHTTPProxy() (SSHCommandRunner, error) {
	gateway, err := GetGateway()
	if err != nil {
		return nil, err
	}

	return NewSSHCommandRunner(gateway.Hostname, gateway.Port, gateway.Username, gateway.AuthMethods)
}

// newClientConfig returns the client config for the user and auth methods.
// Host keys are not verified since the test environments are ephemeral.
func newClientConfig(user string, authMethods []ssh.AuthMethod) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

// newSSHDialerWithRetries dials the host, retrying every interval until the
// timeout. The error from the last dial is returned on timeout.
func newSSHDialerWithRetries(hostname string, port int, config *ssh.ClientConfig, interval time.Duration, timeout time.Duration) (*ssh.Client, error) {
	var (
		newClient *ssh.Client
		lastErr   error
	)

	addr := net.JoinHostPort(hostname, strconv.Itoa(port))

	pollErr := wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(context.Context) (bool, error) {
		newClient, lastErr = ssh.Dial("tcp", addr, config)
		return lastErr == nil, nil
	})
	if lastErr != nil {
		return nil, fmt.Errorf("failed to dial SSH host %s: %w", addr, lastErr)
	}
	if pollErr != nil {
		return nil, fmt.Errorf("failed to dial SSH host %s: %w", addr, pollErr)
	}

	return newClient, nil
}

// NewSSHCommandRunner returns a helper configured with the given host, for the given username and auth methods.
func NewSSHCommandRunner(hostname string, port int, user string, authMethods []ssh.AuthMethod) (SSHCommandRunner, error) {
	newClient, err := newSSHDialerWithRetries(hostname, port, newClientConfig(user, authMethods), sshRetryInterval, sshRetryDuration)
	if err != nil {
		return nil, err
	}

	return &sshCommandRunnerImpl{
		client: newClient,
	}, nil
}

// NewSSHCommandRunnerWithinGateway uses the gateway as a jump box to create and return a helper configured with the given host, for the given username and auth methods.
func NewSSHCommandRunnerWithinGateway(hostname string, port int, user string, authMethods []ssh.AuthMethod, gw Gateway) (SSHCommandRunner, error) {
	gatewayClient, err := newSSHDialerWithRetries(gw.Hostname, gw.Port, newClientConfig(gw.Username, gw.AuthMethods), sshRetryInterval, sshRetryDuration)
	if err != nil {
		return nil, err
	}

	targetHost := net.JoinHostPort(hostname, strconv.Itoa(port))

	conn, err := gatewayClient.Dial("tcp", targetHost)
	if err != nil {
		_ = gatewayClient.Close()
		return nil, fmt.Errorf("failed to dial %s through gateway %s: %w", targetHost, gw.Hostname, err)
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, targetHost, newClientConfig(user, authMethods))
	if err != nil {
		_ = conn.Close()
		_ = gatewayClient.Close()
		return nil, fmt.Errorf("failed to create SSH connection to %s through gateway %s: %w", targetHost, gw.Hostname, err)
	}

	return &sshCommandRunnerImpl{
		client:        ssh.NewClient(ncc, chans, reqs),
		gatewayClient: gatewayClient,
	}, nil
}

func (s *sshCommandRunnerImpl) RunCommand(cmd string) ([]byte, error) {
	if s.client == nil {
		return nil, fmt.Errorf("s.client is nil")
	}

	// Ignore python warnings if the env var is not specified in the command.
	// This avoids parsing errors when the command output contains warnings.
	if !strings.Contains(cmd, "PYTHONWARNINGS") {
		cmd = fmt.Sprintf("PYTHONWARNINGS=ignore %s", cmd)
	}

	remote := s.client.RemoteAddr()

	session, err := s.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session on %s: %w", remote, err)
	}
	defer func() { _ = session.Close() }()

	e2eframework.Logf("Running command via SSH on %s: %s", remote, redact.RedactSensitiveFlags(cmd))

	out, err := session.CombinedOutput(cmd)

	e2eframework.Logf("Ran command via SSH on %s, output: %s, error: %v", remote, redact.RedactSensitiveOutput(string(out)), err)

	return out, err
}

func (s *sshCommandRunnerImpl) Close() error {
	var errs []error

	if s.client != nil {
		errs = append(errs, s.client.Close())
	}
	if s.gatewayClient != nil {
		errs = append(errs, s.gatewayClient.Close())
	}

	return errors.Join(errs...)
}

func GetGateway() (*Gateway, error) {
	httpProxy := os.Getenv(consts.HTTPProxyEnv)
	if httpProxy == "" {
		return nil, errors.New("HTTP_PROXY environment variable should be set")
	}

	parsedURL, err := url.Parse("http://" + httpProxy)
	if err != nil {
		return nil, err
	}

	gatewayIP := parsedURL.Hostname()
	gw := &Gateway{
		Hostname:    gatewayIP,
		Username:    testbed.GatewayUsername,
		Port:        consts.SshPort,
		AuthMethods: []ssh.AuthMethod{ssh.Password(testbed.GatewayPassword)},
	}

	return gw, nil
}
