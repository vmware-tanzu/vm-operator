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
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// VMX key names for first-class advanced fields (mirrors the vmx struct tags).
const (
	vmxPreferHT       = "numa.vcpu.preferHT"
	vmxHugePages      = "sched.mem.lpage.enable1GPage"
	vmxTimeTracker    = "timeTracker.lowLatency"
	vmxCPUAffinity    = "sched.cpu.affinity.exclusiveNoStats"
	vmxVMXSwap        = "sched.swap.vmxSwapEnabled"
	vmxPNUMANodeAffinity = "numa.nodeAffinity"
)

// Condition reason values for VirtualMachineExtraConfigSynced (mirrors reconciler constants).
const (
	extraConfigReasonPowerOffRequired  = "PowerOffRequired"
	extraConfigReasonPowerCyclePending = "PowerCyclePending"
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
		var err error
		linuxVMIName, err = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, linuxImageDisplayName)
		Expect(err).NotTo(HaveOccurred(),
			"failed to get VMI name for display name %q in namespace %q",
			linuxImageDisplayName, input.WCPNamespaceName)
	})

	// ── It block 1: phases 1-2 ────────────────────────────────────────────────
	// Creates a VM with PowerCycle first-class fields and two bag keys, waits for
	// ExtraConfigSynced=True, then exercises bag key CRUD and verifies status.extraConfig
	// reflects the changes.
	It("creates VM with first-class fields and bag keys, syncs immediately, reflects bag key CRUD",
		Label("extraconfig", "core-functional"), func() {

			vmName := fmt.Sprintf("%s-core-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PowerCycle first-class fields and two bag keys")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
						{Key: "custom.test.baz", Value: "qux"},
					},
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
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
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.ExtraConfig = []vmopv1common.KeyValuePair{
					{Key: "custom.test.foo", Value: "updated"},
					{Key: "custom.test.new", Value: "newval"},
				}
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed(),
				"failed to patch VM %s bag keys", vmName)

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
		Label("extraconfig", "core-functional"), func() {

			vmName := fmt.Sprintf("%s-pc-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PowerCycle first-class fields")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Flipping VMXSwapEnabled=false (PowerCycle-mode field) while VM is powered on")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.VMXSwapEnabled = ptr.To(false)
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

			By("Waiting for ExtraConfigSynced=False/PowerCyclePending")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, extraConfigReasonPowerCyclePending)
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
		Label("extraconfig", "core-functional"), func() {

			vmName := fmt.Sprintf("%s-po-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields and one bag key")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
					},
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Adding HugePages1GEnabled=true (PowerOff-mode) while VM is powered on")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.HugePages1GEnabled = ptr.To(true)
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

			By("Waiting for ExtraConfigSynced=False/PowerOffRequired")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, extraConfigReasonPowerOffRequired)
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
		Label("extraconfig", "core-functional"), func() {

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
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass, advanced)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Powering off VM via spec before deletion")
			vmoperator.UpdateVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Deleting VM via Kubernetes API")
			deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)

			By("Recreating VM with identical spec")
			vm = buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass, advanced)
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to recreate VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
				}
			})

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
		Label("extraconfig", "extended-functional"), func() {

			vmName := fmt.Sprintf("%s-oob-off-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields, all synced")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Flipping PreferHTEnabled=false (PowerCycle-mode) to create PowerCyclePending")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.PreferHTEnabled = ptr.To(false)
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, extraConfigReasonPowerCyclePending)

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
		Label("extraconfig", "extended-functional"), func() {

			vmName := fmt.Sprintf("%s-oob-del-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with first-class fields and a bag key")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PreferHTEnabled:                    ptr.To(true),
					TimeTrackerLowLatencyEnabled:       ptr.To(true),
					CPUAffinityExclusiveNoStatsEnabled: ptr.To(false),
					VMXSwapEnabled:                     ptr.To(true),
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "custom.test.foo", Value: "bar"},
					},
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
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
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

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
		Label("extraconfig", "core-functional"), func() {

			vmName := fmt.Sprintf("%s-pnuma-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with PNUMANodeAffinity pinned to NUMA node 0")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					PNUMANodeAffinity: []int32{0},
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
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
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.PNUMANodeAffinity = nil
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

			By("Waiting for ExtraConfigSynced=False/PowerCyclePending")
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, extraConfigReasonPowerCyclePending)

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
		Label("extraconfig", "core-functional"), func() {

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
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
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
		Label("extraconfig", "core-functional"), func() {

			vmName := fmt.Sprintf("%s-prio-%s", specName, capiutil.RandomString(4))
			vmKey := types.NamespacedName{Name: vmName, Namespace: input.WCPNamespaceName}

			By("Creating VM with VMXSwapEnabled=true, no HugePages")
			vm := buildExtraConfigVM(
				vmName, input.WCPNamespaceName, vmClassName, linuxVMIName, storageClass,
				&vmopv1.VirtualMachineAdvancedSpec{
					VMXSwapEnabled: ptr.To(true),
				})
			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", vmName)
			DeferCleanup(func() {
				if !input.SkipCleanup {
					deleteExtraConfigVM(ctx, svClusterClient, config, vmKey)
				}
			})

			vmoperator.WaitForVirtualMachineConditionCreated(
				ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey, metav1.ConditionTrue, "")

			By("Simultaneously adding HugePages1GEnabled=true (PowerOff) and flipping VMXSwapEnabled=false (PowerCycle)")
			Eventually(func(g Gomega) {
				g.Expect(svClusterClient.Get(ctx, vmKey, vm)).To(Succeed())
				vm.Spec.Advanced.HugePages1GEnabled = ptr.To(true)
				vm.Spec.Advanced.VMXSwapEnabled = ptr.To(false)
				g.Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			}, config.GetIntervals("default", "wait-vm-extraconfig-synced")...).Should(Succeed())

			By("Asserting ExtraConfigSynced=False/PowerOffRequired (PowerOff takes priority)")
			cond := waitForExtraConfigSynced(ctx, svClusterClient, config, vmKey,
				metav1.ConditionFalse, extraConfigReasonPowerOffRequired)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Message).To(ContainSubstring(vmxHugePages),
				"condition message should name the deferred PowerOff key")
		})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildExtraConfigVM constructs a v1alpha6 VirtualMachine with the given advanced spec.
// Bootstrap is disabled to avoid cloud-init customization delays.
func buildExtraConfigVM(
	name, namespace, className, imageName, storageClass string,
	advanced *vmopv1.VirtualMachineAdvancedSpec,
) *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"e2e.vmoperator.vmware.com/extraconfig-test": "true",
			},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName:        className,
			ImageName:        imageName,
			StorageClass:     storageClass,
			PowerState:       vmopv1.VirtualMachinePowerStateOn,
			PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeDisabled,
			Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
				Disabled: true,
			},
			Advanced: advanced,
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

// deleteExtraConfigVM deletes the VM and waits for it to be fully removed from Kubernetes.
func deleteExtraConfigVM(
	ctx context.Context,
	client ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
) {
	vm := &vmopv1.VirtualMachine{}
	if err := client.Get(ctx, key, vm); err != nil {
		e2eframework.Logf("VM %s not found during cleanup (may already be deleted): %v", key, err)
		return
	}
	if err := client.Delete(ctx, vm); err != nil {
		e2eframework.Logf("failed to delete VM %s: %v", key, err)
		return
	}
	vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, client, key.Namespace, key.Name)
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
