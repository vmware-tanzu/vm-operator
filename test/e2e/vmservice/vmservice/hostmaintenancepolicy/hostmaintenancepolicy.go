// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package hostmaintenancepolicy contains E2E tests for the
// AutomaticHostEvacuationPolicy and BestEffortRestartPolicy CRDs introduced
// for host-maintenance-mode infra policies.
package hostmaintenancepolicy

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
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

// SpecInput holds the inputs for Spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}

// Spec verifies that a Mandatory AutomaticHostEvacuationPolicy tags a
// matching VM and that the policy appears in the VM's status.policies, per
// .sdd/specs/007-host-maintenance-mode-policy/plan.md's Test strategy item
// 1, plus that an already-created, non-matching VM picks up the policy once
// its match is widened — the latter specifically exercises the
// computePolicyToPolicyEvaluationMapperFn watch/informer path, which a
// fake-client unit test cannot validate. AutomaticHostEvacuationPolicy has
// no WCP admin API of its own yet (unlike ComputePolicy, whose CRs are
// mirrored from a legacy InfraPolicy admin API — see virtualmachinelcm.go),
// so this spec creates the CRD directly, matching spec.md's "CSP admin
// applies AutomaticHostEvacuationPolicy" framing.
// createTagPolicy/createAutomaticHostEvacuationPolicy are the single seams
// to swap for a future WCP admin API call, if one is added.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "host-maintenance-mode-policy"

	var (
		input           SpecInput
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		adminClient     ctrlclient.Client
		vCenterClient   *vim25.Client
		tagManager      *tags.Manager

		vmName     string
		vmYaml     []byte
		matchLabel map[string]string

		tagPolicy           *vspherepolv1.TagPolicy
		evacuationPolicy    *vspherepolv1.AutomaticHostEvacuationPolicy
		restartPolicy       *vspherepolv1.BestEffortRestartPolicy
		policyNameToVMTagID map[string]string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(),
			"Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMEvacuationCapabilityName)

		adminProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to get admin cluster proxy")
		DeferCleanup(func() { adminProxy.Dispose(ctx) })

		adminClient, err = adminProxy.GetAdminClient()
		Expect(err).ToNot(HaveOccurred(), "failed to get admin client")

		vCenterClient = vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
		DeferCleanup(func() { vcenter.LogoutVimClient(vCenterClient) })

		restClient, err := vcenter.NewRestClient(ctx, vCenterClient, testbed.AdminUsername, testbed.AdminPassword)
		Expect(err).ToNot(HaveOccurred(), "failed to create rest client")
		tagManager = tags.NewManager(restClient)

		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		matchLabel = map[string]string{
			"vmoperator.vmware.com/e2e-host-evacuation-test": capiutil.RandomString(6),
		}
		vmYaml = nil
		tagPolicy = nil
		evacuationPolicy = nil
		restartPolicy = nil
	})

	AfterEach(func() {
		if evacuationPolicy != nil {
			_ = adminClient.Delete(ctx, evacuationPolicy)
		}
		if restartPolicy != nil {
			_ = adminClient.Delete(ctx, restartPolicy)
		}
		if tagPolicy != nil {
			_ = adminClient.Delete(ctx, tagPolicy)
		}
		if len(vmYaml) > 0 {
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface the policy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "host-evac")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory AutomaticHostEvacuationPolicy matching the test label")
			evacuationPolicy = createAutomaticHostEvacuationPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			policyNameToVMTagID = map[string]string{
				evacuationPolicy.Name: tagID,
			}

			By("Creating a VM matching the policy's label selector")
			vmYaml = createMatchingVM(ctx, input, svClusterClient, clusterProxy, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and the real vSphere tag assignment")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				policyNameToVMTagID,
				[]string{evacuationPolicy.Name})
		})

	It("Should re-evaluate an already-created VM when a policy's match is widened",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "host-evac-widen")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-widen-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory AutomaticHostEvacuationPolicy that does not yet match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-host-evacuation-test": capiutil.RandomString(6),
			}
			evacuationPolicy = createAutomaticHostEvacuationPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-widen-policy-%s", capiutil.RandomString(4)), nonMatchingLabel, []string{tagPolicy.Name})

			By("Creating a VM that does not match the policy yet")
			vmYaml = createMatchingVM(ctx, input, svClusterClient, clusterProxy, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM does not have the policy applied yet")
			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
			Expect(vm.Status.Policies).To(BeEmpty(),
				"VM should not have any policies applied before the policy's match is widened")

			By("Widening the policy's match to the VM's actual label, without touching the VM")
			evacuationPolicyPatch := evacuationPolicy.DeepCopy()
			evacuationPolicyPatch.Spec.Match = &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			}
			Expect(adminClient.Patch(ctx, evacuationPolicyPatch, ctrlclient.MergeFrom(evacuationPolicy))).
				To(Succeed(), "failed to widen AutomaticHostEvacuationPolicy %q match", evacuationPolicy.Name)

			By("Verifying the already-created VM picks up the widened policy via the AutomaticHostEvacuationPolicy watch")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{evacuationPolicy.Name: tagID},
				[]string{evacuationPolicy.Name})
		})

	It("Should tag a VM that explicitly references a matching Optional BestEffortRestartPolicy, "+
		"and error when the reference does not match",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "host-restart")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-restart-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating an Optional BestEffortRestartPolicy matching the test label")
			restartPolicy = createBestEffortRestartPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-restart-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			By("Creating a VM matching the policy's label selector, explicitly referencing the policy")
			vmYaml = createVMWithExplicitPolicy(ctx, input, svClusterClient, clusterProxy, vmName, matchLabel, restartPolicy.Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and the real vSphere tag assignment")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{restartPolicy.Name: tagID},
				[]string{restartPolicy.Name})

			By("Deleting the VM before re-creating it with a non-matching explicit reference")
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
			vmYaml = nil

			By("Creating a VM that does not match the policy's label, but still explicitly references it")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-host-evacuation-test": capiutil.RandomString(6),
			}
			vmYaml = createVMWithExplicitPolicy(
				ctx, input, svClusterClient, clusterProxy, vmName, nonMatchingLabel, restartPolicy.Name)

			By("Verifying an error is surfaced for the non-matching explicit reference")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
				metav1.Condition{
					Type:   vmopv1.VirtualMachineConditionPlacementReady,
					Status: metav1.ConditionFalse,
					Reason: "NotReady",
				})

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
			cond := apimeta.FindStatusCondition(
				vm.GetConditions(), vmopv1.VirtualMachineConditionPlacementReady)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Message).To(ContainSubstring(
				fmt.Sprintf("%s\" does not match", restartPolicy.Name)))
		})

	It("Should surface both a Mandatory AutomaticHostEvacuationPolicy and an Optional "+
		"BestEffortRestartPolicy in status.policies when a VM matches both",
		Label("core-functional", "experimental"),
		func() {
			evacuationTagID := createVSphereTag(input.WCPClient, tagManager, "host-evac-mixed")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "host-restart-mixed")

			By("Creating TagPolicies referencing the real vSphere tags")
			evacuationTagPolicy := createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-mixed-tag-policy-%s", capiutil.RandomString(4)), []string{evacuationTagID})
			DeferCleanup(func(cleanupCtx context.Context) { _ = adminClient.Delete(cleanupCtx, evacuationTagPolicy) })

			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-restart-mixed-tag-policy-%s", capiutil.RandomString(4)), []string{restartTagID})

			By("Creating a Mandatory AutomaticHostEvacuationPolicy matching the test label")
			evacuationPolicy = createAutomaticHostEvacuationPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-evac-mixed-policy-%s", capiutil.RandomString(4)), matchLabel, []string{evacuationTagPolicy.Name})

			By("Creating an Optional BestEffortRestartPolicy also matching the test label")
			restartPolicy = createBestEffortRestartPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-restart-mixed-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			By("Creating a VM matching both policies' label selector, explicitly referencing the Optional one")
			vmYaml = createVMWithExplicitPolicy(
				ctx, input, svClusterClient, clusterProxy, vmName, matchLabel, restartPolicy.Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying both policies are surfaced in status.policies with their respective tags")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{
					evacuationPolicy.Name: evacuationTagID,
					restartPolicy.Name:    restartTagID,
				},
				[]string{evacuationPolicy.Name, restartPolicy.Name})
		})

	// TODO(vmop-4104): VC level AutomaticHostEvacuationPolicy is not yet available, add a test, once it is available.
	// This test mimics the infraInMaintenance condition though vm_host_affinity ComputePolicy (see
	// pinVMToHost). 
	It("Should surface VirtualMachinePowerStateSynced=False with reason InfraInMaintenance while the VM's "+
		"host is in maintenance mode, and revert once the host exits maintenance mode",
		Label("core-functional", "experimental"),
		func() {
			By("Creating an Optional BestEffortRestartPolicy matching the test label")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "host-maint-restart")
			restartTagPolicy := createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-maint-restart-tag-policy-%s", capiutil.RandomString(4)), []string{restartTagID})
			DeferCleanup(func(cleanupCtx context.Context) { _ = adminClient.Delete(cleanupCtx, restartTagPolicy) })
			restartPolicy = createBestEffortRestartPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("host-maint-restart-policy-%s", capiutil.RandomString(4)), matchLabel, []string{restartTagPolicy.Name})

			// BestEffortRestartPolicy is Optional, so it is only applied to a
			// VM that explicitly references it in spec.policies -- unlike a
			// Mandatory policy, matching labels alone is not enough (see the
			// "explicit reference" It above).
			By("Creating a powered-on VM that explicitly references the BestEffortRestartPolicy")
			vmYaml = createVMWithExplicitPolicy(ctx, input, svClusterClient, clusterProxy, vmName, matchLabel, restartPolicy.Name)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})

			By("Verifying the VM's status.policies includes the BestEffortRestartPolicy")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
				g.Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")

				var found bool
				for _, policy := range vm.Status.Policies {
					if policy.Name == restartPolicy.Name {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "BestEffortRestartPolicy should appear in VM's status.policies")
			}, input.Config.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vmName)

			By("Pinning the VM to its current host so DRS cannot evacuate it during maintenance")
			pinVMToHost(ctx, input.WCPClient, tagManager, svClusterClient, input.Config,
				input.WCPNamespaceName, vmName, hostMoRef, matchLabel, "host-maint")

			By("Putting the VM's host into maintenance mode")
			enterTask := enterHostMaintenanceMode(ctx, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, vCenterClient, hostMoRef, enterTask)
			})

			By("Verifying VirtualMachinePowerStateSynced becomes False with reason InfraInMaintenance")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
				metav1.Condition{
					Type:   vmopv1.VirtualMachinePowerStateSynced,
					Status: metav1.ConditionFalse,
					Reason: "InfraInMaintenance",
				})

			By("Taking the VM's host out of maintenance mode")
			exitHostMaintenanceMode(ctx, vCenterClient, hostMoRef, enterTask)
			enterTask = nil

			By("Verifying VirtualMachinePowerStateSynced reverts to True once the VM is powered back on")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
		})
}

