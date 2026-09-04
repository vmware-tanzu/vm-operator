// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// This file covers ComputePolicy through the same direct-CRD-creation path
// used by AutomaticVMEvictionPolicy/BestEffortRestartPolicy in
// vmevictionpolicy.go, as a regression check that converging all three
// kinds' match-evaluation/result-recording logic onto the shared
// matchablePolicy interface in the policyevaluation controller did not
// change ComputePolicy's own behavior. ComputePolicy already has extensive
// e2e coverage via the legacy InfraPolicy admin API mirroring
// (virtualmachinelcm.go); this spec is intentionally minimal.
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
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

// ComputePolicySpec is a regression check that a Mandatory ComputePolicy
// created directly (bypassing the legacy InfraPolicy admin API mirroring
// exercised elsewhere) still tags a matching VM and surfaces the policy in
// status.policies, unaffected by the policyevaluation controller's shared
// matchablePolicy interface also being implemented by
// AutomaticVMEvictionPolicy/BestEffortRestartPolicy. Reuses SpecInput from
// vmevictionpolicy.go since the required inputs are identical.
func ComputePolicySpec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "compute-policy-regression"

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

		tagPolicy      *vspherepolv1.TagPolicy
		computePolicy  *vspherepolv1.ComputePolicy
		policyNameToID map[string]string
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

		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.IaaSComputePoliciesCapabilityName)

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
			"vmoperator.vmware.com/e2e-compute-policy-test": capiutil.RandomString(6),
		}
		vm = nil
		tagPolicy = nil
		computePolicy = nil
	})

	AfterEach(func() {
		if computePolicy != nil {
			_ = adminClient.Delete(ctx, computePolicy)
		}
		if tagPolicy != nil {
			_ = adminClient.Delete(ctx, tagPolicy)
		}
		if vm != nil {
			vmoperator.DeleteVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineToBeDeleted(ctx, input.Config, svClusterClient, input.WCPNamespaceName, vmName)
		}
	})

	It("Should tag a matching VM and surface a directly-created ComputePolicy in status.policies",
		Label("core-functional", "experimental"),
		func() {
			tagID := createVSphereTag(input.WCPClient, tagManager, "compute-policy-regression")

			By("Creating a TagPolicy referencing the real vSphere tag")
			tagPolicy = createTagPolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("compute-policy-regression-tag-policy-%s", capiutil.RandomString(4)), []string{tagID})

			By("Creating a Mandatory ComputePolicy matching the test label")
			computePolicy = createComputePolicy(ctx, adminClient, input.WCPNamespaceName,
				fmt.Sprintf("compute-policy-regression-%s", capiutil.RandomString(4)), matchLabel, []string{tagPolicy.Name})

			policyNameToID = map[string]string{
				computePolicy.Name: tagID,
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
				policyNameToID,
				[]string{computePolicy.Name})
		})
}

// createComputePolicy creates a Mandatory ComputePolicy CR directly via an
// admin client, matching on the given workload labels. Unlike
// AutomaticVMEvictionPolicy/BestEffortRestartPolicy, PolicyID is optional
// here and intentionally left unset.
func createComputePolicy(
	ctx context.Context,
	adminClient ctrlclient.Client,
	namespace, name string,
	matchLabel map[string]string,
	tagPolicyNames []string) *vspherepolv1.ComputePolicy {

	GinkgoHelper()

	obj := &vspherepolv1.ComputePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vspherepolv1.ComputePolicySpec{
			EnforcementMode: vspherepolv1.PolicyEnforcementModeMandatory,
			Match: &vspherepolv1.MatchSpec{
				Workload: &vspherepolv1.MatchWorkloadSpec{
					Labels: matchLabelSelector(matchLabel),
				},
			},
			Tags: tagPolicyNames,
		},
	}

	Expect(adminClient.Create(ctx, obj)).To(Succeed(), "failed to create ComputePolicy %q", name)

	return obj
}
