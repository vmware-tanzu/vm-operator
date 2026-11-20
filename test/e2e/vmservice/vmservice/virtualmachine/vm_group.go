// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
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

		rootGroup     ctrlclient.Object
		vmgRootName   string
		vmgChildName  string
		vm1Name       string
		vm2Name       string
		vm3Name       string
		vm4Name       string
		vmMemberNames []string

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
		skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMGroupsCapabilityName)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		svClusterClient = clusterProxy.GetClient()
		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() { vcenter.LogoutVimClient(vCenterClient) })

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)

		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		rootGroup = nil
		vmMemberNames = []string{}
		vmgRootName = fmt.Sprintf("%s-%s-root", specName, capiutil.RandomString(4))
		vmgChildName = fmt.Sprintf("%s-child", vmgRootName)
		vm1Name = fmt.Sprintf("%s-vm1", vmgRootName)
		vm2Name = fmt.Sprintf("%s-vm2", vmgRootName)
		vm3Name = fmt.Sprintf("%s-vm3", vmgRootName)
		vm4Name = fmt.Sprintf("%s-vm4", vmgRootName)

		deferCleanupVirtualMachineGroup(
			config,
			svClusterClient,
			clusterProxy.GetKubeconfigPath(),
			input.WCPNamespaceName,
			vmgRootName,
			vmgChildName,
			vmgKind,
			vmKind,
			&rootGroup,
			&vmMemberNames,
		)
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
						Members: []vmopv1.GroupMember{
							{
								Kind: vmKind,
								Name: vm1Name,
							},
						},
					},
					{
						PowerOnDelay: "30s",
						Members: []vmopv1.GroupMember{
							{
								Kind: vmKind,
								Name: vm2Name,
							},
						},
					},
					{
						PowerOnDelay: "1m",
						Members: []vmopv1.GroupMember{
							{
								Kind: vmKind,
								Name: vm3Name,
							},
						},
					},
				},
			}
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmGroupParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}

			By("Creating VMs with initially powered off state and spec.GroupName pointing to the VirtualMachineGroup")

			for _, vmName := range vmMemberNames {
				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: input.WCPNamespaceName,
						Name:      vmName,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						GroupName:    vmgRootName,
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", vm.Name)
				deferDeleteVM(svClusterClient, vm)
			}

			By("Waiting for all VMs to exist")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}

			By("Waiting for all VMs to have group linked condition set to true")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			By("Verifying VirtualMachineGroup has Ready condition set to true")

			readyTrueCondition := metav1.Condition{
				Type:   vmopv1.ReadyConditionType,
				Status: metav1.ConditionTrue,
			}
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOn")

			powerOnSince := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOn
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying each VM powered on once, after the delay of its boot order group")

			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, powerOnSince)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, powerOnSince)
			vm3OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, powerOnSince)
			expectGap(vm2OnTime.Sub(vm1OnTime), 30*time.Second, "vm2 on after vm1")
			expectGap(vm3OnTime.Sub(vm2OnTime), time.Minute, "vm3 on after vm2")

			By("Verifying VirtualMachineGroup has Ready condition set to true after power on")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Verifying VirtualMachineGroup has expected member statuses")

			for _, bootOrder := range vmGroupParameters.BootOrder {
				for _, m := range bootOrder.Members {
					Eventually(func(g Gomega) {
						ms, err := utils.GetVirtualMachineGroupMemberStatus(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, m.Name, m.Kind)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(ms.PowerState).ToNot(BeNil(), "member %s/%s power state status is nil", m.Name, m.Kind)
						g.Expect(*ms.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						// Just check the placement status is not nil here.
						// The exact placement info will be verified in affinity/anti-affinity tests.
						g.Expect(ms.Placement).ToNot(BeNil(), "member %s/%s placement status is nil", m.Name, m.Kind)

						// Verify all expected member conditions are set to true.
						expectedConditionTypes := []string{
							vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
							vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
							vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
						}

						g.Expect(ms.Conditions).To(HaveLen(3)) // GroupLinked, PowerStateSynced, PlacementReady

						for _, c := range ms.Conditions {
							g.Expect(c.Type).To(BeElementOf(expectedConditionTypes))
							g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						}
					}, config.GetIntervals("default", "wait-virtual-machine-group-member-status")...).Should(Succeed(), "Timed out waiting for Group Member Status: %s/%s on VirtualMachineGroup: %q", m.Name, m.Kind, vmgRootName)
				}
			}

			By("Creating a new standalone VM4 with spec.groupName unset and spec.powerState set to PoweredOn")

			vm4 := &vmopv1a5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: input.WCPNamespaceName,
					Name:      vm4Name,
				},
				Spec: vmopv1a5.VirtualMachineSpec{
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1a5.VirtualMachinePowerStateOn,
				},
			}
			Expect(svClusterClient.Create(ctx, vm4)).To(Succeed(), "failed to create VirtualMachine %s", vm4.Name)

			By("Waiting for VM4 to be created and powered on")
			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vm4Name)
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm4Name, vmopv1.VirtualMachinePowerStateOn)

			By("Setting VM4 spec.groupName to the VirtualMachineGroup")

			base := vm4.DeepCopy()
			vm4.Spec.GroupName = vmgRootName
			Expect(svClusterClient.Patch(ctx, vm4, ctrlclient.MergeFrom(base))).To(Succeed(), "failed to update VM %q group name", vm4Name)

			By("Updating VirtualMachineGroup to adopt the existing VM4")

			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.BootOrder = append(g.Spec.BootOrder, vmopv1a5.VirtualMachineGroupBootOrderGroup{
					Members: []vmopv1a5.GroupMember{{Kind: vmKind, Name: vm4Name}},
				})
			}, "failed to update VirtualMachineGroup members")

			vmMemberNames = append(vmMemberNames, vm4Name)

			By("Changing VirtualMachineGroup spec.powerState to PoweredOff and spec.powerOffMode to Hard")

			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1a5.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Waiting for all VMs to be powered off")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOff)
			}

			By("Verifying VirtualMachineGroup has Ready condition set to true after power off")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Waiting for the power off request to be closed")
			waitForGroupRequestClosed(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName})

			By("Changing VM1 spec.powerState directly to PoweredOn")

			vmoperator.PowerOnVirtualMachineAndWait(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)

			By("Verifying VirtualMachineGroup member status has PowerStateSynced condition set to false for VM1")

			powerStateSyncedFalseCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				Status: metav1.ConditionFalse,
				Reason: "NotSynced",
			}
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind, powerStateSyncedFalseCondition)

			By("Verifying VirtualMachineGroup member status has the actual power state recorded for VM1")

			vm1MemberStatus, err := utils.GetVirtualMachineGroupMemberStatus(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm1MemberStatus.PowerState).ToNot(BeNil(), "VM1 member status power state is nil")
			Expect(*vm1MemberStatus.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

			By("Setting VirtualMachineGroup nextForcePowerStateSyncTime to 'now'")

			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.NextForcePowerStateSyncTime = "now"
			}, "failed to update VirtualMachineGroup force sync time")

			By("Verifying VirtualMachineGroup has Ready condition set to true after force power state sync")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Verifying power state sync forces VM1 back to PoweredOff to match group state")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, vmopv1.VirtualMachinePowerStateOff)
		})
	})

	Context("Boot order power-on delay", func() {
		It("Should power on each boot order group with its delay counted from the previous group", func() {
			const (
				// The delay is ignored, which is checked below.
				vm1PowerOnDelay = 2 * time.Minute
				vm2PowerOnDelay = 30 * time.Second
				vm3PowerOnDelay = 1 * time.Minute
			)

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}

			By("Creating a VirtualMachineGroup with 3 boot order groups and power on delays")

			vmGroupParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}},
						PowerOnDelay: vm1PowerOnDelay.String(),
					},
					{
						Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
						PowerOnDelay: vm2PowerOnDelay.String(),
					},
					{
						Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}},
						PowerOnDelay: vm3PowerOnDelay.String(),
					},
				},
			}
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmGroupParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			By("Creating powered off VMs with spec.groupName pointing to the VirtualMachineGroup")

			for _, vmName := range vmMemberNames {
				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: input.WCPNamespaceName,
						Name:      vmName,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						GroupName:    vmgRootName,
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", vm.Name)
				deferDeleteVM(svClusterClient, vm)
			}

			By("Waiting for all VMs to be created in vCenter and linked to the group")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			By("Setting VirtualMachineGroup spec.powerState to PoweredOn")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOn
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying each VM powered on once, with the delay of its boot order group counted from the previous one")

			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since)
			vm3OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since)

			expectGap(vm2OnTime.Sub(vm1OnTime), vm2PowerOnDelay, "vm2 on after vm1")
			expectGap(vm3OnTime.Sub(vm2OnTime), vm3PowerOnDelay, "vm3 on after vm2")
		})

		It("Should power off a member released for power-on when the group is powered off before it powers on", func() {
			// vm2 is released for power-on with a delay, so for this time its
			// desired power state is on and its observed power state is off.
			const vm2PowerOnDelay = time.Minute

			vmMemberNames = []string{vm1Name, vm2Name}

			By("Creating a VirtualMachineGroup with 2 boot order groups")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{
						Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
						PowerOnDelay: vm2PowerOnDelay.String(),
					},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			By("Creating powered off VMs with spec.groupName pointing to the VirtualMachineGroup")

			for _, vmName := range vmMemberNames {
				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: input.WCPNamespaceName,
						Name:      vmName,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						GroupName:    vmgRootName,
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", vm.Name)
				deferDeleteVM(svClusterClient, vm)
			}

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			By("Setting VirtualMachineGroup spec.powerState to PoweredOn")

			groupKey := ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			}, "failed to update VirtualMachineGroup power state")

			By("Waiting for vm2 to be released for power-on while it is still off")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOn")
			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				g.Expect(vm2.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "2m", "1s").Should(Succeed(), "vm2 was not released for power-on")

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff before vm2 powers on")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying the group sets the desired power state of vm2 to PoweredOff")

			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "1m", "1s").Should(Succeed(), "the group did not set vm2 to PoweredOff")

			By("Verifying every VM powers off, and vm2 does not power on after its delay")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)
			Consistently(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, vm2PowerOnDelay+30*time.Second, "5s").Should(Succeed(), "vm2 powered on after the group was powered off")
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
						Members: []vmopv1.GroupMember{
							{
								Kind: vmKind,
								Name: vm1Name,
							},
						},
					},
					{
						PowerOnDelay: "30s",
						Members: []vmopv1.GroupMember{
							{
								Kind: vmgKind,
								Name: vmgChildName,
							},
						},
					},
				},
			}
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgRootParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			vmMemberNames = []string{vm1Name}

			By("Creating a child VirtualMachineGroup with VM-2 and power on delay")

			vmgChildParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgChildName,
				GroupName: vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1.GroupMember{
							{
								Kind: vmKind,
								Name: vm2Name,
							},
						},
						PowerOnDelay: "1m",
					},
				},
			}
			vmgChild, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgChildParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			Expect(svClusterClient.Create(ctx, vmgChild)).To(Succeed(), "failed to create VirtualMachineGroup")

			vmMemberNames = append(vmMemberNames, vm2Name)

			By("Creating VM1 in the root group with powered off")

			vm1 := &vmopv1a5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: input.WCPNamespaceName,
					Name:      vm1Name,
				},
				Spec: vmopv1a5.VirtualMachineSpec{
					GroupName:    vmgRootName,
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
				},
			}
			Expect(svClusterClient.Create(ctx, vm1)).To(Succeed(), "failed to create VirtualMachine %s", vm1.Name)
			deferDeleteVM(svClusterClient, vm1)

			By("Creating VM2 in the child group")

			vm2 := &vmopv1a5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: input.WCPNamespaceName,
					Name:      vm2Name,
				},
				Spec: vmopv1a5.VirtualMachineSpec{
					GroupName:    vmgChildName,
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
				},
			}
			Expect(svClusterClient.Create(ctx, vm2)).To(Succeed(), "failed to create VirtualMachine %s", vm2.Name)
			deferDeleteVM(svClusterClient, vm2)

			By("Waiting for all VMs to exist")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}

			By("Waiting for all group members to have group linked condition set to true")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, groupLinkedTrueCondition)

			By("Verifying both root and child VirtualMachineGroups are ready")

			readyTrueCondition := metav1.Condition{
				Type:   vmopv1.ReadyConditionType,
				Status: metav1.ConditionTrue,
			}
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, readyTrueCondition)
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)

			By("Setting root VirtualMachineGroup spec.powerState to PoweredOn")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOn
			}, "failed to update root VirtualMachineGroup power state")

			By("Verifying VM2 powered on once, after the delay of the child group counted from when VM1 was on")

			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since)
			expectGap(vm2OnTime.Sub(vm1OnTime), 30*time.Second, "VM2 on after VM1")

			By("Changing root VirtualMachineGroup spec.powerState to PoweredOff")

			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1a5.VirtualMachineGroup{}, func(g *vmopv1a5.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1a5.VirtualMachinePowerOpModeHard
			}, "failed to update root VirtualMachineGroup power state")

			By("Waiting for all VMs to be powered off")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOff)
			}

			By("Verifying both VirtualMachineGroups are ready after power off")
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, readyTrueCondition)
			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, readyTrueCondition)
		})
	})

	Context("Group placement with affinity and anti-affinity", func() {
		var (
			tmpNamespaceName    string
			tmpNamespaceVMIName string

			createVMWithAffinityAndAntiAffinityFunc = func(vmName, affinityTier string, antiAffinityTiers []string) {
				GinkgoHelper()

				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: tmpNamespaceName,
						Name:      vmName,
						Labels:    map[string]string{"tier": affinityTier},
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						GroupName:    vmgRootName,
						ImageName:    tmpNamespaceVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
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
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)
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
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMPlacementPoliciesCapabilityName)

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
			tmpNamespaceCtx, err := clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)), input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
			Expect(tmpNamespaceCtx.GetNamespace()).ToNot(BeNil(), "namespace should not be nil")
			tmpNamespaceName = tmpNamespaceCtx.GetNamespace().Name
			DeferCleanup(func() {
				clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
			})

			wcp.WaitForNamespaceReady(input.WCPClient, tmpNamespaceName)
			deferCleanupVirtualMachineGroup(
				config,
				svClusterClient,
				clusterProxy.GetKubeconfigPath(),
				tmpNamespaceName,
				vmgRootName,
				vmgChildName,
				vmgKind,
				vmKind,
				&rootGroup,
				&vmMemberNames,
			)

			By("Ensuring the Linux image is available in the temp namespace")

			tmpNamespaceVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)

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

		It("Should create VMs with expected zonal affinity and anti-affinity placements in the temporary namespace", func() {
			By("Creating a VirtualMachineGroup with 4 VM-kind members")

			vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: tmpNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1.GroupMember{
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
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

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

		When("VMs are pinned to zones via the topology zone label", Label("experimental"), func() {
			var (
				zones       []string
				tinyVMIName string
			)

			BeforeEach(func() {
				skipper.SkipUnlessStretchSupervisorIsEnabled()
				skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMAffinityDuringExecutionCapabilityName)

				namespaceZones, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), tmpNamespaceName)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(namespaceZones.Items)).To(BeNumerically(">=", 3))

				zones = nil
				for _, zone := range namespaceZones.Items {
					zones = append(zones, zone.Name)
				}

				By("Resolving the tiny-core-linux-complex-hw VMI in the temporary namespace")

				tinyVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, "tiny-core-linux-complex-hw")
			})

			It("Should place all members in the zone they are all pinned to", func() {
				By("Creating 3 VMs pinned to the same zone")

				vms := []zonePinnedVM{
					{Name: vm1Name, Zone: zones[0]},
					{Name: vm2Name, Zone: zones[0]},
					{Name: vm3Name, Zone: zones[0]},
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)).To(Equal(zones[0]), "vm %s zone mismatch", vm.Name)
				}
			})

			It("Should place a VM that is not in a group in its pinned zone", func() {
				By("Creating a standalone VM pinned to a zone")

				vms := []zonePinnedVM{{Name: vm1Name, Zone: zones[1]}}
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, "", tinyVMIName, vms)

				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)).To(Equal(zones[1]))
			})

			It("Should place members pinned to the same zone on different hosts with host anti-affinity", func() {
				zone, _ := getZoneWithMinHosts(ctx, clusterProxy, 3)

				By(fmt.Sprintf("Creating 3 VMs pinned to zone %s with required host anti-affinity to each other", zone))

				appLabels := map[string]string{"app": vmgRootName}
				vms := []zonePinnedVM{
					{Name: vm1Name, Zone: zone, Labels: appLabels, Affinity: requiredAntiAffinity(hostTopoKey, appLabels)},
					{Name: vm2Name, Zone: zone, Labels: appLabels, Affinity: requiredAntiAffinity(hostTopoKey, appLabels)},
					{Name: vm3Name, Zone: zone, Labels: appLabels, Affinity: requiredAntiAffinity(hostTopoKey, appLabels)},
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				hosts := map[string]string{}
				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)).To(Equal(zone), "vm %s zone mismatch", vm.Name)

					host := getVMHostFromVmodl(ctx, svClusterClient, vCenterClient, tmpNamespaceName, vm.Name)
					Expect(hosts).ToNot(HaveKey(host), "vms %s and %s are on the same host %s", hosts[host], vm.Name, host)
					hosts[host] = vm.Name
				}
			})
		})

		When("VMs have both AF/AAF and IaaS Policies applied", func() {
			var (
				tagManager        *tags.Manager
				policyNames       []string
				policyNameToTagID map[string]string
			)

			BeforeEach(func() {
				skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.IaaSComputePoliciesCapabilityName)

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

				tagIDs := make([]string, 3)
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
							Members: []vmopv1.GroupMember{
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
				vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgParameters)
				Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
				rootGroup = vmgRoot
				Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

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

				verifyTagsFunc := vmservice.VerifyVMTagsAndPolicyAssignment

				By("Verifying the VMs have the expected tags and policies assigned")
				// The VMs may also have the tags used by the zonal AF/AAF policies attached.
				// VM1 with tier=1 label should have the 1st mandatory policy applied.
				verifyTagsFunc(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm1Name, policyNameToTagID, policyNames[:1])
				// VM2 with tier=2 label should have the 2nd mandatory policy applied.
				verifyTagsFunc(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm2Name, policyNameToTagID, policyNames[1:2])
				// VM3 with tier=3 label should have the 3rd mandatory policy applied.
				verifyTagsFunc(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm3Name, policyNameToTagID, policyNames[2:3])
				// VM4 with tier=1 label should have the 1st mandatory policy applied.
				verifyTagsFunc(ctx, config, svClusterClient, tagManager, tmpNamespaceName, vm4Name, policyNameToTagID, policyNames[:1])
			})

		})
	})

	Context("Group placement with affinity and anti-affinity at host topology", func() {
		const (
			requiredDuringSchedulingPreferredDuringExecution  = "requiredDuringSchedulingPreferredDuringExecution"
			preferredDuringSchedulingPreferredDuringExecution = "preferredDuringSchedulingPreferredDuringExecution"
		)

		var (
			tmpNamespaceName    string
			tmpNamespaceVMIName string

			// topologyKey is the topology key used by the affinity terms of
			// the VMs created by createHostVMWithAffinityAndAntiAffinityFunc.
			topologyKey string
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
				g.Expect(vm.Status.NodeName).ToNot(BeEmpty())
				host = vm.Status.NodeName
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

			return host
		}

		getVMZoneFunc := func(vmName string) string {
			GinkgoHelper()

			var zone string
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vm.Status.Zone).ToNot(BeEmpty())
				zone = vm.Status.Zone

			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

			return zone
		}

		powerOnVMFunc := func(vmName string) {
			GinkgoHelper()

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachine %s", vmName)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
				To(Succeed(), "failed to patch powerState for vm %s", vmName)
		}

		createHostVMWithAffinityAndAntiAffinityFunc := func(vmName, affinityType, label string, affinityTiers, antiAffinityTiers []string) {
			GinkgoHelper()

			labels := make(map[string]string)
			labels["tier"] = label

			var affinityLabelSelector *vmopv1.VMAffinitySpec
			if len(affinityTiers) > 0 {
				terms := []vmopv1.VMAffinityTerm{
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
						TopologyKey: topologyKey,
					},
				}
				switch affinityType {
				case requiredDuringSchedulingPreferredDuringExecution:
					affinityLabelSelector = &vmopv1.VMAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: terms,
					}
				case preferredDuringSchedulingPreferredDuringExecution:
					affinityLabelSelector = &vmopv1.VMAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: terms,
					}
				}
			}

			var antiAffinityLabelSelector *vmopv1.VMAntiAffinitySpec
			if len(antiAffinityTiers) > 0 {
				terms := []vmopv1.VMAffinityTerm{
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
						TopologyKey: topologyKey,
					},
				}
				switch affinityType {
				case requiredDuringSchedulingPreferredDuringExecution:
					antiAffinityLabelSelector = &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingPreferredDuringExecution: terms,
					}
				case preferredDuringSchedulingPreferredDuringExecution:
					antiAffinityLabelSelector = &vmopv1.VMAntiAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: terms,
					}

				}
			}

			vm := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: tmpNamespaceName,
					Name:      vmName,
					Labels:    labels,
				},
				Spec: vmopv1.VirtualMachineSpec{
					GroupName:    vmgRootName,
					ImageName:    tmpNamespaceVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1.VirtualMachinePowerStateOff,
					Affinity: &vmopv1.AffinitySpec{
						VMAffinity:     affinityLabelSelector,
						VMAntiAffinity: antiAffinityLabelSelector,
					},
				},
			}
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)
		}

		verifyAffinity := func(vmHosts map[string]string, affinedVms []string) {
			GinkgoHelper()

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
			GinkgoHelper()

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
			GinkgoHelper()

			vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: tmpNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members: []vmopv1.GroupMember{},
					},
				},
			}
			for _, v := range vmMemberNames {
				vmgParameters.BootOrder[0].Members = append(vmgParameters.BootOrder[0].Members,
					vmopv1.GroupMember{Kind: vmKind, Name: v})
			}

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

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
				createHostVMWithAffinityAndAntiAffinityFunc(v, affinityType, labelTier, affinityTiers, antiAffinityTiers)
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
			// For requiredDuringExecution, placement is verified below, only after
			// the compute policy reports the VMs as compliant.
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
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOn)
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

			tierVMs := make(map[string]string)
			if len(affinedVms) > 0 {
				tierVMs["tier:1"] = affinedVms[0]
			}
			if len(antiAffinedVms) > 0 {
				tierVMs["tier:2"] = antiAffinedVms[0]
			}
			for tagLabel := range tierVMs {
				Eventually(func(g Gomega) {
					policyTag := tagLabel
					entries, err := input.WCPClient.ListComputePolicyTagUsage(tmpNamespaceName, policyTag)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(entries).NotTo(BeEmpty())
					g.Expect(entries[0].Policy).NotTo(BeEmpty())
					tierLabels[tagLabel] = entries[0].Policy
				}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed(),
					"timed out waiting for compute policy tag usage for %s in namespace %s", tagLabel, tmpNamespaceName)
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
				Eventually(func(g Gomega) string {
					vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(vm.Status.UniqueID).ToNot(BeEmpty())

					status, err := input.WCPClient.GetVMPolicyCompliance(policyID, vm.Status.UniqueID)
					g.Expect(err).ToNot(HaveOccurred())

					return status.Status
				}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).
					Should(Or(Equal("COMPLIANT"), Equal("NOT_COMPLIANT")), "expected VM %s to have a compliance status for policy %s", vmName, policyID)
			}
		}

		BeforeEach(func() {
			topologyKey = hostTopoKey

			skipper.SkipUnlessStretchSupervisorIsEnabled()
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMPlacementPoliciesCapabilityName)
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMAffinityDuringExecutionCapabilityName)
			skipper.SkipUnlessSupervisorHasAtleastOneZoneWithHostCount(ctx, clusterProxy, 3)

			supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, config.InfraConfig.KubeconfigPath)
			Expect(supervisorID).ToNot(BeEmpty(), "Supervisor ID should not be empty")
			zoneList, err := clusterProxy.GetZonesBoundWithSupervisor(supervisorID)
			Expect(err).ToNot(HaveOccurred(), "failed to get zones bound with Supervisor")

			By("Creating a temporary namespace")

			vmserviceCLID := vmservice.GetContentLibraryUUIDByName(consts.VMServiceCLName, input.WCPClient)
			clIDs := []string{vmserviceCLID}
			vmClassNames := []string{clusterResources.VMClassName}
			vmsvcSpecs := wcp.NewVMServiceSpecDetails(vmClassNames, clIDs)
			tmpNamespaceCtx, err := clusterProxy.CreateWCPNamespace(ctx, config, vmsvcSpecs, clusterResources.StorageClassName, fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6)), input.ArtifactFolder)
			Expect(err).ToNot(HaveOccurred(), "failed to create wcp namespace")
			Expect(tmpNamespaceCtx.GetNamespace()).ToNot(BeNil(), "namespace should not be nil")
			tmpNamespaceName = tmpNamespaceCtx.GetNamespace().Name
			DeferCleanup(func() {
				if tmpNamespaceName != "" {
					clusterProxy.DeleteWCPNamespace(tmpNamespaceCtx)
					tmpNamespaceName = ""
				}
			})
			wcp.WaitForNamespaceReady(input.WCPClient, tmpNamespaceName)
			deferCleanupVirtualMachineGroup(
				config,
				svClusterClient,
				clusterProxy.GetKubeconfigPath(),
				tmpNamespaceName,
				vmgRootName,
				vmgChildName,
				vmgKind,
				vmKind,
				&rootGroup,
				&vmMemberNames,
			)

			By("Ensuring the Linux image is available in the temp namespace")

			tmpNamespaceVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, linuxImageDisplayName)

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

			It("Should create VMs in the same zone with required host AF and AAF, repeated 5 times", func() {
				const iterations = 5

				By("Determining a zone bound to the temporary namespace")
				namespaceZones, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), tmpNamespaceName)
				Expect(err).NotTo(HaveOccurred())
				Expect(namespaceZones.Items).ToNot(BeEmpty())

				By("Resolving the tiny-core-linux-complex-hw VMI in the temporary namespace")
				tinyImageVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, tmpNamespaceName, "tiny-core-linux-complex-hw")

				createCacheVMFunc := func(vmName, groupName, preferredZone, appLabel string, antiAffinity bool) {
					GinkgoHelper()

					term := vmopv1a5.VMAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": appLabel,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					}

					affinity := &vmopv1a5.AffinitySpec{}
					if antiAffinity {
						affinity.VMAntiAffinity = &vmopv1a5.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{term},
						}
					} else {
						affinity.VMAffinity = &vmopv1a5.VMAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{term},
						}
					}

					vm := &vmopv1a5.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: tmpNamespaceName,
							Name:      vmName,
							Labels: map[string]string{
								"topology.kubernetes.io/zone": preferredZone,
								"app":                         appLabel,
							},
						},
						Spec: vmopv1a5.VirtualMachineSpec{
							GroupName:    groupName,
							ImageName:    tinyImageVMIName,
							ClassName:    clusterResources.VMClassName,
							StorageClass: clusterResources.StorageClassName,
							PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
							Affinity:     affinity,
						},
					}
					Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)
				}

				aafGroupName := fmt.Sprintf("%s-aaf", vmgRootName)
				afGroupName := fmt.Sprintf("%s-af", vmgRootName)
				aafVM1 := fmt.Sprintf("%s-aaf-vm1", vmgRootName)
				aafVM2 := fmt.Sprintf("%s-aaf-vm2", vmgRootName)
				afVM1 := fmt.Sprintf("%s-af-vm1", vmgRootName)
				afVM2 := fmt.Sprintf("%s-af-vm2", vmgRootName)
				allVMNames := []string{aafVM1, aafVM2, afVM1, afVM2}
				vmMemberNames = allVMNames

				for i := range iterations {
					By(fmt.Sprintf("Iteration %d: creating both VirtualMachineGroups (AAF and AF) and their VMs", i))

					// choose a random zone for each iteration
					num := rand.IntN(len(namespaceZones.Items))
					preferredZone := namespaceZones.Items[num].Name

					aafGroupParameters := manifestbuilders.VirtualMachineGroupYaml{
						Namespace: tmpNamespaceName,
						Name:      aafGroupName,
						BootOrder: []manifestbuilders.BootOrder{
							{
								Members: []vmopv1.GroupMember{
									{Kind: vmKind, Name: aafVM1},
									{Kind: vmKind, Name: aafVM2},
								},
							},
						},
					}
					aafGroup, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(aafGroupParameters)
					Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
					Expect(svClusterClient.Create(ctx, aafGroup)).To(Succeed(), "failed to create VirtualMachineGroup")

					afGroupParameters := manifestbuilders.VirtualMachineGroupYaml{
						Namespace: tmpNamespaceName,
						Name:      afGroupName,
						BootOrder: []manifestbuilders.BootOrder{
							{
								Members: []vmopv1.GroupMember{
									{Kind: vmKind, Name: afVM1},
									{Kind: vmKind, Name: afVM2},
								},
							},
						},
					}
					afGroup, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(afGroupParameters)
					Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
					Expect(svClusterClient.Create(ctx, afGroup)).To(Succeed(), "failed to create VirtualMachineGroup")

					createCacheVMFunc(aafVM1, aafGroupName, preferredZone, "cache-cluster-aaf", true)
					createCacheVMFunc(aafVM2, aafGroupName, preferredZone, "cache-cluster-aaf", true)
					createCacheVMFunc(afVM1, afGroupName, preferredZone, "cache-cluster-af", false)
					createCacheVMFunc(afVM2, afGroupName, preferredZone, "cache-cluster-af", false)

					By(fmt.Sprintf("Iteration %d: waiting for all VMs to be created in vSphere (powered off)", i))
					for _, vmName := range allVMNames {
						vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vmName)
					}

					By(fmt.Sprintf("Iteration %d: verifying zone placement and host affinity/anti-affinity", i))
					for _, vmName := range allVMNames {
						vmZone := getVMZoneFunc(vmName)
						Expect(vmZone).To(Equal(preferredZone), "iteration %d: vm %s zone mismatch", i, vmName)
					}

					aafHost1 := getVMHostFunc(aafVM1)
					aafHost2 := getVMHostFunc(aafVM2)
					Expect(aafHost1).ToNot(BeEmpty())
					Expect(aafHost2).ToNot(BeEmpty())
					Expect(aafHost1).ToNot(Equal(aafHost2), "iteration %d: AAF VMs %s and %s are on the same host", i, aafVM1, aafVM2)

					afHost1 := getVMHostFunc(afVM1)
					afHost2 := getVMHostFunc(afVM2)
					Expect(afHost1).ToNot(BeEmpty())
					Expect(afHost2).ToNot(BeEmpty())
					Expect(afHost1).To(Equal(afHost2), "iteration %d: AF VMs %s and %s are on different hosts", i, afVM1, afVM2)

					By(fmt.Sprintf("Iteration %d: deleting both VirtualMachineGroups", i))
					Expect(svClusterClient.Delete(ctx, aafGroup)).To(Succeed())
					Expect(svClusterClient.Delete(ctx, afGroup)).To(Succeed())
					vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, tmpNamespaceName, aafGroupName)
					vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, tmpNamespaceName, afGroupName)
					for _, vmName := range allVMNames {
						vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, tmpNamespaceName, vmName)
					}
				}
			})

			It("Should create VMs matching a VKS nodepool AF/AAF scenario across 3 tiers", func() {
				By("Verifying there are at least 3 zones bound with the Supervisor")

				supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, config.InfraConfig.KubeconfigPath)
				Expect(supervisorID).ToNot(BeEmpty(), "Supervisor ID should not be empty")
				zoneList, err := clusterProxy.GetZonesBoundWithSupervisor(supervisorID)
				Expect(err).ToNot(HaveOccurred(), "failed to get zones bound with Supervisor")
				Expect(len(zoneList.Zones)).To(BeNumerically(">=", 3))

				// createVKSNodePoolVMFunc builds a VM that mimics a VKS nodepool member: it is
				// anti-affined (at host topology) with the other member of its own tier, anti-affined
				// (at zone topology) with members of the other tiers, and affined (at zone topology,
				// required) with the other member of its own tier so that a tier's two nodes always
				// land in the same zone while different tiers land in different zones.
				createVKSNodePoolVMFunc := func(vmName, tier, antiAffinityType string, otherTiers []string) {
					GinkgoHelper()

					hostnameAntiAffinityTerm := vmopv1a5.VMAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"tier": tier},
						},
						TopologyKey: "kubernetes.io/hostname",
					}
					zoneAntiAffinityTerm := vmopv1a5.VMAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "tier",
									Operator: metav1.LabelSelectorOpIn,
									Values:   otherTiers,
								},
							},
						},
						TopologyKey: "topology.kubernetes.io/zone",
					}

					antiAffinityTerms := []vmopv1a5.VMAffinityTerm{hostnameAntiAffinityTerm, zoneAntiAffinityTerm}
					antiAffinity := &vmopv1a5.VMAntiAffinitySpec{}
					if antiAffinityType == preferredDuringSchedulingPreferredDuringExecution {
						antiAffinity.PreferredDuringSchedulingPreferredDuringExecution = antiAffinityTerms
					} else {
						antiAffinity.RequiredDuringSchedulingPreferredDuringExecution = antiAffinityTerms
					}

					vm := &vmopv1a5.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: tmpNamespaceName,
							Name:      vmName,
							Labels:    map[string]string{"tier": tier, "capv.vmware.com/cluster.role": "node"},
						},
						Spec: vmopv1a5.VirtualMachineSpec{
							GroupName:    vmgRootName,
							ImageName:    tmpNamespaceVMIName,
							ClassName:    clusterResources.VMClassName,
							StorageClass: clusterResources.StorageClassName,
							PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
							Affinity: &vmopv1a5.AffinitySpec{
								VMAntiAffinity: antiAffinity,
								VMAffinity: &vmopv1a5.VMAffinitySpec{
									RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"tier": tier},
											},
											TopologyKey: "topology.kubernetes.io/zone",
										},
									},
								},
							},
						},
					}
					Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)
				}

				tier1VM1 := fmt.Sprintf("%s-tier1-vm1", vmgRootName)
				tier1VM2 := fmt.Sprintf("%s-tier1-vm2", vmgRootName)
				tier2VM1 := fmt.Sprintf("%s-tier2-vm1", vmgRootName)
				tier2VM2 := fmt.Sprintf("%s-tier2-vm2", vmgRootName)
				tier3VM1 := fmt.Sprintf("%s-tier3-vm1", vmgRootName)
				tier3VM2 := fmt.Sprintf("%s-tier3-vm2", vmgRootName)
				vmMemberNames = []string{tier1VM1, tier1VM2, tier2VM1, tier2VM2, tier3VM1, tier3VM2}

				By("Creating a VirtualMachineGroup with 6 VM-kind members across 3 tiers")

				vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
					Namespace: tmpNamespaceName,
					Name:      vmgRootName,
					BootOrder: []manifestbuilders.BootOrder{
						{
							Members: []vmopv1.GroupMember{},
						},
					},
				}
				for _, v := range vmMemberNames {
					vmgParameters.BootOrder[0].Members = append(vmgParameters.BootOrder[0].Members,
						vmopv1.GroupMember{Kind: vmKind, Name: v})
				}
				vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgParameters)
				Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
				rootGroup = vmgRoot
				Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

				By(fmt.Sprintf("Creating tier-1 VMs (%s preferred, %s required) host AAF within tier, zone AAF against tiers 2 and 3", tier1VM1, tier1VM2))
				createVKSNodePoolVMFunc(tier1VM1, "1", preferredDuringSchedulingPreferredDuringExecution, []string{"3", "2"})
				createVKSNodePoolVMFunc(tier1VM2, "1", requiredDuringSchedulingPreferredDuringExecution, []string{"3", "2"})

				By(fmt.Sprintf("Creating tier-2 VMs (%s, %s) required host AAF within tier, zone AAF against tiers 1 and 3", tier2VM1, tier2VM2))
				createVKSNodePoolVMFunc(tier2VM1, "2", requiredDuringSchedulingPreferredDuringExecution, []string{"3", "1"})
				createVKSNodePoolVMFunc(tier2VM2, "2", requiredDuringSchedulingPreferredDuringExecution, []string{"3", "1"})

				By(fmt.Sprintf("Creating tier-3 VMs (%s, %s) required host AAF within tier, zone AAF against tiers 1 and 2", tier3VM1, tier3VM2))
				createVKSNodePoolVMFunc(tier3VM1, "3", requiredDuringSchedulingPreferredDuringExecution, []string{"1", "2"})
				createVKSNodePoolVMFunc(tier3VM2, "3", requiredDuringSchedulingPreferredDuringExecution, []string{"1", "2"})

				By("Waiting for all VMs to be created in vSphere (powered off)")

				for _, vmName := range vmMemberNames {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vmName)
				}

				By("Powering on all VMs")
				for _, vmName := range vmMemberNames {
					powerOnVMFunc(vmName)
				}
				for _, vmName := range vmMemberNames {
					vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOn)
				}

				By("Verifying vm.Status.Zone is populated for all VMs after power-on")

				vmZones := make(map[string]string, len(vmMemberNames))
				for _, vmName := range vmMemberNames {
					vmZones[vmName] = getVMZoneFunc(vmName)
				}

				By("Verifying each tier's two VMs are placed in the same zone, per the required zone affinity within the tier")
				Expect(vmZones[tier1VM1]).To(Equal(vmZones[tier1VM2]))
				Expect(vmZones[tier2VM1]).To(Equal(vmZones[tier2VM2]))
				Expect(vmZones[tier3VM1]).To(Equal(vmZones[tier3VM2]))

				By("Verifying each tier is placed in a different zone from the other tiers, per the required zone anti-affinity across tiers")
				Expect(vmZones[tier1VM1]).ToNot(Equal(vmZones[tier2VM1]))
				Expect(vmZones[tier1VM1]).ToNot(Equal(vmZones[tier3VM1]))
				Expect(vmZones[tier2VM1]).ToNot(Equal(vmZones[tier3VM1]))
			})
		})
	})
}

