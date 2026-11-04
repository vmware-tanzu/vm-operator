// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"k8s.io/apimachinery/pkg/api/validate/content"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// ExtraConfigVMTagsKey records managed tag names in the VM namespace category.
const ExtraConfigVMTagsKey = "vmservice.placementpolicies.tags"

// AffinityTagPrefix identifies tags generated for VM Operator affinity policies.
const AffinityTagPrefix = "vmoperator:"

// LabelPair is a resolved affinity selector key/value pair.
type LabelPair struct{ Key, Value string }

// TagName returns the prefixed original tag name for a label pair.
// To recover the pair, remove AffinityTagPrefix before splitting on the colon.
func TagName(key, value string) string { return affinityTagName(key + ":" + value) }

// affinityTagName applies the shared prefix to an original or hashed tag name.
func affinityTagName(name string) string { return AffinityTagPrefix + name }

// StripAffinityTagPrefix returns the vCenter tag name without the
// AffinityTagPrefix. A name without the prefix is returned unchanged.
func StripAffinityTagPrefix(vcName string) string {
	return strings.TrimPrefix(vcName, AffinityTagPrefix)
}

// AddAffinityTagPrefix returns the vCenter tag name for a prefix-free name.
func AddAffinityTagPrefix(name string) string {
	return affinityTagName(name)
}

// PrefixFreeTagName returns the prefix-free original tag name for a label
// pair, ie "<key>:<value>". It is also the Tag source-pair index key.
func PrefixFreeTagName(key, value string) string {
	return kubeutil.TagLabelKeyValue(key, value)
}

// PrefixFreePolicyTagName returns the prefix-free vc tag name of a
// policy hash: the self tag of a grouped anti-affinity policy or the hashed
// tag of a VM affinity policy.
func PrefixFreePolicyTagName(h AffinityPolicyHash) string {
	return StripAffinityTagPrefix(h.TagName)
}

// PrefixFreeGroupedAntiAffinityTargetName returns the prefix-free name of the
// derived target tag of a grouped anti-affinity policy for the label pair. It
// is the prefix-free self tag name followed by "-<key>:<value>". It never
// equals the self tag name because a label pair always contains a colon.
func PrefixFreeGroupedAntiAffinityTargetName(
	h AffinityPolicyHash, pair LabelPair) string {

	return PrefixFreePolicyTagName(h) + "-" + PrefixFreeTagName(pair.Key, pair.Value)
}

// GroupedAntiAffinityTargetTagName returns the vCenter name of the derived
// target tag of a grouped anti-affinity policy for the label pair.
func GroupedAntiAffinityTargetTagName(
	h AffinityPolicyHash, pair LabelPair) string {

	return AddAffinityTagPrefix(PrefixFreeGroupedAntiAffinityTargetName(h, pair))
}

// ConcreteTagResourceName returns the name of the Tag resource for the
// prefix-free tag name in the namespace: "tag-" followed by the 16 hex
// character XXHash64 digest of "<namespace>:<name>".
func ConcreteTagResourceName(namespace, name string) string {
	return "tag-" + pkgutil.XXHash64Hex(namespace+":"+name)
}

// GetVMTagsFromExtraConfig returns the applied name-based tag set. An absent or empty
// record means no recorded associations. Only this receipt identifies managed
// attachments; unrelated configuration is not used to infer ownership.
func GetVMTagsFromExtraConfig(config *vimtypes.VirtualMachineConfigInfo) []string {
	if config == nil {
		return nil
	}
	value, _ := object.OptionValueList(config.ExtraConfig).GetString(ExtraConfigVMTagsKey)
	if value == "" {
		return nil
	}
	return normalizeTagNames(strings.Split(value, ","))
}

func normalizeTagNames(tags []string) []string {
	result := slices.Clone(tags)
	result = slices.DeleteFunc(result, func(name string) bool { return name == "" })
	slices.Sort(result)
	return slices.Compact(result)
}

