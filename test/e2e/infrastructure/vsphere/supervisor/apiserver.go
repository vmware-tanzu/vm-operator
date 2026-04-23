// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"golang.org/x/crypto/ssh"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
)

const (
	APIServerSSHPort = 22
	APIServerSSHUser = "root"
)

var (
	managementAPIServerIP = os.Getenv("MANAGEMENT_API_SERVER_IP")
)

// GetAPIServerCommandRunner returns SSH command runner configured with the SV cluster API server as root user.
func GetAPIServerCommandRunner(ctx context.Context, path string) (e2essh.SSHCommandRunner, error) {
	apiServerURL := kubectl.GetKubectlClusterForCurrentContext(ctx, path)

	parsedAPIServerURL, err := url.Parse(apiServerURL)
	if err != nil {
		return nil, err
	}

	apiServerIP := parsedAPIServerURL.Hostname()
	if apiServerIP == "" {
		return nil, errors.New("unable to get supervisor cluster API server hostname")
	}

	apiServerCredentials, err := GetControlPlaneVMConnectionDetails(ctx, path, apiServerIP)
	if err != nil {
		return nil, err
	}

	apiServerSSHCommandRunner, err := e2essh.NewSSHCommandRunner(apiServerCredentials.ManagementAPIServerIP, APIServerSSHPort, apiServerCredentials.Username, []ssh.AuthMethod{ssh.Password(apiServerCredentials.Password)})
	if err != nil {
		return nil, err
	}

	return apiServerSSHCommandRunner, nil
}

// ControlPlaneVMConnectionDetails contains connection details to SSH into a control plane VM in the Supervisor Cluster.
// Within the control plane VM, SSO roles can be disregarded to test more granular functionality.
type ControlPlaneVMConnectionDetails struct {

	// ManagementAPIServerIP is the floating IP used internally in a Supervisor Cluster, used by wcpsvc to connect to
	// the kube-apiserver. It can also be used to connect for elevated kubectl privileges, or general SSH access to the
	// control plane VMs.
	ManagementAPIServerIP string

	// Username is the SSH username to connect to the control plane VM.
	Username string

	// Password is the SSH password (in conjunction with the Username) to the control plane VM.
	Password string
}

// GetControlPlaneVMConnectionDetails fetches the internal server IP and root credentials.
func GetControlPlaneVMConnectionDetails(ctx context.Context, path string, apiServerIP string) (ControlPlaneVMConnectionDetails, error) {
	vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, path)
	e2eframework.Logf("VC: %s", vCenterHostname)

	// Correlate the kubeconfig cluster IP with the API server management endpoint, which is what wcpsvc uses internally.
	dcliClient := wcp.NewClientUsingKubeconfig(ctx, path)

	clusters, err := dcliClient.ListClusters()
	if err != nil {
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("error listing clusters: %w", err)
	}

	if managementAPIServerIP == "" {
		for _, cluster := range clusters {
			clusterInfo, err := dcliClient.GetCluster(cluster.MoID)
			if err != nil {
				return ControlPlaneVMConnectionDetails{}, fmt.Errorf("error fetching cluster: %w", err)
			}

			// In most cases, the kubeconfig server IP would be the virtual IP (`clusterInfo.APIServerClusterEndpoint`).
			// However, to run gce2e, some consumers may use a proxy instead, which allows them to connect directly over
			// the floating IP (`clusterInfo.APIServerManagementEndpoint`). This case handles both checks to allow both
			// methods of deployment. Both IPs are unique to a single cluster, so this is a safe check.
			if apiServerIP == clusterInfo.APIServerManagementEndpoint || apiServerIP == clusterInfo.APIServerClusterEndpoint {
				managementAPIServerIP = clusterInfo.APIServerManagementEndpoint
				break
			}
		}
	}

	if managementAPIServerIP == "" {
		e2eframework.Logf("No cluster matching cluster endpoint %s at VC %s", apiServerIP, vCenterHostname)
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("no cluster matching cluster endpoint %s", apiServerIP)
	}

	vcSSHCommandRunner, err := e2essh.NewSSHCommandRunner(vCenterHostname, vcenter.VCSSHPort, testbed.RootUsername, []ssh.AuthMethod{ssh.Password(testbed.RootPassword)})
	if err != nil {
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("error getting ssh runner: %w", err)
	}

	cmd := "python3 /usr/lib/vmware-wcp/decryptK8Pwd.py"

	out, err := vcSSHCommandRunner.RunCommand(cmd)
	if err != nil {
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("error running command over ssh: %w", err)
	}

	re := regexp.MustCompile(fmt.Sprintf("IP:\\s*%s\\nPWD:\\s*(.*)\\n", managementAPIServerIP))

	matches := re.FindStringSubmatch(string(out))
	if matches == nil {
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("unable to find the api server info from output: %v", string(out))
	}

	apiServerPassword := matches[1]

	return ControlPlaneVMConnectionDetails{
		ManagementAPIServerIP: managementAPIServerIP,
		Username:              APIServerSSHUser,
		Password:              apiServerPassword,
	}, nil
}

func GetSvAPIServerAsGateway(ctx context.Context, kubeconfigPath string) (*e2essh.Gateway, error) {
	apiServerURL := kubectl.GetKubectlClusterForCurrentContext(ctx, kubeconfigPath)

	parsedAPIServerURL, err := url.Parse(apiServerURL)
	if err != nil {
		return nil, err
	}

	apiServerIP := parsedAPIServerURL.Hostname()
	if apiServerIP == "" {
		return nil, errors.New("unable to get supervisor cluster API server hostname")
	}

	apiServerCredentials, err := GetControlPlaneVMConnectionDetails(ctx, kubeconfigPath, apiServerIP)
	if err != nil {
		return nil, err
	}

	fmt.Print("The API server pwd is " + apiServerCredentials.Password)

	return &e2essh.Gateway{
		Hostname:    apiServerCredentials.ManagementAPIServerIP,
		Username:    apiServerCredentials.Username,
		Port:        22,
		AuthMethods: []ssh.AuthMethod{ssh.Password(apiServerCredentials.Password)},
	}, nil
}
