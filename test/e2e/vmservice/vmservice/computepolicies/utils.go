// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file contains helpers
// shared across the compute-policy kinds covered by this package.
package computepolicies

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vapi/tags"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
)

// createInfraPolicyForComputePolicy fills computePolicyID into
// infraPolicySpec, creates the resulting WCP infrastructure policy, applies
// it -- along with any already-applied infra policies -- to the namespace,
// and registers both for cleanup. Applying the InfraPolicy to the namespace
// is what causes WCP to mirror it into the Supervisor cluster as the
// corresponding CR -- including an internally-created TagPolicy, which the
// caller never manages directly. The only thing that determines which CR
// kind (ComputePolicy/AutomaticVMEvictionPolicy/BestEffortRestartPolicy)
// gets mirrored down is the capability of the underlying ComputePolicy.
//
// UpdateNamespaceWithInfraPolicies sets the namespace's infra-policy list
// rather than appending to it (every other caller in this suite always
// passes the full accumulated list in one call), so callers applying more
// than one infra policy to the same namespace must thread the returned list
// through each successive call to avoid dropping an earlier one.
func createInfraPolicyForComputePolicy(
	wcpClient wcp.WorkloadManagementAPI,
	namespace, computePolicyID string,
	infraPolicySpec wcp.InfraPolicySpec,
	existingInfraPolicyNames []string) []string {

	GinkgoHelper()

	infraPolicySpec.ComputePolicyID = computePolicyID
	Expect(wcpClient.CreateInfraPolicy(infraPolicySpec)).
		To(Succeed(), "failed to create infrastructure policy %q", infraPolicySpec.Name)

	DeferCleanup(func(ctx context.Context) {
		// Delete the infra policy before the compute policy it references.
		_ = wcpClient.DeleteInfraPolicy(infraPolicySpec.Name)

		// Deleting the compute policy right after the infra policy that
		// referenced it can transiently fail while WCP propagates the infra
		// policy removal, so retry for up to a minute. This is best-effort
		// cleanup -- a timeout here is fine and must not fail the spec, so
		// use a Gomega whose fail handler is a no-op instead of the global
		// one.
		g := NewGomega(func(_ string, _ ...int) {})
		g.Eventually(ctx, func() error {
			return wcpClient.DeleteComputePolicy(computePolicyID)
		}).WithTimeout(time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})

	allInfraPolicyNames := append(append([]string{}, existingInfraPolicyNames...), infraPolicySpec.Name)
	Expect(wcpClient.UpdateNamespaceWithInfraPolicies(namespace, allInfraPolicyNames...)).
		To(Succeed(), "failed to apply infrastructure policy %q to namespace", infraPolicySpec.Name)

	return allInfraPolicyNames
}

// createVSphereInfraPolicy creates the real vCenter compute policy described
// by computePolicySpec, then creates and applies the WCP infrastructure
// policy for it (see createInfraPolicyForComputePolicy).
func createVSphereInfraPolicy(
	wcpClient wcp.WorkloadManagementAPI,
	namespace string,
	computePolicySpec wcp.ComputePolicySpec,
	infraPolicySpec wcp.InfraPolicySpec,
	existingInfraPolicyNames []string) []string {

	GinkgoHelper()

	By("Creating a real vCenter compute policy to back the policy's PolicyID")
	computePolicyID, err := wcpClient.CreateComputePolicy(computePolicySpec)
	Expect(err).ToNot(HaveOccurred(), "failed to create compute policy")
	Expect(computePolicyID).NotTo(BeEmpty(), "compute policy ID should be returned")

	return createInfraPolicyForComputePolicy(wcpClient, namespace, computePolicyID, infraPolicySpec, existingInfraPolicyNames)
}

// createVSphereTag creates a real vSphere tag category and tag, registering
// their deletion on cleanup, and returns the tag's ID for use in a
// TagPolicy. suffix is caller-supplied (rather than generated here) so
// callers that create several related objects for the same test can reuse a
// single suffix across all of them, making them easy to correlate in
// vCenter/dcli output.
func createVSphereTag(wcpClient wcp.WorkloadManagementAPI, tagManager *tags.Manager, tagCategoryID, prefix, suffix string) string {
	GinkgoHelper()

	if suffix == "" {
		suffix = capiutil.RandomString(4)
	}

	By("Creating a real vSphere tag to associate with the policy")
	tagName := fmt.Sprintf("%s-tag-%s", prefix, suffix)
	tagID, err := wcpClient.CreateTag(tagName, "e2e VM eviction policy test", tagCategoryID)
	Expect(err).ToNot(HaveOccurred(), "failed to create tag")
	Expect(tagID).NotTo(BeEmpty(), "tag ID should be returned")

	DeferCleanup(func(cleanupCtx context.Context) {
		_ = tagManager.DeleteTag(cleanupCtx, &tags.Tag{ID: tagID})
	})

	return tagID
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

	DeferCleanup(func() {
		_ = adminClient.Delete(ctx, obj)
	})
}
