// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file covers
// AutomaticVMEvictionPolicy and BestEffortRestartPolicy, introduced for
// the VM eviction compute policies.
package computepolicies

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
// fake-client unit test cannot validate. AutomaticVMEvictionPolicy and
// BestEffortRestartPolicy CRs are mirrored from the WCP InfraPolicy admin
// API exactly like ComputePolicy CRs are (see virtualmachinelcm.go's
// pinVMToHost) -- the only difference is the capability of the real
// vCenter compute policy the InfraPolicy references (wcp.AutomaticVMEvictionCapability/
// wcp.BestEffortRestartCapability instead of wcp.ComputePolicyCapabilityVMHostAffinity),
// which is what causes WCP to mirror the corresponding kind down, matching
// spec.md's "CSP admin applies AutomaticVMEvictionPolicy" framing. WCP
// creates the backing TagPolicy CR internally as part of that mirroring;
// createVSphereInfraPolicy/createAutomaticVMEvictionPolicy/
// createBestEffortRestartPolicy are the single seams for this.
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
		if vm != nil {
			vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface the policy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction")

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-policy-%s", capiutil.RandomString(4)),
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
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-widen")

			By("Creating a Mandatory AutomaticVMEvictionPolicy that does not yet match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
			}
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-widen-policy-%s", capiutil.RandomString(4)),
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
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-restart")

			By("Creating an Optional BestEffortRestartPolicy matching the test label")
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-policy-%s", capiutil.RandomString(4)),
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
			tagID := createVSphereTag(input.WCPClient, tagManager, "vm-restart-no-match")

			By("Creating an Optional BestEffortRestartPolicy that does not match the VM's label")
			nonMatchingLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-test": capiutil.RandomString(6),
			}
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-no-match-policy-%s", capiutil.RandomString(4)),
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
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-mixed")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "vm-restart-mixed")

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mixed-policy-%s", capiutil.RandomString(4)),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, infraPolicyNames)

			By("Creating an Optional BestEffortRestartPolicy matching the same label")
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-mixed-policy-%s", capiutil.RandomString(4)),
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
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-both-mandatory")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "vm-restart-both-mandatory")

			By("Creating a Mandatory AutomaticVMEvictionPolicy and a Mandatory BestEffortRestartPolicy, both matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-both-mandatory-policy-%s", capiutil.RandomString(4)),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, infraPolicyNames)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-both-mandatory-policy-%s", capiutil.RandomString(4)),
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
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-both-optional")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "vm-restart-both-optional")

			By("Creating an Optional AutomaticVMEvictionPolicy and an Optional BestEffortRestartPolicy, both matching the test label")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-both-optional-policy-%s", capiutil.RandomString(4)),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, evictionTagID, infraPolicyNames)
			restartPolicy, _ = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-restart-both-optional-policy-%s", capiutil.RandomString(4)),
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
			tagID1 := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-update-1")

			By("Creating a Mandatory AutomaticVMEvictionPolicy tagging the VM with the first real vSphere tag")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-update-policy-%s", capiutil.RandomString(4)),
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

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching the test label")
			evacuationPolicy, _ = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-delete-policy-%s", capiutil.RandomString(4)),
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

	// VM2/VM3's real, directly-created vm_host_affinity ComputePolicy (see pinVMToHost) mimics a real-world
	// VM that DRS cannot evacuate off its host -- e.g. one with a PCI-passthrough/GPU device -- which is what
	// actually drives the InfraInMaintenance condition, regardless of which policy kind is involved. That
	// ComputePolicy capability is gated by consts.IaaSComputePoliciesCapabilityName, the same capability every
	// other use of wcp.ComputePolicyCapabilityVMHostAffinity in this suite skips on (see virtualmachinelcm.go's
	// "IaaS Policies" Context) -- a different, independently-toggled capability than the
	// consts.VMEvictionCapabilityName this Spec's BeforeEach already checks, hence the extra skip below.
	It("Should surface VirtualMachinePowerStateSynced=False with reason InfraInMaintenance for VMs that "+
		"cannot be evacuated off a host entering maintenance mode, and clear once it exits",
		Label("core-functional", "experimental"),
		func() {
			skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.IaaSComputePoliciesCapabilityName)

			By("Creating a Mandatory AutomaticVMEvictionPolicy matching all three VMs' label")
			evictionTagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-mm")
			var infraPolicyNames []string
			evacuationPolicy, infraPolicyNames = createAutomaticVMEvictionPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-policy-%s", capiutil.RandomString(4)),
				vspherepolv1.PolicyEnforcementModeMandatory, matchLabel, evictionTagID, infraPolicyNames)

			By("Creating an Optional BestEffortRestartPolicy matching the test label")
			restartTagID := createVSphereTag(input.WCPClient, tagManager, "vm-eviction-mm-restart")
			restartPolicy, infraPolicyNames = createBestEffortRestartPolicy(ctx, adminClient, input,
				fmt.Sprintf("vm-eviction-mm-restart-policy-%s", capiutil.RandomString(4)),
				vspherepolv1.PolicyEnforcementModeOptional, matchLabel, restartTagID, infraPolicyNames)

			By("Creating VM1, a normal VM matching only the mandatory AutomaticVMEvictionPolicy")
			vm1Name := fmt.Sprintf("%s-vm1-%s", specName, capiutil.RandomString(4))
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, vm1Name, matchLabel)

			hostMoRef := getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name)
			if len(listClusterHostMoRefs(ctx, vCenterClient, hostMoRef)) < 2 {
				Skip("this spec requires more than one host in the cluster to distinguish VM relocation " +
					"from a VM that cannot be evacuated")
			}

			By("Creating VM2, pinned to VM1's host, matching only the mandatory AutomaticVMEvictionPolicy")
			vm2Name := fmt.Sprintf("%s-vm2-%s", specName, capiutil.RandomString(4))
			vm2PinLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-pin": capiutil.RandomString(6),
			}
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, vm2Name, mergeLabels(matchLabel, vm2PinLabel))
			infraPolicyNames = pinVMToHost(ctx, input.WCPClient, tagManager, svClusterClient, input.Config,
				input.WCPNamespaceName, vm2Name, hostMoRef, vm2PinLabel, "vm-eviction-mm-vm2", infraPolicyNames)
			waitForVMHost(ctx, vCenterClient, svClusterClient, input.Config, input.WCPNamespaceName, vm2Name, hostMoRef)

			By("Creating VM3, pinned to VM1's host and explicitly referencing the BestEffortRestartPolicy")
			vm3Name := fmt.Sprintf("%s-vm3-%s", specName, capiutil.RandomString(4))
			vm3PinLabel := map[string]string{
				"vmoperator.vmware.com/e2e-vm-eviction-pin": capiutil.RandomString(6),
			}
			createAndWaitForPoweredOnVM(ctx, input, svClusterClient, vm3Name, mergeLabels(matchLabel, vm3PinLabel),
				explicitPolicyRef(bestEffortRestartPolicyKind, restartPolicy.Name))
			pinVMToHost(ctx, input.WCPClient, tagManager, svClusterClient, input.Config,
				input.WCPNamespaceName, vm3Name, hostMoRef, vm3PinLabel, "vm-eviction-mm-vm3", infraPolicyNames)
			waitForVMHost(ctx, vCenterClient, svClusterClient, input.Config, input.WCPNamespaceName, vm3Name, hostMoRef)

			By("Putting VM1's host into maintenance mode")
			enterTask := enterHostMaintenanceMode(ctx, vCenterClient, hostMoRef)
			DeferCleanup(func(cleanupCtx context.Context) {
				exitHostMaintenanceMode(cleanupCtx, vCenterClient, hostMoRef, enterTask)
			})

			By("Verifying VM1 is relocated off the host entering maintenance mode")
			Eventually(func(g Gomega) {
				g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm1Name)).
					ToNot(Equal(hostMoRef), "VM1 should be relocated off the host entering maintenance mode")
			}, input.Config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed())
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vm1Name,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})

			By("Verifying DRS powers off VM2 and VM3 (unable to evacuate them) and VirtualMachinePowerStateSynced " +
				"surfaces False/InfraInMaintenance once VM Operator's own power-on retry hits the same fault")
			waitForInfraInMaintenance(ctx, input, svClusterClient, vm2Name)
			waitForInfraInMaintenance(ctx, input, svClusterClient, vm3Name)

			By("Taking the host out of maintenance mode")
			exitHostMaintenanceMode(ctx, vCenterClient, hostMoRef, enterTask)
			enterTask = nil

			By("Verifying VM2 powers back on, on the same host")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vm2Name,
				metav1.Condition{Type: vmopv1.VirtualMachinePowerStateSynced, Status: metav1.ConditionTrue})
			Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, input.WCPNamespaceName, vm2Name)).To(Equal(hostMoRef),
				"VM2 should power back on on the same host it was pinned to")

			By("Verifying VM3 powers back on, on any host")
			vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vm3Name,
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

