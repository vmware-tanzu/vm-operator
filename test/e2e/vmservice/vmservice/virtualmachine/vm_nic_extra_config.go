// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a6 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1a6common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMNICExtraConfigSpecInput is the input for the NIC ExtraConfig E2E test spec.
type VMNICExtraConfigSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// nicExtraConfigVMOptions holds optional fields for buildNICExtraConfigVM.
type nicExtraConfigVMOptions struct {
	PowerState                   vmopv1a6.VirtualMachinePowerState
	PromoteDisksMode             vmopv1a6.VirtualMachinePromoteDisksMode
	CoalescingScheme             vmopv1a6.CoalescingScheme
	CoalescingParams             *string
	CtxPerDev                    vmopv1a6.TxContextThreadingMode
	RSSOffloadEnabled            *bool
	UDPRSSEnabled                *vmopv1a6.UDPRSSMode
	PNICFeatures                 []vmopv1a6.PNICQueueFeature
	UPTv2Enabled                 *bool
	VNUMANodeID                  *int32
	AdvancedProperties           []vmopv1a6common.KeyValuePair
	MinHardwareVersion           *int32
	Firmware                     string
	VNUMANodeCount               *int32
	CoresPerSocket               *int32
	MemoryReservationLockedToMax *bool
}

// buildNICExtraConfigVM constructs a VirtualMachine with a single VMXNet3 NIC
// whose properties are controlled by opts.
func buildNICExtraConfigVM(name, namespace, className, imageName, storageClass string, opts nicExtraConfigVMOptions) *vmopv1a6.VirtualMachine {
	ps := vmopv1a6.VirtualMachinePowerStateOn
	if opts.PowerState != "" {
		ps = opts.PowerState
	}
	promoteDisksMode := vmopv1a6.VirtualMachinePromoteDisksModeDisabled
	if opts.PromoteDisksMode != "" {
		promoteDisksMode = opts.PromoteDisksMode
	}
	vm := &vmopv1a6.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmopv1a6.VirtualMachineSpec{
			ClassName:        className,
			ImageName:        imageName,
			StorageClass:     storageClass,
			PromoteDisksMode: promoteDisksMode,
			PowerState:       ps,
			Bootstrap: &vmopv1a6.VirtualMachineBootstrapSpec{
				Disabled: true,
			},
		},
	}

	// Build the single network interface.
	iface := vmopv1a6.VirtualMachineNetworkInterfaceSpec{
		Name: "eth0",
		Type: vmopv1a6.VirtualMachineNetworkInterfaceTypeVMXNet3,
	}

	// Populate VMXNet3-specific fields.
	vmxnet3 := &vmopv1a6.VirtualMachineNetworkInterfaceVMXNet3Spec{}
	hasVMXNet3 := false

	if opts.CoalescingScheme != "" {
		vmxnet3.CoalescingScheme = &opts.CoalescingScheme
		hasVMXNet3 = true
	}
	if opts.CoalescingParams != nil {
		vmxnet3.CoalescingParams = opts.CoalescingParams
		hasVMXNet3 = true
	}
	if opts.CtxPerDev != "" {
		vmxnet3.CtxPerDev = &opts.CtxPerDev
		hasVMXNet3 = true
	}
	if opts.RSSOffloadEnabled != nil {
		vmxnet3.RSSOffloadEnabled = opts.RSSOffloadEnabled
		hasVMXNet3 = true
	}
	if opts.UDPRSSEnabled != nil {
		vmxnet3.UDPRSSEnabled = opts.UDPRSSEnabled
		hasVMXNet3 = true
	}
	if len(opts.PNICFeatures) > 0 {
		vmxnet3.PNICFeatures = opts.PNICFeatures
		hasVMXNet3 = true
	}
	if opts.UPTv2Enabled != nil {
		vmxnet3.UPTv2Enabled = opts.UPTv2Enabled
		hasVMXNet3 = true
	}
	if hasVMXNet3 {
		iface.VMXNet3 = vmxnet3
	}

	if opts.VNUMANodeID != nil {
		iface.VNUMANodeID = opts.VNUMANodeID
	}
	if len(opts.AdvancedProperties) > 0 {
		iface.AdvancedProperties = opts.AdvancedProperties
	}

	vm.Spec.Network = &vmopv1a6.VirtualMachineNetworkSpec{
		Interfaces: []vmopv1a6.VirtualMachineNetworkInterfaceSpec{iface},
	}

	// Apply VM-level options.
	if opts.MinHardwareVersion != nil {
		vm.Spec.MinHardwareVersion = *opts.MinHardwareVersion
	}
	if opts.Firmware != "" {
		if vm.Spec.BootOptions == nil {
			vm.Spec.BootOptions = &vmopv1a6.VirtualMachineBootOptions{}
		}
		vm.Spec.BootOptions.Firmware = vmopv1a6.VirtualMachineBootOptionsFirmwareType(opts.Firmware)
	}
	if opts.VNUMANodeCount != nil || opts.CoresPerSocket != nil {
		topo := &vmopv1a6.VirtualMachineCPUTopologySpec{}
		if opts.VNUMANodeCount != nil {
			topo.VNUMANodeCount = opts.VNUMANodeCount
		}
		if opts.CoresPerSocket != nil {
			topo.CoresPerSocket = opts.CoresPerSocket
		}
		vm.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
			Topology: topo,
		}
	}
	if opts.MemoryReservationLockedToMax != nil {
		vm.Spec.MemoryAdvanced = &vmopv1a6.VirtualMachineMemoryAdvancedSpec{
			ReservationLockedToMax: opts.MemoryReservationLockedToMax,
		}
	}

	return vm
}

