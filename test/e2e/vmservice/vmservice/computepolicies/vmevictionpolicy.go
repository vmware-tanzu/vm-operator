// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file covers
// AutomaticVMEvictionPolicy, introduced for the VM eviction compute
// policies (BestEffortRestartPolicy coverage lands with Phase 4).
package computepolicies

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
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

// SpecInput holds the inputs for Spec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2eConfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}

// Spec verifies that a Mandatory AutomaticVMEvictionPolicy tags a
// matching VM and that the policy appears in the VM's status.policies, per
// .sdd/specs/007-vm-eviction-policy/plan.md's Test strategy item
// 1, plus that an already-created, non-matching VM picks up the policy once
// its match is widened — the latter specifically exercises the
// policyToPolicyEvaluationMapperFn watch/informer path, which a
// fake-client unit test cannot validate. AutomaticVMEvictionPolicy has
// no WCP admin API of its own yet (unlike ComputePolicy, whose CRs are
// mirrored from a legacy InfraPolicy admin API — see virtualmachinelcm.go),
// so this spec creates the CRD directly, matching spec.md's "CSP admin
// applies AutomaticVMEvictionPolicy" framing.
// createTagPolicy/createAutomaticVMEvictionPolicy are the single seams
// to swap for a future WCP admin API call, if one is added.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "vm-eviction-policy"

	var (
		input           SpecInput
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		adminClient     ctrlclient.Client
		vCenterClient   *vim25.Client
		tagManager      *tags.Manager

		vmName     string
		vm         *vmopv1.VirtualMachine
		matchLabel map[string]string

		tagPolicy           *vspherepolv1.TagPolicy
		evacuationPolicy    *vspherepolv1.AutomaticVMEvictionPolicy
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

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMEvictionCapabilityName)

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
			"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
		}
		vm = nil
		tagPolicy = nil
		evacuationPolicy = nil
	})

	AfterEach(func() {
		if evacuationPolicy != nil {
			_ = adminClient.Delete(ctx, evacuationPolicy)
		}
		if tagPolicy != nil {
			_ = adminClient.Delete(ctx, tagPolicy)
		}
		if vm != nil {
			vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface the policy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy = createAutomaticVMEvictionPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			policyNameToVMTagID = map[string]string{
				evacuationPolicy.Name: tagID,
			}

			By("Creating a VM matching the policy's label selector")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
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
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-widen")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-widen-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory AutomaticVMEvictionPolicy that does not yet match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
			}
			evacuationPolicy = createAutomaticVMEvictionPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-widen-policy-%s", capiutil.RandomString(4)), nonMatchingLabel, []string{tagPolicy.Name})

			By("Creating a VM that does not match the policy yet")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM does not have the policy applied yet")
			curVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
			Expect(curVM.Status.Policies).To(BeEmpty(),
				"VM should not have any policies applied before the policy's match is widened")

			By("Widening the policy's match to the VM's actual label, without touching the VM")
			evacuationPolicyPatch := evacuationPolicy.DeepCopy()
			evacuationPolicyPatch.Spec.Match = &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			}
			Expect(adminClient.Patch(ctx, evacuationPolicyPatch, ctrlclient.MergeFrom(evacuationPolicy))).
				To(Succeed(), "failed to widen AutomaticVMEvictionPolicy %q match", evacuationPolicy.Name)

			By("Verifying the already-created VM picks up the widened policy via the AutomaticVMEvictionPolicy watch")
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

	It("Should update the VM's vSphere tag when the policy's Tags are changed",
		Label("core-functional", "experimental"),
		func() {
			tagID1 := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-update-1")

			By("Creating a TagPolicy referencing the first real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-update-tag-policy-1-%s", capiutil.RandomString(4)), []string{tagID1})

			By("Creating a Mandatory AutomaticVMEvictionPolicy referencing the first TagPolicy")
			evacuationPolicy = createAutomaticVMEvictionPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-update-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			By("Creating a VM matching the policy's label selector")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM has the first tag assigned")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{evacuationPolicy.Name: tagID1},
				[]string{evacuationPolicy.Name})

			tagID2 := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-update-2")

			By("Creating a second TagPolicy referencing a second real vSphere tag")
			tagPolicy2 := createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-update-tag-policy-2-%s", capiutil.RandomString(4)), []string{tagID2})
			DeferCleanup(func() { _ = adminClient.Delete(ctx, tagPolicy2) })

			By("Updating the policy's Tags to reference the second TagPolicy instead of the first")
			evacuationPolicyPatch := evacuationPolicy.DeepCopy()
			evacuationPolicyPatch.Spec.Tags = []string{tagPolicy2.Name}
			Expect(adminClient.Patch(ctx, evacuationPolicyPatch, ctrlclient.MergeFrom(evacuationPolicy))).
				To(Succeed(), "failed to update AutomaticVMEvictionPolicy %q tags", evacuationPolicy.Name)
			evacuationPolicy = evacuationPolicyPatch

			By("Verifying the VM now has only the second tag assigned")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{evacuationPolicy.Name: tagID2},
				[]string{evacuationPolicy.Name})
		})

	It("Should remove the VM's vSphere tag and status.policies entry when the policy is deleted",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-delete")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-delete-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy = createAutomaticVMEvictionPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-delete-policy-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			By("Creating a VM matching the policy's label selector")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM has the tag and policy assigned before deletion")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{evacuationPolicy.Name: tagID},
				[]string{evacuationPolicy.Name})

			By("Deleting the AutomaticVMEvictionPolicy")
			Expect(adminClient.Delete(ctx, evacuationPolicy)).
				To(Succeed(), "failed to delete AutomaticVMEvictionPolicy %q", evacuationPolicy.Name)
			evacuationPolicy = nil

			By("Verifying the VM's tag and status.policies entry are removed")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				nil,
				nil)
		})
}

