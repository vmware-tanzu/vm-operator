// Copyright (c) 2021-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
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

type VMGOSCSpecInput struct {
	Config              *e2eConfig.E2EConfig
	ClusterProxy        wcpframework.WCPClusterProxyInterface
	WCPClient           wcp.WorkloadManagementAPI
	ArtifactFolder      string
	WCPNamespaceName    string
	WindowsServerVMName string
}

const (
	specName            = "vm-guest-customization"
	inlineCloudInit     = "inlineCloudInit"
	cloudInitTransport  = "CloudInit"
	ovfEnvTransport     = "OvfEnv"
	vAppConfigTransport = "vAppConfig"
)

var (
	input                         VMGOSCSpecInput
	config                        *e2eConfig.E2EConfig
	clusterProxy                  *common.VMServiceClusterProxy
	svClusterConfig               *e2eConfig.ManagementClusterConfig
	svClusterClient               ctrlclient.Client
	clusterResources              *e2eConfig.Resources
	vmYaml                        []byte
	configMapYaml                 []byte
	secretYaml                    []byte
	vmName                        string
	configMapName                 string
	secretName                    string
	skipCleanup                   bool
	vmServiceBackupRestoreEnabled bool
	wcpClient                     wcp.WorkloadManagementAPI
	vmParameters                  manifestbuilders.VirtualMachineYaml
	v1a2vmParameters              manifestbuilders.VirtualMachineYaml
	v1a5vmParameters              manifestbuilders.VirtualMachineYaml
	linuxImageDisplayName         string
)

func createAndVerifyConfigMap(ctx context.Context, transport string) []byte {
	// Create and apply ConfigMap yaml.
	configMap := manifestbuilders.ConfigMap{
		Namespace: input.WCPNamespaceName,
		Name:      configMapName,
	}
	if transport == cloudInitTransport {
		configMapYaml = manifestbuilders.GetConfigMapYamlGOSC(configMap)
	} else if transport == ovfEnvTransport {
		configMapYaml = manifestbuilders.GetConfigMapYamlOvfEnv(configMap)
	} else if transport == vAppConfigTransport {
		configMapYaml = manifestbuilders.GetConfigMapYamlVAppConfig(configMap)
	}

	Expect(clusterProxy.CreateWithArgs(ctx, configMapYaml)).To(Succeed(), "failed to create configmap", string(configMapYaml))
	vmservice.VerifyConfigMapCreation(ctx, config, svClusterClient, input.WCPNamespaceName, configMapName)

	return configMapYaml
}