// statusExtraConfigMap converts vm.Status.ExtraConfig into a key→value map for
// easy lookup.
func statusExtraConfigMap(vm *vmopv1a6.VirtualMachine) map[string]string {
	m := make(map[string]string, len(vm.Status.ExtraConfig))
	for _, kv := range vm.Status.ExtraConfig {
		m[kv.Key] = kv.Value
	}
	return m
}

// statusNICInterface returns the first interface status entry whose Name matches
// ifaceName, or nil if not found.
func statusNICInterface(vm *vmopv1a6.VirtualMachine, ifaceName string) *vmopv1a6.VirtualMachineNetworkInterfaceStatus {
	if vm.Status.Network == nil {
		return nil
	}
	for i := range vm.Status.Network.Interfaces {
		if vm.Status.Network.Interfaces[i].Name == ifaceName {
			return &vm.Status.Network.Interfaces[i]
		}
	}
	return nil
}

// getNICExtraConfigCondition returns the VirtualMachineNetworkConfigSynced condition
// from the VM status, or nil if it is not present.
func getNICExtraConfigCondition(vm *vmopv1a6.VirtualMachine) *metav1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == vmopv1a6.VirtualMachineNetworkConfigSynced {
			return &vm.Status.Conditions[i]
		}
	}
	return nil
}

// waitForNICExtraConfigSynced polls until the VirtualMachineNetworkConfigSynced
// condition reaches wantStatus (and wantReason, when non-empty). When afterTime
// is provided the condition's LastTransitionTime must be strictly after it.
// Sleeps 1s before returning so callers that capture LastTransitionTime via a
// subsequent read don't race a fast reconcile (metav1.Time has second-level
// granularity).
func waitForNICExtraConfigSynced(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	wantStatus metav1.ConditionStatus,
	wantReason string,
	afterTime ...metav1.Time,
) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, key.Namespace, key.Name)
		g.Expect(err).NotTo(HaveOccurred())
		cond := getNICExtraConfigCondition(vm)
		g.Expect(cond).NotTo(BeNil())
		if len(afterTime) > 0 {
			_, _ = fmt.Fprintf(GinkgoWriter,
				"[waitForNICExtraConfigSynced] %s sentinel=%s condTS=%s after=%v status=%s reason=%s\n",
				key, afterTime[0].UTC().Format(time.RFC3339),
				cond.LastTransitionTime.UTC().Format(time.RFC3339),
				cond.LastTransitionTime.After(afterTime[0].Time),
				cond.Status, cond.Reason)
			g.Expect(cond.LastTransitionTime.After(afterTime[0].Time)).To(BeTrue(),
				"condition lastTransitionTime not yet advanced past sentinel on %s", key)
		}
		g.Expect(cond.Status).To(Equal(wantStatus))
		if wantReason != "" {
			g.Expect(cond.Reason).To(Equal(wantReason))
		}
	}, config.GetIntervals("default", "wait-vm-nic-extra-config-synced")...).Should(Succeed())
	time.Sleep(time.Second)
}

