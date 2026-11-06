// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// AffinityRuleConstraints defines constraints for configuring for affinity configuration at the topology level.
// The flags are configured based on certain VM labels & the nature of affinity config: placement time vs create time.
type AffinityRuleConstraints struct {
	ConfigureHostRules bool
	ConfigureZoneRules bool

	// ConfigureRequiredDuringExecution is true when terms that are required
	// during execution should be configured. It equals the
	// VMHardAffinityDuringExecution capability.
	ConfigureRequiredDuringExecution bool

	// ConfigureHashedTags is true when policy-hash tags are generated and
	// attached, and generated policies are canonicalized. It equals the
	// VMHardAffinityDuringExecution capability.
	ConfigureHashedTags bool

	// ConfigureTagPrefix is true when original tag names carry the
	// vmoperator: prefix instead of the bare key:value form. It equals the
	// VMHardAffinityDuringExecution capability.
	ConfigureTagPrefix bool
}

// ExtraConfigPlacementPoliciesKey records generated affinity policies separately
// from the managed attachments in ExtraConfigVMTagsKey.
const ExtraConfigPlacementPoliciesKey = "vmservice.placementpolicies"

// AddAffinityPoliciesToConfigSpec adds eligible affinity policies and tags to
// configSpec. isPlacement selects placement (true) or creation (false). Creation
// records generated policies and managed tags; placement preserves those receipts.
// Generated policies replace the existing policy list, including class policies,
// as in the legacy path. Without generated policies the existing list is retained.
func AddAffinityPoliciesToConfigSpec(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	isPlacement bool) error {

	constraints := CalculateAffinityConstraints(ctx, vm, isPlacement)
	policies, tags, err := generateAffinityPolicies(ctx, vm, isPlacement, constraints)
	if err != nil {
		return err
	}
	if len(policies) != 0 {
		configSpec.VmPlacementPolicies = policies
	}
	if !constraints.ConfigureHashedTags {
		for _, name := range tags {
			configSpec.TagSpecs = append(configSpec.TagSpecs, vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
				Id:              affinityNameID(vm.Namespace, name),
			})
		}
		return nil
	}

	setAffinityTags(vm, configSpec, tags)
	if isPlacement {
		return nil
	}
	receiptTags := slices.Concat(tags, implicitAffinityTags(policies))
	return recordCreateAffinityState(configSpec, policies, normalizeTagNames(receiptTags))
}

// UpdateConfigSpecAffinity reconciles local mutable policies and attachments in
// one configuration task. Feature disablement does not detach existing state.
func UpdateConfigSpecAffinity(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	spec *vimtypes.VirtualMachineConfigSpec) error {

	features := pkgcfg.FromContext(ctx).Features
	if !features.VMHardAffinityDuringExecution {
		// Policies are immutable if VMHardAffinityDuringExecution is false
		return nil
	}

	constraints := CalculateAffinityConstraints(ctx, vm, false)
	policies, tags, err := generateAffinityPolicies(ctx, vm, false, constraints)
	if err != nil {
		return err
	}
	if err := reconcileAffinityPolicies(config, spec, policies); err != nil {
		return err
	}
	ReconcileVMTags(ctx, vm, config, spec, tags, implicitAffinityTags(policies))
	return nil
}

