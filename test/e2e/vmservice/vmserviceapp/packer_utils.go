// Copyright (c) 2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmserviceapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

const (
	tcpForwardEnabled  = "\"AllowTcpForwarding yes\""
	tcpForwardDisabled = "\"AllowTcpForwarding no\""
	configCheckCmd     = "grep -Fx %s /etc/ssh/sshd_config"
	configChangeCmd    = "sed -i s/%s/%s/g /etc/ssh/sshd_config"
	restartServiceCmd  = "systemctl restart sshd"

	// Path to the example template files in the packer-plugin-vsphere repo.
	pluginDirTemplateExamplePath = "examples/builder/vsphere-supervisor"
)

// PackerBuildCmdOpts contains the options to run a packer build command.
type PackerBuildCmdOpts struct {
	TemplateFilePath  string
	TemplateVariables map[string]string
}

// RunPackerBuildCmd runs a 'packer build' command with the given options.
func RunPackerBuildCmd(ctx context.Context, opts PackerBuildCmdOpts) ([]byte, error) {
	var repoRoot, pluginDirPath string
	if repoRoot = os.Getenv("REPO_ROOT"); repoRoot == "" {
		return nil, errors.New("REPO_ROOT is required for running packer build command")
	}

	if pluginDirPath = os.Getenv("PACKER_PLUGIN_DIR_PATH"); pluginDirPath == "" {
		return nil, errors.New("PACKER_PLUGIN_DIR_PATH is required for running packer build command")
	}

	cmdArgs := []string{"build"}

	// Add all the template variables to the packer build command.
	for key, val := range opts.TemplateVariables {
		cmdArgs = append(cmdArgs, "-var", fmt.Sprintf("%s=%s", key, val))
	}

	// Add the template file path to the end of packer build command.
	cmdArgs = append(cmdArgs, opts.TemplateFilePath)
	cmd := exec.Command("packer", cmdArgs...)

	// Set the command directory to ensure packer-plugin-vsphere binary is accessible.
	cmd.Dir = pluginDirPath
	e2eframework.Logf("Running command: %s (in %s)", cmd.String(), cmd.Dir)

	return cmd.Output()
}

// GetTemplatePathInPackerPluginDir returns the path to the given template file in PACKER_PLUGIN_DIR_PATH.
func GetTemplatePathInPackerPluginDir(templateName string) (string, error) {
	pluginDirPath := os.Getenv("PACKER_PLUGIN_DIR_PATH")
	if pluginDirPath == "" {
		return "", errors.New("PACKER_PLUGIN_DIR_PATH is empty")
	}

	path := filepath.Join(pluginDirPath, pluginDirTemplateExamplePath, templateName)
	if _, err := os.Stat(path); err == nil { //nolint:gosec // G703: path is built from known template dir
		return path, nil
	} else if os.IsNotExist(err) {
		return "", fmt.Errorf("template file %s does not exist: %w", path, err)
	} else {
		return "", fmt.Errorf("failed to check if template file %s exists: %w", path, err)
	}
}

// VerifyTCPForwarding checks the TCP forwarding config on the given gateway VM.
// If disabled, it will enable it and restart the sshd service to allow Packer access to source VMs.
func VerifyTCPForwarding(gatewayIP, gatewayUsername, gatewayPassword string) error {
	authPassword := []ssh.AuthMethod{ssh.Password(gatewayPassword)}

	gatewayCmdRunner, err := e2essh.NewSSHCommandRunner(gatewayIP, 22, gatewayUsername, authPassword)
	if err != nil {
		return err
	}

	checkDisabledCmd := fmt.Sprintf(configCheckCmd, tcpForwardDisabled)

	disabledOut, err := gatewayCmdRunner.RunCommand(checkDisabledCmd)
	if err == nil && len(disabledOut) != 0 {
		e2eframework.Logf("TCP forwarding is disabled on the gateway VM, enabling it now...")

		enableConfigCmd := fmt.Sprintf(configChangeCmd, tcpForwardDisabled, tcpForwardEnabled)
		if _, err := gatewayCmdRunner.RunCommand(enableConfigCmd); err != nil {
			return fmt.Errorf("failed to enable tcp forwarding config on gateway VM: %w", err)
		}

		if _, err := gatewayCmdRunner.RunCommand(restartServiceCmd); err != nil {
			return fmt.Errorf("failed to restart sshd service on gateway VM: %w", err)
		}

		time.Sleep(10 * time.Second) // time buffer for sshd service fully restarted
	}

	return nil
}
