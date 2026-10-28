// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vmClassUpgradePhaseEnv         = "VMCLASS_UPGRADE_PHASE"
	vmClassUpgradeRunIDEnv         = "VMCLASS_UPGRADE_RUN_ID"
	vmClassUpgradeCheckpointPrefix = "vmcu-"
	vmClassUpgradeCPU              = 2
	vmClassUpgradeMemoryMB         = 2048
)

// VMClassSchemaUpgradeInput contains the dependencies for the staged upgrade test.
type VMClassSchemaUpgradeInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}

type vmClassUpgradeCheckpoint struct {
	RunID             string   `json:"runID"`
	Namespace         string   `json:"namespace"`
	Phase             string   `json:"phase"`
	ClassName         string   `json:"className"`
	VMName            string   `json:"vmName"`
	ClassUID          string   `json:"classUID"`
	VMUID             string   `json:"vmUID"`
	VMVCenterMOID     string   `json:"vmVCenterMOID"`
	PowerState        string   `json:"powerState"`
	CPU               int      `json:"cpu"`
	MemoryMB          int      `json:"memoryMB"`
	RealizedCPU       int32    `json:"realizedCPU"`
	RealizedMemoryMB  int32    `json:"realizedMemoryMB"`
	OriginalVMClasses []string `json:"originalVMClasses"`
}

