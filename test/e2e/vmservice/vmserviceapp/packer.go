// Copyright (c) 2022-2023 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmserviceapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// Package specific constants.
const (
	ubuntuImageName = "ubuntu-2004-cloud-init-21.4-kube-v1.20.10"
	vmSSHUsername   = "packer"
	vmSSHPassword   = "packer"
)

type SpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// Spec specific constants.
const (
	packerSpecName = "vmservice-app-packer"
	// This template file exists in the packer-plugin-vsphere repo.
	// Make sure to update the repo if you are changing this file.
	packerSpecTemplateName = "general-template.pkr.hcl"
	randomSSOUserName      = "packer-plugin-random-user"
	randomSSOUserPassword  = "Password!23"
)

func PackerSpec(ctx context.Context, inputGetter func() SpecInput) {
	var (
		input                  SpecInput
		config                 *e2eConfig.E2EConfig
		wcpClient              wcp.WorkloadManagementAPI
		k8sClient              ctrlclient.Client
		clusterResources       *e2eConfig.Resources
		kubeconfigPath         string
		vmserviceCLID          string
		vmName                 string
		vmiName                string
		cvmiName               string
		gatewayIP              string
		gatewayUsername        string
		gatewayPassword        string
		vmImageRegistryEnabled bool
		templateFilePath       string
		packerCmdOpts          PackerBuildCmdOpts
	)

	BeforeEach(func() {
		By("Set up infrastructure related configs")

		input = inputGetter()
		Expect(input.Config).NotTo(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", packerSpecName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", packerSpecName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).NotTo(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", packerSpecName)
		Expect(input.WCPClient).NotTo(BeNil(), "Invalid argument. input.WCPClient can't be nil when calling %s spec", packerSpecName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", packerSpecName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", packerSpecName)

		config = input.Config
		wcpClient = input.WCPClient
		k8sClient = input.ClusterProxy.GetClient()
		kubeconfigPath = input.ClusterProxy.GetKubeconfigPath()
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources

		vmImageRegistryEnabled = utils.IsFssEnabled(ctx, k8sClient,
			config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"),
			config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMImageRegistry"))

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, packerSpecName))
		DeferCleanup(cancelPodWatches)

		vmName = fmt.Sprintf("source-%s", capiutil.RandomString(4))
		vmiObjName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, k8sClient, input.WCPNamespaceName, ubuntuImageName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VM Image name: %s", ubuntuImageName)

		vmiName = vmiObjName
		vmserviceCLID = vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)

		if vmImageRegistryEnabled {
			By("Attaching content library to cluster with Image-Registry FSS enabled")

			clusterMoID := vcenter.GetClusterMoIDFromKubeconfig(ctx, kubeconfigPath)
			err := wcpClient.AssociateContentLibrariesToCluster(clusterMoID, wcp.ClusterContentLibrarySpec{ContentLibrary: vmserviceCLID})
			Expect(err).NotTo(HaveOccurred(), "failed to attach content library %s to cluster", vmserviceCLID)
			cvmiObjName, err := vmoperator.WaitForClusterVirtualMachineImageName(ctx, &config.Config, k8sClient, ubuntuImageName)
			Expect(err).NotTo(HaveOccurred(), "failed to get the CVMI with display name: %s", ubuntuImageName)

			cvmiName = cvmiObjName
		}

		By("Ensure the gateway VM has TCP forwarding enabled to allow SSH access")

		gatewayIP = os.Getenv("GATEWAY_IP")
		Expect(gatewayIP).NotTo(BeEmpty(), "GATEWAY_IP environment variable is not set")

		gatewayUsername = os.Getenv("GATEWAY_VM_USERNAME")
		Expect(gatewayUsername).NotTo(BeEmpty(), "GATEWAY_VM_USERNAME environment variable is not set")

		gatewayPassword = os.Getenv("GATEWAY_VM_PASSWORD")
		Expect(gatewayPassword).NotTo(BeEmpty(), "GATEWAY_VM_PASSWORD environment variable is not set")
		Expect(VerifyTCPForwarding(gatewayIP, gatewayUsername, gatewayPassword)).To(Succeed())

		By("Ensure the packer template file is available in the packer plugin directory")

		templateFilePath, err = GetTemplatePathInPackerPluginDir(packerSpecTemplateName)
		Expect(err).NotTo(HaveOccurred())
		Expect(templateFilePath).NotTo(BeEmpty())
		// Populate all variables to avoid unset errors in running the packer build command.
		packerCmdOpts = PackerBuildCmdOpts{
			TemplateFilePath: templateFilePath,
			TemplateVariables: map[string]string{
				"keep_input_artifact":   "true",
				"kubeconfig_path":       kubeconfigPath,
				"supervisor_namespace":  input.WCPNamespaceName,
				"class_name":            clusterResources.VMClassName,
				"image_name":            vmiName,
				"source_name":           vmName,
				"storage_class":         clusterResources.StorageClassName,
				"ssh_username":          vmSSHUsername,
				"ssh_password":          vmSSHPassword,
				"ssh_bastion_host":      gatewayIP,
				"ssh_bastion_username":  gatewayUsername,
				"ssh_bastion_password":  gatewayPassword,
				"publish_location_name": "",
				"publish_image_name":    "",
			},
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, k8sClient, kubeconfigPath, input.WCPNamespaceName, vmName, "vm")
		}
	})

	It("Packer command to build and deploy a VM from a view only access SSO user", func() {
		// Create SSO user and associate to the namespace with view only access.
		sshCommandRunner, _, supervisorClusterIP := testutils.GetHelpersFromKubeconfig(ctx, kubeconfigPath)
		vCenterAdminCreds := dcli.VCenterUserCredentials{Username: testbed.AdminUsername, Password: testbed.AdminPassword}
		nonAdminUser := vcenter.NewUser(randomSSOUserName, randomSSOUserPassword).WithAdminCreds(vCenterAdminCreds).WithSSHCommandRunner(sshCommandRunner)
		kubectlPlugin := testutils.CreateUserAndLogin(nonAdminUser, supervisorClusterIP, "", "")
		testutils.SetUserPermissionsOnNamespace(wcpClient, nonAdminUser, wcp.ViewAccessType, input.WCPNamespaceName)

		// Get the SSO user's kubeconfig path and set in the packer command variables.
		kubeconfigAbsPath, err := filepath.Abs(kubectlPlugin.KubeconfigPath())
		Expect(err).ToNot(HaveOccurred())

		packerCmdOpts.TemplateVariables["kubeconfig_path"] = kubeconfigAbsPath
		cmdOut, err := RunPackerBuildCmd(ctx, packerCmdOpts)
		Expect(err).To(HaveOccurred())
		Expect(string(cmdOut)).To(ContainSubstring((" cannot create resource")))

		// Delete the SSO user.
		vcenter.DeleteUserOrFail(nonAdminUser)
	})

	It("Packer command to build and deploy a VM from VMI with valid configs and keep_input_artifact set to true", func() {
		// We run this spec regardless of the Image-Registry FSS state with a VMI (exist in both cases).
		cmdOut, err := RunPackerBuildCmd(ctx, packerCmdOpts)
		Expect(err).ToNot(HaveOccurred(), "failed to run packer build command, output: %s", string(cmdOut))
		Expect(string(cmdOut)).To(ContainSubstring("Build 'vsphere-supervisor' finished successfully"))
		// Expect the source VM exists after the command completes.
		sourceName := packerCmdOpts.TemplateVariables["source_name"]
		vm, err := utils.GetVirtualMachine(ctx, k8sClient, input.WCPNamespaceName, sourceName)
		Expect(err).ToNot(HaveOccurred(), "failed to get the source VM built from Packer")
		Expect(vm).ToNot(BeNil())
	})

	It("Packer command to build, deploy, and publish a VM from CVMI with Image-Registry FSS enabled", func() {
		if !vmImageRegistryEnabled {
			Skip("Skipping this test spec as FSS is disabled")
		}

		// We are switching to CVMI here as VMI is already verified in the above test spec.
		packerCmdOpts.TemplateVariables["image_name"] = cvmiName

		// We have multiple packer command runs in this single It to avoid setting up
		// the similar environment (ns, cl, etc.) from calling the BeforeEach every time.
		// Once we upgrade to ginkgo v2, we can considering having multiple Its with BeforeAll.
		By("Publishing to a non-existing content library", func() {
			packerCmdOpts.TemplateVariables["publish_location_name"] = "non-existing-content-library"
			cmdOut, err := RunPackerBuildCmd(ctx, packerCmdOpts)
			Expect(err).To(HaveOccurred())
			Expect(string(cmdOut)).To(ContainSubstring("not found"), "packer build error: ", err.Error())
		})

		// Create a new content library used for the following packer command runs.
		publishCLName := fmt.Sprintf("%s-%s-%s", packerSpecName, "content-library", capiutil.RandomString(4))
		publishCLID := vmservice.CreateLocalContentLibrary(publishCLName, wcpClient)

		By("Publishing to a non-writable content library", func() {
			Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
				ContentLibrary: publishCLID,
				Writable:       false,
			})).To(Succeed(), "failed to attach a non-writable content library to namespace, CL ID: %s", publishCLID)

			k8sContentLibraryName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, k8sClient, input.WCPNamespaceName, publishCLID)
			Expect(err).NotTo(HaveOccurred())

			packerCmdOpts.TemplateVariables["publish_location_name"] = k8sContentLibraryName
			cmdOut, err := RunPackerBuildCmd(ctx, packerCmdOpts)
			Expect(err).To(HaveOccurred())
			Expect(string(cmdOut)).To(ContainSubstring("not writable"), "packer build error: ", err.Error())

			Expect(wcpClient.DisassociateImageRegistryContentLibrariesFromNamespace(input.WCPNamespaceName, publishCLID)).To(Succeed())
		})

		By("Publishing to a writable content library", func() {
			Expect(wcpClient.AssociateImageRegistryContentLibrariesToNamespace(input.WCPNamespaceName, wcp.ContentLibrarySpec{
				ContentLibrary: publishCLID,
				Writable:       true,
			})).To(Succeed(), "failed to attach a writable content library to namespace, CL ID: %s", publishCLID)

			k8sContentLibraryName, err := vmservice.GetK8sContentLibraryNameByUUID(ctx, config, k8sClient, input.WCPNamespaceName, publishCLID)
			Expect(err).NotTo(HaveOccurred())

			packerCmdOpts.TemplateVariables["publish_location_name"] = k8sContentLibraryName
			publishImageDisplayName := fmt.Sprintf("%s-%s-%s", packerSpecName, "publish-image", capiutil.RandomString(4))
			packerCmdOpts.TemplateVariables["publish_image_name"] = publishImageDisplayName
			cmdOut, err := RunPackerBuildCmd(ctx, packerCmdOpts)
			Expect(err).NotTo(HaveOccurred(), "failed to run packer build command, output: %s", string(cmdOut))

			// Ensure the published image is available with expected display name under the namespace.
			publishedImageCRName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, k8sClient, input.WCPNamespaceName, publishImageDisplayName)
			Expect(err).NotTo(HaveOccurred(), "failed to get the published VM Image by displayed name %s", publishImageDisplayName)
			Expect(publishedImageCRName).NotTo(BeEmpty(), "published VM Image resource name is empty")

			Expect(wcpClient.DisassociateImageRegistryContentLibrariesFromNamespace(input.WCPNamespaceName, publishCLID)).To(Succeed())
		})

		Expect(wcpClient.DeleteLocalContentLibrary(publishCLID)).To(Succeed(), "failed to delete the publish content library, CL ID: %s", publishCLID)
	})
}
