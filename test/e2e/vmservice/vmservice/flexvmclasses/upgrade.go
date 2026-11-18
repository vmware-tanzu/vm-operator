// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package flexvmclasses

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
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

const (
	upgradePhaseEnv      = "FLEX_UPGRADE_PHASE"
	upgradeRunIDEnv      = "FLEX_UPGRADE_RUN_ID"
	upgradeCheckpointPre = "flex-"
)

// stagedUpgradePhase returns the staged upgrade phase, or "" when the staged
// upgrade is not being run.
func stagedUpgradePhase() string { return os.Getenv(upgradePhaseEnv) }

// legacyCase is a legacy class fixture. Later stories (devices, instance
// storage, reserved classes, shares) add entries to legacyCases. The generic
// checks in the staged upgrade apply to every case; the hooks add checks that
// are specific to it.
type legacyCase struct {
	// Suffix is part of the class and VM names; keep it short and DNS safe.
	Suffix string
	// Build returns the VI admin API spec the legacy class is created with.
	Build func(className string) wcp.VMClassSpec
	// Edit returns the VI admin API spec used after the upgrade to edit the
	// class the way a vCenter that was not upgraded would.
	Edit func(created wcp.VMClassSpec) wcp.VMClassSpec
	// VerifyPrepared checks facts that must hold before the upgrade.
	VerifyPrepared func(rec legacyRecord)
	// VerifyConverted checks the converted v1alpha6 class after the upgrade.
	VerifyConverted func(class *vmopv1.VirtualMachineClass, rec legacyRecord)
	// VerifyEdited checks the v1alpha6 class after Edit was applied.
	VerifyEdited func(class *vmopv1.VirtualMachineClass, rec legacyRecord, edited wcp.VMClassSpec)
}

var legacyCases = []legacyCase{
	{
		Suffix: "be",
		Build: func(className string) wcp.VMClassSpec {
			return wcp.VMClassSpec{ID: className, CPUCount: new(2), MemoryMB: new(2048)}
		},
		Edit: func(created wcp.VMClassSpec) wcp.VMClassSpec {
			created.CPUCount = new(*created.CPUCount + 1)
			return created
		},
		VerifyPrepared: func(rec legacyRecord) {
			Expect(rec.Compute.CPUReservation).To(BeZero())
			Expect(rec.Compute.MemoryReservation).To(BeZero())
		},
		VerifyConverted: func(class *vmopv1.VirtualMachineClass, _ legacyRecord) {
			Expect(class.Spec.Resources.Requests.CPU).To(BeNil())
			Expect(class.Spec.Resources.Requests.Memory).To(BeNil())
		},
		VerifyEdited: func(class *vmopv1.VirtualMachineClass, _ legacyRecord, _ wcp.VMClassSpec) {
			Expect(class.Spec.Resources.Requests.CPU).To(BeNil())
			Expect(class.Spec.Resources.Requests.Memory).To(BeNil())
		},
	},
	{
		Suffix: "gt",
		Build: func(className string) wcp.VMClassSpec {
			// The reservations are percentages of the class's CPU and memory.
			return wcp.VMClassSpec{
				ID: className, CPUCount: new(2), MemoryMB: new(2048),
				CPUReservation: new(50), MemoryReservation: new(50),
			}
		},
		Edit: func(created wcp.VMClassSpec) wcp.VMClassSpec {
			created.MemoryMB = new(*created.MemoryMB * 2)
			return created
		},
		VerifyPrepared: func(rec legacyRecord) {
			Expect(rec.Compute.CPUReservation).To(BeNumerically(">", 0))
			Expect(rec.Compute.MemoryReservation).To(BeEquivalentTo(rec.Compute.MemoryMB / 2))
		},
		VerifyConverted: func(class *vmopv1.VirtualMachineClass, rec legacyRecord) {
			Expect(class.Spec.Resources.Requests.CPU).NotTo(BeNil())
			Expect(class.Spec.Resources.Requests.Memory).NotTo(BeNil())
			Expect(class.Spec.Resources.Requests.Memory.Default.Value()).To(BeEquivalentTo(rec.Compute.MemoryReservation * mib))
		},
		VerifyEdited: func(class *vmopv1.VirtualMachineClass, _ legacyRecord, edited wcp.VMClassSpec) {
			Expect(class.Spec.Resources.Requests.Memory).NotTo(BeNil())
			Expect(class.Spec.Resources.Requests.Memory.Default.Value()).To(BeEquivalentTo(int64(*edited.MemoryMB) * mib / 2))
		},
	},
}