// createTagPolicy creates a TagPolicy CR directly via an admin client.
//
// TODO(vmop-4104): replace with a call into a WCP admin API once one exists
// for this CRD, mirroring how wcp.WorkloadManagementAPI.CreateInfraPolicy
// mirrors ComputePolicy CRs from the WCP InfraPolicy admin API today.
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

// wcpEnforcementMode converts a vspherepolv1.PolicyEnforcementMode
// ("Mandatory"/"Optional") into the casing the WCP admin API's
// InfraPolicyEnforcementMode enum uses ("MANDATORY"/"OPTIONAL").
func wcpEnforcementMode(mode vspherepolv1.PolicyEnforcementMode) wcp.InfraPolicyEnforcementMode {
	return wcp.InfraPolicyEnforcementMode(strings.ToUpper(string(mode)))
}

// waitForVSpherePolicyCreated waits for the named AutomaticVMEvictionPolicy/
// BestEffortRestartPolicy CR -- mirrored into the Supervisor cluster from
// the WCP InfraPolicy admin object of the same name -- to appear, then
// fetches it into obj.
func waitForVSpherePolicyCreated(
	ctx context.Context,
	input SpecInput,
	adminClient ctrlclient.Client,
	name string,
	obj ctrlclient.Object) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		g.Expect(adminClient.Get(ctx, ctrlclient.ObjectKey{
			Namespace: input.WCPNamespaceName,
			Name:      name,
		}, obj)).To(Succeed())
	}, input.Config.GetIntervals("default", "wait-policy-evaluation-creation")...).
		Should(Succeed(), "%T %q should be mirrored into the Supervisor cluster", obj, name)
}

