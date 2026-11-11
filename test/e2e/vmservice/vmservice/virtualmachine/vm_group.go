// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1a5cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	vmopv1a5common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
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

		// Check this before any of the setup below or in the nested
		// BeforeEach nodes, which create and zone-bind a temporary namespace
		// that a skipped spec would just throw away.
		if slices.Contains(CurrentSpecReport().Labels(), consts.HardAffinityDuringExecutionLabel) {
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMHardAffinityDuringExecutionCapabilityName)
		}

		if slices.Contains(CurrentSpecReport().Labels(), consts.ClasslessVMLabel) {
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMServiceClasslessVMCapabilityName)
		}

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

			By("Verifying each VM is released only after the previous one is on")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm1Name, vm2Name, vm3Name}, vmopv1.VirtualMachinePowerStateOn, 5*time.Minute)

			By("Verifying each VM powered on once, after the delay of its boot order group")

			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, powerOnSince, vmopv1.VirtualMachinePowerStateOn)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, powerOnSince, vmopv1.VirtualMachinePowerStateOn)
			vm3OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, powerOnSince, vmopv1.VirtualMachinePowerStateOn)
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
		It("Should power on each boot order group after the previous one is on, with its delay counted from then", func() {
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
						// The first boot order group powers on first. It has
						// no previous boot order group to wait for, so its
						// PowerOnDelay has no effect.
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

			By("Verifying each boot order group is released only after the previous one is on")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm1Name, vm2Name, vm3Name}, vmopv1.VirtualMachinePowerStateOn, 5*time.Minute)

			By("Verifying each VM powered on once, with the delay of its boot order group counted from the previous one")

			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOn)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOn)
			vm3OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOn)

			// vm1 is in the first boot order group, so its delay is ignored.
			expectDelayIgnored(vm1OnTime.Sub(since), 0, vm1PowerOnDelay, "vm1 on after the request")
			expectGap(vm2OnTime.Sub(vm1OnTime), vm2PowerOnDelay, "vm2 on after vm1")
			expectGap(vm3OnTime.Sub(vm2OnTime), vm3PowerOnDelay, "vm3 on after vm2")
		})
	})

	Context("Boot order power-off delay", Label(consts.ClasslessVMLabel), func() {
		var (
			vmTemplate *vmopv1a5.VirtualMachine
			groupKey   ctrlclient.ObjectKey
		)

		BeforeEach(func() {
			vmTemplate = &vmopv1a5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: input.WCPNamespaceName,
				},
				Spec: vmopv1a5.VirtualMachineSpec{
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1a5.VirtualMachinePowerStateOn,
				},
			}
			groupKey = ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}
		})

		It("Should power off each boot order group after the previous one is off, with its delay counted from then", func() {
			const (
				vm1PowerOffDelay = 1 * time.Minute
				vm2PowerOffDelay = 30 * time.Second
				// The delay is ignored, which is checked below.
				vm3PowerOffDelay = 2 * time.Minute
			)

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}

			By("Creating a VirtualMachineGroup (v1alpha6) with 3 boot order groups and power off delays")

			vmGroupParameters := manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}},
						PowerOffDelay: vm1PowerOffDelay.String(),
					},
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
						PowerOffDelay: vm2PowerOffDelay.String(),
					},
					{
						// The last boot order group powers off first. It has
						// no previous boot order group to wait for, so its
						// PowerOffDelay has no effect.
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}},
						PowerOffDelay: vm3PowerOffDelay.String(),
					},
				},
			}
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(vmGroupParameters)
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			By("Creating the VMs powered on, with spec.groupName pointing to the VirtualMachineGroup")

			for _, vmName := range vmMemberNames {
				vm := vmTemplate.DeepCopy()
				vm.Name = vmName
				vm.Spec.GroupName = vmgRootName
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", vm.Name)
				deferDeleteVM(svClusterClient, vm)
			}

			By("Waiting for all VMs to be powered on")

			for _, vmName := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, vmopv1.VirtualMachinePowerStateOn)
			}

			By("Waiting for all VMs to have group linked condition set to true")

			groupLinkedTrueCondition := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				Status: metav1.ConditionTrue,
			}
			for _, vmName := range vmMemberNames {
				vmoperator.WaitOnVirtualMachineCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, groupLinkedTrueCondition)
			}

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff and spec.powerOffMode to Hard")

			// Use Hard power off so the VMs don't attempt a soft power off
			// first, which can block for up to 5 minutes if the guest is
			// unresponsive and would throw off the power off delay timing
			// assertions below.
			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying each boot order group is released only after the previous one is off")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm3Name, vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff, 5*time.Minute)

			By("Verifying each VM powered off once, with the delay of its boot order group counted from the previous one")

			// vm3 (the last boot order group) is released first, and its
			// delay is ignored.
			vm3OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm1OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)

			expectDelayIgnored(vm3OffTime.Sub(since), 0, vm3PowerOffDelay, "vm3 off after the request")
			expectGap(vm2OffTime.Sub(vm3OffTime), vm2PowerOffDelay, "vm2 off after vm3")
			expectGap(vm1OffTime.Sub(vm2OffTime), vm1PowerOffDelay, "vm1 off after vm2")
		})

		It("Should stamp only the members it releases with a delay, and not move the stamp", func() {
			const (
				// vm1 (zero delay) and vm4 (first released, so its delay is
				// ignored) must get no stamp. vm2 and vm3 share a boot
				// order group. vm4's delay is ignored, which is checked below.
				vm2PowerOffDelay = 45 * time.Second
				vm4PowerOffDelay = 2 * time.Minute
			)

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Powering off vm3 before the group request, so it is already complete")

			vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
			Expect(err).NotTo(HaveOccurred())
			vm3Patch := vm3.DeepCopy()
			vm3Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm3Patch.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			Expect(svClusterClient.Patch(ctx, vm3Patch, ctrlclient.MergeFrom(vm3))).To(Succeed(), "failed to power off vm3")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")

			By("Adding a stale stamp to vm1, which has no delay, so the group must remove it")

			// Only a privileged user can set this annotation on a VM.
			adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to get the admin cluster proxy")
			DeferCleanup(adminClusterProxy.Dispose)
			adminClient, err := adminClusterProxy.GetAdminClient()
			Expect(err).NotTo(HaveOccurred(), "failed to get the admin client")

			vm1, err := utils.GetVirtualMachine(ctx, adminClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			vm1Patch := vm1.DeepCopy()
			if vm1Patch.Annotations == nil {
				vm1Patch.Annotations = map[string]string{}
			}
			vm1Patch.Annotations[pkgconst.ApplyPowerStateTimeAnnotation] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
			Expect(adminClient.Patch(ctx, vm1Patch, ctrlclient.MergeFrom(vm1))).To(Succeed(), "failed to add a stale stamp to vm1")

			By("Creating a VirtualMachineGroup with a zero delay group, a group of two VMs, and a last group with a delay")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}, {Kind: vmKind, Name: vm3Name}},
						PowerOffDelay: vm2PowerOffDelay.String(),
					},
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm4Name}},
						PowerOffDelay: vm4PowerOffDelay.String(),
					},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			// vm3 is already off, so only vm2 marks the release of its group.
			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm4Name}, {vm2Name}, {vm1Name}}, vmopv1.VirtualMachinePowerStateOff)

			By("Verifying vm4, which is released first, has no stamp even though it has a delay")

			// The group sets the stamp in the same patch that releases the
			// VM, and the VM removes it only when its time arrives, so this
			// is checked as soon as vm4 is released.
			Eventually(func(g Gomega) {
				vm4, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm4Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm4.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				g.Expect(vm4.Annotations).ToNot(HaveKey(pkgconst.ApplyPowerStateTimeAnnotation), "vm4 was stamped")
			}, "1m", "1s").Should(Succeed(), "vm4 was not released")

			By("Verifying vm2 is released with a stamp in the future that does not move")

			var stamp string
			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				v, ok := vm2.Annotations[pkgconst.ApplyPowerStateTimeAnnotation]
				g.Expect(ok).To(BeTrue(), "vm2 was released without a stamp")
				stamp = v
			}, "1m", "1s").Should(Succeed())

			// That the stamp delays vm2 is checked with vCenter times below,
			// and not with the local clock, which can differ from the
			// clock of the controller.
			_, err = time.Parse(time.RFC3339Nano, stamp)
			Expect(err).NotTo(HaveOccurred(), "vm2 has an invalid stamp %q", stamp)

			Consistently(func(g Gomega) {
				v, ok := vmStamp(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(ok).To(BeTrue())
				g.Expect(v).To(Equal(stamp), "the stamp of vm2 moved")
			}, 15*time.Second, "5s").Should(Succeed())

			By("Verifying vm1, which has no delay, is released without a stamp once vm2 and vm3 are off")

			Eventually(func(g Gomega) {
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				g.Expect(vm1.Annotations).ToNot(HaveKey(pkgconst.ApplyPowerStateTimeAnnotation), "vm1 was stamped")
			}, "2m", "1s").Should(Succeed(), "vm1 was not released without a stamp")
			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)
			verifyOrder()

			By("Verifying vm4 powered off once without its delay, and vm2 once after its delay")

			vm4OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm4Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			// vm1 is checked for a single change. Its order is checked above.
			vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)
			expectDelayIgnored(vm4OffTime.Sub(since), 0, vm4PowerOffDelay, "vm4 off after the request")
			expectGap(vm2OffTime.Sub(vm4OffTime), vm2PowerOffDelay, "vm2 off after vm4")

			By("Verifying vm3, which was already off, did not change power state")

			// vm3 has no power state change in this request, so no time is
			// checked for it.
			expectNoVMPowerTransition(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since)
		})

		It("Should carry the delay of a child group in the parent's stamp, and ignore the delay of the child's first group", func() {
			const (
				childPowerOffDelay = 30 * time.Second
				vm2PowerOffDelay   = 10 * time.Second
				// The delay is ignored, which is checked below.
				ignoredFirstGroupDelay = 90 * time.Second
			)

			// root: [child] (delay), [vm1]. child: [vm2] (delay), [vm3]
			// (delay, ignored). Power-off order: vm1, then child (vm3, vm2).
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating the root and child VirtualMachineGroups")

			rootGroup = newVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, "", []manifestbuilders.BootOrder{
				{
					Members:       []vmopv1.GroupMember{{Kind: vmgKind, Name: vmgChildName}},
					PowerOffDelay: childPowerOffDelay.String(),
				},
				{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
			})
			newVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgChildName, vmgRootName, []manifestbuilders.BootOrder{
				{
					Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
					PowerOffDelay: vm2PowerOffDelay.String(),
				},
				{
					Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}},
					PowerOffDelay: ignoredFirstGroupDelay.String(),
				},
			})

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgChildName, []string{vm2Name, vm3Name})
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm1Name})

			By("Setting root VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm1Name}, {vm3Name}, {vm2Name}}, vmopv1.VirtualMachinePowerStateOff)

			By("Verifying the parent shows the child group as Pending, naming the VM that is not off, while the delay runs")

			Eventually(func(g Gomega) {
				c := groupMemberPowerStateSynced(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, vmgChildName, vmgKind)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Reason).To(Equal("Pending"))
				g.Expect(c.Message).To(ContainSubstring(vm2Name))
			}, "2m", "1s").Should(Succeed(), "the child group was never reported as Pending on vm2")

			By("Verifying vm3 powers off after the delay of the child group, counted from when vm1 was off, then vm2 after its own delay")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)
			verifyOrder()

			// The delay of the first group of the child must be ignored.
			vm1OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm3OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			expectGap(vm3OffTime.Sub(vm1OffTime), childPowerOffDelay, "vm3 off after vm1")
			expectDelayIgnored(vm3OffTime.Sub(vm1OffTime), childPowerOffDelay, ignoredFirstGroupDelay, "vm3 off after vm1")
			expectGap(vm2OffTime.Sub(vm3OffTime), vm2PowerOffDelay, "vm2 off after vm3")
		})

		It("Should apply changes to the group made while a power-off request is waiting on a delay", func() {
			const (
				// window is how long vm2 waits, which is the time to make
				// the changes.
				window      = 90 * time.Second
				vm1NewDelay = 20 * time.Second
			)

			// vm4 is created now and added to the group during the wait.
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup where vm2 waits a long time")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
						PowerOffDelay: window.String(),
					},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm1Name, vm2Name, vm3Name})

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with TrySoft power off mode")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
			}, "failed to update VirtualMachineGroup power state")

			// vm4 is not in the group until it is added below.
			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm3Name}, {vm2Name, vm4Name}, {vm1Name}}, vmopv1.VirtualMachinePowerStateOff)

			By("Waiting for vm3 to power off and vm2 to be released with a stamp")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")
			var vm2Stamp string
			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				v, ok := vm2.Annotations[pkgconst.ApplyPowerStateTimeAnnotation]
				g.Expect(ok).To(BeTrue(), "vm2 was released without a stamp")
				vm2Stamp = v
			}, "1m", "1s").Should(Succeed())

			By("Changing the mode to Hard, setting a delay on the first group, and adding vm4 to the waiting group")

			// vm4 is released after this time, so it waits the window from
			// a time after it.
			vm4AddTime := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
				g.Spec.BootOrder[0].PowerOffDelay = &metav1.Duration{Duration: vm1NewDelay}
				g.Spec.BootOrder[1].Members = append(g.Spec.BootOrder[1].Members,
					vmopv1.GroupMember{Kind: vmKind, Name: vm4Name})
			}, "failed to update the VirtualMachineGroup")
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm4Name})

			By("Verifying vm2 gets the new mode and keeps its stamp, and vm4 is released with its own stamp")

			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeHard))
				g.Expect(vm2.Annotations[pkgconst.ApplyPowerStateTimeAnnotation]).To(Equal(vm2Stamp), "the stamp of vm2 moved")

				vm4, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm4Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm4.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				g.Expect(vm4.Annotations).To(HaveKey(pkgconst.ApplyPowerStateTimeAnnotation), "vm4 was released without a stamp")
			}, "1m", "1s").Should(Succeed())

			By("Waiting for all VMs to power off, with vm1 held until vm2 and vm4 are off")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, 5*time.Minute)
			verifyOrder()

			By("Verifying vm2 and vm4 each powered off once after the window, counted from when each was released")

			// The order of vm2 and vm4 is not checked. Each one powers off at
			// its own stamp, and the stamps differ only by the time the test
			// took to make the changes.
			vm3OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm4OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm4Name, since, vmopv1.VirtualMachinePowerStateOff)
			expectGap(vm2OffTime.Sub(vm3OffTime), window, "vm2 off after vm3")
			// vm4AddTime is read before vm4 is released, so no allowance for
			// the event of a previous VM is needed.
			Expect(vm4OffTime.Sub(vm4AddTime)).To(BeNumerically(">=", window), "vm4 powered off before the window elapsed after it was added")

			By("Verifying vm1 powered off once with the new delay and mode")

			vm1OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)
			vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm1.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeHard))

			// vm1 is released when the later of vm2 and vm4 is off.
			lastOffTime := vm2OffTime
			if vm4OffTime.After(lastOffTime) {
				lastOffTime = vm4OffTime
			}
			expectGap(vm1OffTime.Sub(lastOffTime), vm1NewDelay, "vm1 off after vm2 and vm4")
		})
	})

	Context("Boot order power-off ordering", func() {
		var (
			vmTemplate *vmopv1a5.VirtualMachine
			groupKey   ctrlclient.ObjectKey
		)

		BeforeEach(func() {
			vmTemplate = &vmopv1a5.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: input.WCPNamespaceName,
				},
				Spec: vmopv1a5.VirtualMachineSpec{
					ImageName:    linuxVMIName,
					ClassName:    clusterResources.VMClassName,
					StorageClass: clusterResources.StorageClassName,
					PowerState:   vmopv1a5.VirtualMachinePowerStateOn,
				},
			}
			groupKey = ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}
		})

		// This test sets powerOffDelay, which the webhook rejects unless the
		// classless VM capability is enabled.
		It("Should power off again a released member that a user powers on, and hold the next boot order group", Label(consts.ClasslessVMLabel), func() {
			// vm2's delay keeps it on for long enough to power on vm3 again
			// while the request is still pending.
			const vm2PowerOffDelay = 2 * time.Minute

			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup with 3 boot order groups")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{
						Members:       []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
						PowerOffDelay: vm2PowerOffDelay.String(),
					},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			// vm3 is left out, as it is powered on again while vm2 is
			// released, which is not a release order violation.
			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm2Name}, {vm1Name}}, vmopv1.VirtualMachinePowerStateOff)

			By("Waiting for vm3 to power off and vm2 to be released")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")
			Eventually(func(g Gomega) {
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm2.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "1m", "2s").Should(Succeed(), "vm2 was not released after vm3 powered off")

			By("Powering on vm3 directly while the group power-off request is pending")

			vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
			Expect(err).NotTo(HaveOccurred())
			vm3Patch := vm3.DeepCopy()
			vm3Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vm3Patch, ctrlclient.MergeFrom(vm3))).To(Succeed(), "failed to power on vm3")

			By("Verifying the group sets vm3 to PoweredOff again and vm1 is not released meanwhile")

			// Only the group sets vm3's desired power state back to off.
			Eventually(func(g Gomega) {
				vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm3.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "1m", "2s").Should(Succeed(), "the group did not set vm3 back to PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")

			// vm2 is still in its delay, so vm1 must stay held.
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm1Name}, vmopv1.VirtualMachinePowerStateOff)

			By("Waiting for all VMs to power off, with vm1 released only after vm2 is off")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, vm2PowerOffDelay+2*time.Minute)
			verifyOrder()

			By("Verifying vm2 powered off once, after its delay counted from when vm3 was first off")

			// vm3 powered off, on and off again, so it has more than one
			// change. Its first power-off released vm2.
			_, vm3Offs, err := vmPowerEvents(ctx, vCenterClient, vmMoRef(ctx, svClusterClient, input.WCPNamespaceName, vm3Name), since)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm3Offs).ToNot(BeEmpty(), "vm3 has no power-off event")
			vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			expectGap(vm2OffTime.Sub(vm3Offs[0]), vm2PowerOffDelay, "vm2 off after vm3")
			vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)
		})

		It("Should power off each boot order group in reverse order and then close the request", func() {
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup with 3 boot order groups and no delays")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying each boot order group is released only after the group before it is off")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm3Name, vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)

			By("Verifying VirtualMachineGroup has Ready condition set to true")

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName,
				metav1.Condition{Type: vmopv1.ReadyConditionType, Status: metav1.ConditionTrue})

			By("Powering on vm1 directly after the request is closed")

			vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			vm1Patch := vm1.DeepCopy()
			vm1Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vm1Patch, ctrlclient.MergeFrom(vm1))).To(Succeed(), "failed to power on vm1")

			By("Verifying the member is reported as not synced and the group does not power it off again")

			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
					Status: metav1.ConditionFalse,
					Reason: "NotSynced",
				})
			Consistently(func(g Gomega) {
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			}, 30*time.Second, "5s").Should(Succeed(), "the group powered off vm1 after the request was closed")
		})

		It("Should hold later boot order groups while a member blocks the walk, and continue when it is fixed", func() {
			var (
				// vm5 is never created, so it keeps the request open.
				vm5Name = fmt.Sprintf("%s-vm5", vmgRootName)
				// vm6 is created but not linked until the walk is blocked on it.
				vm6Name = fmt.Sprintf("%s-vm6", vmgRootName)
			)

			// vm4 and vm6 are created but not linked to the group yet.
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name, vm6Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup where the groups released first hold vm6 and vm4 (not linked)")

			// Power-off order is the reverse of this list.
			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}, {Kind: vmKind, Name: vm5Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}, {Kind: vmKind, Name: vm4Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}, {Kind: vmKind, Name: vm6Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm1Name, vm2Name, vm3Name})

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying vm3 powers off, and vm2 and vm1 are held while vm6 is not linked")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff)

			queued := metav1.Condition{
				Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
				Status: metav1.ConditionFalse,
				Reason: "Queued",
			}
			for _, name := range []string{vm2Name, vm1Name} {
				vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, name, vmKind, queued)
			}
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm6Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
					Status: metav1.ConditionFalse,
					Reason: "NotMember",
				})

			By("Unlinking vm2 from the VM side by clearing its spec.groupName")

			vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
			Expect(err).NotTo(HaveOccurred())
			vm2Patch := vm2.DeepCopy()
			vm2Patch.Spec.GroupName = ""
			Expect(svClusterClient.Patch(ctx, vm2Patch, ctrlclient.MergeFrom(vm2))).To(Succeed(), "failed to unlink vm2")
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm2Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
					Status: metav1.ConditionFalse,
					Reason: "NotMember",
				})

			By("Deleting vm1 while it is held, then creating it again")

			vm1Old, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(svClusterClient.Delete(ctx, vm1Old)).To(Succeed(), "failed to delete vm1")
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name)
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
					Status: metav1.ConditionFalse,
					Reason: "NotFound",
				})
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, []string{vm1Name})
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm1Name})

			By("Linking vm6 to the group")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm6Name})

			By("Verifying vm6 is set to PoweredOff, and vm2 and vm1 are held while vm2 and vm4 are not linked")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm6Name, "PoweredOff")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff)
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm4Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
					Status: metav1.ConditionFalse,
					Reason: "NotMember",
				})

			By("Linking vm2 and vm4 to the group")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm2Name, vm4Name})

			By("Verifying vm2 and vm4 power off, then vm1")

			verifyReleaseOrderGroups(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm3Name, vm6Name}, {vm2Name, vm4Name}, {vm1Name}}, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)

			By("Powering on vm1 while vm5 (missing) keeps the request open")

			vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			vm1Patch := vm1.DeepCopy()
			vm1Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vm1Patch, ctrlclient.MergeFrom(vm1))).To(Succeed(), "failed to power on vm1")

			By("Verifying the group sets vm1 to PoweredOff again")

			Eventually(func(g Gomega) {
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "1m", "2s").Should(Succeed(), "the group did not set vm1 back to PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOff")

			By("Removing vm5 from the boot order")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.BootOrder[0].Members = slices.DeleteFunc(g.Spec.BootOrder[0].Members, func(m vmopv1.GroupMember) bool {
					return m.Name == vm5Name
				})
			}, "failed to remove vm5 from the boot order")

			By("Verifying VirtualMachineGroup has Ready condition set to true")

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName,
				metav1.Condition{Type: vmopv1.ReadyConditionType, Status: metav1.ConditionTrue})
		})

		It("Should delete the group and its members while a missing member blocks the walk", func() {
			vm5Name := fmt.Sprintf("%s-vm5", vmgRootName)

			vmMemberNames = []string{vm1Name, vm2Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup where the group released first holds vm5 (missing)")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}, {Kind: vmKind, Name: vm5Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

			By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Hard power off mode")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying vm2 powers off and vm1 is held")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name, "PoweredOff")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm1Name}, vmopv1.VirtualMachinePowerStateOff)

			By("Deleting the VirtualMachineGroup while the walk is blocked")

			Expect(svClusterClient.Delete(ctx, vmgRoot)).To(Succeed(), "failed to delete VirtualMachineGroup")
			// The cleanup has nothing left to delete.
			rootGroup = nil

			By("Verifying the group is deleted, and its members are deleted through their owner references")

			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName)
			for _, name := range vmMemberNames {
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, name)
			}
		})

		It("Should hold later boot order groups until every member of the first group is powered on", func() {
			var (
				// vm1 has a power on check annotation, vm2 is suspended, vm4
				// is not linked to the group, and vm5 does not exist yet.
				vm5Name = fmt.Sprintf("%s-vm5", vmgRootName)

				vm3PowerOnDelay = 20 * time.Second
				// The delay is edited while vm3 is held, which applies
				// because vm3 is not released yet.
				vm3EditedPowerOnDelay = 40 * time.Second
			)

			// vm6 is created now and added as a new boot order group last.
			vm6Name := fmt.Sprintf("%s-vm6", vmgRootName)
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name, vm6Name}

			By("Creating powered off vm1 (with a power on check annotation), vm3, vm4 and vm6, and a suspended vm2")

			powerOnCheckAnnotation := "poweron.check.vmoperator.vmware.com/e2e"
			vm1 := newPoweredOffVM(vmTemplate, vm1Name, "")
			vm1.Annotations = map[string]string{powerOnCheckAnnotation: "blocked by the test"}

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{
						{Kind: vmKind, Name: vm1Name},
						{Kind: vmKind, Name: vm2Name},
						{Kind: vmKind, Name: vm4Name},
						{Kind: vmKind, Name: vm5Name},
					}},
					{
						PowerOnDelay: vm3PowerOnDelay.String(),
						Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			Expect(svClusterClient.Create(ctx, vm1)).To(Succeed(), "failed to create vm1")
			deferDeleteVM(svClusterClient, vm1)
			for _, name := range []string{vm3Name, vm4Name, vm6Name} {
				vm := newPoweredOffVM(vmTemplate, name, "")
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create %s", name)
				deferDeleteVM(svClusterClient, vm)
			}

			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, []string{vm2Name})
			vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
			Expect(err).NotTo(HaveOccurred())
			vm2Patch := vm2.DeepCopy()
			vm2Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			vm2Patch.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
			Expect(svClusterClient.Patch(ctx, vm2Patch, ctrlclient.MergeFrom(vm2))).To(Succeed(), "failed to suspend vm2")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name, "Suspended")

			// The VMs are linked only after they are created. A new VM that is
			// linked earlier waits for the group placement, which does not
			// finish while a member is missing or not linked.
			for _, name := range []string{vm1Name, vm3Name, vm4Name, vm6Name} {
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, name)
			}
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm1Name, vm2Name, vm3Name})

			By("Setting VirtualMachineGroup spec.powerState to PoweredOn")

			since := vcNow(ctx, vCenterClient)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			}, "failed to update VirtualMachineGroup power state")

			// vm4, vm5 and vm6 join the boot order while the walk runs.
			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm1Name, vm2Name, vm4Name, vm5Name}, {vm3Name}, {vm6Name}}, vmopv1.VirtualMachinePowerStateOn)

			By("Verifying vm2 is resumed, and vm3 is held and Queued while vm1, vm4 and vm5 are not on")

			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name, "PoweredOn")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm3Name}, vmopv1.VirtualMachinePowerStateOn)
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm3Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
					Status: metav1.ConditionFalse,
					Reason: "Queued",
				})

			By("Removing the power on check annotation from vm1 as a privileged user")

			// Only a privileged user can remove the annotation from a VM.
			adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to get the admin cluster proxy")
			DeferCleanup(adminClusterProxy.Dispose)
			adminClient, err := adminClusterProxy.GetAdminClient()
			Expect(err).NotTo(HaveOccurred(), "failed to get the admin client")

			vm1Latest, err := utils.GetVirtualMachine(ctx, adminClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			vm1Patch := vm1Latest.DeepCopy()
			delete(vm1Patch.Annotations, powerOnCheckAnnotation)
			Expect(adminClient.Patch(ctx, vm1Patch, ctrlclient.MergeFrom(vm1Latest))).To(Succeed(), "failed to remove the annotation from vm1")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOn")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm3Name}, vmopv1.VirtualMachinePowerStateOn)

			By("Linking vm4 to the group")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm4Name})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm4Name, "PoweredOn")
			verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm3Name}, vmopv1.VirtualMachinePowerStateOn)

			By("Editing the PowerOnDelay of vm3's boot order group, and adding vm6 as a new last boot order group with no delay, while vm3 is held")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.BootOrder[1].PowerOnDelay = &metav1.Duration{Duration: vm3EditedPowerOnDelay}
				g.Spec.BootOrder = append(g.Spec.BootOrder, vmopv1.VirtualMachineGroupBootOrderGroup{
					Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm6Name}},
				})
			}, "failed to edit the group")
			// vm6 can be linked only after the boot order has it.
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm6Name})

			By("Creating vm5 as a member of the group")

			vm5 := newPoweredOffVM(vmTemplate, vm5Name, "")
			Expect(svClusterClient.Create(ctx, vm5)).To(Succeed(), "failed to create vm5")
			deferDeleteVM(svClusterClient, vm5)
			vmMemberNames = append(vmMemberNames, vm5Name)
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, vm5Name)
			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, []string{vm5Name})

			By("Waiting for all VMs to power on, with vm3 released only after the first boot order group is on, and vm6 after vm3")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOn, 3*time.Minute)
			verifyOrder()

			By("Verifying vm3 powered on once after its edited delay, counted from when vm5 was on")

			// vm5 is the last VM of the first boot order group to power on.
			// vm2 is resumed, not powered on, so it has no event to check.
			vm5OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm5Name, since, vmopv1.VirtualMachinePowerStateOn)
			vm3OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOn)
			expectGap(vm3OnTime.Sub(vm5OnTime), vm3EditedPowerOnDelay, "vm3 on after vm5")
			vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm6Name, since, vmopv1.VirtualMachinePowerStateOn)

			By("Verifying VirtualMachineGroup has Ready condition set to true")

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName,
				metav1.Condition{Type: vmopv1.ReadyConditionType, Status: metav1.ConditionTrue})

			By("Powering off vm1 directly after the request is closed")

			vm1Closed, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).NotTo(HaveOccurred())
			vm1ClosedPatch := vm1Closed.DeepCopy()
			vm1ClosedPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm1ClosedPatch.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			Expect(svClusterClient.Patch(ctx, vm1ClosedPatch, ctrlclient.MergeFrom(vm1Closed))).To(Succeed(), "failed to power off vm1")

			By("Verifying the member is reported as not synced and the group does not power it on again")

			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm1Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
					Status: metav1.ConditionFalse,
					Reason: "NotSynced",
				})
			Consistently(func(g Gomega) {
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm1.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, 20*time.Second, "5s").Should(Succeed(), "the group powered on vm1 after the request was closed")
		})

		It("Should handle suspended members in the suspend, soft power off, power on and hard power off requests", func() {
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, vmMemberNames)

			By("Creating a VirtualMachineGroup with 3 boot order groups and no delays")

			vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
				Namespace: input.WCPNamespaceName,
				Name:      vmgRootName,
				BootOrder: []manifestbuilders.BootOrder{
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}}},
					{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}}},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
			rootGroup = vmgRoot
			Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

			linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

			By("Suspending the group, which suspends all members at once")

			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
				g.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to suspend the VirtualMachineGroup")
			for _, name := range vmMemberNames {
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, name, "Suspended")
			}

			By("Powering on the group, which resumes the suspended members in boot order")

			powerOnGroupInOrder(ctx, svClusterClient, groupKey, []string{vm1Name, vm2Name, vm3Name})

			By("Suspending vm3 directly, then powering off the group in Soft mode")

			suspendVM(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying the suspended vm3 counts as off, so vm2 and then vm1 are released, and vm3 stays suspended and not synced")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)
			vmoperator.WaitOnVirtualMachineGroupMemberCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vm3Name, vmKind,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced,
					Status: metav1.ConditionFalse,
				})
			Consistently(func(g Gomega) {
				vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm3.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended), "vm3 was powered off in Soft mode")
			}, 20*time.Second, "5s").Should(Succeed())

			By("Powering on the group again, then suspending vm3 directly and powering off the group in Hard mode")

			powerOnGroupInOrder(ctx, svClusterClient, groupKey, []string{vm1Name, vm2Name, vm3Name})
			suspendVM(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name)
			patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying vm3 is powered off, not left suspended, before vm2 and vm1")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm3Name, vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)
		})

		// These cases wait minutes for a guest shutdown, so they run in the
		// extended suite and not in core.
		When("a guest does not shut down", Label("extended-functional"), func() {
			const (
				// holdDuration is how long the test verifies the next boot
				// order group is not released.
				holdDuration = 2 * time.Minute

				// softTimeout is the default time VM Operator waits for a
				// guest shutdown in TrySoft mode before it powers off hard.
				softTimeout = 5 * time.Minute
			)

			BeforeEach(func() {
				vmMemberNames = []string{vm1Name, vm2Name}

				By("Creating vm2 with a guest that does not shut down, and vm1")

				secretName := vmgRootName + "-cloud-init"
				secret := manifestbuilders.InlineCloudInitSecret(manifestbuilders.Secret{
					Namespace: input.WCPNamespaceName,
					Name:      secretName,
				})
				Expect(svClusterClient.Create(ctx, secret)).To(Succeed(), "failed to create cloud-init Secret")
				DeferCleanup(func() {
					Expect(ctrlclient.IgnoreNotFound(svClusterClient.Delete(ctx, secret))).To(Succeed())
				})

				createPoweredOnVMs(ctx, config, svClusterClient, vmTemplate, []string{vm1Name})

				blockedVM := vmTemplate.DeepCopy()
				blockedVM.Spec.Bootstrap = &vmopv1a5.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1a5.VirtualMachineBootstrapCloudInitSpec{
						CloudConfig: shutdownBlockedCloudConfig(secretName),
					},
				}
				createPoweredOnVMs(ctx, config, svClusterClient, blockedVM, []string{vm2Name})

				By("Verifying vm2's guest does not shut down")

				vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, input.WCPNamespaceName, vm2Name)
				vm2IP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				// systemctl exits with status 1 for a masked unit, so the command ends with true.
				verifyGuestCmds(ctx, config, clusterProxy, svClusterClient, input.WCPNamespaceName, vm2IP,
					[]string{"systemctl is-enabled poweroff.target; true"}, []string{"masked"})

				By("Creating a VirtualMachineGroup with vm2 powering off before vm1")

				vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
					Namespace: input.WCPNamespaceName,
					Name:      vmgRootName,
					BootOrder: []manifestbuilders.BootOrder{
						{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
						{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}}},
					},
				})
				Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
				rootGroup = vmgRoot
				Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

				linkVMsToGroup(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)

				// A VM that is powered on or in a Soft power off waits for the
				// guest to shut down when it is deleted, which vm2's guest
				// never does. Switch to Hard first so that the cleanup, which
				// deletes the group and so its members, does not wait for the
				// 5 minute timeout of each VM. This cleanup runs before the
				// one that deletes the group, as cleanups run in reverse order.
				DeferCleanup(setPowerOffModeHard, ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, vmMemberNames)
			})

			It("Should hold the next boot order group in Soft mode until the guest is powered off", func() {
				By("Setting VirtualMachineGroup spec.powerState to PoweredOff with Soft power off mode")

				since := vcNow(ctx, vCenterClient)
				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
				}, "failed to update VirtualMachineGroup power state")

				By("Verifying vm2 stays on and vm1 is not released")

				Consistently(func(g Gomega) {
					vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(vm2.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

					vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(vm1.Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOff))
				}, holdDuration, "10s").Should(Succeed())

				By("Forcing a power state sync, which starts a new request and keeps the walk held")

				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.NextForcePowerStateSyncTime = "now"
				}, "failed to update VirtualMachineGroup force sync time")
				verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm1Name}, vmopv1.VirtualMachinePowerStateOff)

				By("Setting VirtualMachineGroup spec.powerState to PoweredOn and back to PoweredOff, which starts the walk again")

				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				}, "failed to update VirtualMachineGroup power state")
				// Wait until the power-on request is closed. A power-off that
				// is set while the controller still handles the power-on can
				// be closed by it, and then the walk does not start.
				vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName,
					metav1.Condition{Type: vmopv1.ReadyConditionType, Status: metav1.ConditionTrue})
				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				}, "failed to update VirtualMachineGroup power state")
				verifyHeld(ctx, svClusterClient, input.WCPNamespaceName, []string{vm1Name}, vmopv1.VirtualMachinePowerStateOff)

				By("Reordering bootOrder so vm1's group is released first and vm2's group last")

				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.BootOrder[0], g.Spec.BootOrder[1] = g.Spec.BootOrder[1], g.Spec.BootOrder[0]
				}, "failed to reorder the boot order")
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm1Name, "PoweredOff")
				// vCenter is checked and not the status, which can trail it.
				expectNoVMPowerTransition(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since)

				By("Changing the group's power off mode to Hard to get out of the stuck power-off")

				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
				}, "failed to update VirtualMachineGroup power off mode")

				By("Verifying vm2 powers off")

				// A guest shutdown wait that is already running is not
				// interrupted, so the Hard power off can wait for it to time
				// out first.
				waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, []string{vm2Name}, vmopv1.VirtualMachinePowerStateOff, softTimeout+2*time.Minute)
				vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
			})

			It("Should power off hard in TrySoft mode after the soft timeout, and only then release the next boot order group", func() {
				By("Setting VirtualMachineGroup spec.powerState to PoweredOff with TrySoft power off mode")

				since := vcNow(ctx, vCenterClient)
				patchVMGroup(ctx, svClusterClient, groupKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
					g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				}, "failed to update VirtualMachineGroup power state")

				verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
					[][]string{{vm2Name}, {vm1Name}}, vmopv1.VirtualMachinePowerStateOff)

				By("Verifying vm1 is not released while vm2's guest shutdown is pending")

				Consistently(func(g Gomega) {
					vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(vm1.Spec.PowerState).ToNot(Equal(vmopv1.VirtualMachinePowerStateOff))
				}, holdDuration, "10s").Should(Succeed())

				By("Verifying vm2 powers off hard after the soft timeout, then vm1")

				waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOff, softTimeout+3*time.Minute)
				verifyOrder()

				// The guest shutdown starts after the request, so vm2 powers
				// off at least the soft timeout after it. since is read before
				// the request, so no allowance for a late event is needed.
				vm2OffTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOff)
				Expect(vm2OffTime.Sub(since)).To(BeNumerically(">=", softTimeout), "vm2 powered off before the soft timeout")
				vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOff)
			})
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

			By("Verifying VM2 is released only after VM1 is on")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm1Name, vm2Name}, vmopv1.VirtualMachinePowerStateOn, 5*time.Minute)

			By("Verifying VM2 powered on once, after the delay of the child group counted from when VM1 was on")

			// The delay of the first group of the child (1m) must be ignored.
			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOn)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOn)
			expectGap(vm2OnTime.Sub(vm1OnTime), 30*time.Second, "VM2 on after VM1")
			expectDelayIgnored(vm2OnTime.Sub(vm1OnTime), 30*time.Second, time.Minute, "VM2 on after VM1")

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

		It("Should power on and off nested groups in order, with the child group counted as on or off only when all its VMs are", func() {
			const (
				childPowerOnDelay      = 20 * time.Second
				ignoredFirstGroupDelay = 90 * time.Second
			)

			// root: [vm1], [child] (delay), child: [vm2], [grandchild], grandchild: [vm3].
			grandchildName := fmt.Sprintf("%s-grandchild", vmgRootName)
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}

			By("Creating the root, child and grandchild VirtualMachineGroups")

			rootGroup = newVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, "", []manifestbuilders.BootOrder{
				{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
				{
					Members:      []vmopv1.GroupMember{{Kind: vmgKind, Name: vmgChildName}},
					PowerOnDelay: childPowerOnDelay.String(),
				},
			})
			newVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgChildName, vmgRootName, []manifestbuilders.BootOrder{
				{
					// The child's first group is released when the child is,
					// so this delay is ignored.
					Members:      []vmopv1.GroupMember{{Kind: vmKind, Name: vm2Name}},
					PowerOnDelay: ignoredFirstGroupDelay.String(),
				},
				{Members: []vmopv1.GroupMember{{Kind: vmgKind, Name: grandchildName}}},
			})
			newVMGroup(ctx, svClusterClient, input.WCPNamespaceName, grandchildName, vmgChildName, []manifestbuilders.BootOrder{
				{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm3Name}}},
			})

			By("Creating powered off VMs in their groups")

			for name, group := range map[string]string{vm1Name: vmgRootName, vm2Name: vmgChildName, vm3Name: grandchildName} {
				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Namespace: input.WCPNamespaceName, Name: name},
					Spec: vmopv1a5.VirtualMachineSpec{
						GroupName:    group,
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", name)
				deferDeleteVM(svClusterClient, vm)
			}
			for _, name := range vmMemberNames {
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, name)
			}

			By("Setting root VirtualMachineGroup spec.powerState to PoweredOn")

			since := vcNow(ctx, vCenterClient)
			rootKey := ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmgRootName}
			patchVMGroup(ctx, svClusterClient, rootKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			}, "failed to update VirtualMachineGroup power state")

			verifyOrder := watchReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[][]string{{vm1Name}, {vm2Name}, {vm3Name}}, vmopv1.VirtualMachinePowerStateOn)

			By("Verifying the parent shows the child group as Pending, naming the VM that is not on, while the delay runs")

			// The child is Queued only while vm1 powers on, which can be too
			// short to see. The order watch checks it is held.
			Eventually(func(g Gomega) {
				c := groupMemberPowerStateSynced(ctx, svClusterClient, input.WCPNamespaceName, vmgRootName, vmgChildName, vmgKind)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Reason).To(Equal("Pending"))
				g.Expect(c.Message).To(ContainSubstring(vm2Name))
			}, "2m", "1s").Should(Succeed(), "the child group was never reported as Pending on vm2")

			By("Verifying vm1 powers on first, then vm2, then vm3, across the nested groups")

			waitForPowerStates(ctx, svClusterClient, input.WCPNamespaceName, vmMemberNames, vmopv1.VirtualMachinePowerStateOn, 5*time.Minute)
			verifyOrder()

			By("Verifying vm2 powers on after the delay of the child group, counted from when vm1 was on, and not after its own delay")

			// The delay of the first group of the child must be ignored.
			vm1OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name, since, vmopv1.VirtualMachinePowerStateOn)
			vm2OnTime := vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name, since, vmopv1.VirtualMachinePowerStateOn)
			vmPowerChangeTime(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm3Name, since, vmopv1.VirtualMachinePowerStateOn)
			expectGap(vm2OnTime.Sub(vm1OnTime), childPowerOnDelay, "vm2 on after vm1")
			expectDelayIgnored(vm2OnTime.Sub(vm1OnTime), childPowerOnDelay, ignoredFirstGroupDelay, "vm2 on after vm1")

			By("Verifying the parent reports the child group as synced and ready")

			vmoperator.WaitOnVirtualMachineGroupCondition(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName,
				metav1.Condition{Type: vmopv1.ReadyConditionType, Status: metav1.ConditionTrue})

			By("Adding a missing member to vm1's boot order group, and setting root spec.powerState to PoweredOff with Hard power off mode")

			// The missing member keeps the root request open after the
			// VMs are off, so a later step can power a descendant on.
			patchVMGroup(ctx, svClusterClient, rootKey, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
				g.Spec.BootOrder[0].Members = append(g.Spec.BootOrder[0].Members,
					vmopv1.GroupMember{Kind: vmKind, Name: fmt.Sprintf("%s-ghost", vmgRootName)})
				g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				g.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
			}, "failed to update VirtualMachineGroup power state")

			By("Verifying vm3 (grandchild) powers off first, then vm2 (child), then vm1 (root)")

			verifyReleaseOrder(ctx, svClusterClient, input.WCPNamespaceName,
				[]string{vm3Name, vm2Name, vm1Name}, vmopv1.VirtualMachinePowerStateOff, 3*time.Minute)

			By("Powering on vm3 directly while the root request is still pending")

			vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
			Expect(err).NotTo(HaveOccurred())
			vm3Patch := vm3.DeepCopy()
			vm3Patch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vm3Patch, ctrlclient.MergeFrom(vm3))).To(Succeed(), "failed to power on vm3")

			By("Verifying the parent starts a new request on the child, which sets vm3 to PoweredOff again")

			Eventually(func(g Gomega) {
				vm3, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm3Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm3.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			}, "2m", "2s").Should(Succeed(), "the group did not set vm3 back to PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vm3Name, "PoweredOff")
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

			asyncCapabilities, err := utils.CheckSupervisorCapabilitiesCRDSupport(ctx, svClusterClient)
			Expect(err).NotTo(HaveOccurred())
			if utils.IsSupervisorCapabilityEnabled(ctx, svClusterClient,
				consts.VMHardAffinityDuringExecutionCapabilityName, asyncCapabilities) {
				restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { Expect(restClient.Logout(ctx)).To(Succeed()) })
				waitForZonalAffinityAndAntiAffinityTags(ctx, config, input.WCPClient, svClusterClient, vCenterClient,
					tags.NewManager(restClient), tmpNamespaceName, vmMemberNames)
			}
		})

		It("Should place VMs without a group on the same host using host affinity", Label("experimental", consts.HardAffinityDuringExecutionLabel), func() {
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			appLabels := map[string]string{"app": vmgRootName}

			for _, vmName := range vmMemberNames {
				By(fmt.Sprintf("Creating VM %s without a group, with required host affinity to VMs labeled %v", vmName, appLabels))

				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: tmpNamespaceName,
						Name:      vmName,
						Labels:    appLabels,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						ImageName:    tmpNamespaceVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOn,
						Affinity:     requiredAffinity(hostTopoKey, appLabels),
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)

				// Ungrouped VMs are placed independently, so wait for each VM
				// to power on and get an IP before creating the next one that
				// is affine to it.
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
			}

			By("Verifying the VMs are on the same host")

			host1 := getVMHostFromVmodl(ctx, svClusterClient, vCenterClient, tmpNamespaceName, vm1Name)
			for _, vmName := range []string{vm2Name, vm3Name} {
				Expect(getVMHostFromVmodl(ctx, svClusterClient, vCenterClient, tmpNamespaceName, vmName)).To(Equal(host1), "vm %s is not on the same host as %s", vmName, vm1Name)
			}
		})

		It("Should place VMs without a group on different hosts using host anti-affinity", Label("experimental", consts.HardAffinityDuringExecutionLabel), func() {
			vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
			appLabels := map[string]string{"app": vmgRootName}

			for _, vmName := range vmMemberNames {
				By(fmt.Sprintf("Creating VM %s without a group, with required host anti-affinity to VMs labeled %v", vmName, appLabels))

				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: tmpNamespaceName,
						Name:      vmName,
						Labels:    appLabels,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						ImageName:    tmpNamespaceVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOn,
						Affinity:     requiredAntiAffinity(hostTopoKey, appLabels),
					},
				}
				Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)

				// Ungrouped VMs are placed independently, so wait for each VM
				// to power on and get an IP before creating the next one that
				// is anti-affine to it.
				vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, tmpNamespaceName, vmName)
			}

			By("Verifying the VMs are on different hosts")

			hosts := map[string]string{}
			for _, vmName := range vmMemberNames {
				host := getVMHostFromVmodl(ctx, svClusterClient, vCenterClient, tmpNamespaceName, vmName)
				Expect(hosts).ToNot(HaveKey(host), "vms %s and %s are on the same host %s", hosts[host], vmName, host)
				hosts[host] = vmName
			}
		})

		When("VMs are pinned to zones via the topology zone label", Label("experimental", consts.HardAffinityDuringExecutionLabel), func() {
			var (
				zones       []string
				tinyVMIName string
			)

			BeforeEach(func() {
				skipper.SkipUnlessStretchSupervisorIsEnabled()

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

			It("Should persist placement errors on root and child group members with conflicting zone affinity", func() {
				By("Creating root and child groups with one VM each")
				vmMemberNames = []string{vm1Name, vm2Name}
				vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(
					manifestbuilders.VirtualMachineGroupYaml{
						Namespace: tmpNamespaceName,
						Name:      vmgRootName,
						BootOrder: []manifestbuilders.BootOrder{{Members: []vmopv1.GroupMember{
							{Kind: vmKind, Name: vm1Name},
							{Kind: vmgKind, Name: vmgChildName},
						}}},
					})
				Expect(err).NotTo(HaveOccurred(), "failed to build root VirtualMachineGroup")
				rootGroup = vmgRoot
				Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed())
				vmgChild, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(
					manifestbuilders.VirtualMachineGroupYaml{
						Namespace: tmpNamespaceName,
						Name:      vmgChildName,
						GroupName: vmgRootName,
						BootOrder: []manifestbuilders.BootOrder{{Members: []vmopv1.GroupMember{
							{Kind: vmKind, Name: vm2Name},
						}}},
					})
				Expect(err).NotTo(HaveOccurred(), "failed to build child VirtualMachineGroup")
				Expect(svClusterClient.Create(ctx, vmgChild)).To(Succeed())

				By("Pinning the VMs to different zones while requiring zone affinity to each other")
				appLabels := map[string]string{"app": vmgRootName}
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName,
					[]zonePinnedVM{{Name: vm1Name, Zone: zones[0], Labels: appLabels, Affinity: requiredAffinity(zoneLabelKey, appLabels)}})
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgChildName, tinyVMIName,
					[]zonePinnedVM{{Name: vm2Name, Zone: zones[1], Labels: appLabels, Affinity: requiredAffinity(zoneLabelKey, appLabels)}})

				By("Verifying both groups persist member-specific placement failures")
				const zoneConflictFault = "VMs that are affined together specify conflicting candidate vSphere Zones ('%s' vs '%s')."
				expectedFault := Or(
					ContainSubstring(fmt.Sprintf(zoneConflictFault, zones[0], zones[1])),
					ContainSubstring(fmt.Sprintf(zoneConflictFault, zones[1], zones[0])),
				)
				Eventually(func(g Gomega) {
					for _, member := range []struct{ groupName, vmName string }{
						{vmgRootName, vm1Name},
						{vmgChildName, vm2Name},
					} {
						ms, err := utils.GetVirtualMachineGroupMemberStatus(ctx, svClusterClient,
							tmpNamespaceName, member.groupName, member.vmName, vmKind)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(ms.Conditions).To(ContainElement(And(
							HaveField("Type", vmopv1.VirtualMachineGroupMemberConditionPlacementReady),
							HaveField("Status", metav1.ConditionFalse),
							HaveField("Reason", "PendingPlacement"),
							HaveField("Message", expectedFault),
						)), "expected conflicting-zone fault for VM %s in group %s", member.vmName, member.groupName)
					}
				}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
			})

			It("Should place group members in the distinct zones pinned via the topology zone label", func() {
				vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
				pinnedZones := make(map[string]string, len(vmMemberNames))
				for i, vmName := range vmMemberNames {
					pinnedZones[vmName] = zones[i]
				}

				By("Creating a VirtualMachineGroup with 3 VM-kind members")

				vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
					Namespace: tmpNamespaceName,
					Name:      vmgRootName,
					BootOrder: []manifestbuilders.BootOrder{
						{
							Members: []vmopv1.GroupMember{
								{Kind: vmKind, Name: vm1Name},
								{Kind: vmKind, Name: vm2Name},
								{Kind: vmKind, Name: vm3Name},
							},
						},
					},
				}
				vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(vmgParameters)
				Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
				rootGroup = vmgRoot
				Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

				By("Creating each VM with a different pinned zone and host affinity only to itself")

				for _, vmName := range vmMemberNames {
					vm := &vmopv1a5.VirtualMachine{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: tmpNamespaceName,
							Name:      vmName,
							Labels: map[string]string{
								"topology.kubernetes.io/zone": pinnedZones[vmName],
								"vm":                          vmName,
							},
						},
						Spec: vmopv1a5.VirtualMachineSpec{
							GroupName:    vmgRootName,
							ImageName:    tinyVMIName,
							ClassName:    clusterResources.VMClassName,
							StorageClass: clusterResources.StorageClassName,
							PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
							Affinity: &vmopv1a5.AffinitySpec{
								VMAffinity: &vmopv1a5.VMAffinitySpec{
									RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"vm": vmName},
											},
											TopologyKey: "kubernetes.io/hostname",
										},
									},
								},
							},
						},
					}
					Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vm.Name)
				}

				By("Waiting for all VMs to be created")

				for _, vmName := range vmMemberNames {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vmName)
				}

				By("Verifying each VM landed in its pinned zone")

				placedZones := make(map[string]struct{}, len(vmMemberNames))
				for _, vmName := range vmMemberNames {
					vmZone := waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vmName)
					Expect(vmZone).To(Equal(pinnedZones[vmName]), "vm %s zone mismatch", vmName)
					placedZones[vmZone] = struct{}{}
				}

				By("Verifying the VMs are in 3 separate zones")
				Expect(placedZones).To(HaveLen(len(vmMemberNames)))
			})

			It("Should place each member in its own pinned zone regardless of member order", func() {
				By("Creating 6 VMs in interleaved zone order, one of them unpinned")

				vms := []zonePinnedVM{
					{Name: fmt.Sprintf("%s-vm1", vmgRootName), Zone: zones[2]},
					{Name: fmt.Sprintf("%s-vm2", vmgRootName), Zone: zones[0]},
					{Name: fmt.Sprintf("%s-vm3", vmgRootName)},
					{Name: fmt.Sprintf("%s-vm4", vmgRootName), Zone: zones[1]},
					{Name: fmt.Sprintf("%s-vm5", vmgRootName), Zone: zones[0]},
					{Name: fmt.Sprintf("%s-vm6", vmgRootName), Zone: zones[2]},
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
				}

				By("Verifying each pinned VM landed in its own zone and the unpinned VM in a namespace zone")

				for _, vm := range vms {
					zone := waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					if vm.Zone != "" {
						Expect(zone).To(Equal(vm.Zone), "vm %s zone mismatch", vm.Name)
					} else {
						Expect(zones).To(ContainElement(zone), "unpinned vm %s zone", vm.Name)
					}
				}
			})

			It("Should place pinned members in their zones when another member is unpinned", func() {
				By("Creating 2 VMs pinned to different zones and 1 unpinned VM")

				vms := []zonePinnedVM{
					{Name: vm1Name, Zone: zones[0]},
					{Name: vm2Name, Zone: zones[1]},
					{Name: vm3Name},
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
				}

				Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)).To(Equal(zones[0]))
				Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm2Name)).To(Equal(zones[1]))
				Expect(zones).To(ContainElement(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm3Name)))
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

			It("Should place members pinned to different zones that also have zone anti-affinity", func() {
				By("Creating 3 VMs pinned to different zones with required zone anti-affinity to each other")

				appLabels := map[string]string{"app": vmgRootName}
				vms := []zonePinnedVM{
					{Name: vm1Name, Zone: zones[0], Labels: appLabels, Affinity: requiredAntiAffinity(zoneLabelKey, appLabels)},
					{Name: vm2Name, Zone: zones[1], Labels: appLabels, Affinity: requiredAntiAffinity(zoneLabelKey, appLabels)},
					{Name: vm3Name, Zone: zones[2], Labels: appLabels, Affinity: requiredAntiAffinity(zoneLabelKey, appLabels)},
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					Expect(waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)).To(Equal(vm.Zone), "vm %s zone mismatch", vm.Name)
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

			It("Should place all members in their pinned zones when each zone has more members than hosts with preferred host anti-affinity", func() {
				const vmCountPerZone = 5

				zoneHostInfos, err := utils.GetHostsPerZone(ctx, clusterProxy.GetClient(), clusterProxy.GetKubeconfigPath())
				Expect(err).NotTo(HaveOccurred(), "failed to list zones with hosts")

				var smallZones []string
				for _, info := range zoneHostInfos {
					if len(info.HostIDs) >= 1 && len(info.HostIDs) < vmCountPerZone {
						smallZones = append(smallZones, info.ZoneName)
					}
				}
				if len(smallZones) < 2 {
					Skip(fmt.Sprintf("skip the test as fewer than 2 zones have fewer than %d hosts", vmCountPerZone))
				}
				pinnedZones := smallZones[:2]

				By(fmt.Sprintf("Creating %d VMs in each of zones %v with preferred host anti-affinity", vmCountPerZone, pinnedZones))

				var vms []zonePinnedVM
				for zi, zone := range pinnedZones {
					appLabels := map[string]string{"app": fmt.Sprintf("%s-zone%d", vmgRootName, zi+1)}
					for i := range vmCountPerZone {
						vms = append(vms, zonePinnedVM{
							Name:     fmt.Sprintf("%s-z%d-vm%d", vmgRootName, zi+1, i+1),
							Zone:     zone,
							Labels:   appLabels,
							Affinity: preferredAntiAffinity(hostTopoKey, appLabels),
						})
					}
				}
				vmMemberNames = getZonePinnedVMNames(vms)
				rootGroup = createVMGroupWithVMs(ctx, clusterProxy, tmpNamespaceName, vmgRootName, vms)
				createZonePinnedVMs(ctx, clusterProxy, clusterResources, tmpNamespaceName, vmgRootName, tinyVMIName, vms)

				zoneCounts := map[string]int{}
				for _, vm := range vms {
					vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					zone := waitForVMZone(ctx, config, svClusterClient, tmpNamespaceName, vm.Name)
					Expect(zone).To(Equal(vm.Zone), "vm %s zone mismatch", vm.Name)
					zoneCounts[zone]++
				}
				for _, zone := range pinnedZones {
					Expect(zoneCounts[zone]).To(Equal(vmCountPerZone), "zone %s VM count mismatch", zone)
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
				vmHardAffinityDuringExecutionEnabled := config.SupervisorCapabilities[consts.VMHardAffinityDuringExecutionCapabilityName]

				verifyTagsFunc := vmservice.VerifyVMTagsAndPolicyAssignment
				if vmHardAffinityDuringExecutionEnabled {
					// Zonal policies are persisted with the new affinity tags.
					waitForZonalAffinityAndAntiAffinityTags(ctx, config, input.WCPClient, svClusterClient, vCenterClient,
						tagManager, tmpNamespaceName, vmMemberNames)
					verifyTagsFunc = vmservice.VerifyVMTagsContainPolicyAssignment
				}

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
			requiredDuringSchedulingRequiredDuringExecution   = "requiredDuringSchedulingRequiredDuringExecution"
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
				case requiredDuringSchedulingRequiredDuringExecution:
					affinityLabelSelector = &vmopv1.VMAffinitySpec{
						RequiredDuringSchedulingRequiredDuringExecution: terms,
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
				case requiredDuringSchedulingRequiredDuringExecution:
					antiAffinityLabelSelector = &vmopv1.VMAntiAffinitySpec{
						RequiredDuringSchedulingRequiredDuringExecution: terms,
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

			asyncCapabilities, err := utils.CheckSupervisorCapabilitiesCRDSupport(ctx, svClusterClient)
			Expect(err).NotTo(HaveOccurred())
			vmHardAffinityDuringExecution := utils.IsSupervisorCapabilityEnabled(ctx, svClusterClient,
				consts.VMHardAffinityDuringExecutionCapabilityName, asyncCapabilities)
			var tagManager *tags.Manager
			if vmHardAffinityDuringExecution {
				restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { Expect(restClient.Logout(ctx)).To(Succeed()) })
				tagManager = tags.NewManager(restClient)
			}
			tierVMs := make(map[string]string)
			if len(affinedVms) > 0 {
				tierVMs["tier:1"] = affinedVms[0]
			}
			if len(antiAffinedVms) > 0 {
				tierVMs["tier:2"] = antiAffinedVms[0]
			}
			strictness := map[string]string{
				preferredDuringSchedulingPreferredDuringExecution: "pds-pde",
				requiredDuringSchedulingPreferredDuringExecution:  "rds-pde",
				requiredDuringSchedulingRequiredDuringExecution:   "rds-rde",
			}[affinityType]
			topology := map[string]string{hostTopoKey: "host", zoneLabelKey: "zone"}[topologyKey]
			Expect(strictness).NotTo(BeEmpty())
			Expect(topology).NotTo(BeEmpty())
			for tagLabel, vmName := range tierVMs {
				kind, suffix := "af", "-"+regexp.QuoteMeta(tagLabel)
				if tagLabel == "tier:2" {
					kind, suffix = "aaf", ""
				}
				policyTagPattern := regexp.MustCompile(fmt.Sprintf(
					`^vmoperator:%s-%s-%s-[0-9a-f]{16}%s$`, kind, strictness, topology, suffix))
				Eventually(func(g Gomega) {
					policyTag := tagLabel
					if vmHardAffinityDuringExecution {
						vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vmName)
						g.Expect(err).NotTo(HaveOccurred())
						vcVM := object.NewVirtualMachine(vCenterClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID})
						names, err := affinityTierTags(ctx, tagManager, vcVM, tmpNamespaceName)
						g.Expect(err).NotTo(HaveOccurred())
						originalTag := "vmoperator:" + tagLabel
						// Resolve the actual hashed AF target or AAF self tag. Original
						// tags can be referenced by other policies and are not the
						// policy identity when VMHardAffinityDuringExecution is enabled.
						policyTag = ""
						for _, name := range names {
							if policyTagPattern.MatchString(name) {
								g.Expect(policyTag).To(BeEmpty(), "multiple matching policy tags on VM %s: %v", vmName, names)
								policyTag = name
							}
						}
						g.Expect(policyTag).NotTo(BeEmpty(), "expected policy tag matching %s on VM %s; got %v", policyTagPattern, vmName, names)
						g.Expect(names).To(ConsistOf(originalTag, policyTag))
					}
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

			if affinityType == requiredDuringSchedulingRequiredDuringExecution {
				By("Verifying each VM is COMPLIANT with its policy")
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
						Should(Equal("COMPLIANT"), "expected VM %s to be compliant with policy %s", vmName, policyID)
				}

				By("Verifying placement now that the policies are compliant")
				postPowerOnLocations := make(map[string]string, len(vmMemberNames))
				for _, vmName := range vmMemberNames {
					if topologyKey == zoneLabelKey {
						postPowerOnLocations[vmName] = getVMZoneFunc(vmName)
					} else {
						postPowerOnLocations[vmName] = getVMHostFunc(vmName)
					}
				}
				verifyAffinity(postPowerOnLocations, affinedVms)
				verifyAntiAffinity(postPowerOnLocations, antiAffinedVms)
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
			DescribeTable("updates affinity and tags", Label("experimental", consts.HardAffinityDuringExecutionLabel), func(powerState vmopv1.VirtualMachinePowerState, tier string) {
				By("Creating a VM with preferred host affinity")
				vmMemberNames = []string{vm1Name}
				runVmVmAffinityAtHostTopoTest(preferredDuringSchedulingPreferredDuringExecution, vmMemberNames, nil)
				restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { Expect(restClient.Logout(ctx)).To(Succeed()) })
				tagManager := tags.NewManager(restClient)

				By(fmt.Sprintf("Setting the VM power state to %s", powerState))
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before := vm.DeepCopy()
				vm.Spec.PowerState = powerState
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vm1Name, vm.Spec.PowerState)

				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())

				By("Rejecting malformed anti-affinity selector keys and values")
				for _, selector := range []metav1.LabelSelector{
					{MatchLabels: map[string]string{"bad:key": "target"}},
					{MatchLabels: map[string]string{"tier": "bad:value"}},
					{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "bad:key", Operator: metav1.LabelSelectorOpIn, Values: []string{"target"}}}},
					{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"target", "bad:value"}}}},
				} {
					invalidVM := vm.DeepCopy()
					invalidVM.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
						PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
							TopologyKey: hostTopoKey, LabelSelector: &selector,
						}},
					}}
					err := svClusterClient.Patch(ctx, invalidVM, ctrlclient.MergeFrom(vm), ctrlclient.DryRunAll)
					Expect(err).To(MatchError(And(
						ContainSubstring("spec.affinity.vmAntiAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector"),
						ContainSubstring("Invalid value:"),
					)))
				}

				By("Changing the tier label and preferred host affinity together")
				before = vm.DeepCopy()
				vm.Labels["tier"] = tier
				vm.Spec.Affinity = &vmopv1.AffinitySpec{VMAffinity: &vmopv1.VMAffinitySpec{
					PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{
						TopologyKey:   "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": tier}},
					}},
				}}
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				vcVM := object.NewVirtualMachine(vCenterClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID})
				previousAffinityTag := waitForPreferredHostAffinityApplied(ctx, config, svClusterClient,
					input.WCPClient, tagManager, vcVM, vm, tier)

				By("Allowing unrelated label patches while preserving affinity")
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				vm.Labels["unrelated"] = "value"
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(vm.Labels).To(HaveKeyWithValue("unrelated", "value"))
				Expect(vm.Spec.Affinity).To(Equal(before.Spec.Affinity))

				By("Pausing the VM as an administrator before changing affinity")
				pauseTask, err := vcVM.Reconfigure(ctx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{&types.OptionValue{Key: vmopv1.PauseVMExtraConfigKey, Value: "TRUE"}},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(pauseTask.Wait(ctx)).To(Succeed())
				adminPaused := true
				DeferCleanup(func() {
					if adminPaused {
						task, err := vcVM.Reconfigure(ctx, types.VirtualMachineConfigSpec{
							ExtraConfig: []types.BaseOptionValue{&types.OptionValue{Key: vmopv1.PauseVMExtraConfigKey, Value: "FALSE"}},
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())
					}
				})
				waitForVMAdminPaused(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)
				var pausedVM mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &pausedVM)).To(Succeed())
				pausedPolicy, _ := object.OptionValueList(pausedVM.Config.ExtraConfig).GetString("vmservice.placementpolicies")
				pausedTags, _ := object.OptionValueList(pausedVM.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")

				By("Changing host affinity from preferred to required while the VM is paused")
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				af := vm.Spec.Affinity.VMAffinity
				af.RequiredDuringSchedulingPreferredDuringExecution = af.PreferredDuringSchedulingPreferredDuringExecution
				af.PreferredDuringSchedulingPreferredDuringExecution = nil
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				expectAffinityStateUnchangedWhilePaused(ctx, config, input.WCPClient, tagManager, vcVM, tmpNamespaceName,
					pausedPolicy, pausedTags, previousAffinityTag, tier)

				By("Unpausing the VM and applying the pending affinity update")
				resumeTask, err := vcVM.Reconfigure(ctx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{&types.OptionValue{Key: vmopv1.PauseVMExtraConfigKey, Value: "FALSE"}},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resumeTask.Wait(ctx)).To(Succeed())
				adminPaused = false
				waitForRequiredHostAffinityApplied(ctx, config, input.WCPClient, tagManager, vcVM, tmpNamespaceName,
					tier, previousAffinityTag)

				By("Removing affinity and verifying its tags are detached")
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				vm.Spec.Affinity = nil
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				waitForAffinityStateRemoved(ctx, config, tagManager, vcVM, tmpNamespaceName)

				By("Adding grouped anti-affinity without matching target labels")
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				vm.Spec.Affinity = &vmopv1.AffinitySpec{VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
					PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{{TopologyKey: hostTopoKey, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "target"}}}},
				}}
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				vcVM = object.NewVirtualMachine(vCenterClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID})
				previousAntiAffinitySelfTag := waitForHostAntiAffinityApplied(ctx, config, input.WCPClient, tagManager,
					vcVM, tmpNamespaceName, "target", "")

				By("Changing the anti-affinity target and verifying its self tag is replaced")
				vm, err = utils.GetVirtualMachine(ctx, svClusterClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				vm.Spec.Affinity.VMAntiAffinity.PreferredDuringSchedulingPreferredDuringExecution[0].LabelSelector.MatchLabels["tier"] = "different"
				Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				waitForHostAntiAffinityApplied(ctx, config, input.WCPClient, tagManager,
					vcVM, tmpNamespaceName, "different", previousAntiAffinitySelfTag)

				By("Removing managed affinity state during retained-VM cleanup")
				adminProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { adminProxy.Dispose(ctx) })
				adminClient, err := adminProxy.GetAdminClient()
				Expect(err).NotTo(HaveOccurred())
				// Restore ordinary deletion if the test fails before retained cleanup.
				// Once the CR is gone, explicitly destroy the retained test VM.
				DeferCleanup(func() {
					current, err := utils.GetVirtualMachine(ctx, adminClient, tmpNamespaceName, vm1Name)
					if err == nil {
						base := current.DeepCopy()
						delete(current.Annotations, "vmoperator.vmware.com.protected/skip-delete-platform-resource")
						Expect(adminClient.Patch(ctx, current, ctrlclient.MergeFrom(base))).To(Succeed())
						vmoperator.DeleteVirtualMachineAndWait(ctx, config, adminClient, tmpNamespaceName, vm1Name)
						return
					}
					Expect(apierrors.IsNotFound(err)).To(BeTrue())
					retainedPowerState, err := vcVM.PowerState(ctx)
					Expect(err).NotTo(HaveOccurred())
					if retainedPowerState != types.VirtualMachinePowerStatePoweredOff {
						powerOffTask, err := vcVM.PowerOff(ctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(powerOffTask.Wait(ctx)).To(Succeed())
					}
					task, err := vcVM.Destroy(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(task.Wait(ctx)).To(Succeed())
				})
				vm, err = utils.GetVirtualMachine(ctx, adminClient, tmpNamespaceName, vm1Name)
				Expect(err).NotTo(HaveOccurred())
				before = vm.DeepCopy()
				if vm.Annotations == nil {
					vm.Annotations = map[string]string{}
				}
				vm.Annotations["vmoperator.vmware.com.protected/skip-delete-platform-resource"] = ""
				Expect(adminClient.Patch(ctx, vm, ctrlclient.MergeFrom(before))).To(Succeed())
				vmoperator.DeleteVirtualMachineAndWait(ctx, config, adminClient, tmpNamespaceName, vm1Name)
				waitForRetainedVMAffinityCleanup(ctx, config, tagManager, vcVM, tmpNamespaceName)
			},
				Entry("on a running VM", vmopv1.VirtualMachinePowerStateOn, "2"),
				Entry("on a powered-off VM", vmopv1.VirtualMachinePowerStateOff, "3"),
			)

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

			When("VMs specify requiredDuringSchedulingRequiredDuringExecution", Label("experimental", consts.HardAffinityDuringExecutionLabel), func() {
				BeforeEach(func() {
					skipper.SkipUnlessStretchSupervisorIsEnabled()
				})

				var (
					// adminClient is required to manage the policies, which needs
					// more than Supervisor admin privileges.
					adminClient ctrlclient.Client
					policy      *vspherepolv1.RequiredDuringExecutionVMPlacementPolicy
				)

				BeforeEach(func() {
					adminProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
					Expect(err).ToNot(HaveOccurred(), "failed to get admin cluster proxy")
					DeferCleanup(func() { adminProxy.Dispose(ctx) })

					adminClient, err = adminProxy.GetAdminClient()
					Expect(err).ToNot(HaveOccurred(), "failed to get admin client")

					policy = &vspherepolv1.RequiredDuringExecutionVMPlacementPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "e2e-required-during-execution",
							Namespace: tmpNamespaceName,
						},
					}
				})

				When("the namespace has no RequiredDuringExecutionVMPlacementPolicy", func() {
					BeforeEach(func() {
						By("Ensuring the namespace has no RequiredDuringExecutionVMPlacementPolicy")
						Expect(adminClient.DeleteAllOf(ctx,
							&vspherepolv1.RequiredDuringExecutionVMPlacementPolicy{},
							ctrlclient.InNamespace(tmpNamespaceName))).To(Succeed(),
							"failed to delete RequiredDuringExecutionVMPlacementPolicies")
						Eventually(func(g Gomega) {
							var list vspherepolv1.RequiredDuringExecutionVMPlacementPolicyList
							g.Expect(adminClient.List(ctx, &list, ctrlclient.InNamespace(tmpNamespaceName))).To(Succeed())
							g.Expect(list.Items).To(BeEmpty())
						}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
					})

					DescribeTable("Should reject a VM with required-required terms",
						func(topologyKey string, antiAffinity bool) {
							vm := withRequiredExecutionAffinity(&vmopv1.VirtualMachine{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: tmpNamespaceName,
									Name:      vm1Name,
								},
								Spec: vmopv1.VirtualMachineSpec{
									GroupName:    vmgRootName,
									ImageName:    tmpNamespaceVMIName,
									ClassName:    clusterResources.VMClassName,
									StorageClass: clusterResources.StorageClassName,
									PowerState:   vmopv1.VirtualMachinePowerStateOff,
								},
							}, topologyKey, antiAffinity)

							err := svClusterClient.Create(ctx, vm)
							Expect(err).To(HaveOccurred(), "VM creation should be rejected without a RequiredDuringExecutionVMPlacementPolicy")
							Expect(err.Error()).To(ContainSubstring("requires a RequiredDuringExecutionVMPlacementPolicy in the namespace"))
						},
						Entry("host affinity", hostTopoKey, false),
						Entry("host anti-affinity", hostTopoKey, true),
						Entry("zone affinity", zoneLabelKey, false),
						Entry("zone anti-affinity", zoneLabelKey, true),
					)

					It("Should allow updating the spec of a VM without required-required terms", func() {
						By("Creating a VirtualMachineGroup and a VM without required-required terms")
						vmMemberNames = []string{vm1Name}
						vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(manifestbuilders.VirtualMachineGroupYaml{
							Namespace: tmpNamespaceName,
							Name:      vmgRootName,
							BootOrder: []manifestbuilders.BootOrder{
								{Members: []vmopv1.GroupMember{{Kind: vmKind, Name: vm1Name}}},
							},
						})
						Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
						rootGroup = vmgRoot
						Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")
						vm := &vmopv1.VirtualMachine{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: tmpNamespaceName,
								Name:      vm1Name,
							},
							Spec: vmopv1.VirtualMachineSpec{
								GroupName:    vmgRootName,
								ImageName:    tmpNamespaceVMIName,
								ClassName:    clusterResources.VMClassName,
								StorageClass: clusterResources.StorageClassName,
								PowerState:   vmopv1.VirtualMachinePowerStateOff,
							},
						}
						Expect(svClusterClient.Create(ctx, vm)).To(Succeed())
						vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)

						By("Powering on the VM")
						powerOnVMFunc(vm1Name)
						vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vm1Name, vmopv1.VirtualMachinePowerStateOn)
					})
				})

				When("the namespace has a RequiredDuringExecutionVMPlacementPolicy", func() {
					BeforeEach(func() {
						By("Creating a RequiredDuringExecutionVMPlacementPolicy in the temporary namespace")
						Expect(adminClient.Create(ctx, policy)).To(Succeed(), "failed to create RequiredDuringExecutionVMPlacementPolicy")
						DeferCleanup(func() { _ = adminClient.Delete(ctx, policy) })
					})

					It("Should reject new VMs and allow spec updates on existing VMs once the policy is deleted", func() {
						By("Creating a VirtualMachineGroup and a VM with required-required host affinity")
						vmMemberNames = []string{vm1Name}
						vmgRoot, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(manifestbuilders.VirtualMachineGroupYaml{
							Namespace: tmpNamespaceName,
							Name:      vmgRootName,
							BootOrder: []manifestbuilders.BootOrder{
								{Members: []vmopv1.GroupMember{
									{Kind: vmKind, Name: vm1Name},
								}},
							},
						})
						Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup")
						rootGroup = vmgRoot
						Expect(svClusterClient.Create(ctx, vmgRoot)).To(Succeed(), "failed to create VirtualMachineGroup")

						vmTemplate := &vmopv1.VirtualMachine{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: tmpNamespaceName,
								Name:      vm1Name,
							},
							Spec: vmopv1.VirtualMachineSpec{
								GroupName:    vmgRootName,
								ImageName:    tmpNamespaceVMIName,
								ClassName:    clusterResources.VMClassName,
								StorageClass: clusterResources.StorageClassName,
								PowerState:   vmopv1.VirtualMachinePowerStateOff,
							},
						}
						Expect(svClusterClient.Create(ctx, withRequiredExecutionAffinity(vmTemplate, hostTopoKey, false))).To(Succeed())
						vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, tmpNamespaceName, vm1Name)

						By("Deleting the RequiredDuringExecutionVMPlacementPolicy")
						Expect(adminClient.Delete(ctx, policy)).To(Succeed())
						Eventually(func(g Gomega) {
							var list vspherepolv1.RequiredDuringExecutionVMPlacementPolicyList
							g.Expect(adminClient.List(ctx, &list, ctrlclient.InNamespace(tmpNamespaceName))).To(Succeed())
							g.Expect(list.Items).To(BeEmpty())
						}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

						By("Adding the second VM to the VirtualMachineGroup")
						var vmGroup vmopv1.VirtualMachineGroup
						Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{
							Namespace: tmpNamespaceName,
							Name:      vmgRootName,
						}, &vmGroup)).To(Succeed())
						vmGroupPatch := vmGroup.DeepCopy()
						vmGroupPatch.Spec.BootOrder[0].Members = append(vmGroupPatch.Spec.BootOrder[0].Members,
							vmopv1.GroupMember{Kind: vmKind, Name: vm2Name})
						Expect(svClusterClient.Patch(ctx, vmGroupPatch, ctrlclient.MergeFrom(&vmGroup))).To(Succeed())
						vmMemberNames = append(vmMemberNames, vm2Name)

						By("Verifying a new VM with required-required terms is rejected")
						vmTemplate.Name = vm2Name
						err = svClusterClient.Create(ctx, withRequiredExecutionAffinity(vmTemplate, hostTopoKey, false))
						Expect(err).To(HaveOccurred(), "VM creation should be rejected once the policy is deleted")
						Expect(err.Error()).To(ContainSubstring("requires a RequiredDuringExecutionVMPlacementPolicy in the namespace"))

						By("Verifying the existing VM can still be powered on")
						powerOnVMFunc(vm1Name)
						vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, tmpNamespaceName, vm1Name, vmopv1.VirtualMachinePowerStateOn)
					})

					It("Should create 4 VMs with required-required zone AF with each other", func() {
						By("Creating a VirtualMachineGroup with 4 VM-kind members")
						topologyKey = zoneLabelKey
						vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
						runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingRequiredDuringExecution,
							[]string{vm1Name, vm2Name, vm3Name, vm4Name},
							[]string{})
					})

					It("Should create 3 VMs with required-required zone AAF with each other", func() {
						namespaceZones, err := utils.ListZonesByNamespace(ctx, input.ClusterProxy.GetClient(), tmpNamespaceName)
						Expect(err).ToNot(HaveOccurred())
						if len(namespaceZones.Items) < 3 {
							Skip("zone anti-affinity of 3 VMs requires at least 3 zones bound to the namespace")
						}

						By("Creating a VirtualMachineGroup with 3 VM-kind members")
						topologyKey = zoneLabelKey
						vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
						runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingRequiredDuringExecution,
							[]string{},
							[]string{vm1Name, vm2Name, vm3Name})
					})

					It("Should create 4 VMs with required-required host AF with each other", func() {
						By("Creating a VirtualMachineGroup with 4 VM-kind members")
						vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
						runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingRequiredDuringExecution,
							[]string{vm1Name, vm2Name, vm3Name, vm4Name},
							[]string{})
					})

					It("Should create 3 VMs with required-required host AAF with each other", func() {
						By("Creating a VirtualMachineGroup with 3 VM-kind members")
						vmMemberNames = []string{vm1Name, vm2Name, vm3Name}
						runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingRequiredDuringExecution,
							[]string{},
							[]string{vm1Name, vm2Name, vm3Name})
					})

					It("Should create VMs with required-required host AF (2vms) & AAF (2vms)", func() {
						By("Creating a VirtualMachineGroup with 4 VM-kind members")
						vmMemberNames = []string{vm1Name, vm2Name, vm3Name, vm4Name}
						runVmVmAffinityAtHostTopoTest(requiredDuringSchedulingRequiredDuringExecution,
							[]string{vm1Name, vm2Name},
							[]string{vm3Name, vm4Name})
					})
				})
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