// waitForNICExtraConfigSyncedWithExtraConfig polls until the
// VirtualMachineNetworkConfigSynced condition is True and
// status.extraConfig[ecKey] equals wantValue.
func waitForNICExtraConfigSyncedWithExtraConfig(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	ecKey, wantValue string,
) {
	waitForNICExtraConfigSyncedWithExtraConfigs(ctx, svClusterClient, config, key, map[string]string{ecKey: wantValue})
}

// waitForNICExtraConfigSyncedWithExtraConfigs polls until the
// VirtualMachineNetworkConfigSynced condition is True and every entry in
// wantExtraConfig is present in status.extraConfig with the expected value.
// Returns the matched condition and sleeps 1s before returning so callers can
// use the returned LastTransitionTime as a sentinel without racing against a
// fast reconcile (metav1.Time has second-level granularity).
func waitForNICExtraConfigSyncedWithExtraConfigs(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	wantExtraConfig map[string]string,
) *metav1.Condition {
	var matched *metav1.Condition
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, key.Namespace, key.Name)
		g.Expect(err).NotTo(HaveOccurred())
		cond := getNICExtraConfigCondition(vm)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		ecMap := statusExtraConfigMap(vm)
		for k, v := range wantExtraConfig {
			g.Expect(ecMap).To(HaveKeyWithValue(k, v))
		}
		matched = cond
	}, config.GetIntervals("default", "wait-vm-nic-extra-config-synced")...).Should(Succeed())
	time.Sleep(time.Second)
	return matched
}

// waitForNICExtraConfigSyncedWithExtraConfigAbsent polls until the
// VirtualMachineNetworkConfigSynced condition is True and the given ecKey is
// absent from status.extraConfig. Returns the matched condition and sleeps 1s
// before returning (metav1.Time has second-level granularity).
func waitForNICExtraConfigSyncedWithExtraConfigAbsent(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	ecKey string,
) *metav1.Condition {
	var matched *metav1.Condition
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, key.Namespace, key.Name)
		g.Expect(err).NotTo(HaveOccurred())
		cond := getNICExtraConfigCondition(vm)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		ecMap := statusExtraConfigMap(vm)
		g.Expect(ecMap).NotTo(HaveKey(ecKey))
		matched = cond
	}, config.GetIntervals("default", "wait-vm-nic-extra-config-synced")...).Should(Succeed())
	time.Sleep(time.Second)
	return matched
}

// deleteNICExtraConfigVM deletes the named VM and waits until it is gone.
func deleteNICExtraConfigVM(ctx context.Context, client ctrlclient.Client, ns, name string) {
	vm := &vmopv1a6.VirtualMachine{}
	vm.Name = name
	vm.Namespace = ns
	_ = client.Delete(ctx, vm)
	Eventually(func() bool {
		err := client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, vm)
		return err != nil
	}, "2m", "5s").Should(BeTrue())
}

// retryUpdateVMA6 retries the get-mutate-update sequence on any error (including
// 409 Conflict when a controller write races the test's read-modify-write).
func retryUpdateVMA6(
	ctx context.Context,
	client ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	namespace, name string,
	mutate func(*vmopv1a6.VirtualMachine),
) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, client, namespace, name)
		g.Expect(err).NotTo(HaveOccurred())
		mutate(vm)
		g.Expect(client.Update(ctx, vm)).To(Succeed())
	}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(Succeed())
}

