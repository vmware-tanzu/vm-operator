// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// ExtraConfigVMTagsKey records managed tag names in the VM namespace category.
const ExtraConfigVMTagsKey = "vmservice.placementpolicies.tags"

// AffinityTagPrefix identifies tags generated for VM Operator affinity policies.
const AffinityTagPrefix = "vmoperator:"

// LabelPair is a resolved affinity selector key/value pair.
type LabelPair struct{ Key, Value string }

// TagResourceName returns the namespace-local resource name for a label pair.
func TagResourceName(namespace, key, value string) string {
	return "tag-" + pkgutil.XXHash64Hex(namespace+":"+key+":"+value)
}

// TagName returns the prefixed original tag name for a label pair.
// To recover the pair, remove AffinityTagPrefix before splitting on the colon.
func TagName(key, value string) string { return affinityTagName(key + ":" + value) }

// affinityTagName applies the shared prefix to an original or hashed tag name.
func affinityTagName(name string) string { return AffinityTagPrefix + name }

// RecordedVMTags returns the applied name-based tag set. An absent or empty
// record means no recorded associations. Only this receipt identifies managed
// attachments; unrelated configuration is not used to infer ownership.
func RecordedVMTags(config *vimtypes.VirtualMachineConfigInfo) []string {
	if config == nil {
		return nil
	}
	value, _ := object.OptionValueList(config.ExtraConfig).GetString(ExtraConfigVMTagsKey)
	if value == "" {
		return nil
	}
	return canonicalTagNames(strings.Split(value, ","))
}

func canonicalTagNames(tags []string) []string {
	result := slices.Clone(tags)
	result = slices.DeleteFunc(result, func(name string) bool { return name == "" })
	slices.Sort(result)
	return slices.Compact(result)
}

// ReconcileVMTags emits the diff between the complete desired managed tag set
// and the previously applied record. Call exactly once with the complete set;
// namespace-wide membership and local policy producers must contribute to
// that set before calling this writer. Unrelated outgoing operations are preserved.
func ReconcileVMTags(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	desired []string) {
	desired = canonicalTagNames(desired)
	previous := RecordedVMTags(config)
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
		if _, exists := slices.BinarySearch(desired, name); !exists {
			emit(name, vimtypes.ArrayUpdateOperationRemove)
		}
	}
	for _, name := range desired {
		if _, exists := slices.BinarySearch(previous, name); !exists {
			emit(name, vimtypes.ArrayUpdateOperationAdd)
		}
	}
	configSpec.ExtraConfig = pkgutil.OptionValues(configSpec.ExtraConfig).Merge(
		&vimtypes.OptionValue{Key: ExtraConfigVMTagsKey, Value: strings.Join(desired, ",")},
	)
}
