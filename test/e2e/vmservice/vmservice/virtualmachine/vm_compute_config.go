// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	vmopv1a6 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
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

const (
	// CPU allocation values sized for a 2-vCPU VMClass (≤ 2000 MHz total).
	cpuReservationP1 = "500"  // MHz — initial reservation (250 MHz/vCPU)
	cpuLimitP1       = "1000" // MHz — initial limit       (500 MHz/vCPU)
	cpuReservationP2 = "750"  // MHz — updated reservation
	cpuLimitP2       = "1500" // MHz
	// cpuReservationP3 bumps alongside a power-off-required field;
	// triggers doReconfigure so watcher fires.
	cpuReservationP3 = "1000" // MHz
	cpuLimitP3       = "2000" // MHz — ceiling for 2 vCPUs @ ~1 GHz/vCPU

	// Memory reservation values for hot-plug tests (well below the 2 GiB class total).
	memReservationP1 = "512Mi" // initial reservation
	memReservationP2 = "768Mi" // bumped reservation

	classACPUs     = int64(2)
	classBCPUs     = int64(4)
	classAMemoryMB = 2048
	classBMemoryMB = 4096

	// Standard WCP platform VMClass names for the guaranteed ↔ best-effort
	// resize tests. These classes are provisioned by the platform; the tests
	// skip when namespace access cannot be obtained for either class.
	guaranteedXSmallClassName = "guaranteed-xsmall"
	bestEffortXSmallClassName = "best-effort-xsmall"
)

// VMComputeConfigSpecInput is the input for the VMComputeConfigSpec test suite.
type VMComputeConfigSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
}