// createVSphereTag creates a real vSphere tag category and tag, registering
// their deletion on cleanup, and returns the tag's ID for use in a
// TagPolicy.
func createVSphereTag(wcpClient wcp.WorkloadManagementAPI, tagManager *tags.Manager, prefix string) string {
	GinkgoHelper()

	By("Creating a real vSphere tag to associate with the policy")
	tagCategoryName := fmt.Sprintf("%s-category-%s", prefix, capiutil.RandomString(4))
	tagCategoryID, err := wcpClient.CreateTagCategory(tagCategoryName, "e2e host evacuation policy test")
	Expect(err).ToNot(HaveOccurred(), "failed to create tag category")
	Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")

	tagName := fmt.Sprintf("%s-tag-%s", prefix, capiutil.RandomString(4))
	tagID, err := wcpClient.CreateTag(tagName, "e2e host evacuation policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create tag")
	Expect(tagID).NotTo(BeEmpty(), "tag ID should be returned")

	DeferCleanup(func(cleanupCtx context.Context) {
		_ = tagManager.DeleteTag(cleanupCtx, &tags.Tag{ID: tagID})
		_ = tagManager.DeleteCategory(cleanupCtx, &tags.Category{ID: tagCategoryID})
	})

	return tagID
}

// createMatchingVM creates a VM in the given namespace with the given
// labels, returning the rendered YAML so the caller can register it for
// AfterEach cleanup.
func createMatchingVM(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	clusterProxy *common.VMServiceClusterProxy,
	vmName string,
	labels map[string]string) []byte {

	GinkgoHelper()

	clusterResources := input.Config.InfraConfig.ManagementClusterConfig.Resources
	imageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
	imageName := vmoperator.WaitForVirtualMachineImageName(
		ctx, &input.Config.Config, svClusterClient, input.WCPNamespaceName, imageDisplayName)

	vmParameters := manifestbuilders.VirtualMachineYaml{
		Namespace:        input.WCPNamespaceName,
		Name:             vmName,
		Labels:           labels,
		ImageName:        imageName,
		VMClassName:      clusterResources.VMClassName,
		StorageClassName: clusterResources.StorageClassName,
		ResourcePolicy:   clusterResources.VMResourcePolicyName,
		PowerState:       "PoweredOn",
	}
	vmYaml := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
	Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

	return vmYaml
}