// VMClassSchemaUpgradeSpec provides prepare and verify phases for a real
// operator upgrade. The test intentionally does not deploy or upgrade the
// operator; the external runner does that between the two invocations.
func VMClassSchemaUpgradeSpec(ctx context.Context, inputGetter func() VMClassSchemaUpgradeInput) {
	var (
		input         VMClassSchemaUpgradeInput
		client        ctrlclient.Client
		proxy         *common.VMServiceClusterProxy
		checkpoint    vmClassUpgradeCheckpoint
		checkpointKey ctrlclient.ObjectKey
	)

	BeforeEach(func() {
		if os.Getenv(vmClassUpgradePhaseEnv) == "" {
			Skip("set VMCLASS_UPGRADE_PHASE to run the staged upgrade fixture")
		}
		input = inputGetter()
		Expect(input.Config).NotTo(BeNil())
		Expect(input.ClusterProxy).NotTo(BeNil())
		Expect(input.WCPClient).NotTo(BeNil())
		Expect(input.WCPNamespaceName).NotTo(BeEmpty())
		Expect(os.Getenv("E2E_NAMESPACE")).To(Equal(input.WCPNamespaceName), "the upgrade fixture requires an explicitly supplied, persistent E2E_NAMESPACE")
		checkpoint.RunID = os.Getenv(vmClassUpgradeRunIDEnv)
		Expect(validation.IsDNS1123Label(checkpoint.RunID)).To(BeEmpty(), "%s must be a DNS label", vmClassUpgradeRunIDEnv)
		Expect(len(checkpoint.RunID)).To(BeNumerically("<=", 31), "%s must be at most 31 characters", vmClassUpgradeRunIDEnv)
		phase := os.Getenv(vmClassUpgradePhaseEnv)
		Expect([]string{"prepare", "verify"}).To(ContainElement(phase), "%s %s", vmClassUpgradePhaseEnv, "must be prepare or verify")
		checkpoint.Namespace = input.WCPNamespaceName
		checkpointKey = ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmClassUpgradeCheckpointPrefix + checkpoint.RunID}
		proxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		client = proxy.GetClient()
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, proxy, consts.TelcoVMServiceAPICapabilityName)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, proxy, capabilities.CapabilityKeyImmutableClasses)
	})

	It("prepares a legacy VMClass and powered-on VM for the operator upgrade", Label("core-functional", "experimental"), func() {
		if os.Getenv(vmClassUpgradePhaseEnv) != "prepare" {
			Skip("set VMCLASS_UPGRADE_PHASE=prepare to create the pre-upgrade fixture")
		}

		checkpoint.ClassName = "vmcu-class-" + checkpoint.RunID
		checkpoint.VMName = "vmcu-vm-" + checkpoint.RunID
		checkpoint.CPU = vmClassUpgradeCPU
		checkpoint.MemoryMB = vmClassUpgradeMemoryMB

		By("recording the namespace's pre-existing class access list")
		namespace, err := input.WCPClient.GetNamespace(checkpoint.Namespace)
		Expect(err).NotTo(HaveOccurred())
		checkpoint.OriginalVMClasses = append([]string(nil), namespace.VMServiceSpec.VMClasses...)

		checkpoint.Phase = "preparing"
		checkpointConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: checkpoint.Namespace, Name: checkpointKey.Name},
			Data:       map[string]string{},
		}
		checkpointJSON, err := json.Marshal(checkpoint)
		Expect(err).NotTo(HaveOccurred())
		checkpointConfigMap.Data["checkpoint.json"] = string(checkpointJSON)
		Expect(client.Create(ctx, checkpointConfigMap)).To(Succeed(), "checkpoint already exists; use a new VMCLASS_UPGRADE_RUN_ID")

		By("creating the old-release compatible DCLI VMClasses")
		cpuCount, memoryMB := checkpoint.CPU, checkpoint.MemoryMB
		Expect(input.WCPClient.CreateVMClass(wcp.VMClassSpec{
			ID: checkpoint.ClassName, CPUCount: &cpuCount, MemoryMB: &memoryMB,
		})).To(Succeed())
		Expect(vmservice.EnsureNamespaceHasAccess(input.WCPClient, checkpoint.ClassName, checkpoint.Namespace)).To(Succeed())

		By("capturing baseline API identities using v1alpha5 reads only")
		legacyClass := &vmopv1a5.VirtualMachineClass{}
		Eventually(func(g Gomega) {
			g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: checkpoint.ClassName}, legacyClass)).To(Succeed())
		}, 5*time.Minute, 5*time.Second).Should(Succeed())
		checkpoint.ClassUID = string(legacyClass.UID)
		Expect(checkpoint.ClassUID).NotTo(BeEmpty())

		By("creating an old API VirtualMachine with its resources omitted")
		// Use only the baseline API while finding the existing namespace image;
		// the old release does not serve the candidate's v1alpha6 image API.
		var imageName string
		imageDisplayName := input.Config.InfraConfig.ManagementClusterConfig.Resources.PhotonImageDisplayName
		Expect(imageDisplayName).NotTo(BeEmpty())
		Eventually(func(g Gomega) {
			images := &vmopv1a5.VirtualMachineImageList{}
			g.Expect(client.List(ctx, images, ctrlclient.InNamespace(checkpoint.Namespace))).To(Succeed())
			for _, image := range images.Items {
				if image.Status.Name == imageDisplayName && len(image.Status.Disks) > 0 && image.Status.ProviderContentVersion != "" {
					imageName = image.Name
					break
				}
			}
			g.Expect(imageName).NotTo(BeEmpty(), "configured Photon image must be ready in the persistent namespace")
		}, input.Config.GetIntervals("default", "wait-virtual-machine-image-creation")...).Should(Succeed())
		vmYAML := manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
			Namespace: checkpoint.Namespace, Name: checkpoint.VMName,
			ImageName: imageName, VMClassName: checkpoint.ClassName,
			StorageClassName: input.Config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			PowerState:       "PoweredOn",
		})
		Expect(proxy.CreateWithArgs(ctx, vmYAML)).To(Succeed(), "failed to create baseline v1alpha5 VM")

		By("waiting for the baseline VM to become powered on and capture its vCenter identity")
		var baselineVM *vmopv1a5.VirtualMachine
		Eventually(func(g Gomega) {
			baselineVM = &vmopv1a5.VirtualMachine{}
			g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: checkpoint.VMName}, baselineVM)).To(Succeed())
			g.Expect(baselineVM.Status.UniqueID).NotTo(BeEmpty())
			g.Expect(baselineVM.Status.PowerState).To(Equal(vmopv1a5.VirtualMachinePowerStateOn))
		}, 10*time.Minute, 10*time.Second).Should(Succeed())
		checkpoint.VMUID = string(baselineVM.UID)
		checkpoint.VMVCenterMOID = baselineVM.Status.UniqueID
		checkpoint.PowerState = string(baselineVM.Status.PowerState)

		By("capturing baseline vSphere compute settings")
		vimClient := vcenter.NewVimClientFromKubeconfig(ctx, proxy.GetKubeconfigPath())
		DeferCleanup(func(context.Context) { vcenter.LogoutVimClient(vimClient) })
		collector := property.DefaultCollector(vimClient)
		vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: checkpoint.VMVCenterMOID}
		var vmMO mo.VirtualMachine
		Expect(collector.RetrieveOne(ctx, vmRef, []string{"config"}, &vmMO)).To(Succeed())
		checkpoint.RealizedCPU = vmMO.Config.Hardware.NumCPU
		checkpoint.RealizedMemoryMB = vmMO.Config.Hardware.MemoryMB
		Expect(checkpoint.RealizedCPU).To(BeEquivalentTo(checkpoint.CPU))
		Expect(checkpoint.RealizedMemoryMB).To(BeEquivalentTo(checkpoint.MemoryMB))

		By("persisting a durable checkpoint for the candidate-release verify phase")
		checkpoint.Phase = "ready"
		checkpointJSON, err = json.Marshal(checkpoint)
		Expect(err).NotTo(HaveOccurred())
		checkpointConfigMap.Data["checkpoint.json"] = string(checkpointJSON)
		Expect(client.Update(ctx, checkpointConfigMap)).To(Succeed())
		Expect(checkpoint.PowerState).To(Equal(string(vmopv1a5.VirtualMachinePowerStateOn)))
	})

	It("verifies the legacy VMClass and powered-on VM after the operator upgrade", Label("core-functional", "experimental"), func() {
		if os.Getenv(vmClassUpgradePhaseEnv) != "verify" {
			Skip("set VMCLASS_UPGRADE_PHASE=verify after deploying the candidate operator")
		}

		By("loading the pre-upgrade checkpoint")
		checkpointConfigMap := &corev1.ConfigMap{}
		Expect(client.Get(ctx, checkpointKey, checkpointConfigMap)).To(Succeed(), "prepare phase checkpoint is missing")
		Expect(json.Unmarshal([]byte(checkpointConfigMap.Data["checkpoint.json"]), &checkpoint)).To(Succeed())
		Expect(checkpoint.RunID).To(Equal(os.Getenv(vmClassUpgradeRunIDEnv)))
		Expect(checkpoint.Namespace).To(Equal(input.WCPNamespaceName))
		Expect(checkpoint.Phase).To(Equal("ready"))

		verificationPassed := false
		DeferCleanup(func(cleanupCtx context.Context) {
			if !verificationPassed || CurrentSpecReport().Failed() {
				_, _ = fmt.Fprintf(GinkgoWriter, "Keeping upgrade fixture %q because verification did not pass\n", checkpoint.RunID)
				return
			}
			By("deleting the candidate VM and the VM preserved through upgrade")
			vmoperator.DeleteVirtualMachineAndWait(cleanupCtx, input.Config, client, checkpoint.Namespace, checkpoint.VMName+"-post")
			vmoperator.DeleteVirtualMachineAndWait(cleanupCtx, input.Config, client, checkpoint.Namespace, checkpoint.VMName)
			By("restoring the namespace's original VMClass access list")
			err := input.WCPClient.UpdateNamespaceVMServiceSpec(checkpoint.Namespace, wcp.NamespaceUpdateVMserviceSpec{VMClasses: &checkpoint.OriginalVMClasses})
			Expect(err).NotTo(HaveOccurred())
			wcp.WaitForNamespaceReady(input.WCPClient, checkpoint.Namespace)
			By("deleting the test VMClasses")
			vmservice.VerifyVMClassDeletion(input.WCPClient, checkpoint.ClassName)
			Expect(client.Delete(cleanupCtx, checkpointConfigMap)).To(Succeed())
		})

		By("verifying the converted typed v1alpha6 VMClass resource defaults")
		class := &vmopv1.VirtualMachineClass{}
		Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: checkpoint.ClassName}, class)).To(Succeed())
		Expect(string(class.UID)).To(Equal(checkpoint.ClassUID))
		Expect(class.Spec.Resources.Size.CPU).NotTo(BeNil())
		Expect(class.Spec.Resources.Size.CPU.Default).To(Equal(resource.NewQuantity(int64(checkpoint.CPU), resource.DecimalSI)))
		Expect(class.Spec.Resources.Size.Memory).NotTo(BeNil())
		Expect(class.Spec.Resources.Size.Memory.Default.Value()).To(Equal(int64(checkpoint.MemoryMB * 1024 * 1024)))

		By("checking the original VM identity, materialized resources, and power state after upgrade")
		vm := &vmopv1.VirtualMachine{}
		Eventually(func(g Gomega) {
			g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: checkpoint.VMName}, vm)).To(Succeed())
			g.Expect(string(vm.UID)).To(Equal(checkpoint.VMUID))
			g.Expect(vm.Status.UniqueID).To(Equal(checkpoint.VMVCenterMOID))
			g.Expect(string(vm.Status.PowerState)).To(Equal(checkpoint.PowerState))
			g.Expect(vm.Spec.Resources).NotTo(BeNil())
			g.Expect(vm.Spec.Resources.Size).NotTo(BeNil())
			g.Expect(vm.Spec.Resources.Size.CPU.Value()).To(Equal(int64(checkpoint.CPU)))
			g.Expect(vm.Spec.Resources.Size.Memory.Value()).To(Equal(int64(checkpoint.MemoryMB * 1024 * 1024)))
		}, 5*time.Minute, 10*time.Second).Should(Succeed())

		By("checking the preserved VM's actual vSphere compute configuration")
		vimClient := vcenter.NewVimClientFromKubeconfig(ctx, proxy.GetKubeconfigPath())
		DeferCleanup(func(context.Context) { vcenter.LogoutVimClient(vimClient) })
		collector := property.DefaultCollector(vimClient)
		vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: checkpoint.VMVCenterMOID}
		var vmMO mo.VirtualMachine
		Expect(collector.RetrieveOne(ctx, vmRef, []string{"config"}, &vmMO)).To(Succeed())
		Expect(vmMO.Config.Hardware.NumCPU).To(Equal(checkpoint.RealizedCPU))
		Expect(vmMO.Config.Hardware.MemoryMB).To(Equal(checkpoint.RealizedMemoryMB))

		By("creating a new typed v1alpha6 VM using the upgraded legacy class")
		postUpgradeVMName := checkpoint.VMName + "-post"
		newVM, err := manifestbuilders.VirtualMachineA6(manifestbuilders.VirtualMachineYaml{
			Namespace: checkpoint.Namespace, Name: postUpgradeVMName,
			ImageName: vm.Spec.ImageName, VMClassName: checkpoint.ClassName,
			StorageClassName: input.Config.InfraConfig.ManagementClusterConfig.Resources.StorageClassName,
			PowerState:       "PoweredOn",
		})
		Expect(err).NotTo(HaveOccurred())
		postUpgradeVMYAML := manifestbuilders.ToYAML(newVM)
		Expect(proxy.CreateWithArgs(ctx, postUpgradeVMYAML)).To(Succeed(), "failed to create v1alpha6 VM after upgrade")
		postUpgradeVM := &vmopv1.VirtualMachine{}
		Eventually(func(g Gomega) {
			g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: checkpoint.Namespace, Name: postUpgradeVMName}, postUpgradeVM)).To(Succeed())
			g.Expect(postUpgradeVM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
			g.Expect(postUpgradeVM.Status.UniqueID).NotTo(BeEmpty())
		}, 10*time.Minute, 10*time.Second).Should(Succeed())
		postUpgradeVMRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: postUpgradeVM.Status.UniqueID}
		var postUpgradeVMO mo.VirtualMachine
		Expect(collector.RetrieveOne(ctx, postUpgradeVMRef, []string{"config"}, &postUpgradeVMO)).To(Succeed())
		Expect(postUpgradeVMO.Config.Hardware.NumCPU).To(BeEquivalentTo(checkpoint.CPU))
		Expect(postUpgradeVMO.Config.Hardware.MemoryMB).To(BeEquivalentTo(checkpoint.MemoryMB))

		verificationPassed = true
	})
}