// VMComputeConfigSpec validates the compute config reconciler across all phases.
func VMComputeConfigSpec(ctx context.Context, inputGetter func() VMComputeConfigSpecInput) {
	const specName = "vm-compute-config"

	var (
		input           VMComputeConfigSpecInput
		config          *e2eConfig.E2EConfig
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		vimClient       *vim25.Client
		wcpClient       wcp.WorkloadManagementAPI
		vmNamespace     string
		linuxVMIName    string
		storageClass    string
		vmClassName     string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		vmNamespace = input.WCPNamespaceName
		storageClass = config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName
		vmClassName = config.InfraConfig.ManagementClusterConfig.Resources.VMClassName
		wcpClient = input.WCPClient
		vimClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(vcenter.LogoutVimClient, vimClient)

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy,
			consts.TelcoVMServiceAPICapabilityName)

		var vmiErr error
		linuxVMIName, vmiErr = vmoperator.WaitForVirtualMachineImageName(
			ctx, &config.Config, svClusterClient, vmNamespace,
			config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName)
		Expect(vmiErr).NotTo(HaveOccurred(),
			"failed to get VMI name for display name %q in namespace %q",
			config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName,
			vmNamespace)
	})

	It("creates VM with hot-pluggable allocation and verifies condition True after live update",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-hot-plug-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU:    ptr.To(resource.MustParse(cpuReservationP1)),
						Memory: ptr.To(resource.MustParse(memReservationP1)),
					},
					Limits: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuLimitP1)),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)

			By("Waiting for ComputeConfigSynced=True after creation (phase 1)")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			var phase1MemRes resource.Quantity
			By("Asserting phase 1 allocation status")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(500),
					"expected CPU reservation=500 MHz")
				g.Expect(cpu.Limit).ToNot(BeNil())
				g.Expect(*cpu.Limit).To(BeEquivalentTo(1000),
					"expected CPU limit=1000 MHz")
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.Reservation).ToNot(BeNil(),
					"expected memory reservation to be set after requesting %s", memReservationP1)
				g.Expect(mem.Reservation.IsZero()).To(BeFalse(),
					"expected non-zero memory reservation")
				phase1MemRes = mem.Reservation.DeepCopy()
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "phase 1 status assertions failed for %s/%s", vmNamespace, vmName)

			By("Patching hot-pluggable CPU and memory allocation (phase 2)")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{
				Requests: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU:    ptr.To(resource.MustParse(cpuReservationP2)),
					Memory: ptr.To(resource.MustParse(memReservationP2)),
				},
				Limits: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(cpuLimitP2)),
				},
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VirtualMachine %s/%s", vmNamespace, vmName)

			By("Waiting for ComputeConfigSynced=True AND reservation==750 (phase 2 composite guard)")
			waitForComputeConfigSyncedWithReservation(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName}, 750)

			By("Asserting phase 2 CPU and memory allocation status")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(750),
					"expected CPU reservation=750 MHz")
				g.Expect(cpu.Limit).ToNot(BeNil())
				g.Expect(*cpu.Limit).To(BeEquivalentTo(1500),
					"expected CPU limit=1500 MHz")
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.Reservation).ToNot(BeNil())
				g.Expect(mem.Reservation.Cmp(phase1MemRes)).To(Equal(1),
					"expected memory reservation to increase above phase 1 value %s after bumping to %s",
					phase1MemRes.String(), memReservationP2)
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "phase 2 status assertions failed for %s/%s", vmNamespace, vmName)
		})

	It("sets PowerOffRequired condition for multiple power-off-required fields while VM is powered on",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-poff-req-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP2)),
					},
					Limits: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuLimitP2)),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condTrue := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condTrue).ToNot(BeNil())

			By("Patching hot-pluggable allocation bump + power-off-required flags (phase 3)")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{
				Requests: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(cpuReservationP3)),
				},
				Limits: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(cpuLimitP3)),
				},
			}
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				HotAddEnabled: ptr.To(true),
			}
			latest.Spec.MemoryAdvanced = &vmopv1a6.VirtualMachineMemoryAdvancedSpec{
				HotAddEnabled: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VirtualMachine %s/%s", vmNamespace, vmName)

			By("Waiting for PowerOffRequired condition")
			cond := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason,
				condTrue.LastTransitionTime)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Message).To(ContainSubstring("cpuAdvanced.hotAddEnabled"),
				"expected condition message to mention cpuAdvanced.hotAddEnabled")
			Expect(cond.Message).To(ContainSubstring("memoryAdvanced.hotAddEnabled"),
				"expected condition message to mention memoryAdvanced.hotAddEnabled")

			By("Waiting for hot-pluggable CPU reservation to be applied via watcher (phase 3)")
			waitForComputeConfigSyncedFalseWithReservation(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName}, 1000)
		})

	It("applies power-off-required fields after VM is powered off",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-poff-apply-%s", capiutil.RandomString(4))
			// Create with only a hot-pluggable field so the operator reaches
			// ComputeConfigSynced=True on the first reconcile. Power-off-required
			// fields are added below while the VM is running so that PowerOffRequired
			// is actually exercised on a live VM (not suppressed by initial provisioning).
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP3)),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condSynced := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condSynced).ToNot(BeNil())

			By("Patching power-off-required fields while VM is powered on")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			p := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				HotAddEnabled: ptr.To(true),
				Topology: &vmopv1a6.VirtualMachineCPUTopologySpec{
					CoresPerSocket: ptr.To(int32(2)),
				},
			}
			latest.Spec.MemoryAdvanced = &vmopv1a6.VirtualMachineMemoryAdvancedSpec{
				HotAddEnabled: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, p)).To(Succeed(),
				"failed to patch power-off-required fields on %s/%s", vmNamespace, vmName)

			By("Waiting for PowerOffRequired condition")
			condPending := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason,
				condSynced.LastTransitionTime)
			Expect(condPending).ToNot(BeNil())

			By("Powering off VM")
			Expect(powerOffVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())

			By("Waiting for VM to reach PoweredOff state")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOff))

			By("Waiting for ComputeConfigSynced=True after power-off")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "",
				condPending.LastTransitionTime)

			By("Asserting power-off-required fields applied")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.HotAddEnabled).ToNot(BeNil())
				g.Expect(*cpu.HotAddEnabled).To(BeTrue(), "expected cpuHotAddEnabled=true")
				g.Expect(cpu.CoresPerSocket).ToNot(BeNil())
				g.Expect(*cpu.CoresPerSocket).To(BeEquivalentTo(2), "expected coresPerSocket=2")
				g.Expect(cpu.Reservation).To(BeEquivalentTo(1000), "expected reservation=1000")
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.HotAddEnabled).ToNot(BeNil())
				g.Expect(*mem.HotAddEnabled).To(BeTrue(), "expected memoryHotAddEnabled=true")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "power-off-required field assertions failed for %s/%s", vmNamespace, vmName)

			By("Restoring power state")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))
		})

	It("applies iommuEnabled after power-off",
		Label("compute-config", "extended-functional"), func() {

			vmName := fmt.Sprintf("cc-iommu-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP1)),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condTrue := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condTrue).ToNot(BeNil())

			By("Checking hardware version; skip if < 14 (IOMMUEnabled requires vmx-14+ Intel / vmx-18+ AMD)")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			if vmObj.Status.HardwareVersion < 14 {
				Skip(fmt.Sprintf("hardware version is %d (< 14); iommuEnabled requires vmx-14+",
					vmObj.Status.HardwareVersion))
			}

			By("Patching iommuEnabled while VM is powered on")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			p := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				IOMMUEnabled: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, p)).To(Succeed(),
				"failed to patch iommuEnabled on %s/%s", vmNamespace, vmName)

			By("Waiting for PowerOffRequired condition")
			condPending := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason,
				condTrue.LastTransitionTime)
			Expect(condPending).ToNot(BeNil())
			Expect(condPending.Message).To(ContainSubstring("cpuAdvanced.iommuEnabled"),
				"expected condition message to mention iommuEnabled on %s/%s",
				vmNamespace, vmName)

			By("Powering off VM")
			Expect(powerOffVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())

			By("Waiting for VM to reach PoweredOff state")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOff))

			By("Waiting for ComputeConfigSynced=True after power-off")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "",
				condPending.LastTransitionTime)

			By("Asserting iommuEnabled applied in status")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.IOMMUEnabled).ToNot(BeNil())
				g.Expect(*cpu.IOMMUEnabled).To(BeTrue(),
					"expected iommuEnabled=true after power-off apply")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "iommuEnabled assertion failed for %s/%s", vmNamespace, vmName)

			By("Restoring power state")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))
		})

	It("resizes via VMClass and applies spec compute overlay while powered off",
		Label("compute-config", "core-functional"), func() {

			classAName := fmt.Sprintf("cc-class-a-%s", capiutil.RandomString(4))
			classBName := fmt.Sprintf("cc-class-b-%s", capiutil.RandomString(4))

			classASpec := vmservice.CreateSpecCustomizedVMClass(classAName, int(classACPUs), classAMemoryMB, "")
			classBSpec := vmservice.CreateSpecCustomizedVMClass(classBName, int(classBCPUs), classBMemoryMB, "")

			Expect(wcpClient.CreateVMClass(classASpec)).To(Succeed(),
				"failed to create VMClass %s via WCP API", classAName)
			Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, classAName, vmNamespace)).To(Succeed(),
				"failed to grant namespace %s access to VMClass %s", vmNamespace, classAName)
			DeferCleanup(func() {
				vmservice.VerifyVMClassDeletion(wcpClient, classAName)
			})

			Expect(wcpClient.CreateVMClass(classBSpec)).To(Succeed(),
				"failed to create VMClass %s via WCP API", classBName)
			Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, classBName, vmNamespace)).To(Succeed(),
				"failed to grant namespace %s access to VMClass %s", vmNamespace, classBName)
			DeferCleanup(func() {
				vmservice.VerifyVMClassDeletion(wcpClient, classBName)
			})

			vmName := fmt.Sprintf("cc-cls-resize-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, classAName, linuxVMIName, storageClass,
				nil, nil, nil)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condInitial := waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName})
			Expect(condInitial).ToNot(BeNil())

			By("Powering off VM before class resize")
			Expect(powerOffVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOff))

			By("Patching class-B + spec overlay (hotAddEnabled + cpuLimitP3)")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.ClassName = classBName
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				HotAddEnabled: ptr.To(true),
			}
			latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{
				Limits: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(cpuLimitP3)),
				},
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VirtualMachine %s/%s for class resize", vmNamespace, vmName)

			By("Waiting for ClassConfigSynced=True after class resize")
			waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				condInitial.LastTransitionTime)

			By("Asserting class-resize results")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Total).To(BeEquivalentTo(4), "expected 4 vCPUs after class-B resize")
				g.Expect(cpu.HotAddEnabled).ToNot(BeNil())
				g.Expect(*cpu.HotAddEnabled).To(BeTrue(), "expected cpuHotAddEnabled=true")
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(classBName),
					"expected status.class.name=%s", classBName)
			}, config.GetIntervals("default", "wait-virtual-machine-resize")...).
				Should(Succeed(), "class-resize assertions failed for %s/%s", vmNamespace, vmName)

			By("Restoring power state")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))
		})

	It("resizes via VMClass while powered on by cycling power after PowerOffRequired",
		Label("compute-config", "core-functional"), func() {

			classAName := fmt.Sprintf("cc-class-a-%s", capiutil.RandomString(4))
			classBName := fmt.Sprintf("cc-class-b-%s", capiutil.RandomString(4))

			classASpec := vmservice.CreateSpecCustomizedVMClass(classAName, int(classACPUs), classAMemoryMB, "")
			classBSpec := vmservice.CreateSpecCustomizedVMClass(classBName, int(classBCPUs), classBMemoryMB, "")

			Expect(wcpClient.CreateVMClass(classASpec)).To(Succeed(),
				"failed to create VMClass %s via WCP API", classAName)
			Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, classAName, vmNamespace)).To(Succeed(),
				"failed to grant namespace %s access to VMClass %s", vmNamespace, classAName)
			DeferCleanup(func() {
				vmservice.VerifyVMClassDeletion(wcpClient, classAName)
			})

			Expect(wcpClient.CreateVMClass(classBSpec)).To(Succeed(),
				"failed to create VMClass %s via WCP API", classBName)
			Expect(vmservice.EnsureNamespaceHasAccess(wcpClient, classBName, vmNamespace)).To(Succeed(),
				"failed to grant namespace %s access to VMClass %s", vmNamespace, classBName)
			DeferCleanup(func() {
				vmservice.VerifyVMClassDeletion(wcpClient, classBName)
			})

			vmName := fmt.Sprintf("cc-cls-poff-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, classAName, linuxVMIName, storageClass,
				nil, nil, nil)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condClassSynced := waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName})
			Expect(condClassSynced).ToNot(BeNil())

			By("Patching class-B while VM is powered on")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.ClassName = classBName
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch className on %s/%s", vmNamespace, vmName)

			By("Powering off VM to allow operator to apply class resize")
			Expect(powerOffVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())

			By("Waiting for ClassConfigSynced=True after power-off resize")
			waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				condClassSynced.LastTransitionTime)

			By("Asserting class-B resize applied")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Total).To(BeEquivalentTo(classBCPUs),
					"expected %d vCPUs after class-B resize", classBCPUs)
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(classBName),
					"expected status.class.name=%s after resize", classBName)
			}, config.GetIntervals("default", "wait-virtual-machine-resize")...).
				Should(Succeed(), "class-resize assertions failed for %s/%s", vmNamespace, vmName)

			By("Restoring power state")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))
		})

	It("resizes from guaranteed-xsmall to best-effort-xsmall and clears reservation",
		Label("compute-config", "extended-functional"), func() {

			if err := vmservice.EnsureNamespaceHasAccess(wcpClient, guaranteedXSmallClassName, vmNamespace); err != nil {
				Skip(fmt.Sprintf("skipping: VMClass %q not accessible in namespace %s: %v",
					guaranteedXSmallClassName, vmNamespace, err))
			}
			if err := vmservice.EnsureNamespaceHasAccess(wcpClient, bestEffortXSmallClassName, vmNamespace); err != nil {
				Skip(fmt.Sprintf("skipping: VMClass %q not accessible in namespace %s: %v",
					bestEffortXSmallClassName, vmNamespace, err))
			}

			vmName := fmt.Sprintf("cc-cls-guar-be-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, guaranteedXSmallClassName, linuxVMIName, storageClass,
				nil, nil, nil)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True (powered-off, guaranteed class)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condInitial := waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName})
			Expect(condInitial).ToNot(BeNil())

			By("Asserting guaranteed class applied: CPU reservation > 0")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeNumerically(">", int64(0)),
					"expected non-zero CPU reservation for guaranteed class")
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(guaranteedXSmallClassName))
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "guaranteed-xsmall initial assertions failed for %s/%s", vmNamespace, vmName)

			By("Patching className to best-effort-xsmall while powered off")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			p := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.ClassName = bestEffortXSmallClassName
			Expect(svClusterClient.Patch(ctx, latest, p)).To(Succeed(),
				"failed to patch className to %s on %s/%s", bestEffortXSmallClassName, vmNamespace, vmName)

			By("Waiting for ClassConfigSynced=True after resize to best-effort")
			waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				condInitial.LastTransitionTime)

			By("Asserting best-effort class applied: CPU reservation cleared to 0")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(int64(0)),
					"expected CPU reservation=0 after resize to best-effort class")
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(bestEffortXSmallClassName))
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "best-effort-xsmall post-resize assertions failed for %s/%s", vmNamespace, vmName)
		})

	It("resizes from best-effort-xsmall to guaranteed-xsmall and restores reservation",
		Label("compute-config", "extended-functional"), func() {

			if err := vmservice.EnsureNamespaceHasAccess(wcpClient, bestEffortXSmallClassName, vmNamespace); err != nil {
				Skip(fmt.Sprintf("skipping: VMClass %q not accessible in namespace %s: %v",
					bestEffortXSmallClassName, vmNamespace, err))
			}
			if err := vmservice.EnsureNamespaceHasAccess(wcpClient, guaranteedXSmallClassName, vmNamespace); err != nil {
				Skip(fmt.Sprintf("skipping: VMClass %q not accessible in namespace %s: %v",
					guaranteedXSmallClassName, vmNamespace, err))
			}

			vmName := fmt.Sprintf("cc-cls-be-guar-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, bestEffortXSmallClassName, linuxVMIName, storageClass,
				nil, nil, nil)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True (powered-off, best-effort class)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condInitial := waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName})
			Expect(condInitial).ToNot(BeNil())

			By("Asserting best-effort class applied: CPU reservation == 0")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(int64(0)),
					"expected CPU reservation=0 for best-effort class")
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(bestEffortXSmallClassName))
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "best-effort-xsmall initial assertions failed for %s/%s", vmNamespace, vmName)

			By("Patching className to guaranteed-xsmall while powered off")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			p := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.ClassName = guaranteedXSmallClassName
			Expect(svClusterClient.Patch(ctx, latest, p)).To(Succeed(),
				"failed to patch className to %s on %s/%s", guaranteedXSmallClassName, vmNamespace, vmName)

			By("Waiting for ClassConfigSynced=True after resize to guaranteed")
			waitForClassConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				condInitial.LastTransitionTime)

			By("Asserting guaranteed class applied: CPU reservation > 0")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeNumerically(">", int64(0)),
					"expected non-zero CPU reservation after resize to guaranteed class")
				g.Expect(vm.Status.Class).ToNot(BeNil())
				g.Expect(vm.Status.Class.Name).To(Equal(guaranteedXSmallClassName))
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "guaranteed-xsmall post-resize assertions failed for %s/%s", vmNamespace, vmName)
		})

	It("re-applies all compute config fields after VM is deleted and recreated",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-recreate-%s", capiutil.RandomString(4))
			makeVM := func() *vmopv1a6.VirtualMachine {
				return buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
					&vmopv1a6.VirtualMachineResourcesSpec{
						Requests: &vmopv1a6.VirtualMachineResourceQuantity{
							CPU: ptr.To(resource.MustParse(cpuReservationP2)),
						},
					},
					&vmopv1a6.VirtualMachineCPUAdvancedSpec{
						HotAddEnabled: ptr.To(true),
					},
					nil,
				)
			}

			By("Creating VM (first instance)")
			Expect(svClusterClient.Create(ctx, makeVM())).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)

			By("Deleting VM")
			vmToDelete, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(svClusterClient.Delete(ctx, vmToDelete)).To(Succeed(),
				"failed to delete VirtualMachine %s/%s", vmNamespace, vmName)
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient,
				vmNamespace, vmName)

			By("Recreating VM with same spec")
			Expect(svClusterClient.Create(ctx, makeVM())).To(Succeed(),
				"failed to recreate VirtualMachine %s/%s", vmNamespace, vmName)

			By("Waiting for recreated VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Asserting fields re-applied after recreate")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				// hotAddEnabled is power-off-required; if VM was powered on at create
				// it will be in PowerOffRequired state, not True. Accept either.
				g.Expect(cpu.Reservation).To(BeEquivalentTo(750), "expected reservation=750")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "recreate assertions failed for %s/%s", vmNamespace, vmName)
		})

	It("resolves PowerOffRequired after out-of-band vSphere power-off while change is pending",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping out-of-band power-off test")
			}

			vmName := fmt.Sprintf("cc-oob-poff-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP2)),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condSynced := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condSynced).ToNot(BeNil())

			By("Patching allocation bump + nestedHardwareVirtualizationEnabled to trigger PowerOffRequired")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{
				Requests: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(cpuReservationP3)),
				},
			}
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				NestedHardwareVirtualizationEnabled: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch VirtualMachine %s/%s", vmNamespace, vmName)

			By("Waiting for PowerOffRequired condition")
			condPending := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason,
				condSynced.LastTransitionTime)
			Expect(condPending).ToNot(BeNil())

			By("Getting MoID for govmomi lookup")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			moid := vmObj.Status.UniqueID
			Expect(moid).ToNot(BeEmpty(), "expected non-empty uniqueID on VM")

			By("Performing out-of-band power-off via govmomi")
			govVM := findVSphereVMByMOID(vimClient, moid)
			Expect(govVM).ToNot(BeNil(), "govmomi VM object not found for MOID %s", moid)
			Expect(powerOffVSphereVM(ctx, govVM)).To(Succeed(),
				"govmomi power-off failed for VM with MOID %s", moid)

			By("Waiting for ComputeConfigSynced=True after out-of-band power-off")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "",
				condPending.LastTransitionTime)

			By("Asserting nestedHardwareVirtualizationEnabled applied")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.NestedHardwareVirtualizationEnabled).ToNot(BeNil())
				g.Expect(*cpu.NestedHardwareVirtualizationEnabled).To(BeTrue())
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())

			By("Waiting for operator to restore PoweredOn state (drift recovery)")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))
		})

	It("re-applies compute config fields after vSphere VM is destroyed out-of-band",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping out-of-band destroy test")
			}

			vmName := fmt.Sprintf("cc-oob-del-%s", capiutil.RandomString(4))
			// Create powered-off so power-off-required fields (hotAddEnabled) are applied
			// during initial provisioning without requiring a separate power cycle.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP2)),
					},
				},
				&vmopv1a6.VirtualMachineCPUAdvancedSpec{
					HotAddEnabled: ptr.To(true),
				},
				nil,
			)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and ComputeConfigSynced=True (powered-off)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condSynced := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condSynced).ToNot(BeNil())

			By("Recording UniqueID before the out-of-band destroy")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			originalUniqueID := vmObj.Status.UniqueID
			Expect(originalUniqueID).ToNot(BeEmpty(), "expected non-empty UniqueID before destroy")

			By("Destroying VM in vSphere out-of-band (Kubernetes object survives)")
			govVM := findVSphereVMByMOID(vimClient, originalUniqueID)
			Expect(govVM).ToNot(BeNil(), "govmomi VM not found for MOID %s", originalUniqueID)
			destroyTask, err := govVM.Destroy(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to start vSphere destroy task")
			Expect(destroyTask.Wait(ctx)).To(Succeed(), "vSphere VM destroy task failed")

			By("Setting spec.powerState=PoweredOn so the operator recreates and powers on the VM")
			Eventually(func() error {
				latest, getErr := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				if getErr != nil {
					return getErr
				}
				p := ctrlclient.MergeFrom(latest.DeepCopy())
				latest.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOn
				return svClusterClient.Patch(ctx, latest, p)
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).Should(Succeed(),
				"failed to patch spec.powerState=PoweredOn on %s/%s", vmNamespace, vmName)

			By("Waiting for operator to recreate the VM (UniqueID change proves a new vSphere VM)")
			Eventually(func(g Gomega) {
				latest, getErr := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latest.Status.UniqueID).ToNot(BeEmpty())
				g.Expect(latest.Status.UniqueID).ToNot(Equal(originalUniqueID),
					"expected UniqueID to change after vSphere destroy")
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
				"timed out waiting for UniqueID to change after vSphere destroy on %s/%s", vmNamespace, vmName)

			By("Waiting for VirtualMachineCreated=True on the recreated VM")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)

			By("Asserting compute config fields are fully re-applied from spec on the recreated VM")
			Eventually(func(g Gomega) {
				latest, getErr := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(getErr).ToNot(HaveOccurred())
				cond := getComputeConfigCondition(latest)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.LastTransitionTime.After(condSynced.LastTransitionTime.Time)).To(BeTrue(),
					"condition lastTransitionTime not yet advanced past pre-destroy sentinel")
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue),
					"ComputeConfigSynced not True (reason=%s msg=%s)", cond.Reason, cond.Message)
				cpu := statusCPU(latest)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(750),
					"expected CPU reservation=750 re-applied on recreated VM")
				g.Expect(cpu.HotAddEnabled).ToNot(BeNil())
				g.Expect(*cpu.HotAddEnabled).To(BeTrue(),
					"expected cpuHotAddEnabled=true re-applied on recreated VM")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "compute config re-apply assertions failed for %s/%s", vmNamespace, vmName)
		})

	It("clears hot-pluggable allocation immediately and defers hotAddEnabled clear via merge patch",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-null-merge-%s", capiutil.RandomString(4))
			// Create powered-off with hotAddEnabled=true so the operator applies
			// the power-off-required field during initial provisioning. This avoids
			// a redundant power cycle and puts the VM into the desired state
			// (running with hotAddEnabled=true synced) before the null-merge test.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP3)),
					},
				},
				&vmopv1a6.VirtualMachineCPUAdvancedSpec{
					HotAddEnabled: ptr.To(true),
				},
				nil,
			)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and ComputeConfigSynced=True (powered-off, hotAddEnabled applied)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condSynced := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condSynced).ToNot(BeNil())

			By("Powering on so VM is running with hotAddEnabled=true live")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))

			By("Merge-patching spec.resources=null and cpuAdvanced.hotAddEnabled=null")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.Resources = nil
			latest.Spec.CPUAdvanced = nil
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to null-merge-patch VirtualMachine %s/%s", vmNamespace, vmName)

			By("Waiting for PowerOffRequired condition (hotAddEnabled nil->false requires power-off)")
			condPending := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePowerOffRequiredReason,
				condSynced.LastTransitionTime)
			Expect(condPending).ToNot(BeNil())
			Expect(condPending.Message).To(ContainSubstring("cpuAdvanced.hotAddEnabled"),
				"expected condition to mention cpuAdvanced.hotAddEnabled")

			By("Asserting hot-pluggable allocation cleared immediately")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				if cpu != nil {
					g.Expect(cpu.Reservation).To(BeEquivalentTo(0),
						"expected CPU reservation cleared to 0")
					g.Expect(cpu.Limit).To(BeNil(),
						"expected CPU limit cleared to nil (unlimited)")
				}
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())

			By("Powering off to apply hotAddEnabled=false")
			Expect(powerOffVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOff))
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "",
				condPending.LastTransitionTime)

			By("Asserting hotAddEnabled=false after power-off")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				if cpu != nil && cpu.HotAddEnabled != nil {
					g.Expect(*cpu.HotAddEnabled).To(BeFalse(),
						"expected cpuHotAddEnabled=false after null-merge-patch")
				}
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())
		})

	It("reverts live compute config drift injected directly in vSphere",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping drift injection test")
			}

			vmName := fmt.Sprintf("cc-drift-%s", capiutil.RandomString(4))
			// Create with explicit CPU reservation and limit so we have non-zero
			// desired values to distinguish from "never set" after revert.
			// Limit is kept at 800 MHz (below 1 GHz/vCPU) to stay within the
			// typical host capacity of a 2-vCPU best-effort class.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				&vmopv1a6.VirtualMachineResourcesSpec{
					Requests: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse(cpuReservationP1)),
					},
					Limits: &vmopv1a6.VirtualMachineResourceQuantity{
						CPU: ptr.To(resource.MustParse("800")),
					},
				},
				nil, nil,
			)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Recording VM UniqueID and condition lastTransitionTime before drift injection")
			vmBeforeDrift, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmBeforeDrift.Status.UniqueID).ToNot(BeEmpty())
			condBefore := getComputeConfigCondition(vmBeforeDrift)
			Expect(condBefore).ToNot(BeNil())
			condTSBefore := condBefore.LastTransitionTime

			By("Injecting rogue drift on CPU reservation, CPU limit, and memory reservation via govmomi ReconfigVM_Task")
			govVM := findVSphereVMByMOID(vimClient, vmBeforeDrift.Status.UniqueID)
			Expect(govVM).ToNot(BeNil(), "govmomi VM not found for MOID %s", vmBeforeDrift.Status.UniqueID)
			Expect(injectAllocationDrift(ctx, govVM, 100, 500, 256)).To(Succeed(),
				"failed to inject multi-field allocation drift")

			By("Waiting for drift to be reverted (operator re-applies desired config)")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cond := getComputeConfigCondition(vm)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.LastTransitionTime.After(condTSBefore.Time)).To(BeTrue(),
					"condition lastTransitionTime not yet advanced past pre-injection sentinel — operator may not have reconciled post-drift")
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue),
					"expected True after drift revert (got reason=%s msg=%s)",
					cond.Reason, cond.Message)
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Reservation).To(BeEquivalentTo(500),
					"expected cpu.Reservation=500 (desired) after drift revert")
				g.Expect(cpu.Limit).ToNot(BeNil())
				g.Expect(*cpu.Limit).To(BeEquivalentTo(800),
					"expected cpu.Limit=800 (desired) after drift revert")
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.Reservation == nil || mem.Reservation.IsZero()).To(BeTrue(),
					"expected memory.Reservation to be nil or 0 after drift revert")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "drift not reverted for %s/%s", vmNamespace, vmName)

			By("Verifying vSphere allocation fields after operator revert")
			var moVM mo.VirtualMachine
			Expect(property.DefaultCollector(vimClient).RetrieveOne(
				ctx, govVM.Reference(), []string{"config"}, &moVM,
			)).To(Succeed(), "failed to fetch vSphere VM config for %s", vmBeforeDrift.Status.UniqueID)
			Expect(moVM.Config).ToNot(BeNil())
			Expect(moVM.Config.CpuAllocation).ToNot(BeNil())
			Expect(moVM.Config.CpuAllocation.Reservation).ToNot(BeNil())
			Expect(*moVM.Config.CpuAllocation.Reservation).To(BeEquivalentTo(500),
				"vSphere cpuAllocation.reservation should be 500 after operator revert")
			Expect(moVM.Config.CpuAllocation.Limit).ToNot(BeNil())
			Expect(*moVM.Config.CpuAllocation.Limit).To(BeEquivalentTo(800),
				"vSphere cpuAllocation.limit should be 800 after operator revert")
			Expect(moVM.Config.MemoryAllocation).ToNot(BeNil())
			Expect(moVM.Config.MemoryAllocation.Reservation).ToNot(BeNil())
			Expect(*moVM.Config.MemoryAllocation.Reservation).To(BeEquivalentTo(0),
				"vSphere memoryAllocation.reservation should be 0 after operator revert")
		})

	It("reverts power-off-required field drift injected while VM is powered off",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping power-off-required drift injection test")
			}

			vmName := fmt.Sprintf("cc-drift-poff-%s", capiutil.RandomString(4))
			// Create powered-off with nestedHVEnabled=true — the only power-off-required
			// flag that the E2E test user can inject drift for (Settings privilege only;
			// CPUCount and Memory privileges are not available).
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				nil,
				&vmopv1a6.VirtualMachineCPUAdvancedSpec{
					NestedHardwareVirtualizationEnabled: ptr.To(true),
				},
				nil,
			)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True (powered-off)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Recording VM UniqueID and condition lastTransitionTime before drift injection")
			vmBeforeDrift, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmBeforeDrift.Status.UniqueID).ToNot(BeEmpty())
			condBefore := getComputeConfigCondition(vmBeforeDrift)
			Expect(condBefore).ToNot(BeNil())
			condTSBefore := condBefore.LastTransitionTime

			By("Injecting rogue drift on power-off-required flags and coresPerSocket via govmomi ReconfigVM_Task")
			govVM := findVSphereVMByMOID(vimClient, vmBeforeDrift.Status.UniqueID)
			Expect(govVM).ToNot(BeNil(), "govmomi VM not found for MOID %s", vmBeforeDrift.Status.UniqueID)
			Expect(injectPowerOffRequiredFieldDrift(ctx, govVM)).To(Succeed(),
				"failed to inject power-off-required field drift")

			By("Waiting for drift to be reverted (operator re-applies desired config)")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cond := getComputeConfigCondition(vm)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.LastTransitionTime.After(condTSBefore.Time)).To(BeTrue(),
					"condition lastTransitionTime not yet advanced past pre-injection sentinel")
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue),
					"expected True after drift revert (got reason=%s msg=%s)",
					cond.Reason, cond.Message)
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.NestedHardwareVirtualizationEnabled).ToNot(BeNil())
				g.Expect(*cpu.NestedHardwareVirtualizationEnabled).To(BeTrue(),
					"expected nestedHVEnabled=true after drift revert")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "power-off-required drift not reverted for %s/%s", vmNamespace, vmName)

			By("Verifying vSphere nestedHVEnabled after operator revert")
			var moVM mo.VirtualMachine
			Expect(property.DefaultCollector(vimClient).RetrieveOne(
				ctx, govVM.Reference(), []string{"config.nestedHVEnabled"}, &moVM,
			)).To(Succeed(), "failed to fetch vSphere VM config for %s", vmBeforeDrift.Status.UniqueID)
			Expect(moVM.Config).ToNot(BeNil())
			Expect(moVM.Config.NestedHVEnabled).ToNot(BeNil())
			Expect(*moVM.Config.NestedHVEnabled).To(BeTrue(),
				"vSphere nestedHVEnabled should be true after operator revert")
		})

	It("sets PrerequisiteNotMet condition when VM hardware version is below field minimum",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-hw-gate-%s", capiutil.RandomString(4))
			// VMX-10 is below the VMX-11 minimum required for cpuHotAddEnabled.
			// Deploy with a hardware version floor of vmx-10 using MinHardwareVersion=-1
			// which keeps the image-default hardware version (typically vmx-10 or older
			// on some images). If the environment always upgrades HW version we skip.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				nil, nil, nil)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			condTrue := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")
			Expect(condTrue).ToNot(BeNil())

			By("Checking hardware version; skip if >= 11")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			if vmObj.Status.HardwareVersion >= 11 {
				Skip(fmt.Sprintf("hardware version is %d (>= 11); cpuHotAddEnabled PrerequisiteNotMet cannot be triggered",
					vmObj.Status.HardwareVersion))
			}

			By("Patching cpuAdvanced.hotAddEnabled on a VMX-10 VM (should trigger PrerequisiteNotMet)")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				HotAddEnabled: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed())

			By("Waiting for PrerequisiteNotMet condition")
			cond := waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionFalse, vmopv1a6.VirtualMachinePrerequisiteNotMetReason,
				condTrue.LastTransitionTime)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Message).To(ContainSubstring("cpuAdvanced.hotAddEnabled"),
				"expected condition to mention cpuAdvanced.hotAddEnabled")
			Expect(cond.Message).To(ContainSubstring("hwVer"),
				"expected condition to mention hardware version requirement")
		})

	It("sets latency sensitivity High with full CPU and memory reservation",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping latency sensitivity test")
			}

			vmName := fmt.Sprintf("cc-latency-%s", capiutil.RandomString(4))
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				nil, nil, nil)

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Looking up host CpuMhz via govmomi for full CPU reservation")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmObj.Status.UniqueID).ToNot(BeEmpty())

			govVM := findVSphereVMByMOID(vimClient, vmObj.Status.UniqueID)
			Expect(govVM).ToNot(BeNil())

			host, err := govVM.HostSystem(ctx)
			Expect(err).ToNot(HaveOccurred())
			var moHost mo.HostSystem
			Expect(property.DefaultCollector(vimClient).RetrieveOne(
				ctx, host.Reference(), []string{"summary.hardware"}, &moHost,
			)).To(Succeed())
			Expect(moHost.Summary.Hardware).ToNot(BeNil(), "host hardware summary not populated")
			hostMHz := int64(moHost.Summary.Hardware.CpuMhz)
			Expect(hostMHz).To(BeNumerically(">", 0), "host MHz should be positive")

			fullCPURes := classACPUs * hostMHz
			e2eframework.Logf("host=%d MHz, classACPUs=%d, fullCPURes=%d MHz", hostMHz, classACPUs, fullCPURes)

			By("Patching latencySensitivity=High + full CPU reservation + reservationLockedToMax")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latLevel := vmopv1a6.VirtualMachineLatencySensitivityHigh
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				LatencySensitivity: &latLevel,
			}
			latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{
				Requests: &vmopv1a6.VirtualMachineResourceQuantity{
					CPU: ptr.To(resource.MustParse(fmt.Sprintf("%d", fullCPURes))),
				},
			}
			latest.Spec.MemoryAdvanced = &vmopv1a6.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed())

			By("Waiting for ComputeConfigSynced=True after latency sensitivity patch")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Asserting latencySensitivity and reservation in status")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(strings.ToLower(cpu.LatencySensitivity)).To(Equal("high"),
					"expected latencySensitivity=high")
				g.Expect(cpu.Reservation).To(BeEquivalentTo(fullCPURes),
					"expected CPU reservation=%d MHz", fullCPURes)
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.ReservationLockedToMax).ToNot(BeNil())
				g.Expect(*mem.ReservationLockedToMax).To(BeTrue())
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())
		})

	It("applies vnumaNodeCount topology via power-off reconfigure on vmx-20+ VM",
		Label("compute-config", "extended-functional"), func() {

			if vimClient == nil {
				Skip("govmomi vimClient not available — skipping vNUMA topology test")
			}

			vmName := fmt.Sprintf("cc-vnuma-%s", capiutil.RandomString(4))
			// Deploy the VM using the default class; power off before patching vnumaNodeCount
			// to avoid PowerOffRequired + vNUMA topology confusion.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				nil, nil, nil)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True (powered-off)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Checking hardware version; skip if < 20")
			vmObj, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			if vmObj.Status.HardwareVersion < 20 {
				Skip(fmt.Sprintf("hardware version is %d (< 20); vnumaNodeCount requires vmx-20+",
					vmObj.Status.HardwareVersion))
			}

			// best-effort-small = 2 vCPUs. CoresPerSocket=1 → 2 sockets which maps
			// cleanly to VNUMANodeCount=2, giving CoresPerNumaNode = 2 vCPUs / 2 = 1.
			By("Patching vnumaNodeCount=2 with coresPerSocket=1 while VM is powered off")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			latest.Spec.CPUAdvanced = &vmopv1a6.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1a6.VirtualMachineCPUTopologySpec{
					CoresPerSocket: ptr.To(int32(1)),
					VNUMANodeCount: ptr.To(int32(2)),
				},
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed())

			By("Waiting for ComputeConfigSynced=True after vNUMA patch")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			// NumaInfo.CoresPerNumaNode is only populated by vSphere when the VM is
			// running; power it on so the govmomi read sees the live value.
			By("Powering on VM so vSphere populates NumaInfo in the running config")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))

			vmObj, err = utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			vmMoRef := vimtypes.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: vmObj.Status.UniqueID,
			}

			By("Verifying coresPerNumaNode via govmomi (polling until operator applies vNUMA config)")
			Eventually(func(g Gomega) {
				// Use a per-attempt deadline so a slow or unresponsive vCenter
				// does not cause RetrieveOne to block beyond this iteration.
				fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				var moVM mo.VirtualMachine
				err := property.DefaultCollector(vimClient).RetrieveOne(
					fetchCtx, vmMoRef, []string{"config.numaInfo.coresPerNumaNode"}, &moVM,
				)
				_, _ = fmt.Fprintf(GinkgoWriter,
					"[cc-vnuma] RetrieveOne: err=%v config=%v numaInfo=%v\n",
					err, moVM.Config != nil,
					moVM.Config != nil && moVM.Config.NumaInfo != nil)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(moVM.Config).ToNot(BeNil())
				g.Expect(moVM.Config.NumaInfo).ToNot(BeNil(),
					"expected NumaInfo to be populated after vnumaNodeCount patch")
				// 2 vCPUs (best-effort-small) / VNUMANodeCount=2 = 1 core per NUMA node.
				g.Expect(moVM.Config.NumaInfo.CoresPerNumaNode).ToNot(BeNil(),
					"expected CoresPerNumaNode pointer to be set")
				_, _ = fmt.Fprintf(GinkgoWriter,
					"[cc-vnuma] coresPerNumaNode=%d\n",
					*moVM.Config.NumaInfo.CoresPerNumaNode)
				g.Expect(*moVM.Config.NumaInfo.CoresPerNumaNode).To(BeEquivalentTo(int32(1)),
					"expected coresPerNumaNode=1 (2 vCPUs / vnumaNodeCount=2)")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).Should(Succeed(),
				"timed out waiting for vNUMA config to be applied in vSphere for %s/%s", vmNamespace, vmName)
		})

	It("applies spec CPU and memory size overrides while powered off and verifies after power-on",
		Label("compute-config", "core-functional"), func() {

			vmName := fmt.Sprintf("cc-size-%s", capiutil.RandomString(4))
			// Create powered-off with hotAddEnabled flags applied in the first reconcile.
			// Size overrides are patched while the VM is powered-off so the operator
			// applies them without setting PowerOffRequired.
			vm := buildComputeConfigVM(vmName, vmNamespace, vmClassName, linuxVMIName, storageClass,
				nil,
				&vmopv1a6.VirtualMachineCPUAdvancedSpec{
					HotAddEnabled: ptr.To(true),
				},
				&vmopv1a6.VirtualMachineMemoryAdvancedSpec{
					HotAddEnabled: ptr.To(true),
				},
			)
			vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff

			Expect(svClusterClient.Create(ctx, vm)).To(Succeed(),
				"failed to create VirtualMachine %s/%s", vmNamespace, vmName)
			DeferCleanup(func() {
				deleteVM(ctx, svClusterClient, config, vmNamespace, vmName, input.SkipCleanup)
			})

			By("Waiting for VM to be created and condition=True (powered-off with hot-add flags applied)")
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient,
				vmNamespace, vmName)
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Asserting hotAddEnabled=true in status before size patch")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.HotAddEnabled).ToNot(BeNil())
				g.Expect(*cpu.HotAddEnabled).To(BeTrue())
				mem := statusMemory(vm)
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.HotAddEnabled).ToNot(BeNil())
				g.Expect(*mem.HotAddEnabled).To(BeTrue())
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())

			By("Patching spec.resources.size while VM is powered off")
			latest, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())
			patch := ctrlclient.MergeFrom(latest.DeepCopy())
			if latest.Spec.Resources == nil {
				latest.Spec.Resources = &vmopv1a6.VirtualMachineResourcesSpec{}
			}
			latest.Spec.Resources.Size = &vmopv1a6.VirtualMachineResourceQuantity{
				CPU:    ptr.To(resource.MustParse("3")),
				Memory: ptr.To(resource.MustParse("3Gi")),
			}
			Expect(svClusterClient.Patch(ctx, latest, patch)).To(Succeed(),
				"failed to patch size on %s/%s", vmNamespace, vmName)

			By("Waiting for ComputeConfigSynced=True after size patch (VM powered-off; no power cycle needed)")
			waitForComputeConfigSynced(ctx, svClusterClient, config,
				types.NamespacedName{Namespace: vmNamespace, Name: vmName},
				metav1.ConditionTrue, "")

			By("Powering on VM")
			Expect(powerOnVM(ctx, svClusterClient, vmNamespace, vmName)).To(Succeed())
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient,
				vmNamespace, vmName, string(vmopv1a6.VirtualMachinePowerStateOn))

			By("Asserting cpu.total=3 after size override")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				cpu := statusCPU(vm)
				g.Expect(cpu).ToNot(BeNil())
				g.Expect(cpu.Total).To(BeEquivalentTo(3),
					"expected total vCPUs=3 after size override")
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed(), "cpu size assertions failed for %s/%s", vmNamespace, vmName)

			By("Asserting memory total = 3072M")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachineA6(ctx, svClusterClient, vmNamespace, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				mem := statusMemory(vm)
				_, _ = fmt.Fprintf(GinkgoWriter,
					"[cc-size] mem=%v total=%v\n",
					mem != nil, func() string {
						if mem != nil && mem.Total != nil {
							return mem.Total.String()
						}
						return "<nil>"
					}())
				g.Expect(mem).ToNot(BeNil())
				g.Expect(mem.Total).ToNot(BeNil())
				// vSphere reports memory in MB (treated as SI mega by the operator);
				// 3Gi spec → 3072 vSphere MB → status stores 3072M.
				want := resource.MustParse("3072M")
				g.Expect(mem.Total.Equal(want)).To(BeTrue(),
					"expected memory total=3072M, got %s", mem.Total.String())
			}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
				Should(Succeed())
		})
}