// vmPowerChangeTime returns the vCenter time of the named VM's change to the
// target power state, PoweredOn or PoweredOff. Call it after the VM is
// observed in the target. It fails if, since the given vCenter time, the VM
// did not change to the target exactly once or changed to the other power
// state. It waits a short time for vCenter to log the event.
func vmPowerChangeTime(
	ctx context.Context,
	vc *vim25.Client,
	c ctrlclient.Client,
	namespace, name string,
	since time.Time,
	target vmopv1.VirtualMachinePowerState) time.Time {

	GinkgoHelper()

	moRef := vmMoRef(ctx, c, namespace, name)

	var t time.Time
	Eventually(func(g Gomega) {
		on, off, err := vmPowerEvents(ctx, vc, moRef, since)
		g.Expect(err).NotTo(HaveOccurred())

		want, other := on, off
		if target == vmopv1.VirtualMachinePowerStateOff {
			want, other = off, on
		}
		if len(want) > 1 || len(other) > 0 {
			StopTrying(fmt.Sprintf(
				"%s has %d power-on and %d power-off events since %v, not one change to %q",
				name, len(on), len(off), since, target)).Now()
		}
		g.Expect(want).To(HaveLen(1), "%s has no event for the change to %q", name, target)
		t = want[0]
	}, "30s", "2s").Should(Succeed(), "%s has no event for the change to %q", name, target)

	return t
}