// vcNow returns the time of the vCenter clock. It is the clock of the power
// events, so the two can be compared without clock skew.
func vcNow(ctx context.Context, c *vim25.Client) time.Time {
	GinkgoHelper()

	t, err := methods.GetCurrentTime(ctx, c)
	Expect(err).NotTo(HaveOccurred(), "failed to get the vCenter time")

	return *t
}

// vmPowerEvents returns the vCenter times of the power-on and power-off events
// of the VM since the given vCenter time, oldest first. The events of a VM
// can only be read by its managed object reference before it is deleted.
func vmPowerEvents(
	ctx context.Context,
	c *vim25.Client,
	moRef types.ManagedObjectReference,
	since time.Time) (on, off []time.Time, err error) {

	events, err := event.NewManager(c).QueryEvents(ctx, types.EventFilterSpec{
		Entity: &types.EventFilterSpecByEntity{
			Entity:    moRef,
			Recursion: types.EventFilterSpecRecursionOptionSelf,
		},
		Time: &types.EventFilterSpecByTime{BeginTime: &since},
	})
	if err != nil {
		return nil, nil, err
	}

	for _, e := range events {
		switch e.(type) {
		case *types.VmPoweredOnEvent, *types.DrsVmPoweredOnEvent:
			on = append(on, e.GetEvent().CreatedTime)
		case *types.VmPoweredOffEvent:
			off = append(off, e.GetEvent().CreatedTime)
		}
	}
	slices.SortFunc(on, time.Time.Compare)
	slices.SortFunc(off, time.Time.Compare)

	return on, off, nil
}

