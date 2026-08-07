// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMX key names for first-class advanced fields (mirrors the vmx struct tags).
const (
	vmxPreferHT          = "numa.vcpu.preferHT"
	vmxHugePages         = "sched.mem.lpage.enable1GPage"
	vmxTimeTracker       = "timeTracker.lowLatency"
	vmxCPUAffinity       = "sched.cpu.affinity.exclusiveNoStats"
	vmxVMXSwap           = "sched.swap.vmxSwapEnabled"
	vmxPNUMANodeAffinity = "numa.nodeAffinity"
)

// VMExtraConfigSpecInput is the input for the ExtraConfig test spec.
type VMExtraConfigSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// VMExtraConfigSpec validates the ExtraConfig reconciler end-to-end on a live WCP cluster.
//
// Each It block creates its own VM and cleans up after itself, so all blocks are
// independently runnable via TEST_FOCUS or LABEL_FILTER.
//
// All blocks are skipped when the TelcoVMServiceAPI supervisor capability
// (supports_telco_vm_service_api) is not enabled on the cluster.
func VMExtraConfigSpec(ctx context.Context, inputGetter func() VMExtraConfigSpecInput) {
	const specName = "vm-extraconfig"

	var (
		input           VMExtraConfigSpecInput
		config          *e2eConfig.E2EConfig
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		vCenterClient   *vim25.Client
		vmClassName     string
		storageClass    string
		linuxVMIName    string
	)

	BeforeEach(func() {
		input = inputGetter()

		Expect(input.Config).NotTo(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).NotTo(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).NotTo(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0o755)).To(Succeed(),
			"Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(
			ctx,
			input.ClusterProxy.(*common.VMServiceClusterProxy),
			consts.TelcoVMServiceAPICapabilityName,
		)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{config.GetVariable("VMOPNamespace")},
			clusterProxy.GetClientSet(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() {
			vcenter.LogoutVimClient(vCenterClient)
		})

		clusterResources := config.InfraConfig.ManagementClusterConfig.Resources
		vmClassName = clusterResources.VMClassName
		storageClass = clusterResources.StorageClassName

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, linuxImageDisplayName)
	})

	// ── It block 1: phases 1-2 ────────────────────────────────────────────────
	// Creates a VM with PowerCycle first-class fields and two bag keys, waits for
	// ExtraConfigSynced=True, then exercises bag key CRUD and verifies status.extraConfig
	// reflects the changes.
	It("creates VM with first-class fields and bag keys, syncs immediately, reflects bag key CRUD",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-core-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PowerCycle first-class fields and two bag keys")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
						{Key: "custom.test.baz", Value: "qux"},
					},
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			By("Waiting for VM to be created in vSphere")
			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Waiting for ExtraConfigSynced=True")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting first-class VMX keys are present in status.extraConfig")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"),
				"expected %s=TRUE", vmxPreferHT)
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"),
				"expected %s=TRUE", vmxTimeTracker)
			Expect(statusExtraConfigValue(vm, vmxCPUAffinity)).To(Equal("FALSE"),
				"expected %s=FALSE", vmxCPUAffinity)
			Expect(statusExtraConfigValue(vm, vmxVMXSwap)).To(Equal("TRUE"),
				"expected %s=TRUE", vmxVMXSwap)

			By("Asserting bag keys are visible in status.extraConfig")
			Expect(statusExtraConfigKeys(vm)).To(ContainElements("custom.test.foo", "custom.test.baz"))
			Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("bar"))

			// Phase 2: bag key CRUD.
			By("Patching spec: add new bag key, update existing value, omit one to trigger deletion")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.ExtraConfig = []vmopv1common.KeyValuePair{
				{Key: "custom.test.foo", Value: "updated"},
				{Key: "custom.test.new", Value: "newval"},
			}
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s bag keys", vmName)
			vm = vmPatch

			By("Waiting for status.extraConfig to reflect the bag key changes")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				g.Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("updated"))
				g.Expect(statusExtraConfigValue(vm, "custom.test.new")).To(Equal("newval"))
				g.Expect(statusExtraConfigKeys(vm)).NotTo(ContainElement("custom.test.baz"))
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed(),
				"timed out waiting for bag key CRUD to be reflected in status.extraConfig")
		})

	// ── It block 2: phase 3 ───────────────────────────────────────────────────
	// Flips a PowerCycle-mode field on a running VM, verifies PowerCyclePending,
	// then power-cycles to apply and verifies the new value.
	It("marks PowerCyclePending when a PowerCycle-mode field changes while VM is powered on",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-pc-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PowerCycle first-class fields")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

			By("Flipping VMXSwapEnabled=false (PowerCycle-mode field) while VM is powered on")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.VMXSwapEnabled = ptr.To(false)
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s VMXSwapEnabled", vmName)
			vm = vmPatch

			By("Waiting for ExtraConfigSynced=False/PowerCyclePending")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1.VirtualMachinePowerCyclePendingReason)
			Expect(cond).NotTo(BeNil())

			By("Power-cycling VM via spec to apply the pending change")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Waiting for ExtraConfigSynced=True once VM is powered off")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Powering VM back on")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

			By("Asserting VMXSwap VMX key reflects FALSE after the power cycle")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				g.Expect(statusExtraConfigValue(vm, vmxVMXSwap)).To(Equal("FALSE"))
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())
		})

	// ── It block 3: phases 4-5 ────────────────────────────────────────────────
	// Adds a PowerOff-mode field on a running VM, verifies PowerOffRequired and
	// that the key is absent from status while deferred, then powers off and verifies
	// all keys applied.
	It("defers a PowerOff-mode field while VM is powered on, applies it after power-off",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-po-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields and one bag key")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
					},
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

			By("Adding HugePages1GEnabled=true (PowerOff-mode) while VM is powered on")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.HugePages1GEnabled = ptr.To(true)
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s HugePages1GEnabled", vmName)

			By("Waiting for ExtraConfigSynced=False/PowerOffRequired")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1.VirtualMachinePowerOffRequiredReason)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Message).To(ContainSubstring(vmxHugePages),
				"condition message should name the deferred VMX key")

			By("Asserting HugePages key is absent from status.extraConfig while deferred")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigKeys(vm)).NotTo(ContainElement(vmxHugePages),
				"deferred key %s should not appear in status.extraConfig while VM is on", vmxHugePages)

			By("Powering off VM via spec to apply the deferred PowerOff-mode key")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Waiting for ExtraConfigSynced=True after power-off")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting all first-class keys and HugePages are present after power-off")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxCPUAffinity)).To(Equal("FALSE"))
			Expect(statusExtraConfigValue(vm, vmxVMXSwap)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxHugePages)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("bar"))
		})

	// ── It block 4: phase 6 ───────────────────────────────────────────────────
	// Deletes a VM via the Kubernetes API and recreates it with an identical spec,
	// verifying that the operator re-applies all extraConfig keys from scratch.
	It("re-applies all extraConfig keys when VM is deleted via Kubernetes and recreated",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-recreate-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			advanced := &vmopv1.VirtualMachineAdvancedSpec{
				PreferHTEnabled:                    ptr.To(true),
				TimeTrackerLowLatencyEnabled:       ptr.To(true),
				CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
				VMXSwapEnabled:                     ptr.To(true),
				ExtraConfig: []vmopv1common.KeyValuePair{
					{Key: "custom.test.foo", Value: "bar"},
					{Key: "custom.test.baz", Value: "qux"},
				},
			}

			By("Creating VM with first-class fields and bag keys")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced:     advanced,
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Powering off VM via spec before deletion")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Deleting VM via Kubernetes API")
			vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)

			By("Recreating VM with identical spec")
			vm = buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced:     advanced,
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to recreate VM %s", vmName)

			By("Waiting for recreated VM to be created in vSphere")
			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Waiting for ExtraConfigSynced=True on the recreated VM")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting first-class keys and bag keys are fully re-applied on the recreated VM")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("bar"))
			Expect(statusExtraConfigValue(vm, "custom.test.baz")).To(Equal("qux"))
		})

	// ── It block 5: phase 7 ───────────────────────────────────────────────────
	// Creates a PowerCyclePending condition, then powers the VM off out-of-band
	// through vSphere directly. Verifies that the operator detects the power-off,
	// applies the pending change, and restores the VM to PoweredOn.
	It("resolves PowerCyclePending after an out-of-band vSphere power-off",
		Label("extended-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-oob-off-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields, all synced")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Flipping PreferHTEnabled=false (PowerCycle-mode) to create PowerCyclePending")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.PreferHTEnabled = ptr.To(false)
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s PreferHTEnabled", vmName)

			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1.VirtualMachinePowerCyclePendingReason)

			By("Getting VM BiosUUID for vSphere lookup")
			biosUUID := waitForBiosUUID(ctx, svClusterClient, config, vmKey)

			By("Powering off VM out-of-band via vSphere (bypassing the operator)")
			vsphereVM := findVSphereVMByBiosUUID(ctx, vCenterClient, biosUUID)
			Expect(vsphereVM).NotTo(BeNil(), "VM with BiosUUID %s not found in vSphere", biosUUID)
			powerOffTask, err := vsphereVM.PowerOff(ctx)
			Expect(err).NotTo(HaveOccurred(), "failed to start out-of-band power-off task")
			Expect(powerOffTask.Wait(ctx)).To(Succeed(), "out-of-band power-off task failed")

			By("Waiting for ExtraConfigSynced=True (pending change applied while VM is off)")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting PreferHT key is updated to FALSE in status.extraConfig")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("FALSE"))

			By("Waiting for operator to restore VM to PoweredOn (drift recovery)")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")

			By("Asserting extraConfig is intact after drift recovery")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("FALSE"))
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"))
		})

	// ── It block 6: phase 8 ───────────────────────────────────────────────────
	// Destroys the VM in vSphere without touching the Kubernetes object. Verifies
	// that the operator detects the orphaned K8s object, recreates the VM in vSphere,
	// and re-applies all extraConfig keys from the spec.
	It("re-applies extraConfig after the vSphere VM is destroyed out-of-band",
		Label("extended-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-oob-del-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields and a bag key")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
					},
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Powering off VM cleanly via spec before destroy")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Recording BiosUUID and UniqueID before the vSphere destroy")
			originalBiosUUID := waitForBiosUUID(ctx, svClusterClient, config, vmKey)
			Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
			originalUniqueID := vm.Status.UniqueID

			By("Confirming vSphere reports the VM as powered off before destroying")
			vsphereVM := findVSphereVMByBiosUUID(ctx, vCenterClient, originalBiosUUID)
			Expect(vsphereVM).NotTo(BeNil(), "VM with BiosUUID %s not found in vSphere", originalBiosUUID)
			Eventually(func(g Gomega) {
				powerState, err := vsphereVM.PowerState(ctx)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(powerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(Succeed(),
				"timed out waiting for vSphere to confirm VM %s is powered off", vmName)

			By("Destroying VM in vSphere out-of-band (Kubernetes object survives)")
			destroyTask, err := vsphereVM.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred(), "failed to start vSphere destroy task")
			Expect(destroyTask.Wait(ctx)).To(Succeed(), "vSphere VM destroy task failed")

			By("Setting spec.powerState=PoweredOn so the operator powers on the recreated VM")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s spec.powerState", vmName)
			vm = vmPatch

			By("Waiting for operator to recreate the VM (UniqueID change proves a new vSphere VM)")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				g.Expect(vm.Status.UniqueID).NotTo(BeEmpty())
				g.Expect(vm.Status.UniqueID).NotTo(Equal(originalUniqueID))
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
				"timed out waiting for UniqueID to change after vSphere destroy")

			By("Waiting for VirtualMachineCreated=True on the recreated VM")
			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Waiting for ExtraConfigSynced=True on the recreated VM")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting extraConfig is fully re-applied from spec on the recreated VM")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("bar"))
		})

	// ── It block 7 ───────────────────────────────────────────────────────────
	// Verifies that the PNUMANodeAffinity field (the only []int32 first-class field)
	// is encoded correctly as a comma-separated string, triggers PowerCyclePending
	// when changed while the VM is running, and resolves after a power cycle.
	It("handles PNUMANodeAffinity ([]int32 field): syncs, marks PowerCyclePending on change, resolves after power-off",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-pnuma-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PNUMANodeAffinity pinned to NUMA node 0")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PNUMANodeAffinity: []int32{0},
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting numa.nodeAffinity=0 is visible in status.extraConfig")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPNUMANodeAffinity)).To(Equal("0"),
				"expected %s=0 in status", vmxPNUMANodeAffinity)

			By("Clearing PNUMANodeAffinity (nil) while VM is powered on — should mark PowerCyclePending")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.PNUMANodeAffinity = nil
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s PNUMANodeAffinity", vmName)

			By("Waiting for ExtraConfigSynced=False/PowerCyclePending")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1.VirtualMachinePowerCyclePendingReason)

			By("Powering off VM to apply the pending clear")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Waiting for ExtraConfigSynced=True after power-off")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting numa.nodeAffinity is no longer in status.extraConfig after the clear")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigKeys(vm)).NotTo(ContainElement(vmxPNUMANodeAffinity),
				"cleared key %s should not appear in status.extraConfig", vmxPNUMANodeAffinity)
		})

	// ── It block 8 ───────────────────────────────────────────────────────────
	// Creates a VM with the default PromoteDisksMode (Online) so that disk
	// promotion runs concurrently with extraConfig application. Verifies that
	// ExtraConfigSynced reaches True even while SvMotion may be in flight,
	// catching ordering or resource-conflict bugs between the two reconcilers.
	It("applies extraConfig correctly while disk promotion runs (default PromoteDisksMode=Online)",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-promo-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with default PromoteDisksMode and first-class fields plus a bag key")
			vm := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmName,
					Namespace: input.WCPNamespaceName,
					Labels: map[string]string{
						"e2e.vmoperator.vmware.com/extraconfig-test": "true",
					},
				},
				Spec: vmopv1.VirtualMachineSpec{
					ClassName:    vmClassName,
					ImageName:    linuxVMIName,
					StorageClass: storageClass,
					PowerState:   vmopv1.VirtualMachinePowerStateOn,
					Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
						Disabled: true,
					},
					Advanced: &vmopv1.VirtualMachineAdvancedSpec{
						PreferHTEnabled:              ptr.To(true),
						TimeTrackerLowLatencyEnabled: ptr.To(true),
						ExtraConfig: []vmopv1common.KeyValuePair{
							{Key: "custom.test.foo", Value: "bar"},
						},
					},
				},
			}
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			By("Waiting for VM to be created in vSphere")
			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Waiting for ExtraConfigSynced=True while disk promotion may be running concurrently")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Asserting extraConfig keys are present despite concurrent disk promotion")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, vmxTimeTracker)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, "custom.test.foo")).To(Equal("bar"))
		})

	// ── It block 9 ───────────────────────────────────────────────────────────
	// When a PowerOff-mode key (HugePages) and a PowerCycle-mode key (VMXSwap)
	// both change simultaneously on a running VM, PowerOffRequired must take
	// priority over PowerCyclePending in the ExtraConfigSynced condition.
	It("PowerOffRequired takes priority over PowerCyclePending when both key types change simultaneously",
		Label("core-functional", "experimental"), func() {

			vmName := fmt.Sprintf("%s-prio-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with VMXSwapEnabled=true, no HugePages")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					VMXSwapEnabled: ptr.To(true),
				},
			})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Simultaneously adding HugePages1GEnabled=true (PowerOff) and flipping VMXSwapEnabled=false (PowerCycle)")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmPatch := vm.DeepCopy()
			vmPatch.Spec.Advanced.HugePages1GEnabled = ptr.To(true)
			vmPatch.Spec.Advanced.VMXSwapEnabled = ptr.To(false)
			Expect(svClusterClient.Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed(),
				"failed to patch VM %s HugePages1GEnabled/VMXSwapEnabled", vmName)

			By("Asserting ExtraConfigSynced=False/PowerOffRequired (PowerOff takes priority)")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, vmopv1.VirtualMachinePowerOffRequiredReason)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Message).To(ContainSubstring(vmxHugePages),
				"condition message should name the deferred PowerOff key")
		})

	// ── It block 10 ──────────────────────────────────────────────────────────
	// A composite "SAP HANA profile" VM combining every VCFA compute
	// advanced-parameter ask that has first-class or bag-key support:
	// numa.vcpu.preferHT (first-class), sched.node{0-3}.affinity and
	// smbios.assetTag (generic spec.advanced.extraConfig bag, since neither
	// has a first-class field), ethernet0.ctxPerDev/pnicFeatures (first-class
	// vmxnet3 fields), a 4-way vNUMA topology so the four sched.nodeX.affinity
	// keys each bind to a real vNUMA client, and full CPU/memory reservation
	// plus a full memory limit (spec.resources / memoryAdvanced). Exercises
	// all three ExtraConfig-family reconcilers (VM-level, NIC-level,
	// compute-config) on one VM. disk.enableUUID is asserted rather than set:
	// it is a system-reserved key vm-operator sets unconditionally on every
	// VM, not a user-configurable field. SVGA video memory has no VM Operator
	// API today (no ExtraConfig key, no first-class field, no VM Class device
	// knob) and is intentionally not covered here.
	It("creates SAP HANA-profile VM with 4-way NUMA node affinity, asset tag, NIC tuning, and full compute reservation",
		Label("extended-functional", "experimental"), func() {

			if vCenterClient == nil {
				Skip("govmomi vCenterClient not available — skipping SAP HANA profile test")
			}

			vmName := fmt.Sprintf("%s-sap-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}
			modePerQueue := vmopv1.TxContextThreadingModePerQueue
			assetTag := "SAP-HANA-" + capiutil.RandomString(6)

			By("Creating powered-off VM with PreferHT, NUMA scheduler affinity + asset tag bag keys, and NIC tuning")
			vm := buildExtraConfigVM(buildExtraConfigVMOpts{
				Name:         vmName,
				Namespace:    input.WCPNamespaceName,
				ClassName:    vmClassName,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				Advanced: &vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled: ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "sched.node0.affinity", Value: "0"},
						{Key: "sched.node1.affinity", Value: "1"},
						{Key: "sched.node2.affinity", Value: "2"},
						{Key: "sched.node3.affinity", Value: "3"},
						{Key: "smbios.assetTag", Value: assetTag},
					},
				},
			})
			// vnumaNodeCount/coresPerSocket and the full CPU/memory reservation
			// below all require power-off to apply; starting powered off avoids
			// an extra power cycle mid-test. vnumaNodeCount also requires
			// vmx-20+, so request it directly rather than skipping the test
			// when the class/image default lands below that.
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Spec.MinHardwareVersion = 20
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
						Type: vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3,
						VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
							CtxPerDev: &modePerQueue,
							PNICFeatures: []vmopv1.PNICQueueFeature{
								vmopv1.PNICQueueFeatureReceiveSideScaling,
							},
						},
					},
				},
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmKey.Namespace, vmKey.Name)
					vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmKey.Namespace, vmKey.Name)
				}
			})

			By("Waiting for VM to be created in vSphere (powered off)")
			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Waiting for ExtraConfigSynced=True (preferHT + NUMA scheduler affinity + asset tag)")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Waiting for NetworkConfigSynced=True (ctxPerDev + pnicFeatures)")
			waitForNICExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Waiting for ComputeConfigSynced=True (memoryAdvanced.reservationLockedToMax)")
			waitForComputeConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			// 4 vCPUs, CoresPerSocket=1 → 4 sockets, VNUMANodeCount=4 → one
			// vNUMA client per socket, giving each of sched.node0-3.affinity a
			// distinct vNUMA client to bind to.
			By("Patching size.cpu=4 and vnumaNodeCount=4/coresPerSocket=1 while powered off")
			latest := getExtraConfigVM(ctx, svClusterClient, vmKey)
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse("4")),
				},
			}
			latest.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: ptr.To(int32(1)),
					VNUMANodeCount: ptr.To(int32(4)),
				},
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VM %s size/topology", vmName)

			By("Waiting for ComputeConfigSynced=True after size/topology patch")
			waitForComputeConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			vmMoRef := vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}

			verifyCoresPerNumaNode := func(when string) {
				By(fmt.Sprintf("Verifying coresPerNumaNode=1 via govmomi (%s)", when))
				Eventually(func(g Gomega) {
					var moVM mo.VirtualMachine
					err := property.DefaultCollector(vCenterClient).RetrieveOne(
						ctx, vmMoRef, []string{"config.numaInfo.coresPerNumaNode"}, &moVM,
					)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(moVM.Config).NotTo(BeNil())
					g.Expect(moVM.Config.NumaInfo).NotTo(BeNil(),
						"expected NumaInfo to be populated after vnumaNodeCount patch (%s)", when)
					g.Expect(moVM.Config.NumaInfo.CoresPerNumaNode).NotTo(BeNil(),
						"expected CoresPerNumaNode pointer to be set (%s)", when)
					// 4 vCPUs / vnumaNodeCount=4 = 1 core per NUMA node.
					g.Expect(*moVM.Config.NumaInfo.CoresPerNumaNode).To(BeEquivalentTo(int32(1)),
						"expected coresPerNumaNode=1 (4 vCPUs / vnumaNodeCount=4) (%s)", when)
				}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).Should(Succeed(),
					"timed out waiting for 4-way vNUMA config to be applied in vSphere for %s (%s)", vmKey, when)
			}
			verifyCoresPerNumaNode("powered off")

			By("Looking up host CpuMhz via govmomi for full CPU reservation")
			govVM := findVSphereVMByMOID(vCenterClient, vm.Status.UniqueID)
			Expect(govVM).NotTo(BeNil())
			host, err := govVM.HostSystem(ctx)
			Expect(err).NotTo(HaveOccurred())
			var moHost mo.HostSystem
			Expect(property.DefaultCollector(vCenterClient).RetrieveOne(
				ctx, host.Reference(), []string{"summary.hardware"}, &moHost,
			)).To(Succeed())
			Expect(moHost.Summary.Hardware).NotTo(BeNil(), "host hardware summary not populated")
			hostMHz := int64(moHost.Summary.Hardware.CpuMhz)
			Expect(hostMHz).To(BeNumerically(">", 0), "host MHz should be positive")

			cpuStatus := statusCPU(vm)
			Expect(cpuStatus).NotTo(BeNil())
			Expect(cpuStatus.Total).To(BeEquivalentTo(4), "expected size.cpu=4 to be reflected in status")
			fullCPURes := int64(cpuStatus.Total) * hostMHz

			memStatus := statusMemory(vm)
			Expect(memStatus).NotTo(BeNil())
			Expect(memStatus.Total).NotTo(BeNil())

			By("Patching full CPU reservation and full memory limit")
			latest = getExtraConfigVM(ctx, svClusterClient, vmKey)
			patch = ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources.Requests = &vmopv1.VirtualMachineResourceQuantity{
				CPU: ptr.To(resource.MustParse(fmt.Sprintf("%d", fullCPURes))),
			}
			latest.Spec.Resources.Limits = &vmopv1.VirtualMachineResourceQuantity{
				Memory: memStatus.Total,
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VM %s full compute reservation", vmName)

			By("Waiting for ComputeConfigSynced=True after full reservation/limit patch")
			waitForComputeConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Powering on VM to confirm the vNUMA topology and reservations persist")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, string(vmopv1.VirtualMachinePowerStateOn))
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, string(vmopv1.VirtualMachinePowerStateOn))

			verifyCoresPerNumaNode("powered on")

			By("Asserting every SAP HANA setting converged")
			vm = getExtraConfigVM(ctx, svClusterClient, vmKey)
			Expect(statusExtraConfigValue(vm, vmxPreferHT)).To(Equal("TRUE"))
			Expect(statusExtraConfigValue(vm, "sched.node0.affinity")).To(Equal("0"))
			Expect(statusExtraConfigValue(vm, "sched.node1.affinity")).To(Equal("1"))
			Expect(statusExtraConfigValue(vm, "sched.node2.affinity")).To(Equal("2"))
			Expect(statusExtraConfigValue(vm, "sched.node3.affinity")).To(Equal("3"))
			Expect(statusExtraConfigValue(vm, "smbios.assetTag")).To(Equal(assetTag))
			Expect(statusExtraConfigValue(vm, "ethernet0.ctxPerDev")).To(Equal("3"))
			Expect(statusExtraConfigValue(vm, "ethernet0.pnicFeatures")).To(Equal("4"))
			Expect(statusExtraConfigValue(vm, "disk.enableUUID")).To(Equal("TRUE"),
				"disk.enableUUID is set unconditionally by vm-operator for every VM")

			cpu := statusCPU(vm)
			Expect(cpu).NotTo(BeNil())
			Expect(cpu.Total).To(BeEquivalentTo(4))
			Expect(cpu.Reservation).To(BeEquivalentTo(fullCPURes),
				"expected CPU reservation=%d MHz", fullCPURes)
			mem := statusMemory(vm)
			Expect(mem).NotTo(BeNil())
			Expect(mem.ReservationLockedToMax).NotTo(BeNil())
			Expect(*mem.ReservationLockedToMax).To(BeTrue())
			Expect(mem.Limit).NotTo(BeNil())
			Expect(mem.Limit.Equal(*memStatus.Total)).To(BeTrue(),
				"expected memory limit=%s (full size), got %s", memStatus.Total.String(), mem.Limit.String())
		})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildExtraConfigVMOpts holds the parameters for buildExtraConfigVM. Using a