// expectNoVMPowerTransition verifies the named VM has no power-on or power-off
// event since the given vCenter time.
func expectNoVMPowerTransition(
	ctx context.Context,
	vc *vim25.Client,
	c ctrlclient.Client,
	namespace, name string,
	since time.Time) {

	GinkgoHelper()

	on, off, err := vmPowerEvents(ctx, vc, vmMoRef(ctx, c, namespace, name), since)
	Expect(err).NotTo(HaveOccurred())
	Expect(on).To(BeEmpty(), "%s was powered on", name)
	Expect(off).To(BeEmpty(), "%s was powered off", name)
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

// expectDelayIgnored verifies a gap is less than the delay that must be
// ignored, added to the delay that applies. The ignored delays are minutes
// longer than the time to release and power a VM, so this does not depend on
// the load of the cluster.
func expectDelayIgnored(gap, applied, ignored time.Duration, desc string) {
	GinkgoHelper()

	By(fmt.Sprintf("%s: %v (delay %v, ignored delay %v)", desc, gap, applied, ignored))
	Expect(gap).To(BeNumerically("<", applied+ignored), "%s: the delay of %v was not ignored", desc, ignored)
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

// releaseOrderViolation returns a description of a boot order group that is
// released while a group before it has not reached the target, or "". A
// group is released when any of its VMs has the target in spec.powerState.
// A VM that does not exist has not reached the target.
//
// A group's spec.powerState is read before the observed status of the group
// before it. The status only moves to the target, so a released group with a
// previous group not yet at the target is a real violation and not a race.
func releaseOrderViolation(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	groups [][]string,
	target vmopv1.VirtualMachinePowerState) (string, error) {

	released := make([]bool, len(groups))
	for i, names := range groups {
		for _, name := range names {
			vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return "", err
			}
			if vm.Spec.PowerState == target {
				released[i] = true
			}
		}
	}

	for i := 0; i+1 < len(groups); i++ {
		if !released[i+1] {
			continue
		}
		for _, name := range groups[i] {
			var state vmopv1.VirtualMachinePowerState
			vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
			switch {
			case err == nil:
				state = vm.Status.PowerState
			case !apierrors.IsNotFound(err):
				return "", err
			}
			if state != target {
				return fmt.Sprintf("group %v was released while %s was %q, not %q",
					groups[i+1], name, state, target), nil
			}
		}
	}

	return "", nil
}

// watchReleaseOrder starts to poll the given boot order groups, in release
// order, in the background, so that the test can make changes meanwhile. The
// returned function stops the polling, and fails if a group was released
// before the group before it reached the target.
func watchReleaseOrder(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	groups [][]string,
	target vmopv1.VirtualMachinePowerState) func() {

	var (
		stop      = make(chan struct{})
		stopOnce  sync.Once
		done      = make(chan struct{})
		violation string
	)

	// The polling is stopped also when the test fails before it calls the
	// returned function.
	stopPolling := func() {
		stopOnce.Do(func() { close(stop) })
		<-done
	}
	DeferCleanup(stopPolling)

	go func() {
		defer GinkgoRecover()
		defer close(done)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			// A failed read is tried again on the next tick.
			if v, err := releaseOrderViolation(ctx, c, namespace, groups, target); err == nil && v != "" {
				violation = v
				return
			}
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return func() {
		GinkgoHelper()

		stopPolling()
		Expect(violation).To(BeEmpty(), "the boot order groups were not released in order")
	}
}

// verifyReleaseOrder polls the given VMs until all of them are observed in the
// target power state, and fails as soon as a boot order group is released
// before the group before it has reached the target. The names are in release
// order, one per boot order group. It checks the Kubernetes objects and not
// vCenter times, so it holds when the power state changes take only seconds.
func verifyReleaseOrder(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	names []string,
	target vmopv1.VirtualMachinePowerState,
	timeout time.Duration) {

	GinkgoHelper()

	groups := make([][]string, len(names))
	for i, name := range names {
		groups[i] = []string{name}
	}
	verifyReleaseOrderGroups(ctx, c, namespace, groups, target, timeout)
}

// verifyReleaseOrderGroups is verifyReleaseOrder for boot order groups that
// hold more than one VM. A group is complete when all of its VMs are observed
// in the target.
func verifyReleaseOrderGroups(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	groups [][]string,
	target vmopv1.VirtualMachinePowerState,
	timeout time.Duration) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		v, err := releaseOrderViolation(ctx, c, namespace, groups, target)
		g.Expect(err).NotTo(HaveOccurred())
		if v != "" {
			StopTrying(v).Now()
		}
		for _, names := range groups {
			for _, name := range names {
				vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vm.Status.PowerState).To(Equal(target), "%s has not reached %q", name, target)
			}
		}
	}, timeout, "1s").Should(Succeed(), "boot order groups did not all reach %q in order", target)
}

