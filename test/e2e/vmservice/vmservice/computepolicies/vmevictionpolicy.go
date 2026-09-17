// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file covers
// AutomaticVMEvictionPolicy and BestEffortRestartPolicy, introduced for
// the VM eviction compute policies.
package computepolicies

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
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
	ArtifactFolder   string
	WCPNamespaceName string
}

// Spec verifies that a Mandatory AutomaticVMEvictionPolicy tags a
// matching VM and that the policy appears in the VM's status.policies, per
// .sdd/specs/007-vm-eviction-policy/plan.md's Test strategy item
// 1, plus that an already-created, non-matching VM picks up the policy once
// its match is widened — the latter specifically exercises the
// policyToPolicyEvaluationMapperFn watch/informer path, which a
// fake-client unit test cannot validate. AutomaticVMEvictionPolicy and
// BestEffortRestartPolicy CRs are mirrored from the WCP InfraPolicy admin
// API exactly like ComputePolicy CRs are (see virtualmachinelcm.go's "IaaS
// Policies" Context for a vm_host_affinity example of the same mirroring) --
// the only difference is the capability of the real
// vCenter compute policy the InfraPolicy references (wcp.AutomaticVMEvictionCapability/
// wcp.BestEffortRestartCapability instead of wcp.ComputePolicyCapabilityVMHostAffinity),
// which is what causes WCP to mirror the corresponding kind down, matching
// spec.md's "CSP admin applies AutomaticVMEvictionPolicy" framing. WCP
// creates the backing TagPolicy CR internally as part of that mirroring;
// createInfraPolicyForComputePolicy, createVSphereInfraPolicy,
// createAutomaticVMEvictionComputePolicy, createAutomaticVMEvictionPolicy,
// and createBestEffortRestartPolicy are the single seams for this.
func Spec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "vm-eviction-policy"

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
		suffix     string

		evacuationPolicy    *vspherepolv1.AutomaticVMEvictionPolicy
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

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VMEvictionCapabilityName)

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

		suffix = capiutil.RandomString(4)

		// One shared, MULTIPLE-cardinality category per spec run: every tag this Spec
		// creates (eviction/restart/host-affinity, across every It) lives in it, so
		// there's no need for a fresh category per tag or per test.
		tagCategoryID, err = input.WCPClient.CreateTagCategory(
			fmt.Sprintf("%s-category-%s", specName, suffix), "e2e VM eviction policy test")
		Expect(err).ToNot(HaveOccurred(), "failed to create tag category")
		Expect(tagCategoryID).NotTo(BeEmpty(), "tag category ID should be returned")
		DeferCleanup(func(cleanupCtx context.Context) {
			_ = tagManager.DeleteCategory(cleanupCtx, &tags.Category{ID: tagCategoryID})
		})

		vmName = fmt.Sprintf("%s-%s", specName, suffix)
		matchLabel = map[string]string{
			"vmoperator.vmware.com/e2e-vm-eviction-test": suffix,
		}
		vm = nil
		evacuationPolicy = nil
		restartPolicy = nil
	})

	AfterEach(func() {
		if vm != nil {
			vmoperator.DeleteVirtualMachineAndWait(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface the policy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID, nil)

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
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-widen", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy that does not yet match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
			}
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-widen-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, nonMatchingLabel, tagID, nil)

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

	It("Should tag a VM that explicitly references a matching Optional BestEffortRestartPolicy",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-restart", suffix)

			By("Creating an Optional BestEffortRestartPolicy matching the test label")
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, tagID, nil)

			By("Creating a VM that explicitly references the policy and matches its label selector")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))
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
		})

	It("Should surface an error when a VM explicitly references a non-matching Optional BestEffortRestartPolicy",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-restart-no-match", suffix)

			By("Creating an Optional BestEffortRestartPolicy that does not match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
			}
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-no-match-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, nonMatchingLabel, tagID, nil)

			By("Creating a VM that explicitly references the non-matching policy")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))

			By("Verifying the VM's PolicyEvaluation reports a not-ready error naming the non-matching policy")
			verifyPolicyEvaluationNotReady(ctx, input, svClusterClient, vmName, "does not match")

			By("Verifying the VM's PlacementReady condition surfaces the non-matching policy error")
			verifyVMPlacementNotReady(ctx, input, svClusterClient, vmName, "does not match")
		})

	It("Should surface both a Mandatory AutomaticVMEvictionPolicy and an Optional BestEffortRestartPolicy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mixed", suffix)
			restartTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-restart-mixed", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mixed-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, infraPolicyNames)

			By("Creating an Optional BestEffortRestartPolicy matching the same label")
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-mixed-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, restartTagID, infraPolicyNames)

			By("Creating a VM that matches the mandatory policy and explicitly references the optional one")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and vSphere tags include both policies")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{
					evacuationPolicy.Name: evictionTagID,
					restartPolicy.Name:    restartTagID,
				},
				[]string{evacuationPolicy.Name, restartPolicy.Name})
		})

	It("Should tag a VM matching both a Mandatory AutomaticVMEvictionPolicy and a Mandatory BestEffortRestartPolicy",
		Label("core-functional", "experimental"),
		func() {
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-both-mandatory", suffix)
			restartTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-restart-both-mandatory", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy and a Mandatory BestEffortRestartPolicy, both matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-both-mandatory-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, infraPolicyNames)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-both-mandatory-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, restartTagID, infraPolicyNames)

			By("Creating a VM matching both policies' label selector, with no explicit references")
			vm = createMatchingVM(ctx, input, svClusterClient, vmName, matchLabel)
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and vSphere tags include both policies")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{
					evacuationPolicy.Name: evictionTagID,
					restartPolicy.Name:    restartTagID,
				},
				[]string{evacuationPolicy.Name, restartPolicy.Name})
		})

	It("Should tag a VM explicitly referencing both an Optional AutomaticVMEvictionPolicy and an Optional BestEffortRestartPolicy",
		Label("core-functional", "experimental"),
		func() {
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-both-optional", suffix)
			restartTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-restart-both-optional", suffix)

			By("Creating an Optional AutomaticVMEvictionPolicy and an Optional BestEffortRestartPolicy, both matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-both-optional-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, evictionTagID, infraPolicyNames)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-both-optional-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, restartTagID, infraPolicyNames)

			By("Creating a VM that explicitly references both Optional policies")
			vm = createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, matchLabel,
				explicitPolicyRef(automaticVMEvictionPolicyKind, evacuationPolicy.Name),
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))
			vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)

			By("Verifying the VM's status.policies and vSphere tags include both explicitly-referenced policies")
			vmservice.VerifyVMTagsAndPolicyAssignment(
				ctx,
				input.Config,
				svClusterClient,
				tagManager,
				input.WCPNamespaceName,
				vmName,
				map[string]string{
					evacuationPolicy.Name: evictionTagID,
					restartPolicy.Name:    restartTagID,
				},
				[]string{evacuationPolicy.Name, restartPolicy.Name})
		})

	It("Should update the VM's vSphere tag when the policy's Tags are changed",
		Label("core-functional", "experimental"),
		func() {
			tagID1 := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-update-1", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy tagging the VM with the first real vSphere tag")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-update-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID1, nil)

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

			tagID2 := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-update-2", suffix)

			By("Creating a second TagPolicy referencing a second real vSphere tag")
			tagPolicy2 := createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("vm-eviction-update-tag-policy-2-%s", suffix), []string{tagID2})
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
			tagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-delete", suffix)

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-delete-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, tagID, nil)

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

	// Each of the three cases below drives host maintenance mode against a single VM whose
	// host is whatever DRS happens to place it on.
	It("Should relocate a VM matching a Mandatory AutomaticVMEvictionPolicy off a host entering "+
		"maintenance mode, keeping it powered on",
		Label("core-functional", "experimental"),
		func() {
			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the VM's label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-avep", suffix)
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-avep-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, nil)

			By("Creating a VM matching the policy's label")
			avepVMName := fmt.Sprintf("%s-avep-%s", specName, suffix)
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, avepVMName, matchLabel)

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, avepVMName)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			By(fmt.Sprintf("Putting the VM's host %s into maintenance mode", hostMoRef.Value))
			enterErr := enterHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, input, vCenterClient, hostMoRef, nil)
			})
			if enterErr != nil {
				Skip(fmt.Sprintf("%v; it may have other VMs (e.g. Supervisor control-plane VMs) that could "+
					"not be evacuated in time", enterErr))
			}

			By("Verifying the VM is relocated off the host and stays powered on")
			Eventually(func(g Gomega) {
				g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, avepVMName)).
					ToNot(Equal(hostMoRef), "VM should be relocated off the host entering maintenance mode")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, avepVMName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
		})

	It("Should restart a VM matching an Optional BestEffortRestartPolicy on another host when its "+
		"current host enters maintenance mode",
		Label("core-functional", "experimental"),
		func() {
			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the VM's label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-ber-avep", suffix)
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-ber-avep-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, nil)

			By("Creating an Optional BestEffortRestartPolicy matching the VM's label")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-ber", suffix)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-ber-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, restartTagID, infraPolicyNames)

			By("Creating a VM matching the policy's label")
			berVMName := fmt.Sprintf("%s-ber-%s", specName, suffix)
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, berVMName, matchLabel)

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, berVMName)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			By(fmt.Sprintf("Putting the VM's host %s into maintenance mode", hostMoRef.Value))
			enterErr := enterHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, input, vCenterClient, hostMoRef, nil)
			})
			if enterErr != nil {
				Skip(fmt.Sprintf("%v; it may have other VMs (e.g. Supervisor control-plane VMs) that could "+
					"not be evacuated in time", enterErr))
			}

			By("Verifying the VM is powered on on another host")
			Eventually(func(g Gomega) {
				g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, berVMName)).
					ToNot(Equal(hostMoRef), "VM should be restarted off the host entering maintenance mode")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, berVMName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
		})

	// This VM's real, directly-created mandatory DRS VM/Host affinity rule (see
	// pinVMToHostViaDRSRule) mimics a real-world VM that DRS cannot evacuate off its host --
	// e.g. one with a PCI-passthrough/GPU device -- which is what actually drives the
	// InfraInMaintenance condition, regardless of which policy kind is involved. A native,
	// mandatory rule is used here (rather than the WCP vm_host_affinity ComputePolicy capability
	// used elsewhere in this suite) because that capability only applies a soft/preferential DRS
	// constraint, which a routine DRS pass can override -- observed as the VM relocating off the
	// host the moment its tag was assigned, the opposite of the intended pin. This spec is gated
	// only by consts.VMEvictionCapabilityName (checked in this Spec's BeforeEach); it no longer
	// depends on consts.IaaSComputePoliciesCapabilityName since it no longer goes through a WCP
	// ComputePolicy/InfraPolicy at all.
	It("Should surface VirtualMachinePowerStateSynced=False with reason InfraInMaintenance for a VM "+
		"pinned to a host entering maintenance mode, and clear once it exits",
		Label("core-functional", "experimental"),
		func() {
			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the VM's label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-pin-avep", suffix)
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-pin-avep-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, nil)

			By("Creating a VM matching the eviction policy's label")
			pinVMName := fmt.Sprintf("%s-pin-%s", specName, suffix)
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, pinVMName, matchLabel)

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			vmMoRef := getVMMoRef(ctx, svClusterClient, input.WCPNamespaceName, pinVMName)
			pinVMToHostViaDRSRule(ctx, vCenterClient, hostMoRef, suffix, vmMoRef)

			By("Verifying the VM is still on its pinned host")
			Eventually(func(g Gomega) {
				g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).
					To(Equal(hostMoRef), "VM should stay on the host it was pinned to")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())

			By(fmt.Sprintf("Putting the VM's host %s into maintenance mode", hostMoRef.Value))
			enterErr := enterHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, input, vCenterClient, hostMoRef, nil)
			})
			if enterErr != nil {
				Skip(fmt.Sprintf("%v; it may have other VMs (e.g. Supervisor control-plane VMs) that could "+
					"not be evacuated in time", enterErr))
			}

			By("Verifying DRS powers off the VM (unable to evacuate it) and VirtualMachinePowerStateSynced " +
				"surfaces False/InfraInMaintenance once VM Operator's own power-on retry hits the same fault")
			waitForPowerStateSyncedFalse(ctx, input, svClusterClient, pinVMName, vmopv1.VirtualMachineInfraInMaintenanceReason)
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).To(Equal(hostMoRef),
				"VM should remain on its pinned host while it is in maintenance mode")

			By(fmt.Sprintf("Taking host %s out of maintenance mode", hostMoRef.Value))
			exitHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef, nil)

			By("Verifying the VM powers back on, on the same host")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, pinVMName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).To(Equal(hostMoRef),
				"VM should power back on on the same host it was pinned to")
		})

	// Same pin mechanism as the previous It, but the VM also explicitly references an Optional
	// BestEffortRestartPolicy. Empirically, that changes the surfaced reason: with only the
	// Mandatory AutomaticVMEvictionPolicy in play, the power-on retry hits the
	// NoCompatibleHost/autoevac fault directly and VirtualMachinePowerStateSynced surfaces
	// False/InfraInMaintenance (previous It); with BestEffortRestartPolicy also referenced, the
	// restart path apparently fails with a different error shape that
	// SetPowerStateSyncedCondition's vmutil.ErrInfraMaintenanceFault check doesn't match, so it
	// falls through to the generic False/"NotSynced" reason instead (see
	// pkg/providers/vsphere/vmprovider_vm.go's SetPowerStateSyncedCondition). Both are asserted
	// here so a regression in either path is caught.
	It("Should surface VirtualMachinePowerStateSynced=False with reason NotSynced for a VM pinned to a "+
		"host entering maintenance mode that also explicitly references an Optional BestEffortRestartPolicy",
		Label("core-functional", "experimental"),
		func() {
			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the VM's label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-pin-ber-avep", suffix)
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-pin-ber-avep-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, nil)

			By("Creating an Optional BestEffortRestartPolicy matching the VM's label")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-pin-ber", suffix)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-pin-ber-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, restartTagID, infraPolicyNames)

			By("Creating a VM matching the eviction policy's label, that also explicitly references the BestEffortRestartPolicy")
			pinVMName := fmt.Sprintf("%s-pin-ber-%s", specName, suffix)
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, pinVMName, matchLabel,
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			vmMoRef := getVMMoRef(ctx, svClusterClient, input.WCPNamespaceName, pinVMName)
			pinVMToHostViaDRSRule(ctx, vCenterClient, hostMoRef, suffix, vmMoRef)

			By("Verifying the VM is still on its pinned host")
			Eventually(func(g Gomega) {
				g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).
					To(Equal(hostMoRef), "VM should stay on the host it was pinned to")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())

			By(fmt.Sprintf("Putting the VM's host %s into maintenance mode", hostMoRef.Value))
			enterErr := enterHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, input, vCenterClient, hostMoRef, nil)
			})
			if enterErr != nil {
				Skip(fmt.Sprintf("%v; it may have other VMs (e.g. Supervisor control-plane VMs) that could "+
					"not be evacuated in time", enterErr))
			}

			By("Verifying DRS powers off the VM (unable to evacuate it) and VirtualMachinePowerStateSynced " +
				"surfaces False/NotSynced")
			waitForPowerStateSyncedFalse(ctx, input, svClusterClient, pinVMName, "NotSynced")
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).To(Equal(hostMoRef),
				"VM should remain on its pinned host while it is in maintenance mode")

			By(fmt.Sprintf("Taking host %s out of maintenance mode", hostMoRef.Value))
			exitHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef, nil)

			By("Verifying the VM powers back on, on the same host")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, pinVMName,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, pinVMName)).To(Equal(hostMoRef),
				"VM should power back on on the same host it was pinned to")
		})

	// Unlike the two pinned-VM Its above, this one pins *two* powered-on VMs to the host and
	// enters maintenance mode via enterHostMaintenanceModeNoWait rather than
	// enterHostMaintenanceMode: with two mandatorily-pinned VMs to evacuate instead of one, and
	// only an Optional (rather than Mandatory) AutomaticVMEvictionPolicy backing the tag/autoevac
	// mechanism, the host is not expected to reliably finish entering maintenance mode within the
	// wait-maintenance-mode interval, so the spec does not wait on that task at all -- it only
	// cares that VM1 already surfaces the InfraInMaintenance condition/power-off, and that
	// cancelling the still-pending EnterMaintenanceMode task lets VM1 recover.
	It("Should surface VirtualMachinePowerStateSynced=False with reason InfraInMaintenance for a VM pinned, "+
		"alongside another pinned VM, to a host that does not finish entering maintenance mode, and recover "+
		"once entering maintenance mode is cancelled",
		Label("core-functional", "experimental"),
		func() {
			By("Creating an Optional AutomaticVMEvictionPolicy matching the VMs' label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, tagCategoryID, "vm-eviction-mm-nowait-avep", suffix)
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-nowait-avep-policy-%s", suffix),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, evictionTagID, nil)

			By("Creating the first VM that explicitly references the Optional policy")
			vm1Name := fmt.Sprintf("%s-mm-nowait-1-%s", specName, suffix)
			vm2Name := fmt.Sprintf("%s-mm-nowait-2-%s", specName, suffix)
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, vm1Name, matchLabel,
				explicitPolicyRef(automaticVMEvictionPolicyKind, evacuationPolicy.Name))

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			// DRS's mandatory VM/Host affinity rule below only migrates a VM within its own
			// cluster; a VM placed in a different zone (a different cluster in a multi-zone
			// Supervisor) could never be moved onto VM1's host by that rule. Pinning VM2's zone
			// to VM1's zone at creation guarantees both land in the same cluster.
			vm1, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM1 CR")
			Expect(vm1.Status.Zone).ToNot(BeEmpty(), "VM1 should have a zone assigned")

			By(fmt.Sprintf("Creating the second VM in VM1's zone %q, also referencing the Optional policy", vm1.Status.Zone))
			vm2Labels := make(map[string]string, len(matchLabel)+1)
			for k, v := range matchLabel {
				vm2Labels[k] = v
			}
			vm2Labels["topology.kubernetes.io/zone"] = vm1.Status.Zone
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, vm2Name, vm2Labels,
				explicitPolicyRef(automaticVMEvictionPolicyKind, evacuationPolicy.Name))

			vm1MoRef := getVMMoRef(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
			vm2MoRef := getVMMoRef(ctx, svClusterClient, input.WCPNamespaceName, vm2Name)

			By("Pinning both VMs to that host via a single mandatory DRS VM/Host affinity rule")
			pinVMToHostViaDRSRule(ctx, vCenterClient, hostMoRef, suffix, vm1MoRef, vm2MoRef)

			By(fmt.Sprintf("Kicking off a migration of VM2 onto host %s immediately, rather than waiting on "+
				"DRS's periodic invocation to satisfy the mandatory rule", hostMoRef.Value))
			migrateTask, err := object.NewVirtualMachine(vCenterClient, vm2MoRef).
				Migrate(ctx, nil, object.NewHostSystem(vCenterClient, hostMoRef), vimtypes.VirtualMachineMovePriorityDefaultPriority, "")
			Expect(err).ToNot(HaveOccurred(), "failed to start VM2 migration task")

			if _, err := migrateTask.WaitForResult(ctx); err != nil {
				Skip(fmt.Sprintf("Migrating VM2 onto host %s failed (%v); the host may not have had enough "+
					"capacity to accommodate VM2 alongside VM1", hostMoRef.Value, err))
			}

			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name)).To(Equal(hostMoRef),
				"VM2 should be migrated onto the pinned host")

			By(fmt.Sprintf("Putting the VMs' host %s into maintenance mode, without waiting for the task", hostMoRef.Value))
			enterTask := enterHostMaintenanceModeNoWait(ctx, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, input, vCenterClient, hostMoRef, enterTask)
			})

			By("Verifying VM1's VirtualMachinePowerStateSynced surfaces False/InfraInMaintenance and it is powered off")
			waitForPowerStateSyncedFalse(ctx, input, svClusterClient, vm1Name, vmopv1.VirtualMachineInfraInMaintenanceReason)
			Eventually(func(g Gomega) {
				curVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
				g.Expect(curVM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff),
					"VM1 should be powered off by DRS while its host is entering maintenance mode")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())

			By("Cancelling the still-pending EnterMaintenanceMode task")
			exitHostMaintenanceMode(ctx, input, vCenterClient, hostMoRef, enterTask)

			By("Verifying VM1 powers back on, on the same host")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vm1Name,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
			Eventually(func(g Gomega) {
				curVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vm1Name)
				g.Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")
				g.Expect(curVM.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn),
					"VM1 should be powered back on once maintenance mode is cancelled")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name)).To(Equal(hostMoRef),
				"VM1 should power back on on the same host it was pinned to")
		})
}