// struct instead of a long positional parameter list means adding an optional
// field later won't require touching every call site.
type buildExtraConfigVMOpts struct {
	Name         string
	Namespace    string
	ClassName    string
	ImageName    string
	StorageClass string
	Advanced     *vmopv1.VirtualMachineAdvancedSpec
}

// buildExtraConfigVM constructs a v1alpha6 VirtualMachine with the given advanced spec.
// Bootstrap is disabled to avoid cloud-init customization delays.
func buildExtraConfigVM(opts buildExtraConfigVMOpts) *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.Name,
			Namespace: opts.Namespace,
			Labels: map[string]string{
				"e2e.vmoperator.vmware.com/extraconfig-test": "true",
			},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName:        opts.ClassName,
			ImageName:        opts.ImageName,
			StorageClass:     opts.StorageClass,
			PowerState:       vmopv1.VirtualMachinePowerStateOn,
			PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeDisabled,
			Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
				Disabled: true,
			},
			Advanced: opts.Advanced,
		},
	}
}

// waitForExtraConfigSynced polls until VirtualMachineExtraConfigSynced reaches
// wantStatus (and optionally wantReason when non-empty). Returns the matched condition.
func waitForExtraConfigSynced(
	ctx context.Context,
	client ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	vmKey types.NamespacedName,
	wantStatus metav1.ConditionStatus,
	wantReason string,
) *metav1.Condition {
	desc := string(wantStatus)
	if wantReason != "" {
		desc = fmt.Sprintf("%s/%s", wantStatus, wantReason)
	}

	var matched *metav1.Condition
	Eventually(func(g Gomega) {
		vm := &vmopv1.VirtualMachine{}
		g.Expect(client.Get(ctx, vmKey, vm)).To(Succeed())

		var cond *metav1.Condition
		for i := range vm.Status.Conditions {
			if vm.Status.Conditions[i].Type == vmopv1.VirtualMachineExtraConfigSynced {
				cond = &vm.Status.Conditions[i]
				break
			}
		}
		g.Expect(cond).NotTo(BeNil(),
			"%s condition not yet present on VM %s", vmopv1.VirtualMachineExtraConfigSynced, vmKey)
		g.Expect(cond.Status).To(Equal(wantStatus),
			"%s: got status=%s reason=%s message=%s",
			vmopv1.VirtualMachineExtraConfigSynced, cond.Status, cond.Reason, cond.Message)
		if wantReason != "" {
			g.Expect(cond.Reason).To(Equal(wantReason),
				"%s: got reason=%s, want=%s",
				vmopv1.VirtualMachineExtraConfigSynced, cond.Reason, wantReason)
		}
		matched = cond
	}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed(),
		"timed out waiting for %s=%s on VM %s",
		vmopv1.VirtualMachineExtraConfigSynced, desc, vmKey)

	return matched
}

