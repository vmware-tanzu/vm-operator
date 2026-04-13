// Copyright (c) 2024-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

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

type VMVPCSpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

const (
	vpcAPIVersion     = "crd.nsx.vmware.com/v1alpha1"
	subnetKind        = "Subnet"
	subnetSetKind     = "SubnetSet"
	subnetDHCPName    = "vmsvc-subnet-dhcp"
	subnetSetDHCPName = "vmsvc-subnetset-dhcp"
	subnetCIDRName    = "vmsvc-subnet-cidr"
	subnetName        = "vmsvc-subnet-test-communication"
	subnetSetCIDRName = "vmsvc-subnetset-cidr"
	dhcpConfig        = "DHCP"
	cidrConfig        = "CIDR"
	nic1Name          = "subnet-dhcp-nic1"
	nic2Name          = "subnetset-cidr-nic2"
)

func createSubnetOrSubnetSet(ctx context.Context, g Gomega, clusterProxy *common.VMServiceClusterProxy, kind, name, ns, networkConfigType string, private bool) {
	GinkgoHelper()

	sYaml := utils.CreateSubnetOrSubnetSetYaml(kind, name, ns, networkConfigType, private)
	g.Expect(clusterProxy.CreateWithArgs(ctx, sYaml)).To(Succeed(), "failed to create the %s Subnet or SubnetSet: %s", dhcpConfig, string(sYaml))
}

func createSecurityPolicy(ctx context.Context, g Gomega, clusterProxy *common.VMServiceClusterProxy, name, ns string) {
	GinkgoHelper()

	sYaml := utils.CreateSecurityPolicyYaml(name, ns)
	g.Expect(clusterProxy.CreateWithArgs(ctx, sYaml)).To(Succeed(), "failed to create SecurityPolicy: %s", string(sYaml))
}

func createSecret(ctx context.Context, g Gomega, clusterProxy *common.VMServiceClusterProxy, ns, secretName string) {
	GinkgoHelper()
	// Create and apply Secret yaml.
	secret := manifestbuilders.Secret{
		Namespace: ns,
		Name:      secretName,
	}
	secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
	g.Expect(clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create secret: %s", string(secretYaml))
}

func createVM(ctx context.Context, g Gomega, clusterProxy *common.VMServiceClusterProxy, vmYaml []byte) {
	GinkgoHelper()
	g.Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine: %s", string(vmYaml))
}

// Delete VM before deleting Subnet or SubnetSet.
func deleteVMAndSubnet(ctx context.Context, config *e2eConfig.E2EConfig, svClusterClient ctrlclient.Client, namespace, vmName, subnetName, subnetKind string) {
	vmoperator.DeleteVirtualMachine(ctx, svClusterClient, namespace, vmName)
	vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, namespace, vmName)
	vmoperator.DeleteSubnetOrSubnetSet(ctx, svClusterClient, namespace, subnetName, subnetKind)
	vmoperator.WaitForSubnetOrSubnetSetToBeDeleted(ctx, config, svClusterClient, namespace, subnetName, subnetKind)
}

