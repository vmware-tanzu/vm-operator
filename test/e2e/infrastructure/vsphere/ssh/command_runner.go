// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
)

const (
	sshRetryDuration = time.Minute
	sshRetryInterval = 5 * time.Second
)

// SSHCommandRunner knows how to run SSH commands on an arbitrary host.
//
//	Useful if users want to directly run commands, eg. on the VC or a GC node.
//
// RunCommand runs the given command on the host the helper is configured with, returning its STDOUT.
type SSHCommandRunner interface {
	// Run a command and return its STDOUT.
	RunCommand(cmd string) ([]byte, error)
	RunCommandWindows(cmd string) ([]byte, error)
	Close() error
}

// sshCommandRunnerImpl implements the SSHCommandRunner interface, using the Golang ssh library.
type sshCommandRunnerImpl struct {
	client *ssh.Client
}

type Gateway struct {
	Hostname    string
	Username    string
	Port        int
	AuthMethods []ssh.AuthMethod
}

// NewSSHCommandRunner returns a helper configured based on the HTTP_PROXY set in the env.
func NewSSHCommandRunnerFromHTTPProxy() (SSHCommandRunner, error) {
	gateway, err := GetGateway()
	if err != nil {
		return nil, err
	}

	return NewSSHCommandRunner(gateway.Hostname, gateway.Port, gateway.Username, gateway.AuthMethods)
}

func newSSHDialerWithRetries(hostname string, port int, config *ssh.ClientConfig, interval time.Duration, timeout time.Duration) (*ssh.Client, error) {
	var newClient *ssh.Client

	//nolint:staticcheck // E2E SSH dial retries; migration to PollUntilContextTimeout is tracked separately.
	err := wait.PollWithContext(context.Background(), interval, timeout, func(ctx context.Context) (bool, error) {
		var err error
		if newClient, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", hostname, port), config); err != nil {
			return false, err
		}

		return true, nil
	})

	return newClient, err
}

// NewSSHCommandRunner returns a helper configured with the given host, for the given username and auth methods.
func NewSSHCommandRunner(hostname string, port int, user string, authMethods []ssh.AuthMethod) (SSHCommandRunner, error) {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // G106: E2E test clusters use host key validation disabled.
	}

	newClient, err := newSSHDialerWithRetries(hostname, port, config, sshRetryInterval, sshRetryDuration)
	if err != nil {
		return nil, err
	}

	return &sshCommandRunnerImpl{
		client: newClient,
	}, nil
}

// NewSSHCommandRunnerWithinGateway uses the gateway as a jump box to create and turn a helper configured with the given host, for the given username and auth methods,.
func NewSSHCommandRunnerWithinGateway(hostname string, port int, user string, authMethods []ssh.AuthMethod, gw Gateway) (SSHCommandRunner, error) {
	gwConfig := &ssh.ClientConfig{
		User:            gw.Username,
		Auth:            gw.AuthMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // G106: E2E test clusters use host key validation disabled.
	}

	gatewayClient, err := newSSHDialerWithRetries(gw.Hostname, gw.Port, gwConfig, sshRetryInterval, sshRetryDuration)
	if err != nil {
		return nil, err
	}

	targetHost := fmt.Sprintf("%s:%d", hostname, port)

	conn, err := gatewayClient.Dial("tcp", targetHost)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // G106: E2E test clusters use host key validation disabled.
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, targetHost, config)
	if err != nil {
		return nil, err
	}

	newClient := ssh.NewClient(ncc, chans, reqs)

	return &sshCommandRunnerImpl{
		client: newClient,
	}, nil
}

func (s *sshCommandRunnerImpl) RunCommand(cmd string) ([]byte, error) {
	e2eframework.Logf("Running command via SSH; remote: %s, command: %s", s.client.RemoteAddr(), cmd)

	if s.client == nil {
		return nil, fmt.Errorf("s.client is nil")
	}

	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	// Ignore python warnings if the env var is not specified in the command.
	// This avoid parsing errors when the command output contains warnings.
	if !strings.Contains(cmd, "PYTHONWARNINGS") {
		cmd = fmt.Sprintf("PYTHONWARNINGS=ignore %s", cmd)
	}

	combinedOutput, err := session.CombinedOutput(cmd)

	e2eframework.Logf("Ran command via SSH; remote: %s, command: %s, output: %s, error: %v", s.client.RemoteAddr(), cmd, string(combinedOutput), err)

	return combinedOutput, err
}

func (s *sshCommandRunnerImpl) RunCommandWindows(cmd string) ([]byte, error) {
	e2eframework.Logf("Running command via SSH; remote: %s, command: %s", s.client.RemoteAddr(), cmd)

	if s.client == nil {
		return nil, fmt.Errorf("s.client is nil")
	}

	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}

	combinedOutput, err := session.CombinedOutput(cmd)

	e2eframework.Logf("Ran command via SSH; remote: %s, command: %s, output: %s, error: %v", s.client.RemoteAddr(), cmd, string(combinedOutput), err)

	return combinedOutput, err
}

func (s *sshCommandRunnerImpl) Close() error {
	err := s.client.Close()
	if err != nil {
		return err
	}

	return nil
}

func GetGateway() (*Gateway, error) {
	httpProxy := os.Getenv("HTTP_PROXY" /*consts.HttpProxyEnv*/)
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
		Port:        22, // consts.DefaultSSHPort,
		AuthMethods: []ssh.AuthMethod{ssh.Password(testbed.GatewayPassword)},
	}

	return gw, nil
}
