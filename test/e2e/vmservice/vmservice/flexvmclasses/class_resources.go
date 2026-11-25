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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	// Specs for classes with min/max ranges are added with the v2 class API
	// (T912, vmop-4327): no supported path can author such a class today,
	// because the standard E2E user cannot write classes through the Kubernetes
	// API and the VI admin API cannot express ranges.

	// New behavior: the installed class CRDs print the size defaults and
	// reject zero or negative limits. Classes cannot be written by the E2E
	// user, so this reads the installed schema; envtest covers admission.
	ginkgo.It("publishes the v1alpha6 class size columns and limit rule", ginkgo.Label(FeatureLabel, "extended-functional", "experimental"), func() {
		names := []string{"virtualmachineclasses"}
		// The class instance CRD is installed only with immutable classes.
		if input.Config.SupervisorCapabilities[consts.VMServiceImmutableClassesCapabilityName] {
			names = append(names, "virtualmachineclassinstances")
		}
		for _, name := range names {
			version := classCRDVersion(ctx, client, name+".vmoperator.vmware.com", "v1alpha6")
			columns, _, err := unstructured.NestedSlice(version, "additionalPrinterColumns")
			Expect(err).NotTo(HaveOccurred())
			paths := map[string]any{}
			for _, column := range columns {
				column := column.(map[string]any)
				paths[column["name"].(string)] = column["jsonPath"]
			}
			Expect(paths).To(HaveKeyWithValue("CPU", ".spec.cpu.default"), name)
			Expect(paths).To(HaveKeyWithValue("Memory", ".spec.memory.default"), name)

			rules, _, err := unstructured.NestedSlice(version,
				"schema", "openAPIV3Schema", "properties", "spec", "properties", "resources", "properties", "limits", "x-kubernetes-validations")
			Expect(err).NotTo(HaveOccurred())
			Expect(rules).To(ContainElement(HaveKeyWithValue("message", "limits must be greater than 0")), name)
		}
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
		expectFixedRange(a6.Spec.CPU, *resource.NewQuantity(cpuCount, resource.DecimalSI), "size cpu")
		expectFixedRange(a6.Spec.Memory, a5.Spec.Hardware.Memory, "size memory")
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
}

// classCRDVersion returns one served version of an installed CRD.
func classCRDVersion(ctx context.Context, client ctrlclient.Client, name, version string) map[string]any {
	ginkgo.GinkgoHelper()
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	Expect(client.Get(ctx, ctrlclient.ObjectKey{Name: name}, crd)).To(Succeed(), "failed to get CRD %s", name)
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range versions {
		if item := entry.(map[string]any); item["name"] == version {
			return item
		}
	}
	ginkgo.Fail("CRD " + name + " does not serve " + version)
	return nil
}