// legacyRecord is what the prepare phase records for one legacy case.
type legacyRecord struct {
	Suffix     string `json:"suffix"`
	ClassName  string `json:"className"`
	VMName     string `json:"vmName"`
	ClassUID   string `json:"classUID"`
	VMUID      string `json:"vmUID"`
	VMMOID     string `json:"vmMOID"`
	PowerState string `json:"powerState"`

	// Legacy v1alpha5 view of the class.
	CPUs          int64             `json:"cpus"`
	Memory        resource.Quantity `json:"memory"`
	CPURequest    resource.Quantity `json:"cpuRequest"`
	MemoryRequest resource.Quantity `json:"memoryRequest"`
	CPULimit      resource.Quantity `json:"cpuLimit"`
	MemoryLimit   resource.Quantity `json:"memoryLimit"`

	// Compute is the realized configuration of the powered-on VM.
	Compute computeSnapshot `json:"compute"`
}

type upgradeCheckpoint struct {
	RunID             string         `json:"runID"`
	Namespace         string         `json:"namespace"`
	Phase             string         `json:"phase"`
	ImageName         string         `json:"imageName"`
	OriginalVMClasses []string       `json:"originalVMClasses"`
	Cases             []legacyRecord `json:"cases"`
}

func (c *upgradeCheckpoint) record(suffix string) legacyRecord {
	ginkgo.GinkgoHelper()
	for _, r := range c.Cases {
		if r.Suffix == suffix {
			return r
		}
	}
	ginkgo.Fail(fmt.Sprintf("checkpoint has no record for case %q", suffix))
	return legacyRecord{}
}