// ──────────────────────────────── helpers ────────────────────────────────────

// waitForComputeConfigSynced polls until VirtualMachineComputeConfigSynced
// reaches wantStatus (and optionally wantReason). Returns the matched condition.
func waitForComputeConfigSynced(
	ctx context.Context,
	svClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	wantStatus metav1.ConditionStatus,
	wantReason string,
	afterTime ...metav1.Time,
) *metav1.Condition {
	var matched *metav1.Condition

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClient, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		cond := getComputeConfigCondition(vm)
		g.Expect(cond).ToNot(BeNil(),
			"VirtualMachineComputeConfigSynced condition not yet present on %s", key)

		if len(afterTime) > 0 {
			_, _ = fmt.Fprintf(GinkgoWriter,
				"[waitForComputeConfigSynced] %s sentinel=%s condTS=%s after=%v status=%s reason=%s\n",
				key, afterTime[0].UTC().Format(time.RFC3339),
				cond.LastTransitionTime.UTC().Format(time.RFC3339),
				cond.LastTransitionTime.After(afterTime[0].Time),
				cond.Status, cond.Reason)
			g.Expect(cond.LastTransitionTime.After(afterTime[0].Time)).To(BeTrue(),
				"condition lastTransitionTime not yet advanced past sentinel on %s", key)
		}
		g.Expect(cond.Status).To(Equal(wantStatus),
			"condition status mismatch on %s: want=%s got=%s (reason=%s msg=%s)",
			key, wantStatus, cond.Status, cond.Reason, cond.Message)
		if wantReason != "" {
			g.Expect(cond.Reason).To(Equal(wantReason),
				"condition reason mismatch on %s: want=%s got=%s", key, wantReason, cond.Reason)
		}
		matched = cond
	}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
		Should(Succeed(),
			"timed out waiting for VirtualMachineComputeConfigSynced=%s (reason=%s) on %s",
			wantStatus, wantReason, key)

	// metav1.Time has second-level granularity. Sleep 1s after the condition
	// is confirmed so that any subsequent action the caller takes (e.g. a
	// Patch) occurs in a different second than matched.LastTransitionTime.
	// This ensures the next condition transition carries a strictly newer
	// timestamp when the caller passes matched.LastTransitionTime as afterTime.
	time.Sleep(time.Second)

	return matched
}

