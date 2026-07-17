// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// validateISO validates spec.bootstrap.iso: at least one spec.hardware.cdrom
// entry must reference an ISO-type image, and every asset's Secret name
// must be a valid DNS subdomain.
func (v validator) validateISO(
	ctx *pkgctx.WebhookRequestContext,
	p *field.Path,
	vm *vmopv1.VirtualMachine,
	iso *vmopv1.VirtualMachineBootstrapISOSpec) field.ErrorList {

	var allErrs field.ErrorList

	allErrs = append(allErrs, v.validateISOCdromImage(ctx, vm)...)

	assetsPath := p.Child("assets")
	for i, asset := range iso.Assets {
		if errs := k8svalidation.IsDNS1123Subdomain(asset.Name); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(
				assetsPath.Index(i).Child("name"), asset.Name, strings.Join(errs, ", ")))
		}
	}

	return allErrs
}

// validateISOCdromImage confirms at least one spec.hardware.cdrom entry
// references a VirtualMachineImage or ClusterVirtualMachineImage whose
// Status.Type is ISO.
func (v validator) validateISOCdromImage(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var newCD []vmopv1.VirtualMachineCdromSpec
	if hw := vm.Spec.Hardware; hw != nil {
		newCD = hw.Cdrom
	}

	if len(newCD) == 0 {
		return field.ErrorList{field.Required(
			field.NewPath("spec", "hardware", "cdrom"),
			"at least one CD-ROM referencing an ISO-type image is required when spec.bootstrap.iso is set")}
	}

	for _, c := range newCD {
		if v.isISOTypeImage(ctx, vm.Namespace, c.Image) {
			return nil
		}
	}

	return field.ErrorList{field.Invalid(
		field.NewPath("spec", "hardware", "cdrom"), "cdrom",
		"at least one CD-ROM must reference an ISO-type image when spec.bootstrap.iso is set")}
}

// isISOTypeImage reports whether imageRef resolves to a
// VirtualMachineImage/ClusterVirtualMachineImage whose Status.Type is ISO.
// Any error resolving the image (not found, not ready, etc.) is treated as
// "not an ISO image" -- the caller surfaces a single, generic field error
// rather than leaking resolution details for an image the requesting user
// may not have read access to.
func (v validator) isISOTypeImage(
	ctx *pkgctx.WebhookRequestContext,
	namespace string,
	imageRef vmopv1.VirtualMachineImageRef) bool {

	var vmi vmopv1.VirtualMachineImage

	switch imageRef.Kind {
	case vmiKind:
		if err := v.client.Get(ctx, ctrlclient.ObjectKey{
			Name: imageRef.Name, Namespace: namespace}, &vmi); err != nil {
			return false
		}
	case cvmiKind:
		var cvmi vmopv1.ClusterVirtualMachineImage
		if err := v.client.Get(ctx, ctrlclient.ObjectKey{Name: imageRef.Name}, &cvmi); err != nil {
			return false
		}
		vmi = vmopv1.VirtualMachineImage(cvmi)
	default:
		return false
	}

	return vmi.Status.Type == string(imgregv1a1.ContentLibraryItemTypeIso)
}

// validateISOWhenPoweredOn forbids changing spec.bootstrap.iso.commands once
// boot commands have already been sent (tracked via the bootstrap-iso-hash
// annotation) while the VM is powered on.
func (v validator) validateISOWhenPoweredOn(
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if oldVM.Annotations[pkgconst.BootstrapISOHashAnnotationKey] == "" {
		// Commands have never been sent -- nothing to protect yet.
		return allErrs
	}

	oldISO, newISO := oldVM.Spec.Bootstrap, vm.Spec.Bootstrap
	if oldISO == nil || oldISO.ISO == nil || newISO == nil || newISO.ISO == nil {
		return allErrs
	}

	p := field.NewPath("spec", "bootstrap", "iso", "commands")
	if !equalStringSlices(oldISO.ISO.Commands, newISO.ISO.Commands) {
		allErrs = append(allErrs, field.Forbidden(p, updatesNotAllowedWhenPowerOn))
	}

	return allErrs
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
