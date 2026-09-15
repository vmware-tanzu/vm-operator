// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package computepolicies contains E2E tests for the compute-policy CRDs
// reconciled by the policyevaluation controller. This file contains helpers
// shared across the compute-policy kinds covered by this package.
package computepolicies

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
)

// createVSphereInfraPolicy creates the real vCenter compute policy and WCP
// infrastructure policy backing an AutomaticVMEvictionPolicy/
// BestEffortRestartPolicy/ControlledRebalancingPolicy admin object's required
// PolicyID field, exactly mirroring how pinVMToHost creates a host-affinity
// ComputePolicy+InfraPolicy pair (see virtualmachinelcm.go).
// vmTagID -- the real vSphere tag the caller already created via
// createVSphereTag -- becomes the ComputePolicy's VM tag. Applying the
// resulting InfraPolicy to the namespace is what causes WCP to mirror it
// into the Supervisor cluster as the corresponding CR -- including an
// internally-created TagPolicy wrapping vmTagID, which the caller never
// manages directly. The only thing that determines which CR kind
// (ComputePolicy/AutomaticVMEvictionPolicy/BestEffortRestartPolicy/
// ControlledRebalancingPolicy) gets mirrored down is the capability of the
// underlying ComputePolicy.
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

	computePolicyID, err := wcpClient.CreateComputePolicy(wcp.ComputePolicySpec{
		Name:        fmt.Sprintf("%s-compute-policy-%s", name, capiutil.RandomString(4)),
		Description: "e2e compute policy test",
		VMTagID:     vmTagID,
		Capability:  capability,
	})
	Expect(err).ToNot(HaveOccurred(), "failed to create compute policy")
	Expect(computePolicyID).NotTo(BeEmpty(), "compute policy ID should be returned")

	Expect(wcpClient.CreateInfraPolicy(wcp.InfraPolicySpec{
		Name:               name,
		Description:        "e2e compute policy test",
		ComputePolicyID:    computePolicyID,
		EnforcementMode:    enforcementMode,
		MatchWorkloadLabel: matchLabel,
	})).To(Succeed(), "failed to create infrastructure policy %q", name)

	allInfraPolicyNames := append(append([]string{}, existingInfraPolicyNames...), name)
	Expect(wcpClient.UpdateNamespaceWithInfraPolicies(namespace, allInfraPolicyNames...)).
		To(Succeed(), "failed to apply infrastructure policy %q to namespace", name)

	return allInfraPolicyNames
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