// ReconcileVMTagsDuringVMUpdate emits the diff between the complete desired managed tag set
// and the previously applied record. Call exactly once with the complete set;
// namespace-wide label-derived and local policy producers must contribute to
// that set before calling this writer. Unrelated outgoing operations are preserved.
// The implicit tags are recorded and detached like the others, but never
// attached, because vCenter attaches them itself.
func ReconcileVMTagsDuringVMUpdate(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	attach, implicit []string) {

	attach = normalizeTagNames(attach)
	desired := normalizeTagNames(append(slices.Clone(attach), implicit...))
	previous := GetVMTagsFromExtraConfig(config)
	if slices.Equal(previous, desired) {
		return
	}
	emit := func(name string, operation vimtypes.ArrayUpdateOperation) {
		if slices.ContainsFunc(configSpec.TagSpecs, func(tag vimtypes.TagSpec) bool {
			return tag.Operation == operation && tag.Id.NameId != nil &&
				tag.Id.NameId.Tag == name && tag.Id.NameId.Category == vm.Namespace
		}) {
			return
		}
		configSpec.TagSpecs = append(configSpec.TagSpecs, vimtypes.TagSpec{
			ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: operation},
			Id: vimtypes.TagId{NameId: &vimtypes.TagIdNameId{
				Tag: name, Category: vm.Namespace,
			}},
		})
		pkglog.FromContextOrDefault(ctx).Info("Reconciling VM tag association", "operation", operation,
			"tag", name, "category", vm.Namespace)
	}
	for _, name := range previous {
		if !slices.Contains(desired, name) {
			emit(name, vimtypes.ArrayUpdateOperationRemove)
		}
	}
	for _, name := range desired {
		if !slices.Contains(attach, name) {
			continue
		}
		if !slices.Contains(previous, name) {
			emit(name, vimtypes.ArrayUpdateOperationAdd)
		}
	}
	configSpec.ExtraConfig = pkgutil.OptionValues(configSpec.ExtraConfig).Merge(
		&vimtypes.OptionValue{Key: ExtraConfigVMTagsKey, Value: strings.Join(desired, ",")},
	)
}

func originalTagSpec(pair LabelPair) vspherepolv1.TagSpec {
	return vspherepolv1.TagSpec{
		Name:       PrefixFreeTagName(pair.Key, pair.Value),
		Kind:       vspherepolv1.TagKindOriginal,
		Membership: vspherepolv1.TagMembershipSourceLabel,
		Key:        pair.Key,
		Value:      pair.Value,
	}
}

// policyTagSpec returns the spec of the hashed tag of a policy: the self tag
// of a grouped anti-affinity policy or the hashed tag of an affinity policy.
func policyTagSpec(h AffinityPolicyHash) vspherepolv1.TagSpec {
	spec := vspherepolv1.TagSpec{
		Name:       PrefixFreePolicyTagName(h),
		Kind:       vspherepolv1.TagKindGroupedAntiAffinitySelf,
		Membership: vspherepolv1.TagMembershipPolicyRequesters,
		Policy: &vspherepolv1.TagPolicyIdentity{
			Type:       string(h.Type),
			Strictness: string(h.Strictness),
			Topology:   string(h.Topology),
		},
	}
	for _, pair := range h.Targets {
		spec.Policy.Targets = append(spec.Policy.Targets,
			vspherepolv1.TagLabelPair{Key: pair.Key, Value: pair.Value})
	}
	if h.Type == AffinityPolicyTypeAffinity {
		spec.Kind = vspherepolv1.TagKindAffinityTarget
		spec.Membership = vspherepolv1.TagMembershipSourceLabel
		spec.Key, spec.Value = h.Targets[0].Key, h.Targets[0].Value
	}
	return spec
}

func groupedAntiAffinityTargetTagSpec(
	h AffinityPolicyHash, pair LabelPair) vspherepolv1.TagSpec {

	spec := policyTagSpec(h)
	spec.Name = PrefixFreeGroupedAntiAffinityTargetName(h, pair)
	spec.Kind = vspherepolv1.TagKindGroupedAntiAffinityTarget
	spec.Membership = vspherepolv1.TagMembershipSourceLabel
	spec.Key, spec.Value = pair.Key, pair.Value
	return spec
}

// addDesiredTagIdentity validates the Tag and records it by resource name. Two
// different identities for one resource name are an error.
func addDesiredTagIdentity(
	identities map[string]vspherepolv1.Tag,
	tag vspherepolv1.Tag) error {

	if err := ValidateTagIdentity(&tag); err != nil {
		return fmt.Errorf("invalid Tag %q: %w", tag.Name, err)
	}
	return recordDesiredTagIdentity(identities, tag)
}

// recordDesiredTagIdentity records an already validated Tag by resource name.
// Two different identities for one resource name are an error.
func recordDesiredTagIdentity(
	identities map[string]vspherepolv1.Tag,
	tag vspherepolv1.Tag) error {

	// Account for the API server's System default when comparing a newly
	// computed identity to a cached object. No other field is normalized.
	tag.Spec.Type = vspherepolv1.TagTypeSystem
	if existing, ok := identities[tag.Name]; ok &&
		!reflect.DeepEqual(existing.Spec, tag.Spec) {

		return fmt.Errorf("conflicting identities for Tag %q", tag.Name)
	}
	identities[tag.Name] = tag
	return nil
}