// createVSphereInfraPolicy creates the real vCenter compute policy and WCP
// infrastructure policy backing an AutomaticVMEvictionPolicy/
// BestEffortRestartPolicy admin object's required PolicyID field, exactly
// mirroring how pinVMToHost creates a host-affinity ComputePolicy+InfraPolicy
// pair (see virtualmachinelcm.go): a host tag/VM tag pair is created, the
// host tag is assigned to an arbitrary real host (the eviction/restart
// mechanism doesn't act on a specific host, but CreateComputePolicy always
// requires a host tag), and vmTagID -- the real vSphere tag the caller
// already created via createVSphereTag -- becomes the ComputePolicy's VM
// tag. Applying the resulting InfraPolicy to the namespace is what causes
// WCP to mirror it into the Supervisor cluster as the corresponding CR --
// including an internally-created TagPolicy wrapping vmTagID, which the
// caller never manages directly. The only thing that determines which CR
// kind (ComputePolicy/AutomaticVMEvictionPolicy/BestEffortRestartPolicy)
// gets mirrored down is the capability of the underlying ComputePolicy.
//
// UpdateNamespaceWithInfraPolicies sets the namespace's infra-policy list
// rather than appending to it (every other caller in this suite always
// passes the full accumulated list in one call -- see pinVMToHost), so
// callers applying more than one infra policy to the same namespace must
// thread the returned list through each successive call to avoid dropping
// an earlier one.
func createVSphereInfraPolicy(
	wcpClient wcp.WorkloadManagementAPI,
	namespace, name string,
	capability wcp.ComputePolicyCapability,
	enforcementMode wcp.InfraPolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string,
	existingInfraPolicyNames []string) []string {

	GinkgoHelper()

	By("Creating a real vCenter compute policy to back the policy's PolicyID")
	hostIDs, err := wcpClient.ListHostIDs()
	Expect(err).ToNot(HaveOccurred(), "failed to list host IDs")
	Expect(hostIDs).NotTo(BeEmpty(), "at least one host should be available")

	tagCategoryID, err := wcpClient.CreateTagCategory(
		fmt.Sprintf("%s-host-category-%s", name, capiutil.RandomString(4)), "e2e VM eviction policy test")
	Expect(err).ToNot(HaveOccurred(), "failed to create host tag category")

	hostTagID, err := wcpClient.CreateTag(
		fmt.Sprintf("%s-host-tag-%s", name, capiutil.RandomString(4)), "e2e VM eviction policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create host tag")

	Expect(wcpClient.AssignTagsToHost([]string{hostTagID}, hostIDs[0])).
		To(Succeed(), "failed to assign tag to host %q", hostIDs[0])

	computePolicyID, err := wcpClient.CreateComputePolicy(wcp.ComputePolicySpec{
		Name:        fmt.Sprintf("%s-compute-policy-%s", name, capiutil.RandomString(4)),
		Description: "e2e VM eviction policy test",
		HostTagID:   hostTagID,
		VMTagID:     vmTagID,
		Capability:  capability,
	})
	Expect(err).ToNot(HaveOccurred(), "failed to create compute policy")
	Expect(computePolicyID).NotTo(BeEmpty(), "compute policy ID should be returned")

	Expect(wcpClient.CreateInfraPolicy(wcp.InfraPolicySpec{
		Name:               name,
		Description:        "e2e VM eviction policy test",
		ComputePolicyID:    computePolicyID,
		EnforcementMode:    enforcementMode,
		MatchWorkloadLabel: matchLabel,
	})).To(Succeed(), "failed to create infrastructure policy %q", name)

	allInfraPolicyNames := append(append([]string{}, existingInfraPolicyNames...), name)
	Expect(wcpClient.UpdateNamespaceWithInfraPolicies(namespace, allInfraPolicyNames...)).
		To(Succeed(), "failed to apply infrastructure policy %q to namespace", name)

	return allInfraPolicyNames
}

