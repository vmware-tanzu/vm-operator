// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VMLongevityInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

func VMLongevitySpec(ctx context.Context, inputGetter func() VMLongevityInput) {
	const (
		specName            = "vm-longevity"
		vmClassName         = "longevity-vm-class"
		secondNamespaceName = specName + "-second-namespace"
	)

	var (
		input                 VMLongevityInput
		wcpClient             wcp.WorkloadManagementAPI
		config                *e2eConfig.E2EConfig
		clusterProxy          *common.VMServiceClusterProxy
		svClusterConfig       *e2eConfig.ManagementClusterConfig
		svClusterClient       ctrlclient.Client
		clusterResources      *e2eConfig.Resources
		secondNSContext       wcpframework.NamespaceContext
		vmYaml                []byte
		vmName                string
		classReady            metav1.Condition
		imageReady            metav1.Condition
		storageReady          metav1.Condition
		linuxImageDisplayName string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterConfig = config.InfraConfig.ManagementClusterConfig
		clusterResources = svClusterConfig.Resources
		svClusterClient = clusterProxy.GetClient()
		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

		By("Create a second namespace without VMClass")

		if _, err := wcpClient.GetNamespace(secondNamespaceName); err != nil {
			vmsvcSpecs := wcp.NewVMServiceSpecDetails([]string{}, []string{})
			secondNSContext, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, secondNamespaceName, input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "Failed to create a second test WCP namespace")
			wcp.WaitForNamespaceReady(wcpClient, secondNamespaceName)
		}

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, input.ClusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		By("Create VMClass only one time")

		if _, err := wcpClient.GetVMClassInfo(vmClassName); err != nil {
			createSpec, err := vmservice.GenerateVMClassSpecFunction("customize", vmClassName, 2, 1024, "description")
			Expect(err).NotTo(HaveOccurred(), "failed to create vmclass %s", vmClassName)
			Expect(wcpClient.CreateVMClass(createSpec.(wcp.VMClassSpec))).To(Succeed())
		}

		Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, vmClassName, input.WCPNamespaceName)).To(Succeed())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmClassName, "vmclass")
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, "vm")
		}

		// Delete and verify the virtual machine doesn't exist.
		if len(vmYaml) > 0 {
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	When("VM Class is associated with default namespace, not associated with the second namespace", func() {
		It("VMClass CR should exist in default namespace and not exist in the second namespace", Label("smoke"), func() {
			// Skip testing if WCP_Namespaced_VM_Class FSS is not enabled.
			skipper.SkipUnlessNamespacedVMClassFSSEnabled(ctx, svClusterClient, config)

			vmclass, err := vmoperator.GetVMClassInNamespace(ctx, svClusterClient, config, input.WCPNamespaceName, vmClassName)
			Expect(vmclass).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred(), "vmclass should exist in namespace where it's associated with")

			vmclass, err = vmoperator.GetVMClassInNamespace(ctx, svClusterClient, config, secondNamespaceName, vmClassName)
			Expect(vmclass).To(BeNil())
			Expect(err).To(HaveOccurred(), "vmclass shouldn't exist in namespace where it's not associated with")
		})
	})

	When("VM class is changed after VM successfully deployed", func() {
		BeforeEach(func() {
			// Associate VMClass to second namespace
			Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, vmClassName, secondNamespaceName)).To(Succeed())

			By("Create VirtualMachine")

			vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vmName,
				ImageName:        linuxImageDisplayName,
				VMClassName:      vmClassName,
				StorageClassName: clusterResources.StorageClassName,
				ResourcePolicy:   clusterResources.VMResourcePolicyName,
				PowerState:       "PoweredOn",
			}
			vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine", string(vmYaml))
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		})

		JustBeforeEach(func() {
			classReady = metav1.Condition{
				Type:   vmopv1a2.VirtualMachineConditionClassReady,
				Status: metav1.ConditionTrue,
			}

			imageReady = metav1.Condition{
				Type:   vmopv1a2.VirtualMachineConditionImageReady,
				Status: metav1.ConditionTrue,
			}

			storageReady = metav1.Condition{
				Type:   vmopv1a2.VirtualMachineConditionStorageReady,
				Status: metav1.ConditionTrue,
			}

			By("Verify that we have a single VirtualMachine with expected condition")
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, classReady)
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, imageReady)
			vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, storageReady)
		})

		AfterEach(func() {
			// Delete namespace
			clusterProxy.DeleteWCPNamespace(secondNSContext)
		})

		It("VMClass CR should get changed but not impact VM", func() {
			// Update VMClass
			var (
				vmclass1, vmclass2 *vmopv1a2.VirtualMachineClass
				err                error
			)

			updatedCpu := 4
			updatedMemory := 2048
			updatedSpec, err := vmservice.GenerateVMClassSpecFunction("customize", vmClassName, updatedCpu, updatedMemory, "customize-description")
			Expect(err).NotTo(HaveOccurred(), "failed to generate customized vmclass")
			Expect(wcpClient.UpdateVMClass(updatedSpec.(wcp.VMClassSpec))).To(Succeed())

			By("Updating VMClass in dcli will trigger VMClass CR get updated in both associated namespace")
			Eventually(func() bool {
				if vmclass1, err = vmoperator.GetVMClassInNamespace(ctx, svClusterClient, config, input.WCPNamespaceName, vmClassName); err != nil {
					e2eframework.Logf("failed to get vmclass in namespace %s. err: %v", input.WCPNamespaceName, err)
					return false
				}

				if vmclass2, err = vmoperator.GetVMClassInNamespace(ctx, svClusterClient, config, secondNamespaceName, vmClassName); err != nil {
					e2eframework.Logf("failed to get vmclass in namespace %s. err: %v", secondNamespaceName, err)
					return false
				}

				cpu_num1 := vmclass1.Spec.Hardware.Cpus
				memory_num1 := vmoperator.MemoryQuantityToMb(vmclass1.Spec.Hardware.Memory)
				cpu_num2 := vmclass2.Spec.Hardware.Cpus
				memory_num2 := vmoperator.MemoryQuantityToMb(vmclass2.Spec.Hardware.Memory)

				return int(cpu_num1) == updatedCpu && int(cpu_num2) == updatedCpu && memory_num1 == updatedMemory && memory_num2 == updatedMemory
			}, 30*time.Second, 3*time.Second).Should(BeTrue())

			By("VirtualMachinePrereqReadyCondition on VMs should not change")
			vmoperator.CheckVirtualMachinesConditionConsistent(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, classReady)
			vmoperator.CheckVirtualMachinesConditionConsistent(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, imageReady)
			vmoperator.CheckVirtualMachinesConditionConsistent(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, storageReady)

			By("Deleting VMClass in dcli will trigger VMClass CR get deleted in both associated namespace")
			vmservice.VerifyVMClassDeletion(wcpClient, vmClassName)

			By("VirtualMachineClassReadyCondition must be set to false")

			classReadyFalseCondition := metav1.Condition{
				Type:   vmopv1a2.VirtualMachineConditionClassReady,
				Status: metav1.ConditionFalse,
				Reason: "NotFound",
			}
			vmoperator.WaitOnVirtualMachineConditionUpdate(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, classReadyFalseCondition)
		})
	})
}