func sortedDesiredTagIdentities(
	identities map[string]vspherepolv1.Tag) []vspherepolv1.Tag {

	tags := make([]vspherepolv1.Tag, 0, len(identities))
	for _, tag := range identities {
		tags = append(tags, tag)
	}
	slices.SortFunc(tags, func(a, b vspherepolv1.Tag) int {
		return strings.Compare(a.Spec.Name, b.Spec.Name)
	})
	return tags
}

// TagVCenterNames returns the sorted, deduplicated vCenter names of the Tags.
// Tag names are prefix-free; the result carries the AffinityTagPrefix.
func TagVCenterNames(tags []vspherepolv1.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, AddAffinityTagPrefix(tag.Spec.Name))
	}
	return normalizeTagNames(names)
}

// appendAffinityTag adds the named tag to the ConfigSpec's TagSpecs unless the
// same addition is already present. Existing TagSpecs are never removed, unlike
// setAffinityTags, which replaces the additions named in the receipt.
func appendAffinityTag(
	spec *vimtypes.VirtualMachineConfigSpec,
	namespace, name string) {

	for _, ts := range spec.TagSpecs {
		if ts.Operation == vimtypes.ArrayUpdateOperationAdd &&
			ts.Id.NameId != nil &&
			ts.Id.NameId.Category == namespace &&
			ts.Id.NameId.Tag == name {

			return
		}
	}
	spec.TagSpecs = append(spec.TagSpecs, vimtypes.TagSpec{
		ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{Operation: vimtypes.ArrayUpdateOperationAdd},
		Id:              affinityNameID(namespace, name),
	})
}

// ValidateTagIdentity verifies a Tag's complete immutable identity. Admission
// and adoption share this check so neither accepts inconsistent policy inputs,
// noncanonical targets, or an ambiguous backend name interpretation. All names
// in a Tag are prefix-free.
func ValidateTagIdentity(tag *vspherepolv1.Tag) error {
	spec := tag.Spec
	if spec.Type != "" && spec.Type != vspherepolv1.TagTypeSystem {
		return fmt.Errorf("spec.type must be System")
	}

	var wantName string
	switch spec.Kind {
	case vspherepolv1.TagKindOriginal,
		vspherepolv1.TagKindAffinityTarget,
		vspherepolv1.TagKindGroupedAntiAffinityTarget:

		if spec.Membership != vspherepolv1.TagMembershipSourceLabel {
			return fmt.Errorf("spec.membership must be SourceLabel for original/affinity tags")
		}
		if err := validateTagPair(LabelPair{Key: spec.Key, Value: spec.Value}); err != nil {
			return fmt.Errorf("spec.key/spec.value: %w", err)
		}
	case vspherepolv1.TagKindGroupedAntiAffinitySelf:
		if spec.Membership != vspherepolv1.TagMembershipPolicyRequesters {
			return fmt.Errorf("spec.membership must be PolicyRequesters for self tags")
		}
		if spec.Key != "" || spec.Value != "" {
			return fmt.Errorf("spec.key/spec.value must be absent for self tags")
		}
	default:
		return fmt.Errorf("unsupported spec.kind %q", spec.Kind)
	}

	if spec.Kind == vspherepolv1.TagKindOriginal {
		if spec.Policy != nil {
			return fmt.Errorf("spec.policy must be absent for original tags")
		}
		wantName = PrefixFreeTagName(spec.Key, spec.Value)
	} else {
		if spec.Policy == nil {
			return fmt.Errorf("spec.policy is required for derived tags")
		}
		policy := spec.Policy
		wantType := AffinityPolicyTypeAffinity
		if spec.Kind == vspherepolv1.TagKindGroupedAntiAffinitySelf ||
			spec.Kind == vspherepolv1.TagKindGroupedAntiAffinityTarget {

			wantType = AffinityPolicyTypeGroupedAntiAffinity
		}
		if policy.Type != string(wantType) {
			return fmt.Errorf("spec.policy.type must be %s for %s", wantType, spec.Kind)
		}
		targets := make([]LabelPair, len(policy.Targets))
		for i, pair := range policy.Targets {
			targets[i] = LabelPair{Key: pair.Key, Value: pair.Value}
			if err := validateTagPair(targets[i]); err != nil {
				return fmt.Errorf("spec.policy.targets[%d]: %w", i, err)
			}
		}
		h, err := NewAffinityPolicyHash(
			wantType,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness(policy.Strictness),
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopology(policy.Topology),
			targets)
		if err != nil {
			return fmt.Errorf("spec.policy: %w", err)
		}
		if !slices.Equal(targets, h.Targets) {
			return fmt.Errorf("spec.policy.targets must be sorted and unique")
		}
		pair := LabelPair{Key: spec.Key, Value: spec.Value}
		if spec.Kind == vspherepolv1.TagKindAffinityTarget && targets[0] != pair {
			return fmt.Errorf("spec.policy.targets must match the affinity source label")
		}
		wantName = PrefixFreePolicyTagName(h)
		if spec.Kind == vspherepolv1.TagKindGroupedAntiAffinityTarget {
			if !slices.Contains(h.Targets, pair) {
				return fmt.Errorf("spec.policy.targets must contain the grouped anti-affinity source label")
			}
			wantName = PrefixFreeGroupedAntiAffinityTargetName(h, pair)
		}
	}

	if spec.Name != wantName {
		return fmt.Errorf("spec.name must equal %q", wantName)
	}
	if want := ConcreteTagResourceName(tag.Namespace, spec.Name); tag.Name != want {
		return fmt.Errorf("metadata.name must equal %q, derived from namespace and spec.name", want)
	}
	return nil
}

