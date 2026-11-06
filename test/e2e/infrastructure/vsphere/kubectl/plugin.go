// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kubectl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	kubectlCommand = "kubectl"
	pluginCommand  = "kubectl-vsphere"

	// loginTimeout bounds how long the kubectl-vsphere login subprocess may run.
	loginTimeout = 2 * time.Minute
)

// Libraries to run the kubectl plugin against a WCP testbed.
// TODO a helper to download it as part of the framework (eg. BeforeSuite or something) would be nice, to allow us to move
//  away from bash. However, that doesn't buy us much in the short term, so leaving this as a TODO for now.

// KubectlPlugin knows how to run commands using the kubectl-vsphere plugin.
// It assumes that the kubectl-vsphere plugin is in the user's PATH.
type KubectlPlugin struct {
	username                        string
	password                        string
	server                          string
	insecure                        bool
	kubeconfigPath                  string
	tanzuKubernetesClusterName      string
	tanzuKubernetesClusterNamespace string
}

func NewKubectlPlugin(kubeconfigPath string) *KubectlPlugin {
	return &KubectlPlugin{kubeconfigPath: kubeconfigPath}
}

// WithUsername configures the username to use when logging in.
func (k *KubectlPlugin) WithUsername(username string) *KubectlPlugin {
	k.username = username
	return k
}

// WithPassword configures the password to use when logging in.
func (k *KubectlPlugin) WithPassword(password string) *KubectlPlugin {
	k.password = password
	return k
}

// WithServer sets the supervisor cluster to log in to.
func (k *KubectlPlugin) WithServer(server string) *KubectlPlugin {
	k.server = server
	return k
}

// WithInsecureFlag runs the kubectl-vsphere plugin (and thus sets up the kubeconfig contexts)
// to ignore TLS checks.
func (k *KubectlPlugin) WithInsecureFlag(insecure bool) *KubectlPlugin {
	k.insecure = insecure
	return k
}

// WithTanzuKubernetesClusterName sets the name for the TKC to be logged in to.
func (k *KubectlPlugin) WithTanzuKubernetesClusterName(name string) *KubectlPlugin {
	k.tanzuKubernetesClusterName = name
	return k
}

// WithTanzuKubernetesClusterNamespace sets the namespace for the TKC to be logged in to.
func (k *KubectlPlugin) WithTanzuKubernetesClusterNamespace(namespace string) *KubectlPlugin {
	k.tanzuKubernetesClusterNamespace = namespace
	return k
}

// Login runs the kubectl-vsphere plugin to login to the supervisor cluster that the plugin
// is configured with.
func (k *KubectlPlugin) Login() error {
	kubectlPath, pluginPath, err := checkPluginExistsInPath()
	if err != nil {
		return fmt.Errorf("failed to find kubectl or kubectl-vsphere in PATH: %w", err)
	}
	// TODO Support non vsphere.local domains
	vsphereUsername := k.username
	if !strings.HasSuffix(k.username, "@vsphere.local") {
		vsphereUsername = k.username + "@vsphere.local"
	}

	args := []string{"login", "--vsphere-username", vsphereUsername, "--server", k.server}
	if k.insecure {
		args = append(args, "--insecure-skip-tls-verify")
	}

	if k.tanzuKubernetesClusterName != "" {
		args = append(args, "--tanzu-kubernetes-cluster-name", k.tanzuKubernetesClusterName)
	}

	if k.tanzuKubernetesClusterNamespace != "" {
		args = append(args, "--tanzu-kubernetes-cluster-namespace", k.tanzuKubernetesClusterNamespace)
	}

	// Build the child environment explicitly so every variable the plugin
	// needs is listed here. NO_PROXY / no_proxy must be forwarded alongside
	// the proxy vars so that kubectl-vsphere bypasses the proxy for the
	// supervisor cluster IP, which is a direct internal address (not routed
	// via squid). KUBECTL_VSPHERE_PASSWORD lets the plugin read the password
	// non-interactively instead of prompting on a TTY.
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}

	ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pluginPath, args...)
	cmd.Env = []string{
		"KUBECONFIG=" + k.kubeconfigPath,
		"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
		"NO_PROXY=" + noProxy,
		"no_proxy=" + noProxy,
		"PATH=" + filepath.Dir(kubectlPath),
		"KUBECTL_VSPHERE_PASSWORD=" + k.password,
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to login: %w, output: %s", err, out.String())
	}

	return nil
}

// Helper method that looks for the kubectl and kubectl-vsphere binaries in the user's
// PATH, and returns the paths to each, or an error if one of them is not found.
func checkPluginExistsInPath() (string, string, error) {
	kubectlPath, err := exec.LookPath(kubectlCommand)
	if err != nil {
		return "", "", fmt.Errorf("failed to find '%s' in PATH: %+w", kubectlCommand, err)
	}

	pluginPath, err := exec.LookPath(pluginCommand)
	if err != nil {
		return "", "", fmt.Errorf("failed to find '%s' in PATH: %+w", pluginCommand, err)
	}

	return kubectlPath, pluginPath, nil
}

// Logout removes the WCP enabled clusters and associated credentials from the plugin's kubeconfig file.
func (k *KubectlPlugin) Logout() error {
	_, pluginPath, err := checkPluginExistsInPath()
	if err != nil {
		return fmt.Errorf("failed to find kubectl or kubectl-vsphere in PATH: %+w", err)
	}

	args := []string{"logout"}
	cmd := exec.Command(pluginPath, args...)

	return cmd.Run()
}

// KubeconfigPath returns the path to the kubeconfig file that the plugin is configured with.
func (k *KubectlPlugin) KubeconfigPath() string {
	return k.kubeconfigPath
}
