// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	flexFieldNotSupportedYet   = "not supported yet"
	flexFieldRequiresClassless = "requires the VMServiceClasslessVM capability"
)

// validateFlexFields gates the Flex VM fields spec.hardware.vGPUs,
// spec.hardware.dynamicDirectPathIODevices and spec.reservedProfileName. A
// field is only checked on create or on update when its value differs from the
// old VM, so VMs that already carry a value can still be updated. There is no
// exemption for privileged accounts, matching the other feature-gated fields.
func (v validator) validateFlexFields(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs     field.ErrorList
		specPath    = field.NewPath("spec")
		hwPath      = specPath.Child("hardware")
		capEnabled  = pkgcfg.FromContext(ctx).Features.VMServiceClasslessVM
		newHardware vmopv1.VirtualMachineHardwareSpec
		oldHardware vmopv1.VirtualMachineHardwareSpec
		oldProfile  *string
	)

	if hw := newVM.Spec.Hardware; hw != nil {
		newHardware = *hw
	}
	if oldVM != nil {
		if hw := oldVM.Spec.Hardware; hw != nil {
			oldHardware = *hw
		}
		oldProfile = oldVM.Spec.ReservedProfileName
	}

	gate := func(p *field.Path) {
		// Not yet supported: always rejected. Remove this check when the
		// provider implements the field (T108a, T110 and the vmop stories
		// that consume it). The capability check below stays permanently.
		allErrs = append(allErrs, field.Forbidden(p, flexFieldNotSupportedYet))

		if !capEnabled {
			allErrs = append(allErrs, field.Forbidden(p, flexFieldRequiresClassless))
		}
	}

	if len(newHardware.VGPUs) > 0 &&
		!apiequality.Semantic.DeepEqual(newHardware.VGPUs, oldHardware.VGPUs) {
		gate(hwPath.Child("vGPUs"))
	}

	if len(newHardware.DynamicDirectPathIODevices) > 0 &&
		!apiequality.Semantic.DeepEqual(
			newHardware.DynamicDirectPathIODevices,
			oldHardware.DynamicDirectPathIODevices) {
		gate(hwPath.Child("dynamicDirectPathIODevices"))
	}

	if newVM.Spec.ReservedProfileName != nil &&
		!apiequality.Semantic.DeepEqual(newVM.Spec.ReservedProfileName, oldProfile) {
		gate(specPath.Child("reservedProfileName"))
	}

	return allErrs
}
