// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

// AffinityStatusEnabled returns true when the VM affinity status feature is
// enabled. The feature requires both the TaggingAPI and the
// VMHardAffinityDuringExecution features.
func AffinityStatusEnabled(ctx context.Context) bool {
	f := pkgcfg.FromContext(ctx).Features
	return f.TaggingAPI && f.VMHardAffinityDuringExecution
}

type affinityStatusRetryContextKeyType uint8

const affinityStatusRetryContextKey affinityStatusRetryContextKeyType = 0

// WithAffinityStatusRetryContext returns a new context that records whether
// the affinity status of the VM being reconciled should be retried. The VM
// controller adds it to every reconcile, and reads it with
// IsAffinityStatusRetry to requeue the VM.
func WithAffinityStatusRetryContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		affinityStatusRetryContextKey,
		func() bool { return false })
}

// MarkAffinityStatusRetry records that an error prevented the affinity status
// from being fully resolved, so the VM should be reconciled again. It is a
// no-op when ctx was not created by WithAffinityStatusRetryContext.
func MarkAffinityStatusRetry(ctx context.Context) {
	if !ctxgen.ValidateContext[bool](ctx, affinityStatusRetryContextKey) {
		return
	}
	ctxgen.SetContext(
		ctx,
		affinityStatusRetryContextKey,
		func(bool) bool { return true })
}

// IsAffinityStatusRetry returns true when MarkAffinityStatusRetry was called
// on ctx. It returns false when ctx was not created by
// WithAffinityStatusRetryContext.
func IsAffinityStatusRetry(ctx context.Context) bool {
	if !ctxgen.ValidateContext[bool](ctx, affinityStatusRetryContextKey) {
		return false
	}
	return ctxgen.FromContext(
		ctx,
		affinityStatusRetryContextKey,
		func(val bool) bool { return val })
}

// IsAffinityPolicyTag returns true when the Tag defines a vCenter compute
// policy, which is the case for the AffinityTarget and
// GroupedAntiAffinitySelf kinds.
func IsAffinityPolicyTag(tag *vspherepolv1.Tag) bool {
	switch tag.Spec.Kind {
	case vspherepolv1.TagKindAffinityTarget,
		vspherepolv1.TagKindGroupedAntiAffinitySelf:
		return true
	case vspherepolv1.TagKindOriginal,
		vspherepolv1.TagKindGroupedAntiAffinityTarget:
		return false
	default:
		return false
	}
}

// RelatedAffinityPolicyTags returns the policy-defining Tags that are related
// to the VM, sorted by Tag name with no duplicates. A VM is related to a Tag
// if it:
// - owns it, because its spec.affinity requests the policy, or
// - carries it, because it has the source label of an AffinityTarget Tag.
//
// Owned Tags are the AffinityTarget and GroupedAntiAffinitySelf identities
// from DesiredAffinityTags, each read from k8sClient by
// ConcreteTagResourceName so that its status is available. An owned Tag that
// is not found is returned as its identity, with an empty status, so it is
// still reported as Pending.
//
// Carried Tags come from ListLabelDerivedTags, which never returns
// GroupedAntiAffinitySelf Tags, and are filtered to AffinityTarget.
//
// k8sClient must be a cache-backed client with the source-pair index. The
// result is either complete or an error is returned.
func RelatedAffinityPolicyTags(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) ([]vspherepolv1.Tag, error) {

	desired, err := DesiredAffinityTags(ctx, vm)
	if err != nil {
		return nil, fmt.Errorf("get desired affinity Tags: %w", err)
	}

	related := map[string]vspherepolv1.Tag{}

	var owned []vspherepolv1.Tag
	for i := range desired {
		if !IsAffinityPolicyTag(&desired[i]) {
			continue
		}
		var tag vspherepolv1.Tag
		if err := k8sClient.Get(
			ctx,
			ctrlclient.ObjectKeyFromObject(&desired[i]),
			&tag); err != nil {

			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf(
					"get affinity Tag %q: %w", desired[i].Name, err)
			}
			tag = desired[i]
		}
		owned = append(owned, tag)
		related[tag.Name] = tag
	}

	carried, err := ListLabelDerivedTags(ctx, k8sClient, vm, owned)
	if err != nil {
		return nil, fmt.Errorf("list label-derived Tags: %w", err)
	}
	for _, tag := range carried {
		if tag.Spec.Kind != vspherepolv1.TagKindAffinityTarget {
			continue
		}
		if _, ok := related[tag.Name]; !ok {
			related[tag.Name] = tag
		}
	}

	tags := make([]vspherepolv1.Tag, 0, len(related))
	for _, tag := range related {
		tags = append(tags, tag)
	}
	slices.SortFunc(tags, func(a, b vspherepolv1.Tag) int {
		return strings.Compare(a.Name, b.Name)
	})
	return tags, nil
}

// The compliance values that the VM's computePolicies property reports.
const (
	computePolicyComplianceCompliant     = "Compliant"
	computePolicyComplianceNonCompliant  = "NonCompliant"
	computePolicyComplianceNotApplicable = "NotApplicable"
)