// createAutomaticVMEvictionPolicy creates the real vCenter compute policy
// and WCP infrastructure policy that back an AutomaticVMEvictionPolicy (see
// createVSphereInfraPolicy), then waits for WCP to mirror it into the
// Supervisor cluster as the corresponding CR. It returns the mirrored CR and
// the updated list of infra policy names applied to the namespace, which
// the caller must thread into any subsequent createVSphereInfraPolicy-based
// call (createBestEffortRestartPolicy, pinVMToHost) in the same namespace.
func createAutomaticVMEvictionPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	input SpecInput,
	name string,
	enforcementMode vspherepolv1.PolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string,
	existingInfraPolicyNames []string) (*vspherepolv1.AutomaticVMEvictionPolicy, []string) {

	GinkgoHelper()

	infraPolicyNames := createVSphereInfraPolicy(input.WCPClient, input.WCPNamespaceName, name,
		wcp.AutomaticVMEvictionCapability, wcpEnforcementMode(enforcementMode), matchLabel, vmTagID, existingInfraPolicyNames)

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
// call (createAutomaticVMEvictionPolicy, pinVMToHost) in the same namespace.
func createBestEffortRestartPolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	input SpecInput,
	name string,
	enforcementMode vspherepolv1.PolicyEnforcementMode,
	matchLabel map[string]string,
	vmTagID string,
	existingInfraPolicyNames []string) (*vspherepolv1.BestEffortRestartPolicy, []string) {

	GinkgoHelper()

	infraPolicyNames := createVSphereInfraPolicy(input.WCPClient, input.WCPNamespaceName, name,
		wcp.BestEffortRestartCapability, wcpEnforcementMode(enforcementMode), matchLabel, vmTagID, existingInfraPolicyNames)

	obj := &vspherepolv1.BestEffortRestartPolicy{}
	waitForVSpherePolicyCreated(ctx, input, adminClient, name, obj)

	return obj, infraPolicyNames
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
	prefix string,
	existingInfraPolicyNames []string) []string {

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
	})).To(Succeed(), "failed to assign infra policy")

	// UpdateNamespaceWithInfraPolicies sets the namespace's infra-policy list rather than
	// appending to it (every other caller in this suite always passes the full accumulated
	// list in one call), so callers pinning more than one VM must thread the returned list
	// through each successive call to avoid dropping an earlier pin.
	allInfraPolicyNames := append(append([]string{}, existingInfraPolicyNames...), infraPolicyName)
	Expect(wcpClient.UpdateNamespaceWithInfraPolicies(namespace, allInfraPolicyNames...)).
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

	return allInfraPolicyNames
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

// mergeLabels returns a new map containing every key/value pair from both
// given label maps.
func mergeLabels(a, b map[string]string) map[string]string {
	merged := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}

	return merged
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

// waitForVMHost waits until the named VM's host matches wantHostMoRef,
// e.g. after pinVMToHost's affinity constraint has driven a vMotion there.
func waitForVMHost(
	ctx context.Context,
	vCenterClient *vim25.Client,
	svClusterClient ctrlclient.Client,
	config *e2eConfig.E2EConfig,
	namespace, vmName string,
	wantHostMoRef vimtypes.ManagedObjectReference) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		g.Expect(getVMHostMoRef(ctx, vCenterClient, svClusterClient, namespace, vmName)).To(Equal(wantHostMoRef))
	}, config.GetIntervals("default", "wait-policy-evaluation-compliant")...).Should(Succeed(),
		"VM %q should be relocated to host %q", vmName, wantHostMoRef.Value)
}

// waitForInfraInMaintenance waits for the named VM's VirtualMachinePowerStateSynced
// condition to surface False/InfraInMaintenance. spec.powerState is never
// touched here: the named VM stays pinned to a host that is in maintenance
// mode and cannot be evacuated, so DRS's own autoevac powers it off, and VM
// Operator's reconciler -- observing spec.powerState=PoweredOn out of sync
// with the now-PoweredOff status -- attempts to power it back on, which
// fails with the same NoCompatibleHost/autoevac fault, surfacing the
// condition.
func waitForInfraInMaintenance(
	ctx context.Context,
	input SpecInput,
	svClusterClient ctrlclient.Client,
	vmName string) {

	GinkgoHelper()

	vmoperator.WaitOnVirtualMachineCondition(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName,
		metav1.Condition{
			Type:   vmopv1.VirtualMachinePowerStateSynced,
			Status: metav1.ConditionFalse,
			Reason: vmopv1.VirtualMachineInfraInMaintenanceReason,
		})
}
