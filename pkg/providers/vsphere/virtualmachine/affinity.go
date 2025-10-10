// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"slices"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

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

	// Any label on the VM can participate in an affinity/anti-affinity policy.
	// It does not matter if a label is not participating in any policy.
	// It could be specified by this, or any other VM's placement policy later.
	for key, value := range filteredLabels {
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

	// Sort the tags to maintain consistent ordering.
	slices.SortFunc(tagsToAdd, func(a, b vimtypes.TagSpec) int {
		return strings.Compare(a.Id.NameId.Tag, b.Id.NameId.Tag)
	})

	configSpec.TagSpecs = tagsToAdd
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
	requiredZoneLabels := extractTermLabelsByTopology(
		vmCtx,
		affinity.RequiredDuringSchedulingPreferredDuringExecution,
		corev1.LabelTopologyZone,
	)
	for _, label := range requiredZoneLabels {
		placementPols = append(placementPols, &vimtypes.VmVmAffinity{
			AffinedVmsTagName: label,
			PolicyStrictness:  string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:    string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process preferred affinity terms associated with zone topology.
	preferredZoneLabels := extractTermLabelsByTopology(
		vmCtx,
		affinity.PreferredDuringSchedulingPreferredDuringExecution,
		corev1.LabelTopologyZone,
	)
	for _, label := range preferredZoneLabels {
		placementPols = append(placementPols, &vimtypes.VmVmAffinity{
			AffinedVmsTagName: label,
			PolicyStrictness:  string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
			PolicyTopology:    string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	return placementPols
}

// processVMAntiAffinity returns placement policies from VM anti-affinity rules.
// Use a single VmToVmGroupsAntiAffinity policy if the labels are non-empty.
func processVMAntiAffinity(
	vmCtx pkgctx.VirtualMachineContext,
	antiAffinity *vmopv1.VMAntiAffinitySpec) []vimtypes.BaseVmPlacementPolicy {

	var placementPols []vimtypes.BaseVmPlacementPolicy //nolint:prealloc

	// Process required anti-affinity terms associated with zone topology.
	requiredZoneLabels := extractTermLabelsByTopology(
		vmCtx,
		antiAffinity.RequiredDuringSchedulingPreferredDuringExecution,
		corev1.LabelTopologyZone,
	)
	if len(requiredZoneLabels) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: requiredZoneLabels,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process required anti-affinity terms associated with host topology.
	requiredHostLabels := extractTermLabelsByTopology(
		vmCtx,
		antiAffinity.RequiredDuringSchedulingPreferredDuringExecution,
		corev1.LabelHostname,
	)
	if len(requiredHostLabels) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: requiredHostLabels,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
		})
	}

	// Process preferred anti-affinity terms associated with zone topology.
	preferredZoneLabels := extractTermLabelsByTopology(
		vmCtx,
		antiAffinity.PreferredDuringSchedulingPreferredDuringExecution,
		corev1.LabelTopologyZone,
	)
	if len(preferredZoneLabels) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: preferredZoneLabels,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone),
		})
	}

	// Process preferred anti-affinity terms associated with host topology.
	preferredHostLabels := extractTermLabelsByTopology(
		vmCtx,
		antiAffinity.PreferredDuringSchedulingPreferredDuringExecution,
		corev1.LabelHostname,
	)
	if len(preferredHostLabels) > 0 {
		placementPols = append(placementPols, &vimtypes.VmToVmGroupsAntiAffinity{
			AntiAffinedVmGroupTags: preferredHostLabels,
			PolicyStrictness:       string(vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution),
			PolicyTopology:         string(vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost),
		})
	}

	return placementPols
}

// extractTermLabelsByTopology returns a list of labels extracted from VM's
// affinity/anti-affinity terms with the specified topology key.
func extractTermLabelsByTopology(
	vmCtx pkgctx.VirtualMachineContext,
	terms []vmopv1.VMAffinityTerm,
	topologyKey string) []string {

	var labels []string

	for _, term := range terms {
		if term.TopologyKey != topologyKey {
			continue
		}

		termLabels, err := extractLabelsFromSelector(term.LabelSelector)
		if err != nil {
			vmCtx.Logger.Error(err, "invalid label selector")
			continue
		}

		labels = append(labels, termLabels...)
	}

	return labels
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