// createVSphereTag creates a real vSphere tag category and tag, registering
// their deletion on cleanup, and returns the tag's ID for use in a
// TagPolicy.
func createVSphereTag(wcpClient wcp.WorkloadManagementAPI, tagManager *tags.Manager, prefix string) string {
	GinkgoHelper()

	By("Creating a real vSphere tag to associate with the policy")
	tagCategoryName := fmt.Sprintf("%s-category-%s", prefix, capiutil.RandomString(4))
	tagCategoryID, err := wcpClient.CreateTagCategory(tagCategoryName, "e2e VM eviction policy test")
	Expect(err).ToNot(HaveOccurred(), "failed to create tag category")
	Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")

	tagName := fmt.Sprintf("%s-tag-%s", prefix, capiutil.RandomString(4))
	tagID, err := wcpClient.CreateTag(tagName, "e2e VM eviction policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create tag")
	Expect(tagID).NotTo(BeEmpty(), "tag ID should be returned")

	DeferCleanup(func(cleanupCtx context.Context) {
		_ = tagManager.DeleteTag(cleanupCtx, &tags.Tag{ID: tagID})
		_ = tagManager.DeleteCategory(cleanupCtx, &tags.Category{ID: tagCategoryID})
	})

	return tagID
}

// createMatchingVM creates a VM in the given namespace with the given
// labels, returning the created object so the caller can register it for
// AfterEach cleanup.
func createMatchingVM(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName string,
	labels map[string]string) *vmopv1.VirtualMachine {

	GinkgoHelper()

	clusterResources := input.Config.InfraConfig.ManagementClusterConfig.Resources
	imageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
	imageName := vmoperator.WaitForVirtualMachineImageName(
		ctx, &input.Config.Config, svClusterClient, input.WCPNamespaceName, imageDisplayName)

	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: input.WCPNamespaceName,
			Labels:    labels,
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:    imageName,
			ClassName:    clusterResources.VMClassName,
			StorageClass: clusterResources.StorageClassName,
			Reserved: &vmopv1.VirtualMachineReservedSpec{
				ResourcePolicyName: clusterResources.VMResourcePolicyName,
			},
			PowerState: vmopv1.VirtualMachinePowerStateOn,
		},
	}
	Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create virtualmachine %q", vmName)

	return vm
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

// createAutomaticVMEvictionPolicy creates a Mandatory
// AutomaticVMEvictionPolicy CR directly via an admin client, matching
// on the given workload labels.
//
// TODO(vmop-4104): replace with a call into a WCP admin API once one exists
// for this CRD, mirroring how wcp.WorkloadManagementAPI.CreateInfraPolicy
// mirrors ComputePolicy CRs from a legacy admin API today.
func createAutomaticVMEvictionPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	namespace, name string,
	matchLabel map[string]string,
	tagPolicyNames []string) *vspherepolv1.AutomaticVMEvictionPolicy {

	GinkgoHelper()

	obj := &vspherepolv1.AutomaticVMEvictionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vspherepolv1.AutomaticVMEvictionPolicySpec{
			// PolicyID is required by the schema but not otherwise used by
			// this test: nothing here drives an actual vCenter compute
			// policy, so any non-empty value satisfies validation.
			PolicyID:        "e2e-dummy-policy-id",
			EnforcementMode: vspherepolv1.PolicyEnforcementModeMandatory,
			Match: &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			},
			Tags: tagPolicyNames,
		},
	}

	Expect(adminClient.Create(ctx, obj)).To(Succeed(), "failed to create AutomaticVMEvictionPolicy %q", name)

	return obj
}