// CalculateAffinityConstraints calculates the constraints based on the VM
// and the workflow. isPlacement is true for placement and false for
// creation or update.
//
// When VMHardAffinityDuringExecution is enabled, ConfigureZoneRules equals
// the VMPlacementPolicies capability and ConfigureHostRules equals the
// VMAffinityDuringExecution capability, with no other exclusions.
//
// Otherwise, ConfigureZoneRules is true when the VMPlacementPolicies
// capability is enabled, except:
//   - It is disabled during VM Create configspec creation, as DRS only
//     evaluates zonal policies during placement. Therefore, when the VM is
//     being created, we want to omit zonal policies from the ConfigSpec.
//   - It is disabled for a VKS node VM (CAPI labels) with a zone label set,
//     i.e. an explicit zone is already specified by the VM.
//   - It is disabled when VMAffinityDuringExecution is enabled and the VM has
//     a zone label set, i.e. an explicit zone is already specified by the VM.
//
// ConfigureHostRules is true when the VMAffinityDuringExecution capability
// is enabled, except for a VKS node VM (CAPI labels), since its host
// anti-affinity is handled via cluster modules instead.
func CalculateAffinityConstraints(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	isPlacement bool,
) AffinityRuleConstraints {
	features := pkgcfg.FromContext(ctx).Features
	constraints := AffinityRuleConstraints{}

	// set base condition for zonal rules based on capability.
	if features.VMPlacementPolicies {
		constraints.ConfigureZoneRules = true
	}

	// set base condition for host rules based on capability.
	if features.VMAffinityDuringExecution {
		constraints.ConfigureHostRules = true
	}

	// When VMHardAffinityDuringExecution is enabled, the capability checks
	// above are the only constraints: zone rules are valid on create, host
	// rules apply to VKS node VMs, and zone labels do not exclude zone rules.
	constraints.ConfigureRequiredDuringExecution = features.VMHardAffinityDuringExecution
	constraints.ConfigureHashedTags = features.VMHardAffinityDuringExecution
	constraints.ConfigureTagPrefix = features.VMHardAffinityDuringExecution
	if features.VMHardAffinityDuringExecution {
		return constraints
	}

	// Host anti-affinity for CAPI VMs is handled via cluster module.
	isVksNodeVM := kubeutil.HasCAPILabels(vm.Labels)
	if isVksNodeVM {
		constraints.ConfigureHostRules = false
	}

	// Zonal policies are only supported on placement.
	if !isPlacement {
		constraints.ConfigureZoneRules = false
	}

	// For VKS nodes with zone labels or failure domains set, or for VMs with
	// a zone label when VMAffinityDuringExecution is enabled, do not configure
	// zonal affinity rules. Zones are assigned from the zone label.
	hasZoneLabel := kubeutil.HasZoneLabel(vm.Labels)
	if hasZoneLabel && (isVksNodeVM || features.VMAffinityDuringExecution) {
		constraints.ConfigureZoneRules = false
	}

	return constraints
}

// implicitAffinityTags returns the target tag of each AF policy. vCenter
// attaches it to the VM along with the policy, even when the VM does not carry
// the source label, so the tag must be recorded in the receipt. Detaching it
// later is the only way to remove the policy. It is never attached by VM
// Operator, which would make the VM a member of the target group.
//
// Placement policies target the original tag names, not these, so they are
// never recorded and this must not be called for them.
func implicitAffinityTags(policies []vimtypes.BaseVmPlacementPolicy) []string {
	var names []string
	for _, policy := range policies {
		if af, ok := policy.(*vimtypes.VmVmAffinity); ok && af.AffinedVmsTag.NameId != nil {
			names = append(names, af.AffinedVmsTag.NameId.Tag)
		}
	}
	return names
}