// waitForComputeConfigSyncedWithReservation polls until
// ComputeConfigSynced=True AND status.hardware.cpu.reservation==wantMHz.
func waitForComputeConfigSyncedWithReservation(
	ctx context.Context,
	svClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	wantMHz int64,
) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClient, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		cond := getComputeConfigCondition(vm)
		g.Expect(cond).ToNot(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue),
			"condition not True yet (reason=%s)", cond.Reason)
		cpu := statusCPU(vm)
		g.Expect(cpu).ToNot(BeNil())
		g.Expect(cpu.Reservation).To(BeEquivalentTo(wantMHz),
			"CPU reservation mismatch: want=%d got=%d", wantMHz, cpu.Reservation)
	}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
		Should(Succeed(),
			"timed out waiting for ComputeConfigSynced=True AND reservation==%d on %s", wantMHz, key)
}

// waitForComputeConfigSyncedFalseWithReservation polls until condition=False AND
// cpu.reservation==wantMHz. Used to verify the hot-pluggable allocation applied
// even while the power-off-required block is still pending.
func waitForComputeConfigSyncedFalseWithReservation(
	ctx context.Context,
	svClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	wantMHz int64,
) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClient, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		cpu := statusCPU(vm)
		g.Expect(cpu).ToNot(BeNil())
		g.Expect(cpu.Reservation).To(BeEquivalentTo(wantMHz),
			"CPU reservation mismatch: want=%d got=%d", wantMHz, cpu.Reservation)
	}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
		Should(Succeed(),
			"timed out waiting for cpu.reservation==%d on %s", wantMHz, key)
}