// verifyHeld verifies none of the given VMs is released to the given power
// state, which means that spec.powerState does not become that state, for a
// fixed time.
func verifyHeld(
	ctx context.Context,
	c ctrlclient.Client,
	namespace string,
	names []string,
	released vmopv1.VirtualMachinePowerState) {

	GinkgoHelper()

	Consistently(func(g Gomega) {
		for _, name := range names {
			vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Spec.PowerState).ToNot(Equal(released), "%s was released", name)
		}
	}, 30*time.Second, "5s").Should(Succeed())
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

// vmStamp returns the apply power state time annotation of the named VM.
func vmStamp(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, name string) (string, bool) {

	GinkgoHelper()

	vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
	Expect(err).NotTo(HaveOccurred())
	v, ok := vm.Annotations[pkgconst.ApplyPowerStateTimeAnnotation]

	return v, ok
}

// newPoweredOffVM returns a copy of the template named name, powered off and
// in the named group, which can be empty.
func newPoweredOffVM(
	template *vmopv1a5.VirtualMachine,
	name, groupName string) *vmopv1a5.VirtualMachine {

	vm := template.DeepCopy()
	vm.Name = name
	vm.Spec.GroupName = groupName
	vm.Spec.PowerState = vmopv1a5.VirtualMachinePowerStateOff

	return vm
}

