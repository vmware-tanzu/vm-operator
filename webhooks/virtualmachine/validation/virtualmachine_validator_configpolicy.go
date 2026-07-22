// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicy"
)

// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies,verbs=get;list

// validateConfigPolicy enforces the namespace's VirtualMachineConfigPolicy
// (if any) for the zone this VM is (or will be) placed in. It is a no-op
// unless the VirtualMachineConfigPolicy capability is enabled and the VM has
// a resolved zone.
//
// oldVM is nil for a create request.
func (v validator) validateConfigPolicy(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	zoneName := vm.Labels[corev1.LabelTopologyZone]
	if zoneName == "" {
		// This VM has not yet been placed into a zone, so the applicable
		// policy cannot be resolved. Enforcement resumes on the next update
		// once placement sets the zone label. See spec.md's discussion of
		// the not-yet-placed VM edge case (Spike vmop-3794).
		return nil
	}

	policy, err := v.getVMConfigPolicy(ctx, vm.Namespace, zoneName)
	if err != nil {
		return field.ErrorList{field.InternalError(field.NewPath("spec"), err)}
	}
	if policy == nil {
		// No policy governs this zone; nothing to enforce.
		return nil
	}

	if !configpolicy.AppliesToVM(policy.Spec, vm.Spec.ClassName) {
		return nil
	}

	isCreate := oldVM == nil

	// isPowerOnTransition is true only when this request is the specific
	// edge that flips the VM from not-powered-on to powered-on: a create
	// with spec.powerState already "On", or an update whose oldVM was not
	// "On" and whose vm now is. An update that merely keeps an already-"On"
	// VM powered on (e.g. an unrelated field changing) is not a transition.
	var isPowerOnTransition bool
	if isCreate {
		isPowerOnTransition = vm.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn
	} else {
		wasNotOn := oldVM.Spec.PowerState != vmopv1.VirtualMachinePowerStateOn
		isPowerOnTransition = wasNotOn &&
			vm.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn
	}

	// A policy's three modes each gate a different kind of request, and
	// this request can match more than one at a time (e.g. a create that is
	// also a power-on transition). shouldEnforce is true if ANY mode that
	// applies to this request is set to Deny -- so createMode/updateMode and
	// powerOnMode are independent gates, not a single combined decision:
	//   - createMode governs this request only when it is a create.
	//   - updateMode governs this request only when it is an update.
	//   - powerOnMode governs this request whenever it is a power-on
	//     transition, REGARDLESS of whether it is also a create or an
	//     update. This lets a policy demand compliance at power-on time
	//     even when create/update requests are otherwise left alone (e.g.
	//     updateMode=Allow but powerOnMode=Deny still blocks a
	//     non-compliant VM from powering on).
	// If none of the modes that apply here are Deny, this request is
	// allowed without ever evaluating compliance below.
	deny := vimv1.VirtualMachineConfigPolicyModeDeny
	shouldEnforce := (isCreate && policy.Spec.CreateMode == deny) ||
		(!isCreate && policy.Spec.UpdateMode == deny) ||
		(isPowerOnTransition && policy.Spec.PowerOnMode == deny)

	if !shouldEnforce {
		return nil
	}

	in := configpolicy.InputFromVM(vm)
	violation := configpolicy.Validate(ctx, policy.Spec, in)

	return toFieldErrors(vm, violation)
}

