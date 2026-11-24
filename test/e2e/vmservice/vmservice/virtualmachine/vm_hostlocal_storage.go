// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
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
	// esxHostMoIDNodeAnnotationKey is the annotation a Supervisor Node carries
	// naming the MoID of the ESXi host it corresponds to.
	esxHostMoIDNodeAnnotationKey = "vmware-system-esxi-node-moid"

	// pvcSelectedNodeAnnotationKey is the annotation VM Operator stamps on a
	// pending WaitForFirstConsumer PVC to name the node CNS must provision it
	// on.
	pvcSelectedNodeAnnotationKey = "volume.kubernetes.io/selected-node"
)

type VMHostLocalStorageSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	ArtifactFolder   string
	WCPNamespaceName string
}

// VMHostLocalStorageSpec verifies that a VM with host-local storage lands on
// the host that can reach its volumes, both when it is placed on its own and
// when it is placed with a VirtualMachineGroup. Both go through
// PlaceVmsXCluster, which keeps a disk named by its existing datastore path
// in place and only recommends hosts that can reach it (CRM-4964).
func VMHostLocalStorageSpec(ctx context.Context, inputGetter func() VMHostLocalStorageSpecInput) {
	const (
		specName = "vm-hostlocal"
		vmKind   = "VirtualMachine"
	)

	var (
		input            VMHostLocalStorageSpecInput
		config           *e2eConfig.E2EConfig
		clusterProxy     *common.VMServiceClusterProxy
		svClusterClient  ctrlclient.Client
		vCenterClient    *vim25.Client
		clusterResources *e2eConfig.Resources
		linuxVMIName     string
		namePrefix       string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.HostLocalStorageCapabilityName)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterResources = config.InfraConfig.ManagementClusterConfig.Resources
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() { vcenter.LogoutVimClient(vCenterClient) })

		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient,
			input.WCPNamespaceName, vmservice.GetDefaultImageDisplayName(clusterResources))

		namePrefix = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
	})

	Context("Single VM", func() {
		BeforeEach(func() {
			skipper.SkipIfEmpty(clusterResources.HostLocalImmediateStorageClassName, "HOST_LOCAL_IMMEDIATE_STORAGE_CLASS")
		})

		It("Should place a VM on the host of its Bound host-local PVC", Label("extended-functional"), func() {
			vmName := namePrefix + "-vm"
			pvc := hostLocalPVC(input.WCPNamespaceName, vmName, clusterResources.HostLocalImmediateStorageClassName)

			By("Creating a host-local PVC that is provisioned immediately")
			createPVCs(ctx, svClusterClient, pvc)
			pvHost := waitForHostLocalPVCHost(ctx, config, svClusterClient, input.WCPNamespaceName, pvc.ClaimName)

			By("Creating a VM that uses the PVC")
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy,
				hostLocalVM(input.WCPNamespaceName, vmName, "", linuxVMIName, clusterResources, pvc), false)
			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM is on the host that can reach the PVC")
			Expect(waitForVMHost(ctx, config, svClusterClient, vCenterClient, input.WCPNamespaceName, vmName)).
				To(Equal(pvHost), "VM %s is not on the host of its host-local PVC %s", vmName, pvc.ClaimName)
		})
	})

	Context("Group placement", func() {
		var (
			vmgName string
		)

		BeforeEach(func() {
			skipper.SkipUnlessSupervisorCapabilityEnabled(input.Config.SupervisorCapabilities, consts.VMGroupsCapabilityName)
			vmgName = namePrefix + "-vmg"
		})

		It("Should place each member on the host of its own Bound host-local PVC", Label("extended-functional"), func() {
			skipper.SkipIfEmpty(clusterResources.HostLocalImmediateStorageClassName, "HOST_LOCAL_IMMEDIATE_STORAGE_CLASS")

			vmNames := []string{namePrefix + "-vm1", namePrefix + "-vm2", namePrefix + "-vm3"}
			pvcs := make([]manifestbuilders.PVC, len(vmNames))
			for i, vmName := range vmNames {
				pvcs[i] = hostLocalPVC(input.WCPNamespaceName, vmName, clusterResources.HostLocalImmediateStorageClassName)
			}

			By("Creating a host-local PVC for each member that is provisioned immediately")
			createPVCs(ctx, svClusterClient, pvcs...)
			pvHosts := make([]string, len(pvcs))
			for i, pvc := range pvcs {
				pvHosts[i] = waitForHostLocalPVCHost(ctx, config, svClusterClient, input.WCPNamespaceName, pvc.ClaimName)
			}
			e2eframework.Logf("Host-local PVC hosts: %v", pvHosts)

			By("Creating a VirtualMachineGroup with the members")
			createHostLocalVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgName, vmKind, vmNames)

			By("Creating each member with its own PVC")
			for i, vmName := range vmNames {
				vmoperator.CreateVirtualMachine(ctx, config, clusterProxy,
					hostLocalVM(input.WCPNamespaceName, vmName, vmgName, linuxVMIName, clusterResources, pvcs[i]), false)
			}

			By("Verifying each member is on the host of its own PVC")
			for i, vmName := range vmNames {
				vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
				Expect(waitForVMHost(ctx, config, svClusterClient, vCenterClient, input.WCPNamespaceName, vmName)).
					To(Equal(pvHosts[i]), "VM %s is not on the host of its host-local PVC %s", vmName, pvcs[i].ClaimName)
			}
		})

		It("Should provision a member's WaitForFirstConsumer host-local PVC on the member's host", Label("extended-functional"), func() {
			skipper.SkipIfEmpty(clusterResources.HostLocalStorageClassName, "HOST_LOCAL_STORAGE_CLASS")

			vmName := namePrefix + "-vm1"
			pvc := hostLocalPVC(input.WCPNamespaceName, vmName, clusterResources.HostLocalStorageClassName)

			By("Creating a host-local PVC that waits for its first consumer")
			createPVCs(ctx, svClusterClient, pvc)

			By("Creating a VirtualMachineGroup with a member that uses the PVC")
			createHostLocalVMGroup(ctx, svClusterClient, input.WCPNamespaceName, vmgName, vmKind, []string{vmName})
			vmoperator.CreateVirtualMachine(ctx, config, clusterProxy,
				hostLocalVM(input.WCPNamespaceName, vmName, vmgName, linuxVMIName, clusterResources, pvc), false)
			vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			vmHost := waitForVMHost(ctx, config, svClusterClient, vCenterClient, input.WCPNamespaceName, vmName)

			By("Verifying the PVC is told to provision on the member's host, and binds")
			Eventually(func(g Gomega) {
				var obj corev1.PersistentVolumeClaim
				g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: pvc.ClaimName}, &obj)).To(Succeed())

				nodeName := obj.Annotations[pvcSelectedNodeAnnotationKey]
				g.Expect(nodeName).ToNot(BeEmpty(), "PVC %s has no selected node", pvc.ClaimName)
				g.Expect(nodeHostMoID(ctx, svClusterClient, nodeName)).To(Equal(vmHost))
				g.Expect(obj.Status.Phase).To(Equal(corev1.ClaimBound))
			}, config.GetIntervals("default", "wait-pvc-attachment")...).Should(Succeed(),
				"timed out waiting for PVC %s to be provisioned on host %s", pvc.ClaimName, vmHost)
		})
	})
}

