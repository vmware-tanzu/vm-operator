// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
)

// CleanupVMServiceState removes all modifications made by VM Operator
// from a vCenter VM. This should be called when deleting a VM
// resource with the skip-delete-platform-resource annotation to
// ensure the vCenter VM is left in a clean state.
func CleanupVMServiceState(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	vmCtx.Logger.Info("Sanitizing vCenter VM")

	var moVM mo.VirtualMachine
	if err := vcVM.Properties(
		vmCtx,
		vcVM.Reference(),
		[]string{"config"},
		&moVM); err != nil {

		return fmt.Errorf("failed to fetch vm properties for cleanup: %w", err)
	}

	if moVM.Config == nil {
		vmCtx.Logger.Info("VM config is nil, skipping cleanup")
		return nil
	}

	configSpec := vimtypes.VirtualMachineConfigSpec{}

	// Remove ExtraConfig keys
	removeVMOperatorExtraConfig(moVM.Config, &configSpec)

	// Clear ManagedBy field
	clearManagedBy(moVM.Config, &configSpec)

	// Remove tag associations
	if pkgcfg.FromContext(vmCtx).Features.VSpherePolicies {
		removeTagAssociations(moVM.Config, &configSpec)
	}

	// Clear the extension compatibility constraints VM Operator registered,
	// since it no longer manages this VM.
	if pkgcfg.FromContext(vmCtx).Features.ExtensionCompatConstraint {
		ClearConfigSpecExtensionCompatibilityConstraint(moVM.Config, &configSpec)
	}

	// Apply the configuration changes
	if err := reconfigureVM(vmCtx, vcVM, configSpec); err != nil {
		return fmt.Errorf("failed to apply cleanup configuration: %w", err)
	}

	vmCtx.Logger.Info("Successfully sanitized the vCenter VM")
	return nil
}

// removeTagAssociations removes any tags that were attached to the VM
// by VM operator.
func removeTagAssociations(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	if config == nil {
		return
	}

	// VM operator associates tags to VMs that specify either
	// mandatory or optional compute policy. Remove those from the
	// VMs.
	// VM operator stores any tag that it applies to the VM in the
	// ExtraConfig.
	ec := object.OptionValueList(config.ExtraConfig)
	if v, _ := ec.GetString(policy.ExtraConfigPolicyTagsKey); v != "" {
		for tag := range strings.SplitSeq(v, ",") {
			// Just because a tag is present in the ExtraConfig
			// does not guarantee that it is also associated to
			// the VM. However, talking to tagging service for
			// each tag might be non-performant. And Reconfigure
			// will ignore these tags anyway.
			ts := vimtypes.TagSpec{
				ArrayUpdateSpec: vimtypes.ArrayUpdateSpec{
					Operation: vimtypes.ArrayUpdateOperationRemove,
				},
				Id: vimtypes.TagId{
					Uuid: tag,
				},
			}
			configSpec.TagSpecs = append(configSpec.TagSpecs, ts)
		}
	}
}

// removeVMOperatorExtraConfig identifies and marks for removal all
// ExtraConfig keys that were added by VM Operator.
//
// Unlike FilteredExtraConfig which preserves allow-listed keys for reuse
// scenarios, this method removes ALL VM Operator keys (both deny-listed
// and allow-listed) since we're unregistering the VM from Supervisor.
func removeVMOperatorExtraConfig(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	if config == nil {
		return
	}

	currentEC := object.OptionValueList(config.ExtraConfig)

	// Get the deny prefixes and allow keys from extraconfig.go
	// We need to remove keys matching BOTH deny prefixes AND allow list
	denyPrefixes := []string{
		"guestinfo.",
		"vmservice.",
		"imageregistry.",
	}

	for _, optVal := range currentEC {
		if opt := optVal.GetOptionValue(); opt != nil {
			key := opt.Key

			// Clean up the key if it has a denyPrefix. But crucially,
			// also clean up if the key is allowed since this is a
			// total unregister of a VM.
			denyPrefixMatch := slices.ContainsFunc(denyPrefixes, func(prefix string) bool {
				return strings.HasPrefix(strings.ToLower(key), strings.ToLower(prefix))
			})

			if denyPrefixMatch {
				// Mark for removal by setting value to empty
				// string. An ExtraConfig key with an empty value
				// is either ignored (if not present) or removed
				// (if present) from the VM.
				configSpec.ExtraConfig = append(configSpec.ExtraConfig,
					&vimtypes.OptionValue{
						Key:   key,
						Value: "",
					})
			}
		}
	}
}

// clearManagedBy removes the ManagedBy field if it was set by VM
// Operator.
func clearManagedBy(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	if config == nil || config.ManagedBy == nil {
		return
	}

	// Check if this VM is managed by VM Operator
	if config.ManagedBy.ExtensionKey == vmopv1.ManagedByExtensionKey &&
		config.ManagedBy.Type == vmopv1.ManagedByExtensionType {

		// Set managedBy to empty values so they are unset by
		// Reconfigure.
		configSpec.ManagedBy = &vimtypes.ManagedByInfo{}
	}
}

// reconfigureVM applies the configuration changes to the VM.
func reconfigureVM(
	ctx context.Context,
	vcVM *object.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec) error {

	// Check if there are any changes to apply. This must stay above the
	// bypass gate below: setting the skip flag unconditionally would turn
	// every no-op cleanup call into a real reconfigure task.
	if len(configSpec.ExtraConfig) == 0 && configSpec.ManagedBy == nil &&
		len(configSpec.TagSpecs) == 0 && configSpec.ExtensionCompatibilityConstraint == nil {
		// No changes to apply
		return nil
	}

	// VM Operator registered these constraints on the VM (see CreateConfigSpec
	// and the extension compat constraint reconciler), so its own reconfigures
	// -- including this one, which unsets managedBy as part of releasing the
	// VM -- must not be blocked by them. VM Operator holds the
	// ExtensionCompat.Bypass privilege required for vpxd to honor this flag.
	if pkgcfg.FromContext(ctx).Features.ExtensionCompatConstraint {
		configSpec.SkipExtensionCompatibilityChecks = ptr.To(true)
	}

	task, err := vcVM.Reconfigure(ctx, configSpec)
	if err != nil {
		return fmt.Errorf("failed to start reconfigure task: %w", err)
	}

	taskInfo, err := task.WaitForResult(ctx)
	if err != nil {
		if taskInfo != nil {
			return fmt.Errorf("reconfigure task failed: %w, taskInfo: %+v", err, taskInfo)
		}
		return fmt.Errorf("reconfigure task failed: %w", err)
	}

	return nil
}
