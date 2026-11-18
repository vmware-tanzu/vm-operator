// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package flexvmclasses

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

// ClassResourcesSpec verifies how VM class resources (size, requests and
// limits) are exposed and realized. Every new behavior is paired with a check
// of the existing behavior it must not break: classes created through the
// VI admin API, the v1alpha5 API, and writers that predate v1alpha6 ranges.
func ClassResourcesSpec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "flex-vm-classes-class-resources"

	var (
		input     SpecInput
		proxy     *common.VMServiceClusterProxy
		client    ctrlclient.Client
		vimClient *vim25.Client
		imageName string
		storage   string
	)

	ginkgo.BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).NotTo(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).NotTo(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPClient).NotTo(BeNil(), "Invalid argument. input.WCPClient can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).NotTo(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMServiceClasslessVMCapabilityName)
		if phase := stagedUpgradePhase(); phase != "" {
			ginkgo.Skip(fmt.Sprintf("%s=%s selects the staged upgrade specs only", upgradePhaseEnv, phase))
		}

		proxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		client = proxy.GetClient()
		resources := input.Config.InfraConfig.ManagementClusterConfig.Resources
		storage = resources.StorageClassName
		imageName = vmoperator.WaitForVirtualMachineImageName(ctx, &input.Config.Config, client, input.WCPNamespaceName, vmservice.GetDefaultImageDisplayName(resources))

		vimClient = vcenter.NewVimClientFromKubeconfig(ctx, proxy.GetKubeconfigPath())
		ginkgo.DeferCleanup(func(context.Context) { vcenter.LogoutVimClient(vimClient) })
	})

	// New behavior: a class authored with v1alpha6 ranges.
	ginkgo.It("realizes the v1alpha6 size defaults for a v1alpha6 VirtualMachine", ginkgo.Label(FeatureLabel, "core-functional", "experimental"), func() {
		className := "flex-new-" + capiutil.RandomString(6)
		classCPU, classMemory := resource.MustParse("2"), resource.MustParse("2Gi")
		class := &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Namespace: input.WCPNamespaceName, Name: className},
			Spec: vmopv1.VirtualMachineClassSpec{
				Resources: vmopv1.VirtualMachineClassResources{
					Size: vmopv1.VirtualMachineClassResourceSpec{
						CPU:    &vmopv1.ResourceQuantityRange{Default: &classCPU},
						Memory: &vmopv1.ResourceQuantityRange{Default: &classMemory},
					},
				},
			},
		}
		Expect(client.Create(ctx, class)).To(Succeed(), "failed to create v1alpha6 VirtualMachineClass")
		// DeferCleanup nodes run last-in-first-out, so the VM cleanup that
		// CreateVirtualMachine registers below deletes the VM and waits for
		// it to be gone before the class is deleted.
		ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
			Expect(ctrlclient.IgnoreNotFound(client.Delete(cleanupCtx, class))).To(Succeed(), "failed to delete v1alpha6 VirtualMachineClass")
		})

		vmName := "flex-new-vm-" + capiutil.RandomString(6)
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Namespace: input.WCPNamespaceName, Name: vmName},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:    imageName,
				ClassName:    className,
				StorageClass: storage,
				PowerState:   vmopv1.VirtualMachinePowerStateOff,
			},
		}
		vmoperator.CreateVirtualMachine(ctx, input.Config, proxy, vm, false)
		moID := vmoperator.WaitForVirtualMachineMOID(ctx, input.Config, client, input.WCPNamespaceName, vmName)

		got := &vmopv1.VirtualMachineClass{}
		Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(class), got)).To(Succeed())
		Expect(got.Spec.Resources.Size.CPU).NotTo(BeNil())
		Expect(got.Spec.Resources.Size.Memory).NotTo(BeNil())
		Expect(got.Spec.Resources.Size.CPU.Default.Cmp(classCPU)).To(BeZero())
		Expect(got.Spec.Resources.Size.Memory.Default.Cmp(classMemory)).To(BeZero())

		s := getComputeSnapshot(ctx, vimClient, moID)
		Expect(s.NumCPU).To(BeEquivalentTo(classCPU.Value()))
		Expect(s.MemoryMB).To(BeEquivalentTo(classMemory.Value() / mib))
		Expect(s.CPUReservation).To(BeZero(), "a class without requests must not reserve CPU")
		Expect(s.MemoryReservation).To(BeZero(), "a class without requests must not reserve memory")
	})

	// Existing behavior: a class created through the VI admin API.
	ginkgo.It("exposes a legacy VI admin class through v1alpha5 and v1alpha6 and realizes it for a v1alpha5 VirtualMachine", ginkgo.Label(FeatureLabel, "core-functional", "experimental"), func() {
		const (
			cpuCount      = 2
			memoryMB      = 2048
			reservationPC = 50
		)
		className := "flex-legacy-" + capiutil.RandomString(6)

		// CPUReservation and MemoryReservation are percentages of the class's
		// CPU and memory, as with the existing "guaranteed" test classes
		// that use 100.
		cpu, mem, cpuRes, memRes := cpuCount, memoryMB, reservationPC, reservationPC
		Expect(input.WCPClient.CreateVMClass(wcp.VMClassSpec{
			ID: className, CPUCount: &cpu, MemoryMB: &mem, CPUReservation: &cpuRes, MemoryReservation: &memRes,
		})).To(Succeed(), "failed to create legacy VM class %s", className)
		ginkgo.DeferCleanup(func() { vmservice.VerifyVMClassDeletion(input.WCPClient, className) })
		Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, className, input.WCPNamespaceName)).To(Succeed())

		classKey := ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: className}

		ginkgo.By("reading the class as v1alpha5 with its original values")
		a5 := &vmopv1a5.VirtualMachineClass{}
		Eventually(func(g Gomega) {
			g.Expect(client.Get(ctx, classKey, a5)).To(Succeed())
		}, 5*time.Minute, 5*time.Second).Should(Succeed())
		Expect(a5.Spec.Hardware.Cpus).To(BeEquivalentTo(cpuCount))
		Expect(a5.Spec.Hardware.Memory.Value()).To(BeEquivalentTo(memoryMB * mib))
		requests := a5.Spec.Policies.Resources.Requests
		Expect(requests.Cpu.IsZero()).To(BeFalse(), "a CPU reservation must appear as a CPU request")
		Expect(requests.Memory.Value()).To(BeEquivalentTo(memoryMB*mib*reservationPC/100), "memory request must be the reserved share of memory")
		Expect(a5.Spec.Policies.Resources.Limits.Cpu.IsZero()).To(BeTrue())
		Expect(a5.Spec.Policies.Resources.Limits.Memory.IsZero()).To(BeTrue())

		ginkgo.By("reading the class as v1alpha6 with fixed ranges that match")
		a6 := &vmopv1.VirtualMachineClass{}
		Expect(client.Get(ctx, classKey, a6)).To(Succeed())
		expectFixedRange(a6.Spec.Resources.Size.CPU, *resource.NewQuantity(cpuCount, resource.DecimalSI), "size cpu")
		expectFixedRange(a6.Spec.Resources.Size.Memory, a5.Spec.Hardware.Memory, "size memory")
		expectLegacyAllocation(a6.Spec.Resources.Requests.CPU, requests.Cpu, "requests cpu")
		expectLegacyAllocation(a6.Spec.Resources.Requests.Memory, requests.Memory, "requests memory")
		Expect(a6.Spec.Resources.Limits.CPU).To(BeNil())
		Expect(a6.Spec.Resources.Limits.Memory).To(BeNil())

		ginkgo.By("creating a v1alpha5 VirtualMachine from the class")
		vmName := "flex-legacy-vm-" + capiutil.RandomString(6)
		vm := manifestbuilders.VirtualMachineA5(manifestbuilders.VirtualMachineYaml{
			Namespace: input.WCPNamespaceName, Name: vmName, ImageName: imageName,
			VMClassName: className, StorageClassName: storage, PowerState: "PoweredOff",
		})
		vmoperator.CreateVirtualMachine(ctx, input.Config, proxy, vm, false)
		moID := vmoperator.WaitForVirtualMachineMOID(ctx, input.Config, client, input.WCPNamespaceName, vmName)

		ginkgo.By("verifying the realized vSphere hardware and allocations")
		s := getComputeSnapshot(ctx, vimClient, moID)
		Expect(s.NumCPU).To(BeEquivalentTo(cpuCount))
		Expect(s.MemoryMB).To(BeEquivalentTo(memoryMB))
		Expect(s.CPUReservation).To(BeNumerically(">", 0))
		Expect(s.CPULimit).To(BeEquivalentTo(-1))
		Expect(s.MemoryReservation).To(BeEquivalentTo(memoryMB * reservationPC / 100))
		Expect(s.MemoryLimit).To(BeEquivalentTo(-1))
	})

	// Existing behavior: an older client writing a class that has v1alpha6
	// ranges only changes the values it changes.
	ginkgo.It("preserves v1alpha6 ranges across v1alpha5 writes unless the writer changes the projected value", ginkgo.Label(FeatureLabel, "core-functional", "experimental"), func() {
		className := "flex-compat-" + capiutil.RandomString(6)
		q := resource.MustParse
		class := &vmopv1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Namespace: input.WCPNamespaceName, Name: className},
			Spec: vmopv1.VirtualMachineClassSpec{
				Description: "original",
				Resources: vmopv1.VirtualMachineClassResources{
					Size: vmopv1.VirtualMachineClassResourceSpec{
						CPU:    &vmopv1.ResourceQuantityRange{Min: ptrQ(q("1")), Default: ptrQ(q("2")), Max: ptrQ(q("4"))},
						Memory: &vmopv1.ResourceQuantityRange{Min: ptrQ(q("1Gi")), Default: ptrQ(q("2Gi")), Max: ptrQ(q("4Gi"))},
					},
				},
			},
		}
		Expect(client.Create(ctx, class)).To(Succeed(), "failed to create v1alpha6 VirtualMachineClass")
		ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
			Expect(ctrlclient.IgnoreNotFound(client.Delete(cleanupCtx, class))).To(Succeed())
		})
		key := ctrlclient.ObjectKeyFromObject(class)

		expectRanges := func(wantCPU resource.Quantity, fixedCPU bool) {
			ginkgo.GinkgoHelper()
			got := &vmopv1.VirtualMachineClass{}
			Expect(client.Get(ctx, key, got)).To(Succeed())
			if fixedCPU {
				expectFixedRange(got.Spec.Resources.Size.CPU, wantCPU, "size cpu")
			} else {
				expectRange(got.Spec.Resources.Size.CPU, q("1"), q("2"), q("4"), "size cpu")
			}
			expectRange(got.Spec.Resources.Size.Memory, q("1Gi"), q("2Gi"), q("4Gi"), "size memory")
		}

		ginkgo.By("writing an unrelated field through v1alpha5")
		a5 := &vmopv1a5.VirtualMachineClass{}
		Expect(client.Get(ctx, key, a5)).To(Succeed())
		Expect(a5.Spec.Hardware.Cpus).To(BeEquivalentTo(2), "v1alpha5 projects the size default")
		a5.Spec.Description = "edited by an older client"
		Expect(client.Update(ctx, a5)).To(Succeed(), "failed to update the class through v1alpha5")

		got := &vmopv1.VirtualMachineClass{}
		Expect(client.Get(ctx, key, got)).To(Succeed())
		Expect(got.Spec.Description).To(Equal("edited by an older client"))
		expectRanges(resource.Quantity{}, false)

		ginkgo.By("changing the projected CPU value through v1alpha5")
		Expect(client.Get(ctx, key, a5)).To(Succeed())
		a5.Spec.Hardware.Cpus = 3
		Expect(client.Update(ctx, a5)).To(Succeed(), "failed to update the class through v1alpha5")
		expectRanges(q("3"), true)
	})
}

func ptrQ(q resource.Quantity) *resource.Quantity { return &q }