// newVMGroup creates a v1alpha6 VirtualMachineGroup with the given boot order,
// in the named parent group, which can be empty.
func newVMGroup(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, name, parent string,
	bootOrder []manifestbuilders.BootOrder) *vmopv1.VirtualMachineGroup {

	GinkgoHelper()

	g, err := manifestbuilders.VirtualMachineGroupWithBootOrderA6(manifestbuilders.VirtualMachineGroupYaml{
		Namespace: namespace,
		Name:      name,
		GroupName: parent,
		BootOrder: bootOrder,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to build VirtualMachineGroup %s", name)
	Expect(c.Create(ctx, g)).To(Succeed(), "failed to create VirtualMachineGroup %s", name)

	return g
}

// suspendVM patches the named VM to Suspended with the Hard suspend mode, and
// waits for it to be suspended.
func suspendVM(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	c ctrlclient.Client,
	namespace, name string) {

	GinkgoHelper()

	vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
	Expect(err).NotTo(HaveOccurred())
	vmPatch := vm.DeepCopy()
	vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
	vmPatch.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
	Expect(c.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(), "failed to suspend %s", name)
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, c, namespace, name, "Suspended")
}

// powerOnGroupInOrder sets the group to PoweredOn and verifies the VMs, given
// in boot order, are powered on in that order.
func powerOnGroupInOrder(
	ctx context.Context,
	c ctrlclient.Client,
	key ctrlclient.ObjectKey,
	names []string) {

	GinkgoHelper()

	patchVMGroup(ctx, c, key, &vmopv1.VirtualMachineGroup{}, func(g *vmopv1.VirtualMachineGroup) {
		g.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
	}, "failed to update VirtualMachineGroup power state")
	verifyReleaseOrder(ctx, c, key.Namespace, names, vmopv1.VirtualMachinePowerStateOn, 3*time.Minute)
}

// groupMemberPowerStateSynced returns the PowerStateSynced condition of a
// member in the status of the named group, or nil.
func groupMemberPowerStateSynced(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, groupName, memberName, memberKind string) *metav1.Condition {

	GinkgoHelper()

	g, err := utils.GetVirtualMachineGroup(ctx, c, namespace, groupName)
	Expect(err).NotTo(HaveOccurred())
	for i := range g.Status.Members {
		m := &g.Status.Members[i]
		if m.Name == memberName && m.Kind == memberKind {
			return meta.FindStatusCondition(m.Conditions, vmopv1.VirtualMachineGroupMemberConditionPowerStateSynced)
		}
	}

	return nil
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

// createPoweredOnVMs creates a powered on VM from the template for each of
// the given names, and waits for them to be powered on.
func createPoweredOnVMs(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	c ctrlclient.Client,
	template *vmopv1a5.VirtualMachine,
	names []string) {

	GinkgoHelper()

	for _, name := range names {
		vm := template.DeepCopy()
		vm.Name = name
		Expect(c.Create(ctx, vm)).To(Succeed(), "failed to create VirtualMachine %s", name)
		deferDeleteVM(c, vm)
	}

	for _, name := range names {
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, c, template.Namespace, name, "PoweredOn")
	}
}

// linkVMsToGroup sets each VM's spec.groupName to the given group, and waits
// for the VMs to be linked to the group.
func linkVMsToGroup(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	c ctrlclient.Client,
	namespace, groupName string,
	names []string) {

	GinkgoHelper()

	for _, name := range names {
		vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
		Expect(err).ToNot(HaveOccurred(), "failed to get VM %q", name)
		vmPatch := vm.DeepCopy()
		vmPatch.Spec.GroupName = groupName
		Expect(c.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).
			To(Succeed(), "failed to patch groupName for VM %q", name)
	}

	groupLinked := metav1.Condition{
		Type:   vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
		Status: metav1.ConditionTrue,
	}
	for _, name := range names {
		vmoperator.WaitOnVirtualMachineCondition(ctx, config, c, namespace, name, groupLinked)
	}
}

// shutdownBlockedCloudConfig returns a cloud-init config that adds the
// default login user and masks the systemd power off and halt targets, so a
// guest shutdown requested through VMware Tools does not power off the guest.
// The password is read from the vmsvc-pwd key of the named Secret.
func shutdownBlockedCloudConfig(secretName string) *vmopv1a5cloudinit.CloudConfig {
	return &vmopv1a5cloudinit.CloudConfig{
		DefaultUserEnabled: true,
		SSHPwdAuth:         new(true),
		Users: []vmopv1a5cloudinit.User{
			{
				Name:       consts.DefaultVMUserName,
				LockPasswd: new(false),
				Passwd: &vmopv1a5common.SecretKeySelector{
					Name: secretName,
					Key:  "vmsvc-pwd",
				},
			},
		},
		RunCmd: json.RawMessage(`[["systemctl", "mask", "poweroff.target", "halt.target"]]`),
	}
}

// verifyGuestCmds logs in to the VM with the given IP and verifies the output
// of each command contains the expected output.
func verifyGuestCmds(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	clusterProxy *common.VMServiceClusterProxy,
	c ctrlclient.Client,
	namespace, vmIP string,
	cmds, expectedOutput []string) {

	GinkgoHelper()

	switch config.InfraConfig.NetworkingTopology {
	case consts.NSX:
		vmservice.WaitForPodReady(ctx, config, c, namespace, consts.JumpboxPodVMName)
		vmservice.VerifyLoginAndRunCmdsInNSXSetup(ctx, config, clusterProxy, namespace, consts.JumpboxPodVMName, vmIP, cmds, expectedOutput)
	case consts.VDS:
		vmservice.VerifyLoginAndRunCmdsInVDSSetup(config, vmIP, cmds, expectedOutput)
	default:
		Fail(fmt.Sprintf("Unsupported NetworkingTopology: %v", config.InfraConfig.NetworkingTopology))
	}
}

const (
	pinnedVMKind = "VirtualMachine"
	zoneLabelKey = "topology.kubernetes.io/zone"
	hostTopoKey  = "kubernetes.io/hostname"
)

// withRequiredExecutionAffinity returns a copy of the given VM with a single
// requiredDuringSchedulingRequiredDuringExecution term on the given topology
// key. The term is a VM anti-affinity term when antiAffinity is true, and a VM
// affinity term otherwise. AF requesters also carry the selected tier label.
func withRequiredExecutionAffinity(
	template *vmopv1.VirtualMachine,
	topologyKey string,
	antiAffinity bool) *vmopv1.VirtualMachine {

	vm := template.DeepCopy()
	terms := []vmopv1.VMAffinityTerm{
		{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"tier": "1"},
			},
			TopologyKey: topologyKey,
		},
	}
	if antiAffinity {
		vm.Spec.Affinity = &vmopv1.AffinitySpec{
			VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
				RequiredDuringSchedulingRequiredDuringExecution: terms,
			},
		}
	} else {
		metav1.SetMetaDataLabel(&vm.ObjectMeta, "tier", "1")
		vm.Spec.Affinity = &vmopv1.AffinitySpec{
			VMAffinity: &vmopv1.VMAffinitySpec{
				RequiredDuringSchedulingRequiredDuringExecution: terms,
			},
		}
	}

	return vm
}