// UpgradeSpec provides prepare and verify phases for a real operator upgrade.
// The specs intentionally do not deploy or upgrade the operator; the external
// runner does that between the two invocations.
func UpgradeSpec(ctx context.Context, inputGetter func() SpecInput) {
	var (
		input         SpecInput
		client        ctrlclient.Client
		proxy         *common.VMServiceClusterProxy
		checkpoint    upgradeCheckpoint
		checkpointKey ctrlclient.ObjectKey
	)

	className := func(c legacyCase) string { return "flex-" + c.Suffix + "-" + checkpoint.RunID }
	vmName := func(c legacyCase) string { return "flex-vm-" + c.Suffix + "-" + checkpoint.RunID }

	ginkgo.BeforeEach(func() {
		phase := stagedUpgradePhase()
		if phase == "" {
			ginkgo.Skip("set " + upgradePhaseEnv + " to run the staged upgrade fixture")
		}
		input = inputGetter()
		Expect(input.Config).NotTo(BeNil())
		Expect(input.ClusterProxy).NotTo(BeNil())
		Expect(input.WCPClient).NotTo(BeNil())
		Expect(input.WCPNamespaceName).NotTo(BeEmpty())
		Expect(os.Getenv("E2E_NAMESPACE")).To(Equal(input.WCPNamespaceName), "the upgrade fixture requires an explicitly supplied, persistent E2E_NAMESPACE")
		checkpoint.RunID = os.Getenv(upgradeRunIDEnv)
		Expect(validation.IsDNS1123Label(checkpoint.RunID)).To(BeEmpty(), "%s must be a DNS label", upgradeRunIDEnv)
		Expect(len(checkpoint.RunID)).To(BeNumerically("<=", 31), "%s must be at most 31 characters", upgradeRunIDEnv)
		Expect([]string{"prepare", "verify"}).To(ContainElement(phase), "%s must be prepare or verify", upgradePhaseEnv)
		checkpoint.Namespace = input.WCPNamespaceName
		checkpointKey = ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: upgradeCheckpointPre + checkpoint.RunID}
		proxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		client = proxy.GetClient()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMServiceClasslessVMCapabilityName)
	})

	ginkgo.It("prepares legacy Flex VM Classes fixtures for an operator upgrade", ginkgo.Label(FeatureLabel, "core-functional", "experimental"), func() {
		if stagedUpgradePhase() != "prepare" {
			ginkgo.Skip("set " + upgradePhaseEnv + "=prepare to create the pre-upgrade fixtures")
		}

		ginkgo.By("recording the namespace's pre-existing class access list")
		namespace, err := input.WCPClient.GetNamespace(checkpoint.Namespace)
		Expect(err).NotTo(HaveOccurred())
		checkpoint.OriginalVMClasses = append([]string(nil), namespace.VMServiceSpec.VMClasses...)

		ginkgo.By("finding the namespace's image using the baseline v1alpha5 API")
		// The old release does not serve the candidate's v1alpha6 image API.
		imageDisplayName := input.Config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName
		Expect(imageDisplayName).NotTo(BeEmpty())
		Eventually(func(g Gomega) {
			images := &vmopv1a5.VirtualMachineImageList{}
			g.Expect(client.List(ctx, images, ctrlclient.InNamespace(checkpoint.Namespace))).To(Succeed())
			for _, image := range images.Items {
				if image.Status.Name == imageDisplayName && len(image.Status.Disks) > 0 && image.Status.ProviderContentVersion != "" {
					checkpoint.ImageName = image.Name
					break
				}
			}
			g.Expect(checkpoint.ImageName).NotTo(BeEmpty(), "configured Photon image must be ready in the persistent namespace")
		}, input.Config.GetIntervals("default", "wait-virtual-machine-image-creation")...).Should(Succeed())

		checkpoint.Phase = "preparing"
		checkpointConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: checkpoint.Namespace, Name: checkpointKey.Name},
			Data:       map[string]string{},
		}
		writeCheckpoint := func() {
			ginkgo.GinkgoHelper()
			data, err := json.Marshal(checkpoint)
			Expect(err).NotTo(HaveOccurred())
			checkpointConfigMap.Data["checkpoint.json"] = string(data)
		}
		writeCheckpoint()
		Expect(client.Create(ctx, checkpointConfigMap)).To(Succeed(), "checkpoint already exists; use a new %s", upgradeRunIDEnv)

		vimClient := vcenter.NewVimClientFromKubeconfig(ctx, proxy.GetKubeconfigPath())
		ginkgo.DeferCleanup(func(context.Context) { vcenter.LogoutVimClient(vimClient) })

		for _, c := range legacyCases {
			rec := prepareLegacyCase(ctx, input, client, vimClient, checkpoint, c, className(c), vmName(c))
			checkpoint.Cases = append(checkpoint.Cases, rec)
		}

		ginkgo.By("persisting a durable checkpoint for the candidate-release verify phase")
		checkpoint.Phase = "ready"
		writeCheckpoint()
		Expect(client.Update(ctx, checkpointConfigMap)).To(Succeed())
	})

	ginkgo.It("verifies legacy Flex VM Classes fixtures after an operator upgrade", ginkgo.Label(FeatureLabel, "core-functional", "experimental"), func() {
		if stagedUpgradePhase() != "verify" {
			ginkgo.Skip("set " + upgradePhaseEnv + "=verify after deploying the candidate operator")
		}

		ginkgo.By("loading the pre-upgrade checkpoint")
		checkpointConfigMap := &corev1.ConfigMap{}
		Expect(client.Get(ctx, checkpointKey, checkpointConfigMap)).To(Succeed(), "prepare phase checkpoint is missing")
		Expect(json.Unmarshal([]byte(checkpointConfigMap.Data["checkpoint.json"]), &checkpoint)).To(Succeed())
		Expect(checkpoint.RunID).To(Equal(os.Getenv(upgradeRunIDEnv)))
		Expect(checkpoint.Namespace).To(Equal(input.WCPNamespaceName))
		Expect(checkpoint.Phase).To(Equal("ready"))

		verificationPassed := false
		ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
			if !verificationPassed || ginkgo.CurrentSpecReport().Failed() {
				_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Keeping upgrade fixture %q because verification did not pass\n", checkpoint.RunID)
				return
			}
			ginkgo.By("deleting the candidate VMs and the VMs preserved through upgrade")
			for _, c := range legacyCases {
				vmoperator.DeleteVirtualMachineAndWait(cleanupCtx, input.Config, client, checkpoint.Namespace, vmName(c)+"-post")
				vmoperator.DeleteVirtualMachineAndWait(cleanupCtx, input.Config, client, checkpoint.Namespace, vmName(c))
			}
			ginkgo.By("restoring the namespace's original VMClass access list")
			err := input.WCPClient.UpdateNamespaceVMServiceSpec(checkpoint.Namespace, wcp.NamespaceUpdateVMserviceSpec{VMClasses: &checkpoint.OriginalVMClasses})
			Expect(err).NotTo(HaveOccurred())
			wcp.WaitForNamespaceReady(input.WCPClient, checkpoint.Namespace)
			ginkgo.By("deleting the test VMClasses")
			for _, c := range legacyCases {
				vmservice.VerifyVMClassDeletion(input.WCPClient, className(c))
			}
			Expect(client.Delete(cleanupCtx, checkpointConfigMap)).To(Succeed())
		})

		vimClient := vcenter.NewVimClientFromKubeconfig(ctx, proxy.GetKubeconfigPath())
		ginkgo.DeferCleanup(func(context.Context) { vcenter.LogoutVimClient(vimClient) })

		for _, c := range legacyCases {
			ginkgo.By(fmt.Sprintf("verifying legacy case %q", c.Suffix))
			verifyLegacyCase(ctx, input, client, proxy, vimClient, checkpoint, c, checkpoint.record(c.Suffix))
		}

		verificationPassed = true
	})
}