// OwnedAffinityPolicyTagNames returns the names of the policy-defining Tags
// that the VM's own spec.affinity requests, which are the AffinityTarget and
// GroupedAntiAffinitySelf identities from DesiredAffinityTags.
func OwnedAffinityPolicyTagNames(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) (sets.Set[string], error) {

	desired, err := DesiredAffinityTags(ctx, vm)
	if err != nil {
		return nil, fmt.Errorf("get desired affinity Tags: %w", err)
	}

	names := sets.New[string]()
	for i := range desired {
		if IsAffinityPolicyTag(&desired[i]) {
			names.Insert(desired[i].Name)
		}
	}
	return names, nil
}

// AffinityComplianceFromVCenter maps a compliance state from the VM's
// computePolicies property to a status compliance. Unknown and any
// unrecognized value map to Unknown.
func AffinityComplianceFromVCenter(
	compliance string) vmopv1.VirtualMachineAffinityCompliance {

	switch compliance {
	case computePolicyComplianceCompliant:
		return vmopv1.VirtualMachineAffinityComplianceCompliant
	case computePolicyComplianceNonCompliant:
		return vmopv1.VirtualMachineAffinityComplianceNotCompliant
	case computePolicyComplianceNotApplicable:
		return vmopv1.VirtualMachineAffinityComplianceNotApplicable
	default:
		return vmopv1.VirtualMachineAffinityComplianceUnknown
	}
}

// AffinityPolicyStatusFromTag returns the status entry of the policy that a
// policy-defining Tag defines, with the given compliance and no policy ID.
// The type, strictness, topology key, and targets come from the Tag's
// spec.kind and spec.policy. Targets are sorted by key, then value, with no
// duplicates.
//
// False is returned when the Tag does not define a policy or its policy
// identity holds a value that has no status mapping.
func AffinityPolicyStatusFromTag(
	tag *vspherepolv1.Tag,
	compliance vmopv1.VirtualMachineAffinityCompliance) (
	vmopv1.VirtualMachineAffinityPolicyStatus, bool) {

	var status vmopv1.VirtualMachineAffinityPolicyStatus

	policy := tag.Spec.Policy
	if policy == nil {
		return status, false
	}

	switch tag.Spec.Kind {
	case vspherepolv1.TagKindAffinityTarget:
		status.Type = vmopv1.VirtualMachineAffinityPolicyTypeAffinity
	case vspherepolv1.TagKindGroupedAntiAffinitySelf:
		status.Type = vmopv1.VirtualMachineAffinityPolicyTypeAntiAffinity
	default:
		return status, false
	}

	switch vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness(policy.Strictness) {
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution:
		status.Strictness = vmopv1.VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingRequiredDuringExecution
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution:
		status.Strictness = vmopv1.VirtualMachineAffinityPolicyStrictnessRequiredDuringSchedulingPreferredDuringExecution
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution:
		status.Strictness = vmopv1.VirtualMachineAffinityPolicyStrictnessPreferredDuringSchedulingPreferredDuringExecution
	default:
		return status, false
	}

	switch vimtypes.VmPlacementPolicyVmPlacementPolicyTopology(policy.Topology) {
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost:
		status.TopologyKey = corev1.LabelHostname
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone:
		status.TopologyKey = corev1.LabelTopologyZone
	default:
		return status, false
	}

	status.Targets = make(
		[]vmopv1.VirtualMachineAffinityLabel, 0, len(policy.Targets))
	for _, target := range policy.Targets {
		status.Targets = append(status.Targets,
			vmopv1.VirtualMachineAffinityLabel{
				Key:   target.Key,
				Value: target.Value,
			})
	}
	slices.SortFunc(status.Targets, compareAffinityLabels)
	status.Targets = slices.Compact(status.Targets)

	status.Compliance = compliance
	return status, true
}

// SortAffinityPolicyStatuses sorts the entries by topology key, strictness,
// type, and targets, so that rebuilding the same set produces the same list.
// Entries that compare equal are ordered by policy ID.
func SortAffinityPolicyStatuses(
	entries []vmopv1.VirtualMachineAffinityPolicyStatus) {

	slices.SortFunc(entries, func(a, b vmopv1.VirtualMachineAffinityPolicyStatus) int {
		return cmp.Or(
			strings.Compare(a.TopologyKey, b.TopologyKey),
			strings.Compare(string(a.Strictness), string(b.Strictness)),
			strings.Compare(string(a.Type), string(b.Type)),
			slices.CompareFunc(a.Targets, b.Targets, compareAffinityLabels),
			strings.Compare(a.PolicyID, b.PolicyID),
		)
	})
}

func compareAffinityLabels(a, b vmopv1.VirtualMachineAffinityLabel) int {
	return cmp.Or(
		strings.Compare(a.Key, b.Key),
		strings.Compare(a.Value, b.Value),
	)
}