// zonePinnedVM describes a VirtualMachineGroup member VM that may be pinned to
// a zone via the topology.kubernetes.io/zone label.
type zonePinnedVM struct {
	Name string
	// Zone is the zone the VM is pinned to. Empty means the VM is not pinned.
	Zone     string
	Labels   map[string]string
	Affinity *vmopv1a5.AffinitySpec
}

// preferredAntiAffinity returns an AffinitySpec that prefers the VM to be
// placed in a different topologyKey domain than the VMs matching matchLabels.
func preferredAntiAffinity(topologyKey string, matchLabels map[string]string) *vmopv1a5.AffinitySpec {
	return &vmopv1a5.AffinitySpec{
		VMAntiAffinity: &vmopv1a5.VMAntiAffinitySpec{
			PreferredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
				TopologyKey:   topologyKey,
			}},
		},
	}
}

// requiredAffinity returns an AffinitySpec that requires the VM to be placed in
// the same topologyKey domain as the VMs matching matchLabels.
func requiredAffinity(topologyKey string, matchLabels map[string]string) *vmopv1a5.AffinitySpec {
	return &vmopv1a5.AffinitySpec{
		VMAffinity: &vmopv1a5.VMAffinitySpec{
			RequiredDuringSchedulingPreferredDuringExecution: []vmopv1a5.VMAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
				TopologyKey:   topologyKey,
			}},
		},
	}
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