// matchLabelSelector converts a label map into the equality LabelSelectorRequirements
// used by MatchWorkloadSpec.Labels.
func matchLabelSelector(labels map[string]string) []metav1.LabelSelectorRequirement {
	matchExpressions := make([]metav1.LabelSelectorRequirement, 0, len(labels))
	for k, v := range labels {
		matchExpressions = append(matchExpressions, metav1.LabelSelectorRequirement{
			Key:      k,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{v},
		})
	}

	return matchExpressions
}

// createTagPolicy creates a TagPolicy CR directly via an admin client.
//
// TODO(vmop-4104): replace with a call into a WCP admin API once one exists
// for this CRD, mirroring how wcp.WorkloadManagementAPI.CreateInfraPolicy
// mirrors ComputePolicy CRs from a legacy admin API today.
func createTagPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	namespace, name string,
	tagIDs []string) *vspherepolv1.TagPolicy {

	GinkgoHelper()

	obj := &vspherepolv1.TagPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vspherepolv1.TagPolicySpec{
			Tags: tagIDs,
		},
	}

	Expect(adminClient.Create(ctx, obj)).To(Succeed(), "failed to create TagPolicy %q", name)

	return obj
}

// createAutomaticHostEvacuationPolicy creates a Mandatory
// AutomaticHostEvacuationPolicy CR directly via an admin client, matching
// on the given workload labels.
//
// TODO(vmop-4104): replace with a call into a WCP admin API once one exists
// for this CRD, mirroring how wcp.WorkloadManagementAPI.CreateInfraPolicy
// mirrors ComputePolicy CRs from a legacy admin API today.
func createAutomaticHostEvacuationPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	namespace, name string,
	matchLabel map[string]string,
	tagPolicyNames []string) *vspherepolv1.AutomaticHostEvacuationPolicy {

	GinkgoHelper()

	obj := &vspherepolv1.AutomaticHostEvacuationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vspherepolv1.AutomaticHostEvacuationPolicySpec{
			EnforcementMode: vspherepolv1.PolicyEnforcementModeMandatory,
			Match: &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			},
			Tags: tagPolicyNames,
		},
	}

	Expect(adminClient.Create(ctx, obj)).To(Succeed(), "failed to create AutomaticHostEvacuationPolicy %q", name)

	return obj
}