// prepareLegacyCase creates one legacy class and a powered-on v1alpha5 VM from
// it, using only baseline-compatible APIs, and records what it observed.
func prepareLegacyCase(
	ctx context.Context,
	input SpecInput,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	checkpoint upgradeCheckpoint,
	c legacyCase,
	className, vmName string) legacyRecord {

	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("creating legacy case %q through the VI admin API", c.Suffix))
	Expect(input.WCPClient.CreateVMClass(c.Build(className))).To(Succeed())
	Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, className, checkpoint.Namespace)).To(Succeed())

	rec := legacyRecord{Suffix: c.Suffix, ClassName: className, VMName: vmName}

	ginkgo.By("capturing the baseline v1alpha5 class")
	legacyClass := &vmopv1a5.VirtualMachineClass{}
	Eventually(func(g Gomega) {
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: className}, legacyClass)).To(Succeed())
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
	rec.ClassUID = string(legacyClass.UID)
	Expect(rec.ClassUID).NotTo(BeEmpty())
	rec.CPUs = legacyClass.Spec.Hardware.Cpus
	rec.Memory = legacyClass.Spec.Hardware.Memory
	rec.CPURequest = legacyClass.Spec.Policies.Resources.Requests.Cpu
	rec.MemoryRequest = legacyClass.Spec.Policies.Resources.Requests.Memory
	rec.CPULimit = legacyClass.Spec.Policies.Resources.Limits.Cpu
	rec.MemoryLimit = legacyClass.Spec.Policies.Resources.Limits.Memory

	ginkgo.By("creating a powered-on v1alpha5 VirtualMachine")
	vmObj := manifestbuilders.VirtualMachineA5(manifestbuilders.VirtualMachineYaml{
		Namespace: checkpoint.Namespace, Name: vmName,
		ImageName: checkpoint.ImageName, VMClassName: className,
		StorageClassName: input.Config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
		PowerState:       "PoweredOn",
	})
	Expect(client.Create(ctx, vmObj)).To(Succeed(), "failed to create baseline v1alpha5 VM")

	var vm *vmopv1a5.VirtualMachine
	Eventually(func(g Gomega) {
		vm = &vmopv1a5.VirtualMachine{}
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: vmName}, vm)).To(Succeed())
		g.Expect(vm.Status.UniqueID).NotTo(BeEmpty())
		g.Expect(vm.Status.PowerState).To(Equal(vmopv1a5.VirtualMachinePowerStateOn))
	}, 10*time.Minute, 10*time.Second).Should(Succeed())
	rec.VMUID = string(vm.UID)
	rec.VMMOID = vm.Status.UniqueID
	rec.PowerState = string(vm.Status.PowerState)

	ginkgo.By("capturing the baseline vSphere compute settings")
	rec.Compute = getComputeSnapshot(ctx, vimClient, rec.VMMOID)
	Expect(rec.Compute.NumCPU).To(BeEquivalentTo(rec.CPUs))
	Expect(rec.Compute.MemoryMB).To(BeEquivalentTo(rec.Memory.Value() / mib))
	if c.VerifyPrepared != nil {
		c.VerifyPrepared(rec)
	}
	return rec
}