// Policy kind names as recorded in a VM's spec.policies/status.policies
// entries, mirroring the private kind constants of the same name in
// controllers/vspherepolicy/policyevaluation/policyevaluation_controller.go.
const (
	automaticVMEvictionPolicyKind = "AutomaticVMEvictionPolicy"
	bestEffortRestartPolicyKind   = "BestEffortRestartPolicy"
)

// explicitPolicyRef builds a spec.policies entry explicitly referencing the
// named CR of the given compute-policy kind.
func explicitPolicyRef(kind, name string) vmopv1.PolicySpec {
	return vmopv1.PolicySpec{
		APIVersion: vspherepolv1.GroupVersion.String(),
		Kind:       kind,
		Name:       name,
	}
}

// buildVM constructs (without creating) a VM in the given namespace with
// the given labels and explicit policy references.
func buildVM(
	input SpecInput,
	imageName, vmName string,
	labels map[string]string,
	policies []vmopv1.PolicySpec) *vmopv1.VirtualMachine {

	clusterResources := input.Config.InfraConfig.ManagementClusterConfig.Resources

	return &vmopv1.VirtualMachine{
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
			Policies:   policies,
		},
	}
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

	return createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, labels)
}

// createVMWithExplicitPolicyRefs creates a VM in the given namespace with
// the given labels and explicit spec.policies references, returning the
// created object so the caller can register it for AfterEach cleanup.
func createVMWithExplicitPolicyRefs(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName string,
	labels map[string]string,
	policies ...vmopv1.PolicySpec) *vmopv1.VirtualMachine {

	GinkgoHelper()

	clusterResources := input.Config.InfraConfig.ManagementClusterConfig.Resources
	imageDisplayName := vmservice.GetDefaultImageDisplayName(clusterResources)
	imageName := vmoperator.WaitForVirtualMachineImageName(
		ctx, &input.Config.Config, svClusterClient, input.WCPNamespaceName, imageDisplayName)

	vm := buildVM(input, imageName, vmName, labels, policies)
	Expect(svClusterClient.Create(ctx, vm)).To(Succeed(), "failed to create virtualmachine %q", vmName)

	return vm
}