// eventSkew allows for vCenter to log the power event of a VM after the new
// power state is visible, and so after VM Operator can act on it. A gap that
// starts at such an event looks shorter than it is.
const eventSkew = 2 * time.Second

// vmPowerChangeTime returns the vCenter time of the named VM's change to
// PoweredOn. Call it after the VM is observed powered on. It fails if, since
// the given vCenter time, the VM did not power on exactly once or powered off.
// It waits a short time for vCenter to log the event.
func vmPowerChangeTime(
	ctx context.Context,
	vc *vim25.Client,
	c ctrlclient.Client,
	namespace, name string,
	since time.Time) time.Time {

	GinkgoHelper()

	moRef := vmMoRef(ctx, c, namespace, name)

	var t time.Time
	Eventually(func(g Gomega) {
		on, off, err := vmPowerEvents(ctx, vc, moRef, since)
		g.Expect(err).NotTo(HaveOccurred())

		if len(on) > 1 || len(off) > 0 {
			StopTrying(fmt.Sprintf(
				"%s has %d power-on and %d power-off events since %v, not one power-on",
				name, len(on), len(off), since)).Now()
		}
		g.Expect(on).To(HaveLen(1), "%s has no event for the power-on", name)
		t = on[0]
	}, "30s", "2s").Should(Succeed(), "%s has no event for the power-on", name)

	return t
}