// verifyLegacyCase checks one legacy case after the operator upgrade.
func verifyLegacyCase(
	ctx context.Context,
	input SpecInput,
	client ctrlclient.Client,
	proxy *common.VMServiceClusterProxy,
	vimClient *vim25.Client,
	checkpoint upgradeCheckpoint,
	c legacyCase,
	rec legacyRecord) {

	ginkgo.GinkgoHelper()

	classKey := ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: rec.ClassName}

	ginkgo.By("verifying the converted v1alpha6 class")
	class := &vmopv1.VirtualMachineClass{}
	Expect(client.Get(ctx, classKey, class)).To(Succeed())
	Expect(string(class.UID)).To(Equal(rec.ClassUID))
	expectFixedRange(class.Spec.Resources.Size.CPU, *resource.NewQuantity(rec.CPUs, resource.DecimalSI), "size cpu")
	expectFixedRange(class.Spec.Resources.Size.Memory, rec.Memory, "size memory")
	expectLegacyAllocation(class.Spec.Resources.Requests.CPU, rec.CPURequest, "requests cpu")
	expectLegacyAllocation(class.Spec.Resources.Requests.Memory, rec.MemoryRequest, "requests memory")
	expectLegacyAllocation(class.Spec.Resources.Limits.CPU, rec.CPULimit, "limits cpu")
	expectLegacyAllocation(class.Spec.Resources.Limits.Memory, rec.MemoryLimit, "limits memory")
	if c.VerifyConverted != nil {
		c.VerifyConverted(class, rec)
	}

	ginkgo.By("verifying the v1alpha5 view still returns the legacy fields")
	legacy := &vmopv1a5.VirtualMachineClass{}
	Expect(client.Get(ctx, classKey, legacy)).To(Succeed())
	Expect(string(legacy.UID)).To(Equal(rec.ClassUID))
	Expect(legacy.Spec.Hardware.Cpus).To(Equal(rec.CPUs))
	Expect(legacy.Spec.Hardware.Memory.Cmp(rec.Memory)).To(BeZero())
	Expect(legacy.Spec.Policies.Resources.Requests.Cpu.Cmp(rec.CPURequest)).To(BeZero())
	Expect(legacy.Spec.Policies.Resources.Requests.Memory.Cmp(rec.MemoryRequest)).To(BeZero())
	Expect(legacy.Spec.Policies.Resources.Limits.Cpu.Cmp(rec.CPULimit)).To(BeZero())
	Expect(legacy.Spec.Policies.Resources.Limits.Memory.Cmp(rec.MemoryLimit)).To(BeZero())

	ginkgo.By("checking the original VM identity and power state")
	vm := &vmopv1.VirtualMachine{}
	Eventually(func(g Gomega) {
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: rec.VMName}, vm)).To(Succeed())
		g.Expect(string(vm.UID)).To(Equal(rec.VMUID))
		g.Expect(vm.Status.UniqueID).To(Equal(rec.VMMOID))
		g.Expect(string(vm.Status.PowerState)).To(Equal(rec.PowerState))
	}, 5*time.Minute, 10*time.Second).Should(Succeed())

	ginkgo.By("checking the preserved VM's vSphere hardware and allocations")
	Expect(getComputeSnapshot(ctx, vimClient, rec.VMMOID)).To(Equal(rec.Compute))

	ginkgo.By("creating a new v1alpha6 VM from the upgraded class")
	postName := rec.VMName + "-post"
	postVM := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: checkpoint.Namespace, Name: postName},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:    vm.Spec.ImageName,
			ClassName:    rec.ClassName,
			StorageClass: input.Config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			PowerState:   vmopv1.VirtualMachinePowerStateOn,
		},
	}
	vmoperator.CreateVirtualMachine(ctx, input.Config, proxy, postVM, false)
	Eventually(func(g Gomega) {
		got := &vmopv1.VirtualMachine{}
		g.Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(postVM), got)).To(Succeed())
		g.Expect(got.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
		g.Expect(got.Status.UniqueID).NotTo(BeEmpty())
		postVM = got
	}, 10*time.Minute, 10*time.Second).Should(Succeed())
	Expect(getComputeSnapshot(ctx, vimClient, postVM.Status.UniqueID)).To(Equal(rec.Compute), "a new VM must realize the same compute settings as before the upgrade")

	ginkgo.By("editing the class through the VI admin API as a not-yet-upgraded vCenter would")
	// Build is called again so the edit starts from the spec the class was
	// created with.
	edited := c.Edit(c.Build(rec.ClassName))
	Expect(input.WCPClient.UpdateVMClass(edited)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(client.Get(ctx, classKey, class)).To(Succeed())
		g.Expect(class.Spec.Resources.Size.CPU).NotTo(BeNil())
		g.Expect(class.Spec.Resources.Size.Memory).NotTo(BeNil())
		g.Expect(class.Spec.Resources.Size.CPU.Default.Value()).To(BeEquivalentTo(*edited.CPUCount))
		g.Expect(class.Spec.Resources.Size.Memory.Default.Value()).To(BeEquivalentTo(int64(*edited.MemoryMB) * mib))
	}, 5*time.Minute, 5*time.Second).Should(Succeed(), "the VI admin edit must be reflected in the v1alpha6 view")
	expectFixedRange(class.Spec.Resources.Size.CPU, *resource.NewQuantity(int64(*edited.CPUCount), resource.DecimalSI), "edited size cpu")
	expectFixedRange(class.Spec.Resources.Size.Memory, *resource.NewQuantity(int64(*edited.MemoryMB)*mib, resource.BinarySI), "edited size memory")
	Expect(string(class.UID)).To(Equal(rec.ClassUID))
	if c.VerifyEdited != nil {
		c.VerifyEdited(class, rec, edited)
	}
}
