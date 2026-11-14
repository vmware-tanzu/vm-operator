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
	"golang.org/x/net/proxy"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/redact"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
)

const (
	sshRetryDuration = time.Minute
	sshRetryInterval = 5 * time.Second

	// sshHandshakeTimeout bounds the TCP connect and SSH handshake of a single
	// dial attempt.
	sshHandshakeTimeout = 30 * time.Second
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
		Timeout:         sshHandshakeTimeout,
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
		newClient, lastErr = dialSSH(addr, config)
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

// dialSSH opens an SSH connection to addr. The TCP connection honors the
// ALL_PROXY / NO_PROXY environment variables (via proxy.FromEnvironment), so in
// an isolated testbed, where ALL_PROXY=socks5h://127.0.0.1:<port> points at a
// tunnel through the jumphost, the vCenter and Supervisor become reachable. With
// no ALL_PROXY set this is a plain direct TCP dial.
func dialSSH(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	conn, err := proxy.FromEnvironment().Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Bound the handshake so a proxy that accepts the connection but never
	// completes it cannot hang the caller forever.
	if config.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(config.Timeout))
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	return ssh.NewClient(ncc, chans, reqs), nil
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

// GetGatewayHost returns the host of the SSH gateway VM (the external gateway
// that fronts guest VMs). GATEWAY_IP is authoritative. For backwards
// compatibility it falls back to the host of HTTP_PROXY, which on a normal
// testbed is the same gateway running Squid. HTTP_PROXY is never used when it
// is a URL with a scheme (e.g. socks5h://127.0.0.1:1080 in an isolated
// testbed), since that is a proxy address, not the gateway.
func GetGatewayHost() (string, error) {
	if ip := strings.TrimSpace(os.Getenv(consts.GatewayIPEnv)); ip != "" {
		return ip, nil
	}

	httpProxy := os.Getenv(consts.HTTPProxyEnv)
	if httpProxy == "" {
		return "", fmt.Errorf("%s or %s environment variable should be set",
			consts.GatewayIPEnv, consts.HTTPProxyEnv)
	}

	if strings.Contains(httpProxy, "://") {
		return "", fmt.Errorf("%s=%q is a proxy URL, not a gateway address; set %s",
			consts.HTTPProxyEnv, httpProxy, consts.GatewayIPEnv)
	}

	parsedURL, err := url.Parse("http://" + httpProxy)
	if err != nil {
		return "", err
	}

	return parsedURL.Hostname(), nil
}

// GetGateway returns the SSH gateway VM identity. See GetGatewayHost.
func GetGateway() (*Gateway, error) {
	host, err := GetGatewayHost()
	if err != nil {
		return nil, err
	}

	return &Gateway{
		Hostname:    host,
		Username:    testbed.GatewayUsername,
		Port:        consts.SshPort,
		AuthMethods: []ssh.AuthMethod{ssh.Password(testbed.GatewayPassword)},
	}, nil
}