// expectGap verifies the time between two power state changes, both vCenter
// times, is at least the delay, less eventSkew. There is no upper bound, as
// the time to release and power a VM depends on the load of the cluster. The
// order of changes with no delay is checked with the Kubernetes objects, see
// watchReleaseOrder, as two events can be logged out of order.
func expectGap(gap, delay time.Duration, desc string) {
	GinkgoHelper()

	By(fmt.Sprintf("%s: %v (delay %v)", desc, gap, delay))
	Expect(delay).To(BeNumerically(">", 0), "%s: there is no delay to check", desc)
	Expect(gap).To(BeNumerically(">=", delay-eventSkew), "%s: the change came before the delay elapsed", desc)
}

// waitForPowerStates waits for each of the named VMs to be observed in the
// target power state.
func waitForPowerStates(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	names []string,
	target vmopv1.VirtualMachinePowerState,
	timeout time.Duration) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		for _, name := range names {
			vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Status.PowerState).To(Equal(target), "%s has not reached %q", name, target)
		}
	}, timeout, "2s").Should(Succeed(), "the VMs did not all reach %q", target)
}

// vmMoRef returns the vCenter managed object reference of the named VM.
func vmMoRef(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, name string) types.ManagedObjectReference {

	GinkgoHelper()

	return types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: vmoperator.GetVirtualMachineMOID(ctx, c, namespace, name),
	}
}