// hostLocalPVC returns a 1Mi PVC of the given host-local storage class, named
// after the VM that uses it.
func hostLocalPVC(namespace, vmName, storageClassName string) manifestbuilders.PVC {
	return manifestbuilders.PVC{
		Namespace:        namespace,
		VolumeName:       vmName + "-volume",
		ClaimName:        vmName + "-claim",
		StorageClassName: storageClassName,
		RequestSize:      "1Mi",
	}
}

// createPVCs creates the given PVCs and deletes them when the spec ends.
func createPVCs(ctx context.Context, client ctrlclient.Client, pvcs ...manifestbuilders.PVC) {
	GinkgoHelper()

	objs, err := manifestbuilders.PersistentVolumeClaims(pvcs)
	Expect(err).ToNot(HaveOccurred(), "failed to build PVCs")

	for _, obj := range objs {
		Expect(client.Create(ctx, obj)).To(Succeed(), "failed to create PVC %s", obj.Name)
		DeferCleanup(func(ctx context.Context) {
			Expect(ctrlclient.IgnoreNotFound(client.Delete(ctx, obj))).To(Succeed(), "failed to delete PVC %s", obj.Name)
		})
	}
}

// hostLocalVM returns a powered off VM that mounts the given PVC. groupName
// is left empty for a VM that is placed on its own.
func hostLocalVM(
	namespace, name, groupName, imageName string,
	resources *e2eConfig.Resources,
	pvc manifestbuilders.PVC) *vmopv1a5.VirtualMachine {

	return &vmopv1a5.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmopv1a5.VirtualMachineSpec{
			GroupName:    groupName,
			ImageName:    imageName,
			ClassName:    resources.VMClassName,
			StorageClass: resources.StorageClassName,
			// Which host the VM lands on is known once it is created. Leaving
			// it powered off keeps the spec about placement.
			PowerState: vmopv1a5.VirtualMachinePowerStateOff,
			Volumes: []vmopv1a5.VirtualMachineVolume{
				{
					Name: pvc.VolumeName,
					VirtualMachineVolumeSource: vmopv1a5.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1a5.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pvc.ClaimName,
							},
						},
					},
				},
			},
		},
	}
}