// createBestEffortRestartPolicy creates an Optional BestEffortRestartPolicy
// CR directly via an admin client, matching on the given workload labels.
//
// TODO(vmop-4104): replace with a call into a WCP admin API once one exists
// for this CRD, mirroring how wcp.WorkloadManagementAPI.CreateInfraPolicy
// mirrors ComputePolicy CRs from a legacy admin API today.
func createBestEffortRestartPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	namespace, name string,
	matchLabel map[string]string,
	tagPolicyNames []string) *vspherepolv1.BestEffortRestartPolicy {

	GinkgoHelper()

	obj := &vspherepolv1.BestEffortRestartPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vspherepolv1.BestEffortRestartPolicySpec{
			EnforcementMode: vspherepolv1.PolicyEnforcementModeOptional,
			Match: &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			},
			Tags: tagPolicyNames,
		},
	}

	Expect(adminClient.Create(ctx, obj)).To(Succeed(), "failed to create BestEffortRestartPolicy %q", name)

	return obj
}

// createVMWithExplicitPolicy creates a VM in the given namespace with the
// given labels, explicitly referencing a BestEffortRestartPolicy by name in
// spec.policies. Returns the rendered YAML so the caller can register it for
// AfterEach cleanup.
func createVMWithExplicitPolicy(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	clusterProxy *common.VMServiceClusterProxy,
	vmName string,
	labels map[string]string,
	restartPolicyName string) []byte {

	GinkgoHelper()

	clusterResources := input.Config.InfraConfig.ManagementClusterConfig.Resources
	imageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
	imageName := vmoperator.WaitForVirtualMachineImageName(
		ctx, &input.Config.Config, svClusterClient, input.WCPNamespaceName, imageDisplayName)

	vmParameters := manifestbuilders.VirtualMachineYaml{
		Namespace:        input.WCPNamespaceName,
		Name:             vmName,
		Labels:           labels,
		ImageName:        imageName,
		VMClassName:      clusterResources.VMClassName,
		StorageClassName: clusterResources.StorageClassName,
		ResourcePolicy:   clusterResources.VMResourcePolicyName,
		PowerState:       "PoweredOn",
		Policies: []vmopv1.PolicySpec{
			{
				APIVersion: vspherepolv1.GroupVersion.String(),
				Kind:       "BestEffortRestartPolicy",
				Name:       restartPolicyName,
			},
		},
	}
	vmYaml := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
	Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create virtualmachine:\n %s", string(vmYaml))

	return vmYaml
}