// waitForGroupRequestClosed waits for the last power state request of the
// group to be closed. A request is closed when the status time is after the
// time of the request, the same test the controller uses. While a request is
// open, the controller sets the desired power state of a member back to the
// group's, so a test must not change a member directly until it is closed.
// The group being Ready, or its members being in the power state, does not
// mean the request is closed, as the controller closes it in a later reconcile.
func waitForGroupRequestClosed(
	ctx context.Context,
	c ctrlclient.Client,
	key ctrlclient.ObjectKey) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		group, err := utils.GetVirtualMachineGroup(ctx, c, key.Namespace, key.Name)
		g.Expect(err).NotTo(HaveOccurred())

		v := group.Annotations[pkgconst.LastUpdatedPowerStateTimeAnnotation]
		g.Expect(v).ToNot(BeEmpty(), "the group has no power state request")
		requestTime, err := time.Parse(time.RFC3339Nano, v)
		g.Expect(err).NotTo(HaveOccurred())

		closed := group.Status.LastUpdatedPowerStateTime
		g.Expect(closed).ToNot(BeNil(), "the power state request is not closed")
		g.Expect(closed.After(requestTime)).To(BeTrue(),
			"the power state request at %s is not closed, the status is %s", v, closed)
	}, 2*time.Minute, "1s").Should(Succeed(), "the power state request of %s was not closed", key.Name)
}