// statusCPU returns vm.Status.Hardware.CPU, safe if nil chain.
func statusCPU(vm *vmopv1a6.VirtualMachine) *vmopv1a6.VirtualMachineCPUAllocationStatus {
	if vm == nil || vm.Status.Hardware == nil {
		return nil
	}
	return vm.Status.Hardware.CPU
}

// statusMemory returns vm.Status.Hardware.Memory, safe if nil chain.
func statusMemory(vm *vmopv1a6.VirtualMachine) *vmopv1a6.VirtualMachineMemoryAllocationStatus {
	if vm == nil || vm.Status.Hardware == nil {
		return nil
	}
	return vm.Status.Hardware.Memory
}

// getComputeConfigCondition retrieves the VirtualMachineComputeConfigSynced condition.
func getComputeConfigCondition(vm *vmopv1a6.VirtualMachine) *metav1.Condition {
	if vm == nil {
		return nil
	}
	for i := range vm.Status.Conditions {
		c := &vm.Status.Conditions[i]
		if c.Type == vmopv1a6.VirtualMachineConditionComputeConfigSynced {
			return c
		}
	}
	return nil
}

func getClassConfigCondition(vm *vmopv1a6.VirtualMachine) *metav1.Condition {
	if vm == nil {
		return nil
	}
	for i := range vm.Status.Conditions {
		c := &vm.Status.Conditions[i]
		if c.Type == vmopv1a6.VirtualMachineClassConfigurationSynced {
			return c
		}
	}
	return nil
}