// verifyPolicyEvaluationNotReady asserts that the VM's PolicyEvaluation
// object reports a not-ready Ready condition whose message contains
// wantMessageSubstring.
func verifyPolicyEvaluationNotReady(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName, wantMessageSubstring string) {

	GinkgoHelper()

	var policyEvaluation vspherepolv1.PolicyEvaluation
	Eventually(func(g Gomega) {
		g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{
			Namespace: input.WCPNamespaceName,
			Name:      fmt.Sprintf("vm-%s", vmName),
		}, &policyEvaluation)).To(Succeed(), "PolicyEvaluation object should exist")

		cond := apimeta.FindStatusCondition(policyEvaluation.Status.Conditions, vspherepolv1.ReadyConditionType)
		g.Expect(cond).NotTo(BeNil(), "Ready condition should be present")
		g.Expect(cond.Status).To(Equal(metav1.ConditionFalse), "PolicyEvaluation should not be ready")
		g.Expect(cond.Message).To(ContainSubstring(wantMessageSubstring))
	}, input.Config.GetIntervals("default", "wait-policy-evaluation-creation")...).
		Should(Succeed(), "PolicyEvaluation should report the expected not-ready condition")
}

// verifyVMPlacementNotReady asserts that the VM's PlacementReady condition
// is False with a message containing wantMessageSubstring.
func verifyVMPlacementNotReady(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName, wantMessageSubstring string) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		curVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")

		cond := apimeta.FindStatusCondition(curVM.GetConditions(), vmopv1.VirtualMachineConditionPlacementReady)
		g.Expect(cond).NotTo(BeNil(), "PlacementReady condition should be present")
		g.Expect(cond.Status).To(Equal(metav1.ConditionFalse), "PlacementReady should not be ready")
		g.Expect(cond.Message).To(ContainSubstring(wantMessageSubstring))
	}, input.Config.GetIntervals("default", "wait-policy-evaluation-creation")...).
		Should(Succeed(), "PlacementReady should report the expected not-ready condition")
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