// deferDeleteVM registers a cleanup that deletes the VM, and ignores a VM that
// is already gone. It keeps the VM from lingering when the group it is meant
// to join was never created, since no owner reference would delete it then.
func deferDeleteVM(c ctrlclient.Client, vm *vmopv1a5.VirtualMachine) {
	GinkgoHelper()

	DeferCleanup(func(ctx context.Context) {
		Expect(ctrlclient.IgnoreNotFound(c.Delete(ctx, vm))).To(Succeed(), "failed to delete VirtualMachine %s", vm.Name)
	})
}

const (
	pinnedVMKind = "VirtualMachine"
	zoneLabelKey = "topology.kubernetes.io/zone"
	hostTopoKey  = "kubernetes.io/hostname"
)

// zonePinnedVM describes a VirtualMachineGroup member VM that may be pinned to
// a zone via the topology.kubernetes.io/zone label.
type zonePinnedVM struct {
	Name string
	// Zone is the zone the VM is pinned to. Empty means the VM is not pinned.
	Zone     string
	Labels   map[string]string
	Affinity *vmopv1a5.AffinitySpec
}

// requiredAntiAffinity returns an AffinitySpec that requires the VM to be
// placed in a different topologyKey domain than the VMs matching matchLabels.
func requiredAntiAffinity(topologyKey string, matchLabels map[string]string) *vmopv1a5.AffinitySpec {
	return &vmopv1a5.AffinitySpec{
		VMAntiAffinity: &vmopv1a5.VMAntiAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
				TopologyKey:   topologyKey,
			}},
		},
	}
}

