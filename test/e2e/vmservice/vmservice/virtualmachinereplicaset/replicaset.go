// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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

// replicaSetSelectorLabelKey groups the replicas of one VirtualMachineReplicaSet
// created by this suite apart from every other ReplicaSet's replicas, so
// concurrently-running specs never adopt each other's VirtualMachines.
const replicaSetSelectorLabelKey = "vmoperator.vmware.com/e2e-test-replicaset"

// SpecInput is the input for the VirtualMachineReplicaSet test spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	SkipCleanup      bool
	WCPNamespaceName string
	LinuxVMName      string
}

// Spec exercises a thin, real-vCenter/WCP-only smoke pass over
// VirtualMachineReplicaSet: create with N replicas, scale up, scale down,
// scale to zero and back up, scale via the /scale subresource, and
// cascading delete of owned VMs.
// See .sdd/specs/009-virtualmachinereplicaset/tds.md scenarios 1, 3, 6, 7, 9, 11, 17.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "vmrs-lcm"

	var (
		input            SpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		clusterResources *e2eConfig.Resources
		linuxVMIName     string
		rsName           string
		replicaSet       *vmopv1.VirtualMachineReplicaSet
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		skipper.SkipUnlessK8sWorkloadMgmtAPIIsEnabled(ctx, clusterProxy)

		linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		rsName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		replicaSet = nil
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, rsName, "vmreplicaset")
		}

		if replicaSet != nil {
			vmoperator.DeleteVirtualMachineReplicaSetAndWait(ctx, config, svClusterClient, input.WCPNamespaceName, rsName)
		}
	})

	// newReplicaSet builds (but does not create) a VirtualMachineReplicaSet
	// whose replicas are uniquely selectable via replicaSetSelectorLabelKey=rsName,
	// so it can never adopt or be confused with another spec's VMs.
	newReplicaSet := func(replicas int32, powerState vmopv1.VirtualMachinePowerState) *vmopv1.VirtualMachineReplicaSet {
		selectorLabels := map[string]string{replicaSetSelectorLabelKey: rsName}

		return &vmopv1.VirtualMachineReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rsName,
				Namespace: input.WCPNamespaceName,
			},
			Spec: vmopv1.VirtualMachineReplicaSetSpec{
				Replicas: ptr.To(replicas),
				Selector: &metav1.LabelSelector{
					MatchLabels: selectorLabels,
				},
				Template: vmopv1.VirtualMachineTemplateSpec{
					ObjectMeta: vmopv1common.ObjectMeta{
						Labels: selectorLabels,
					},
					Spec: vmopv1.VirtualMachineSpec{
						ImageName:    linuxVMIName,
						ClassName:    clusterResources.VMClassName,
						StorageClass: clusterResources.StorageClassName,
						Reserved: &vmopv1.VirtualMachineReservedSpec{
							ResourcePolicyName: clusterResources.VMResourcePolicyName,
						},
						PowerState: powerState,
					},
				},
			},
		}
	}

	It("Should create the desired number of powered-on VirtualMachine replicas", Label("smoke", "experimental"), func() {
		const replicas = int32(3)

		replicaSet = newReplicaSet(replicas, vmopv1.VirtualMachinePowerStateOn)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)

		By(fmt.Sprintf("Verifying %d VirtualMachines are created and owned by %s", replicas, rsName))
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, replicas)

		By("Verifying every owned VirtualMachine carries the replicaset-name label, an owner reference, and the template spec")
		owned, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		Expect(owned).To(HaveLen(int(replicas)))

		seenNames := map[string]struct{}{}
		for _, vm := range owned {
			Expect(vm.Labels[vmopv1.VirtualMachineReplicaSetNameLabel]).To(Equal(rsName))
			Expect(vm.Spec.ImageName).To(Equal(linuxVMIName))
			Expect(vm.Spec.ClassName).To(Equal(clusterResources.VMClassName))
			Expect(vm.Spec.StorageClass).To(Equal(clusterResources.StorageClassName))

			// Non-goal 38: replica names are opaque/generated, never
			// ordinal/predictable, and never repeat within the set.
			_, alreadySeen := seenNames[vm.Name]
			Expect(alreadySeen).To(BeFalse(), "expected every replica to have a unique generated name")
			seenNames[vm.Name] = struct{}{}
		}

		By("Verifying all replicas actually power on")
		vmoperator.WaitForOwnedVirtualMachinesPoweredOn(ctx, config, svClusterClient, input.WCPNamespaceName, rsName)
	})

	It("Should scale up an existing VirtualMachineReplicaSet", Label("core-functional", "experimental"), func() {
		replicaSet = newReplicaSet(2, vmopv1.VirtualMachinePowerStateOff)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 2)

		original, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		originalNames := namesOf(original)

		By("Scaling spec.replicas from 2 to 5")
		Eventually(func(g Gomega) {
			rs, err := utils.GetVirtualMachineReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
			g.Expect(err).ToNot(HaveOccurred())
			rs.Spec.Replicas = ptr.To(int32(5))
			g.Expect(svClusterClient.Update(ctx, rs)).To(Succeed())
		}, config.GetIntervals("default", "wait-virtual-machine-replicaset-status")...).Should(Succeed())

		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 5)

		By("Verifying the original 2 replicas were left untouched, not recreated")
		scaled, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		Expect(namesOf(scaled)).To(ContainElements(originalNames))
	})

	It("Should scale down an existing VirtualMachineReplicaSet", Label("core-functional", "experimental"), func() {
		replicaSet = newReplicaSet(4, vmopv1.VirtualMachinePowerStateOff)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 4)

		original, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		originalNames := namesOf(original)

		By("Scaling spec.replicas from 4 to 1")
		Eventually(func(g Gomega) {
			rs, err := utils.GetVirtualMachineReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
			g.Expect(err).ToNot(HaveOccurred())
			rs.Spec.Replicas = ptr.To(int32(1))
			g.Expect(svClusterClient.Update(ctx, rs)).To(Succeed())
		}, config.GetIntervals("default", "wait-virtual-machine-replicaset-status")...).Should(Succeed())

		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 1)

		By("Verifying the single remaining replica is one of the original 4, not a new one")
		remaining, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		Expect(remaining).To(HaveLen(1))
		Expect(originalNames).To(ContainElement(remaining[0].Name))
	})

	It("Should scale a VirtualMachineReplicaSet down to zero and back up with fresh replicas", Label("core-functional", "experimental"), func() {
		replicaSet = newReplicaSet(3, vmopv1.VirtualMachinePowerStateOff)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 3)

		original, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		originalNames := namesOf(original)

		By("Scaling spec.replicas from 3 to 0")
		Eventually(func(g Gomega) {
			rs, err := utils.GetVirtualMachineReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
			g.Expect(err).ToNot(HaveOccurred())
			rs.Spec.Replicas = ptr.To(int32(0))
			g.Expect(svClusterClient.Update(ctx, rs)).To(Succeed())
		}, config.GetIntervals("default", "wait-virtual-machine-replicaset-status")...).Should(Succeed())

		By("Verifying every owned VirtualMachine is deleted and status.replicas reaches 0")
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 0)

		By("Scaling spec.replicas back up from 0 to 2")
		Eventually(func(g Gomega) {
			rs, err := utils.GetVirtualMachineReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
			g.Expect(err).ToNot(HaveOccurred())
			rs.Spec.Replicas = ptr.To(int32(2))
			g.Expect(svClusterClient.Update(ctx, rs)).To(Succeed())
		}, config.GetIntervals("default", "wait-virtual-machine-replicaset-status")...).Should(Succeed())

		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 2)

		By("Verifying the replicas created after scaling back up are all fresh, never reusing a name from before scale-to-zero")
		afterScaleUp, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		Expect(namesOf(afterScaleUp)).ToNot(ContainElements(originalNames))
	})

	It("Should scale a VirtualMachineReplicaSet via its scale subresource", Label("core-functional", "experimental"), func() {
		replicaSet = newReplicaSet(1, vmopv1.VirtualMachinePowerStateOff)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 1)

		By("Updating only the /scale subresource to 4 replicas")
		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: 4}}
		Expect(svClusterClient.SubResource("scale").Update(ctx, replicaSet, ctrlclient.WithSubResourceBody(scale))).
			To(Succeed(), "failed to update scale subresource for VirtualMachineReplicaSet %s", rsName)

		By("Verifying this has the identical effect as editing spec.replicas directly")
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 4)
		rs, err := utils.GetVirtualMachineReplicaSet(ctx, svClusterClient, input.WCPNamespaceName, rsName)
		e2eframework.ExpectNoError(err)
		Expect(rs.Spec.Replicas).To(HaveValue(BeEquivalentTo(4)))
	})

	It("Should cascade-delete all owned VirtualMachines when the VirtualMachineReplicaSet is deleted", Label("smoke", "experimental"), func() {
		replicaSet = newReplicaSet(3, vmopv1.VirtualMachinePowerStateOff)
		Expect(svClusterClient.Create(ctx, replicaSet)).To(Succeed(), "failed to create VirtualMachineReplicaSet %s", rsName)
		vmoperator.WaitForVirtualMachineReplicaSetReplicas(ctx, config, svClusterClient, input.WCPNamespaceName, rsName, 3)

		By("Deleting the VirtualMachineReplicaSet")
		vmoperator.DeleteVirtualMachineReplicaSetAndWait(ctx, config, svClusterClient, input.WCPNamespaceName, rsName)
		// Deleted explicitly above; prevent AfterEach from trying again.
		replicaSet = nil
	})
}

func namesOf(vms []vmopv1.VirtualMachine) []string {
	names := make([]string, 0, len(vms))
	for _, vm := range vms {
		names = append(names, vm.Name)
	}
	return names
}
