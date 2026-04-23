// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
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

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

// DCLICommandRunner knows how to run DCLI commands on a vCenter instance.
type DCLICommandRunner interface {
	RunDCLICommand(string) ([]byte, error)
	RunCommandAndUnmarshalJSONResult(string, any) error
}

// These credentials are useful both for VC API and kubectl plugin interactions.
type VCenterUserCredentials struct {
	Username string
	Password string
}

type dcliCommandRunnerImpl struct {
	vCenterHostname  string
	vCenterPort      int
	adminCredentials VCenterUserCredentials
	sshHelper        e2essh.SSHCommandRunner
}

func NewDCLICommandRunner(vCenterHostname string, vCenterPort int, adminUsername, adminPassword, sshUser, sshPassword string) (DCLICommandRunner, error) {
	return newDCLICommandRunnerImpl(vCenterHostname, vCenterPort, adminUsername, adminPassword, sshUser, sshPassword)
}

func newDCLICommandRunnerImpl(vCenterHostname string, vCenterPort int, adminUsername, adminPassword, sshUser, sshPassword string) (*dcliCommandRunnerImpl, error) {
	newSSHCommandRunner, err := e2essh.NewSSHCommandRunner(vCenterHostname, vCenterPort, sshUser, []ssh.AuthMethod{ssh.Password(sshPassword)})
	if err != nil {
		return nil, err
	}

	retVal := &dcliCommandRunnerImpl{
		vCenterHostname:  vCenterHostname,
		vCenterPort:      vCenterPort,
		adminCredentials: VCenterUserCredentials{Username: adminUsername, Password: adminPassword},
		sshHelper:        newSSHCommandRunner,
	}

	return retVal, nil
}

func (d *dcliCommandRunnerImpl) RunCommandAndUnmarshalJSONResult(cmd string, unmarshalDestination any) error {
	output, err := d.RunDCLICommand(cmd)
	if err != nil {
		return err
	}

	dec := types.NewJSONDecoder(bytes.NewReader(output))

	return dec.Decode(unmarshalDestination)
}

func (d *dcliCommandRunnerImpl) RunDCLICommand(cmd string) ([]byte, error) {
	cmdWithCreds := addDCLIParameters(cmd, d.adminCredentials.Username, d.adminCredentials.Password)
	fmt.Printf("\nRunning command: %s", cmdWithCreds)

	var (
		stdout  []byte
		lastErr error
	)

	// Retry on transient errors like SERVICE_UNAVAILABLE
	pollErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		var err error

		stdout, err = d.sshHelper.RunCommand(cmdWithCreds)
		fmt.Printf("\nSTDOUT: %s", string(stdout))

		if err == nil {
			return true, nil
		}

		lastErr = err

		// Check for transient errors that should be retried
		errStr := err.Error() + string(stdout)
		if strings.Contains(errStr, "ServiceUnavailable") || strings.Contains(errStr, "SERVICE_UNAVAILABLE") {
			fmt.Printf("\nTransient error detected, retrying...")
			return false, nil // Continue polling
		}

		// Non-transient error, stop polling
		return false, err
	})
	if pollErr != nil {
		if lastErr != nil {
			return stdout, lastErr
		}

		return stdout, pollErr
	}

	return stdout, nil
}

func addDCLIParameters(cmd, username, password string) string {
	return fmt.Sprintf("dcli %s +username '%s' +password '%s'", cmd, username, password)
}