// setPowerOffModeHard sets the power off mode of the named group and of the
// named VMs to Hard. A group or a VM that does not exist is skipped.
func setPowerOffModeHard(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, groupName string,
	vmNames []string) {

	GinkgoHelper()

	group := &vmopv1.VirtualMachineGroup{}
	err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: groupName}, group)
	if err == nil {
		patch := ctrlclient.MergeFrom(group.DeepCopy())
		group.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
		Expect(c.Patch(ctx, group, patch)).To(Succeed(), "failed to set the power off mode of group %s", groupName)
	} else {
		Expect(ctrlclient.IgnoreNotFound(err)).To(Succeed(), "failed to get group %s", groupName)
	}

	for _, name := range vmNames {
		vm, err := utils.GetVirtualMachine(ctx, c, namespace, name)
		if err != nil {
			Expect(ctrlclient.IgnoreNotFound(err)).To(Succeed(), "failed to get VM %s", name)

			continue
		}
		patch := ctrlclient.MergeFrom(vm.DeepCopy())
		vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
		Expect(c.Patch(ctx, vm, patch)).To(Succeed(), "failed to set the power off mode of VM %s", name)
	}
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

var (
	affinityTierTagPattern = regexp.MustCompile(`^vmoperator:af-(pds-pde|rds-pde|rds-rde)-(host|zone)-[0-9a-f]{16}-tier:`)
	affinitySelfTagPattern = regexp.MustCompile(`^vmoperator:aaf-(pds-pde|rds-pde|rds-rde)-(host|zone)-[0-9a-f]{16}$`)
)