func validateTagPair(pair LabelPair) error {
	if errs := content.IsLabelKey(pair.Key); len(errs) != 0 {
		return fmt.Errorf("invalid label key %q: %v", pair.Key, errs)
	}
	if errs := content.IsLabelValue(pair.Value); len(errs) != 0 {
		return fmt.Errorf("invalid label value %q: %v", pair.Value, errs)
	}
	if kubeutil.HasVMOperatorLabels(map[string]string{pair.Key: pair.Value}) {
		return fmt.Errorf("reserved label key %q", pair.Key)
	}
	return nil
}

// DesiredAffinityTags returns the complete, prefix-free Tag identities that the
// VM's own affinity declares: for each eligible rule, the original tag of every
// target pair and the derived tags of the policy that the rule produces.
//
// The set is computed from the same inputs as generateAffinityPolicies, using
// CalculateAffinityOpts for eligibility, affinityTargetsForTopology for
// selector resolution and NewAffinityPolicyHash for identities. It is a pure
// function: it writes nothing and sets no owner references. The caller must
// finish this computation before ensuring or pruning ownership.
//
// Invalid or reserved label pairs are logged and skipped, as
// generateAffinityPolicies does, so one bad pair cannot fail the VM's
// reconcile. An invalid pair skips its own tags for an affinity rule and the
// whole policy for an anti-affinity rule. An error is returned only when two
// different identities share one Tag name.
//
// Tag identities are defined only for hashed tag names, so nothing is returned
// unless the hashed-tag behavior (VMHardAffinityDuringExecution) is enabled.
// Feature gating of the Tag resources themselves is the caller's concern.
func DesiredAffinityTags(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) ([]vspherepolv1.Tag, error) {

	// Under the hashed-tag behavior the constraints do not depend on whether
	// this is placement or creation, so one calculation covers both.
	constraints := CalculateAffinityOpts(ctx, vm, false, nil)
	if !constraints.ConfigureHashedTags || vm.Spec.Affinity == nil {
		return nil, nil
	}

	logger := pkglog.FromContextOrDefault(ctx)
	identities := map[string]vspherepolv1.Tag{}
	tagFor := func(spec vspherepolv1.TagSpec) vspherepolv1.Tag {
		return vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: vm.Namespace,
				Name:      ConcreteTagResourceName(vm.Namespace, spec.Name),
			},
			Spec: spec,
		}
	}
	// addAll records the specs together or not at all. Specs that are not valid
	// Tag identities, such as a reserved label key, are logged and skipped as a
	// unit, like generateAffinityPolicies skips an invalid target. Only a
	// conflict between two identities is returned as an error.
	addAll := func(specs []vspherepolv1.TagSpec, what string) error {
		tags := make([]vspherepolv1.Tag, 0, len(specs))
		for _, spec := range specs {
			tag := tagFor(spec)
			if err := ValidateTagIdentity(&tag); err != nil {
				logger.Error(err, "invalid affinity target", "what", what, "tag", spec.Name)
				return nil
			}
			tags = append(tags, tag)
		}
		for _, tag := range tags {
			if err := recordDesiredTagIdentity(identities, tag); err != nil {
				return err
			}
		}
		return nil
	}

	addAffinity := func(
		pairs []LabelPair,
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) error {

		for _, pair := range pairs {
			h, err := NewAffinityPolicyHash(
				AffinityPolicyTypeAffinity, strictness, topology, []LabelPair{pair})
			if err != nil {
				// Skip the invalid pair, as generateAffinityPolicies does.
				logger.Error(err, "invalid affinity target", "key", pair.Key, "value", pair.Value)
				continue
			}
			if err := addAll([]vspherepolv1.TagSpec{
				originalTagSpec(pair), policyTagSpec(h),
			}, "affinity"); err != nil {
				return err
			}
		}
		return nil
	}

	addAntiAffinity := func(
		pairs []LabelPair,
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness,
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology) error {

		if len(pairs) == 0 {
			return nil
		}
		h, err := NewAffinityPolicyHash(
			AffinityPolicyTypeGroupedAntiAffinity, strictness, topology, pairs)
		if err != nil {
			// Skip the invalid policy, as generateAffinityPolicies does.
			logger.Error(err, "invalid anti-affinity targets")
			return nil
		}
		specs := []vspherepolv1.TagSpec{policyTagSpec(h)}
		for _, pair := range h.Targets {
			specs = append(specs,
				originalTagSpec(pair), groupedAntiAffinityTargetTagSpec(h, pair))
		}
		return addAll(specs, "anti-affinity")
	}

	var af vmopv1.VMAffinitySpec
	if vm.Spec.Affinity.VMAffinity != nil {
		af = *vm.Spec.Affinity.VMAffinity
	}
	var aaf vmopv1.VMAntiAffinitySpec
	if vm.Spec.Affinity.VMAntiAffinity != nil {
		aaf = *vm.Spec.Affinity.VMAntiAffinity
	}

	type termSet struct {
		strictness vimtypes.VmPlacementPolicyVmPlacementPolicyStrictness
		affinity   []vmopv1.VMAffinityTerm
		anti       []vmopv1.VMAffinityTerm
	}
	termSets := []termSet{
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
		termSets = append(termSets, termSet{
			strictness: vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessRequiredDuringPlacementRequiredDuringExecution,
			affinity:   af.RequiredDuringSchedulingRequiredDuringExecution,
			anti:       aaf.RequiredDuringSchedulingRequiredDuringExecution,
		})
	}

	topologies := []struct {
		topology vimtypes.VmPlacementPolicyVmPlacementPolicyTopology
		eligible bool
	}{
		{vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyVSphereZone, constraints.ConfigureZoneRules},
		{vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost, constraints.ConfigureHostRules},
	}
	for _, t := range topologies {
		if !t.eligible {
			continue
		}
		for _, s := range termSets {
			if err := addAffinity(
				affinityTargetsForTopology(ctx, s.affinity, t.topology),
				s.strictness, t.topology); err != nil {
				return nil, err
			}
			if err := addAntiAffinity(
				affinityTargetsForTopology(ctx, s.anti, t.topology),
				s.strictness, t.topology); err != nil {
				return nil, err
			}
		}
	}

	return sortedDesiredTagIdentities(identities), nil
}

