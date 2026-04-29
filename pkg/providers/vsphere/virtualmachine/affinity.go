// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"slices"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// extractAffinityLabelsFromVM extracts all "key:value" labels referenced
// in the VM's affinity and anti-affinity rules.
// Returns a set where keys are "key:value" strings
// that are used in affinity rules.
func extractAffinityLabelsFromVM(vmCtx pkgctx.VirtualMachineContext) sets.Set[string] {
	affinityLabels := sets.New[string]()

	affinity := vmCtx.VM.Spec.Affinity
	if affinity == nil {
		return nil
	}

	// helper to extract "key:value" labels from VMAffinityTerm slice
	extractFromTerms := func(terms []vmopv1.VMAffinityTerm) {
		for _, term := range terms {
			if term.LabelSelector == nil {
				continue
			}

			labels, err := extractLabelsFromSelector(term.LabelSelector)
			if err != nil {
				vmCtx.Logger.Error(err, "invalid label selector")
				continue
			}

			for _, label := range labels {
				affinityLabels.Insert(label)
			}
		}
	}

	// process VM affinity rules
	if affinity.VMAffinity != nil {
		extractFromTerms(affinity.VMAffinity.RequiredDuringSchedulingPreferredDuringExecution)
		extractFromTerms(affinity.VMAffinity.PreferredDuringSchedulingPreferredDuringExecution)
	}

	// process VM anti-affinity rules
	if affinity.VMAntiAffinity != nil {
		extractFromTerms(affinity.VMAntiAffinity.RequiredDuringSchedulingPreferredDuringExecution)
		extractFromTerms(affinity.VMAntiAffinity.PreferredDuringSchedulingPreferredDuringExecution)
	}

	return affinityLabels
}

// genConfigSpecTagSpecsFromVMLabels generates tag specs from VM labels.
// This is required when setting affinity/anti-affinity policies on the VM.
func genConfigSpecTagSpecsFromVMLabels(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	filteredLabels := kubeutil.RemoveVMOperatorLabels(vmCtx.VM.Labels)
	if len(filteredLabels) == 0 {
		return
	}

	tagsToAdd := make([]vimtypes.TagSpec, 0, len(filteredLabels))

	policyLabels := sets.New[string]()
	if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution {
		policyLabels = extractAffinityLabelsFromVM(vmCtx)
	}

	// Any label on the VM can participate in an affinity/anti-affinity
	// policy.
	//
	// For when VMAffinityDuringExecution is enabled,
	// 	When VM is created, labels which only participate in affinity/anti-affinity
	// 	policies are added to the configSpec.TagSpecs.
	// 	This is because DRS expects tags which are ONLY referenced in
	// 	affinity/anti-affinity policies.
	//
	// TODO(for Day 2 operations):
	// 	When VM is updated and policies are added which refer more labels,
	// 	this method is called again and the tags are added to the
	// 	configSpec.TagSpecs.
	for key, value := range filteredLabels {
		if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution &&
			!policyLabels.Has(key+":"+value) {
			continue
		}

		tagsToAdd = append(tagsToAdd, vimtypes.TagSpec{
			ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
				Operation: vimtypes.ArrayUpdateOperationAdd,
			},
			Id: vimtypes.TagId{
				NameId: &vimtypes.TagIdNameId{
					Tag:      key + ":" + value,
					Category: vmCtx.VM.Namespace,
				},
			},
		})
	}

	configSpec.TagSpecs = append(configSpec.TagSpecs, tagsToAdd...)
}

// genConfigSpecAffinityPolicies generates placement policies from VM's
// affinity/anti-affinity rules.
func genConfigSpecAffinityPolicies(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	affinity := vmCtx.VM.Spec.Affinity
	if affinity == nil {
		return
	}

	var placementPols []vimtypes.BaseVmPlacementPolicy //nolint:prealloc

	if affinity.VMAffinity != nil {
		placementPols = append(placementPols, processVMAffinity(vmCtx, affinity.VMAffinity)...)
	}

	if affinity.VMAntiAffinity != nil {
		placementPols = append(placementPols, processVMAntiAffinity(vmCtx, affinity.VMAntiAffinity)...)
	}

	if len(placementPols) > 0 {
		configSpec.VmPlacementPolicies = placementPols
	}
}

