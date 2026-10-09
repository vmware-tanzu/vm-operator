package testutils

import (
	"context"
	"fmt"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	libssh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
)

func GetHelpersFromKubeconfig(ctx context.Context, kubeconfigPath string) (libssh.SSHCommandRunner, wcp.WorkloadManagementAPI, string) {
	vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, kubeconfigPath)
	Expect(vCenterHostname).NotTo(Equal(""), "Unable to determine VC PNID")

	var err error
	// This is the port to SSH into the VC with.
	vCenterPort := 22
	wcpClient := wcp.NewClientUsingKubeconfigFile(ctx, kubeconfigPath)

	supervisorClusterIPRaw := kubectl.GetKubectlClusterForCurrentContext(ctx, kubeconfigPath)
	parsedSupervisorURL, err := url.Parse(supervisorClusterIPRaw)
	Expect(err).NotTo(HaveOccurred(), "failed to parse supervisor cluster ip due to %v", err)

	supervisorClusterIP := parsedSupervisorURL.Hostname()
	Expect(supervisorClusterIP).NotTo(Equal(""), "Unable to get supervisor cluster host")

	sshCommandRunner, err := libssh.NewSSHCommandRunner(vCenterHostname, vCenterPort, testbed.RootUsername, []ssh.AuthMethod{ssh.Password(testbed.RootPassword)})
	Expect(err).NotTo(HaveOccurred())

	return sshCommandRunner, wcpClient, supervisorClusterIP
}

// Helper method to create a user on the VC and login, returning a kubectl plugin handle with a reference the their kubeconfig.
func CreateUserAndLogin(user *vcenter.User, supervisorClusterIP, tanzuKubernetesClusterForTest string, tanzuKubernetesClusterNamespaceForTest string) *kubectl.KubectlPlugin {
	By("creating a new user")
	vcenter.CreateUserOrFail(user)

	return LoginWithUserWithRetry(user, supervisorClusterIP, tanzuKubernetesClusterForTest, tanzuKubernetesClusterNamespaceForTest)
}

// Helper method to login with the given user on the VC and login, returning a kubectl plugin handle with a reference the their kubeconfig.
func LoginWithUserWithRetry(user *vcenter.User, supervisorClusterIP, tanzuKubernetesClusterForTest string, tanzuKubernetesClusterNamespaceForTest string) *kubectl.KubectlPlugin {
	By("logging in as the user")

	kubectlPlugin := kubectl.NewKubectlPlugin(fmt.Sprintf("%s-kubeconfig", user.Credentials.Username)).
		WithUsername(user.Credentials.Username).
		WithPassword(user.Credentials.Password).
		WithServer(supervisorClusterIP).
		WithInsecureFlag(true)

	if tanzuKubernetesClusterForTest != "" {
		kubectlPlugin = kubectlPlugin.WithTanzuKubernetesClusterName(tanzuKubernetesClusterForTest)
	}

	if tanzuKubernetesClusterNamespaceForTest != "" {
		kubectlPlugin = kubectlPlugin.WithTanzuKubernetesClusterNamespace(tanzuKubernetesClusterNamespaceForTest)
	}

	Eventually(func() error {
		return kubectlPlugin.Login()
	}, 5*time.Minute, 10*time.Second).Should(Succeed(), "time out while trying to vsphere login as %s", user.Credentials.Username)

	return kubectlPlugin
}

func SetUserPermissionsOnNamespace(wcpClient wcp.WorkloadManagementAPI, user *vcenter.User, accessType wcp.AccessType, namespaceForTest string) {
	By("granting the new user permissions in the supervisor namespace")
	Expect(wcpClient.CreateNamespacePermissions(
		wcp.Principal{Type: wcp.UserSubjectType,
			Name:   user.Credentials.Username,
			Domain: "vsphere.local"},
		namespaceForTest,
		accessType,
	)).To(Succeed())
}