// ListLabelDerivedTags returns the Tags the VM carries because of the labels it
// has, as opposed to the policies it declares: for every label pair on the VM, the
// original, affinity target and grouped anti-affinity target Tags whose source
// label is that pair. The result is sorted by Tag name.
//
// Tags are found through the namespace-scoped source-pair index, so k8sClient
// must be a cache-backed client. ownedTags are Tags that this reconcile created
// or adopted; they make the result independent of informer lag. Only Tags whose
// source pair matches a carried label are returned, so a VM that declares a
// policy against a label it does not carry gets none of that label's Tags.
// Self Tags are never returned, and owning a Tag never makes a VM a member.
//
// The lookup either returns the complete set or an error; callers must not
// continue with a partial set.
func ListLabelDerivedTags(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	ownedTags []vspherepolv1.Tag) ([]vspherepolv1.Tag, error) {

	labels := kubeutil.RemoveVMOperatorLabels(vm.Labels)
	identities := map[string]vspherepolv1.Tag{}

	// matches rechecks the label match instead of trusting an index entry or a
	// cached ownership list to establish that the VM carries the label.
	matches := func(tag vspherepolv1.Tag) bool {
		if tag.Namespace != vm.Namespace ||
			tag.Spec.Membership != vspherepolv1.TagMembershipSourceLabel ||
			tag.Spec.Kind == vspherepolv1.TagKindGroupedAntiAffinitySelf {

			return false
		}
		value, ok := labels[tag.Spec.Key]
		return ok && value == tag.Spec.Value
	}

	for _, key := range sets.List(sets.KeySet(labels)) {
		var list vspherepolv1.TagList
		if err := k8sClient.List(ctx, &list,
			ctrlclient.InNamespace(vm.Namespace),
			ctrlclient.MatchingFields{
				kubeutil.TagSourceLabelIndexKey: PrefixFreeTagName(key, labels[key]),
			}); err != nil {
			return nil, fmt.Errorf("list affinity Tags for %q: %w", key, err)
		}
		for _, tag := range list.Items {
			if !matches(tag) {
				continue
			}
			if err := addDesiredTagIdentity(identities, tag); err != nil {
				return nil, err
			}
		}
	}

	for _, tag := range ownedTags {
		if !matches(tag) {
			continue
		}
		if err := addDesiredTagIdentity(identities, tag); err != nil {
			return nil, err
		}
	}

	return sortedDesiredTagIdentities(identities), nil
}