// wcpEnforcementMode converts a vspherepolv1.PolicyEnforcementMode
// ("Mandatory"/"Optional") into the casing the WCP admin API's
// InfraPolicyEnforcementMode enum uses ("MANDATORY"/"OPTIONAL").
func wcpEnforcementMode(mode vspherepolv1.PolicyEnforcementMode) wcp.InfraPolicyEnforcementMode {
	return wcp.InfraPolicyEnforcementMode(strings.ToUpper(string(mode)))
}

// createAutomaticVMEvictionComputePolicy creates the real vCenter compute
// policy backing an AutomaticVMEvictionPolicy admin object's required
// PolicyID field, then creates and applies the WCP infrastructure policy for
// it (see createInfraPolicyForComputePolicy). Unlike createVSphereInfraPolicy's
// CreateComputePolicy, dcli's "compute policies create" verb is not
// supported server-side for automatic_vm_eviction; the VM selector and
// restart action must go through createtagsandpolicies instead, which is why
// this can't just build a wcp.ComputePolicySpec for createVSphereInfraPolicy
// like every other capability in this suite does.
func createAutomaticVMEvictionComputePolicy(
	wcpClient wcp.WorkloadManagementAPI,
	namespace string,
	infraPolicySpec wcp.InfraPolicySpec,
	vmTagID string,
	existingInfraPolicyNames []string) []string {

	GinkgoHelper()

	By("Creating a real vCenter compute policy to back the policy's PolicyID")
	computePolicyID, err := wcpClient.CreateComputePolicyWithSpec(
		fmt.Sprintf("%s-compute-policy", infraPolicySpec.Name),
		infraPolicySpec.Description,
		wcp.AutomaticVMEvictionCapability,
		map[string]any{
			"vm_selector": map[string]any{
				"tag": map[string]any{
					"tag_id": vmTagID,
				},
			},
			"restart_action": map[string]any{
				"kind":       "RESTART_ON_CURRENT_HOST",
				"strictness": "REQUIRED",
			},
		})
	Expect(err).ToNot(HaveOccurred(), "failed to create compute policy")
	Expect(computePolicyID).NotTo(BeEmpty(), "compute policy ID should be returned")

	return createInfraPolicyForComputePolicy(wcpClient, namespace, computePolicyID, infraPolicySpec, existingInfraPolicyNames)
}