// getVMHostMoRef returns the ManagedObjectReference of the ESX host
// currently running the named VM.
func getVMHostMoRef(
	ctx context.Context,
	vCenterClient *vim25.Client,
	svClusterClient ctrlclient.Client,
	namespace, vmName string) vimtypes.ManagedObjectReference {

	GinkgoHelper()

	vm, err := utils.GetVirtualMachine(ctx, svClusterClient, namespace, vmName)
	Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")

	vmMoRef := vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}

	var vmMO mo.VirtualMachine
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"runtime.host"}, &vmMO)).To(Succeed())
	Expect(vmMO.Runtime.Host).ToNot(BeNil(), "VM %q has no host in its runtime info", vmName)

	return *vmMO.Runtime.Host
}

// pinVMToHost creates a real, mandatory vm_host_affinity ComputePolicy that
// tags the given host and matching VMs, forcing DRS to keep vmName on
// hostMoRef instead of relocating it. It waits for the resulting
// PolicyEvaluation to report the VM as compliant, so callers can be sure the
// real vSphere tag -- and therefore DRS's placement constraint -- is in
// effect before relying on it.
func pinVMToHost(
	ctx context.Context,
	wcpClient wcp.WorkloadManagementAPI,
	tagManager *tags.Manager,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	namespace, vmName string,
	hostMoRef vimtypes.ManagedObjectReference,
	matchLabel map[string]string,
	prefix string) {

	GinkgoHelper()

	By("Creating a real vSphere host/VM tag pair for a VM/Host affinity ComputePolicy")
	tagCategoryName := fmt.Sprintf("%s-category-%s", prefix, capiutil.RandomString(4))
	tagCategoryID, err := wcpClient.CreateTagCategory(tagCategoryName, "e2e host maintenance policy test")
	Expect(err).ToNot(HaveOccurred(), "failed to create tag category")
	Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")

	hostTagID, err := wcpClient.CreateTag(
		fmt.Sprintf("%s-host-tag-%s", prefix, capiutil.RandomString(4)), "e2e host maintenance policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create host tag")
	Expect(hostTagID).NotTo(BeEmpty(), "host tag ID should be returned")

	vmTagID, err := wcpClient.CreateTag(
		fmt.Sprintf("%s-vm-tag-%s", prefix, capiutil.RandomString(4)), "e2e host maintenance policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create VM tag")
	Expect(vmTagID).NotTo(BeEmpty(), "VM tag ID should be returned")

	DeferCleanup(func(cleanupCtx context.Context) {
		_ = tagManager.DeleteTag(cleanupCtx, &tags.Tag{ID: hostTagID})
		_ = tagManager.DeleteTag(cleanupCtx, &tags.Tag{ID: vmTagID})
		_ = tagManager.DeleteCategory(cleanupCtx, &tags.Category{ID: tagCategoryID})
	})

	By("Assigning the host tag to the VM's current host")
	Expect(wcpClient.AssignTagsToHost([]string{hostTagID}, hostMoRef.Value)).
		To(Succeed(), "failed to assign tag to host %q", hostMoRef.Value)

	By("Creating a Mandatory VM/Host affinity ComputePolicy and InfraPolicy pinning the VM to its host")
	computePolicyID, err := wcpClient.CreateComputePolicy(wcp.ComputePolicySpec{
		Name:        fmt.Sprintf("%s-compute-policy-%s", prefix, capiutil.RandomString(4)),
		Description: "pin VM to its host for e2e host maintenance policy test",
		HostTagID:   hostTagID,
		VMTagID:     vmTagID,
		Capability:  wcp.ComputePolicyCapabilityVMHostAffinity,
	})
	Expect(err).ToNot(HaveOccurred(), "failed to create compute policy")
	Expect(computePolicyID).NotTo(BeEmpty(), "compute policy ID should be returned")

	infraPolicyName := fmt.Sprintf("%s-infra-policy-%s", prefix, capiutil.RandomString(4))
	Expect(wcpClient.CreateInfraPolicy(wcp.InfraPolicySpec{
		Name:               infraPolicyName,
		Description:        "pin VM to its host for e2e host maintenance policy test",
		ComputePolicyID:    computePolicyID,
		EnforcementMode:    wcp.InfraPolicyEnforcementModeMandatory,
		MatchWorkloadLabel: matchLabel,
	})).To(Succeed(), "failed to create infra policy")

	Expect(wcpClient.UpdateNamespaceWithInfraPolicies(namespace, infraPolicyName)).
		To(Succeed(), "failed to assign infra policy to namespace")

	By("Waiting for the VM to be tagged compliant with the host affinity policy")
	policyEvaluationName := fmt.Sprintf("vm-%s", vmName)
	Eventually(func(g Gomega) {
		var policyEvaluation vspherepolv1.PolicyEvaluation
		g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: policyEvaluationName}, &policyEvaluation)).
			To(Succeed(), "PolicyEvaluation object should exist")

		var found bool
		for _, policy := range policyEvaluation.Status.Policies {
			if strings.Contains(policy.Name, infraPolicyName) {
				found = true
				g.Expect(policy.Tags).To(ContainElement(vmTagID))
			}
		}
		g.Expect(found).To(BeTrue(), "host affinity policy should appear in PolicyEvaluation")

		cond := apimeta.FindStatusCondition(policyEvaluation.Status.Conditions, vspherepolv1.ReadyConditionType)
		g.Expect(cond).NotTo(BeNil(), "Ready condition should be present")
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue), "PolicyEvaluation should be compliant")
	}, config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())
}

