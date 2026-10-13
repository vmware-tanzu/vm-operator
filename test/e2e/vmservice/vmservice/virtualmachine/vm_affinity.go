// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// affinityTagResourceName returns the derived name of the Tag resource for a
// namespace and label key/value pair: "tag-" followed by the 16 hex character
// XXHash64 digest of "<namespace>:<key>:<value>". Byte-for-byte what
// pkg/providers/vsphere/virtualmachine.TagResourceName computes; reimplemented
// here rather than imported so this e2e module does not pick up that
// package's production dependency graph.
func affinityTagResourceName(namespace, key, value string) string {
	return fmt.Sprintf("tag-%016x", xxhash.Sum64String(namespace+":"+key+":"+value))
}

// VMAffinityTagSpecInput is the input to VMAffinityTagSpec.
type VMAffinityTagSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

// VMAffinityTagSpec exercises how VirtualMachines own Tag resources: a VM
// whose spec.affinity references a label pair owns the Tag for that pair,
// regardless of whether the VM itself carries the label, and the Tag lives
// for exactly as long as at least one VM owns it.
//
// The suite gates on VMAffinityDuringExecutionCapabilityName because it
// references labels via a required term whose topologyKey is
// kubernetes.io/hostname, which the admission webhook only accepts once that
// capability is enabled (without it, only topology.kubernetes.io/zone is
// accepted).
func VMAffinityTagSpec(ctx context.Context, inputGetter func() VMAffinityTagSpecInput) {
	const (
		specName    = "vm-affinity-tag"
		vmKind      = "VirtualMachine"
		vmgKind     = "VirtualMachineGroup"
		affinityKey = "tier"
	)

	var (
		input           VMAffinityTagSpecInput
		config          *e2eConfig.E2EConfig
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client

		linuxImageDisplayName string
		linuxVMIName          string

		vmNames []string

		vmgRootName    string
		vmgRootYaml    []byte
		vmgMemberNames []string
	)

	// affinityFor returns a spec.affinity referencing key:value via a
	// required VM-affinity term on the host topology.
	affinityFor := func(key, value string) *vmopv1.AffinitySpec {
		return &vmopv1.AffinitySpec{
			VMAffinity: &vmopv1.VMAffinitySpec{
				RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{key: value},
						},
						TopologyKey: corev1.LabelHostname,
					},
				},
			},
		}
	}

	// ensureVMInGroup adds vmName as a member of the suite's root
	// VirtualMachineGroup, creating the group on the first call and
	// applying the updated member list on every subsequent call.
	// spec.affinity requires a non-empty spec.groupName (webhook rule
	// validateVMAffinity, independent of this feature), so every VM this
	// suite creates must belong to a group.
	ensureVMInGroup := func(vmName string) {
		GinkgoHelper()

		vmgMemberNames = append(vmgMemberNames, vmName)

		vmgParameters := manifestbuilders.VirtualMachineGroupYaml{
			Namespace: input.WCPNamespaceName,
			Name:      vmgRootName,
		}
		for _, m := range vmgMemberNames {
			vmgParameters.Members = append(vmgParameters.Members, vmopv1.GroupMember{Kind: vmKind, Name: m})
		}
		vmgRootYaml = manifestbuilders.GetVirtualMachineGroupYaml(vmgParameters)

		if len(vmgMemberNames) == 1 {
			Expect(clusterProxy.CreateWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to create VirtualMachineGroup %q", vmgRootName)
		} else {
			Expect(clusterProxy.ApplyWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to update VirtualMachineGroup %q", vmgRootName)
		}
	}

	// vmYAML renders a powered-on VM carrying labels and, when affinity is
	// non-nil, referencing it via spec.affinity, as a member of groupName.
	vmYAML := func(vmName, groupName string, labels map[string]string, affinity *vmopv1.AffinitySpec) []byte {
		clusterResources := config.InfraConfig.ManagementClusterConfig.Resources

		return manifestbuilders.GetVirtualMachineYamlA5(manifestbuilders.VirtualMachineYaml{
			Namespace:        input.WCPNamespaceName,
			Name:             vmName,
			GroupName:        groupName,
			Labels:           labels,
			Affinity:         affinity,
			ImageName:        linuxVMIName,
			VMClassName:      clusterResources.VMClassName,
			StorageClassName: clusterResources.StorageClassName,
			PowerState:       "PoweredOn",
		})
	}

	// waitForVMReady waits for the VM to exist, be created, and be powered on.
	waitForVMReady := func(vmName string) {
		GinkgoHelper()

		vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOn")
	}

	// createVM creates a powered-on VM in the suite's root VirtualMachineGroup.
	createVM := func(vmName string, labels map[string]string, affinity *vmopv1.AffinitySpec) {
		GinkgoHelper()

		ensureVMInGroup(vmName)

		yaml := vmYAML(vmName, vmgRootName, labels, affinity)
		e2eframework.Logf("VM YAML:\n%s", string(yaml))
		Expect(clusterProxy.CreateWithArgs(ctx, yaml)).To(Succeed(), "failed to create VM %q:\n %s", vmName, string(yaml))

		vmNames = append(vmNames, vmName)

		waitForVMReady(vmName)
	}

	// deleteVM deletes a VM and waits for it to be gone, removing it from
	// vmNames so AfterEach does not try again.
	deleteVM := func(vmName string) {
		GinkgoHelper()

		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		Expect(err).ToNot(HaveOccurred(), "failed to get VM %q before deleting it", vmName)
		Expect(svClusterClient.Delete(ctx, vm)).To(Succeed(), "failed to delete VM %q", vmName)

		vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

		for i, n := range vmNames {
			if n == vmName {
				vmNames = append(vmNames[:i], vmNames[i+1:]...)
				break
			}
		}
	}

	// deferDeleteVM registers cleanup for a VM that a spec creates outside
	// the suite's root group.
	deferDeleteVM := func(vmName string) {
		DeferCleanup(func() {
			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			if err == nil {
				Expect(svClusterClient.Delete(ctx, vm)).To(Succeed(), "failed to delete VM %q", vmName)
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}
		})
	}

	// createGroup creates a VirtualMachineGroup with the given members and
	// registers its cleanup.
	createGroup := func(groupName string, members ...string) {
		GinkgoHelper()

		groupParameters := manifestbuilders.VirtualMachineGroupYaml{
			Namespace: input.WCPNamespaceName,
			Name:      groupName,
		}
		for _, m := range members {
			groupParameters.Members = append(groupParameters.Members, vmopv1.GroupMember{Kind: vmKind, Name: m})
		}
		groupYaml := manifestbuilders.GetVirtualMachineGroupYaml(groupParameters)

		Expect(clusterProxy.CreateWithArgs(ctx, groupYaml)).To(Succeed(), "failed to create VirtualMachineGroup %q", groupName)
		DeferCleanup(func() {
			Expect(clusterProxy.DeleteWithArgs(ctx, groupYaml)).To(Succeed(), "failed to delete VirtualMachineGroup %q", groupName)
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, groupName)
		})
	}

	// getTag returns the Tag resource with the given derived name, or nil
	// if it does not exist (or is not yet observable).
	getTag := func(name string) *vspherepolv1alpha1.Tag {
		tag := &vspherepolv1alpha1.Tag{}
		if err := svClusterClient.Get(
			ctx,
			ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: name},
			tag); err != nil {

			return nil
		}
		return tag
	}

	// waitForNoTag asserts that the Tag CR derived from key/value never
	// appears within the wait interval, so a regression that creates one
	// anyway is caught rather than racing to a false pass.
	waitForNoTag := func(key, value string) {
		GinkgoHelper()

		name := affinityTagResourceName(input.WCPNamespaceName, key, value)
		Consistently(func() bool {
			return getTag(name) == nil
		}, config.GetIntervals("default", "wait-vm-affinity-tag-applied")...).Should(
			BeTrue(), "expected no Tag %q to be created in namespace %q", name, input.WCPNamespaceName)
	}

	// waitForTagOwnerCount waits for the Tag CR derived from key/value to
	// have exactly count owner references.
	waitForTagOwnerCount := func(key, value string, count int) *vspherepolv1alpha1.Tag {
		GinkgoHelper()

		name := affinityTagResourceName(input.WCPNamespaceName, key, value)
		var tag *vspherepolv1alpha1.Tag

		Eventually(func(g Gomega) {
			tag = getTag(name)
			g.Expect(tag).ToNot(BeNil(), "Tag %q should exist", name)
			g.Expect(tag.OwnerReferences).To(HaveLen(count))
		}, config.GetIntervals("default", "wait-vm-affinity-tag-applied")...).Should(
			Succeed(), "timed out waiting for Tag %q to have %d owner(s)", name, count)

		return tag
	}

	// waitForTagDeleted waits for the Tag CR derived from key/value to be
	// removed from the namespace.
	waitForTagDeleted := func(key, value string) {
		GinkgoHelper()

		name := affinityTagResourceName(input.WCPNamespaceName, key, value)
		Eventually(func() bool {
			return getTag(name) == nil
		}, config.GetIntervals("default", "wait-tag-cr-deleted")...).Should(
			BeTrue(), "timed out waiting for Tag %q to be deleted from namespace %q", name, input.WCPNamespaceName)
	}

	// ownerUIDs returns the UIDs of the Tag's owner references.
	ownerUIDs := func(tag *vspherepolv1alpha1.Tag) []string {
		uids := make([]string, len(tag.OwnerReferences))
		for i, ref := range tag.OwnerReferences {
			uids[i] = string(ref.UID)
		}
		return uids
	}

	// patchVM mutates an existing VM's spec via Get/mutate/Update, retrying
	// on conflict.
	patchVM := func(vmName string, mutate func(vm *vmopv1.VirtualMachine)) {
		GinkgoHelper()

		key := ctrlclient.ObjectKey{Namespace: input.WCPNamespaceName, Name: vmName}
		Eventually(func() error {
			vm := &vmopv1.VirtualMachine{}
			if err := svClusterClient.Get(ctx, key, vm); err != nil {
				return err
			}

			mutate(vm)

			return svClusterClient.Update(ctx, vm)
		}, config.GetIntervals("default", "wait-vm-affinity-tag-applied")...).Should(
			Succeed(), "failed to patch VM %q", vmName)
	}

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(ctx, []string{config.GetVariable("VMOPNamespace")}, clusterProxy.GetRESTConfig(), filepath.Join(input.ArtifactFolder, specName))
		DeferCleanup(cancelPodWatches)

		svClusterClient = clusterProxy.GetClient()

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMAffinityDuringExecutionCapabilityName)

		// spec.affinity requires a non-empty spec.groupName, and
		// spec.groupName itself is rejected unless VM Groups is enabled.
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMGroupsCapabilityName)

		skipper.SkipUnlessTaggingAPIFSSEnabled(ctx, svClusterClient, config)

		// Skip entirely when the Tag CRD is not registered on this
		// Supervisor at all. This is a distinct condition from the FSS being
		// off: a Supervisor that never installed the CRD can still report
		// the FSS as enabled.
		if err := svClusterClient.List(ctx, &vspherepolv1alpha1.TagList{}, ctrlclient.InNamespace(input.WCPNamespaceName)); err != nil {
			if apimeta.IsNoMatchError(err) {
				framework.SkipInternalf(1, "skip the test as the Tag CRD is not registered on this Supervisor")
			}
			Expect(err).ToNot(HaveOccurred(), "failed to list Tags")
		}

		clusterResources := config.InfraConfig.ManagementClusterConfig.Resources
		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(clusterResources)
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, svClusterClient, input.WCPNamespaceName, linuxImageDisplayName)

		vmNames = nil

		vmgRootName = fmt.Sprintf("%s-%s-root", specName, capiutil.RandomString(4))
		vmgRootYaml = nil
		vmgMemberNames = nil
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			for _, vmName := range vmNames {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmName, vmKind)
			}
			if len(vmgRootYaml) > 0 {
				vmoperator.DescribeResourceIfExists(ctx, svClusterClient, clusterProxy.GetKubeconfigPath(), input.WCPNamespaceName, vmgRootName, vmgKind)
			}
		}

		for _, vmName := range vmNames {
			By(fmt.Sprintf("Deleting VM %s", vmName))
			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			if err == nil {
				Expect(svClusterClient.Delete(ctx, vm)).To(Succeed(), "failed to delete VM %q", vmName)
				vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			}
		}
		vmNames = nil

		if len(vmgRootYaml) > 0 {
			By(fmt.Sprintf("Deleting the root VirtualMachineGroup %s", vmgRootName))
			Expect(clusterProxy.DeleteWithArgs(ctx, vmgRootYaml)).To(Succeed(), "failed to delete VirtualMachineGroup %q", vmgRootName)
			vmoperator.WaitForVirtualMachineGroupToBeDeleted(ctx, config, svClusterClient, input.WCPNamespaceName, vmgRootName)
		}
	})

	Context("A VM owns the Tag for each label pair its affinity references", func() {
		It("creates a Tag CR with the VM as sole owner when its affinity references its own label",
			Label("core-functional", "extended-functional", "experimental"), func() {
				vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM %s with label %s=blue and affinity referencing %s=blue", vmName, affinityKey, affinityKey))
				createVM(vmName, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))

				By("Waiting for the Tag CR to be created with the derived name, label mirror, and this VM as sole owner")
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				Expect(err).ToNot(HaveOccurred())

				tag := waitForTagOwnerCount(affinityKey, "blue", 1)
				Expect(tag.Spec.Key).To(Equal(affinityKey))
				Expect(tag.Spec.Value).To(Equal("blue"))
				Expect(tag.Labels).To(HaveKeyWithValue(affinityKey, "blue"))
				Expect(tag.OwnerReferences[0].UID).To(Equal(vm.UID))
				Expect(tag.OwnerReferences[0].Name).To(Equal(vmName))
			})

		It("appends a second VM as an owner of the same Tag CR rather than replacing the first",
			Label("core-functional", "extended-functional", "experimental"), func() {
				vm1Name := fmt.Sprintf("%s-%s-1", specName, capiutil.RandomString(6))
				vm2Name := fmt.Sprintf("%s-%s-2", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM1 (%s) with label %s=blue and affinity referencing %s=blue", vm1Name, affinityKey, affinityKey))
				createVM(vm1Name, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))
				waitForTagOwnerCount(affinityKey, "blue", 1)

				By(fmt.Sprintf("Creating VM2 (%s) with the same label and affinity", vm2Name))
				createVM(vm2Name, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))

				By("Verifying exactly one Tag CR exists with both VMs recorded as owners")
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				Expect(err).ToNot(HaveOccurred())
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				Expect(err).ToNot(HaveOccurred())

				tag := waitForTagOwnerCount(affinityKey, "blue", 2)
				Expect(ownerUIDs(tag)).To(ConsistOf(string(vm1.UID), string(vm2.UID)))
			})

		It("creates a Tag CR only for the label pair referenced by affinity, not the unreferenced one",
			Label("core-functional", "extended-functional", "experimental"), func() {
				vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM %s with labels %s=blue and env=prod, affinity referencing only %s=blue", vmName, affinityKey, affinityKey))
				createVM(vmName,
					map[string]string{affinityKey: "blue", "env": "prod"},
					affinityFor(affinityKey, "blue"))

				By("Verifying a Tag CR exists for the referenced pair")
				waitForTagOwnerCount(affinityKey, "blue", 1)

				By("Verifying no Tag CR exists for the unreferenced pair")
				waitForNoTag("env", "prod")
			})
	})

	Context("Tag ownership follows the VM's lifetime", func() {
		It("keeps the Tag CR with the surviving owner when one of two owning VMs is deleted",
			Label("core-functional", "extended-functional", "experimental"), func() {
				vm1Name := fmt.Sprintf("%s-%s-1", specName, capiutil.RandomString(6))
				vm2Name := fmt.Sprintf("%s-%s-2", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM1 (%s) and VM2 (%s), both with label %s=blue and affinity referencing it", vm1Name, vm2Name, affinityKey))
				createVM(vm1Name, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))
				createVM(vm2Name, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))
				waitForTagOwnerCount(affinityKey, "blue", 2)

				By(fmt.Sprintf("Deleting VM1 (%s)", vm1Name))
				deleteVM(vm1Name)

				By("Verifying the Tag CR survives with exactly one owner, VM2")
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				Expect(err).ToNot(HaveOccurred())

				tag := waitForTagOwnerCount(affinityKey, "blue", 1)
				Expect(tag.OwnerReferences[0].UID).To(Equal(vm2.UID))
			})

		It("deletes the Tag CR once its only owning VM is deleted",
			Label("core-functional", "extended-functional", "experimental"), func() {
				vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM %s with label %s=blue and affinity referencing it", vmName, affinityKey))
				createVM(vmName, map[string]string{affinityKey: "blue"}, affinityFor(affinityKey, "blue"))
				waitForTagOwnerCount(affinityKey, "blue", 1)

				By(fmt.Sprintf("Deleting the sole owning VM %s", vmName))
				deleteVM(vmName)

				By("Verifying the Tag CR is deleted from the namespace")
				waitForTagDeleted(affinityKey, "blue")
			})

		It("keeps the Tag CR and the VM's ownership when the VM drops the label its affinity references",
			Label("core-functional", "extended-functional", "experimental"), func() {
				const value = "ownership-survives-label-drop"

				vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(6))

				By(fmt.Sprintf("Creating VM %s with label %s=%s and affinity referencing it", vmName, affinityKey, value))
				createVM(vmName, map[string]string{affinityKey: value}, affinityFor(affinityKey, value))
				tagBefore := waitForTagOwnerCount(affinityKey, value, 1)

				By(fmt.Sprintf("Patching VM %s to drop the label while keeping the (immutable) affinity reference", vmName))
				patchVM(vmName, func(vm *vmopv1.VirtualMachine) {
					delete(vm.Labels, affinityKey)
				})

				By("Verifying the Tag CR survives with the VM still recorded as owner, since its spec.affinity still references the pair")
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				Expect(err).ToNot(HaveOccurred())

				tagAfter := waitForTagOwnerCount(affinityKey, value, 1)
				Expect(tagAfter.UID).To(Equal(tagBefore.UID))
				Expect(tagAfter.OwnerReferences[0].UID).To(Equal(vm.UID))
			})
	})

	Context("Edge cases", func() {
		It("shares one Tag CR across VMs in different VirtualMachineGroups",
			Label("core-functional", "extended-functional", "experimental"), func() {
				const value = "cross-group"

				vm1Name := fmt.Sprintf("%s-%s-group1", specName, capiutil.RandomString(6))
				vm2Name := fmt.Sprintf("%s-%s-group2", specName, capiutil.RandomString(6))
				secondGroupName := fmt.Sprintf("%s-%s-group2", specName, capiutil.RandomString(4))

				By(fmt.Sprintf("Creating VM1 (%s) with label %s=%s and affinity referencing it, as a member of the suite's shared VirtualMachineGroup", vm1Name, affinityKey, value))
				createVM(vm1Name, map[string]string{affinityKey: value}, affinityFor(affinityKey, value))

				By(fmt.Sprintf("Creating a second, unrelated VirtualMachineGroup and VM2 (%s) with the same label and affinity, as its sole member", vm2Name))
				createGroup(secondGroupName, vm2Name)

				vm2Yaml := vmYAML(vm2Name, secondGroupName, map[string]string{affinityKey: value}, affinityFor(affinityKey, value))
				Expect(clusterProxy.CreateWithArgs(ctx, vm2Yaml)).To(Succeed(), "failed to create VM %q:\n %s", vm2Name, string(vm2Yaml))
				deferDeleteVM(vm2Name)
				waitForVMReady(vm2Name)

				By("Verifying one Tag CR with both VMs as owners, despite belonging to different VirtualMachineGroups")
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				Expect(err).ToNot(HaveOccurred())
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				Expect(err).ToNot(HaveOccurred())

				tag := waitForTagOwnerCount(affinityKey, value, 2)
				Expect(ownerUIDs(tag)).To(ConsistOf(string(vm1.UID), string(vm2.UID)))
			})

		It("converges a re-created VM back onto the same Tag CR",
			Label("core-functional", "extended-functional", "experimental"), func() {
				const value = "recreate-cycle"

				recreatedVMName := fmt.Sprintf("%s-%s-recreate", specName, capiutil.RandomString(6))
				survivorVMName := fmt.Sprintf("%s-%s-survivor", specName, capiutil.RandomString(6))
				groupName := fmt.Sprintf("%s-%s-recreate-group", specName, capiutil.RandomString(4))

				labels := map[string]string{affinityKey: value}
				affinity := affinityFor(affinityKey, value)

				// This spec uses its own group instead of the suite's root
				// group, whose member list createVM appends to: re-creating
				// a VM under the same name would otherwise duplicate the
				// member entry.
				By(fmt.Sprintf("Creating a dedicated VirtualMachineGroup naming %s and %s as members", recreatedVMName, survivorVMName))
				createGroup(groupName, recreatedVMName, survivorVMName)

				// The group names both VMs as members while neither exists
				// yet, so both Creates must land before waiting on either's
				// readiness: a VM's reconcile cannot get a group placement
				// result until every declared member exists.
				By(fmt.Sprintf("Creating VM %s and VM %s, both with label %s=%s and affinity referencing it", recreatedVMName, survivorVMName, affinityKey, value))
				for _, vmName := range []string{recreatedVMName, survivorVMName} {
					Expect(clusterProxy.CreateWithArgs(ctx, vmYAML(vmName, groupName, labels, affinity))).To(Succeed(), "failed to create VM %q", vmName)
					deferDeleteVM(vmName)
				}
				waitForVMReady(recreatedVMName)
				waitForVMReady(survivorVMName)

				tagBefore := waitForTagOwnerCount(affinityKey, value, 2)

				By(fmt.Sprintf("Deleting VM %s while the survivor VM %s keeps the Tag CR alive", recreatedVMName, survivorVMName))
				deleteVM(recreatedVMName)

				tagAfterDelete := waitForTagOwnerCount(affinityKey, value, 1)
				Expect(tagAfterDelete.UID).To(Equal(tagBefore.UID), "expected the Tag CR to survive the delete since the survivor VM still owns it")

				By(fmt.Sprintf("Re-creating a new VM named %s with the same label and affinity", recreatedVMName))
				Expect(clusterProxy.CreateWithArgs(ctx, vmYAML(recreatedVMName, groupName, labels, affinity))).To(Succeed(), "failed to re-create VM %q", recreatedVMName)
				waitForVMReady(recreatedVMName)

				By("Verifying the re-created VM converges back onto the same Tag CR as a second owner")
				recreatedVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, recreatedVMName)
				Expect(err).ToNot(HaveOccurred())
				survivorVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, survivorVMName)
				Expect(err).ToNot(HaveOccurred())

				tagAfterRecreate := waitForTagOwnerCount(affinityKey, value, 2)
				Expect(tagAfterRecreate.UID).To(Equal(tagBefore.UID), "expected the same Tag CR object to persist through the delete/re-create cycle")
				Expect(ownerUIDs(tagAfterRecreate)).To(ConsistOf(string(survivorVM.UID), string(recreatedVM.UID)))
			})

		It("converges two VMs created at once on a single Tag CR",
			Label("core-functional", "extended-functional", "experimental"), func() {
				const value = "concurrent-create"

				vm1Name := fmt.Sprintf("%s-%s-concurrent1", specName, capiutil.RandomString(6))
				vm2Name := fmt.Sprintf("%s-%s-concurrent2", specName, capiutil.RandomString(6))
				groupName := fmt.Sprintf("%s-%s-concurrent-group", specName, capiutil.RandomString(4))

				labels := map[string]string{affinityKey: value}
				affinity := affinityFor(affinityKey, value)

				// createVM waits for each VM to become ready before
				// returning, which would serialize the two creates and leave
				// the second VM adopting a Tag the first had long since
				// committed. Creating both back to back lets their first
				// reconciles overlap, so both race to create the Tag for the
				// same pair and the loser has to adopt the winner's resource.
				By(fmt.Sprintf("Creating a dedicated VirtualMachineGroup naming %s and %s as members", vm1Name, vm2Name))
				createGroup(groupName, vm1Name, vm2Name)

				By(fmt.Sprintf("Creating VM %s and VM %s back to back, both with label %s=%s and affinity referencing it", vm1Name, vm2Name, affinityKey, value))
				for _, vmName := range []string{vm1Name, vm2Name} {
					Expect(clusterProxy.CreateWithArgs(ctx, vmYAML(vmName, groupName, labels, affinity))).To(Succeed(), "failed to create VM %q", vmName)
					deferDeleteVM(vmName)
				}
				waitForVMReady(vm1Name)
				waitForVMReady(vm2Name)

				By("Verifying one Tag CR carries both VMs as owners rather than one create failing or overwriting the other")
				vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				Expect(err).ToNot(HaveOccurred())
				vm2, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)
				Expect(err).ToNot(HaveOccurred())

				tag := waitForTagOwnerCount(affinityKey, value, 2)
				Expect(tag.Spec.Key).To(Equal(affinityKey))
				Expect(tag.Spec.Value).To(Equal(value))
				Expect(ownerUIDs(tag)).To(ConsistOf(string(vm1.UID), string(vm2.UID)))
			})
	})
}
