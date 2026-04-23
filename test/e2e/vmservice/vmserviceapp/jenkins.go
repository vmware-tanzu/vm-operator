// Copyright (c) 2023 Broadcom. All Rights Reserved.
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
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
)

const (
	jenkinsSpecName = "vmservice-app-jenkins"
	// This template file exists in the packer-plugin-vsphere repo.
	// Make sure to update the repo if you are changing this file.
	jenkinsSpecTemplateName = "jenkins-template.pkr.hcl"
)

func JenkinsSpec(ctx context.Context, inputGetter func() SpecInput) {
	var (
		input            SpecInput
		config           *e2eConfig.E2EConfig
		k8sClient        ctrlclient.Client
		clusterResources *e2eConfig.Resources
		kubeconfigPath   string
		vmName           string
		ubuntuVMIName    string
		gatewayIP        string
		gatewayUsername  string
		gatewayPassword  string
		templateFilePath string
	)

	BeforeEach(func() {
		By("Set up infrastructure related configs")

		input = inputGetter()
		Expect(input.Config).NotTo(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", jenkinsSpecName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", jenkinsSpecName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).NotTo(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", jenkinsSpecName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", jenkinsSpecName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", jenkinsSpecName)

		config = input.Config
		k8sClient = input.ClusterProxy.GetClient()
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		kubeconfigPath = input.ClusterProxy.GetKubeconfigPath()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, jenkinsSpecName))
		DeferCleanup(cancelPodWatches)

		vmName = fmt.Sprintf("source-%s", capiutil.RandomString(4))
		vmiName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, k8sClient, input.WCPNamespaceName, ubuntuImageName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VM Image name: %s", ubuntuImageName)

		ubuntuVMIName = vmiName

		By("Ensure the gateway VM has TCP forwarding enabled to allow SSH access")

		gatewayIP = os.Getenv("GATEWAY_IP")
		Expect(gatewayIP).NotTo(BeEmpty(), "GATEWAY_IP environment variable is not set")

		gatewayUsername = os.Getenv("GATEWAY_VM_USERNAME")
		Expect(gatewayUsername).NotTo(BeEmpty(), "GATEWAY_VM_USERNAME environment variable is not set")

		gatewayPassword = os.Getenv("GATEWAY_VM_PASSWORD")
		Expect(gatewayPassword).NotTo(BeEmpty(), "GATEWAY_VM_PASSWORD environment variable is not set")
		Expect(VerifyTCPForwarding(gatewayIP, gatewayUsername, gatewayPassword)).To(Succeed())

		By("Ensure the Jenkins template file is available in the packer plugin directory")

		templateFilePath, err = GetTemplatePathInPackerPluginDir(jenkinsSpecTemplateName)
		Expect(err).NotTo(HaveOccurred())
		Expect(templateFilePath).NotTo(BeEmpty())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, k8sClient, kubeconfigPath, input.WCPNamespaceName, vmName, "vm")
		}
	})

	It("Should successfully run the Jenkins workload using Packer", func() {
		opts := PackerBuildCmdOpts{
			TemplateFilePath: templateFilePath,
			TemplateVariables: map[string]string{
				"keep_input_artifact":  "true",
				"kubeconfig_path":      kubeconfigPath,
				"supervisor_namespace": input.WCPNamespaceName,
				"class_name":           clusterResources.VMClassName,
				"image_name":           ubuntuVMIName,
				"source_name":          vmName,
				"storage_class":        clusterResources.StorageClassName,
				"ssh_username":         vmSSHUsername,
				"ssh_password":         vmSSHPassword,
				"ssh_bastion_host":     gatewayIP,
				"ssh_bastion_username": gatewayUsername,
				"ssh_bastion_password": gatewayPassword,
			},
		}

		cmdOut, err := RunPackerBuildCmd(ctx, opts)
		Expect(err).NotTo(HaveOccurred(), "failed to run the packer build command, output: %s", string(cmdOut))
	})
}