// enterHostMaintenanceMode starts putting the given host into maintenance
// mode and returns the task once it has been submitted, without waiting for
// it to complete. On real vCenter, a powered-on VM must be evacuated before
// the host fully enters maintenance mode, so waiting here could hang
// indefinitely if evacuation cannot proceed (e.g. no DRS/vMotion capacity).
// The VM Operator side only needs the task to be in progress to observe the
// transitioning InfraInMaintenance state. It is a no-op (returning nil) if
// the host is already in maintenance mode. Callers must cancel the returned
// task (if non-nil) before attempting to exit maintenance mode, since the
// task may still be queued/running when it does so.
func enterHostMaintenanceMode(ctx context.Context, vCenterClient *vim25.Client, hostMoRef vimtypes.ManagedObjectReference) *object.Task {
	GinkgoHelper()

	if isHostInMaintenanceMode(ctx, vCenterClient, hostMoRef) {
		return nil
	}

	task, err := object.NewHostSystem(vCenterClient, hostMoRef).EnterMaintenanceMode(ctx, 0, false, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to start EnterMaintenanceMode task for host %q", hostMoRef.Value)
	return task
}

// exitHostMaintenanceMode takes the given host out of maintenance mode,
// waiting for the task to complete. If enterTask is non-nil, it is cancelled
// first (best-effort) in case it is still queued/running from a prior,
// non-waited call to enterHostMaintenanceMode. It is a no-op if the host is
// not in maintenance mode.
func exitHostMaintenanceMode(ctx context.Context, vCenterClient *vim25.Client, hostMoRef vimtypes.ManagedObjectReference, enterTask *object.Task) {
	GinkgoHelper()

	if enterTask != nil {
		_ = enterTask.Cancel(ctx)
	}

	if !isHostInMaintenanceMode(ctx, vCenterClient, hostMoRef) {
		return
	}

	task, err := object.NewHostSystem(vCenterClient, hostMoRef).ExitMaintenanceMode(ctx, 0)
	Expect(err).ToNot(HaveOccurred(), "failed to start ExitMaintenanceMode task for host %q", hostMoRef.Value)
	Expect(task.Wait(ctx)).To(Succeed(), "ExitMaintenanceMode task failed for host %q", hostMoRef.Value)
}

func isHostInMaintenanceMode(ctx context.Context, vCenterClient *vim25.Client, hostMoRef vimtypes.ManagedObjectReference) bool {
	GinkgoHelper()

	var hostMO mo.HostSystem
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, hostMoRef, []string{"runtime.inMaintenanceMode"}, &hostMO)).To(Succeed())

	return hostMO.Runtime.InMaintenanceMode
}