// createAutomaticVMEvictionPolicy creates the real vCenter compute policy
// and WCP infrastructure policy that back an AutomaticVMEvictionPolicy (see
// createAutomaticVMEvictionComputePolicy), then waits for WCP to mirror it
// into the Supervisor cluster as the corresponding CR. It returns the
// mirrored CR and the updated list of infra policy names applied to the
// namespace, which the caller must thread into any subsequent
// createVSphereInfraPolicy-based call (createBestEffortRestartPolicy) in the
// same namespace.
func createAutomaticVMEvictionPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	input SpecInput,
	name string,
	enforcementMode vspherepolv1.PolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string,
	// existingInfraPolicyNames mirrors createBestEffortRestartPolicy's chaining
	// contract; no existing test happens to call this helper after another one
	// in the same namespace, so it is always nil today.
	existingInfraPolicyNames []string) (*vspherepolv1.AutomaticVMEvictionPolicy, []string) { //nolint:unparam

	GinkgoHelper()

	infraPolicyNames := createAutomaticVMEvictionComputePolicy(input.WCPClient, input.WCPNamespaceName, wcp.InfraPolicySpec{
		Name:               name,
		Description:        "e2e VM eviction policy test",
		EnforcementMode:    wcpEnforcementMode(enforcementMode),
		MatchWorkloadLabel: matchLabel,
	}, vmTagID, existingInfraPolicyNames)

	obj := &vspherepolv1.AutomaticVMEvictionPolicy{}
	waitForVSpherePolicyCreated(ctx, input, adminClient, name, obj)

	return obj, infraPolicyNames
}

