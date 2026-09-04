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

// VMVPCSpec exercises VPC Subnet/SubnetSet-backed VM networking.
//
// clusterProxy.CreateWithArgs/ApplyWithArgs and the controller-runtime client
// returned by GetClient already retry transient connectivity errors
// internally (see framework/retry_client.go and
// vmservice/common/vmservice_clusterproxy.go), so most creates below are
// single-shot. Subnet, SubnetSet, VirtualMachine, and SecurityPolicy creates
// are the exception and keep an explicit Eventually: their admission
// webhooks (NSX operator, VM Operator) can reject with a real, non-transient
// validation error simply because a referenced object -- a just-created
// Subnet/SubnetSet, or the namespace's VPC/NSX projection -- has not
// finished realizing yet. That is a distinct failure class from the
// dial/timeout errors the lower-level retries classify as transient, so it
// still needs to be ridden out here. These retried creates use
// ApplyWithArgs rather than CreateWithArgs: CreateWithArgs only tolerates
// AlreadyExists within its own internal retry attempts, so wrapping it in an
// outer Eventually can surface a hard AlreadyExists failure if an earlier
// outer attempt's write landed after a delayed/lost response -- Apply has
// no such create-vs-update distinction to trip over.
func VMVPCSpec(ctx context.Context, inputGetter func() VMVPCSpecInput) {
	const (
		specName = "vm-vpc-networking"

		vpcAPIVersion     = "crd.nsx.vmware.com/v1alpha1"
		subnetDHCPName    = "vmsvc-subnet-dhcp"
		subnetSetDHCPName = "vmsvc-subnetset-dhcp"
		subnetCIDRName    = "vmsvc-subnet-cidr"
		subnetName        = "vmsvc-subnet-test-communication"
		subnetSetCIDRName = "vmsvc-subnetset-cidr"
		nic1Name          = "subnet-dhcp-nic1"
		nic2Name          = "subnetset-cidr-nic2"
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
		vm2Namespace          string
		secretName            string
		linuxImageDisplayName string
		linuxVMIName          string
	)

	// createSubnetOrSubnetSet creates a Subnet or SubnetSet, retrying on a
	// real (non-transient) validation error while its dependencies finish
	// realizing; see the VMVPCSpec doc comment.
	createSubnetOrSubnetSet := func(kind, name, ns, networkConfigType string, private bool) {
		GinkgoHelper()

		subnetYaml := utils.CreateSubnetOrSubnetSetYaml(kind, name, ns, networkConfigType, private)
		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.ApplyWithArgs(ctx, subnetYaml)).To(Succeed(), "failed to create the %s %s/%s: %s", kind, ns, name, string(subnetYaml))
		}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out in creating %s %s/%s", kind, ns, name)
	}

	// createVM creates a VirtualMachine, retrying on a real (non-transient)
	// validation error while its dependencies finish realizing; see the
	// VMVPCSpec doc comment.
	createVM := func(vmYaml []byte) {
		GinkgoHelper()

		Eventually(func(g Gomega) {
			g.Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine: %s", string(vmYaml))
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out in creating the VirtualMachine")
	}

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
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		vm1Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vm2Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vm2Namespace = input.WCPNamespaceName
		secretName = fmt.Sprintf("%s-%s", "secret", capiutil.RandomString(4))

		secretYaml := manifestbuilders.GetSecretYamlCloudConfig(manifestbuilders.Secret{
			Namespace: input.WCPNamespaceName,
			Name:      secretName,
		})
		Expect(clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create secret: %s", string(secretYaml))
		vmservice.VerifySecretCreation(ctx, config, svClusterClient, input.WCPNamespaceName, secretName)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		v1a2vmParameters = manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			VMClassName:      clusterResources.VMClassName,
			ImageName:        linuxVMIName,
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
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), vm2Namespace, vm2Name, "vm")
		}
	})

	Context("VPC DHCP should successfully create VMs", func() {
		It("using customized DHCP Subnet/SubnetSet to assign valid ip addresses and ping each other", Label("smoke"), func() {
			By("Creating a DHCP Private Subnet for VM1 and a DHCP Private SubnetSet for VM2")
			createSubnetOrSubnetSet(utils.SubnetKind, subnetDHCPName, input.WCPNamespaceName, utils.DHCPConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetDHCPName, utils.SubnetKind)

			createSubnetOrSubnetSet(utils.SubnetSetKind, subnetSetDHCPName, input.WCPNamespaceName, utils.DHCPConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetDHCPName, utils.SubnetSetKind)

			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetDHCPName, utils.SubnetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetDHCPName, utils.SubnetSetKind)

			By("Creating VM1 using the DHCP Private Subnet and VM2 using the DHCP Private SubnetSet")
			// vm1Params/vm2Params are shallow copies of v1a2vmParameters: only
			// value fields (Name, Namespace, NetworkA2, ImageName) are set
			// below, so the shared Bootstrap.CloudInit pointer is never
			// mutated through either copy. Do not start writing through
			// vmNParams.Bootstrap.* without giving it its own copy first.
			vm1Params := v1a2vmParameters
			vm1Params.Name = vm1Name
			vm1Params.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: subnetDHCPName, Kind: utils.SubnetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineYamlA2(vm1Params))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)

			vm2Params := v1a2vmParameters
			vm2Params.Name = vm2Name
			vm2Params.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: subnetSetDHCPName, Kind: utils.SubnetSetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineYamlA2(vm2Params))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("Waiting for both VMs to be created with IPs")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)
			vm2IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("In the same ns, two VMs on independent Private Subnet/SubnetSets should be able to communicate with each other")
			verifyLoginAndPingVM(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm1IP, vm2IP)
		})
	})

	Context("VPC CIDR should successfully create VM", func() {
		It("using customized CIDR Subnet to assign valid ip address", func() {
			By("Creating a CIDR Private Subnet for VM1 and a CIDR Public SubnetSet for VM2")
			createSubnetOrSubnetSet(utils.SubnetKind, subnetCIDRName, input.WCPNamespaceName, utils.CIDRConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetCIDRName, utils.SubnetKind)

			createSubnetOrSubnetSet(utils.SubnetSetKind, subnetSetCIDRName, input.WCPNamespaceName, utils.CIDRConfig, false)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetCIDRName, utils.SubnetSetKind)

			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetCIDRName, utils.SubnetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetSetCIDRName, utils.SubnetSetKind)

			By("Creating VM1 using the CIDR Private Subnet and VM2 using the CIDR Public SubnetSet")
			vm1Params := v1a2vmParameters
			vm1Params.Name = vm1Name
			vm1Params.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: subnetCIDRName, Kind: utils.SubnetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineYamlA2(vm1Params))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)

			vm2Params := v1a2vmParameters
			vm2Params.Name = vm2Name
			vm2Params.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: subnetSetCIDRName, Kind: utils.SubnetSetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineYamlA2(vm2Params))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("Waiting for both VMs to be created with IPs")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)
			vm2IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("In the same ns, two VMs on Private and Public Subnet/SubnetSets should be able to communicate with each other")
			verifyLoginAndPingVM(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm1IP, vm2IP)
		})
	})

	Context("VPC supports multiple NICs", func() {
		It("VM deployment should succeed with 2 NICs", func() {
			By("Creating VM with 2 NICs")
			createSubnetOrSubnetSet(utils.SubnetKind, nic1Name, input.WCPNamespaceName, utils.DHCPConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, nic1Name, utils.SubnetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, nic1Name, utils.SubnetKind)

			createSubnetOrSubnetSet(utils.SubnetSetKind, nic2Name, input.WCPNamespaceName, utils.CIDRConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, nic2Name, utils.SubnetSetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, nic2Name, utils.SubnetSetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: nic1Name, Kind: utils.SubnetKind, APIVersion: vpcAPIVersion},
					{Name: nic2Name, Kind: utils.SubnetSetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineWithMultiNetworkYamlA2(v1a2vmParameters))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
		})
	})

	Context("Across namespaces, VPC Public and Private accessMode", func() {
		var (
			secondNamespaceName string
			secondNamespaceCtx  wcpframework.NamespaceContext
		)

		It("one VirtualMachine within Private Subnet can communicate to another VM in another ns within Public Subnet", func() {
			By("Create VM1 using Private Subnet")
			createSubnetOrSubnetSet(utils.SubnetKind, subnetName, input.WCPNamespaceName, utils.CIDRConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetName, utils.SubnetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetName, utils.SubnetKind)

			v1a2vmParameters.Name = vm1Name
			v1a2vmParameters.NetworkA2 = manifestbuilders.NetworkA2{
				Interfaces: []manifestbuilders.InterfaceSpec{
					{Name: subnetName, Kind: utils.SubnetKind, APIVersion: vpcAPIVersion},
				},
			}
			createVM(manifestbuilders.GetVirtualMachineYamlA2(v1a2vmParameters))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)

			By("Create a second namespace")

			secondNamespaceName = fmt.Sprintf("%s-second", input.WCPNamespaceName)
			vm2Namespace = secondNamespaceName
			clID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
			vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{clusterResources.VMClassName}, []string{clID})

			var err error

			secondNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, secondNamespaceName, input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "Failed to create a second test WCP namespace")
			DeferCleanup(clusterProxy.DeleteWCPNamespace, secondNamespaceCtx)
			wcp.WaitForNamespaceReady(wcpClient, secondNamespaceName)

			By("Wait for Linux VM Image to be available in the second namespace")
			vmImageName2 := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, secondNamespaceName, linuxImageDisplayName)
			Expect(vmImageName2).NotTo(BeEmpty(), "VM Image CR name is empty for the second namespace")

			// The second namespace was created moments ago: its RBAC/quota/NSX
			// CRD projection onto the SV API server can lag behind
			// WaitForNamespaceReady, which only observes the WCP-side config
			// status. That is a second, namespace-readiness reason (on top of
			// the general one in the VMVPCSpec doc comment) why this secret
			// create keeps an explicit Eventually despite being a plain
			// corev1 object.
			By("Create a secret in the second namespace with cloud-init config")
			secretYaml := manifestbuilders.GetSecretYamlCloudConfig(manifestbuilders.Secret{
				Namespace: secondNamespaceName,
				Name:      secretName,
			})
			Eventually(func(g Gomega) {
				g.Expect(clusterProxy.ApplyWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create secret: %s", string(secretYaml))
			}, config.GetIntervals("default", "wait-secret-creation")...).Should(Succeed(), "Timed out in creating the Secret")
			vmservice.VerifySecretCreation(ctx, config, svClusterClient, secondNamespaceName, secretName)

			By("Create VM2 with a Public Subnet in the second ns")
			// Create a Public subnet with the same name but in a different ns
			createSubnetOrSubnetSet(utils.SubnetKind, subnetName, secondNamespaceName, utils.CIDRConfig, false)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, secondNamespaceName, subnetName, utils.SubnetKind)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, secondNamespaceName, subnetName, utils.SubnetKind)

			vm2Parameters := v1a2vmParameters
			vm2Parameters.Name = vm2Name
			vm2Parameters.Namespace = secondNamespaceName
			vm2Parameters.ImageName = vmImageName2
			createVM(manifestbuilders.GetVirtualMachineYamlA2(vm2Parameters))
			DeferCleanup(vmoperator.DeleteVirtualMachineAndWait, ctx, config, svClusterClient, secondNamespaceName, vm2Name)

			By("Label VM2 and apply Security Policy that allows ingress")
			Expect(vmservice.LabelVM(ctx, config, clusterProxy, vm2Name, secondNamespaceName, "role", "allow-ingress")).To(Succeed())

			securityPolicyName := "allow-all-ingress"
			securityPolicyYaml := utils.CreateSecurityPolicyYaml(securityPolicyName, secondNamespaceName)

			Eventually(func(g Gomega) {
				g.Expect(clusterProxy.ApplyWithArgs(ctx, securityPolicyYaml)).To(Succeed(), "failed to create SecurityPolicy: %s", string(securityPolicyYaml))
			}, config.GetIntervals("default", "wait-security-policy-creation")...).Should(Succeed(), "Timed out in creating SecurityPolicy")
			vmservice.VerifySecurityPolicyCreation(ctx, config, svClusterClient, secondNamespaceName, securityPolicyName)

			By("Wait for both VMs to be created with IPs")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm1IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, secondNamespaceName, vm2Name)
			vm2IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, secondNamespaceName, vm2Name)

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
