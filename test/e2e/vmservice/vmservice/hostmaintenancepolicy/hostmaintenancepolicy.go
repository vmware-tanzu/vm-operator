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
	corev1 "k8s.io/api/core/v1"
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
			verifyVMHasFailureEvent(ctx, svClusterClient, input.WCPNamespaceName, vmName,
				fmt.Sprintf("%s\" does not match", restartPolicy.Name))
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

// verifyVMHasFailureEvent waits for a Warning event on the given VM whose
// message contains messageSubstring. Explicit policy-reference mismatches
// surface as a reconcile error, which the VM controller records via
// record.Recorder.EmitEvent as a Warning "UpdateFailure" event rather than a
// status condition (see controllers/virtualmachine/virtualmachine).
func verifyVMHasFailureEvent(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	namespace, vmName, messageSubstring string) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, namespace, vmName)
		g.Expect(err).ToNot(HaveOccurred(), "failed to get K8s VM CR")

		var events corev1.EventList
		g.Expect(svClusterClient.List(ctx, &events, ctrlclient.InNamespace(namespace))).To(Succeed())

		var found bool
		for _, e := range events.Items {
			if e.InvolvedObject.UID == vm.UID &&
				e.Type == corev1.EventTypeWarning &&
				strings.Contains(e.Message, messageSubstring) {
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(),
			"expected a Warning event on VM %q containing %q", vmName, messageSubstring)
	}).Should(Succeed())
}
