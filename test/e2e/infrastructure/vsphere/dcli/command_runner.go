// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package dcli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

const (
	retryInterval = 5 * time.Second
	retryTimeout  = 30 * time.Second
)

// transientErrors are substrings in the output of a failed command that
// indicate the command is worth retrying.
var transientErrors = []string{
	"ServiceUnavailable",
	"SERVICE_UNAVAILABLE",
	"InternalServerError",
}

// DCLICommandRunner knows how to run DCLI commands on a vCenter instance.
type DCLICommandRunner interface {
	// RunDCLICommand runs the DCLI command, retrying transient failures, and
	// returns its combined STDOUT and STDERR.
	RunDCLICommand(string) ([]byte, error)
	// RunCommandAndUnmarshalJSONResult runs the DCLI command and decodes its
	// JSON output into the given destination.
	RunCommandAndUnmarshalJSONResult(string, any) error
}

// VCenterUserCredentials are useful both for VC API and kubectl plugin
// interactions.
type VCenterUserCredentials struct {
	Username string
	Password string
}

type dcliCommandRunnerImpl struct {
	adminCredentials VCenterUserCredentials
	sshHelper        e2essh.SSHCommandRunner
}

// NewDCLICommandRunner returns a runner that executes DCLI commands over SSH on
// the vCenter, authenticating DCLI with the given admin credentials.
func NewDCLICommandRunner(vCenterHostname string, vCenterPort int, adminUsername, adminPassword, sshUser, sshPassword string) (DCLICommandRunner, error) {
	sshHelper, err := e2essh.NewSSHCommandRunner(vCenterHostname, vCenterPort, sshUser, []ssh.AuthMethod{ssh.Password(sshPassword)})
	if err != nil {
		return nil, err
	}

	return &dcliCommandRunnerImpl{
		adminCredentials: VCenterUserCredentials{Username: adminUsername, Password: adminPassword},
		sshHelper:        sshHelper,
	}, nil
}

func (d *dcliCommandRunnerImpl) RunCommandAndUnmarshalJSONResult(cmd string, unmarshalDestination any) error {
	output, err := d.RunDCLICommand(cmd)
	if err != nil {
		return err
	}

	return types.NewJSONDecoder(bytes.NewReader(output)).Decode(unmarshalDestination)
}

// RunDCLICommand runs the command. The SSH runner already logs the (redacted)
// command and its output, so nothing is logged here other than retries.
func (d *dcliCommandRunnerImpl) RunDCLICommand(cmd string) ([]byte, error) {
	cmdWithCreds := addDCLIParameters(cmd, d.adminCredentials)

	var (
		stdout  []byte
		lastErr error
	)

	pollErr := wait.PollUntilContextTimeout(context.Background(), retryInterval, retryTimeout, true, func(context.Context) (bool, error) {
		stdout, lastErr = d.sshHelper.RunCommand(cmdWithCreds)
		if lastErr == nil {
			return true, nil
		}

		if isTransient(lastErr, stdout) {
			e2eframework.Logf("Transient DCLI error, retrying: %v", lastErr)
			return false, nil
		}

		return false, lastErr
	})
	if lastErr != nil {
		// Prefer the command's error over the poll's context deadline.
		return stdout, lastErr
	}

	return stdout, pollErr
}

// isTransient returns true if the command failure is worth retrying.
func isTransient(err error, output []byte) bool {
	msg := err.Error() + string(output)
	for _, s := range transientErrors {
		if strings.Contains(msg, s) {
			return true
		}
	}

	return false
}

func addDCLIParameters(cmd string, creds VCenterUserCredentials) string {
	return fmt.Sprintf("dcli %s +username %s +password %s", cmd, shellQuote(creds.Username), shellQuote(creds.Password))
}

// shellQuote single-quotes s for a POSIX shell, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