// createBestEffortRestartPolicy creates the real vCenter compute policy and
// WCP infrastructure policy that back a BestEffortRestartPolicy (see
// createVSphereInfraPolicy), then waits for WCP to mirror it into the
// Supervisor cluster as the corresponding CR. It returns the mirrored CR and
// the updated list of infra policy names applied to the namespace, which
// the caller must thread into any subsequent createVSphereInfraPolicy-based
// call (createAutomaticVMEvictionPolicy) in the same namespace; no existing
// test calls this helper before another one in the same namespace, so the
// second return value is always discarded today.
func createBestEffortRestartPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	input SpecInput,
	name string,
	enforcementMode vspherepolv1.PolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string,
	existingInfraPolicyNames []string) (*vspherepolv1.BestEffortRestartPolicy, []string) { //nolint:unparam

	GinkgoHelper()

	infraPolicyNames := createVSphereInfraPolicy(input.WCPClient, input.WCPNamespaceName, wcp.ComputePolicySpec{
		Name:        fmt.Sprintf("%s-compute-policy", name),
		Description: "e2e VM eviction policy test",
		VMTagID:     vmTagID,
		Capability:  wcp.BestEffortRestartCapability,
	}, wcp.InfraPolicySpec{
		Name:               name,
		Description:        "e2e VM eviction policy test",
		EnforcementMode:    wcpEnforcementMode(enforcementMode),
		MatchWorkloadLabel: matchLabel,
	}, existingInfraPolicyNames)

	obj := &vspherepolv1.BestEffortRestartPolicy{}
	waitForVSpherePolicyCreated(ctx, input, adminClient, name, obj)

	return obj, infraPolicyNames
}

// getVMMoRef returns the ManagedObjectReference of the real vCenter VM
// backing the named K8s VM CR.
func getVMMoRef(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	namespace, vmName string) vimtypes.ManagedObjectReference {

	GinkgoHelper()

	vm, err := utils.GetVirtualMachine(ctx, svClusterClient, namespace, vmName)
	Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")

	return vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}
}

// getVMHostMoRef returns the ManagedObjectReference of the ESX host
// currently running the named VM.
func getVMHostMoRef(
	ctx context.Context,
	vCenterClient *vim25.Client,
	svClusterClient ctrlclient.Client,
	namespace, vmName string) vimtypes.ManagedObjectReference {

	GinkgoHelper()

	vmMoRef := getVMMoRef(ctx, svClusterClient, namespace, vmName)

	var vmMO mo.VirtualMachine
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"runtime.host"}, &vmMO)).To(Succeed())
	Expect(vmMO.Runtime.Host).ToNot(BeNil(), "VM %q has no host in its runtime info", vmName)

	return *vmMO.Runtime.Host
}