// statusExtraConfigValue returns the value for key in vm.Status.ExtraConfig, or "" if absent.
func statusExtraConfigValue(vm *vmopv1.VirtualMachine, key string) string {
	for _, kv := range vm.Status.ExtraConfig {
		if kv.Key == key {
			return kv.Value
		}
	}
	return ""
}

// statusExtraConfigKeys returns all keys present in vm.Status.ExtraConfig.
func statusExtraConfigKeys(vm *vmopv1.VirtualMachine) []string {
	keys := make([]string, 0, len(vm.Status.ExtraConfig))
	for _, kv := range vm.Status.ExtraConfig {
		keys = append(keys, kv.Key)
	}
	return keys
}

// getExtraConfigVM fetches the VirtualMachine or fails the test immediately.
func getExtraConfigVM(ctx context.Context, client ctrlclient.Client, key types.NamespacedName) *vmopv1.VirtualMachine {
	vm := &vmopv1.VirtualMachine{}
	Expect(client.Get(ctx, key, vm)).To(Succeed(), "failed to get VM %s", key)
	return vm
}

// waitForBiosUUID polls until vm.Status.BiosUUID is non-empty and returns it.
func waitForBiosUUID(
	ctx context.Context,
	client ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	vmKey types.NamespacedName,
) string {
	var biosUUID string
	Eventually(func(g Gomega) {
		vm := &vmopv1.VirtualMachine{}
		g.Expect(client.Get(ctx, vmKey, vm)).To(Succeed())
		g.Expect(vm.Status.BiosUUID).NotTo(BeEmpty(),
			"BiosUUID not yet populated on VM %s", vmKey)
		biosUUID = vm.Status.BiosUUID
	}, config.GetIntervals("default", "wait-virtual-machine-moid")...).Should(Succeed(),
		"timed out waiting for BiosUUID on VM %s", vmKey)
	return biosUUID
}

// findVSphereVMByBiosUUID locates a govmomi VirtualMachine object by its BIOS UUID.
// Returns nil when the VM is not found.
func findVSphereVMByBiosUUID(ctx context.Context, vimClient *vim25.Client, biosUUID string) *object.VirtualMachine {
	si := object.NewSearchIndex(vimClient)
	// instanceUuid=false searches by BIOS UUID (not instance UUID).
	ref, err := si.FindByUuid(ctx, nil, biosUUID, true, vimtypes.NewBool(false))
	if err != nil || ref == nil {
		e2eframework.Logf("VM with BiosUUID %s not found in vSphere: %v", biosUUID, err)
		return nil
	}
	return object.NewVirtualMachine(vimClient, ref.Reference())
}