// VMNICExtraConfigSpec exercises the NIC ExtraConfig feature through five It
// blocks covering live-mode fields, powercycle-mode fields, the AdvancedProperties
// bag, UPTv2Enabled prerequisites, and VNUMANodeID prerequisites.
func VMNICExtraConfigSpec(ctx context.Context, inputGetter func() VMNICExtraConfigSpecInput) {
	const specName = "nic-extra-config"

	var (
		input            VMNICExtraConfigSpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources

		vmNamespace   string
		linuxVMIName  string
		vCenterClient *vim25.Client
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, input.ClusterProxy.(*common.VMServiceClusterProxy), consts.TelcoVMServiceAPICapabilityName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		vmNamespace = input.WCPNamespaceName

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() {
			if vCenterClient != nil {
				vcenter.LogoutVimClient(vCenterClient)
			}
		})

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		var err error
		linuxVMIName, err = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, vmNamespace, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(),
			"failed to get VMI name for display name %q in namespace %q", linuxImageDisplayName, vmNamespace)
	})

	It("creates VM with live-mode ExtraConfig and verifies both coalescingScheme and coalescingParams",
		Label("nic-extra-config", "core-functional"), func() {
			vmName := "nic-ec-live-" + capiutil.RandomString(5)
			vmKey := types.NamespacedName{Namespace: vmNamespace, Name: vmName}

			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmName)
			})

			By("Phase 1: creating VM with CoalescingScheme=Disabled")

			vm := buildNICExtraConfigVM(
				vmName, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:       vmopv1a6.VirtualMachinePowerStateOn,
					CoalescingScheme: vmopv1a6.CoalescingSchemeDisabled,
				},
			)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s", vmName)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmName)

			By("Waiting for ethernet0.coalescingScheme=disabled in status.extraConfig")

			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.coalescingScheme", "disabled")

			By("Phase 2: patching CoalescingScheme=Static + CoalescingParams=32")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				schemeStatic := vmopv1a6.CoalescingSchemeStatic
				params32 := "32"
				vm.Spec.Network.Interfaces[0].VMXNet3.CoalescingScheme = &schemeStatic
				vm.Spec.Network.Interfaces[0].VMXNet3.CoalescingParams = &params32
			})

			By("Waiting for ethernet0.coalescingScheme=static and ethernet0.coalescingParams=32 in status.extraConfig")

			waitForNICExtraConfigSyncedWithExtraConfigs(ctx, svClusterClient, config, vmKey,
				map[string]string{
					"ethernet0.coalescingScheme": "static",
					"ethernet0.coalescingParams": "32",
				})
		})

	It("sets PowerCyclePending for all powercycle-mode fields and verifies positive and negative wire values across two power cycles",
		Label("nic-extra-config", "core-functional"), func() {
			vmName := "nic-ec-pc-" + capiutil.RandomString(5)
			vmKey := types.NamespacedName{Namespace: vmNamespace, Name: vmName}

			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmName)
			})

			By("Creating VM with live-mode baseline CoalescingScheme=Adapt")

			vm := buildNICExtraConfigVM(
				vmName, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:       vmopv1a6.VirtualMachinePowerStateOn,
					CoalescingScheme: vmopv1a6.CoalescingSchemeAdapt,
				},
			)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s", vmName)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmName)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.coalescingScheme", "adapt")

			By("Phase 3: patching all four powercycle-mode fields to positive values")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				modePerQueue := vmopv1a6.TxContextThreadingModePerQueue
				udpEnabled := vmopv1a6.UDPRSSModeEnabled
				vm.Spec.Network.Interfaces[0].VMXNet3.CtxPerDev = &modePerQueue
				vm.Spec.Network.Interfaces[0].VMXNet3.RSSOffloadEnabled = ptr.To(true)
				vm.Spec.Network.Interfaces[0].VMXNet3.UDPRSSEnabled = &udpEnabled
				vm.Spec.Network.Interfaces[0].VMXNet3.PNICFeatures = []vmopv1a6.PNICQueueFeature{
					vmopv1a6.PNICQueueFeatureReceiveSideScaling,
				}
			})

			waitForNICExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerCyclePendingReason)

			By("Phase 4: power cycling the VM")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOff))

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOn
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOn))

			condPhase4 := waitForNICExtraConfigSyncedWithExtraConfigs(ctx, svClusterClient, config, vmKey,
				map[string]string{
					"ethernet0.ctxPerDev":    "3",
					"ethernet0.rssoffload":   "TRUE",
					"ethernet0.udpRSS":       "1",
					"ethernet0.pnicFeatures": "4",
				})

			By("Phase 5: patching negative wire values and verifying after another power cycle")

			t1 := condPhase4.LastTransitionTime

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				udpDisabled := vmopv1a6.UDPRSSModeDisabled
				vm.Spec.Network.Interfaces[0].VMXNet3.RSSOffloadEnabled = ptr.To(false)
				vm.Spec.Network.Interfaces[0].VMXNet3.UDPRSSEnabled = &udpDisabled
			})

			waitForNICExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerCyclePendingReason,
				t1)

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOff))

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOn
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOn))

			Eventually(func(g Gomega) {
				vm2, err2 := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err2).NotTo(HaveOccurred())
				cond := getNICExtraConfigCondition(vm2)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				ecMap := statusExtraConfigMap(vm2)
				g.Expect(ecMap).To(HaveKeyWithValue("ethernet0.rssoffload", "FALSE"))
				g.Expect(ecMap).To(HaveKeyWithValue("ethernet0.udpRSS", "2"))
				g.Expect(cond.LastTransitionTime.After(t1.Time)).To(BeTrue(),
					"expected LastTransitionTime to advance after second power cycle")
			}, config.GetIntervals("default", "wait-vm-nic-extra-config-synced")...).Should(Succeed())
		})

	It("manages AdvancedProperties bag lifecycle",
		Label("nic-extra-config", "core-functional"), func() {
			vmName := "nic-ec-bag-" + capiutil.RandomString(5)
			vmKey := types.NamespacedName{Namespace: vmNamespace, Name: vmName}

			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmName)
			})

			By("Creating VM with live-mode and powercycle-mode fields")

			udpEnabled := vmopv1a6.UDPRSSModeEnabled
			modePerQueue := vmopv1a6.TxContextThreadingModePerQueue
			vm := buildNICExtraConfigVM(
				vmName, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:        vmopv1a6.VirtualMachinePowerStateOn,
					CoalescingScheme:  vmopv1a6.CoalescingSchemeAdapt,
					CtxPerDev:         modePerQueue,
					RSSOffloadEnabled: ptr.To(true),
					UDPRSSEnabled:     &udpEnabled,
					PNICFeatures: []vmopv1a6.PNICQueueFeature{
						vmopv1a6.PNICQueueFeatureReceiveSideScaling,
					},
				},
			)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s", vmName)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmName)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.coalescingScheme", "adapt")

			By("Power cycling so all powercycle-mode fields go live")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOff))

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOn
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName,
				string(vmopv1a6.VirtualMachinePowerStateOn))

			waitForNICExtraConfigSyncedWithExtraConfigs(ctx, svClusterClient, config, vmKey,
				map[string]string{
					"ethernet0.ctxPerDev":    "3",
					"ethernet0.rssoffload":   "TRUE",
					"ethernet0.udpRSS":       "1",
					"ethernet0.pnicFeatures": "4",
				})

			By("Step A: adding AdvancedProperties bag key innerRSS=TRUE")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.Network.Interfaces[0].AdvancedProperties = []vmopv1a6common.KeyValuePair{
					{Key: "innerRSS", Value: "TRUE"},
				}
			})

			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.innerRSS", "TRUE")

			By("Step B: removing AdvancedProperties bag key and verifying it disappears from status.extraConfig")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.Network.Interfaces[0].AdvancedProperties = []vmopv1a6common.KeyValuePair{}
			})

			waitForNICExtraConfigSyncedWithExtraConfigAbsent(ctx, svClusterClient, config, vmKey,
				"ethernet0.innerRSS")
		})

	It("applies UPTv2Enabled hot-pluggable DeviceChange and checks prerequisite gate",
		Label("nic-extra-config", "extended-functional"), func() {
			hwVersion20 := int32(20)

			By("Step A: prereq check — VM without full memory reservation should block UPTv2")

			vmNameA := "nic-ec-uptv2-noreserv-" + capiutil.RandomString(5)
			vmA := buildNICExtraConfigVM(
				vmNameA, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:         vmopv1a6.VirtualMachinePowerStateOn,
					MinHardwareVersion: &hwVersion20,
					CoalescingScheme:   vmopv1a6.CoalescingSchemeAdapt,
					// MemoryAdvanced intentionally omitted to trigger the prereq gate.
				},
			)
			Expect(svClusterClient.Create(ctx, vmA)).To(Succeed(),
				"failed to create VirtualMachine %s", vmNameA)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmNameA)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmNameA},
				"ethernet0.coalescingScheme", "adapt")

			// Retry only on 409 Conflict; a webhook rejection (4xx != 409) is a
			// valid terminal outcome and must not be retried.
			var currentVMA *vmopv1a6.VirtualMachine
			var updateErr error
			Eventually(func(g Gomega) {
				vma, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmNameA)
				g.Expect(err).NotTo(HaveOccurred())
				vma.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled = ptr.To(true)
				currentVMA = vma
				updateErr = svClusterClient.Update(ctx, vma)
				if apierrors.IsConflict(updateErr) {
					g.Expect(updateErr).NotTo(HaveOccurred())
				}
			}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(Succeed())
			if updateErr != nil {
				// Webhook rejected the update — prereq gate enforced at admission.
				By(fmt.Sprintf("webhook rejected UPTv2 update for %s (expected): %v", vmNameA, updateErr))
			} else {
				// Webhook allowed through — expect condition to reflect the prereq failure.
				waitForNICExtraConfigSynced(ctx, svClusterClient, config,
					types.NamespacedName{Namespace: vmNamespace, Name: vmNameA},
					metav1.ConditionFalse, vmopv1a6.VirtualMachinePrerequisiteNotMetReason)
			}

			Expect(svClusterClient.Delete(ctx, currentVMA)).To(Succeed())
			Eventually(func() bool {
				tmp := &vmopv1a6.VirtualMachine{}
				err := svClusterClient.Get(ctx, types.NamespacedName{Namespace: vmNamespace, Name: vmNameA}, tmp)
				return err != nil
			}, "2m", "5s").Should(BeTrue())

			By("Step B: happy path — VM with full memory reservation should allow UPTv2")

			vmNameB := "nic-ec-uptv2-" + capiutil.RandomString(5)
			vmKeyB := types.NamespacedName{Namespace: vmNamespace, Name: vmNameB}
			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmNameB)
			})

			vmB := buildNICExtraConfigVM(
				vmNameB, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:                   vmopv1a6.VirtualMachinePowerStateOn,
					MinHardwareVersion:           &hwVersion20,
					CoalescingScheme:             vmopv1a6.CoalescingSchemeAdapt,
					MemoryReservationLockedToMax: ptr.To(true),
				},
			)
			Expect(svClusterClient.Create(ctx, vmB)).To(Succeed(),
				"failed to create VirtualMachine %s", vmNameB)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmNameB)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKeyB,
				"ethernet0.coalescingScheme", "adapt")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmNameB, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.Network.Interfaces[0].VMXNet3.UPTv2Enabled = ptr.To(true)
			})

			// Wait until the condition exits PrerequisiteNotMet — any other state is a success.
			Eventually(func(g Gomega) {
				vm2, err2 := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmNameB)
				g.Expect(err2).NotTo(HaveOccurred())
				cond := getNICExtraConfigCondition(vm2)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Reason).NotTo(Equal(vmopv1a6.VirtualMachinePrerequisiteNotMetReason))
			}, config.GetIntervals("default", "wait-vm-nic-extra-config-synced")...).Should(Succeed())

			// If status.Network.Interfaces is populated, verify UPTv2 is reflected.
			finalVMB, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmNameB)
			Expect(err).NotTo(HaveOccurred())
			ifaceStatus := statusNICInterface(finalVMB, "eth0")
			if ifaceStatus != nil && ifaceStatus.VMXNet3 != nil && ifaceStatus.VMXNet3.UPTv2Enabled != nil {
				Expect(*ifaceStatus.VMXNet3.UPTv2Enabled).To(BeTrue(),
					"expected UPTv2Enabled=true in interface status for %s", vmNameB)
			}
		})

	It("applies VNUMANodeID poweroff-required DeviceChange and checks prerequisite gates",
		Label("nic-extra-config", "extended-functional"), func() {
			hwVersion20 := int32(20)

			By("Step A: EFI prereq check — VM without EFI firmware should block VNUMANodeID")

			vmNameA := "nic-ec-vnuma-nofi-" + capiutil.RandomString(5)
			vmA := buildNICExtraConfigVM(
				vmNameA, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:         vmopv1a6.VirtualMachinePowerStateOn,
					MinHardwareVersion: &hwVersion20,
					VNUMANodeCount:     ptr.To(int32(2)),
					CoresPerSocket:     ptr.To(int32(1)),
					CoalescingScheme:   vmopv1a6.CoalescingSchemeAdapt,
					// Explicitly BIOS: the VM class/image may default to EFI on
					// newer hardware versions, which would satisfy the EFI
					// prerequisite and mask the gate this step is testing.
					Firmware: string(vmopv1a6.VirtualMachineBootOptionsFirmwareTypeBIOS),
				},
			)
			Expect(svClusterClient.Create(ctx, vmA)).To(Succeed(),
				"failed to create VirtualMachine %s", vmNameA)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmNameA)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmNameA},
				"ethernet0.coalescingScheme", "adapt")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmNameA, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.Network.Interfaces[0].VNUMANodeID = ptr.To(int32(1))
			})

			waitForNICExtraConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmNameA},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePrerequisiteNotMetReason)

			vmA2, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmNameA)
			Expect(err).NotTo(HaveOccurred())
			condA := getNICExtraConfigCondition(vmA2)
			Expect(condA).NotTo(BeNil())
			Expect(condA.Message).To(ContainSubstring("EFI firmware required"))

			Expect(svClusterClient.Delete(ctx, &vmopv1a6.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Namespace: vmNamespace, Name: vmNameA},
			})).To(Succeed())
			Eventually(func() bool {
				tmp := &vmopv1a6.VirtualMachine{}
				err := svClusterClient.Get(ctx, types.NamespacedName{Namespace: vmNamespace, Name: vmNameA}, tmp)
				return err != nil
			}, "2m", "5s").Should(BeTrue())

			By("Step B: happy path — EFI + vNUMA topology + poweroff lifecycle")

			vmNameB := "nic-ec-vnuma-" + capiutil.RandomString(5)
			vmKeyB := types.NamespacedName{Namespace: vmNamespace, Name: vmNameB}
			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmNameB)
			})

			vmB := buildNICExtraConfigVM(
				vmNameB, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:         vmopv1a6.VirtualMachinePowerStateOn,
					MinHardwareVersion: &hwVersion20,
					Firmware:           string(vmopv1a6.VirtualMachineBootOptionsFirmwareTypeEFI),
					VNUMANodeCount:     ptr.To(int32(2)),
					CoresPerSocket:     ptr.To(int32(1)),
					CoalescingScheme:   vmopv1a6.CoalescingSchemeAdapt,
				},
			)
			Expect(svClusterClient.Create(ctx, vmB)).To(Succeed(),
				"failed to create VirtualMachine %s", vmNameB)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmNameB)
			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKeyB,
				"ethernet0.coalescingScheme", "adapt")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmNameB, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.Network.Interfaces[0].VNUMANodeID = ptr.To(int32(1))
			})

			waitForNICExtraConfigSynced(ctx, svClusterClient, config, vmKeyB,
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason)

			By("Powering off to apply the VNUMANodeID assignment")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmNameB, func(vm *vmopv1a6.VirtualMachine) {
				vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff
			})
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmNameB,
				string(vmopv1a6.VirtualMachinePowerStateOff))

			waitForNICExtraConfigSynced(ctx, svClusterClient, config, vmKeyB,
				metav1.ConditionTrue, "")

			// If status.Network.Interfaces is populated, verify VNUMANodeID is reflected.
			finalVMB, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmNameB)
			Expect(err).NotTo(HaveOccurred())
			ifaceStatus := statusNICInterface(finalVMB, "eth0")
			if ifaceStatus != nil && ifaceStatus.VNUMANodeID != nil {
				Expect(*ifaceStatus.VNUMANodeID).To(Equal(int32(1)),
					"expected VNUMANodeID=1 in interface status for %s", vmNameB)
			}

			// Verify via govmomi that the ethernet0 device's NumaNode matches the
			// desired vNUMA assignment, independent of the operator's own status
			// mapping checked above.
			moid := vmoperator.GetVirtualMachineMOID(ctx, svClusterClient, vmNamespace, vmNameB)
			Expect(moid).NotTo(BeEmpty(), "failed to resolve MoRef for %s", vmNameB)

			vcClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vcClient)
			propCollector := property.DefaultCollector(vcClient)
			moRef := vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: moid}
			var vmMO mo.VirtualMachine
			Expect(propCollector.RetrieveOne(ctx, moRef, []string{"config.hardware.device"}, &vmMO)).
				To(Succeed(), "failed to retrieve VM device config for %s", vmNameB)

			var numaNode *int32
			for _, dev := range vmMO.Config.Hardware.Device {
				if _, ok := dev.(*vimtypes.VirtualVmxnet3); ok {
					numaNode = dev.GetVirtualDevice().NumaNode
					break
				}
			}
			Expect(numaNode).NotTo(BeNil(), "expected ethernet0 device to report a NumaNode for %s", vmNameB)
			Expect(*numaNode).To(Equal(int32(1)), "expected ethernet0 NumaNode=1 in vSphere for %s", vmNameB)
		})

	It("applies live-mode ExtraConfig on a VM with PromoteDisksMode=Online",
		Label("nic-extra-config", "core-functional"), func() {
			vmName := "nic-ec-promotedisks-" + capiutil.RandomString(5)
			vmKey := types.NamespacedName{Namespace: vmNamespace, Name: vmName}

			DeferCleanup(func() {
				deleteNICExtraConfigVM(ctx, svClusterClient, vmNamespace, vmName)
			})

			By("Creating VM with PromoteDisksMode=Online and CoalescingScheme=Disabled")

			vm := buildNICExtraConfigVM(
				vmName, vmNamespace,
				clusterResources.VMClassName, linuxVMIName, clusterResources.StorageClassName,
				nicExtraConfigVMOptions{
					PowerState:       vmopv1a6.VirtualMachinePowerStateOn,
					PromoteDisksMode: vmopv1a6.VirtualMachinePromoteDisksModeOnline,
					CoalescingScheme: vmopv1a6.CoalescingSchemeDisabled,
				},
			)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s", vmName)

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, vmNamespace, vmName)

			By("Waiting for ethernet0.coalescingScheme=disabled in status.extraConfig")

			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.coalescingScheme", "disabled")

			By("Patching CoalescingScheme=Static and verifying the live update is applied")

			retryUpdateVMA6(ctx, svClusterClient, config, vmNamespace, vmName, func(vm *vmopv1a6.VirtualMachine) {
				schemeStatic := vmopv1a6.CoalescingSchemeStatic
				vm.Spec.Network.Interfaces[0].VMXNet3.CoalescingScheme = &schemeStatic
			})

			By("Waiting for ethernet0.coalescingScheme=static in status.extraConfig")

			waitForNICExtraConfigSyncedWithExtraConfig(ctx, svClusterClient, config, vmKey,
				"ethernet0.coalescingScheme", "static")

			By("Verifying PromoteDisksMode is preserved in the VM spec after NIC ExtraConfig reconciliation")

			finalVM, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalVM.Spec.PromoteDisksMode).To(Equal(vmopv1a6.VirtualMachinePromoteDisksModeOnline),
				"PromoteDisksMode must remain Online after NIC ExtraConfig reconciliation")
		})
}