// processVMAffinity returns placement policies for VM affinity rules.
// VM affinity is bidirectional, so we only need to send in the label specified
// in the VM affinity policy. Not additional labels.
func processVMAffinity(
	vmCtx pkgctx.VirtualMachineContext,
	affinity *vmopv1.VMAffinitySpec) []vimtypes.BaseVmPlacementPolicy {

	var placementPols []vimtypes.BaseVmPlacementPolicy //nolint:prealloc

	// Process required affinity terms associated with zone topology.
	requiredZoneTagIDs := buildTagIDsFromZoneTopology(
		vmCtx,
		affinity.RequiredDuringSchedulingPreferredDuringExecution,
	)
	for _, tagID := range requiredZoneTagIDs {
		placementPols = append(placementPols, &vimtypes.VmVmAffinity{
			AffinedVmsTag:    tagID,
			PolicyStrictness: string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:   string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process host-topology affinity terms only when VMAffinityDuringExecution is enabled.
	if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution {
		// Process required affinity terms associated with host topology.
		requiredHostTagIDs := buildTagIDsFromHostTopology(
			vmCtx,
			affinity.RequiredDuringSchedulingPreferredDuringExecution,
		)
		for _, tagID := range requiredHostTagIDs {
			placementPols = append(placementPols, &vimtypes.VmVmAffinity{
				AffinedVmsTag:    tagID,
				PolicyStrictness: string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
				PolicyTopology:   string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
			})
		}
	}

	// Process preferred affinity terms associated with zone topology.
	preferredZoneTagIDs := buildTagIDsFromZoneTopology(
		vmCtx,
		affinity.PreferredDuringSchedulingPreferredDuringExecution,
	)
	for _, tagID := range preferredZoneTagIDs {
		placementPols = append(placementPols, &vimtypes.VmVmAffinity{
			AffinedVmsTag:    tagID,
			PolicyStrictness: string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
			PolicyTopology:   string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process host-topology affinity terms only when VMAffinityDuringExecution is enabled.
	if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution {
		// Process preferred affinity terms associated with host topology.
		preferredHostTagIDs := buildTagIDsFromHostTopology(
			vmCtx,
			affinity.PreferredDuringSchedulingPreferredDuringExecution,
		)
		for _, tagID := range preferredHostTagIDs {
			placementPols = append(placementPols, &vimtypes.VmVmAffinity{
				AffinedVmsTag:    tagID,
				PolicyStrictness: string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
				PolicyTopology:   string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
			})
		}
	}

	return placementPols
}

// processVMAntiAffinity returns placement policies from VM anti-affinity rules.
// Use a single VmToVmGroupsAntiAffinity policy per topology/strictness
// combination if the labels are non-empty.
func processVMAntiAffinity(
	vmCtx pkgctx.VirtualMachineContext,
	antiAffinity *vmopv1.VMAntiAffinitySpec) []vimtypes.BaseVmPlacementPolicy {

	var placementPols []vimtypes.BaseVmPlacementPolicy //nolint:prealloc

	// Process required anti-affinity terms associated with zone topology.
	requiredZoneTagIDs := buildTagIDsFromZoneTopology(
		vmCtx,
		antiAffinity.RequiredDuringSchedulingPreferredDuringExecution,
	)
	if len(requiredZoneTagIDs) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: requiredZoneTagIDs,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process host-topology anti-affinity terms only when VMAffinityDuringExecution is enabled.
	if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution {
		requiredHostTagIDs := buildTagIDsFromHostTopology(
			vmCtx,
			antiAffinity.RequiredDuringSchedulingPreferredDuringExecution,
		)
		for _, tagID := range requiredHostTagIDs {
			placementPols = append(placementPols, &vimtypes.VmVmAntiAffinity{
				AntiAffinedVmsTag: tagID,
				PolicyStrictness:  string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
				PolicyTopology:    string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
			})
		}
	}

	// Process preferred anti-affinity terms associated with zone topology.
	preferredZoneTagIDs := buildTagIDsFromZoneTopology(
		vmCtx,
		antiAffinity.PreferredDuringSchedulingPreferredDuringExecution,
	)
	if len(preferredZoneTagIDs) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: preferredZoneTagIDs,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process host-topology anti-affinity terms only when VMAffinityDuringExecution is enabled.
	if pkgcfg.FromContext(vmCtx).Features.VMAffinityDuringExecution {
		preferredHostTagIDs := buildTagIDsFromHostTopology(
			vmCtx,
			antiAffinity.PreferredDuringSchedulingPreferredDuringExecution,
		)
		for _, tagID := range preferredHostTagIDs {
			placementPols = append(placementPols, &vimtypes.VmVmAntiAffinity{
				AntiAffinedVmsTag: tagID,
				PolicyStrictness:  string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
				PolicyTopology:    string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
			})
		}
	}

	return placementPols
}

// buildTagIDsFromTopology returns a list of TagIds built from the given
// affinity/anti-affinity terms that match the specified topology key.
// Terms with other topology types are ignored.
func buildTagIDsFromTopology(
	vmCtx pkgctx.VirtualMachineContext,
	terms []vmopv1.VMAffinityTerm,
	topologyKey string) []vimtypes.TagId {

	var tagIDs []vimtypes.TagId

	for _, term := range terms {
		if term.TopologyKey != topologyKey {
			continue
		}

		termLabels, err := extractLabelsFromSelector(term.LabelSelector)
		if err != nil {
			vmCtx.Logger.Error(err, "invalid label selector")
			continue
		}

		// Convert label strings (in "key:value" format) to TagId objects.
		for _, label := range termLabels {
			tagIDs = append(tagIDs, vimtypes.TagId{
				NameId: &vimtypes.TagIdNameId{
					Tag:      label,
					Category: vmCtx.VM.Namespace,
				},
			})
		}
	}

	return tagIDs
}

// buildTagIDsFromZoneTopology returns a list of TagIds built from the given
// affinity/anti-affinity terms that have zone topology.
func buildTagIDsFromZoneTopology(
	vmCtx pkgctx.VirtualMachineContext,
	terms []vmopv1.VMAffinityTerm) []vimtypes.TagId {

	return buildTagIDsFromTopology(vmCtx, terms, corev1.LabelTopologyZone)
}

// buildTagIDsFromHostTopology returns a list of TagIds built from the given
// affinity/anti-affinity terms that have host topology.
func buildTagIDsFromHostTopology(
	vmCtx pkgctx.VirtualMachineContext,
	terms []vmopv1.VMAffinityTerm) []vimtypes.TagId {

	return buildTagIDsFromTopology(vmCtx, terms, corev1.LabelHostname)
}

// extractLabelsFromSelector extracts all labels from a LabelSelector, handling both
// MatchLabels and MatchExpressions with "In" operator (supporting multiple values).
// Returns a slice of formatted labels in "key:value" format.
// If any operation other than "In" is specified in the MatchExpressions, we return an error.
// We don't de-duplicate labels since DRS handles it on the backend.
func extractLabelsFromSelector(selector *metav1.LabelSelector) ([]string, error) {
	if selector == nil {
		return nil, nil
	}

	labels := make([]string, 0)

	// Handle MatchLabels - direct key-value pairs.
	for key, value := range selector.MatchLabels {
		label := fmt.Sprintf("%s:%s", key, value)
		labels = append(labels, label)
	}

	// Handle MatchExpressions - only support "In" operator with AND logic.
	//
	// Note: Similar to Kubernetes, we expect the user to know
	// what they are doing when specifying labels. So, we don't
	// enforce unique keys across labels and expressions.
	for _, expr := range selector.MatchExpressions {
		switch expr.Operator {
		case metav1.LabelSelectorOpIn:
			// For "In" operator, create a label for each value
			for _, value := range expr.Values {
				label := fmt.Sprintf("%s:%s", expr.Key, value)
				labels = append(labels, label)
			}
		default:
			// We only support "In" operator for now as specified in requirements
			return nil, fmt.Errorf("unsupported MatchExpression operator %q, only 'In' is supported",
				expr.Operator)
		}
	}

	// Sort the labels to maintain consistent ordering.
	slices.Sort(labels)

	return labels, nil
}
