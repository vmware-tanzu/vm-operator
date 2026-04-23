// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kubectl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	expect "github.com/google/goexpect"
)

const (
	kubectlCommand = "kubectl"
	pluginCommand  = "kubectl-vsphere"
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

var passwordPromptRE = regexp.MustCompile("Password:")

// noopWriteCloser is a WriteCloser that does nothing when Close is called, so that goexpect doesn't cause a panic.
type noopWriteCloser struct {
	io.Writer
}

func (*noopWriteCloser) Close() error {
	return nil
}

// Login runs the kubectl-vsphere plugin to login to the supervisor cluster that the plugin
// is configured with.
func (k *KubectlPlugin) Login() error {
	kubectlPath, err := checkPluginExistsInPath()
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

	kcmd, errch, err := expect.Spawn("kubectl-vsphere "+strings.Join(args, " "), 2*time.Minute, //nolint:mnd
		expect.Tee(&noopWriteCloser{os.Stdout}),
		expect.SetEnv([]string{
			"KUBECONFIG=" + k.kubeconfigPath,
			"HTTP_PROXY=" + os.Getenv("HTTP_PROXY"),
			"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"),
			"PATH=" + filepath.Dir(kubectlPath),
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to spawn kubectl-vsphere: %w", err)
	}

	defer func() { _ = kcmd.Close() }()

	if out, _, err := kcmd.Expect(passwordPromptRE, -1); err != nil {
		return fmt.Errorf("failed to get password prompt: %w, output: %s", err, out)
	}

	if err := kcmd.Send(k.password + "\n"); err != nil {
		return fmt.Errorf("failed to send password: %+w", err)
	}

	if err := <-errch; err != nil {
		return fmt.Errorf("failed to login: %+w", err)
	}

	return nil
}

// Helper method that looks for the kubectl and kubectl-vsphere binaries in the user's
// PATH, and returns the paths to each, or an error if one of them is not found.
func checkPluginExistsInPath() (string, error) {
	kubectlPath, err := exec.LookPath(kubectlCommand)
	if err != nil {
		return "", fmt.Errorf("failed to find '%s' in PATH: %+w", kubectlCommand, err)
	}

	_, err = exec.LookPath(pluginCommand)
	if err != nil {
		return "", fmt.Errorf("failed to find '%s' in PATH: %+w", pluginCommand, err)
	}

	return kubectlPath, nil
}

// Logout removes the WCP enabled clusters and associated credentials from the plugin's kubeconfig file.
func (k *KubectlPlugin) Logout() error {
	_, err := checkPluginExistsInPath()
	if err != nil {
		return fmt.Errorf("failed to find kubectl or kubectl-vsphere in PATH: %+w", err)
	}

	args := []string{"logout"}
	cmd := exec.Command("kubectl-vsphere", args...)

	return cmd.Run()
}

// KubeconfigPath returns the path to the kubeconfig file that the plugin is configured with.
func (k *KubectlPlugin) KubeconfigPath() string {
	return k.kubeconfigPath
}