// patchVMGroup gets the VirtualMachineGroup identified by key into group,
// applies mutate to it, and patches the resulting changes. Pass an empty group
// of the API version to use, e.g. &vmopv1a5.VirtualMachineGroup{}.
func patchVMGroup[T ctrlclient.Object](
	ctx context.Context,
	c ctrlclient.Client,
	key ctrlclient.ObjectKey,
	group T,
	mutate func(T),
	failureMsg string) {

	GinkgoHelper()

	Expect(c.Get(ctx, key, group)).To(Succeed(), "failed to get VirtualMachineGroup %s", key)

	base, ok := group.DeepCopyObject().(T)
	Expect(ok).To(BeTrue(), "unexpected type %T from DeepCopyObject", group)

	mutate(group)
	Expect(c.Patch(ctx, group, ctrlclient.MergeFrom(base))).To(Succeed(), failureMsg)
}

// createVMGroupWithVMs creates a root VirtualMachineGroup whose boot order has
// the given VMs as its members, and returns the group.
func createVMGroupWithVMs(
	ctx context.Context,
	clusterProxy framework.ClusterProxyInterface,
	namespace, groupName string,
	vms []zonePinnedVM) *vmopv1a5.VirtualMachineGroup {

	GinkgoHelper()

	members := make([]vmopv1.GroupMember, len(vms))
	for i, vm := range vms {
		members[i] = vmopv1.GroupMember{Kind: pinnedVMKind, Name: vm.Name}
	}

	group, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(
		manifestbuilders.VirtualMachineGroupYaml{
			Namespace: namespace,
			Name:      groupName,
			BootOrder: []manifestbuilders.BootOrder{{Members: members}},
		})
	Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
	Expect(clusterProxy.GetClient().Create(ctx, group)).To(Succeed())

	return group
}