// generateAffinityPolicies resolves the spec's tags before constructing policies.
// Mutable affinity changes tag calculation and enables hard execution terms;
// policy constructors receive only final tags, strictness, and topology.
func generateAffinityPolicies(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	isPlacement bool,
	constraints AffinityRuleConstraints) ([]vimtypes.BaseVmPlacementPolicy, []string, error) {

	affinity := vm.Spec.Affinity
	if affinity == nil {
		return nil, nil, nil
	}
	var policies []vimtypes.BaseVmPlacementPolicy
	var tags []string

	addAffinity := func(terms []vmopv1.VMAffinityTerm,
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) error {

		pairs := affinityTargetsForTopology(ctx, terms, topology)
		targets, attachments, err := calculateAffinityTags(vm, pairs, strictness, topology, isPlacement, constraints)
		if err != nil {
			return err
		}
		for _, target := range targets {
			policies = append(policies, newAffinityPolicy(target, strictness, topology))
		}
		tags = append(tags, attachments...)
		return nil
	}
	addAntiAffinity := func(terms []vmopv1.VMAffinityTerm,
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) error {

		pairs := affinityTargetsForTopology(ctx, terms, topology)
		targets, self, attachments, err := calculateAntiAffinityTags(vm, pairs, strictness, topology, constraints)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return nil
		}
		if constraints.ConfigureHashedTags || topology == vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone {
			policies = append(policies, newGroupedAntiAffinityPolicy(self, targets, strictness, topology))
		} else {
			for _, target := range targets {
				policies = append(policies, newAntiAffinityPolicy(target, strictness, topology))
			}
		}
		tags = append(tags, attachments...)
		return nil
	}

	// addTopology adds the affinity and anti-affinity policies for a topology.
	// Terms required during execution are only added when the constraints allow.
	addTopology := func(topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) error {
		type termSet struct {
			strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness
			affinity   []vmopv1.VMAffinityTerm
			anti       []vmopv1.VMAffinityTerm
		}
		var af vmopv1.VMAffinitySpec
		var aaf vmopv1.VMAntiAffinitySpec
		if affinity.VMAffinity != nil {
			af = *affinity.VMAffinity
		}
		if affinity.VMAntiAffinity != nil {
			aaf = *affinity.VMAntiAffinity
		}
		sets := []termSet{
			{
				strictness: vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
				affinity:   af.PreferredDuringSchedulingPreferredDuringExecution,
				anti:       aaf.PreferredDuringSchedulingPreferredDuringExecution,
			},
			{
				strictness: vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementPreferredDuringExecution,
				affinity:   af.RequiredDuringSchedulingPreferredDuringExecution,
				anti:       aaf.RequiredDuringSchedulingPreferredDuringExecution,
			},
		}
		if constraints.ConfigureRequiredDuringExecution {
			sets = append(sets, termSet{
				strictness: vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution,
				affinity:   af.RequiredDuringSchedulingRequiredDuringExecution,
				anti:       aaf.RequiredDuringSchedulingRequiredDuringExecution,
			})
		}
		for _, s := range sets {
			if err := addAffinity(s.affinity, s.strictness, topology); err != nil {
				return err
			}
		}
		for _, s := range sets {
			if err := addAntiAffinity(s.anti, s.strictness, topology); err != nil {
				return err
			}
		}
		return nil
	}

	if constraints.ConfigureZoneRules {
		if err := addTopology(
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone); err != nil {
			return nil, nil, err
		}
	}
	if constraints.ConfigureHostRules {
		if err := addTopology(
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost); err != nil {
			return nil, nil, err
		}
	}
	if constraints.ConfigureHashedTags {
		var err error
		policies, err = normalizeAffinityPolicies(policies)
		if err != nil {
			return nil, nil, err
		}
	}
	return policies, normalizeTagNames(tags), nil
}

// calculateAffinityTags selects the original or mutable AF tag calculation.
func calculateAffinityTags(
	vm *vmopv1.VirtualMachine,
	pairs []LabelPair,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
	isPlacement bool,
	constraints AffinityRuleConstraints) ([]vimtypes.TagId, []string, error) {

	if constraints.ConfigureHashedTags {
		return calculateHashedAffinityTags(vm, pairs, strictness, topology, isPlacement)
	}

	targets, tags := calculateOriginalAffinityTags(vm, pairs, constraints.ConfigureTagPrefix)
	return targets, tags, nil
}