// pinVMToHostViaDRSRule hard-pins vmMoRefs to hostMoRef using a native,
// mandatory vSphere DRS VM/Host affinity rule -- a Host DRS group containing
// hostMoRef, a VM DRS group containing vmMoRefs, and a mandatory
// ClusterVmHostRuleInfo binding them -- rather than the WCP
// vm_host_affinity ComputePolicy capability, which only applies a
// soft/preferential DRS constraint that a routine DRS pass can override.
// Mandatory is the real hard/must-run knob here: per ClusterRuleInfo's
// documented semantics, "a mandatory rule will prevent a virtual machine
// from being powered on or migrated to a host that does not satisfy the
// rule," which is what actually guarantees DRS can't evacuate the VM,
// driving the InfraInMaintenance condition below. The rule and both groups
// are registered for cleanup, rule first since a group referenced by a rule
// can't be removed while the reference exists.
func pinVMToHostViaDRSRule(
	ctx context.Context,
	vCenterClient *vim25.Client,
	hostMoRef vimtypes.ManagedObjectReference,
	suffix string,
	vmMoRefs ...vimtypes.ManagedObjectReference) {

	GinkgoHelper()

	var hostMO mo.HostSystem
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, hostMoRef, []string{"parent"}, &hostMO)).To(Succeed())
	Expect(hostMO.Parent).ToNot(BeNil(), "host %q has no parent compute resource", hostMoRef.Value)

	cluster := object.NewClusterComputeResource(vCenterClient, *hostMO.Parent)

	hostGroupName := fmt.Sprintf("e2e-host-group-%s", suffix)
	vmGroupName := fmt.Sprintf("e2e-vm-group-%s", suffix)
	ruleName := fmt.Sprintf("e2e-affinity-rule-%s", suffix)

	By(fmt.Sprintf("Creating a mandatory DRS VM/Host affinity rule %q pinning the VM(s) to host %s", ruleName, hostMoRef.Value))
	addSpec := &vimtypes.ClusterConfigSpecEx{
		GroupSpec: []vimtypes.ClusterGroupSpec{
			{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Info: &vimtypes.ClusterHostGroup{
					ClusterGroupInfo: vimtypes.ClusterGroupInfo{Name: hostGroupName},
					Host:             []vimtypes.ManagedObjectReference{hostMoRef},
				},
			},
			{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Info: &vimtypes.ClusterVmGroup{
					ClusterGroupInfo: vimtypes.ClusterGroupInfo{Name: vmGroupName},
					Vm:               vmMoRefs,
				},
			},
		},
		RulesSpec: []vimtypes.ClusterRuleSpec{
			{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Info: &vimtypes.ClusterVmHostRuleInfo{
					ClusterRuleInfo: vimtypes.ClusterRuleInfo{
						Name:      ruleName,
						Enabled:   ptr.To(true),
						Mandatory: ptr.To(true),
					},
					VmGroupName:         vmGroupName,
					AffineHostGroupName: hostGroupName,
				},
			},
		},
	}

	task, err := cluster.Reconfigure(ctx, addSpec, true)
	Expect(err).ToNot(HaveOccurred(), "failed to start cluster reconfigure task adding VM/Host affinity rule")
	Expect(task.Wait(ctx)).To(Succeed(), "failed to add VM/Host affinity rule %q", ruleName)

	DeferCleanup(func(cleanupCtx context.Context) {
		config, err := cluster.Configuration(cleanupCtx)
		if err != nil {
			return
		}

		var ruleKey int32
		var ruleFound bool
		for _, r := range config.Rule {
			if info := r.GetClusterRuleInfo(); info != nil && info.Name == ruleName {
				ruleKey = info.Key
				ruleFound = true
				break
			}
		}

		// Only issue the remove if the rule was actually found: RemoveKey's zero
		// value is a real, if unlikely, rule Key, so sending it unconditionally
		// risks removing an unrelated rule instead of silently no-op'ing.
		if ruleFound {
			removeRuleSpec := &vimtypes.ClusterConfigSpecEx{
				RulesSpec: []vimtypes.ClusterRuleSpec{
					{ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationRemove, RemoveKey: ruleKey}},
				},
			}
			if task, err := cluster.Reconfigure(cleanupCtx, removeRuleSpec, true); err == nil {
				_ = task.Wait(cleanupCtx)
			}
		}

		removeGroupsSpec := &vimtypes.ClusterConfigSpecEx{
			GroupSpec: []vimtypes.ClusterGroupSpec{
				{ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationRemove, RemoveKey: hostGroupName}},
				{ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationRemove, RemoveKey: vmGroupName}},
			},
		}
		if task, err := cluster.Reconfigure(cleanupCtx, removeGroupsSpec, true); err == nil {
			_ = task.Wait(cleanupCtx)
		}
	})
}

// enterHostMaintenanceMode puts the given host into maintenance mode,
// waiting up to the "wait-maintenance-mode" interval (see
// e2eConfig.E2EConfig.GetIntervals) for the host to actually report being in
// maintenance mode before returning. It is a no-op (returning nil, nil) if
// the host is already in maintenance mode.
//
// On real vCenter, a powered-on VM must be evacuated before the host fully
// enters maintenance mode. A host can carry VMs this suite doesn't control
// -- e.g. Supervisor control-plane VMs -- whose evacuation this suite has no
// way to speed up, so if the host does not finish entering maintenance mode
// within the interval, this cancels the still-in-progress task (best-effort)
// and returns a non-nil error rather than failing the spec via Expect, so
// the caller can choose to Skip instead of treating environmental
// evacuation delay as a product bug.
func enterHostMaintenanceMode(
	ctx context.Context,
	input SpecInput,
	vCenterClient *vim25.Client,
	hostMoRef vimtypes.ManagedObjectReference) error {

	GinkgoHelper()

	if isHostInMaintenanceMode(ctx, vCenterClient, hostMoRef) {
		return nil
	}

	task, err := object.NewHostSystem(vCenterClient, hostMoRef).EnterMaintenanceMode(ctx, 0, false, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to start EnterMaintenanceMode task for host %q", hostMoRef.Value)

	intervals := input.Config.GetIntervals("default", "wait-maintenance-mode")
	timeout, err := time.ParseDuration(intervals[0].(string))
	Expect(err).ToNot(HaveOccurred(), "failed to parse enter-maintenance-mode timeout interval %q", intervals[0])

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := task.Wait(waitCtx); err != nil {
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			_ = task.Cancel(ctx)
			return fmt.Errorf("host %q did not enter maintenance mode within %s", hostMoRef.Value, timeout)
		}
		Expect(err).ToNot(HaveOccurred(), "EnterMaintenanceMode task failed for host %q", hostMoRef.Value)
	}

	return nil
}

