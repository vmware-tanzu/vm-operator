// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"slices"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"k8s.io/apimachinery/pkg/api/validate/content"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// AffinityPolicyType identifies a policy that has a derived tag identity.
type AffinityPolicyType string

const (
	// AffinityPolicyTypeAffinity identifies a single-pair VM affinity policy.
	AffinityPolicyTypeAffinity AffinityPolicyType = "VmVmAffinity"
	// AffinityPolicyTypeGroupedAntiAffinity identifies a grouped anti-affinity policy.
	AffinityPolicyTypeGroupedAntiAffinity AffinityPolicyType = "VmToVmGroupsAntiAffinity"
)

// AffinityPolicyHash contains the canonical inputs, hash, and vc tag name for
// a derived affinity tag. Namespace supplies the tag category separately; neither
// namespace nor VM identity contributes to the policy hash. Changing policy
// inputs requires calculating a new hash.
type AffinityPolicyHash struct {
	Type       AffinityPolicyType
	Strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness
	Topology   vimtypes.VmPlacementPolicyVmPlacementPolicyTopology
	Targets    []LabelPair
	Hash       string
	TagName    string
}

// NewAffinityPolicyHash canonicalizes resolved selector pairs and derives
// the policy hash and backend tag name.
// It rejects unsupported policy parameters, malformed labels, and label keys
// in the reserved VM Operator domain. The caller
// remains responsible for feature, topology eligibility, and namespace
// authorization checks. The returned targets do not alias the input slice.
func NewAffinityPolicyHash(
	policyType AffinityPolicyType,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
	targets []LabelPair) (AffinityPolicyHash, error) {

	if policyType != AffinityPolicyTypeAffinity && policyType != AffinityPolicyTypeGroupedAntiAffinity {
		return AffinityPolicyHash{}, fmt.Errorf("unsupported affinity policy type %q", policyType)
	}

	var strictnessName string
	switch strictness {
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution:
		strictnessName = "pds-pde"
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution:
		strictnessName = "rds-pde"
	case vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution:
		strictnessName = "rds-rde"
	default:
		return AffinityPolicyHash{}, fmt.Errorf("unsupported affinity policy strictness %q", strictness)
	}

	var topologyName string
	switch topology {
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost:
		topologyName = "host"
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone:
		topologyName = "zone"
	default:
		return AffinityPolicyHash{}, fmt.Errorf("unsupported affinity policy topology %q", topology)
	}

	if len(targets) == 0 {
		return AffinityPolicyHash{}, fmt.Errorf("affinity policy requires at least one target")
	}

	for _, pair := range targets {
		if errs := content.IsLabelKey(pair.Key); len(errs) != 0 {
			return AffinityPolicyHash{}, fmt.Errorf("invalid affinity target key %q: %s", pair.Key, strings.Join(errs, "; "))
		}
		if errs := content.IsLabelValue(pair.Value); len(errs) != 0 {
			return AffinityPolicyHash{}, fmt.Errorf("invalid affinity target value %q: %s", pair.Value, strings.Join(errs, "; "))
		}
		if kubeutil.HasVMOperatorLabels(map[string]string{pair.Key: pair.Value}) {
			return AffinityPolicyHash{}, fmt.Errorf("reserved affinity target key %q", pair.Key)
		}
	}

	targets = slices.Clone(targets)
	slices.SortFunc(targets, func(a, b LabelPair) int {
		return strings.Compare(TagName(a.Key, a.Value), TagName(b.Key, b.Value))
	})
	targets = slices.Compact(targets)
	if policyType == AffinityPolicyTypeAffinity && len(targets) != 1 {
		return AffinityPolicyHash{}, fmt.Errorf("VM affinity policy requires one distinct target, got %d", len(targets))
	}

	var input strings.Builder
	for _, field := range []string{string(policyType), string(strictness), string(topology)} {
		_, _ = fmt.Fprintf(&input, "%d:%s", len(field), field)
	}
	for _, pair := range targets {
		// Hash the label pair independently of the backend tag-name prefix.
		field := pair.Key + ":" + pair.Value
		_, _ = fmt.Fprintf(&input, "%d:%s", len(field), field)
	}

	hash := pkgutil.XXHash64Hex(input.String())
	name := "aaf-" + strictnessName + "-" + topologyName + "-" + hash
	if policyType == AffinityPolicyTypeAffinity {
		name = "af-" + strictnessName + "-" + topologyName + "-" + hash + "-" + targets[0].Key + ":" + targets[0].Value
	}

	return AffinityPolicyHash{
		Type: policyType, Strictness: strictness, Topology: topology,
		Targets: targets, Hash: hash, TagName: affinityTagName(name),
	}, nil
}