// createZonePinnedVMs creates the given VMs as members of the group, powered
// off since only placement is under test. VMs with a Zone get the
// topology.kubernetes.io/zone label.
func createZonePinnedVMs(
	ctx context.Context,
	clusterProxy framework.ClusterProxyInterface,
	clusterResources *e2eConfig.Resources,
	namespace, groupName, imageName string,
	vms []zonePinnedVM) {

	GinkgoHelper()

	for _, vm := range vms {
		labels := map[string]string{}
		maps.Copy(labels, vm.Labels)
		if vm.Zone != "" {
			labels[zoneLabelKey] = vm.Zone
		}

		vmToCreate := &vmopv1a5.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      vm.Name,
				Labels:    labels,
			},
			Spec: vmopv1a5.VirtualMachineSpec{
				GroupName:    groupName,
				ImageName:    imageName,
				ClassName:    clusterResources.VMClassName,
				StorageClass: clusterResources.StorageClassName,
				PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
				Affinity:     vm.Affinity,
			},
		}
		Expect(clusterProxy.GetClient().Create(ctx, vmToCreate)).To(Succeed(), "failed to create VM %s", vm.Name)
	}
}

// getZonePinnedVMNames returns the names of the given VMs.
func getZonePinnedVMNames(vms []zonePinnedVM) []string {
	names := make([]string, len(vms))
	for i, vm := range vms {
		names[i] = vm.Name
	}

	return names
}

// getZoneWithMinHosts returns a zone with at least minHostCount hosts and its
// host count. It skips the test if there is no such zone.
func getZoneWithMinHosts(
	ctx context.Context,
	clusterProxy framework.ClusterProxyInterface,
	minHostCount int) (string, int) {

	GinkgoHelper()

	zoneHostInfos, err := utils.GetHostsPerZone(ctx, clusterProxy.GetClient(), clusterProxy.GetKubeconfigPath())
	Expect(err).NotTo(HaveOccurred(), "failed to list zones with hosts")

	for _, zoneHostInfo := range zoneHostInfos {
		if len(zoneHostInfo.HostIDs) >= minHostCount {
			return zoneHostInfo.ZoneName, len(zoneHostInfo.HostIDs)
		}
	}

	Skip(fmt.Sprintf("skip the test as no zone has at least %d hosts", minHostCount))

	return "", 0
}

// waitForVMZone waits for the VM to have a zone in its status and returns it.
func waitForVMZone(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, vmName string) string {

	GinkgoHelper()

	var zone string
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, client, namespace, vmName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(vm.Status.Zone).ToNot(BeEmpty())
		zone = vm.Status.Zone
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"timed out waiting for VirtualMachine %s to have a zone", vmName)

	return zone
}

// getVMHostFromVmodl returns the host MoID of the VM from vSphere directly. Use
// this when the VM is not powered on, since vm.Status.NodeName is not yet set.
func getVMHostFromVmodl(
	ctx context.Context,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	namespace, vmName string) string {

	GinkgoHelper()

	moid := vmoperator.GetVirtualMachineMOID(ctx, client, namespace, vmName)
	vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: moid}

	var vmMO mo.VirtualMachine
	Expect(property.DefaultCollector(vimClient).RetrieveOne(ctx, vmMoRef, []string{"runtime.host"}, &vmMO)).To(Succeed())
	Expect(vmMO.Runtime.Host).ToNot(BeNil())

	return vmMO.Runtime.Host.Value
}

func deferCleanupVirtualMachineGroup(
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	kubeconfigPath, namespace, rootName, childName, groupKind, vmKind string,
	rootGroup *ctrlclient.Object,
	vmMemberNames *[]string) {

	vmoperator.DeferCleanupWithDumpOnFailure(
		func(ctx context.Context) {
			if *rootGroup == nil {
				return
			}

			By("Deleting the root VirtualMachineGroup")
			// The group may not exist, ex. when its creation was rejected.
			// Tolerate that so the VMs below are still cleaned up.
			Expect(ctrlclient.IgnoreNotFound(client.Delete(ctx, *rootGroup))).To(Succeed(), "failed to delete VirtualMachineGroup")
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, client, namespace, rootName)

			By("Waiting for all group members to be deleted automatically due to owner reference to the root group")
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, client, namespace, childName)

			for _, vmName := range *vmMemberNames {
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, client, namespace, vmName)
			}

			*rootGroup = nil
		},
		func(ctx context.Context) {
			if *rootGroup == nil {
				return
			}

			vmoperator.DescribeResource(kubeconfigPath, groupKind, namespace, rootName)(ctx)
			vmoperator.DescribeResource(kubeconfigPath, groupKind, namespace, childName)(ctx)
			for _, vmName := range *vmMemberNames {
				vmoperator.DescribeResource(kubeconfigPath, vmKind, namespace, vmName)(ctx)
			}
		},
	)
}