// calculateOriginalAffinityTags returns original target tags and the subset
// whose source labels this VM carries. Mutable affinity prefixes these names;
// the legacy path retains bare key:value names for both AF and AAF.
func calculateOriginalAffinityTags(vm *vmopv1.VirtualMachine, pairs []LabelPair, useTagPrefix bool) ([]vimtypes.TagId, []string) {
	labels := kubeutil.RemoveVMOperatorLabels(vm.Labels)
	targets := make([]vimtypes.TagId, 0, len(pairs))
	var tags []string
	for _, pair := range pairs {
		original := pair.Key + ":" + pair.Value
		if useTagPrefix {
			original = TagName(pair.Key, pair.Value)
		}
		targets = append(targets, affinityNameID(vm.Namespace, original))
		if value, exists := labels[pair.Key]; exists && value == pair.Value {
			tags = append(tags, original)
		}
	}
	return targets, tags
}

// calculateHashedAffinityTags derives AF hashes and local membership.
// Placement targets the original names; both create and placement attach the
// original and hashed tags when this VM carries the source label.
func calculateHashedAffinityTags(
	vm *vmopv1.VirtualMachine,
	pairs []LabelPair,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
	isPlacement bool) ([]vimtypes.TagId, []string, error) {

	labels := kubeutil.RemoveVMOperatorLabels(vm.Labels)
	var targets []vimtypes.TagId
	var tags []string
	for _, pair := range pairs {
		policyHash, err := NewAffinityPolicyHash(AffinityPolicyTypeAffinity, strictness, topology, []LabelPair{pair})
		if err != nil {
			return nil, nil, fmt.Errorf("calculate affinity tag: %w", err)
		}
		original := TagName(pair.Key, pair.Value)
		target := policyHash.TagName
		if isPlacement {
			target = original
		}
		targets = append(targets, affinityNameID(vm.Namespace, target))
		if value, exists := labels[pair.Key]; exists && value == pair.Value {
			tags = append(tags, original, policyHash.TagName)
		}
	}
	return targets, tags, nil
}

// calculateAntiAffinityTags selects original targets with either an empty self
// tag for the legacy path or a hashed self tag for mutable AAF.
func calculateAntiAffinityTags(
	vm *vmopv1.VirtualMachine,
	pairs []LabelPair,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology,
	constraints AffinityRuleConstraints) ([]vimtypes.TagId, vimtypes.TagId, []string, error) {

	if constraints.ConfigureHashedTags {
		return calculateHashedAntiAffinityTags(vm, pairs, strictness, topology)
	}
	targets, tags := calculateOriginalAffinityTags(vm, pairs, constraints.ConfigureTagPrefix)
	return targets, vimtypes.TagId{}, tags, nil
}

// calculateHashedAntiAffinityTags combines canonical original targets with a
// hashed self tag that is always attached to the requesting VM.
func calculateHashedAntiAffinityTags(
	vm *vmopv1.VirtualMachine,
	pairs []LabelPair,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) ([]vimtypes.TagId, vimtypes.TagId, []string, error) {

	if len(pairs) == 0 {
		return nil, vimtypes.TagId{}, nil, nil
	}
	policyHash, err := NewAffinityPolicyHash(AffinityPolicyTypeGroupedAntiAffinity, strictness, topology, pairs)
	if err != nil {
		return nil, vimtypes.TagId{}, nil, fmt.Errorf("calculate anti-affinity tags: %w", err)
	}
	targets, tags := calculateOriginalAffinityTags(vm, policyHash.Targets, true)
	self := affinityNameID(vm.Namespace, policyHash.TagName)
	tags = append(tags, policyHash.TagName)
	return targets, self, tags, nil
}

func newAffinityPolicy(target vimtypes.TagId,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) *vimtypes.VmVmAffinity {

	return &vimtypes.VmVmAffinity{
		AffinedVmsTag:    target,
		PolicyStrictness: string(strictness),
		PolicyTopology:   string(topology),
	}
}

func newAntiAffinityPolicy(target vimtypes.TagId,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) *vimtypes.VmVmAntiAffinity {

	return &vimtypes.VmVmAntiAffinity{
		AntiAffinedVmsTag: target,
		PolicyStrictness:  string(strictness),
		PolicyTopology:    string(topology),
	}
}

