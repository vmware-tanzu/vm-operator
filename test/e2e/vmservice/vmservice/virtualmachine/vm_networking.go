// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/test/e2e/appple2e/util"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VMNetworkSpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

func VMNetworkSpec(ctx context.Context, inputGetter func() VMNetworkSpecInput) {
	const (
		specName           = "vm-networking"
		mutableNetworksCap = "supports_VM_service_mutable_networks"
	)

	var (
		input            VMNetworkSpecInput
		wcpClient        wcp.WorkloadManagementAPI
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources
		tmpNamespaceCtx  wcpframework.NamespaceContext
		vmYaml           []byte
		vmName           string

		isVMMutableNetworksCapEnabled bool
		linuxImageDisplayName         string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		svClusterProxy := input.ClusterProxy
		wcpClient = input.WCPClient
		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		vmYaml = nil
		tmpNamespaceCtx = wcpframework.NamespaceContext{}
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))

		sshCommandRunner, _ := e2essh.NewSSHCommandRunner(
			vcenter.GetVCPNIDFromKubeconfigFile(ctx, svClusterProxy.GetKubeconfigPath()),
			vcenter.VCSSHPort, testbed.RootUsername, []ssh.AuthMethod{ssh.Password(testbed.RootPassword)})
		isAsyncSvUpgradeEnabled, _ := util.IsFSSEnabled(sshCommandRunner, utils.SupervisorAsyncUpgradeFSS)
		isVMMutableNetworksCapEnabled = utils.IsSupervisorCapabilityEnabled(ctx,
			svClusterProxy.GetClientSet(), svClusterProxy.GetDynamicClient(), mutableNetworksCap, isAsyncSvUpgradeEnabled)
	})

	AfterEach(func() {
		vmNamespaceName := input.WCPNamespaceName
		if tmpNamespaceCtx.GetNamespace() != nil {
			vmNamespaceName = tmpNamespaceCtx.GetNamespace().Name
		}

		if CurrentGinkgoTestDescription().Failed {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), vmNamespaceName, vmName, "vm")
		}

		// Delete the virtual machine if it was created.
		if len(vmYaml) > 0 {
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			// Verify that virtual machine does not exist.
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmNamespaceName, vmName)
		}

		// Delete the temporary namespace if it was created.
		if tmpNamespaceCtx.GetNamespace() != nil {
			clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
			wcp.WaitForNamespaceDeleted(wcpClient, tmpNamespaceCtx.GetNamespace().Name)
		}
	})

	It("Should allow network interface to be added to VirtualMachine when mutability cap is enabled", Label("smoke"), func() {
		if !isVMMutableNetworksCapEnabled {
			Skip("VM Mutable Networks capability is not enabled")
		}

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOff",
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoID := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}

		vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		propCollector := property.DefaultCollector(vCenterClient)

		var vmMO mo.VirtualMachine
		Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
		ethCards := object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*types.VirtualEthernetCard)(nil))
		Expect(ethCards).To(HaveLen(1), "VM config should have one EthernetCard")

		By("Add second network interface to VM Spec")

		key := ctrlclient.ObjectKey{Name: vmName, Namespace: input.WCPNamespaceName}
		Eventually(func() bool {
			vm := &vmopv1a3.VirtualMachine{}

			err := svClusterClient.Get(ctx, key, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			Expect(vm.Spec.Network).ToNot(BeNil())
			Expect(vm.Spec.Network.Interfaces).To(HaveLen(1))
			vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, vm.Spec.Network.Interfaces[0])

			vm.Spec.Network.Interfaces[1].Name = "eth1"

			err = svClusterClient.Update(ctx, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			return true
		}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(BeTrue(), "Timed out updating VirtualMachine %s to add second network interface", vmName)

		By("Wait for VM to be reconfigured with second EthernetCard")
		Eventually(func(g Gomega) {
			g.Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
			ethCards := object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*types.VirtualEthernetCard)(nil))
			g.Expect(ethCards).To(HaveLen(2), "VM should have two EthernetCards configured")
		}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(), "VM reconfigured with second EthernetCard")

		By("Power on VM")
		Eventually(func() bool {
			vm := &vmopv1a3.VirtualMachine{}

			err := svClusterClient.Get(ctx, key, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			vm.Spec.PowerState = vmopv1a3.VirtualMachinePowerStateOn

			err = svClusterClient.Update(ctx, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			return true
		}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(BeTrue(), "Timed out updating VirtualMachine %s PowerState to On", vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
		vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		By("Powered On VM should still have two EthernetCards configured")
		Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
		ethCards = object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*types.VirtualEthernetCard)(nil))
		Expect(ethCards).To(HaveLen(2), "VM config should have two EthernetCards")
	})
}