// getVMConfigPolicy returns the VirtualMachineConfigPolicy that governs
// zoneName, or nil if none exists. The Zone controller always names a zone's
// policy after the zone (controllers/infra/zone), so this is a single,
// informer-cached Get -- no List, no label selector.
func (v validator) getVMConfigPolicy(
	ctx *pkgctx.WebhookRequestContext,
	namespace, zoneName string) (*vimv1.VirtualMachineConfigPolicy, error) {

	policy := &vimv1.VirtualMachineConfigPolicy{}
	key := ctrlclient.ObjectKey{Namespace: namespace, Name: zoneName}

	if err := v.client.Get(ctx, key, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return policy, nil
}

// toFieldErrors translates violation -- nil, or the result of
// configpolicy.Validate -- into the field.ErrorList this webhook is
// expected to return, mapping each violation to the specific spec field it
// concerns.
func toFieldErrors(vm *vmopv1.VirtualMachine, violation error) field.ErrorList {
	var allErrs field.ErrorList

	for _, v := range configpolicy.Violations(violation) {
		if fieldErr := toFieldError(vm, v); fieldErr != nil {
			allErrs = append(allErrs, fieldErr)
		}
	}

	return allErrs
}

// toFieldError translates a single violation -- one of the Err*Violation
// types in pkg/util/configpolicy -- into the field.Error naming the spec
// field it concerns. It uses errors.As, not a type switch, because a
// violation may be wrapped (its own Err field, or any future wrapping
// between configpolicy.Validate and here); a type switch would silently
// drop a wrapped violation instead of reporting it. It returns nil if v is
// not (or does not wrap) a recognized violation type.
func toFieldError(vm *vmopv1.VirtualMachine, v error) *field.Error {
	var extraConfigErr *configpolicy.ErrExtraConfigViolation
	if errors.As(v, &extraConfigErr) {
		return extraConfigFieldError(vm, extraConfigErr)
	}

	var hardwareVersionErr *configpolicy.ErrHardwareVersionViolation
	if errors.As(v, &hardwareVersionErr) {
		path := field.NewPath("spec", "minHardwareVersion")
		return field.Invalid(
			path, vm.Spec.MinHardwareVersion, hardwareVersionErr.Error())
	}

	var cpuCoresErr *configpolicy.ErrCPUCoresViolation
	if errors.As(v, &cpuCoresErr) {
		path := field.NewPath("spec", "resources", "size", "cpu")
		return field.Invalid(path, cpuCoresErr.Got, cpuCoresErr.Error())
	}

	var memoryErr *configpolicy.ErrMemoryViolation
	if errors.As(v, &memoryErr) {
		path := field.NewPath("spec", "resources", "size", "memory")
		return field.Invalid(path, memoryErr.Got.String(), memoryErr.Error())
	}

	var numaNodesErr *configpolicy.ErrNUMANodesViolation
	if errors.As(v, &numaNodesErr) {
		path := field.NewPath("spec", "cpuAdvanced", "topology", "vnumaNodeCount")
		return field.Invalid(path, numaNodesErr.Got, numaNodesErr.Error())
	}

	var iommuErr *configpolicy.ErrIOMMUViolation
	if errors.As(v, &iommuErr) {
		path := field.NewPath("spec", "cpuAdvanced", "iommuEnabled")
		return field.Forbidden(path, iommuErr.Error())
	}

	var memoryLockedErr *configpolicy.ErrMemoryLockedToMaxViolation
	if errors.As(v, &memoryLockedErr) {
		path := field.NewPath("spec", "memoryAdvanced", "reservationLockedToMax")
		return field.Forbidden(path, memoryLockedErr.Error())
	}

	var hugePagesErr *configpolicy.ErrHugePagesViolation
	if errors.As(v, &hugePagesErr) {
		path := field.NewPath("spec", "advanced", "hugePages1GEnabled")
		return field.Forbidden(path, hugePagesErr.Error())
	}

	return nil
}

// extraConfigFieldError builds the field.Error for e, locating the
// violating key's index in vm.Spec.Advanced.ExtraConfig so the error names
// the exact list entry in violation.
func extraConfigFieldError(
	vm *vmopv1.VirtualMachine,
	e *configpolicy.ErrExtraConfigViolation) *field.Error {
	ecPath := field.NewPath("spec", "advanced", "extraConfig")

	if vm.Spec.Advanced != nil {
		for i, kv := range vm.Spec.Advanced.ExtraConfig {
			if kv.Key == e.Key {
				return field.Forbidden(ecPath.Index(i).Child("key"), e.Error())
			}
		}
	}

	return field.Forbidden(ecPath, e.Error())
}
