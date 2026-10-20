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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
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
// VM creates use the shared typed-client helper, which retries transient
// connectivity errors and registers diagnostics and cleanup. Subnet,
// SubnetSet, and SecurityPolicy creates keep an explicit Eventually because
// NSX admission can reject them while their dependencies finish realizing.
// Those retried creates use ApplyWithArgs so they remain idempotent if a
// write succeeds but its response is lost between outer attempts.
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
		svClusterClient       ctrlclient.Client
		wcpClient             wcp.WorkloadManagementAPI
		clusterResources      *e2eConfig.Resources
		vmTemplate            *vmopv1a2.VirtualMachine
		vm1Name               string
		vm2Name               string
		secretName            string
		linuxImageDisplayName string
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
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		svClusterConfig := config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		vm1Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vm2Name = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		secretName = fmt.Sprintf("%s-%s", "secret", capiutil.RandomString(4))

		secret := manifestbuilders.CloudConfigSecret(manifestbuilders.Secret{
			Namespace: input.WCPNamespaceName,
			Name:      secretName,
		})
		Expect(svClusterClient.Create(ctx, secret)).To(Succeed(), "failed to create Secret %s/%s", secret.Namespace, secret.Name)
		vmservice.VerifySecretCreation(ctx, config, svClusterClient, input.WCPNamespaceName, secretName)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		vmTemplate = &vmopv1a2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Namespace: input.WCPNamespaceName},
			Spec: vmopv1a2.VirtualMachineSpec{
				ClassName:    clusterResources.VMClassName,
				ImageName:    linuxVMIName,
				StorageClass: clusterResources.StorageClassName,
				PowerState:   vmopv1a2.VirtualMachinePowerStateOn,
				Bootstrap: &vmopv1a2.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1a2.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &vmopv1a2common.SecretKeySelector{
							Key:  "user-data",
							Name: secretName,
						},
					},
				},
			},
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
			// Each VM gets its own copy of the bootstrap and network settings.
			vm1Params := vmTemplate.DeepCopy()
			vm1Params.Name = vm1Name
			vm1Params.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetKind},
							Name:     subnetDHCPName,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm1Params, false)

			vm2Params := vmTemplate.DeepCopy()
			vm2Params.Name = vm2Name
			vm2Params.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetSetKind},
							Name:     subnetSetDHCPName,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm2Params, false)

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
			vm1Params := vmTemplate.DeepCopy()
			vm1Params.Name = vm1Name
			vm1Params.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetKind},
							Name:     subnetCIDRName,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm1Params, false)

			vm2Params := vmTemplate.DeepCopy()
			vm2Params.Name = vm2Name
			vm2Params.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetSetKind},
							Name:     subnetSetCIDRName,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm2Params, false)

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

			vm1 := vmTemplate.DeepCopy()
			vm1.Name = vm1Name
			vm1.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetKind},
							Name:     nic1Name,
						},
					},
					{
						Name: "eth1",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetSetKind},
							Name:     nic2Name,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm1, false)
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
		})
	})

	Context("Across namespaces, VPC Public and Private accessMode", func() {
		It("one VirtualMachine within Private Subnet can communicate to another VM in another ns within Public Subnet", func() {
			By("Create VM1 using Private Subnet")
			createSubnetOrSubnetSet(utils.SubnetKind, subnetName, input.WCPNamespaceName, utils.CIDRConfig, true)
			DeferCleanup(vmoperator.DeleteSubnetOrSubnetSetAndWait, ctx, config, svClusterClient, input.WCPNamespaceName, subnetName, utils.SubnetKind)
			vmservice.VerifySubnetOrSubnetSetCreation(ctx, config, svClusterClient, input.WCPNamespaceName, subnetName, utils.SubnetKind)

			vm1 := vmTemplate.DeepCopy()
			vm1.Name = vm1Name
			vm1.Spec.Network = &vmopv1a2.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1a2.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Network: &vmopv1a2common.PartialObjectRef{
							TypeMeta: metav1.TypeMeta{APIVersion: vpcAPIVersion, Kind: utils.SubnetKind},
							Name:     subnetName,
						},
					},
				},
			}
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm1.DeepCopy(), false)

			By("Create a second namespace")

			secondNamespaceName := fmt.Sprintf("%s-second", input.WCPNamespaceName)
			clID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
			vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{clusterResources.VMClassName}, []string{clID})

			secondNamespaceCtx, err := clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, secondNamespaceName, input.ArtifactFolder)
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

			vm2Parameters := vm1.DeepCopy()
			vm2Parameters.Name = vm2Name
			vm2Parameters.Namespace = secondNamespaceName
			vm2Parameters.Spec.ImageName = vmImageName2
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm2Parameters, false)

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
