// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file covers
// ControlledRebalancingPolicy.
package computepolicies

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

// ControlledRebalancingSpec verifies that a Mandatory
// ControlledRebalancingPolicy tags a matching VM and that the policy appears
// in the VM's status.policies, mirroring the VM eviction policy Spec's
// coverage in vmevictionpolicy.go for the single ControlledRebalancingPolicy
// kind. ControlledRebalancingPolicy CRs are mirrored from the WCP InfraPolicy
// admin API exactly like AutomaticVMEvictionPolicy/BestEffortRestartPolicy
// are (see vmevictionpolicy.go) -- the only difference is the capability of
// the real vCenter compute policy the InfraPolicy references
// (wcp.ControlledRebalancingCapability).
func ControlledRebalancingSpec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "controlled-rebalancing-policy"

	var (
		input           SpecInput
		clusterProxy    *common.VMServiceClusterProxy
		svClusterClient ctrlclient.Client
		adminClient     ctrlclient.Client
		vCenterClient   *vim25.Client
		tagManager      *tags.Manager
		tagCategoryID   string

		vmName     string
		vm         *vmopv1.VirtualMachine
		matchLabel map[string]string

		rebalancingPolicy   *vspherepolv1.ControlledRebalancingPolicy
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

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.ControlledRebalancingPolicyCapabilityName)

		cancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{input.Config.GetVariable("VMOPNamespace")},
			clusterProxy.GetRESTConfig(),
			filepath.Join(input.ArtifactFolder, specName),
		)
		DeferCleanup(cancelPodWatches)

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

		// One shared, MULTIPLE-cardinality category per spec run: every tag this Spec
		// creates lives in it, so there's no need for a fresh category per tag or per test.
		tagCategoryID, err = input.WCPClient.CreateTagCategory(
			fmt.Sprintf("%s-category-%s", specName, capiutil.RandomString(4)), "e2e controlled rebalancing policy test")
		Expect(err).ToNot(HaveOccurred(), "failed to create tag category")
		Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")
		DeferCleanup(func(cleanupCtx context.Context) {
			_ = tagManager.DeleteCategory(cleanupCtx, &tags.Category{ID: tagCategoryID})
		})

		vmName = fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
		matchLabel = map[string]string{
			"vmoperator.vmware.com/e2e-controlled-rebalancing-test": capiutil.RandomString(6),
		}
		vm = nil
		rebalancingPolicy = nil
	})

	AfterEach(func() {
		if rebalancingPolicy != nil {
			_ = adminClient.Delete(ctx, rebalancingPolicy)
		}
		if vm != nil {
			vmoperator.DeleteVirtualMachineAndWait(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface the policy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing", suffix)

			By("Creating a Mandatory ControlledRebalancingPolicy matching the test label")
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID)

			policyNameToVMTagID = map[string]string{
				rebalancingPolicy.Name: tagID,
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
				[]string{rebalancingPolicy.Name})

			By("Verifying the real vCenter compute policy attached to the VM has the DisableDrsVmotion capability")
			verifyComputePolicyCapability(ctx, input, tagManager, tagID, wcp.ControlledRebalancingCapability)
		})

	It("Should re-evaluate an already-created VM when a policy's match is widened",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-widen", suffix)

			By("Creating a Mandatory ControlledRebalancingPolicy that does not yet match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-controlled-rebalancing-test": capiutil.RandomString(6),
			}
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-widen-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, nonMatchingLabel, tagID)

			By("Creating a VM that does not match the policy yet")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM does not have the policy applied yet")
			curVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
			Expect(curVM.Status.Policies).To(BeEmpty(),
				"VM should not have any policies applied before the policy's match is widened")

			By("Widening the policy's match to the VM's actual label, without touching the VM")
			rebalancingPolicyPatch := rebalancingPolicy.DeepCopy()
			rebalancingPolicyPatch.Spec.Match = &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			}
			Expect(adminClient.Patch(ctx, rebalancingPolicyPatch, ctrlclient.MergeFrom(rebalancingPolicy))).
				To(Succeed(), "failed to widen ControlledRebalancingPolicy %q match", rebalancingPolicy.Name)

			By("Verifying the already-created VM picks up the widened policy via the ControlledRebalancingPolicy watch")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{rebalancingPolicy.Name: tagID},
				[]string{rebalancingPolicy.Name})
		})

	It("Should tag a VM that explicitly references a matching Optional ControlledRebalancingPolicy",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-optional", suffix)

			By("Creating an Optional ControlledRebalancingPolicy matching the test label")
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-optional-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, tagID)

			By("Creating a VM that explicitly references the policy and matches its label selector")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(controlledRebalancingPolicyKind, rebalancingPolicy.Name))
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and the real vSphere tag assignment")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{rebalancingPolicy.Name: tagID},
				[]string{rebalancingPolicy.Name})
		})

	It("Should surface an error when a VM explicitly references a non-matching Optional ControlledRebalancingPolicy",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-no-match", suffix)

			By("Creating an Optional ControlledRebalancingPolicy that does not match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-controlled-rebalancing-test": capiutil.RandomString(6),
			}
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-no-match-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, nonMatchingLabel, tagID)

			By("Creating a VM that explicitly references the non-matching policy")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(controlledRebalancingPolicyKind, rebalancingPolicy.Name))

			By("Verifying the VM's PolicyEvaluation reports a not-ready error naming the non-matching policy")
			verifyPolicyEvaluationNotReady(ctx, input, svClusterClient, vmName, "does not match")

			By("Verifying the VM's PlacementReady condition surfaces the non-matching policy error")
			verifyVMPlacementNotReady(ctx, input, svClusterClient, vmName, "does not match")
		})

	It("Should update the VM's vSphere tag when the policy's Tags are changed",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID1 := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-update-1", suffix)

			By("Creating a Mandatory ControlledRebalancingPolicy tagging the VM with the first real vSphere tag")
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-update-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID1)

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
				map[string]string{rebalancingPolicy.Name: tagID1},
				[]string{rebalancingPolicy.Name})

			tagID2 := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-update-2", suffix)

			By("Creating a second TagPolicy referencing a second real vSphere tag")
			tagPolicy2 := createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("controlled-rebalancing-update-tag-policy-2-%s", suffix), []string{tagID2})
			DeferCleanup(func() { _ = adminClient.Delete(ctx, tagPolicy2) })

			By("Updating the policy's Tags to reference the second TagPolicy instead of the first")
			rebalancingPolicyPatch := rebalancingPolicy.DeepCopy()
			rebalancingPolicyPatch.Spec.Tags = []string{tagPolicy2.Name}
			Expect(adminClient.Patch(ctx, rebalancingPolicyPatch, ctrlclient.MergeFrom(rebalancingPolicy))).
				To(Succeed(), "failed to update ControlledRebalancingPolicy %q tags", rebalancingPolicy.Name)
			rebalancingPolicy = rebalancingPolicyPatch

			By("Verifying the VM now has only the second tag assigned")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{rebalancingPolicy.Name: tagID2},
				[]string{rebalancingPolicy.Name})
		})

	It("Should remove the VM's vSphere tag and status.policies entry when the policy is deleted",
		Label("core-functional", "experimental"),
		func() {
			suffix := capiutil.RandomString(4)
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "controlled-rebalancing-delete", suffix)

			By("Creating a Mandatory ControlledRebalancingPolicy matching the test label")
			rebalancingPolicy = createControlledRebalancingPolicy(ctx, adminClient, input,
				fmt.Sprintf("controlled-rebalancing-delete-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID)

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
				map[string]string{rebalancingPolicy.Name: tagID},
				[]string{rebalancingPolicy.Name})

			By("Deleting the ControlledRebalancingPolicy")
			Expect(adminClient.Delete(ctx, rebalancingPolicy)).
				To(Succeed(), "failed to delete ControlledRebalancingPolicy %q", rebalancingPolicy.Name)
			rebalancingPolicy = nil

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

// Policy kind name as recorded in a VM's spec.policies/status.policies
// entries, mirroring the private controlledRebalancingPolicyKind constant of
// the same name in
// controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go.
const controlledRebalancingPolicyKind = "ControlledRebalancingPolicy"

// createControlledRebalancingPolicy creates the real vCenter compute policy
// and WCP infrastructure policy that back a ControlledRebalancingPolicy (see
// createVSphereInfraPolicy), then waits for WCP to mirror it into the
// Supervisor cluster as the corresponding CR. It returns the mirrored CR and
// the updated list of infra policy names applied to the namespace, which the
// caller must thread into any subsequent createVSphereInfraPolicy-based call
// in the same namespace.
func createControlledRebalancingPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	input SpecInput,
	name string,
	enforcementMode vspherepolv1.PolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string) *vspherepolv1.ControlledRebalancingPolicy {

	GinkgoHelper()

	_ = createVSphereInfraPolicy(input, input.WCPClient, input.WCPNamespaceName, wcp.ComputePolicySpec{
		Name:        fmt.Sprintf("%s-compute-policy", name),
		Description: "e2e controlled rebalancing policy test",
		VMTagID:     vmTagID,
		Capability:  wcp.ControlledRebalancingCapability,
	}, wcp.InfraPolicySpec{
		Name:               name,
		Description:        "e2e controlled rebalancing policy test",
		EnforcementMode:    wcpEnforcementMode(enforcementMode),
		MatchWorkloadLabel: matchLabel,
	}, nil)

	obj := &vspherepolv1.ControlledRebalancingPolicy{}
	waitForVSpherePolicyCreated(ctx, input, adminClient, name, obj)

	return obj
}

// verifyComputePolicyCapability confirms, directly against vCenter's Compute
// Policies engine, that the real compute policy backing vmTagID has
// wantCapability -- e.g. that a ControlledRebalancingPolicy's compute policy
// is indeed wcp.ControlledRebalancingCapability (disable_drs_vmotion). This
// is independent of, and does not require, the VM being reported compliant
// against the policy (see wcp.WorkloadManagementAPI.GetVMPolicyCompliance),
// since a mandatory policy's tag/capability are attached to a matching VM as
// soon as it is created, before any compliance evaluation completes.
func verifyComputePolicyCapability(
	ctx context.Context,
	input SpecInput,
	tagManager *tags.Manager,
	vmTagID string,
	wantCapability wcp.ComputePolicyCapability) {

	GinkgoHelper()

	tag, err := tagManager.GetTag(ctx, vmTagID)
	Expect(err).ToNot(HaveOccurred(), "failed to get vSphere tag %q", vmTagID)

	category, err := tagManager.GetCategory(ctx, tag.CategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to get vSphere tag category %q", tag.CategoryID)

	Eventually(func(g Gomega) {
		entries, err := input.WCPClient.ListComputePolicyTagUsage(category.Name, tag.Name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(HaveLen(1), "expected exactly one compute policy tag usage entry for tag %q", tag.Name)
		g.Expect(entries[0].Capability).To(Equal(string(wantCapability)))
	}, input.Config.GetIntervals("default", "wait-virtual-machine-compute-policy-status-update")...).
		Should(Succeed(), "compute policy backing tag %q should have capability %q", tag.Name, wantCapability)
}
