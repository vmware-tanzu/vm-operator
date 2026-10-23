// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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
		specName = "vm-networking"
	)

	var (
		input            VMNetworkSpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources
		vmName           string

		linuxImageDisplayName string
		linuxVMIName          string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))

		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)
	})

	DescribeTable("Should configure CloudInit DHCP overrides", Label("extended-functional", "experimental"),
		func(interfaceDNS bool) {
			vm, err := manifestbuilders.VirtualMachineA6(manifestbuilders.VirtualMachineYaml{
				Namespace: input.WCPNamespaceName, Name: vmName, ImageName: linuxVMIName,
				VMClassName: clusterResources.VMClassName, StorageClassName: clusterResources.StorageClassName,
				PowerState: string(vmopv1.VirtualMachinePowerStateOn),
			})
			Expect(err).ToNot(HaveOccurred())
			vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
			}
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Nameservers: []string{"192.0.2.53"},
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{
					Name: "eth0", DHCP4: ptr.To(true), DHCP6: ptr.To(false), Gateway4: "None",
					Routes: []vmopv1.VirtualMachineNetworkRouteSpec{{To: "198.51.100.0/24", Via: "192.0.2.1"}},
				}},
			}
			wantDNS := vm.Spec.Network.Nameservers
			if interfaceDNS {
				wantDNS = []string{"192.0.2.54"}
				vm.Spec.Network.Interfaces[0].Nameservers = wantDNS
			}

			By("Rejecting mismatched dual-stack overrides at admission")
			invalidVM := vm.DeepCopy()
			invalidVM.Spec.Network.Interfaces[0].DHCP6 = ptr.To(true)
			err = svClusterClient.Create(ctx, invalidVM, ctrlclient.DryRunAll)
			Expect(err).To(MatchError(ContainSubstring("dual-stack DHCP requires identical DHCP overrides")))

			By("Creating a DHCP VM with explicit DNS and route suppression")
			vmYaml := manifestbuilders.ToYAML(vm)
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed())
			vmoperator.DeferCleanupVirtualMachine(
				config, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, false)
			vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmMoRef := types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName),
			}
			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			propCollector := property.DefaultCollector(vCenterClient)

			By("Verifying the CloudInit network metadata sent to the guest")
			Eventually(func(g Gomega) {
				var vmMO mo.VirtualMachine
				g.Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
				g.Expect(vmMO.Config).ToNot(BeNil())
				var encodedMetadata string
				for _, value := range vmMO.Config.ExtraConfig {
					if option := value.GetOptionValue(); option.Key == constants.CloudInitGuestInfoMetadata {
						encodedMetadata, _ = option.Value.(string)
					}
				}
				g.Expect(encodedMetadata).ToNot(BeEmpty())
				data, err := pkgutil.TryToDecodeBase64Gzip([]byte(encodedMetadata))
				g.Expect(err).ToNot(HaveOccurred())
				var metadata struct {
					Network netplan.Network `json:"network"`
				}
				g.Expect(yaml.Unmarshal([]byte(data), &metadata)).To(Succeed())
				g.Expect(metadata.Network.Ethernets).To(HaveKey("eth0"))
				eth := metadata.Network.Ethernets["eth0"]
				g.Expect(eth.Dhcp4).To(HaveValue(BeTrue()))
				g.Expect(eth.Dhcp4Overrides).ToNot(BeNil())
				g.Expect(eth.Dhcp4Overrides.UseDNS).To(HaveValue(BeFalse()))
				g.Expect(eth.Dhcp4Overrides.UseRoutes).To(HaveValue(BeFalse()))
				g.Expect(eth.Dhcp6Overrides).To(BeNil())
				g.Expect(eth.Nameservers).ToNot(BeNil())
				g.Expect(eth.Nameservers.Addresses).To(Equal(wantDNS))
				g.Expect(eth.Gateway4).To(BeNil())
				g.Expect(eth.Routes).To(HaveLen(1))
				g.Expect(eth.Routes[0].To).To(HaveValue(Equal("198.51.100.0/24")))
			}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed())
		},
		Entry("global nameservers", false),
		Entry("interface nameservers take precedence", true),
	)

	It("Should allow network interface to be added to VirtualMachine when mutability cap is enabled", Label("smoke"), func() {
		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxVMIName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOff",
		}
		vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.DeferCleanupVirtualMachine(
			config, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, false)

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
			vm := &vmopv1.VirtualMachine{}

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
			vm := &vmopv1.VirtualMachine{}

			err := svClusterClient.Get(ctx, key, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

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

	It("Should allow network interface to be moved to a different subnet when mutability cap is enabled", Label("smoke"), func() {
		if !vmoperator.IsNetworkNsxtVPC(ctx, svClusterClient, config) {
			Skip("Test requires VPC networking environment to create SubnetSet")
		}

		subnetSetName := "custom-subnetset"

		By("Creating custom SubnetSet")
		Eventually(func(g Gomega) {
			sYaml := utils.CreateSubnetOrSubnetSetYaml(utils.SubnetSetKind, subnetSetName, input.WCPNamespaceName, utils.DHCPConfig, true)
			g.Expect(clusterProxy.CreateWithArgs(ctx, sYaml)).To(Succeed(), "failed to create the SubnetSet: %s", string(sYaml))
		}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating SubnetSet")
		vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetName, utils.SubnetSetKind)

		DeferCleanup(func() {
			vmoperator.DeleteSubnetOrSubnetSet(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetName, utils.SubnetSetKind)
			vmoperator.WaitForSubnetOrSubnetSetToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetName, utils.SubnetSetKind)
		})

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxVMIName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOff",
		}
		vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.DeferCleanupVirtualMachine(
			config, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, false)

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

		By("Change network interface to a different subnet")

		key := ctrlclient.ObjectKey{Name: vmName, Namespace: input.WCPNamespaceName}
		Eventually(func() bool {
			vm := &vmopv1.VirtualMachine{}

			err := svClusterClient.Get(ctx, key, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			Expect(vm.Spec.Network).ToNot(BeNil())
			Expect(vm.Spec.Network.Interfaces).To(HaveLen(1))

			// Change the network name to the custom SubnetSet
			vm.Spec.Network.Interfaces[0].Network.Name = subnetSetName
			vm.Spec.Network.Interfaces[0].Network.Kind = utils.SubnetSetKind
			vm.Spec.Network.Interfaces[0].Network.APIVersion = "crd.nsx.vmware.com/v1alpha1"

			err = svClusterClient.Update(ctx, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			return true
		}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(BeTrue(), "Timed out updating VirtualMachine %s to change subnet", vmName)

		By("Wait for VM to be reconfigured with updated EthernetCard")
		Eventually(func(g Gomega) {
			g.Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
			ethCards := object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*types.VirtualEthernetCard)(nil))
			g.Expect(ethCards).To(HaveLen(1), "VM should still have one EthernetCard configured")
			// We can add validation here to actually check the ethCard.  For now, just validate that
			// the reconfiguring the NIC to connect to a different network succeeds.
		}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(Succeed(), "VM reconfigured with updated EthernetCard")

		By("Power on VM")
		Eventually(func() bool {
			vm := &vmopv1.VirtualMachine{}
			err := svClusterClient.Get(ctx, key, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

			err = svClusterClient.Update(ctx, vm)
			if err != nil {
				e2eframework.Logf("retry due to: %v", err)
				return false
			}

			return true
		}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(BeTrue(), "Timed out updating VirtualMachine %s PowerState to On", vmName)

		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
		vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		By("Powered On VM should still have one EthernetCard configured")
		Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
		ethCards = object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*types.VirtualEthernetCard)(nil))
		Expect(ethCards).To(HaveLen(1), "VM config should have one EthernetCard")
	})
}