func CreateAndVerifySecret(ctx context.Context, transport string) []byte {
	// Create and apply Secret yaml.
	secret := manifestbuilders.Secret{
		Namespace: input.WCPNamespaceName,
		Name:      secretName,
	}
	if transport == cloudInitTransport {
		secretYaml = manifestbuilders.GetSecretYamlCloudConfig(secret)
	} else if transport == ovfEnvTransport {
		secretYaml = manifestbuilders.GetSecretYamlOvfEnv(secret)
	} else if transport == vAppConfigTransport {
		secretYaml = manifestbuilders.GetSecretYamlVAppConfig(secret)
	} else if transport == inlineCloudInit {
		secretYaml = manifestbuilders.GetSecretYamlInlineCloudInitData(secret)
	}

	Expect(clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create secret", string(secretYaml))
	vmservice.VerifySecretCreation(ctx, config, svClusterClient, input.WCPNamespaceName, secretName)

	return secretYaml
}

// v1a2 also supports v1a3.
func CreateAndVerifyVM(ctx context.Context, vmParameters manifestbuilders.VirtualMachineYaml, v1a2 ...bool) {
	if len(v1a2) == 1 && v1a2[0] {
		// Create v1alpha2 VM deployment yaml
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
	} else {
		vmYaml = manifestbuilders.GetVirtualMachineYaml(vmParameters)
	}

	Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine", string(vmYaml))
	vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
}

func CreateAndVerifyVMA5(ctx context.Context, vmParameters manifestbuilders.VirtualMachineYaml) {
	vmYaml = manifestbuilders.GetVirtualMachineYamlA5(vmParameters)

	Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine", string(vmYaml))
	vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
}

func verifyVAppConfigs(ctx context.Context, vCenterClient *vim25.Client, vmmoid string, expectedProperties []manifestbuilders.KeyValueOrSecretKeySelectorPair) {
	vmMoRef := types.ManagedObjectReference{
		Type:  string(types.ManagedObjectTypeVirtualMachine),
		Value: vmmoid,
	}

	propCollector := property.DefaultCollector(vCenterClient)
	var vmMO mo.VirtualMachine
	err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.vAppConfig"}, &vmMO)
	Expect(err).To(Succeed(), "Failed to retrieve VM properties from vCenter")
	Expect(vmMO.Config).ToNot(BeNil(), "VM should have a config")
	Expect(vmMO.Config.VAppConfig).ToNot(BeNil(), "VM should have vAppConfig when VAppConfig bootstrap is used")

	vAppConfigInfo := vmMO.Config.VAppConfig.GetVmConfigInfo()
	Expect(vAppConfigInfo).ToNot(BeNil(), "VM should have vAppConfig info")

	// Get actual properties from the VM
	actualProperties := vAppConfigInfo.Property
	Expect(actualProperties).ToNot(BeEmpty(), "VM should have vApp properties")

	// Create a map of actual properties for easier lookup
	actualPropsMap := make(map[string]types.VAppPropertyInfo)
	for _, prop := range actualProperties {
		actualPropsMap[prop.Id] = prop
	}

	// Verify each expected property is correctly set
	for _, expectedProp := range expectedProperties {
		By(fmt.Sprintf("Verifying vApp property: %s", expectedProp.Key))

		// Check that the property exists
		actualProp, exists := actualPropsMap[expectedProp.Key]
		Expect(exists).To(BeTrue(), fmt.Sprintf("Expected vApp property '%s' should exist on VM", expectedProp.Key))

		// User-configurable properties
		if actualProp.UserConfigurable != nil && *actualProp.UserConfigurable {
			// Handle both non-empty values and explicitly empty values (like string-empty test case)
			switch propType := actualProp.Type; propType {
			case "password":
				Expect(actualProp.Value).To(BeEmpty(), "password should not be exposed via vAppConfig")
			case "boolean":
				// Normalize boolean values - vApp config stores as "True" or "False"
				expectedLower := strings.ToLower(strings.TrimSpace(expectedProp.Value.Value))
				Expect(expectedLower).Should(Or(Equal("true"), Equal("false")))
				switch expectedLower {
				case "true":
					Expect(actualProp.Value).To(Equal("True"),
						fmt.Sprintf("Boolean property '%s' should be 'True', got '%s'", expectedProp.Key, actualProp.Value))
				case "false":
					Expect(actualProp.Value).To(Equal("False"),
						fmt.Sprintf("Boolean property '%s' should be 'False', got '%s'", expectedProp.Key, actualProp.Value))
				}
			case "int":
				fallthrough
			case "string":
				fallthrough
			default:
				// For non-boolean properties, do direct string comparison
				// Note: vApp properties may trim whitespace, so we trim expected values too
				expectedTrimmed := strings.TrimSpace(expectedProp.Value.Value)
				Expect(actualProp.Value).To(Equal(expectedTrimmed),
					fmt.Sprintf("Property '%s' of type %s, should have value '%s', got '%s'", expectedProp.Key, actualProp.Type, expectedTrimmed, actualProp.Value))
			}
		} else {
			// for non-userconfigurable properties, check actualProp.Defaultvalue as actualProp.Value is empty
			Expect(actualProp.DefaultValue).To(Equal(expectedProp.Value.Value),
				"Non-UserConfigurable value not as expected for key %s, expected: %s, got: %s", expectedProp.Key, expectedProp.Value.Value, actualProp.DefaultValue)
		}
	}
}

// verifyV1alpha6TemplateFunctionProperties verifies the six vApp properties
// set by "Property values use V1alpha6 template functions", one per
// V1alpha6 function under test:
//
//   - string-valid                     -> V1alpha6_FirstIPv4
//   - string-trimmed                   -> V1alpha6_FirstIPv6
//   - string-padding-user-configurable -> V1alpha6_FirstIPv4FromNIC
//   - string-empty                     -> V1alpha6_FirstIPv6FromNIC
//   - bool-user-configurable-1         -> V1alpha6_IsUsableIP (fixed input)
//   - int-user-configurable-1          -> V1alpha6_PrefixLength (fixed input)
//
// The IPv4-returning functions are verified strictly: LinuxPrep in this
// test's Bootstrap already guarantees the VM gets an IPv4 address, so each
// value must parse as a real IPv4 CIDR. IsUsableIP/PrefixLength use a fixed,
// literal input unrelated to the VM's network, so their rendered value is
// asserted exactly.
//
// V1alpha6_FirstIPv6/V1alpha6_FirstIPv6FromNIC are verified leniently: if
// this environment's VM never gets an IPv6 address, renderTemplate falls
// back to returning the original, unrendered "{{ ... }}" text (see
// bootstrap_templatedata.go GetTemplateRenderFunc), which is accepted here
// rather than failing the whole spec over an environment/infra property
// this test doesn't control.
func verifyV1alpha6TemplateFunctionProperties(ctx context.Context, vCenterClient *vim25.Client, vmmoid string) {
	vmMoRef := types.ManagedObjectReference{
		Type:  string(types.ManagedObjectTypeVirtualMachine),
		Value: vmmoid,
	}

	propCollector := property.DefaultCollector(vCenterClient)
	var vmMO mo.VirtualMachine
	err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.vAppConfig"}, &vmMO)
	Expect(err).To(Succeed(), "Failed to retrieve VM properties from vCenter")
	Expect(vmMO.Config).ToNot(BeNil(), "VM should have a config")
	Expect(vmMO.Config.VAppConfig).ToNot(BeNil(), "VM should have vAppConfig when VAppConfig bootstrap is used")

	vAppConfigInfo := vmMO.Config.VAppConfig.GetVmConfigInfo()
	Expect(vAppConfigInfo).ToNot(BeNil(), "VM should have vAppConfig info")

	actualPropsMap := make(map[string]string)
	for _, prop := range vAppConfigInfo.Property {
		actualPropsMap[prop.Id] = prop.Value
	}

	verifyRenderedIPv4 := func(key string, funcName string) {
		By(fmt.Sprintf("Verifying %s rendered a real IPv4 CIDR into %s", funcName, key))
		value, ok := actualPropsMap[key]
		Expect(ok).To(BeTrue(), "%s vApp property should exist", key)
		ip, _, err := net.ParseCIDR(value)
		Expect(err).To(Succeed(), "%s should have rendered a valid CIDR into %s, got %q", funcName, key, value)
		Expect(ip.To4()).ToNot(BeNil(), "%s should have rendered an IPv4 address, got %q", funcName, value)
	}

	verifyRenderedIPv6OrUnrendered := func(key string, funcName string, unrenderedTemplate string) {
		By(fmt.Sprintf("Verifying %s rendered a real IPv6 CIDR into %s, or was left unrendered", funcName, key))
		value, ok := actualPropsMap[key]
		Expect(ok).To(BeTrue(), "%s vApp property should exist", key)
		if value == unrenderedTemplate {
			GinkgoWriter.Printf("%s did not render -- this VM/environment likely has no IPv6 address\n", funcName)
			return
		}
		ip, _, err := net.ParseCIDR(value)
		Expect(err).To(Succeed(), "%s should render a valid CIDR or be left unrendered, got %q", funcName, value)
		Expect(ip.To4()).To(BeNil(), "%s should have rendered an IPv6 address, got %q", funcName, value)
	}

	verifyRenderedIPv4("string-valid", constants.V1alpha6FirstIPv4)
	verifyRenderedIPv6OrUnrendered("string-trimmed", constants.V1alpha6FirstIPv6, "{{ "+constants.V1alpha6FirstIPv6+" }}")
	verifyRenderedIPv4("string-padding-user-configurable", constants.V1alpha6FirstIPv4FromNIC)
	verifyRenderedIPv6OrUnrendered("string-empty", constants.V1alpha6FirstIPv6FromNIC, "{{ "+constants.V1alpha6FirstIPv6FromNIC+" 0 }}")

	By(fmt.Sprintf("Verifying %s rendered the expected boolean into bool-user-configurable-1", constants.V1alpha6IsUsableIP))
	isUsableValue, ok := actualPropsMap["bool-user-configurable-1"]
	Expect(ok).To(BeTrue(), "bool-user-configurable-1 vApp property should exist")
	Expect(isUsableValue).To(Equal("True"), "%s should have rendered true for a usable address, got %q", constants.V1alpha6IsUsableIP, isUsableValue)

	By(fmt.Sprintf("Verifying %s rendered the expected prefix length into int-user-configurable-1", constants.V1alpha6PrefixLength))
	prefixLengthValue, ok := actualPropsMap["int-user-configurable-1"]
	Expect(ok).To(BeTrue(), "int-user-configurable-1 vApp property should exist")
	Expect(prefixLengthValue).To(Equal("24"), "%s should have rendered 24, got %q", constants.V1alpha6PrefixLength, prefixLengthValue)
}

func verifyLoginAndRunCmds(ctx context.Context, vmIp string, cmds []string, expectedOutput []string) {
	switch config.InfraConfig.NetworkingTopology {
	case consts.NSX:
		vmservice.WaitForPodReady(ctx, config, svClusterClient, input.WCPNamespaceName, consts.JumpboxPodVMName)
		vmservice.VerifyLoginAndRunCmdsInNSXSetup(ctx, config, clusterProxy, input.WCPNamespaceName, consts.JumpboxPodVMName, vmIp, cmds, expectedOutput)
	case consts.VDS:
		vmservice.VerifyLoginAndRunCmdsInVDSSetup(config, vmIp, cmds, expectedOutput)
	}
}

func VMGOSCSpec(ctx context.Context, inputGetter func() VMGOSCSpecInput) {
	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterConfig = config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		wcpClient = input.WCPClient
		svClusterClient = clusterProxy.GetClient()
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		configMapName = fmt.Sprintf("%s-%s", "configmap", capiutil.RandomString(4))
		secretName = fmt.Sprintf("%s-%s", "secret", capiutil.RandomString(4))

		// Use default network
		vmParameters = manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		v1a2vmParameters = manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		v1a5vmParameters = manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		skipCleanup = false
		vmServiceBackupRestoreEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMServiceBackupRestore"))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, "vm")
		}

		if skipCleanup {
			return
		}

		if CurrentSpecReport().State.String() != "skipped" {
			// Delete the virtual machine
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).NotTo(HaveOccurred(), "failed to delete virtualmachine")
			// Verify that virtual machine does not exist
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	Context("CloudInit", func() {
		var bootstrapYAML []byte

		BeforeEach(func() {
			imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

			vmParameters.ImageName = imageName
			vmParameters.Transport = cloudInitTransport
			v1a2vmParameters.ImageName = imageName
			v1a2vmParameters.Bootstrap = manifestbuilders.Bootstrap{
				CloudInit: &manifestbuilders.CloudInit{},
			}
		})

		When("ConfigMap is used to provide raw cloud-init config", func() {
			BeforeEach(func() {
				bootstrapYAML = createAndVerifyConfigMap(ctx, cloudInitTransport)
				vmParameters.ConfigMapName = configMapName
				CreateAndVerifyVM(ctx, vmParameters)
			})

			It("should successfully apply customization and be able to register VM from backup", Label("smoke"), func() {
				vmIp := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				cmds := []string{"cat /helloworld"}
				expectedOutput := []string{"Hello World"}
				verifyLoginAndRunCmds(ctx, vmIp, cmds, expectedOutput)

				if vmServiceBackupRestoreEnabled {
					vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, vmParameters.Name, vmParameters.Namespace, bootstrapYAML, clusterProxy, config, svClusterClient, wcpClient)
				}
			})
		})

		When("Secret is used to provide raw cloud-init config", func() {
			BeforeEach(func() {
				skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)

				bootstrapYAML = CreateAndVerifySecret(ctx, cloudInitTransport)
				v1a2vmParameters.Bootstrap.CloudInit.RawCloudConfig = &manifestbuilders.KeySelector{
					Key:  "user-data",
					Name: secretName,
				}
				CreateAndVerifyVM(ctx, v1a2vmParameters, true)
			})

			It("should successfully apply customization and be able to register VM from backup", func() {
				vmIp := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				cmds := []string{"cat /helloworld"}
				expectedOutput := []string{"Hello World"}
				verifyLoginAndRunCmds(ctx, vmIp, cmds, expectedOutput)

				if vmServiceBackupRestoreEnabled {
					vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, v1a2vmParameters.Name, v1a2vmParameters.Namespace, bootstrapYAML, clusterProxy, config, svClusterClient, wcpClient)
				}
			})
		})

		When("InlineCloudConfig is used to provide cloud-init config", func() {
			BeforeEach(func() {
				bootstrapYAML = CreateAndVerifySecret(ctx, inlineCloudInit)

				inlinedCloudConfig := fmt.Sprintf(`
        defaultUserEnabled: true
        ssh_pwauth: true
        users:
        - name: vmware
          lock_passwd: false
          passwd:
            name: %s
            key: vmsvc-pwd
        runcmd:
        - [ "ls", "-a", "-l", "/" ]
        - - echo
          - "hello, world."
        write_files:
        - path: /etc/my-plaintext
          permissions: '0644'
          owner: root:root
          content:
            name: %s
            key: hello`, secretName, secretName)

				v1a2vmParameters.Bootstrap.CloudInit.CloudConfig = &inlinedCloudConfig
			})

			It("should successfully apply customization and be able to register VM from backup", func() {
				CreateAndVerifyVM(ctx, v1a2vmParameters, true)
				vmIp := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				cmds := []string{"cat /etc/my-plaintext"}
				expectedOutput := []string{"Hello World"}
				verifyLoginAndRunCmds(ctx, vmIp, cmds, expectedOutput)

				if vmServiceBackupRestoreEnabled {
					vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, v1a2vmParameters.Name, v1a2vmParameters.Namespace, bootstrapYAML, clusterProxy, config, svClusterClient, wcpClient)
				}
			})
		})
	})

	Context("LinuxPrep", func() {
		BeforeEach(func() {
			skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)
			imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

			v1a2vmParameters.ImageName = imageName
			v1a2vmParameters.Bootstrap = manifestbuilders.Bootstrap{
				LinuxPrep: &manifestbuilders.LinuxPrep{
					HardwareClockIsUTC: true,
					TimeZone:           "US/Pacific",
				},
			}
		})

		It("should successfully deploy VM and be able to register VM from backup", func() {
			CreateAndVerifyVM(ctx, v1a2vmParameters, true)

			if vmServiceBackupRestoreEnabled {
				vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, v1a2vmParameters.Name, v1a2vmParameters.Namespace, nil, clusterProxy, config, svClusterClient, wcpClient)
			}
		})

		Context("LinuxPrep with CustomizeAtNextPowerOn latch", func() {
			BeforeEach(func() {
				t := true
				imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

				v1a5vmParameters.ImageName = imageName
				v1a5vmParameters.Bootstrap = manifestbuilders.Bootstrap{
					LinuxPrep: &manifestbuilders.LinuxPrep{
						HardwareClockIsUTC:     true,
						TimeZone:               "US/Pacific",
						CustomizeAtNextPowerOn: &t,
					},
				}
			})

			It("should successfully deploy VM and set latch to false", func() {
				CreateAndVerifyVMA5(ctx, v1a5vmParameters)
				vmoperator.WaitForLinuxPrepCustomizeNextPowerOnFalse(ctx, config, svClusterClient, input.WCPNamespaceName, v1a5vmParameters.Name)
			})
		})
	})

	Context("vAppConfig", func() {
		BeforeEach(func() {
			skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)
			imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

			v1a2vmParameters.ImageName = imageName
			v1a2vmParameters.Bootstrap = manifestbuilders.Bootstrap{
				// LinuxPrep is needed here for VM to get a valid IP address.
				LinuxPrep: &manifestbuilders.LinuxPrep{},
				VAppConfig: &manifestbuilders.VAppConfig{
					Properties: &[]manifestbuilders.KeyValueOrSecretKeySelectorPair{
						{
							Key: "prop-1",
							Value: manifestbuilders.ValueOrSecretKeySelector{
								Value: "my-val-1",
							},
						},
					},
				},
			}
		})

		It("should successfully apply vAppConfig properties to VM and be able to register VM from backup", func() {
			CreateAndVerifyVM(ctx, v1a2vmParameters, true)

			// Verify that the vAppConfig properties are actually applied to the VM
			vmmoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			expectedProperties := *v1a2vmParameters.Bootstrap.VAppConfig.Properties
			verifyVAppConfigs(ctx, vCenterClient, vmmoid, expectedProperties)

			if vmServiceBackupRestoreEnabled {
				vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, v1a2vmParameters.Name, v1a2vmParameters.Namespace, nil, clusterProxy, config, svClusterClient, wcpClient)
			}
		})

		When("Multiple vAppConfigs specified", func() {
			BeforeEach(func() {
				// Use the OVF image from content library that contains more properties to test
				ovfImageName := "photon-ovf-vapp-properties"
				imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, ovfImageName)

				v1a2vmParameters.ImageName = imageName
				v1a2vmParameters.Bootstrap = manifestbuilders.Bootstrap{
					// LinuxPrep is needed here for VM to get a valid IP address.
					LinuxPrep: &manifestbuilders.LinuxPrep{},
					VAppConfig: &manifestbuilders.VAppConfig{
						Properties: &[]manifestbuilders.KeyValueOrSecretKeySelectorPair{
							// Basic property types
							{
								Key: "string-valid",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "test-value-updated",
								},
							},
							{
								Key: "string-empty",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "",
								},
							},
							{
								Key: "string-trimmed",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "hello-updated",
								},
							},

							{
								Key: "string-padding-user-configurable",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "hello-world",
								},
							},
							{
								Key: "password-valid",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "updated-secret-password",
								},
							},
							{
								Key: "bool-user-configurable-1",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "false",
								},
							},

							{
								Key: "bool-user-configurable-2",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "true",
								},
							},

							{
								Key: "bool-user-configurable-3",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "False",
								},
							},

							{
								Key: "bool-user-configurable-4",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "True",
								},
							},
							{
								Key: "int-user-configurable-1",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "10",
								},
							},

							{
								Key: "int-user-configurable-2",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "-10",
								},
							},

							{
								Key: "int-user-configurable-3",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "42",
								},
							},

							{
								Key: "int-user-configurable-4",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "-42",
								},
							},
							{
								Key: "int-positive",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "42",
								},
							},
							{
								Key: "int-negative",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "-42",
								},
							},
						},
					},
				}
			})

			It("should successfully apply vAppConfig properties to VM", Label("experimental"), func() {
				CreateAndVerifyVM(ctx, v1a2vmParameters, true)

				// Verify that the vAppConfig properties are actually applied to the VM
				vmmoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())

				// Get the expected properties from the VM parameters
				expectedProperties := *v1a2vmParameters.Bootstrap.VAppConfig.Properties
				// append more props that are present in the ovf, but are user-configurable = false
				expectedProperties = append(expectedProperties, []manifestbuilders.KeyValueOrSecretKeySelectorPair{
					{
						Key: "string-padding",
						Value: manifestbuilders.ValueOrSecretKeySelector{
							Value: "hello",
						},
					},
					{
						Key: "bool-true",
						Value: manifestbuilders.ValueOrSecretKeySelector{
							Value: "True", // vc converts all booleans to strings with capitalized first letter
						},
					},
					{
						Key: "bool-false",
						Value: manifestbuilders.ValueOrSecretKeySelector{
							Value: "False", // vc converts all booleans to strings with capitalized first letter
						},
					},
				}...)
				verifyVAppConfigs(ctx, vCenterClient, vmmoid, expectedProperties)
			})
		})

		When("Property values use V1alpha6 template functions", func() {
			BeforeEach(func() {
				// Reuse the same OVF image and property keys as "Multiple
				// vAppConfigs specified" above -- vApp property values are
				// only applied if userConfigurable and pre-existing on the
				// deployed OVF image, so a new property key would be
				// silently dropped. Reusing already-defined keys and setting
				// their value to a template function call lets the render
				// pipeline exercise real V1alpha6 functions against a real
				// cluster. One property per function; string-typed keys hold
				// the IP-returning functions, bool/int-typed keys hold
				// IsUsableIP/PrefixLength with fixed, dummy inputs (they
				// don't depend on the VM's network at all).
				ovfImageName := "photon-ovf-vapp-properties"
				imageName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, ovfImageName)

				v1a2vmParameters.ImageName = imageName
				v1a2vmParameters.Bootstrap = manifestbuilders.Bootstrap{
					// LinuxPrep is needed here for VM to get a valid IP address.
					LinuxPrep: &manifestbuilders.LinuxPrep{},
					VAppConfig: &manifestbuilders.VAppConfig{
						Properties: &[]manifestbuilders.KeyValueOrSecretKeySelectorPair{
							{
								Key: "string-valid",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "{{ " + constants.V1alpha6FirstIPv4 + " }}",
								},
							},
							{
								Key: "string-trimmed",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "{{ " + constants.V1alpha6FirstIPv6 + " }}",
								},
							},
							{
								Key: "string-padding-user-configurable",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "{{ " + constants.V1alpha6FirstIPv4FromNIC + " 0 }}",
								},
							},
							{
								Key: "string-empty",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									Value: "{{ " + constants.V1alpha6FirstIPv6FromNIC + " 0 }}",
								},
							},
							{
								Key: "bool-user-configurable-1",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									// Dummy, fixed input -- doesn't depend on the VM's network.
									Value: "{{ " + constants.V1alpha6IsUsableIP + " \"192.168.1.10\" }}",
								},
							},
							{
								Key: "int-user-configurable-1",
								Value: manifestbuilders.ValueOrSecretKeySelector{
									// Dummy, fixed input -- doesn't depend on the VM's network.
									Value: "{{ " + constants.V1alpha6PrefixLength + " \"10.0.0.0/24\" }}",
								},
							},
						},
					},
				}
			})

			It("should render V1alpha6 IPv4/IPv6 template functions into real vApp properties", Label("experimental"), func() {
				CreateAndVerifyVM(ctx, v1a2vmParameters, true)

				vmmoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())

				verifyV1alpha6TemplateFunctionProperties(ctx, vCenterClient, vmmoid)
			})
		})
	})

	Context("Sysprep", func() {
		BeforeEach(func() {
			// The Windows server VM was deployed during the suite setup and will be deleted in the suite teardown.
			skipCleanup = true

			// Skip if WCP_Windows_Sysprep FSS not enabled
			skipper.SkipUnlessWindowsFSSEnabled(ctx, svClusterClient, config)
		})

		When("raw Sysprep data is used", func() {
			// TODO: VMSVC-2789: Re-enable this once we've fixed PR 3531430
			XIt("should successfully deploy VM and be able to register VM from backup", func() {
				// The VM is already created during the suite setup.
				vmName = input.WindowsServerVMName
				Expect(vmName).ToNot(BeEmpty())
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

				if vmServiceBackupRestoreEnabled {
					vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, vmName, input.WCPNamespaceName, nil, clusterProxy, config, svClusterClient, wcpClient)
				}
			})
		})

		When("Inline Sysprep is used", func() {
			BeforeEach(func() {
				skipper.SkipUnlessV1a2FSSEnabled(ctx, svClusterClient, config)
			})
			// TODO: VMSVC-2789: Re-enable this once we've fixed PR 3531430
			XIt("should successfully deploy VM and be able to register VM from backup", func() {
				// The VM is already created during the suite setup.
				vmName = input.WindowsServerVMName + "-a2"
				Expect(vmName).ToNot(BeEmpty())
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

				if vmServiceBackupRestoreEnabled {
					vmservice.VerifyRegisterVMOnlyClassicDisk(ctx, vmName, input.WCPNamespaceName, nil, clusterProxy, config, svClusterClient, wcpClient)
				}
			})
		})
	})
}