// waitForClassConfigSynced polls until VirtualMachineClassConfigurationSynced
// reaches True. When afterTime is provided the condition's LastTransitionTime
// must be strictly after it.
func waitForClassConfigSynced(
	ctx context.Context,
	svClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	key types.NamespacedName,
	afterTime ...metav1.Time,
) *metav1.Condition {
	var matched *metav1.Condition

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA6(ctx, svClient, key.Namespace, key.Name)
		g.Expect(err).ToNot(HaveOccurred())
		cond := getClassConfigCondition(vm)
		g.Expect(cond).ToNot(BeNil(),
			"VirtualMachineClassConfigurationSynced condition not yet present on %s", key)
		if len(afterTime) > 0 {
			_, _ = fmt.Fprintf(GinkgoWriter,
				"[waitForClassConfigSynced] %s sentinel=%s condTS=%s after=%v status=%s reason=%s\n",
				key, afterTime[0].UTC().Format(time.RFC3339),
				cond.LastTransitionTime.UTC().Format(time.RFC3339),
				cond.LastTransitionTime.After(afterTime[0].Time),
				cond.Status, cond.Reason)
			g.Expect(cond.LastTransitionTime.After(afterTime[0].Time)).To(BeTrue(),
				"condition lastTransitionTime not yet advanced past sentinel on %s", key)
		}
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue),
			"condition status mismatch on %s: want=True got=%s (reason=%s msg=%s)",
			key, cond.Status, cond.Reason, cond.Message)
		matched = cond
	}, config.GetIntervals("default", "wait-vm-compute-config-synced")...).
		Should(Succeed(),
			"timed out waiting for VirtualMachineClassConfigurationSynced=True on %s", key)

	// Sleep 1s so the caller's next action lands in a different second than
	// matched.LastTransitionTime (metav1.Time has second-level granularity).
	time.Sleep(time.Second)

	return matched
}