// AffinityPlacementMember pairs a VM with its transient placement
// configuration.
type AffinityPlacementMember struct {
	VM         *vmopv1.VirtualMachine
	ConfigSpec *vimtypes.VirtualMachineConfigSpec
}

// AddGroupAntiAffinityPlacementTags adds peer target membership after every
// member's placement configuration is built. A member's anti-affinity targets
// may be carried by peers in the same group that have no Tag resources yet, so
// each peer that carries a targeted label gets the target tag for placement.
//
// It needs no Tag resources and changes only the placement TagSpecs: tags
// already on a ConfigSpec are kept, and receipts and self tags are untouched.
// It does nothing unless the tagging API is enabled.
func AddGroupAntiAffinityPlacementTags(
	ctx context.Context,
	members []AffinityPlacementMember) error {

	if !pkgcfg.FromContext(ctx).Features.TaggingAPI {
		return nil
	}

	// targets holds, per namespace, the vCenter names of the target tags that
	// some member's policy references, keyed by label pair.
	type pair struct{ key, value string }
	targets := map[string]map[pair]sets.Set[string]{}

	for _, member := range members {
		local, err := DesiredAffinityTags(ctx, member.VM)
		if err != nil {
			return fmt.Errorf(
				"resolve group anti-affinity targets for %s/%s: %w",
				member.VM.Namespace, member.VM.Name, err)
		}

		referenced := sets.New[string]()
		for _, raw := range member.ConfigSpec.VmPlacementPolicies {
			var ids []vimtypes.TagId
			switch policy := raw.(type) {
			case *vimtypes.VmToVmGroupsAntiAffinity:
				ids = policy.AntiAffinedVmGroupTags
			case *vimtypes.VmVmAntiAffinity:
				ids = []vimtypes.TagId{policy.AntiAffinedVmsTag}
			}
			for _, id := range ids {
				if id.NameId != nil && id.NameId.Category == member.VM.Namespace {
					referenced.Insert(id.NameId.Tag)
				}
			}
		}

		if targets[member.VM.Namespace] == nil {
			targets[member.VM.Namespace] = map[pair]sets.Set[string]{}
		}
		for _, tag := range local {
			if tag.Spec.Membership != vspherepolv1.TagMembershipSourceLabel {
				continue
			}
			name := AddAffinityTagPrefix(tag.Spec.Name)
			if !referenced.Has(name) {
				continue
			}
			p := pair{tag.Spec.Key, tag.Spec.Value}
			if targets[member.VM.Namespace][p] == nil {
				targets[member.VM.Namespace][p] = sets.New[string]()
			}
			targets[member.VM.Namespace][p].Insert(name)
		}
	}

	for _, member := range members {
		// Placement ConfigSpecs may share backing arrays with the create
		// candidates, so work on a copy.
		member.ConfigSpec.TagSpecs = slices.Clone(member.ConfigSpec.TagSpecs)
		labels := kubeutil.RemoveVMOperatorLabels(member.VM.Labels)
		for p, names := range targets[member.VM.Namespace] {
			if value, ok := labels[p.key]; !ok || value != p.value {
				continue
			}
			for _, name := range sets.List(names) {
				appendAffinityTag(member.ConfigSpec, member.VM.Namespace, name)
			}
		}
	}
	return nil
}