func VMVPCSpec(ctx context.Context, inputGetter func() VMVPCSpecInput) {
	const (
		specName = "vm-vpc-networking"
	)

	var (
		input                 VMVPCSpecInput
		config                *e2eConfig.E2EConfig
		clusterProxy          *common.VMServiceClusterProxy
		svClusterConfig       *e2eConfig.ManagementClusterConfig
		svClusterClient       ctrlclient.Client
		wcpClient             wcp.WorkloadManagementAPI
		clusterResources      *e2eConfig.Resources
		v1a2vmParameters      manifestbuilders.VirtualMachineYaml
		vm1Name               string
		vm2Name               string
		vm1IP                 string
		vm2IP                 string
		secretName            string
		vmYaml                []byte
		linuxImageDisplayName string
	)

	BeforeEach(func() {
		input = inputGetter()
		config = input.Config
		Expect(config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		wcpClient = input.WCPClient
		// This test is specific for networking VPC
		skipper.SkipUnlessNetworkingIsVPC(ctx, svClusterClient, config)
		// Skip if WCP_VMService_v1alpha2 FSS not enabled
		skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		svClusterConfig = config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		vm1Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vm2Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vm1IP = ""
		vm2IP = ""
		secretName = fmt.Sprintf("%s-%s", "secret", capiutil.RandomString(4))

		Eventually(func(g Gomega) {
			createSecret(ctx, g, clusterProxy, input.WCPNamespaceName, secretName)
		}, config.GetIntervals("default", "wait-secret-creation")...).Should(Succeed(), "Timed out in creating the Secret")
		vmservice.VerifySecretCreation(ctx, config, svClusterClient, input.WCPNamespaceName, secretName)

		v1a2vmParameters = manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			VMClassName:      clusterResources.VMClassName,
			ImageName:        linuxImageDisplayName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
			Bootstrap: manifestbuilders.Bootstrap{
				CloudInit: &manifestbuilders.CloudInit{
					RawCloudConfig: &manifestbuilders.KeySelector{
						Key:  "user-data",
						Name: secretName,
					},
				},
			},
		}
	})

	// Describe the VMs if the test failed before they are deleted.
	JustAfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vm1Name, "vm")
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vm2Name, "vm")
		}
	})

	AfterEach(func() {
	})

	Context("VPC DHCP should successfully create VMs", func() {
		AfterEach(func() {
			if vm1IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, subnetDHCPName, subnetKind)
			}

			if vm2IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name, subnetSetDHCPName, subnetSetKind)
			}
		})

		It("using customized DHCP Subnet/SubnetSet to assign valid ip addresses and ping each other", Label("smoke"), func() {
			By("Creating VM1 using DHCP Private Subnet")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetKind, subnetDHCPName, input.WCPNamespaceName, dhcpConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating Subnet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetDHCPName, subnetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       subnetDHCPName,
						Kind:       subnetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)

			By("Creating VM2 using DHCP Private SubnetSet")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetSetKind, subnetSetDHCPName, input.WCPNamespaceName, dhcpConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating SubnetSet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetDHCPName, subnetSetKind)

			v1a2vmParameters.Name = vm2Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       subnetSetDHCPName,
						Kind:       subnetSetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)
			vm2IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("In the same ns, two VMs on independent Private Subnet/SubnetSets should be able to communicate with each other")
			verifyLoginAndPingVM(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm1IP, vm2IP)
		})
	})

	Context("VPC CIDR should successfully create VM", func() {
		AfterEach(func() {
			if vm1IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, subnetCIDRName, subnetKind)
			}

			if vm2IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name, subnetSetCIDRName, subnetSetKind)
			}
		})

		It("using customized CIDR Subnet to assign valid ip address", func() {
			By("Creating VM1 using CIDR Private Subnet")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetKind, subnetCIDRName, input.WCPNamespaceName, cidrConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating Subnet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetCIDRName, subnetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       subnetCIDRName,
						Kind:       subnetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)

			By("Creating VM2 using CIDR Public SubnetSet")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetSetKind, subnetSetCIDRName, input.WCPNamespaceName, cidrConfig, false)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating SubnetSet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetCIDRName, subnetSetKind)

			v1a2vmParameters.Name = vm2Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       subnetSetCIDRName,
						Kind:       subnetSetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)
			vm2IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("In the same ns, two VMs on Private and Public Subnet/SubnetSets should be able to communicate with each other")
			verifyLoginAndPingVM(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm1IP, vm2IP)
		})
	})

	Context("VPC supports multiple NICs", func() {
		AfterEach(func() {
			if vm1IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, nic1Name, subnetKind)
			}
			// Delete second SubnetSet
			vmoperator.DeleteSubnetOrSubnetSet(ctx, svClusterClient, input.WCPNamespaceName, nic2Name, subnetSetKind)
			vmoperator.WaitForSubnetOrSubnetSetToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, nic2Name, subnetSetKind)
		})

		It("VM deployment should succeed with 2 NICs", func() {
			By("Creating VM with 2 NICs")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetKind, nic1Name, input.WCPNamespaceName, dhcpConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating Subnet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, nic1Name, subnetKind)

			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetSetKind, nic2Name, input.WCPNamespaceName, cidrConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating SubnetSet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, nic2Name, subnetSetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       nic1Name,
						Kind:       subnetKind,
						APIVersion: vpcAPIVersion,
					},
					{
						Name:       nic2Name,
						Kind:       subnetSetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineWithMultiNetworkYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
		})
	})

	Context("Across namespaces, VPC Public and Private accessMode", func() {
		var (
			secondNamespaceName string
			secondNamespaceCtx  wcpframework.NamespaceContext
		)

		AfterEach(func() {
			if vm1IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, subnetName, subnetKind)
			}

			if vm2IP != "" {
				deleteVMAndSubnet(ctx, config, svClusterClient, secondNamespaceName, vm2Name, subnetName, subnetKind)
			}
			// Delete the second namespace if it was created.
			if secondNamespaceCtx.GetNamespace() != nil {
				clusterProxy.DeleteWCPNamespace(secondNamespaceCtx)
			}
		})

		It("one VirtualMachine within Private Subnet can communicate to another VM in another ns within Public Subnet", func() {
			By("Create VM1 using Private Subnet")
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetKind, subnetName, input.WCPNamespaceName, cidrConfig, true)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating Subnet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetName, subnetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{
						Name:       subnetName,
						Kind:       subnetKind,
						APIVersion: vpcAPIVersion,
					},
				},
			}
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)

			By("Create a second namespace")

			secondNamespaceName = fmt.Sprintf("%s-second", input.WCPNamespaceName)
			clID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
			vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{clusterResources.VMClassName}, []string{clID})

			var err error

			secondNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, secondNamespaceName, input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "Failed to create a second test WCP namespace")
			wcp.WaitForNamespaceReady(wcpClient, secondNamespaceName)

			By("Wait for Linux VM Image to be available in the second namespace")

			var vmImageName2 string

			vmImageName2, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, secondNamespaceName, linuxImageDisplayName)
			Expect(err).NotTo(HaveOccurred(), "failed to get the VM Image name in namespace %q", secondNamespaceName)
			Expect(vmImageName2).NotTo(BeEmpty(), "VM Image CR name is empty for the second namespace")

			By("Create a secret in the second namespace with cloud-init config")
			Eventually(func(g Gomega) {
				createSecret(ctx, g, clusterProxy, secondNamespaceName, secretName)
			}, config.GetIntervals("default", "wait-secret-creation")...).Should(Succeed(), "Timed out in creating the Secret")
			vmservice.VerifySecretCreation(ctx, config, svClusterClient, secondNamespaceName, secretName)

			By("Create VM2 with a Public Subnet in the second ns")
			// Create a Public subnet with the same name but in a different ns
			Eventually(func(g Gomega) {
				createSubnetOrSubnetSet(ctx, g, clusterProxy, subnetKind, subnetName, secondNamespaceName, cidrConfig, false)
			}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating Subnet")
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, secondNamespaceName, subnetName, subnetKind)

			v1a2vmParameters.Name = vm2Name
			v1a2vmParameters.Namespace = secondNamespaceName
			v1a2vmParameters.ImageName = vmImageName2
			// Create v1alpha2 VM deployment yaml
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters)

			Eventually(func(g Gomega) {
				createVM(ctx, g, clusterProxy, vmYaml)
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, secondNamespaceName, vm2Name)
			vm2IP = vmoperator.GetVirtualMachineIP(ctx, svClusterClient, secondNamespaceName, vm2Name)

			By("Label VM2 and apply Security Policy that allows ingress")
			Expect(vmservice.LabelVM(ctx, config, clusterProxy, vm2Name, secondNamespaceName, "role", "allow-ingress")).To(Succeed())

			securityPolicyName := "allow-all-ingress"

			Eventually(func(g Gomega) {
				createSecurityPolicy(ctx, g, clusterProxy, securityPolicyName, secondNamespaceName)
			}, config.GetIntervals("default", "wait-security-policy-creation")...).Should(Succeed(), "Timed out in creating SecurityPolicy")
			vmservice.VerifySecurityPolicyCreation(ctx, config, svClusterClient, secondNamespaceName, securityPolicyName)

			By("VM1 on Private Subnet should now be able to ping VM2 on Public Subnet with Security Policy")
			verifyLoginAndPingVM(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm1IP, vm2IP)
		})
	})
}

// verifyLoginAndPingVM creates a jumpbox PodVM and exec inside the PodVM.
// From there, it SSH into vm1IP and use /dev/tcp to verify vm2IP is reachable.
func verifyLoginAndPingVM(ctx context.Context, config *e2eConfig.E2EConfig, clusterProxy *common.VMServiceClusterProxy, svClusterClient ctrlclient.Client, wcpNamespace, vm1IP, vm2IP string) {
	vmservice.WaitForPodReady(ctx, config, svClusterClient, wcpNamespace, consts.JumpboxPodVMName)

	// Photon5 VM has ICMP (ping) traffic blocked by default.
	// To verify VM communication, use the built-in /dev/tcp to open a TCP
	// connection to the other VM's IP address at port 22.
	cmds := []string{fmt.Sprintf("timeout 5 bash -c 'echo > /dev/tcp/%s/22' && echo 'VM communication successful'", vm2IP)}
	// Expect successful output for VM communication testing
	expectedOutput := []string{"VM communication successful"}
	vmservice.VerifyLoginAndRunCmdsInNSXSetup(ctx, config, clusterProxy, wcpNamespace, consts.JumpboxPodVMName, vm1IP, cmds, expectedOutput)
}