func newGroupedAntiAffinityPolicy(self vimtypes.TagId, targets []vimtypes.TagId,
	strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) *vimtypes.VmToVmGroupsAntiAffinity {

	return &vimtypes.VmToVmGroupsAntiAffinity{
		SelfTag:                self,
		AntiAffinedVmGroupTags: targets,
		PolicyStrictness:       string(strictness),
		PolicyTopology:         string(topology),
	}
}

func affinityNameID(namespace, name string) vimtypes.TagId {
	return vimtypes.TagId{NameId: &vimtypes.TagIdNameId{Tag: name, Category: namespace}}
}

// affinityTargetsForTopology resolves selectors only for terms with this topology.
func affinityTargetsForTopology(ctx context.Context, terms []vmopv1.VMAffinityTerm,
	topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) []LabelPair {

	var topologyKey string
	switch topology {
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost:
		topologyKey = corev1.LabelHostname
	case vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone:
		topologyKey = corev1.LabelTopologyZone
	default:
		return nil
	}
	var targets []LabelPair
	for _, term := range terms {
		if term.TopologyKey != topologyKey {
			continue
		}
		pairs, err := extractLabelPairsFromSelector(term.LabelSelector)
		if err != nil {
			pkglog.FromContextOrDefault(ctx).V(4).Error(err, "invalid label selector")
			continue
		}
		targets = append(targets, pairs...)
	}
	return targets
}

// extractLabelPairsFromSelector resolves MatchLabels and In expressions without
// deduplicating pairs. An unsupported operator invalidates the entire selector.
func extractLabelPairsFromSelector(selector *metav1.LabelSelector) ([]LabelPair, error) {
	if selector == nil {
		return nil, nil
	}

	var pairs []LabelPair
	for key, value := range selector.MatchLabels {
		pairs = append(pairs, LabelPair{Key: key, Value: value})
	}
	for _, expr := range selector.MatchExpressions {
		if expr.Operator != metav1.LabelSelectorOpIn {
			return nil, fmt.Errorf("unsupported MatchExpression operator %q, only 'In' is supported", expr.Operator)
		}
		for _, value := range expr.Values {
			pairs = append(pairs, LabelPair{Key: expr.Key, Value: value})
		}
	}
	slices.SortFunc(pairs, func(a, b LabelPair) int {
		return strings.Compare(TagName(a.Key, a.Value), TagName(b.Key, b.Value))
	})
	return pairs, nil
}

// recordCreateAffinityState records generated policies and actual managed tag
// additions. Class policies remain outside the builder's ownership receipt.
func recordCreateAffinityState(spec *vimtypes.VirtualMachineConfigSpec,
	policies []vimtypes.BaseVmPlacementPolicy, tags []string) error {

	record, err := affinityPolicyRecord(policies)
	if err != nil {
		return err
	}
	if old, _ := object.OptionValueList(spec.ExtraConfig).GetString(ExtraConfigPlacementPoliciesKey); len(policies) != 0 || old != "" {
		spec.ExtraConfig = pkgutil.OptionValues(spec.ExtraConfig).Merge(
			&vimtypes.OptionValue{Key: ExtraConfigPlacementPoliciesKey, Value: record},
		)
	}
	if old, _ := object.OptionValueList(spec.ExtraConfig).GetString(ExtraConfigVMTagsKey); len(tags) != 0 || old != "" {
		spec.ExtraConfig = pkgutil.OptionValues(spec.ExtraConfig).Merge(
			&vimtypes.OptionValue{Key: ExtraConfigVMTagsKey, Value: strings.Join(tags, ",")},
		)
	}
	return nil
}