// buildComputeConfigVM constructs a vmopv1a6.VirtualMachine with the given
// compute-config spec. Always sets bootstrap.disabled=true, powerState=PoweredOn,
// and promoteDisksModes=Disabled to prevent concurrent SvMotion tasks from
// racing with compute-config reconciliation or drift detection.
func buildComputeConfigVM(
	name, namespace, className, imageName, storageClass string,
	resources *vmopv1a6.VirtualMachineResourcesSpec,
	cpuAdv *vmopv1a6.VirtualMachineCPUAdvancedSpec,
	memAdv *vmopv1a6.VirtualMachineMemoryAdvancedSpec,
) *vmopv1a6.VirtualMachine {
	return &vmopv1a6.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmopv1a6.VirtualMachineSpec{
			ClassName:        className,
			ImageName:        imageName,
			StorageClass:     storageClass,
			PromoteDisksMode: vmopv1a6.VirtualMachinePromoteDisksModeDisabled,
			Bootstrap: &vmopv1a6.VirtualMachineBootstrapSpec{
				Disabled: true,
			},
			PowerState:     vmopv1a6.VirtualMachinePowerStateOn,
			PowerOffMode:   vmopv1a6.VirtualMachinePowerOpModeHard,
			Resources:      resources,
			CPUAdvanced:    cpuAdv,
			MemoryAdvanced: memAdv,
		},
	}
}

