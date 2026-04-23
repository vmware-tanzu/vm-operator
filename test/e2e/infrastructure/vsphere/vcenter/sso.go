// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

// sso contains helpers to manage users on a vCenter instance.
// This can be useful when testing RBAC, for instance.

const (
	noSuchUserErrorString = "ERROR_NO_SUCH_USER"
)

// Use dir-cli to create commands
// /usr/lib/vmware-vmafd/bin/dir-cli
// Example for user creation
// user create --account <account name>
//--user-password <password>
//--first-name    <first name>
//--last-name     <last name>
// [ --login    <admin user id>            ]
// [ --password <password>                 ]

// User is a basic type used to create vSphere SSO users.
type User struct {
	// Credentials for the user.
	Credentials dcli.VCenterUserCredentials
	// Credentials to be used to create the user.
	adminCreds dcli.VCenterUserCredentials
	cmdRunner  ssh.SSHCommandRunner
}

// Helper method to return the path to the vmware-vmafd binary.
func getDirCLIBinaryPath() string {
	return filepath.Join("/usr", "lib", "vmware-vmafd", "bin", "dir-cli")
}

func addAdminCredentialsToCommand(cmd string, creds dcli.VCenterUserCredentials) string {
	return fmt.Sprintf("%s --login '%s' --password '%s'", cmd, creds.Username, creds.Password)
}

// NewUser creates a basic user that should have the given username and password.
func NewUser(username, password string) *User {
	return &User{Credentials: dcli.VCenterUserCredentials{Username: username, Password: password}}
}

// WithAdminCreds configures the credentials to be used to create this user.
func (u *User) WithAdminCreds(creds dcli.VCenterUserCredentials) *User {
	u.adminCreds = creds
	return u
}

// WithSSHCommandRunner configures the SSH helper to be used to run the commands to create the user.
func (u *User) WithSSHCommandRunner(cmdRunner ssh.SSHCommandRunner) *User {
	u.cmdRunner = cmdRunner
	return u
}

func (u *User) checkIfUserExists() (bool, error) {
	binaryPath := getDirCLIBinaryPath()

	// Check if the user already exists.
	cmd := fmt.Sprintf("%s user find-by-name --account '%s' ", binaryPath, u.Credentials.Username)
	cmd = addAdminCredentialsToCommand(cmd, u.adminCreds)
	fmt.Printf("Running command %s\n", cmd)
	result, err := u.cmdRunner.RunCommand(cmd)
	// This command can fail either due to intermittent issues, or because
	// the user does not exist. Either way, return false, and the
	// underlying error.
	if err != nil {
		// Check if the user didn't exist. dir-cli logs a specific
		// error message when this happens. Look for that string to
		// differentiate between 'something went wrong running the
		// command' and 'the user does not exist'
		if strings.Contains(string(result), noSuchUserErrorString) {
			return false, nil
		}
		// Lookup may have failed intermittently. Return an error.
		fmt.Printf("Command output: %s\n", string(result))

		return false, err
	}
	// At this point, the user must exist. Return true.
	return true, nil
}

// Create runs dir-cli on the VC to create the given user.
//
//	It is idempotent in that if the given user already exists, it will not
//	attempt to create them again (and simply succeed). This allows it to be
//	retried in an Eventually() block safely.
func (u *User) Create() error {
	// TODO You might have a bad time if you run this on Windows.
	// Then again, if you're developing on Windows, maybe you're already having a bad time.
	if exists, err := u.checkIfUserExists(); err == nil && exists {
		// NOOP if the user definitely exists. Otherwise, retry. Worst
		// case, we intermittently failed to check and it'll error out
		// anyway.
		return nil
	}

	binaryPath := getDirCLIBinaryPath()

	cmd := fmt.Sprintf("%s user create --account '%s' --user-password '%s' --first-name '%s First name' --last-name '%s Last name'", binaryPath, u.Credentials.Username, u.Credentials.Password, u.Credentials.Username, u.Credentials.Username)
	cmd = addAdminCredentialsToCommand(cmd, u.adminCreds)
	fmt.Printf("Running command %s\n", cmd)
	result, err := u.cmdRunner.RunCommand(cmd)
	fmt.Printf("Command output: %s\n", string(result))

	return err
}

// user delete --account <account name>
// [ --login    <admin user id>            ]
// [ --password <password>                 ]

// Delete runs dir-cli on the VC to delete the given user.
// This method is idempotent, in that if the user is not found in dir-cli, it
// simply exits as a NOOP without any error.
func (u *User) Delete() error {
	if exists, err := u.checkIfUserExists(); err == nil && !exists {
		// NOOP if the user definitely does not exist. Otherwise, retry. Worst
		// case, we intermittently failed to check and it'll error out
		// anyway.
		return nil
	}

	binaryPath := getDirCLIBinaryPath()
	cmd := fmt.Sprintf("%s user delete --account '%s'", binaryPath, u.Credentials.Username)
	cmd = addAdminCredentialsToCommand(cmd, u.adminCreds)
	_, err := u.cmdRunner.RunCommand(cmd)

	return err
}

// CreateUserOrFail is a convenience method that retries user creation until it
// succeeds or times out.
func CreateUserOrFail(user *User) {
	Eventually(
		// user.Create() returns an error if it failed to create the
		// user, and is meant to be idempotent.
		user.Create, 1*time.Minute, 5*time.Second).Should(Succeed(), "User should have eventually been created", user.Credentials.Username)
}

// DeleteUserOrFail is a convenience method that retries user deletion until it
// succeeds or times out.
func DeleteUserOrFail(user *User) {
	Eventually(
		// user.Delete() returns an error if it failed to delete the
		// user, and is meant to be idempotent.
		user.Delete, 1*time.Minute, 5*time.Second).Should(Succeed(), "User should have eventually been deleted", user.Credentials.Username)
}

// TODO Similar abstractions for groups.
