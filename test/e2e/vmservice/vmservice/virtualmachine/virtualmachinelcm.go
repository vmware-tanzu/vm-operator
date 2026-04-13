// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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

const (
	poweredOnState  = "PoweredOn"
	poweredOffState = "PoweredOff"
)

type VMSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
	LinuxVMName      string
}

func VMSpec(ctx context.Context, inputGetter func() VMSpecInput) {
	const (
		specName = "vm-lcm"
	)

	var (
		input                             VMSpecInput
		wcpClient                         wcp.WorkloadManagementAPI
		vCenterClient                     *vim25.Client
		propCollector                     *property.Collector
		config                            *e2eConfig.E2EConfig
		clusterProxy                      *common.VMServiceClusterProxy
		svClusterClient                   ctrlclient.Client
		clusterResources                  *e2eConfig.Resources
		tmpNamespaceCtx                   wcpframework.NamespaceContext
		vmYaml                            []byte
		vmName                            string
		instanceStorageFssEnabled         bool
		vmClassAsConfigDaynDateFssEnabled bool
		namespacedVMClassFSSEnabled       bool
		vmResizeCPUMemoryFssEnabled       bool
		isoSupportFSSEnabled              bool
		linuxImageDisplayName             string
		linuxImageGuestID                 string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(input.LinuxVMName).ToNot(BeEmpty(), "Invalid argument. input.LinuxVMName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxImageGuestID = vmservice.GetDefaultImageGuestID()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		propCollector = property.DefaultCollector(vCenterClient)

		instanceStorageFssEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSInstanceStorage"))
		vmClassAsConfigDaynDateFssEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMClassAsConfigDaynDate"))
		namespacedVMClassFSSEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSNamespacedVMClass"))
		vmResizeCPUMemoryFssEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMResizeCPUMemory"))
		isoSupportFSSEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSIsoSupport"))

		vmYaml = nil
		tmpNamespaceCtx = wcpframework.NamespaceContext{}
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
	})

	AfterEach(func() {
		vmNamespaceName := input.WCPNamespaceName
		if tmpNamespaceCtx.GetNamespace() != nil {
			vmNamespaceName = tmpNamespaceCtx.GetNamespace().Name
		}

		if CurrentSpecReport().Failed() {
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

		if vCenterClient != nil {
			vcenter.LogoutVimClient(vCenterClient)
		}
	})

	It("Should create expected resources for a single VirtualMachine", Label("smoke"), func() {
		// Use the Linux VM deployed in the vmservice_suite_test.go to avoid
		// creating another VM with similar config in this read only test.
		vmName = input.LinuxVMName
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		By("Verifying VM gets expected BIOS and instance UUID")
		virtualMachine, err := utils.GetVirtualMachineA3(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		e2eframework.ExpectNoError(err)
		Expect(virtualMachine.Spec.BiosUUID).To(Equal(virtualMachine.Status.BiosUUID))

		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}
		var vmMO mo.VirtualMachine
		Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)).To(Succeed())
		Expect(virtualMachine.Spec.BiosUUID).To(Equal(vmMO.Config.Uuid))
		Expect(virtualMachine.Spec.InstanceUUID).To(Equal(vmMO.Config.InstanceUuid))
	})

	It("Should create expected resources for a single poweredOff VirtualMachine when VM_Class_as_Config_DaynDate Enabled", func() {
		if !vmClassAsConfigDaynDateFssEnabled {
			Skip("VM_Class_as_Config_DaynDate FSS is not enabled")
		}

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOff",
		}

		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}

		var vmMO mo.VirtualMachine
		err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)
		e2eframework.ExpectNoError(err)

		hw := vmMO.Config.Hardware
		var vmClass *vmopv1a2.VirtualMachineClass
		// Depending on namespacedVMClassFSS, return namespaced VM Class or cluster scoped VM Class
		vmClass, err = utils.GetVirtualMachineClass(ctx, svClusterClient, clusterResources.VMClassName, input.WCPNamespaceName, namespacedVMClassFSSEnabled)
		e2eframework.ExpectNoError(err)

		Expect(hw.NumCPU).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
		Expect(hw.MemoryMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
	})

	It("Should resize powered off VirtualMachine when VM_Resize_CPU_Memory is Enabled", func() {
		if !vmResizeCPUMemoryFssEnabled {
			Skip("VM_Resize_CPU_Memory FSS is not enabled")
		}

		const newVMClassName = "guaranteed-large"

		Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, clusterResources.VMClassName, input.WCPNamespaceName)).To(Succeed())
		Expect(vmservice.EnsureVMClassPresent(wcpClient, newVMClassName)).To(Succeed())
		Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, newVMClassName, input.WCPNamespaceName)).To(Succeed())

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOff",
		}

		vmYaml = manifestbuilders.GetVirtualMachineYaml(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		By(fmt.Sprintf("Verify that a single VirtualMachine '%s/%s' is created", input.WCPNamespaceName, vmName))
		vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}

		var vmMO mo.VirtualMachine
		err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)
		e2eframework.ExpectNoError(err)

		// Verify initial CPU and memory.
		vmClass, err := utils.GetVirtualMachineClass(ctx, svClusterClient, clusterResources.VMClassName, input.WCPNamespaceName, namespacedVMClassFSSEnabled)
		e2eframework.ExpectNoError(err)
		Expect(vmMO.Config.Hardware.NumCPU).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
		Expect(vmMO.Config.Hardware.MemoryMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))

		// Change VM's ClassName.
		vmoperator.UpdateVirtualMachineClassName(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, newVMClassName)

		// Wait for Reconfigure.
		vmoperator.WaitForVirtualMachineStatusClassUpdated(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, newVMClassName)

		classConfigSyncedCondition := metav1.Condition{
			Type:   vmopv1a2.VirtualMachineClassConfigurationSynced,
			Status: metav1.ConditionTrue,
		}
		vmoperator.WaitOnVirtualMachineConditionUpdate(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, classConfigSyncedCondition)

		newVMClass, err := utils.GetVirtualMachineClass(ctx, svClusterClient, newVMClassName, input.WCPNamespaceName, namespacedVMClassFSSEnabled)
		e2eframework.ExpectNoError(err)
		// Assert that we can tell that a resize happened.
		Expect(vmClass.Spec.Hardware.Cpus).ToNot(Equal(newVMClass.Spec.Hardware.Cpus))
		Expect(vmClass.Spec.Hardware.Memory).ToNot(Equal(newVMClass.Spec.Hardware.Memory))

		By("VC VM configuration should have been updated")
		err = propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)
		e2eframework.ExpectNoError(err)
		Expect(vmMO.Config.Hardware.NumCPU).To(BeEquivalentTo(newVMClass.Spec.Hardware.Cpus))
		Expect(vmMO.Config.Hardware.MemoryMB).To(BeEquivalentTo(newVMClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
		// Make sure the VM reflects the guaranteed class.
		cpuAlloc := vmMO.Config.CpuAllocation
		Expect(cpuAlloc).ToNot(BeNil())
		Expect(cpuAlloc.Limit).To(HaveValue(BeEquivalentTo(-1)))
		Expect(cpuAlloc.Reservation).To(HaveValue(BeNumerically(">", int64(0))))
		memAlloc := vmMO.Config.MemoryAllocation
		Expect(memAlloc).ToNot(BeNil())
		Expect(memAlloc.Limit).To(HaveValue(BeEquivalentTo(-1)))
		Expect(memAlloc.Reservation).To(HaveValue(BeNumerically(">", int64(0))))

		vmservice.VerifyVMClassDeletion(wcpClient, newVMClassName)
	})

	It("Should create expected resources for a single VirtualMachine when VM_Class_as_Config_DaynDate Enabled", func() {
		if !vmClassAsConfigDaynDateFssEnabled {
			Skip("VM_Class_as_Config_DaynDate FSS is not enabled")
		}

		Expect(vmservice.EnsureVMClassPresent(wcpClient, vmservice.VMClassE1000)).To(Succeed())

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      vmservice.VMClassE1000,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)

		err := vmservice.EnsureNamespaceHasAccess(input.WCPClient, vmservice.VMClassE1000, input.WCPNamespaceName)
		e2eframework.ExpectNoError(err)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}

		var vmMO mo.VirtualMachine
		err = propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)
		e2eframework.ExpectNoError(err)

		hw := vmMO.Config.Hardware
		// verify nic type
		virtualDevices := object.VirtualDeviceList(hw.Device)
		currentEthCards := virtualDevices.SelectByType((*types.VirtualE1000)(nil))
		Expect(len(currentEthCards)).To(Equal(1))

		extraConfig := vmMO.Config.ExtraConfig
		ecMap := make(map[string]string)
		for _, ec := range extraConfig {
			if optionValue := ec.GetOptionValue(); optionValue != nil {
				ecMap[optionValue.Key] = optionValue.Value.(string)
			}
		}
		Expect(ecMap).To(HaveKeyWithValue("hello-test-key", "hello-test-value"))

		// verify cpu and memory
		var vmClass *vmopv1a2.VirtualMachineClass
		// Depending on namespacedVMClassFSS, return namespaced VM Class or cluster scoped VM Class
		vmClass, err = utils.GetVirtualMachineClass(ctx, svClusterClient, vmservice.VMClassE1000, input.WCPNamespaceName, namespacedVMClassFSSEnabled)
		e2eframework.ExpectNoError(err)
		Expect(hw.NumCPU).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
		Expect(hw.MemoryMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))

		vmservice.VerifyVMClassDeletion(wcpClient, vmservice.VMClassE1000)
	})

	It("Should create expected resources for a single VirtualMachine with hardware version 22", func() {
		Expect(vmservice.EnsureVMClassPresent(wcpClient, vmservice.VMClassVMX22)).To(Succeed())

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      vmservice.VMClassVMX22,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
			Annotations: map[string]string{
				"vmoperator.vmware.com/fast-deploy": "\"false\"",
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)

		err := vmservice.EnsureNamespaceHasAccess(input.WCPClient, vmservice.VMClassVMX22, input.WCPNamespaceName)
		e2eframework.ExpectNoError(err)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}

		var vmMO mo.VirtualMachine
		err = propCollector.RetrieveOne(ctx, vmMoRef, []string{"config"}, &vmMO)
		e2eframework.ExpectNoError(err)

		hwVersion := vmMO.Config.Version
		hw := vmMO.Config.Hardware
		// verify hardware version is set
		Expect(hwVersion).To(Equal("vmx-22"))

		// verify cpu and memory
		var vmClass *vmopv1a2.VirtualMachineClass
		vmClass, err = utils.GetVirtualMachineClass(ctx, svClusterClient, vmservice.VMClassVMX22, input.WCPNamespaceName, namespacedVMClassFSSEnabled)
		e2eframework.ExpectNoError(err)
		Expect(hw.NumCPU).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
		Expect(hw.MemoryMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))

		vmservice.VerifyVMClassDeletion(wcpClient, vmservice.VMClassVMX22)
	})

	It("Should create expected resources for a VirtualMachine from poweredOff to poweredOn", Label("smoke"), func() {
		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			ImageName:        linuxImageDisplayName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOff",
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

		By("Verify that the VirtualMachine can be powered on and also powered off after")
		vmParameters.PowerState = poweredOnState
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		e2eframework.Logf("Updating the VM's PowerState to '%v'", vmParameters.PowerState)
		Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to power-on virtualmachine", string(vmYaml))
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
		vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		// Power off the VM and verify that this doesn't cause any issues.
		vmParameters.PowerState = poweredOffState
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		e2eframework.Logf("Updating the VM's PowerState to '%v'", vmParameters.PowerState)
		Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to power-off virtualmachine", string(vmYaml))
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
	})

	It("Should create expected resources for a VirtualMachine on Namespaces recreated with identical names", func() {
		// Create a new namespace here to avoid overwriting the existing namespace also used by other specs.
		tmpNamespaceName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))
		vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
		clIDs := []string{vmserviceCLID}
		vmClassNames := []string{clusterResources.VMClassName}
		vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
		var err error
		tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, tmpNamespaceName, input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace")
		wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

		// Ensure the Linux VMI name is present in the temp namespace.
		vmiName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", tmpNamespaceName)

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}

		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		// Delete the virtual machine and temporary namespace to recreate the latter with the same name.
		Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
		// Reset variable since temporary VM and namespace deletion is handled in AfterEach.
		vmYaml = nil
		vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, tmpNamespaceName, vmName)
		clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
		wcp.WaitForNamespaceDeleted(wcpClient, tmpNamespaceName)

		// Recreate the namespace with the same name and spec.
		tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, tmpNamespaceName, input.ArtifactFolder)
		Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
		wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

		// Ensure the Linux VMI name is present in the temp namespace.
		vmiName, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", tmpNamespaceName)
		vmParameters.ImageName = vmiName

		// Create a new VM and verify the creation is successful.
		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		vmParameters.Name = vmName
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
	})

	It("Should emit a condition for a VMClass that doesn't exist in the cluster for a Virtual Machine creation", func() {
		vmClassName := "test-vmClass"
		expectedCondition := metav1.Condition{
			Type:    vmopv1a2.VirtualMachineConditionClassReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NotFound",
			Message: fmt.Sprintf("Failed to get VirtualMachineClass %s", vmClassName),
		}

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
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		By("Verify that we have a single VirtualMachine with expected condition")
		vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, expectedCondition)
	})

	It("Should emit a condition for a VMClass that is not attached to the namespace for a Virtual Machine creation", func() {
		vmClassName := "e2etest-guaranteed-medium"
		// Create another workload and attach 'e2etest-guaranteed-medium' VM class to it,
		// otherwise 'e2etest-guaranteed-medium' CR would be deleted if it has zero workload associations
		Expect(vmservice.EnsureVMClassPresent(wcpClient, vmClassName)).To(Succeed())
		newNamespaceName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))
		vmClassNames := []string{vmClassName}
		clIDs := []string{}
		vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
		// Not assigning to the tmpNamespaceCtx because no VMs will be deployed in this namespace.
		// So that in AfterEach, it will delete the VM from the correct namespace (input.WCPNamespaceName).
		newNamespaceCtx, err := clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, newNamespaceName, input.ArtifactFolder)
		Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
		wcp.WaitForNamespaceReady(wcpClient, newNamespaceName)

		vmClassInfo, err := wcpClient.GetVMClassInfo(vmClassName)
		Expect(err).ToNot(HaveOccurred())
		Expect(vmClassInfo.Namespaces).ShouldNot(ContainElement(input.WCPNamespaceName))
		expectedCondition := metav1.Condition{
			Type:    vmopv1a2.VirtualMachineConditionClassReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NotFound",
			Message: fmt.Sprintf("Namespace does not have access to VirtualMachineClass. className: %s, namespace: %s", vmClassName, input.WCPNamespaceName),
		}

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
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		By("Verify that we have a single VirtualMachine with expected condition")
		vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, expectedCondition)

		clusterProxy.DeleteWCPNamespace(newNamespaceCtx)
		// VM class should be deleted as it has no workload associations.
		vmservice.VerifyVMClassDeletion(wcpClient, vmClassName)
	})

	It("Should Create Expected Resources For a Single Virtual Machine with Instance Storage", func() {
		// Instance storage can't be tested on kind cluster
		skipper.SkipUnlessInfraIs(config.InfraConfig.InfraName, "wcp")

		// We don't need to check framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.VDS)
		// as Instance Storage FSS is disabled on VDS
		if !instanceStorageFssEnabled {
			Skip("Instance Storage FSS is not enabled")
		}

		isVsanDEnabled, err := vcenter.IsVSANDEnabledCluster(ctx, vCenterClient, clusterProxy.GetKubeconfigPath())
		Expect(err).ShouldNot(HaveOccurred())
		if !isVsanDEnabled {
			Skip("Cluster is not VSAND enabled")
		}

		storageProfileName := "gc-e2e-vsand-profile"
		var vSANDDStoragePolicyIDInt any
		vSANDDStoragePolicyID, err := vcenter.GetOrCreateVsanDirectStoragePolicyID(ctx, vCenterClient, storageProfileName)
		vSANDDStoragePolicyIDInt = vSANDDStoragePolicyID
		Expect(err).ShouldNot(HaveOccurred())
		Expect(vSANDDStoragePolicyID).ShouldNot(BeEmpty())

		// Create a new namespace here to avoid overwriting the existing namespace also used by other specs.
		tmpNamespaceName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))
		vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
		clIDs := []string{vmserviceCLID}
		vmClassNames := []string{clusterResources.VMClassName}
		vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
		tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, tmpNamespaceName, input.ArtifactFolder)
		Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace %s", tmpNamespaceName)
		wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

		nsDetails, err := wcpClient.GetNamespace(tmpNamespaceName)
		Expect(err).ShouldNot(HaveOccurred())

		storageSpec := []wcp.StorageSpec{
			{Policy: vSANDDStoragePolicyID, Limit: 1024 * 100},
			{Policy: nsDetails.VMStorageSpec[0].Policy, Limit: 1024 * 100},
		}
		Expect(wcpClient.SetNamespaceStorageSpecs(tmpNamespaceName, storageSpec)).Should(Succeed())

		wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

		Expect(vmservice.EnsureVMClassPresent(wcpClient, vmservice.VMClassInstanceStorage, vSANDDStoragePolicyIDInt)).To(Succeed())
		Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, vmservice.VMClassInstanceStorage, tmpNamespaceName)).To(Succeed())

		vmiName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get VMI name in namespace: %s", tmpNamespaceName)

		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        tmpNamespaceName,
			Name:             vmName,
			ImageName:        vmiName,
			VMClassName:      vmservice.VMClassInstanceStorage,
			StorageClassName: clusterResources.StorageClassName,
			ResourcePolicy:   clusterResources.VMResourcePolicyName,
			PowerState:       "PoweredOn",
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

		vmoperator.WaitForVirtualMachineInstanceStorageAnnotations(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

		By("Verify that instance volumes are attached to virtual machine as expected")
		var vmcInfo *vmopv1a2.VirtualMachineClass
		// Depending on namespacedVMClassFSS, return namespaced VM Class or cluster scoped VM Class
		vmcInfo, err = utils.GetVirtualMachineClass(ctx, svClusterClient, vmservice.VMClassInstanceStorage, tmpNamespaceName, namespacedVMClassFSSEnabled)

		Expect(err).NotTo(HaveOccurred())
		vmcISVols := vmcInfo.Spec.Hardware.InstanceStorage.Volumes

		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, tmpNamespaceName, vmName)
		vmdetails, err := wcpClient.GetVirtualMachine(vmMoid)
		Expect(err).NotTo(HaveOccurred())
		e2eframework.Logf("%v", vmdetails)

		// TODO validate desired state in a separate fn
		var findInVMCISVols = func(vmDiskCapacity int64) bool {
			for _, vol := range vmcISVols {
				if vol.Size.Value() == vmDiskCapacity {
					return true
				}
			}
			return false
		}
		var vmInstanceDisksCount int
		for _, deviceInfo := range vmdetails.Disks {
			// Skip vsanDatastore
			// TODO find vsand datastore using DS type rather than pattern matching
			if !strings.Contains(deviceInfo.Backing.VMDKFile, "vSAND_") {
				continue
			}
			Expect(findInVMCISVols(deviceInfo.Capacity)).Should(BeTrue())
			vmInstanceDisksCount++
		}

		Expect(vmInstanceDisksCount).Should(BeEquivalentTo(len(vmcISVols)))
	})

	It("Should create expected resources for a VirtualMachine deployed from ISO", func() {
		skipper.SkipUnlessInfraIs(config.InfraConfig.InfraName, "wcp")
		if !isoSupportFSSEnabled {
			Skip("ISO Support FSS is not enabled")
		}

		if os.Getenv("RUN_CANONICAL_TEST") == "true" {
			Skip("These tests will be skipped for Canonical OVA testing.")
		}

		By("Get the ISO-type image CR name")
		isoImageDisplayName := "ubuntu-24.04-live-server-amd64"
		isoImageName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, isoImageDisplayName)
		Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", input.WCPNamespaceName)

		By("Create a VM with CD-ROM attached and backed by the ISO-type image")
		vmParameters := manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			GuestID:          "ubuntu64Guest",
			Cdrom: []manifestbuilders.Cdrom{
				{
					Name:              "cdrom1",
					ImageName:         isoImageName,
					ImageKind:         "VirtualMachineImage",
					Connected:         true,
					AllowGuestControl: true,
				},
			},
		}
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM with CD-ROM:\n %s", string(vmYaml))
		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

		By("Verifying network provider CR has an IP assigned for the VM")
		netInfo := vmoperator.WaitForVMNetworkProviderInfo(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		Expect(netInfo).NotTo(BeEmpty(), "VM network info from provider is empty")

		By(fmt.Sprintf("Verifying VM status field has the expected network config info: %+v", netInfo))
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Status.Network).NotTo(BeNil())
			g.Expect(vm.Status.Network.Config).NotTo(BeNil())

			// Check the DNS field is populated with nameservers.
			dns := vm.Status.Network.Config.DNS
			g.Expect(dns).NotTo(BeNil())
			g.Expect(dns.Nameservers).NotTo(BeEmpty())

			// Check the Interface field is populated and matches the network provider info.
			interfaces := vm.Status.Network.Config.Interfaces
			g.Expect(interfaces).To(HaveLen(len(netInfo)))
			for i, ifc := range interfaces {
				vmIP := ifc.IP
				g.Expect(vmIP.Addresses).NotTo(BeEmpty())
				// Parse the VM CIDR notation to compare both the IP and subnet mask.
				cidr := vmIP.Addresses[0]
				ip, ipNet, err := net.ParseCIDR(cidr)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ip.String()).To(Equal(netInfo[i].IPv4))
				subnetMask := net.IP(ipNet.Mask).String()
				g.Expect(subnetMask).To(Equal(netInfo[i].SubnetMask))
				// Check the Gateway field matches either the IPv4 or IPv6 gateway.
				g.Expect(netInfo[i].Gateway).To(Or(Equal(vmIP.Gateway4), Equal(vmIP.Gateway6)))
			}
		}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed())

		By("Verifying VM has the expected CD-ROM device")
		vmMoid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoid}
		cdrom := verifyCdromConnectionState(ctx, vmMoRef, propCollector, true, true)
		Expect(cdrom.Backing).NotTo(BeNil())

		By("Verifying VM's CD-ROM has expected backing file from the specified ISO-type image's content library item")
		//nolint:staticcheck // VirtualMachineYaml (v1alpha3) still exposes deprecated Cdrom for this spec path.
		clItemName := strings.Replace(vmParameters.Cdrom[0].ImageName, "vmi", "clitem", 1)
		clItem := imgregv1a1.ContentLibraryItem{}
		Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Name: clItemName, Namespace: input.WCPNamespaceName}, &clItem)).To(Succeed())
		Expect(clItem.Status.FileInfo).NotTo(BeEmpty())
		cdromFileName := cdrom.Backing.(*types.VirtualCdromIsoBackingInfo).FileName
		Expect(cdromFileName).To(Equal(clItem.Status.FileInfo[0].StorageURI))

		By("Verifying CD-ROM can be disconnected from the VM")
		//nolint:staticcheck // VirtualMachineYaml (v1alpha3) still exposes deprecated Cdrom for this spec path.
		vmParameters.Cdrom[0].Connected = false
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply updated VM YAML with CD-ROM disconnected %s", string(vmYaml))
		verifyCdromConnectionState(ctx, vmMoRef, propCollector, false, true)

		By("Verifying CD-ROM can be reconnected to the VM with allowGuestControl disabled")
		//nolint:staticcheck // VirtualMachineYaml (v1alpha3) still exposes deprecated Cdrom for this spec path.
		vmParameters.Cdrom[0].Connected = true
		//nolint:staticcheck
		vmParameters.Cdrom[0].AllowGuestControl = false
		vmYaml = manifestbuilders.GetVirtualMachineYamlA3(vmParameters)
		Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply updated VM YAML with CD-ROM reconnected %s", string(vmYaml))
		verifyCdromConnectionState(ctx, vmMoRef, propCollector, true, false)
	})

	Context("IaaS Policies", func() {

		/*
			This test validates the IaaS Compute Policies feature for VMs.

			Setup:
			1. Create 4 host and VM tags and assign the host tags to the host running the test VM.
			2. Create 4 compute policies, each linked to one tag pair.
			3. Create 4 infra policies from those compute policies:
				- Mandatory (match all): Always enforced on all VMs of the namespace where this policy is assigned.
				- Mandatory (match by label): Applied if the VM has the label matched.
				- Optional (matchGuestID): Applied if the VM explicitly specifies and match the guest ID.
				- Optional (matchWorkloadLabel): Applied if the VM explicitly specifies and match the labels.
			4. Create a Supervisor namespace with all 4 infra policies assigned.

			Test Scenarios:
			1. Existing VM (created before policy assignment to namespace):
				-> Receives only the mandatory policy (match all) and its tag.

			2. New VM with explicitly specified optional policy (matchGuestID):
				-> Receives both the mandatory policy (match all) and the specified policy (matchGuestID), along with their corresponding tags.

			3. Update VM labels and policies to use match-by-label:
				-> Mandatory policy: retained
				-> Mandatory policy (match by label): added
				-> Guest ID-matched policy: removed
				-> Label-matched policy: added
				-> Tags updated accordingly

			4. Update VM labels to not match the policies and remove the explicit optional policy:
				-> Mandatory policy: retained
				-> Mandatory policy (match by label): removed
				-> Guest ID-matched policy: removed
				-> Label-matched policy: removed
				-> Tags updated accordingly
		*/

		var (
			tagManager                      *tags.Manager
			tmpNamespaceName                string
			mandatoryPolicyNameMatchAll     string
			mandatoryPolicyNameMatchByLabel string
			opPolicyNameMatchByGuestID      string
			opPolicyNameMatchByLabel        string
			matchLabel                      map[string]string
			policyNameToVMTagID             map[string]string
		)

		BeforeEach(func() {
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.IaaSComputePoliciesCapabilityName)

			By("Creating a tag manager to verify actual tag assignment")
			restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
			Expect(err).NotTo(HaveOccurred(), "failed to create rest client")
			tagManager = tags.NewManager(restClient)

			By("Creating a new Supervisor namespace")
			tmpNamespaceName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))
			vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, wcpClient)
			clIDs := []string{vmserviceCLID}
			vmClassNames := []string{clusterResources.VMClassName}
			vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
			tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, tmpNamespaceName, input.ArtifactFolder)
			Expect(err).NotTo(HaveOccurred(), "failed to create wcp namespace")
			wcp.WaitForNamespaceReady(wcpClient, tmpNamespaceName)

			By("Deploying a VM without any explicit policies")
			vmiName, err := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
			Expect(err).NotTo(HaveOccurred(), "failed to get the VMI name in namespace %q", tmpNamespaceName)
			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        tmpNamespaceName,
				Name:             vmName,
				ImageName:        vmiName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				ResourcePolicy:   clusterResources.VMResourcePolicyName,
				PowerState:       "PoweredOn",
			}
			vmYaml = manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
			e2eframework.Logf("VM YAML without spec.policies:\n%s", string(vmYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

			By("Creating a new tag category")
			tagCategoryName := fmt.Sprintf("tag-category-%s", capiutil.RandomString(4))
			tagCategoryID, err := input.WCPClient.CreateTagCategory(tagCategoryName, "test-tag-category")
			Expect(err).NotTo(HaveOccurred(), "failed to create tag category")
			Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")

			By("Creating 4 new tag pairs (host tag and VM tag) under the tag category")
			newTagIDPairs := make([][2]string, 4)
			for i := range newTagIDPairs {
				newHostTagID, err := input.WCPClient.CreateTag(fmt.Sprintf("host-tag-%s", capiutil.RandomString(4)), "test-tag", tagCategoryID)
				Expect(err).NotTo(HaveOccurred(), "failed to create host tag")
				Expect(newHostTagID).NotTo(BeEmpty(), "host tag ID should be returned")
				newVMTagID, err := input.WCPClient.CreateTag(fmt.Sprintf("vm-tag-%s", capiutil.RandomString(4)), "test-tag", tagCategoryID)
				Expect(err).NotTo(HaveOccurred(), "failed to create VM tag")
				Expect(newVMTagID).NotTo(BeEmpty(), "VM tag ID should be returned")
				newTagIDPairs[i] = [2]string{newHostTagID, newVMTagID}
			}

			By("Getting the host ID of the deployed VM")
			vmMoID := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, tmpNamespaceName, vmName)
			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}
			var vmMO mo.VirtualMachine
			Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"runtime.host"}, &vmMO)).To(Succeed())
			hostID := vmMO.Runtime.Host.Value
			Expect(hostID).NotTo(BeEmpty(), "host ID should be present")

			By("Assigning the previously created host tag IDs to the host")
			hostTagIDs := make([]string, len(newTagIDPairs))
			for i, tagIDPair := range newTagIDPairs {
				hostTagIDs[i] = tagIDPair[0]
			}
			Expect(input.WCPClient.AssignTagsToHost(hostTagIDs, hostID)).To(Succeed(), "failed to assign tags to host")

			By("Creating 4 compute policies with different tag ID pairs")
			vmTagIDToCPID := make(map[string]string, len(newTagIDPairs))
			for i, tagIDPair := range newTagIDPairs {
				// Cannot use tagID in policy name because it's too long.
				policyName := fmt.Sprintf("%s-%d", capiutil.RandomString(4), i)
				cpSpec := wcp.ComputePolicySpec{
					Name:        policyName,
					Description: policyName,
					HostTagID:   tagIDPair[0],
					VMTagID:     tagIDPair[1],
					Capability:  wcp.ComputePolicyCapabilityVMHostAffinity,
				}
				cpID, err := input.WCPClient.CreateComputePolicy(cpSpec)
				Expect(err).NotTo(HaveOccurred(), "failed to create compute policy")
				Expect(cpID).NotTo(BeEmpty(), "compute policy ID should be returned")
				vmTagIDToCPID[tagIDPair[1]] = cpID
			}

			policyNameToVMTagID = make(map[string]string, len(newTagIDPairs))

			By("Creating a mandatory infra policy from the compute policy with the 1st tag ID")
			tagIDIndex := 0
			mandatoryInfraPolicySpec := wcp.InfraPolicySpec{
				Name:            fmt.Sprintf("mandatory-%s-%d", capiutil.RandomString(4), tagIDIndex),
				Description:     fmt.Sprintf("mandatory infra policy for tag %s", newTagIDPairs[tagIDIndex]),
				ComputePolicyID: vmTagIDToCPID[newTagIDPairs[tagIDIndex][1]],
				EnforcementMode: wcp.InfraPolicyEnforcementModeMandatory,
			}
			Expect(input.WCPClient.CreateInfraPolicy(mandatoryInfraPolicySpec)).To(Succeed(), "failed to create mandatory infra policy")
			mandatoryPolicyNameMatchAll = mandatoryInfraPolicySpec.Name
			policyNameToVMTagID[mandatoryPolicyNameMatchAll] = newTagIDPairs[tagIDIndex][1]
			tagIDIndex++

			By("Creating an optional infra policy with match guest ID value from the compute policy with the 2nd tag ID")
			optionalInfraPolicyMatchByGuestIDSpec := wcp.InfraPolicySpec{
				Name:              fmt.Sprintf("optional-match-by-guest-id-%s-%d", capiutil.RandomString(4), tagIDIndex),
				Description:       fmt.Sprintf("optional infra policy for tag %s", newTagIDPairs[tagIDIndex]),
				ComputePolicyID:   vmTagIDToCPID[newTagIDPairs[tagIDIndex][1]],
				EnforcementMode:   wcp.InfraPolicyEnforcementModeOptional,
				MatchGuestIDValue: linuxImageGuestID,
			}
			Expect(input.WCPClient.CreateInfraPolicy(optionalInfraPolicyMatchByGuestIDSpec)).To(Succeed(), "failed to create optional infra policy match by guest ID")
			opPolicyNameMatchByGuestID = optionalInfraPolicyMatchByGuestIDSpec.Name
			policyNameToVMTagID[opPolicyNameMatchByGuestID] = newTagIDPairs[tagIDIndex][1]
			tagIDIndex++

			By("Creating an optional infra policy with match workload label from the compute policy with the 3rd tag ID")
			matchLabel = map[string]string{"test-label": "test-value"}
			optionalInfraPolicyMatchWorkloadLabelSpec := wcp.InfraPolicySpec{
				Name:               fmt.Sprintf("optional-match-by-label-%s-%d", capiutil.RandomString(4), tagIDIndex),
				Description:        fmt.Sprintf("optional infra policy for tag %s", newTagIDPairs[tagIDIndex]),
				ComputePolicyID:    vmTagIDToCPID[newTagIDPairs[tagIDIndex][1]],
				EnforcementMode:    wcp.InfraPolicyEnforcementModeOptional,
				MatchWorkloadLabel: matchLabel,
			}
			Expect(input.WCPClient.CreateInfraPolicy(optionalInfraPolicyMatchWorkloadLabelSpec)).To(Succeed(), "failed to create optional infra policy match by label")
			opPolicyNameMatchByLabel = optionalInfraPolicyMatchWorkloadLabelSpec.Name
			policyNameToVMTagID[opPolicyNameMatchByLabel] = newTagIDPairs[tagIDIndex][1]
			tagIDIndex++

			By("Creating a mandatory infra policy with match by label from the compute policy with the 4th tag ID")
			mandatoryInfraPolicyMatchByLabelSpec := wcp.InfraPolicySpec{
				Name:               fmt.Sprintf("mandatory-match-by-label-%s-%d", capiutil.RandomString(4), tagIDIndex),
				Description:        fmt.Sprintf("mandatory infra policy for tag %s", newTagIDPairs[tagIDIndex]),
				ComputePolicyID:    vmTagIDToCPID[newTagIDPairs[tagIDIndex][1]],
				EnforcementMode:    wcp.InfraPolicyEnforcementModeMandatory,
				MatchWorkloadLabel: matchLabel,
			}
			Expect(input.WCPClient.CreateInfraPolicy(mandatoryInfraPolicyMatchByLabelSpec)).To(Succeed(), "failed to create mandatory infra policy match by label")
			mandatoryPolicyNameMatchByLabel = mandatoryInfraPolicyMatchByLabelSpec.Name
			policyNameToVMTagID[mandatoryPolicyNameMatchByLabel] = newTagIDPairs[tagIDIndex][1]
			tagIDIndex++

			Expect(tagIDIndex).To(BeEquivalentTo(len(newTagIDPairs)), "expected to create %d policies", len(newTagIDPairs))

			By("Assigning all 4 policies to the namespace")
			Expect(input.WCPClient.UpdateNamespaceWithInfraPolicies(tmpNamespaceName, mandatoryPolicyNameMatchAll, opPolicyNameMatchByGuestID, opPolicyNameMatchByLabel, mandatoryPolicyNameMatchByLabel)).To(Succeed(), "failed to assign policies to namespace")
		})

		It("Should apply policies and tags correctly during VM creation and update", func() {
			By("Verifying the existing VM has only the mandatory policy (match all) and its tag applied")
			expectedPolicyNames := []string{mandatoryPolicyNameMatchAll}
			vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vmName, policyNameToVMTagID, expectedPolicyNames)

			By("Creating a new VM with explicitly specified the optional policy match by guest ID")
			vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        tmpNamespaceName,
				Name:             vmName,
				GuestID:          linuxImageGuestID,
				ImageName:        linuxImageDisplayName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				ResourcePolicy:   clusterResources.VMResourcePolicyName,
				PowerState:       "PoweredOn",
				Policies: []vmopv1a5.PolicySpec{
					{
						APIVersion: "vsphere.policy.vmware.com/v1alpha1",
						Kind:       "ComputePolicy",
						Name:       opPolicyNameMatchByGuestID,
					},
				},
			}
			vmYaml = manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
			e2eframework.Logf("Creating VM with explicit spec.policies:\n%s", string(vmYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM with explicitly specified optional policy")
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)

			By("Verifying the new VM has both mandatory (match all) and optional policies (match by guest ID) and their tags applied")
			expectedPolicyNames = []string{mandatoryPolicyNameMatchAll, opPolicyNameMatchByGuestID}
			vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vmName, policyNameToVMTagID, expectedPolicyNames)

			By("Updating the VM's labels and policies to match by label")
			vmParameters.Labels = matchLabel
			vmParameters.Policies = []vmopv1a5.PolicySpec{
				{
					APIVersion: "vsphere.policy.vmware.com/v1alpha1",
					Kind:       "ComputePolicy",
					Name:       opPolicyNameMatchByLabel,
				},
			}
			vmYaml = manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
			e2eframework.Logf("Updating VM's labels and spec.policies:\n%s", string(vmYaml))
			Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply updated VM YAML")

			By("Verifying the VM has both mandatory policies (match all and match by label) and optional policies (match by label) and their tags applied")
			expectedPolicyNames = []string{mandatoryPolicyNameMatchAll, mandatoryPolicyNameMatchByLabel, opPolicyNameMatchByLabel}
			vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vmName, policyNameToVMTagID, expectedPolicyNames)

			By("Updating the VM's label to not match the policies and remove the explicit optional policy")
			vmObj, err := utils.GetVirtualMachineA5(ctx, svClusterClient, tmpNamespaceName, vmName)
			Expect(err).NotTo(HaveOccurred(), "failed to get existing VM")
			vmLabels := vmObj.Labels
			for key := range matchLabel {
				Expect(vmLabels).To(HaveKey(key))
				delete(vmLabels, key)
			}
			vmParameters.Labels = vmLabels
			vmParameters.Policies = []vmopv1a5.PolicySpec{}
			vmYaml = manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
			e2eframework.Logf("Updating VM's labels:\n%s", string(vmYaml))
			Expect(clusterProxy.ApplyWithArgs(ctx, vmYaml)).To(Succeed(), "failed to apply updated VM YAML")

			By("Verifying the VM has only the mandatory policy (match all) and its tag applied")
			expectedPolicyNames = []string{mandatoryPolicyNameMatchAll}
			vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vmName, policyNameToVMTagID, expectedPolicyNames)
		})
	})
}

func verifyCdromConnectionState(
	ctx context.Context,
	vmMoRef types.ManagedObjectReference,
	propCollector *property.Collector,
	connected, allowGuestControl bool) *types.VirtualDevice {

	var (
		moVM  mo.VirtualMachine
		cdrom *types.VirtualDevice
	)

	Eventually(func(g Gomega) {
		g.Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.hardware.device"}, &moVM)).To(Succeed())
		virtualDevices := object.VirtualDeviceList(moVM.Config.Hardware.Device)
		curCdroms := virtualDevices.SelectByType((*types.VirtualCdrom)(nil))
		g.Expect(len(curCdroms)).To(Equal(1))
		cdrom = curCdroms[0].GetVirtualDevice()
		g.Expect(cdrom.Connectable.Connected).To(Equal(connected))
		g.Expect(cdrom.Connectable.AllowGuestControl).To(Equal(allowGuestControl))
	}, 1*time.Minute, 5*time.Second).Should(Succeed(), "VM CD-ROM did not have expected connection state")

	return cdrom
}
