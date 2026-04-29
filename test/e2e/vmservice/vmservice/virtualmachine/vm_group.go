// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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

type VMGroupSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

func VMGroupSpec(ctx context.Context, inputGetter func() VMGroupSpecInput) {
	const (
		specName = "vm-group"
		vmKind   = "VirtualMachine"
		vmgKind  = "VirtualMachineGroup"
	)

	var (
		input            VMGroupSpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		vCenterClient    *vim25.Client
		clusterResources *e2eConfig.Resources

		vmgRootYaml   []byte
		vmgRootName   string
		vmgChildName  string
		vm1Name       string
		vm2Name       string
		vm3Name       string
		vm4Name       string
		vmMemberNames []string

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

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetClientSet(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMGroupsCapabilityName)

		svClusterClient = clusterProxy.GetClient()
		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

		vmgRootYaml = nil
		vmMemberNames = []string{}
		vmgRootName = fmt.Sprintf("%s-%s-root", specName, capiutil.RandomString(4))
		vmgChildName = fmt.Sprintf("%s-child", vmgRootName)
		vm1Name = fmt.Sprintf("%s-vm1", vmgRootName)
		vm2Name = fmt.Sprintf("%s-vm2", vmgRootName)
		vm3Name = fmt.Sprintf("%s-vm3", vmgRootName)
		vm4Name = fmt.Sprintf("%s-vm4", vmgRootName)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmgRootName, vmgKind)
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmgChildName, vmgKind)

			for _, vmName := range vmMemberNames {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, vmKind)
			}
		}

		// Delete the root VirtualMachineGroup if created.
		if len(vmgRootYaml) > 0 {
			By("Deleting the root VirtualMachineGroup")
			Expect(clusterProxy.DeleteWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to delete VirtualMachineGroup")
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName)

			By("Waiting for all group members to be deleted automatically due to owner reference to the root group")
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName)

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}
		}

		if vCenterClient != nil {
			vcenter.LogoutVimClient(vCenterClient)
		}
	})

	Context("Flat group", func() {
		It("Should create and manage a VirtualMachineGroup with VM-kind members only", Label("smoke"), func() {
			By("Creating a VirtualMachineGroup with 3 VMs and power on delays")

			vmGroupParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						// No power on delay for the first boot order.
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm1Name,
							},
						},
					},
					{
						PowerOnDelay: "30s",
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm2Name,
							},
						},
					},
					{
						PowerOnDelay: "1m",
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm3Name,
							},
						},
					},
				},
			}
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmGroupParameters)
			e2eframework.Logf("VirtualMachineGroup YAML:\n%s", string(vmgRootYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed())

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}

			By("Creating VMs with initially powered off state and spec.GroupName pointing to the VirtualMachineGroup")

			for _, vmName := range vmMemberNames {
				vmParameters := manifestbuilders.VirtualMachineYaml{
					Namespace:        input.WCPNamespaceName,
					Name:             vmName,
					GroupName:        vmgRootName,
					ImageName:        linuxImageDisplayName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					PowerState:       "PoweredOff",
				}
				vmYaml := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
				Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM %q:\n %s", vmName, string(vmYaml))
			}

			By("Waiting for all VMs to exist")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}

			By("Waiting for all VMs to have group linked condition set to true")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1a5.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			By("Verifying VirtualMachineGroup has Ready condition set to true")

			readyTrueCondition := metav1.Condition{
				Type:   vmopv1a5.ReadyConditionType,
				Status: metav1.ConditionTrue,
			}
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOn")

			vmGroupParameters.PowerState = "PoweredOn"
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmGroupParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update VirtualMachineGroup power state:\n %s", string(vmgRootYaml))

			By("Waiting for all VMs to be powered on")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
			}

			By("Verifying VirtualMachineGroup has Ready condition set to true after power on")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Verifying VirtualMachineGroup has expected member statuses")

			for _, bootOrder := range vmGroupParameters.BootOrder {
				for _, m := range bootOrder.Members {
					ms, err := utils.GetVirtualMachineGroupMemberStatus(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, m.Name, m.Kind)
					Expect(err).ToNot(HaveOccurred())
					Expect(ms.PowerState).ToNot(BeNil(), "member %s/%s power state status is nil", m.Name, m.Kind)
					Expect(*ms.PowerState).To(BeEquivalentTo("PoweredOn"))
					// Just check the placement status is not nil here.
					// The exact placement info will be verified in affinity/anti-affinity tests.
					Expect(ms.Placement).ToNot(BeNil(), "member %s/%s placement status is nil", m.Name, m.Kind)

					// Verify all expected member conditions are set to true.
					expectedConditionTypes := []string{
						vmopv1a5.VirtualMachineGroupMemberConditionGroupLinked,
						vmopv1a5.VirtualMachineGroupMemberConditionPowerStateSynced,
						vmopv1a5.VirtualMachineGroupMemberConditionPlacementReady,
					}

					Expect(ms.Conditions).To(HaveLen(3)) // GroupLinked, PowerStateSynced, PlacementReady

					for _, c := range ms.Conditions {
						Expect(c.Type).To(BeElementOf(expectedConditionTypes))
						Expect(c.Status).To(Equal(metav1.ConditionTrue))
					}
				}
			}

			By("Verifying group member VMs were powered on with specified delays", func() {
				vcClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vcClient)

				propCollector := property.DefaultCollector(vcClient)

				vm1Moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				vm2Moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				vm3Moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)

				vm1MoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm1Moid}
				vm2MoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm2Moid}
				vm3MoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm3Moid}

				var vm1MO, vm2MO, vm3MO mo.VirtualMachine
				Expect(propCollector.RetrieveOne(ctx, vm1MoRef, []string{"runtime.powerState", "runtime.bootTime"}, &vm1MO)).To(Succeed())
				Expect(propCollector.RetrieveOne(ctx, vm2MoRef, []string{"runtime.powerState", "runtime.bootTime"}, &vm2MO)).To(Succeed())
				Expect(propCollector.RetrieveOne(ctx, vm3MoRef, []string{"runtime.powerState", "runtime.bootTime"}, &vm3MO)).To(Succeed())

				// Verify all VMs are powered on.
				Expect(vm1MO.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))
				Expect(vm2MO.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))
				Expect(vm3MO.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))

				// Verify boot times to be within the expected delays.
				// VM1 should boot with no delay (VM1's boot order has no powerOnDelay).
				// VM2 should boot ~30 seconds after VM1 is powered on (VM2's boot order has powerOnDelay set to 30s).
				// VM3 should boot ~1 minute after VM2 is powered on (VM3's boot order has powerOnDelay set to 1m).
				Expect(vm1MO.Runtime.BootTime).ToNot(BeNil())
				Expect(vm2MO.Runtime.BootTime).ToNot(BeNil())
				Expect(vm3MO.Runtime.BootTime).ToNot(BeNil())
				vm1BootTime := *vm1MO.Runtime.BootTime
				vm2BootTime := *vm2MO.Runtime.BootTime
				vm3BootTime := *vm3MO.Runtime.BootTime
				vm2DelayFromVM1 := vm2BootTime.Sub(vm1BootTime)
				vm3DelayFromVM2 := vm3BootTime.Sub(vm2BootTime)
				By(fmt.Sprintf("VM boot timing: VM1: %v, VM2: %v (delay from VM1: %v), VM3: %v (delay from VM2: %v)",
					vm1BootTime, vm2BootTime, vm2DelayFromVM1, vm3BootTime, vm3DelayFromVM2))

				// Allow some tolerance (±10 seconds) for VM controller to reconcile and actually power on the VMs.
				Expect(vm2DelayFromVM1).To(BeNumerically(">=", 20*time.Second), "VM2 boot delay should be at least 20s from VM1")
				Expect(vm2DelayFromVM1).To(BeNumerically("<=", 40*time.Second), "VM2 boot delay should be at most 40s from VM1")
				Expect(vm3DelayFromVM2).To(BeNumerically(">=", 50*time.Second), "VM3 boot delay should be at least 50s from VM2")
				Expect(vm3DelayFromVM2).To(BeNumerically("<=", 70*time.Second), "VM3 boot delay should be at most 70s from VM2")
			})

			By("Creating a new standalone VM4 with spec.groupName unset and spec.powerState set to PoweredOn")

			vm4Parameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vm4Name,
				PowerState:       "PoweredOn",
				ImageName:        linuxImageDisplayName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
			}
			vm4Yaml := manifestbuilders.GetVirtualMachineYamlA5(vm4Parameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vm4Yaml)).To(Succeed(), "failed to create VM %q:\n %s", vm4Name, string(vm4Yaml))

			By("Waiting for VM4 to be created and powered on")
			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vm4Name)
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm4Name, "PoweredOn")

			By("Setting VM4 spec.groupName to the VirtualMachineGroup")

			vm4Parameters.GroupName = vmgRootName
			vm4Yaml = manifestbuilders.GetVirtualMachineYamlA5(vm4Parameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vm4Yaml)).To(Succeed(), "failed to update VM %q group name:\n %s", vm4Name, string(vm4Yaml))

			By("Updating VirtualMachineGroup to adopt the existing VM4")

			vmGroupParameters.BootOrder = append(vmGroupParameters.BootOrder, manifestbuilders.BootOrder{
				Members: []vmopv1a5.GroupMember{
					{
						Kind: vmKind,
						Name: vm4Name,
					},
				},
			})
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmGroupParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update VirtualMachineGroup members:\n %s", string(vmgRootYaml))

			vmMemberNames = append(vmMemberNames, vm4Name)

			By("Changing VirtualMachineGroup spec.powerState to PoweredOff and spec.powerOffMode to Hard")

			vmGroupParameters.PowerState = "PoweredOff"
			vmGroupParameters.PowerOffMode = "Hard"
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmGroupParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update VirtualMachineGroup power state:\n %s", string(vmgRootYaml))

			By("Waiting for all VMs to be powered off")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			}

			By("Verifying VirtualMachineGroup has Ready condition set to true after power off")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Changing VM1 spec.powerState directly to PoweredOn")

			vm1Parameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vm1Name,
				PowerState:       "PoweredOn",
				GroupName:        vmgRootName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				ImageName:        linuxImageDisplayName,
			}
			vm1Yaml := manifestbuilders.GetVirtualMachineYamlA5(vm1Parameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vm1Yaml)).To(Succeed(), "failed to update VM %q power state:\n %s", vm1Name, string(vm1Yaml))

			By("Waiting for VM1 to be powered on")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOn")

			By("Verifying VirtualMachineGroup member status has PowerStateSynced condition set to false for VM1")

			powerStateSyncedFalseCondition := metav1.Condition{
				Type:   vmopv1a5.VirtualMachineGroupMemberConditionPowerStateSynced,
				Status: metav1.ConditionFalse,
				Reason: "NotSynced",
			}
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind, powerStateSyncedFalseCondition)

			By("Verifying VirtualMachineGroup member status has the actual power state recorded for VM1")

			vm1MemberStatus, err := utils.GetVirtualMachineGroupMemberStatus(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm1MemberStatus.PowerState).ToNot(BeNil(), "VM1 member status power state is nil")
			Expect(*vm1MemberStatus.PowerState).To(BeEquivalentTo("PoweredOn"))

			By("Setting VirtualMachineGroup nextForcePowerStateSyncTime to 'now'")

			vmGroupParameters.NextForcePowerStateSyncTime = "now"
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmGroupParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update VirtualMachineGroup force sync time:\n %s", string(vmgRootYaml))

			By("Verifying VirtualMachineGroup has Ready condition set to true after force power state sync")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Verifying power state sync forces VM1 back to PoweredOff to match group state")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOff")
		})
	})

	Context("Nested group", func() {
		It("Should create and manage a VirtualMachineGroup with both VMG-kind and VM-kind members", func() {
			By("Creating a root VirtualMachineGroup with VM-1 and a child group with power on delays")

			vmgRootParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm1Name,
							},
						},
					},
					{
						PowerOnDelay: "30s",
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmgKind,
								Name: vmgChildName,
							},
						},
					},
				},
			}
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgRootParameters)
			e2eframework.Logf("Root VirtualMachineGroup YAML:\n%s", string(vmgRootYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed())

			vmMemberNames = []string{vm1Name}

			By("Creating a child VirtualMachineGroup with VM-2 and power on delay")

			vmgChildParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgChildName,
				GroupName: vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm2Name,
							},
						},
						PowerOnDelay: "1m",
					},
				},
			}
			vmgChildYaml := manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgChildParameters)
			e2eframework.Logf("Child VirtualMachineGroup YAML:\n%s", string(vmgChildYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmgChildYaml)).To(Succeed())

			vmMemberNames = append(vmMemberNames, vm2Name)

			By("Creating VM1 in the root group with powered off")

			vm1Parameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vm1Name,
				GroupName:        vmgRootName,
				ImageName:        linuxImageDisplayName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				PowerState:       "PoweredOff",
			}
			vm1Yaml := manifestbuilders.GetVirtualMachineYamlA5(vm1Parameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vm1Yaml)).To(Succeed(), "failed to create vm1:\n %s", string(vm1Yaml))

			By("Creating VM2 in the child group")

			vm2Parameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vm2Name,
				GroupName:        vmgChildName,
				ImageName:        linuxImageDisplayName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				PowerState:       "PoweredOff",
			}
			vm2Yaml := manifestbuilders.GetVirtualMachineYamlA5(vm2Parameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vm2Yaml)).To(Succeed(), "failed to create vm2:\n %s", string(vm2Yaml))

			By("Waiting for all VMs to exist")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}

			By("Waiting for all group members to have group linked condition set to true")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1a5.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, groupLinkedTrueCondition)

			By("Verifying both root and child VirtualMachineGroups are ready")

			readyTrueCondition := metav1.Condition{
				Type:   vmopv1a5.ReadyConditionType,
				Status: metav1.ConditionTrue,
			}
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, readyTrueCondition)
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Setting root VirtualMachineGroup spec.powerState to PoweredOn")

			vmgRootParameters.PowerState = "PoweredOn"
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgRootParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update root VirtualMachineGroup power state:\n %s", string(vmgRootYaml))

			By("Waiting for all VMs to be powered on")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
			}

			By("Verifying group member VMs were powered on with specified delays", func() {
				vcClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
				defer vcenter.LogoutVimClient(vcClient)

				propCollector := property.DefaultCollector(vcClient)

				vm1Moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				vm2Moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

				vm1MoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm1Moid}
				vm2MoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm2Moid}

				var vm1MO, vm2MO mo.VirtualMachine
				Expect(propCollector.RetrieveOne(ctx, vm1MoRef, []string{"runtime.powerState", "runtime.bootTime"}, &vm1MO)).To(Succeed())
				Expect(propCollector.RetrieveOne(ctx, vm2MoRef, []string{"runtime.powerState", "runtime.bootTime"}, &vm2MO)).To(Succeed())

				// Verify all VMs are powered on.
				Expect(vm1MO.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))
				Expect(vm2MO.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))

				// Verify boot times to be within the expected delays.
				// VM1 should boot with no delay (VM1's boot order has no powerOnDelay).
				// VM2 should boot ~90 seconds after VM1 is powered on (VM2's boot order has 1m powerOnDelay in child group, which also has 30s powerOnDelay in root group).
				Expect(vm1MO.Runtime.BootTime).ToNot(BeNil())
				Expect(vm2MO.Runtime.BootTime).ToNot(BeNil())
				vm1BootTime := *vm1MO.Runtime.BootTime
				vm2BootTime := *vm2MO.Runtime.BootTime
				vm2DelayFromVM1 := vm2BootTime.Sub(vm1BootTime)
				By(fmt.Sprintf("VM boot timing: VM1: %v, VM2: %v (delay from VM1: %v)",
					vm1BootTime, vm2BootTime, vm2DelayFromVM1))

				// Allow some tolerance (±30 seconds from 90 seconds) for VM controller to reconcile nested groups and actually power on the VMs.
				Expect(vm2DelayFromVM1).To(BeNumerically(">=", 60*time.Second), "VM2 boot delay should be at least 60s from VM1")
				Expect(vm2DelayFromVM1).To(BeNumerically("<=", 120*time.Second), "VM2 boot delay should be at most 120s from VM1")
			})

			By("Changing root VirtualMachineGroup spec.powerState to PoweredOff")

			vmgRootParameters.PowerState = "PoweredOff"
			vmgRootParameters.PowerOffMode = "Hard"
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgRootParameters)
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update root VirtualMachineGroup power state:\n %s", string(vmgRootYaml))

			By("Waiting for all VMs to be powered off")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			}

			By("Verifying both VirtualMachineGroups are ready after power off")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, readyTrueCondition)
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)
		})
	})

	Context("Group placement with affinity and anti-affinity", func() {
		var (
			tmpNamespaceName string
			tmpNamespaceCtx  wcpframework.NamespaceContext

			createVMWithAffinityAndAntiAffinityFunc = func(vmName, affinityTier string, antiAffinityTiers []string) {
				GinkgoHelper()

				vmParameters := manifestbuilders.VirtualMachineYaml{
					Namespace:        tmpNamespaceName,
					Name:             vmName,
					GroupName:        vmgRootName,
					Labels:           map[string]string{"tier": affinityTier},
					ImageName:        linuxImageDisplayName,
					VMClassName:      clusterResources.VMClassName,
					StorageClassName: clusterResources.StorageClassName,
					Affinity: &vmopv1a5.AffinitySpec{
						VMAffinity: &vmopv1a5.VMAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"tier": affinityTier,
										},
									},
									TopologyKey: "topology.kubernetes.io/zone",
								},
							},
						},
						VMAntiAffinity: &vmopv1a5.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "tier",
												Operator: metav1.LabelSelectorOpIn,
												Values:   antiAffinityTiers,
											},
										},
									},
									TopologyKey: "topology.kubernetes.io/zone",
								},
							},
						},
					},
				}
				vmYAML := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
				e2eframework.Logf("VM YAML:\n%s", string(vmYAML))
				Expect(clusterProxy.ApplyWithArgs(ctx, vmYAML)).To(Succeed(), "failed to create vm %s:\n %s", vmName, string(vmYAML))
			}

			getVMZoneFunc = func(vmName string) string {
				GinkgoHelper()

				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
				Expect(err).ToNot(HaveOccurred())
				Expect(vm.Status.Zone).ToNot(BeEmpty())

				return vm.Status.Zone
			}
		)

		BeforeEach(func() {
			skipper.SkipUnlessStretchSupervisorIsEnabled()
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMPlacementPoliciesCapabilityName)

			By("Verifying there are at least 3 zones bound with the Supervisor")

			supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, config.InfraConfig.KubeconfigPath)
			Expect(supervisorID).ToNot(BeEmpty(), "Supervisor ID should not be empty")
			zoneList, err := clusterProxy.GetZonesBoundWithSupervisor(supervisorID)
			Expect(err).ToNot(HaveOccurred(), "failed to get zones bound with Supervisor")
			Expect(len(zoneList.Zones)).To(BeNumerically(">=", 3))

			By("Creating a temporary namespace")

			vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, input.WCPClient)
			clIDs := []string{vmserviceCLID}
			vmClassNames := []string{clusterResources.VMClassName}
			vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
			tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)), input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
			Expect(tmpNamespaceCtx.GetNamespace()).ToNot(BeNil(), "namespace should not be nil")
			tmpNamespaceName = tmpNamespaceCtx.GetNamespace().Name
			wcp.WaitForNamespaceReady(input.WCPClient, tmpNamespaceName)

			By("Ensuring the Linux image is available in the temp namespace")

			_, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
			Expect(err).NotTo(HaveOccurred(), "failed to get VMI by display name %q in namespace %q", linuxImageDisplayName, tmpNamespaceName)

			By("Binding all zones to the temporary namespace")

			namespaceZones, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), tmpNamespaceName)
			Expect(err).NotTo(HaveOccurred())

			boundZones := make(map[string]struct{}, len(namespaceZones.Items))
			for _, zone := range namespaceZones.Items {
				boundZones[zone.Name] = struct{}{}
			}

			unboundZones := []string{}

			for _, zone := range zoneList.Zones {
				if _, ok := boundZones[zone.Zone]; !ok {
					unboundZones = append(unboundZones, zone.Zone)
				}
			}

			if len(unboundZones) > 0 {
				_, err = clusterProxy.UpdateNamespaceWithZones(ctx, tmpNamespaceName, unboundZones, svClusterClient)
				Expect(err).ToNot(HaveOccurred(), "failed to update namespace with Zones")
			}
		})

		AfterEach(func() {
			if tmpNamespaceName != "" {
				clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
				tmpNamespaceName = ""
			}
		})

		It("Should create VMs with expected zonal affinity and anti-affinity placements in the temporary namespace", func() {
			By("Creating a VirtualMachineGroup with 4 VM-kind members")

			vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: tmpNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1a5.GroupMember{
							{
								Kind: vmKind,
								Name: vm1Name,
							},
							{
								Kind: vmKind,
								Name: vm2Name,
							},
							{
								Kind: vmKind,
								Name: vm3Name,
							},
							{
								Kind: vmKind,
								Name: vm4Name,
							},
						},
					},
				},
			}
			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgParameters)
			e2eframework.Logf("VirtualMachineGroup YAML:\n%s", string(vmgRootYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed())

			By(fmt.Sprintf("Creating VM1 (%s) with tier=1 label, affinity to tier=1, and anti-affinity to tier=2 and tier=3", vm1Name))
			createVMWithAffinityAndAntiAffinityFunc(vm1Name, "1", []string{"2", "3"})
			By(fmt.Sprintf("Creating VM2 (%s) with tier=2 label, affinity to tier=2, and anti-affinity to tier=1 and tier=3", vm2Name))
			createVMWithAffinityAndAntiAffinityFunc(vm2Name, "2", []string{"1", "3"})
			By(fmt.Sprintf("Creating VM3 (%s) with tier=3 label, affinity to tier=3, and anti-affinity to tier=1 and tier=2", vm3Name))
			createVMWithAffinityAndAntiAffinityFunc(vm3Name, "3", []string{"1", "2"})
			By(fmt.Sprintf("Creating VM4 (%s) with tier=1 label, affinity to tier=1, and anti-affinity to tier=2, tier=3", vm4Name))
			createVMWithAffinityAndAntiAffinityFunc(vm4Name, "1", []string{"2", "3"})

			By("Waiting for all VMs to be created")

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
			}

			By("Getting VM placement zones")

			vm1Zone := getVMZoneFunc(vm1Name)
			vm2Zone := getVMZoneFunc(vm2Name)
			vm3Zone := getVMZoneFunc(vm3Name)
			vm4Zone := getVMZoneFunc(vm4Name)

			By("Verifying VM1 and VM4 are in the same zone")
			Expect(vm1Zone).To(Equal(vm4Zone))

			By("Verifying VM1, VM2, and VM3 are in different zones respectively")
			Expect(vm1Zone).ToNot(Equal(vm2Zone))
			Expect(vm1Zone).ToNot(Equal(vm3Zone))
			Expect(vm2Zone).ToNot(Equal(vm3Zone))
		})

		When("VMs have both AF/AAF and IaaS Policies applied", func() {
			var (
				tagManager        *tags.Manager
				tagIDs            []string
				policyNames       []string
				policyNameToTagID map[string]string
			)

			BeforeEach(func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.IaaSComputePoliciesCapabilityName)

				By("Creating a tag manager to verify actual tag assignment")

				restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
				Expect(err).NotTo(HaveOccurred(), "failed to create rest client")

				tagManager = tags.NewManager(restClient)

				By("Creating a new tag category")

				tagCategoryName := fmt.Sprintf("tag-category-%s", capiutil.RandomString(4))
				tagCategoryID, err := input.WCPClient.CreateTagCategory(tagCategoryName, "test-tag-category")
				Expect(err).NotTo(HaveOccurred(), "failed to create tag category")
				Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")

				By("Creating 3 new tags under the tag category")

				tagIDs = make([]string, 3)
				for i := range tagIDs {
					tagIDs[i], err = input.WCPClient.CreateTag(fmt.Sprintf("tag-%s-%d", capiutil.RandomString(4), i+1), "test-tag", tagCategoryID)
					Expect(err).NotTo(HaveOccurred(), "failed to create tag")
					Expect(tagIDs[i]).NotTo(BeEmpty(), "tag ID should be returned")
				}

				By("Getting all 3 Supervisor hosts in SSV testbed")

				hostIDs, err := input.WCPClient.ListHostIDs()
				Expect(err).NotTo(HaveOccurred(), "failed to list host IDs")
				Expect(len(hostIDs)).To(BeNumerically(">=", 3))
				// SSV testbed has 3 Supervisor hosts and 1 infra host.
				supervisorHostIDs := hostIDs[:3]

				By("Assigning one tag to each Supervisor host")

				for i, hostID := range supervisorHostIDs {
					Expect(input.WCPClient.AssignTagsToHost(tagIDs[i:i+1], hostID)).To(Succeed(), "failed to assign tag %s to host %s", tagIDs[i], hostID)
				}

				By("Creating 3 compute policies with different tag IDs")

				tagIDToCPID := make(map[string]string, len(tagIDs))
				for i, tagID := range tagIDs {
					// Cannot use tagID in policy name because it's too long.
					policyName := fmt.Sprintf("%s-%d", capiutil.RandomString(4), i+1)
					cpSpec := wcp.ComputePolicySpec{
						Name:        policyName,
						Description: policyName,
						HostTagID:   tagID,
						VMTagID:     tagID,
						Capability:  wcp.ComputePolicyCapabilityVMHostAffinity,
					}
					cpID, err := input.WCPClient.CreateComputePolicy(cpSpec)
					Expect(err).NotTo(HaveOccurred(), "failed to create compute policy")
					Expect(cpID).NotTo(BeEmpty(), "compute policy ID should be returned")
					tagIDToCPID[tagID] = cpID
				}

				policyNames = make([]string, len(tagIDs))
				policyNameToTagID = make(map[string]string, len(tagIDs))

				By("Creating 3 mandatory infra policies matched by label (tier=1, 2, 3) from the above compute policies")

				for i, tagID := range tagIDs {
					policyName := fmt.Sprintf("%s-%d", capiutil.RandomString(4), i+1)
					infraPolicySpec := wcp.InfraPolicySpec{
						Name:               policyName,
						Description:        policyName,
						ComputePolicyID:    tagIDToCPID[tagID],
						EnforcementMode:    wcp.InfraPolicyEnforcementModeMandatory,
						MatchWorkloadLabel: map[string]string{"tier": fmt.Sprintf("%d", i+1)},
					}
					Expect(input.WCPClient.CreateInfraPolicy(infraPolicySpec)).To(Succeed(), "failed to create mandatory infra policy matched by label")

					policyNames[i] = policyName
					policyNameToTagID[policyName] = tagID
				}

				By("Assigning all 3 mandatory infra policies to the namespace")
				Expect(input.WCPClient.UpdateNamespaceWithInfraPolicies(tmpNamespaceName, policyNames...)).To(Succeed(), "failed to assign policies to namespace")
			})

			It("Should create VMs with expected zonal AF/AAF and IaaS Policies & Tags applied", func() {
				By("Creating a VirtualMachineGroup with 4 VM-kind members")

				vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
					Namespace: tmpNamespaceName,
					Name:      vmgRootName,
					BootOrder: []manifestbuilders.BootOrder{
						{
							Members: []vmopv1a5.GroupMember{
								{
									Kind: vmKind,
									Name: vm1Name,
								},
								{
									Kind: vmKind,
									Name: vm2Name,
								},
								{
									Kind: vmKind,
									Name: vm3Name,
								},
								{
									Kind: vmKind,
									Name: vm4Name,
								},
							},
						},
					},
				}
				vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgParameters)
				e2eframework.Logf("VirtualMachineGroup YAML:\n%s", string(vmgRootYaml))
				Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed())

				By(fmt.Sprintf("Creating VM1 (%s) with tier=1 label, affinity to tier=1, and anti-affinity to tier=2 and tier=3", vm1Name))
				createVMWithAffinityAndAntiAffinityFunc(vm1Name, "1", []string{"2", "3"})
				By(fmt.Sprintf("Creating VM2 (%s) with tier=2 label, affinity to tier=2, and anti-affinity to tier=1 and tier=3", vm2Name))
				createVMWithAffinityAndAntiAffinityFunc(vm2Name, "2", []string{"1", "3"})
				By(fmt.Sprintf("Creating VM3 (%s) with tier=3 label, affinity to tier=3, and anti-affinity to tier=1 and tier=2", vm3Name))
				createVMWithAffinityAndAntiAffinityFunc(vm3Name, "3", []string{"1", "2"})
				By(fmt.Sprintf("Creating VM4 (%s) with tier=1 label, affinity to tier=1, and anti-affinity to tier=2, tier=3", vm4Name))
				createVMWithAffinityAndAntiAffinityFunc(vm4Name, "1", []string{"2", "3"})

				By("Waiting for all VMs to be created")

				vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
				for _, vmName := range vmMemberNames {
					vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
				}

				By("Getting VM placement zones")

				vm1Zone := getVMZoneFunc(vm1Name)
				vm2Zone := getVMZoneFunc(vm2Name)
				vm3Zone := getVMZoneFunc(vm3Name)
				vm4Zone := getVMZoneFunc(vm4Name)

				By("Verifying VM1 and VM4 are in the same zone")
				Expect(vm1Zone).To(Equal(vm4Zone))

				By("Verifying VM1, VM2, and VM3 are in different zones respectively")
				Expect(vm1Zone).ToNot(Equal(vm2Zone))
				Expect(vm1Zone).ToNot(Equal(vm3Zone))
				Expect(vm2Zone).ToNot(Equal(vm3Zone))

				By("Verifying the VMs have the expected tags and policies assigned")
				// VM1 with tier=1 label should have the 1st mandatory policy applied.
				vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm1Name, policyNameToTagID, policyNames[:1])
				// VM2 with tier=2 label should have the 2nd mandatory policy applied.
				vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm2Name, policyNameToTagID, policyNames[1:2])
				// VM3 with tier=3 label should have the 3rd mandatory policy applied.
				vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm3Name, policyNameToTagID, policyNames[2:3])
				// VM4 with tier=1 label should have the 1st mandatory policy applied.
				vmservice.VerifyVMTagsAndPolicyAssignment(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm4Name, policyNameToTagID, policyNames[:1])
			})

		})
	})

	Context("Group placement with affinity and anti-affinity at host topology", func() {
		const (
			requiredDuringSchedulingPreferredDuringExecution  = "requiredDuringSchedulingPreferredDuringExecution"
			preferredDuringSchedulingPreferredDuringExecution = "preferredDuringSchedulingPreferredDuringExecution"
		)

		var (
			tmpNamespaceName string
			tmpNamespaceCtx  wcpframework.NamespaceContext
		)

		// getVMHostFromVmodlFunc retrieves the host moref value from vSphere directly.
		// Use this before power-on, when vm.Status.Host is not yet populated.
		getVMHostFromVmodlFunc := func(vmName string) string {
			GinkgoHelper()

			moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, tmpNamespaceName, vmName)
			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: moid}

			propCollector := property.DefaultCollector(vCenterClient)
			var vmMO mo.VirtualMachine
			Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"runtime.host"}, &vmMO)).To(Succeed())
			Expect(vmMO.Runtime.Host).ToNot(BeNil())

			return vmMO.Runtime.Host.Value
		}

		// getVMHostFunc retrieves the host from vm.Status.Host.
		// Use this after power-on, once the operator has updated the status.
		getVMHostFunc := func(vmName string) string {
			GinkgoHelper()

			var host string
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vm.Status.Host).ToNot(BeEmpty())
				host = vm.Status.Host
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

			return host
		}

		powerOnVMFunc := func(vmName string) {
			GinkgoHelper()

			vm, err := utils.GetVirtualMachineA5(ctx, svClusterClient, tmpNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine %s", vmName)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
				To(Succeed(), "failed to patch powerState for vm %s", vmName)
		}

		createHostVMWithAffinityAndAntiAffinityFunc := func(vmName, affinityType string, labelValues, affinityTiers, antiAffinityTiers []string) {
			GinkgoHelper()

			labels := make(map[string]string)
			for _, v := range labelValues {
				labels["tier"] = v
			}

			var affinityLabelSelector *vmopv1a5.VMAffinitySpec
			if len(affinityTiers) > 0 {
				terms := []vmopv1a5.VMAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "tier",
									Operator: metav1.LabelSelectorOpIn,
									Values:   affinityTiers,
								},
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				}
				if affinityType == requiredDuringSchedulingPreferredDuringExecution {
					affinityLabelSelector = &vmopv1a5.VMAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: terms,
					}
				} else if affinityType == preferredDuringSchedulingPreferredDuringExecution {
					affinityLabelSelector = &vmopv1a5.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: terms,
					}
				}
			}

			var antiAffinityLabelSelector *vmopv1a5.VMAntiAffinitySpec
			if len(antiAffinityTiers) > 0 {
				terms := []vmopv1a5.VMAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "tier",
									Operator: metav1.LabelSelectorOpIn,
									Values:   antiAffinityTiers,
								},
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				}
				if affinityType == requiredDuringSchedulingPreferredDuringExecution {
					antiAffinityLabelSelector = &vmopv1a5.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: terms,
					}
				} else if affinityType == preferredDuringSchedulingPreferredDuringExecution {
					antiAffinityLabelSelector = &vmopv1a5.VMAntiAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: terms,
					}
				}
			}

			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        tmpNamespaceName,
				Name:             vmName,
				GroupName:        vmgRootName,
				Labels:           labels,
				ImageName:        linuxImageDisplayName,
				VMClassName:      clusterResources.VMClassName,
				StorageClassName: clusterResources.StorageClassName,
				PowerState:       string(vmopv1a5.VirtualMachinePowerStateOff),
				Affinity: &vmopv1a5.AffinitySpec{
					VMAffinity:     affinityLabelSelector,
					VMAntiAffinity: antiAffinityLabelSelector,
				},
			}
			vmYAML := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
			e2eframework.Logf("VM YAML:\n%s", string(vmYAML))
			Expect(clusterProxy.ApplyWithArgs(ctx, vmYAML)).To(Succeed(), "failed to create vm %s:\n %s", vmName, string(vmYAML))
		}

		getVMPolicyComplianceFunc := func(policyID, vmName string) wcp.VMPolicyComplianceStatus {
			GinkgoHelper()

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.UniqueID).ToNot(BeEmpty())

			status, err := input.WCPClient.GetVMPolicyCompliance(policyID, vm.Status.UniqueID)
			Expect(err).ToNot(HaveOccurred())

			return status
		}

		verifyAffinity := func(vmHosts map[string]string, affinedVms []string) {
			if len(affinedVms) <= 1 {
				return
			}

			vm1Host, ok := vmHosts[affinedVms[0]]
			Expect(ok).To(BeTrue())
			Expect(vm1Host).ToNot(BeEmpty())

			for i := 1; i < len(affinedVms); i++ {
				vmNHost, ok := vmHosts[affinedVms[i]]
				Expect(ok).To(BeTrue())
				Expect(vmNHost).To(Equal(vm1Host))
			}
		}

		// all hosts of Vms must be different from each other.
		verifyAntiAffinity := func(vmHosts map[string]string, antiAffinedVms []string) {
			if len(antiAffinedVms) <= 1 {
				return
			}

			seen := make(map[string]string, len(antiAffinedVms))
			for _, vmName := range antiAffinedVms {
				host, ok := vmHosts[vmName]
				Expect(ok).To(BeTrue())
				Expect(host).ToNot(BeEmpty())
				Expect(seen).ToNot(HaveKey(host), "VMs %s and %s are both on host %s, but anti-affinity requires them to be on different hosts", seen[host], vmName, host)
				seen[host] = vmName
			}
		}

		runVmVmAffinityAtHostTopoTest := func(affinityType string, affinedVms, antiAffinedVms []string) {
			vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: tmpNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1a5.GroupMember{},
					},
				},
			}
			for _, v := range vmMemberNames {
				vmgParameters.BootOrder[0].Members = append(vmgParameters.BootOrder[0].Members,
					vmopv1a5.GroupMember{Kind: vmKind, Name: v})
			}

			vmgRootYaml = manifestbuilders.GetVirtualMachineGroupWithBootOrderYaml(vmgParameters)
			e2eframework.Logf("VirtualMachineGroup YAML:\n%s", string(vmgRootYaml))
			Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed())

			affinedSet := make(map[string]bool, len(affinedVms))
			for _, v := range affinedVms {
				affinedSet[v] = true
			}
			antiAffinedSet := make(map[string]bool, len(antiAffinedVms))
			for _, v := range antiAffinedVms {
				antiAffinedSet[v] = true
			}

			for _, v := range vmMemberNames {
				labelTier := "1"
				if antiAffinedSet[v] {
					labelTier = "2"
				}

				var affinityTiers, antiAffinityTiers []string
				if affinedSet[v] {
					affinityTiers = []string{"1"}
				}
				if antiAffinedSet[v] {
					antiAffinityTiers = []string{"2"}
				}
				createHostVMWithAffinityAndAntiAffinityFunc(v, affinityType, []string{labelTier}, affinityTiers, antiAffinityTiers)
			}

			By("Waiting for all VMs to be created in vSphere (powered off)")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vmName)
			}

			By("Verifying host placement before power-on via vSphere vmodl as vm.status.host may not be populated yet.")
			vmHosts := make(map[string]string)
			for _, v := range vmMemberNames {
				vmHosts[v] = getVMHostFromVmodlFunc(v)
			}

			// Check placement enforcement for requiredDuringScheduling
			// This is skipped for preferredDuringScheduling as that is non-deterministic.
			if affinityType == requiredDuringSchedulingPreferredDuringExecution {
				By("Verifying all VMs are placed on different hosts before poweron, i.e. placement is as expected")
				verifyAffinity(vmHosts, affinedVms)
				verifyAntiAffinity(vmHosts, antiAffinedVms)
			}

			By("Powering on all VMs")
			for _, vmName := range vmMemberNames {
				powerOnVMFunc(vmName)
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, "PoweredOn")
			}

			By("Verifying vm.Status.Host is populated for all VMs after power-on")
			for _, vmName := range vmMemberNames {
				Expect(getVMHostFunc(vmName)).ToNot(BeEmpty())
			}

			By("Verifying each VM is configured with the policy with the correct tags.")

			// Build the set of tier labels that are actually in use so we can look up
			// each policy ID once and then check each VM against its own policy.
			tierLabels := make(map[string]string) // tier label -> policy ID
			if len(affinedVms) > 0 {
				tierLabels["tier:1"] = ""
			}
			if len(antiAffinedVms) > 0 {
				tierLabels["tier:2"] = ""
			}

			for tagLabel := range tierLabels {
				Eventually(func(g Gomega) {
					entries, err := input.WCPClient.ListComputePolicyTagUsage(tmpNamespaceName, tagLabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(entries).ToNot(BeEmpty())
					g.Expect(entries[0].Policy).ToNot(BeEmpty())
					tierLabels[tagLabel] = entries[0].Policy
				}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed(),
					"timed out waiting for compute policy tag usage entry for tag %s in namespace %s", tagLabel, tmpNamespaceName)
			}

			// vmPolicyID maps each VM to the policy ID it should be checked against.
			vmPolicyID := make(map[string]string, len(vmMemberNames))
			for _, vmName := range affinedVms {
				vmPolicyID[vmName] = tierLabels["tier:1"]
			}
			for _, vmName := range antiAffinedVms {
				vmPolicyID[vmName] = tierLabels["tier:2"]
			}

			for _, vmName := range vmMemberNames {
				policyID := vmPolicyID[vmName]
				Eventually(func() string {
					status := getVMPolicyComplianceFunc(policyID, vmName)
					return status.Status
				}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).
					Should(Or(Equal("COMPLIANT"), Equal("NOT_COMPLIANT")), "expected VM %s to have a compliance status for policy %s", vmName, policyID)
			}
		}

		BeforeEach(func() {
			skipper.SkipUnlessStretchSupervisorIsEnabled()
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMPlacementPoliciesCapabilityName)
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMAffinityDuringExecutionCapabilityName)

			supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, config.InfraConfig.KubeconfigPath)
			Expect(supervisorID).ToNot(BeEmpty(), "Supervisor ID should not be empty")
			zoneList, err := clusterProxy.GetZonesBoundWithSupervisor(supervisorID)
			Expect(err).ToNot(HaveOccurred(), "failed to get zones bound with Supervisor")

			By("Creating a temporary namespace")

			vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, input.WCPClient)
			clIDs := []string{vmserviceCLID}
			vmClassNames := []string{clusterResources.VMClassName}
			vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
			tmpNamespaceCtx, err = clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, clusterResources.WorkerStorageClassName, fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)), input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
			Expect(tmpNamespaceCtx.GetNamespace()).ToNot(BeNil(), "namespace should not be nil")
			tmpNamespaceName = tmpNamespaceCtx.GetNamespace().Name
			wcp.WaitForNamespaceReady(input.WCPClient, tmpNamespaceName)

			By("Ensuring the Linux image is available in the temp namespace")

			_, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)
			Expect(err).NotTo(HaveOccurred(), "failed to get VMI by display name %q in namespace %q", linuxImageDisplayName, tmpNamespaceName)

			By("Binding all zones to the temporary namespace")

			namespaceZones, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), tmpNamespaceName)
			Expect(err).NotTo(HaveOccurred())

			boundZones := make(map[string]struct{}, len(namespaceZones.Items))
			for _, zone := range namespaceZones.Items {
				boundZones[zone.Name] = struct{}{}
			}

			unboundZones := []string{}

			for _, zone := range zoneList.Zones {
				if _, ok := boundZones[zone.Zone]; !ok {
					unboundZones = append(unboundZones, zone.Zone)
				}
			}

			if len(unboundZones) > 0 {
				_, err = clusterProxy.UpdateNamespaceWithZones(ctx, tmpNamespaceName, unboundZones, svClusterClient)
				Expect(err).ToNot(HaveOccurred(), "failed to update namespace with Zones")
			}

			zoneHostInfos, err := utils.GetHostsPerZone(ctx, input.ClusterProxy.GetClient(), input.ClusterProxy.GetKubeconfigPath())
			Expect(err).NotTo(HaveOccurred(), "failed to list zones with hosts")

			minHostsRequirementSatisfied := false
			for _, zoneHostInfo := range zoneHostInfos {
				if len(zoneHostInfo.HostIDs) >= 3 {
					minHostsRequirementSatisfied = true
					break
				}
			}

			Expect(minHostsRequirementSatisfied).To(BeTrue(), "at least 1 zone with 3 hosts expected")
		})

		AfterEach(func() {
			if tmpNamespaceName != "" {
				clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
				tmpNamespaceName = ""
			}
		})

		When("Group placement with affinity and anti-affinity at host topology", func() {
			It("Should create 4 VMs with preferred host AF with each other", func() {
				By("Creating a VirtualMachineGroup with 4 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
				runVmVmAffinityAtHostTopoTest(preferredDuringSchedulingPreferredDuringExecution,
					[]string{vm1Name, vm2Name, vm3Name, vm4Name},
					[]string{})

			})

			It("Should create 3 VMs with preferred host AAF with each other", func() {
				By("Creating a VirtualMachineGroup with 3 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
				runVmVmAffinityAtHostTopoTest(preferredDuringSchedulingPreferredDuringExecution,
					[]string{},
					[]string{vm1Name, vm2Name, vm3Name})

			})

			It("Should create VMs with preferred host AF (2vms) & AAF (2vms)", func() {
				By("Creating a VirtualMachineGroup with 4 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
				runVmVmAffinityAtHostTopoTest(preferredDuringSchedulingPreferredDuringExecution,
					[]string{vm1Name, vm2Name},
					[]string{vm3Name, vm4Name})
			})

			It("Should create 4 VMs with required host AF with each other", func() {
				By("Creating a VirtualMachineGroup with 4 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
				runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingPreferredDuringExecution,
					[]string{vm1Name, vm2Name, vm3Name, vm4Name},
					[]string{})

			})

			It("Should create 3 VMs with required host AAF with each other", func() {
				By("Creating a VirtualMachineGroup with 3 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
				runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingPreferredDuringExecution,
					[]string{},
					[]string{vm1Name, vm2Name, vm3Name})

			})

			It("Should create VMs with required host AF (2vms) & AAF (2vms)", func() {
				By("Creating a VirtualMachineGroup with 4 VM-kind members")
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
				runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingPreferredDuringExecution,
					[]string{vm1Name, vm2Name},
					[]string{vm3Name, vm4Name})
			})
		})
	})
}
