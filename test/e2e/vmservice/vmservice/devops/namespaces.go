// Copyright (c) 2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package devops

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	libssh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type DevOpsSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}

const (
	devopsSpecName        = "devops-namespaces"
	randomSSOUserName     = "devops-sso-user"
	randomSSOUserPassword = "Password!23"
)

func DevOpsSpec(ctx context.Context, inputGetter func() DevOpsSpecInput) {
	var (
		input                       DevOpsSpecInput
		config                      *e2eConfig.E2EConfig
		wcpClient                   wcp.WorkloadManagementAPI
		k8sClient                   ctrlclient.Client
		kubeconfigPath              string
		namespacedVMClassFSSEnabled bool
		vmImageRegistryEnabled      bool
		namespace                   string
		sshCommandRunner            libssh.SSHCommandRunner
		supervisorClusterIP         string
		vCenterAdminCreds           dcli.VCenterUserCredentials
		user                        *vcenter.User
		userKubeconfigPath          string
	)

	BeforeEach(func() {
		By("Set up infrastructure related configs")

		input = inputGetter()
		Expect(input.Config).NotTo(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", devopsSpecName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", devopsSpecName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		Expect(input.ClusterProxy).NotTo(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", devopsSpecName)
		Expect(input.WCPClient).NotTo(BeNil(), "Invalid argument. input.WCPClient can't be nil when calling %s spec", devopsSpecName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", devopsSpecName)
		config = input.Config
		wcpClient = input.WCPClient
		namespace = input.WCPNamespaceName
		k8sClient = input.ClusterProxy.GetClient()
		kubeconfigPath = input.ClusterProxy.GetKubeconfigPath()
		vCenterAdminCreds = dcli.VCenterUserCredentials{Username: testbed.AdminUsername, Password: testbed.AdminPassword}
		namespacedVMClassFSSEnabled = utils.IsFssEnabled(ctx, k8sClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSNamespacedVMClass"))
		vmImageRegistryEnabled = utils.IsFssEnabled(ctx, k8sClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMImageRegistry"))
		// Create SSO user
		sshCommandRunner, _, supervisorClusterIP = testutils.GetHelpersFromKubeconfig(ctx, kubeconfigPath)
		user = vcenter.NewUser(randomSSOUserName, randomSSOUserPassword).WithAdminCreds(vCenterAdminCreds).WithSSHCommandRunner(sshCommandRunner)
		kubectlPlugin := testutils.CreateUserAndLogin(user, supervisorClusterIP, "", "")
		userKubeconfigPath = kubectlPlugin.KubeconfigPath()
	})

	Context("When Devops have view permission to namespace", func() {
		BeforeEach(func() {
			// Grant SSO user with view only access to the supervisor namespace.
			testutils.SetUserPermissionsOnNamespace(wcpClient, user, wcp.ViewAccessType, namespace)
		})

		AfterEach(func() {
			// Remove the user's permissions on the namespace.
			err := wcpClient.RemoveNamespacePermissions(wcp.Principal{Type: wcp.UserSubjectType, Name: user.Credentials.Username, Domain: "vsphere.local"}, namespace)
			Expect(err).NotTo(HaveOccurred())
			// Delete the SSO user.
			vcenter.DeleteUserOrFail(user)
		})

		It("Should have correct view permission", Label("smoke"), func() {
			By("able to get resources within assigned namespace")

			if namespacedVMClassFSSEnabled {
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineclass", "-n", namespace)
			} else {
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineclassbinding", "-n", namespace)
			}

			if vmImageRegistryEnabled {
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "clustervirtualmachineimage")
			}

			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineimage", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachine", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineservices", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachinepublishrequests", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "webconsolerequests", "-n", namespace)

			By("not able to get resources outside assigned namespace")

			if namespacedVMClassFSSEnabled {
				kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachineclass", "-A")
			} else {
				// VM Class is cluster scoped resource when WCP_Namespaced_VM_Class disabled
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineclass")
				kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachineclassbinding", "-A")
			}

			if vmImageRegistryEnabled {
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "clustervirtualmachineimage")
				kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachineimage", "-A")
			} else {
				// VM Image is cluster scoped resource when WCP_VM_Image_Registry disabled
				kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "get", "virtualmachineimage")
			}

			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachine", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachineservices", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "virtualmachinepublishrequests", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "get", "webconsolerequests", "-A")
		})
	})

	Context("When Devops have edit permission to namespace", func() {
		BeforeEach(func() {
			// Grant SSO user with edit access to the supervisor namespace.
			testutils.SetUserPermissionsOnNamespace(wcpClient, user, wcp.EditAccessType, namespace)
		})

		AfterEach(func() {
			// Remove the user's permissions on the namespace.
			err := wcpClient.RemoveNamespacePermissions(wcp.Principal{Type: wcp.UserSubjectType, Name: user.Credentials.Username, Domain: "vsphere.local"}, namespace)
			Expect(err).NotTo(HaveOccurred())
			// Delete the SSO user.
			vcenter.DeleteUserOrFail(user)
		})

		It("Should have correct edit permission ", func() {
			By("able to create within assigned namespace")
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "create", "virtualmachine", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "create", "virtualmachineservices", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "create", "virtualmachinepublishrequests", "-n", namespace)
			kubectl.AssertKubectlUserCan(ctx, userKubeconfigPath, "create", "webconsolerequests", "-n", namespace)

			By("not able to create outside assigned namespace")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "create", "virtualmachine", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "create", "virtualmachineservices", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "create", "virtualmachinepublishrequests", "-A")
			kubectl.AssertKubectlUserCannot(ctx, userKubeconfigPath, "create", "webconsolerequests", "-A")
		})
	})
}
