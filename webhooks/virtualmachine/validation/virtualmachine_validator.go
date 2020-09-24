// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha1,name=default.validating.virtualmachine.vmoperator.vmware.com,sideEffects=None
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator())
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachine validation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator() builder.Validator {
	return validator{
		// TODO BMV Use the Context.scheme instead
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachine{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateMetadata(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateImage(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateClass(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateNetwork(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVolumes(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVmVolumeProvisioningOptions(ctx, vm)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVM, err := v.vmFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateAllowedChanges(ctx, vm, oldVM)...)
	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	if vm.Spec.VmMetadata == nil {
		return validationErrs
	}

	if vm.Spec.VmMetadata.Transport != vmopv1.VirtualMachineMetadataExtraConfigTransport &&
		vm.Spec.VmMetadata.Transport != vmopv1.VirtualMachineMetadataOvfEnvTransport {
		validationErrs = append(validationErrs, messages.MetadataTransportNotSupported)
	}

	if vm.Spec.VmMetadata.ConfigMapName == "" {
		validationErrs = append(validationErrs, messages.MetadataTransportConfigMapNotSpecified)
	}

	return validationErrs
}

func (v validator) validateImage(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	if vm.Spec.ImageName == "" {
		return []string{messages.ImageNotSpecified}
	}

	return validationErrs
}

func (v validator) validateClass(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	if vm.Spec.ClassName == "" {
		return []string{messages.ClassNotSpecified}
	}

	return validationErrs
}

func (v validator) validateNetwork(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	for i, nif := range vm.Spec.NetworkInterfaces {
		if nif.NetworkName == "" {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkNameNotSpecifiedFmt, i))
		}
		switch nif.NetworkType {
		case vsphere.NsxtNetworkType, vsphere.VdsNetworkType, "":
		default:
			validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkTypeNotSupportedFmt, i))
		}
	}

	return validationErrs
}

func (v validator) validateVolumes(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	volumeNames := map[string]bool{}
	for i, volume := range vm.Spec.Volumes {
		if volume.Name != "" {
			if volumeNames[volume.Name] {
				validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNameDuplicateFmt, i))
			} else {
				volumeNames[volume.Name] = true
			}
		} else {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNameNotSpecifiedFmt, i))
		}
		if volume.PersistentVolumeClaim == nil && volume.VsphereVolume == nil {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNotSpecifiedFmt, i, i))
		}
		if volume.PersistentVolumeClaim != nil && volume.VsphereVolume != nil {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.MultipleVolumeSpecifiedFmt, i, i))
		} else if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == "" {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.PersistentVolumeClaimNameNotSpecifiedFmt, i))
		}

		if volume.VsphereVolume != nil {
			validationErrs = append(validationErrs, v.validateVsphereVolume(ctx, volume.VsphereVolume, i)...)
		}
	}

	return validationErrs
}

func (v validator) validateVsphereVolume(ctx *context.WebhookRequestContext, vsphereVolume *vmopv1.VsphereVolumeSource, specVolIndex int) []string {
	var validationErrs []string
	if vsphereVolume != nil {
		// Validate that the desired size is a multiple of a megabyte
		if _, mbMultiple := vsphereVolume.Capacity.StorageEphemeral().AsScale(resource.Mega); !mbMultiple {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VsphereVolumeSizeNotMBMultipleFmt, specVolIndex))
		}
	}
	return validationErrs
}

func (v validator) validateVmVolumeProvisioningOptions(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string
	if vm.Spec.AdvancedOptions != nil && vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
		provOpts := vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions
		if provOpts.ThinProvisioned != nil && *provOpts.ThinProvisioned && provOpts.EagerZeroed != nil && *provOpts.EagerZeroed {
			validationErrs = append(validationErrs, messages.EagerZeroedAndThinProvisionedNotSupported)
		}
	}
	return validationErrs
}

// validateAllowedChanges returns true only if immutable fields have not been modified.
// TODO BMV Exactly what is immutable?
func (v validator) validateAllowedChanges(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) []string {
	var validationErrs []string
	allowed := true

	if vm.Spec.ImageName != oldVM.Spec.ImageName {
		allowed = false
	}
	if vm.Spec.ClassName != oldVM.Spec.ClassName {
		allowed = false
	}

	if !allowed {
		validationErrs = append(validationErrs, messages.UpdatingImmutableFieldsNotAllowed)
	}

	return validationErrs
}

// vmFromUnstructured returns the VirtualMachine from the unstructured object.
func (v validator) vmFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachine, error) {
	vm := &vmopv1.VirtualMachine{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vm); err != nil {
		return nil, err
	}
	return vm, nil
}
