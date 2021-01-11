// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/pkg/errors"
	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/volume"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

const (
	webHookName                    = "default"
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha1,name=default.validating.virtualmachine.vmoperator.vmware.com,sideEffects=None
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachine validation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client client.Client) builder.Validator {
	return validator{
		client: client,
		// TODO BMV Use the Context.scheme instead
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    client.Client
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
	validationErrs = append(validationErrs, v.validateStorageClass(ctx, vm)...)
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

	val := vm.Annotations[vsphere.VMOperatorVMGOSSupportCheckKey]

	if val != vsphere.VMOperatorVMGOSSupportDisable {
		image := vmopv1.VirtualMachineImage{}
		err := v.client.Get(ctx, types.NamespacedName{Name: vm.Spec.ImageName}, &image)
		if err != nil {
			validationErrs = append(validationErrs, fmt.Sprintf("error validating image: %v", err))
			return validationErrs
		}

		if image.Status.SupportedGuestOS != nil && !*image.Status.SupportedGuestOS {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.GuestOSNotSupported, image.Spec.OSInfo.Type, image.Name))
		}
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

func (v validator) validateStorageClass(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	if vm.Spec.StorageClass == "" {
		return nil
	}

	var validationErrs []string
	scName := vm.Spec.StorageClass
	namespace := vm.Namespace

	resourceQuotas := &v1.ResourceQuotaList{}
	if err := v.client.List(ctx, resourceQuotas, client.InNamespace(namespace)); err != nil {
		validationErrs = append(validationErrs, fmt.Sprintf("error validating storageClass: %v", err))
		return validationErrs
	}

	if len(resourceQuotas.Items) == 0 {
		validationErrs = append(validationErrs, fmt.Sprintf("no ResourceQuotas assigned to namespace %s", namespace))
		return validationErrs
	}

	prefix := scName + storageResourceQuotaStrPattern
	for _, resourceQuota := range resourceQuotas.Items {
		for resourceName := range resourceQuota.Spec.Hard {
			if strings.HasPrefix(resourceName.String(), prefix) {
				return validationErrs
			}
		}
	}
	validationErrs = append(validationErrs, fmt.Sprintf("StorageClass %s is not assigned to any ResourceQuotas in namespace %s", scName, namespace))
	return validationErrs
}

func (v validator) validateNetwork(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string
	var networkNames = map[string]struct{}{}

	for i, nif := range vm.Spec.NetworkInterfaces {
		switch nif.NetworkType {
		case vsphere.NsxtNetworkType, "":
		case vsphere.VdsNetworkType:
			if nif.NetworkName == "" {
				validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkNameNotSpecifiedFmt, i))
			}
		default:
			validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkTypeNotSupportedFmt, i))
		}
		if _, ok := networkNames[nif.NetworkName]; ok {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.MultipleNetworkInterfacesNotSupportedFmt, i))
		}
		networkNames[nif.NetworkName] = struct{}{}
	}

	return validationErrs
}

func (v validator) validateVolumes(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	volumeNames := map[string]bool{}
	for i, vol := range vm.Spec.Volumes {
		if vol.Name != "" {
			if volumeNames[vol.Name] {
				validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNameDuplicateFmt, i))
			} else {
				volumeNames[vol.Name] = true
			}
		} else {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNameNotSpecifiedFmt, i))
		}

		if vol.PersistentVolumeClaim == nil && vol.VsphereVolume == nil {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNotSpecifiedFmt, i, i))
		} else if vol.PersistentVolumeClaim != nil && vol.VsphereVolume != nil {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.MultipleVolumeSpecifiedFmt, i, i))
		} else {
			if vol.PersistentVolumeClaim != nil {
				validationErrs = append(validationErrs, v.validateVolumeWithPVC(vm, vol, i)...)
			} else { // vol.VsphereVolume != nil
				validationErrs = append(validationErrs, v.validateVsphereVolume(vol.VsphereVolume, i)...)
			}
		}
	}

	return validationErrs
}

func (v validator) validateVolumeWithPVC(vm *vmopv1.VirtualMachine, vol vmopv1.VirtualMachineVolume, idx int) []string {
	var validationErrs []string

	// Check that the name used for the CnsNodeVmAttachment will be valid. Don't double up errors if name is missing.
	if vol.Name != "" {
		errs := validation.IsDNS1123Subdomain(volume.CNSAttachmentNameForVolume(vm, vol.Name))
		if len(errs) > 0 {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.VolumeNameNotValidObjectNameFmt, idx, strings.Join(errs, ",")))
		}
	}

	pvcSource := vol.PersistentVolumeClaim
	if pvcSource.ClaimName == "" {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.PersistentVolumeClaimNameNotSpecifiedFmt, idx))
	}
	if pvcSource.ReadOnly {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.PersistentVolumeClaimNameReadOnlyFmt, idx))
	}

	return validationErrs
}

func (v validator) validateVsphereVolume(vsphereVolume *vmopv1.VsphereVolumeSource, idx int) []string {
	var validationErrs []string

	// Validate that the desired size is a multiple of a megabyte
	megaByte := resource.MustParse("1Mi")
	if vsphereVolume.Capacity.StorageEphemeral().Value()%megaByte.Value() != 0 {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.VsphereVolumeSizeNotMBMultipleFmt, idx))
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
	var validationErrs, fieldNames []string
	allowed := true

	if vm.Spec.ImageName != oldVM.Spec.ImageName {
		allowed = false
		fieldNames = append(fieldNames, "Spec.ImageName")
	}
	if vm.Spec.ClassName != oldVM.Spec.ClassName {
		allowed = false
		fieldNames = append(fieldNames, "Spec.ClassName")
	}
	if vm.Spec.StorageClass != oldVM.Spec.StorageClass {
		allowed = false
		fieldNames = append(fieldNames, "Spec.StorageClass")
	}

	if !allowed {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.UpdatingImmutableFieldsNotAllowed, fieldNames))
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