// enterHostMaintenanceModeNoWait starts the EnterMaintenanceMode task for the
// given host and returns immediately without waiting for it to complete. Use
// this instead of enterHostMaintenanceMode when the host is expected to never
// fully reach maintenance mode within the spec (e.g. it has powered-on VMs
// pinned to it that vSphere -- absent an AutomaticVMEvictionPolicy driving
// autoevac -- will not evacuate on its own), so the spec does not block
// waiting for a task that will never complete. It is a no-op (returning nil)
// if the host is already in maintenance mode. Unlike enterHostMaintenanceMode
// -- which cancels a timed-out task itself -- callers here must cancel the
// returned task (if non-nil) before attempting to exit maintenance mode,
// since it is left running.
func enterHostMaintenanceModeNoWait(ctx context.Context, vCenterClient *vim25.Client, hostMoRef vimtypes.ManagedObjectReference) *object.Task {
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
// first (best-effort) as a safety net in case the earlier
// enterHostMaintenanceMode call timed out waiting for evacuation and the
// task is still queued/running. It is a no-op if the host is not in
// maintenance mode.
func exitHostMaintenanceMode(
	ctx context.Context,
	input SpecInput,
	vCenterClient *vim25.Client,
	hostMoRef vimtypes.ManagedObjectReference,
	enterTask *object.Task) {

	GinkgoHelper()

	if enterTask != nil {
		_ = enterTask.Cancel(ctx)
	}

	if !isHostInMaintenanceMode(ctx, vCenterClient, hostMoRef) {
		return
	}

	// Exiting maintenance mode can transiently fail (e.g. a fault from a
	// still-settling evacuation/DRS operation on the host), so retry up to
	// the "wait-maintenance-mode" interval before failing -- unlike the
	// compute-policy cleanup above, the host must actually leave maintenance
	// mode for later specs sharing the cluster to behave correctly, so a
	// timeout here still fails.
	Eventually(func(g Gomega) {
		task, err := object.NewHostSystem(vCenterClient, hostMoRef).ExitMaintenanceMode(ctx, 0)
		g.Expect(err).ToNot(HaveOccurred(), "failed to start ExitMaintenanceMode task for host %q", hostMoRef.Value)
		g.Expect(task.Wait(ctx)).To(Succeed(), "ExitMaintenanceMode task failed for host %q", hostMoRef.Value)
	}, input.Config.GetIntervals("default", "wait-maintenance-mode")...).Should(Succeed())
}

func isHostInMaintenanceMode(ctx context.Context, vCenterClient *vim25.Client, hostMoRef vimtypes.ManagedObjectReference) bool {
	GinkgoHelper()

	var hostMO mo.HostSystem
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, hostMoRef, []string{"runtime.inMaintenanceMode"}, &hostMO)).To(Succeed())

	return hostMO.Runtime.InMaintenanceMode
}

// listClusterHostMoRefs returns the ManagedObjectReferences of every host in
// hostMoRef's cluster (its parent ComputeResource), including hostMoRef
// itself.
func listClusterHostMoRefs(
	ctx context.Context,
	vCenterClient *vim25.Client,
	hostMoRef vimtypes.ManagedObjectReference) []vimtypes.ManagedObjectReference {

	GinkgoHelper()

	var hostMO mo.HostSystem
	propCollector := property.DefaultCollector(vCenterClient)
	Expect(propCollector.RetrieveOne(ctx, hostMoRef, []string{"parent"}, &hostMO)).To(Succeed())
	Expect(hostMO.Parent).ToNot(BeNil(), "host %q has no parent compute resource", hostMoRef.Value)

	hosts, err := object.NewComputeResource(vCenterClient, *hostMO.Parent).Hosts(ctx)
	Expect(err).ToNot(HaveOccurred(), "failed to list hosts in host %q's compute resource", hostMoRef.Value)

	hostMoRefs := make([]vimtypes.ManagedObjectReference, 0, len(hosts))
	for _, h := range hosts {
		hostMoRefs = append(hostMoRefs, h.Reference())
	}

	return hostMoRefs
}

// createAndWaitForPoweredOnVM creates a VM with the given labels and
// explicit spec.policies references, registers its deletion on cleanup, and
// waits for it to be created and powered on.
func createAndWaitForPoweredOnVM(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName string,
	labels map[string]string,
	policies ...vmopv1.PolicySpec) {

	GinkgoHelper()

	createVMWithExplicitPolicyRefs(ctx, input, svClusterClient, vmName, labels, policies...)
	DeferCleanup(func(cleanupCtx context.Context) {
		vmoperator.DeleteVirtualMachine(cleanupCtx, svClusterClient, input.WCPNamespaceName, vmName)
		vmoperator.WaitForVirtualMachineToBeDeleted(cleanupCtx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
	})

	vmoperator.WaitForVirtualMachineCreation(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
	vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
		metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
}

// waitForPowerStateSyncedFalse waits for the named VM's
// VirtualMachinePowerStateSynced condition to surface False with the given
// reason. spec.powerState is never touched here: the named VM stays pinned
// to a host that is in maintenance mode and cannot be evacuated, so DRS's
// own autoevac powers it off, and VM Operator's reconciler -- observing
// spec.powerState=PoweredOn out of sync with the now-PoweredOff status --
// attempts to power it back on, which fails, surfacing the condition. The
// reason depends on the shape of that failure:
// vmopv1.VirtualMachineInfraInMaintenanceReason when the power-on attempt
// hits the NoCompatibleHost/autoevac fault directly (see
// pkg/providers/vsphere/vmprovider_vm.go's SetPowerStateSyncedCondition);
// plain "NotSynced" when the VM also explicitly references an Optional
// BestEffortRestartPolicy, whose restart path apparently fails with a
// different error shape that SetPowerStateSyncedCondition's
// vmutil.ErrInfraMaintenanceFault check doesn't match, falling through to
// the generic reason.
func waitForPowerStateSyncedFalse(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName, reason string) {

	GinkgoHelper()

	vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
		metav1.Condition{
			Type:   vmopv1.VirtualMachinePowerStateSynced,
			Status: metav1.ConditionFalse,
			Reason: reason,
		})
}