// createHostLocalVMGroup creates a VirtualMachineGroup with the given VMs in
// a single boot order group, and deletes it when the spec ends.
func createHostLocalVMGroup(
	ctx context.Context,
	client ctrlclient.Client,
	namespace, name, vmKind string,
	vmNames []string) {

	GinkgoHelper()

	members := make([]vmopv1.GroupMember, 0, len(vmNames))
	for _, vmName := range vmNames {
		members = append(members, vmopv1.GroupMember{Kind: vmKind, Name: vmName})
	}

	vmg, err := manifestbuilders.VirtualMachineGroupWithBootOrderA5(manifestbuilders.VirtualMachineGroupYaml{
		Namespace: namespace,
		Name:      name,
		BootOrder: []manifestbuilders.BootOrder{{Members: members}},
	})
	Expect(err).ToNot(HaveOccurred(), "failed to build VirtualMachineGroup")
	Expect(client.Create(ctx, vmg)).To(Succeed(), "failed to create VirtualMachineGroup %s", name)
	DeferCleanup(func(ctx context.Context) {
		Expect(ctrlclient.IgnoreNotFound(client.Delete(ctx, vmg))).To(Succeed(), "failed to delete VirtualMachineGroup %s", name)
	})
}

// waitForVMHost waits for the VM to be created in vCenter and returns the
// MoID of its host.
func waitForVMHost(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	vimClient *vim25.Client,
	namespace, vmName string) string {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		var vm vmopv1.VirtualMachine
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: vmName}, &vm)).To(Succeed())
		g.Expect(vm.Status.UniqueID).ToNot(BeEmpty())
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"timed out waiting for VirtualMachine %s to be created in vCenter", vmName)

	return getVMHostFromVmodl(ctx, client, vimClient, namespace, vmName)
}

// waitForHostLocalPVCHost waits for the PVC to be Bound and returns the MoID
// of the one host that can reach its volume, as named by its PV's node
// affinity.
func waitForHostLocalPVCHost(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, pvcName string) string {

	GinkgoHelper()

	var pvc corev1.PersistentVolumeClaim
	Eventually(func(g Gomega) {
		g.Expect(client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: pvcName}, &pvc)).To(Succeed())
		g.Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound))
		g.Expect(pvc.Spec.VolumeName).ToNot(BeEmpty())
	}, config.GetIntervals("default", "wait-pvc-attachment")...).Should(Succeed(),
		"timed out waiting for PVC %s to be Bound", pvcName)

	var pv corev1.PersistentVolume
	Expect(client.Get(ctx, ctrlclient.ObjectKey{Name: pvc.Spec.VolumeName}, &pv)).To(Succeed(), "failed to get PV %s", pvc.Spec.VolumeName)
	Expect(pv.Spec.NodeAffinity).ToNot(BeNil(), "host-local PV %s has no node affinity", pv.Name)
	Expect(pv.Spec.NodeAffinity.Required).ToNot(BeNil(), "host-local PV %s has no required node affinity", pv.Name)

	var nodeNames []string
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == corev1.LabelHostname && expr.Operator == corev1.NodeSelectorOpIn {
				nodeNames = append(nodeNames, expr.Values...)
			}
		}
	}
	Expect(nodeNames).To(HaveLen(1), "host-local PV %s must name exactly one %s, has node affinity %v",
		pv.Name, corev1.LabelHostname, pv.Spec.NodeAffinity.Required)

	return nodeHostMoID(ctx, client, nodeNames[0])
}

// nodeHostMoID returns the MoID of the ESXi host that the named Supervisor
// Node corresponds to.
func nodeHostMoID(ctx context.Context, client ctrlclient.Client, nodeName string) string {
	GinkgoHelper()

	var node corev1.Node
	Expect(client.Get(ctx, ctrlclient.ObjectKey{Name: nodeName}, &node)).To(Succeed(), "failed to get Node %s", nodeName)

	moID := strings.TrimSpace(node.Annotations[esxHostMoIDNodeAnnotationKey])
	Expect(moID).ToNot(BeEmpty(), "Node %s has no %s annotation", nodeName, esxHostMoIDNodeAnnotationKey)

	return moID
}
