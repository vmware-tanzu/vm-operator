// Copyright (c) 2023-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vmWebConsoleSpecName = "vm-web-console"
)

type VMWebConsoleRequestSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	LinuxVMName      string
	WCPNamespaceName string
}

func VMWebConsoleRequestSpec(ctx context.Context, inputGetter func() VMWebConsoleRequestSpecInput) {
	var (
		input            VMWebConsoleRequestSpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		webconsoleName   string
		resourceName     string
		webconsoleParams manifestbuilders.VirtualMachineWebConsoleRequestYaml
	)

	BeforeEach(func() {
		// Set up infrastructure related configs.
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", vmWebConsoleSpecName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", vmWebConsoleSpecName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", vmWebConsoleSpecName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", vmWebConsoleSpecName)
		Expect(input.LinuxVMName).ToNot(BeEmpty(), "Invalid argument. input.LinuxVMName can't be empty when calling %s spec", vmWebConsoleSpecName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", vmWebConsoleSpecName)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, vmWebConsoleSpecName))
		DeferCleanup(cancelPodWatches)

		webconsoleName = fmt.Sprintf("%s-%s", vmWebConsoleSpecName, capiutil.RandomString(4))
		webconsoleParams = manifestbuilders.VirtualMachineWebConsoleRequestYaml{
			Namespace: input.WCPNamespaceName,
			Name:      webconsoleName,
			VMName:    input.LinuxVMName,
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, webconsoleName, resourceName)
		}
	})

	Context("Create web console request CR", func() {
		It("should successfully create v1a1 webconsolerequests and populate status", Label("smoke"), func() {
			resourceName = "webconsolerequests"
			webconsoleYaml := manifestbuilders.GetV1A1WebConsoleRequestYaml(webconsoleParams)
			e2eframework.Logf("%v", string(webconsoleYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, webconsoleYaml)).NotTo(HaveOccurred(), "failed to create v1a1 webconsole request", string(webconsoleYaml))
			// Verify webconsole request creation.
			vmservice.VerifyWebConsoleRequestCreation(ctx, config, svClusterClient, input.WCPNamespaceName, webconsoleName)
			vmoperator.VerifyWebConsoleRequestStatus(ctx, config, svClusterClient, input.WCPNamespaceName, webconsoleName)
		})

		It("When WCP_VMService_v1alpha2 enabled, should successfully create v1a2 virtualmachinewebconsolerequests and populate status", func() {
			// Skip if WCP_VMService_v1alpha2 FSS not enabled
			skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)

			resourceName = "virtualmachinewebconsolerequests"
			vmWebconsoleYaml := manifestbuilders.GetVirtualMachineWebConsoleRequestYaml(webconsoleParams)
			e2eframework.Logf("%v", string(vmWebconsoleYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmWebconsoleYaml)).NotTo(HaveOccurred(), "failed to create v1a2 virtualmachinewebconsole request", string(vmWebconsoleYaml))
			// Verify virtualmachine webconsole request creation.
			vmservice.VerifyVirtualMachineWebConsoleRequestCreation(ctx, config, svClusterClient, input.WCPNamespaceName, webconsoleName)
			vmoperator.VerifyVirtualMachineWebConsoleRequestStatus(ctx, config, svClusterClient, input.WCPNamespaceName, webconsoleName)
		})
	})
}