// setAffinityTags replaces outgoing additions named in ExtraConfigVMTagsKey with
// the requested tags, retaining unrelated tag operations. Placement inherits the
// create configuration and its receipt, so this removes create-time managed
// additions before adding placement tags. It emits no detach operations;
// ReconcileVMTags removes persisted attachments during updates and cleanup.
func setAffinityTags(vm *vmopv1.VirtualMachine, spec *vimtypes.VirtualMachineConfigSpec, tags []string) {
	previous := sets.New(RecordedVMTags(&vimtypes.VirtualMachineConfigInfo{ExtraConfig: spec.ExtraConfig})...)
	added := sets.New[string]()
	var tagSpecs []vimtypes.TagSpec
	for _, tag := range spec.TagSpecs {
		if tag.Operation == vimtypes.ArrayUpdateOperationAdd && tag.Id.NameId != nil &&
			tag.Id.NameId.Category == vm.Namespace {
			if previous.Has(tag.Id.NameId.Tag) {
				continue
			}
			added.Insert(tag.Id.NameId.Tag)
		}
		tagSpecs = append(tagSpecs, tag)
	}
	for _, name := range tags {
		if added.Has(name) {
			continue
		}
		tagSpecs = append(tagSpecs, vimtypes.TagSpec{
			ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
			Id:              affinityNameID(vm.Namespace, name),
		})
		added.Insert(name)
	}
	spec.TagSpecs = tagSpecs
}

// normalizeAffinityPolicies sorts policies by their serialized form and drops
// duplicates. The deterministic result keeps the recorded policy snapshot
// stable regardless of term order in the VM spec.
func normalizeAffinityPolicies(policies []vimtypes.BaseVmPlacementPolicy) ([]vimtypes.BaseVmPlacementPolicy, error) {
	type entry struct {
		key    string
		policy vimtypes.BaseVmPlacementPolicy
	}
	entries := make([]entry, 0, len(policies))
	for _, policy := range policies {
		data, err := json.Marshal(policy)
		if err != nil {
			return nil, fmt.Errorf("marshal affinity policy: %w", err)
		}
		entries = append(entries, entry{string(data), policy})
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.key, b.key) })
	entries = slices.CompactFunc(entries, func(a, b entry) bool { return a.key == b.key })
	var result []vimtypes.BaseVmPlacementPolicy
	for _, entry := range entries {
		result = append(result, entry.policy)
	}
	return result, nil
}

func affinityPolicyRecord(policies []vimtypes.BaseVmPlacementPolicy) (string, error) {
	data, err := json.Marshal(struct {
		Policies []vimtypes.BaseVmPlacementPolicy `json:"policies,omitempty"`
	}{Policies: policies})
	if err != nil {
		return "", fmt.Errorf("marshal affinity policy record: %w", err)
	}
	return string(data), nil
}

// reconcileAffinityPolicies compares a deterministic policy-only snapshot and
// emits changed policies and their receipt into the same configuration task.
// It never reads or writes tag attachments; the complete-set writer owns those.
// VmPlacementPolicies specifies additional policies, not a replacement of the
// VM's existing compute-policy associations. Preserve other outgoing policies
// while recording only the generated policies owned by this reconciler.
// Clearing policies records an empty snapshot; backend policy garbage collection
// follows attachment removal rather than an explicit policy delete operation.
func reconcileAffinityPolicies(
	config *vimtypes.VirtualMachineConfigInfo, spec *vimtypes.VirtualMachineConfigSpec,
	desired []vimtypes.BaseVmPlacementPolicy) error {

	policies, err := normalizeAffinityPolicies(desired)
	if err != nil {
		return err
	}
	record, err := affinityPolicyRecord(policies)
	if err != nil {
		return err
	}
	var old string
	if config != nil {
		old, _ = object.OptionValueList(config.ExtraConfig).GetString(ExtraConfigPlacementPoliciesKey)
	}
	if old == record || old == "" && len(policies) == 0 {
		return nil
	}
	spec.VmPlacementPolicies = append(slices.Clone(spec.VmPlacementPolicies), policies...)
	spec.ExtraConfig = pkgutil.OptionValues(spec.ExtraConfig).Merge(
		&vimtypes.OptionValue{Key: ExtraConfigPlacementPoliciesKey, Value: record},
	)
	return nil
}