// affinityTierTags returns this scenario's affinity tags in its namespace category.
func affinityTierTags(ctx context.Context, manager *tags.Manager, vm *object.VirtualMachine, namespace string) ([]string, error) {
	ids, err := manager.ListAttachedTags(ctx, vm.Reference())
	if err != nil {
		return nil, fmt.Errorf("list affinity tags: %w", err)
	}
	var names []string
	for _, id := range ids {
		tag, err := manager.GetTag(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get affinity tag: %w", err)
		}
		hashed := affinityTierTagPattern.MatchString(tag.Name)
		self := affinitySelfTagPattern.MatchString(tag.Name)
		if !hashed && !self && !strings.HasPrefix(tag.Name, "vmoperator:tier:") {
			continue
		}
		category, err := manager.GetCategory(ctx, tag.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("get affinity tag category: %w", err)
		}
		if category.Name == namespace {
			names = append(names, tag.Name)
		}
	}
	return names, nil
}

// waitForZonalAffinityAndAntiAffinityTags verifies both policies' new tag names
// for the four-VM zonal scenarios when VMHardAffinityDuringExecution is enabled.
func waitForZonalAffinityAndAntiAffinityTags(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	wcpClient wcp.WorkloadManagementAPI,
	client ctrlclient.Client,
	vcClient *vim25.Client,
	manager *tags.Manager,
	namespace string,
	vmNames []string) {

	GinkgoHelper()
	By("Verifying the new zonal affinity and anti-affinity tags")
	for _, vmName := range vmNames {
		Eventually(func(g Gomega) {
			vm, err := utils.GetVirtualMachine(ctx, client, namespace, vmName)
			g.Expect(err).NotTo(HaveOccurred())
			tier := vm.Labels["tier"]
			g.Expect(tier).NotTo(BeEmpty())
			vcVM := object.NewVirtualMachine(vcClient, types.ManagedObjectReference{
				Type: "VirtualMachine", Value: vm.Status.UniqueID,
			})
			names, err := affinityTierTags(ctx, manager, vcVM, namespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(names).To(ConsistOf(
				"vmoperator:tier:"+tier,
				MatchRegexp(`^vmoperator:af-rds-pde-zone-[0-9a-f]{16}-tier:`+regexp.QuoteMeta(tier)+`$`),
				MatchRegexp(`^vmoperator:aaf-rds-pde-zone-[0-9a-f]{16}$`),
			))
			for _, name := range names {
				var capability wcp.ComputePolicyCapability
				switch {
				case strings.HasPrefix(name, "vmoperator:af-"):
					capability = wcp.ComputePolicyCapabilityVMVMAffinity
				case strings.HasPrefix(name, "vmoperator:aaf-"):
					capability = wcp.ComputePolicyCapabilityVMToVMGroupsAntiAffinity
				default:
					continue
				}
				expectConfiguredAffinityPolicy(g, wcpClient, namespace, name, capability,
					wcp.ComputePolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
					types.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone)
			}
		}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed(),
			"timed out waiting for zonal affinity and anti-affinity tags on VM %s", vmName)
	}
}

// waitForPreferredHostAffinityApplied verifies preferred affinity tags, receipts, the backend policy, and VM identity.
func waitForPreferredHostAffinityApplied(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	wcpClient wcp.WorkloadManagementAPI,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	vm *vmopv1.VirtualMachine,
	tier string) string {

	GinkgoHelper()
	var previousAffinityTag string
	Eventually(func(g Gomega) {
		names, err := affinityTierTags(ctx, tagManager, vcVM, vm.Namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(HaveLen(2))
		g.Expect(names).To(ContainElement("vmoperator:tier:" + tier))
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &actual)).To(Succeed())
		g.Expect(actual.Config).NotTo(BeNil())
		record, ok := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")
		g.Expect(ok).To(BeTrue())
		g.Expect(strings.Split(record, ",")).To(ConsistOf(names))
		current, err := utils.GetVirtualMachine(ctx, client, vm.Namespace, vm.Name)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(current.UID).To(Equal(vm.UID))
		g.Expect(current.Status.UniqueID).To(Equal(vm.Status.UniqueID))
		policies, ok := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies")
		g.Expect(ok).To(BeTrue())
		g.Expect(policies).To(ContainSubstring("policies"))
		policyTag := ""
		for _, name := range names {
			if strings.HasPrefix(name, "vmoperator:af-") {
				policyTag = name
			}
		}
		g.Expect(policyTag).To(MatchRegexp(`^vmoperator:af-pds-pde-host-[0-9a-f]{16}-tier:` + tier + `$`))
		previousAffinityTag = policyTag
		expectConfiguredAffinityPolicy(g, wcpClient, vm.Namespace, policyTag,
			wcp.ComputePolicyCapabilityVMVMAffinity,
			wcp.ComputePolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			types.VmPlacementPolicyVmPlacementPolicyTopologyHost)
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
	return previousAffinityTag
}

// waitForVMAdminPaused waits for the VM to report an administrator pause.
func waitForVMAdminPaused(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, vmName string) {

	GinkgoHelper()
	Eventually(func(g Gomega) {
		current, err := utils.GetVirtualMachine(ctx, client, namespace, vmName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(current.Labels[vmopv1.PausedVMLabelKey]).To(Equal("admin"))
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
}

// expectAffinityStateUnchangedWhilePaused checks that policy receipts and attached tags remain unchanged throughout the pause.
func expectAffinityStateUnchangedWhilePaused(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	wcpClient wcp.WorkloadManagementAPI,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	namespace string,
	pausedPolicy, pausedTags, previousAffinityTag, tier string) {

	GinkgoHelper()
	Consistently(func(g Gomega) {
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &actual)).To(Succeed())
		policy, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies")
		record, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")
		g.Expect(policy).To(Equal(pausedPolicy))
		g.Expect(record).To(Equal(pausedTags))
		names, err := affinityTierTags(ctx, tagManager, vcVM, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(ConsistOf(previousAffinityTag, "vmoperator:tier:"+tier))
		expectConfiguredAffinityPolicy(g, wcpClient, namespace, previousAffinityTag,
			wcp.ComputePolicyCapabilityVMVMAffinity,
			wcp.ComputePolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			types.VmPlacementPolicyVmPlacementPolicyTopologyHost)
	}, config.GetIntervals("default", "consistent-virtual-machine-condition")...).Should(Succeed())
}

// waitForRequiredHostAffinityApplied verifies replacement of the preferred affinity tag and updates to the backend policy and receipts.
func waitForRequiredHostAffinityApplied(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	wcpClient wcp.WorkloadManagementAPI,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	namespace string,
	tier, previousAffinityTag string) {

	GinkgoHelper()
	Eventually(func(g Gomega) {
		names, err := affinityTierTags(ctx, tagManager, vcVM, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(HaveLen(2))
		g.Expect(names).To(ContainElement("vmoperator:tier:" + tier))
		g.Expect(names).NotTo(ContainElement(previousAffinityTag))
		var replacementAffinityTag string
		for _, name := range names {
			if strings.HasPrefix(name, "vmoperator:af-") {
				replacementAffinityTag = name
			}
		}
		g.Expect(replacementAffinityTag).To(MatchRegexp(`^vmoperator:af-rds-pde-host-[0-9a-f]{16}-tier:` + tier + `$`))
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &actual)).To(Succeed())
		policy, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies")
		g.Expect(policy).To(ContainSubstring("RequiredDuringPlacementPreferredDuringExecution"))
		g.Expect(policy).To(ContainSubstring(replacementAffinityTag))
		g.Expect(policy).NotTo(ContainSubstring(previousAffinityTag))
		record, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")
		g.Expect(strings.Split(record, ",")).To(ConsistOf(names))
		expectConfiguredAffinityPolicy(g, wcpClient, namespace, replacementAffinityTag,
			wcp.ComputePolicyCapabilityVMVMAffinity,
			wcp.ComputePolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
			types.VmPlacementPolicyVmPlacementPolicyTopologyHost)
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
}

// waitForAffinityStateRemoved waits for affinity tags to detach and the policy and tag receipts to be cleared.
func waitForAffinityStateRemoved(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	namespace string) {

	GinkgoHelper()
	Eventually(func(g Gomega) {
		names, err := affinityTierTags(ctx, tagManager, vcVM, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(BeEmpty())
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &actual)).To(Succeed())
		g.Expect(actual.Config).NotTo(BeNil())
		record, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")
		g.Expect(record).To(BeEmpty())
		policies, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies")
		g.Expect(policies).To(Equal("{}"))
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
}

// waitForHostAntiAffinityApplied verifies the anti-affinity target, replacement self tag, tag receipt, and backend policy.
func waitForHostAntiAffinityApplied(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	wcpClient wcp.WorkloadManagementAPI,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	namespace string,
	target, previousAntiAffinitySelfTag string) string {

	GinkgoHelper()
	var currentAntiAffinitySelfTag string
	Eventually(func(g Gomega) {
		names, err := affinityTierTags(ctx, tagManager, vcVM, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(HaveLen(1))
		g.Expect(names[0]).To(MatchRegexp(`^vmoperator:aaf-(pp|rp|rr)-host-[0-9a-f]{16}$`))
		g.Expect(names[0]).NotTo(Equal(previousAntiAffinitySelfTag))
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.extraConfig"}, &actual)).To(Succeed())
		policy, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies")
		g.Expect(policy).To(ContainSubstring("vmoperator:tier:" + target))
		g.Expect(policy).To(ContainSubstring(names[0]))
		record, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString("vmservice.placementpolicies.tags")
		g.Expect(record).To(Equal(names[0]))
		expectConfiguredAffinityPolicy(g, wcpClient, namespace, names[0],
			wcp.ComputePolicyCapabilityVMToVMGroupsAntiAffinity,
			wcp.ComputePolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			types.VmPlacementPolicyVmPlacementPolicyTopologyHost)
		currentAntiAffinitySelfTag = names[0]
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
	return currentAntiAffinitySelfTag
}

// waitForRetainedVMAffinityCleanup waits for detached affinity tags, cleared receipts, and removed management ownership.
func waitForRetainedVMAffinityCleanup(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	tagManager *tags.Manager,
	vcVM *object.VirtualMachine,
	namespace string) {

	GinkgoHelper()
	Eventually(func(g Gomega) {
		names, err := affinityTierTags(ctx, tagManager, vcVM, namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(BeEmpty())
		var actual mo.VirtualMachine
		g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &actual)).To(Succeed())
		g.Expect(actual.Config.ManagedBy == nil || actual.Config.ManagedBy.ExtensionKey == "").To(BeTrue())
		for _, key := range []string{"vmservice.placementpolicies.tags", "vmservice.placementpolicies"} {
			record, _ := object.OptionValueList(actual.Config.ExtraConfig).GetString(key)
			g.Expect(record).To(BeEmpty())
		}
	}, config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).Should(Succeed())
}

// expectConfiguredAffinityPolicy fetches the policy selected by an attached VM
// tag and checks its type, strictness, topology, and association with that tag.
// Call it inside Eventually for updates or Consistently while reconciliation is paused.
func expectConfiguredAffinityPolicy(
	g Gomega,
	wcpClient wcp.WorkloadManagementAPI,
	namespace, attachedTagName string,
	capability wcp.ComputePolicyCapability,
	strictness wcp.ComputePolicyStrictness,
	topology types.VmPlacementPolicyVmPlacementPolicyTopology) {

	GinkgoHelper()
	entries, err := wcpClient.ListComputePolicyTagUsage(namespace, attachedTagName)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(entries).To(HaveLen(1), "expected one policy for VM tag %s/%s", namespace, attachedTagName)
	entry := entries[0]
	g.Expect(entry.Policy).NotTo(BeEmpty())
	g.Expect(entry.Tag).NotTo(BeEmpty())

	policy, err := wcpClient.GetComputePolicy(entry.Policy)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(policy.Capability).To(Equal(capability), "policy %s has the wrong type", entry.Policy)
	g.Expect(policy.Strictness).To(Equal(strictness), "policy %s has the wrong strictness", entry.Policy)
	g.Expect(policy.Topology.Value).To(Equal(string(topology)), "policy %s has the wrong topology", entry.Policy)
	g.Expect(policy.VMSelector.Tag.ID).To(Equal(entry.Tag), "policy %s does not select the VM's attached tag", entry.Policy)
}