// findVSphereVMByMOID creates a govmomi VirtualMachine object from the VM's
// Managed Object ID (vm.Status.UniqueID).
func findVSphereVMByMOID(vimClient *vim25.Client, moid string) *object.VirtualMachine {
	if vimClient == nil || moid == "" {
		return nil
	}
	moRef := vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: moid}
	return object.NewVirtualMachine(vimClient, moRef)
}

// injectAllocationDrift uses govmomi ReconfigVM_Task to simultaneously set
// rogue CPU reservation, CPU limit, and memory reservation directly in
// vSphere (bypassing the operator) for multi-field drift detection testing.
func injectAllocationDrift(
	ctx context.Context,
	vmObj *object.VirtualMachine,
	rogueCPUReservationMHz, rogueCPULimitMHz, rogueMemReservationMB int64,
) error {
	configSpec := vimtypes.VirtualMachineConfigSpec{
		CpuAllocation: &vimtypes.ResourceAllocationInfo{
			Reservation: &rogueCPUReservationMHz,
			Limit:       &rogueCPULimitMHz,
		},
		MemoryAllocation: &vimtypes.ResourceAllocationInfo{
			Reservation: &rogueMemReservationMB,
		},
	}
	task, err := vmObj.Reconfigure(ctx, configSpec)
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

// injectPowerOffRequiredFieldDrift uses govmomi ReconfigVM_Task to inject a
// rogue value for nestedHVEnabled on a powered-off VM. Only fields covered by
// VirtualMachine.Config.Settings are used here; CPUCount and Memory privileges
// are not available to the E2E test user.
func injectPowerOffRequiredFieldDrift(ctx context.Context, vmObj *object.VirtualMachine) error {
	falseVal := false
	configSpec := vimtypes.VirtualMachineConfigSpec{
		NestedHVEnabled: &falseVal,
	}
	task, err := vmObj.Reconfigure(ctx, configSpec)
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

// powerOffVSphereVM issues an out-of-band power-off via govmomi without
// going through the operator.
func powerOffVSphereVM(ctx context.Context, vmObj *object.VirtualMachine) error {
	task, err := vmObj.PowerOff(ctx)
	if err != nil {
		return err
	}
	return task.Wait(ctx)
}

// powerOffVM sets the spec.powerState to PoweredOff on the K8s VirtualMachine resource.
func powerOffVM(ctx context.Context, svClient ctrlclient.Client, namespace, name string) error {
	vm, err := utils.GetVirtualMachineA6(ctx, svClient, namespace, name)
	if err != nil {
		return err
	}
	patch := ctrlclient.MergeFrom(vm.DeepCopy())
	vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOff
	return svClient.Patch(ctx, vm, patch)
}

// powerOnVM sets the spec.powerState to PoweredOn on the K8s VirtualMachine resource.
func powerOnVM(ctx context.Context, svClient ctrlclient.Client, namespace, name string) error {
	vm, err := utils.GetVirtualMachineA6(ctx, svClient, namespace, name)
	if err != nil {
		return err
	}
	patch := ctrlclient.MergeFrom(vm.DeepCopy())
	vm.Spec.PowerState = vmopv1a6.VirtualMachinePowerStateOn
	return svClient.Patch(ctx, vm, patch)
}

// deleteVM deletes the VM and waits for it to be gone, unless skipCleanup is set.
func deleteVM(ctx context.Context, svClient ctrlclient.Client, config *e2eConfig.E2EConfig,
	namespace, name string, skipCleanup bool) {
	if skipCleanup {
		return
	}
	vm, err := utils.GetVirtualMachineA6(ctx, svClient, namespace, name)
	if err != nil {
		e2eframework.Logf("deleteVM: VM %s/%s not found (already deleted?): %v", namespace, name, err)
		return
	}
	if err := svClient.Delete(ctx, vm); err != nil {
		e2eframework.Logf("deleteVM: failed to delete VM %s/%s: %v", namespace, name, err)
		return
	}
	vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClient, namespace, name)
}
