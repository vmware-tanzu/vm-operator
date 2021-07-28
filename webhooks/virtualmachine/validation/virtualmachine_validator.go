// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

const (
	webHookName                          = "default"
	storageResourceQuotaStrPattern       = ".storageclass.storage.k8s.io/"
	isRestrictedNetworkKey               = "IsRestrictedNetwork"
	allowedRestrictedNetworkTCPProbePort = 6443
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha1,name=default.validating.virtualmachine.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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
	validationErrs = append(validationErrs, v.validateAvailabilityZone(ctx, vm, nil)...)
	validationErrs = append(validationErrs, v.validateImage(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateClass(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateStorageClass(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateNetwork(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVolumes(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVmVolumeProvisioningOptions(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateReadinessProbe(ctx, vm)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate validates if the given VirtualMachineSpec update is valid.
// Updates to following fields are not allowed:
//   - ImageName
//   - ClassName
//   - StorageClass
//   - ResourcePolicyName

// Following fields can only be updated when the VM is powered off.
//   - Ports
//   - VmMetaData
//   - NetworkInterfaces
//   - Volumes referencing a VsphereVolume
//   - AdvancedOptions
//     - DefaultVolumeProvisioningOptions

// All other updates are allowed.
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

	// Check if an immutable field has been modified.
	validationErrs = append(validationErrs, v.validateImmutableFields(ctx, vm, oldVM)...)

	currentPowerState := oldVM.Spec.PowerState
	desiredPowerState := vm.Spec.PowerState

	// If a VM is powered off, all config changes are allowed.
	// If a VM is requesting a power off, we can Reconfigure the VM _after_ we power it off - all changes are allowed.
	// If a VM is requesting a power on, we can Reconfigure the VM _before_ we power it on - all changes are allowed.
	// So, we only run these validations when the VM is powered on, and is not requesting a power state change.
	if currentPowerState == desiredPowerState && currentPowerState == vmopv1.VirtualMachinePoweredOn {
		invalidFields := v.validateUpdatesWhenPoweredOn(ctx, vm, oldVM)
		if len(invalidFields) > 0 {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerStateFmt, invalidFields, vm.Spec.PowerState))
		}
	}

	// Validations for allowed updates. Return validation responses here for conditional updates regardless
	// of whether the update is allowed or not.
	validationErrs = append(validationErrs, v.validateMetadata(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateAvailabilityZone(ctx, vm, oldVM)...)
	validationErrs = append(validationErrs, v.validateNetwork(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVolumes(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateVmVolumeProvisioningOptions(ctx, vm)...)
	validationErrs = append(validationErrs, v.validateReadinessProbe(ctx, vm)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	var validationErrs []string

	if vm.Spec.VmMetadata == nil {
		return validationErrs
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

	vmoperatorImageSupportedCheck := vm.Annotations[constants.VMOperatorImageSupportedCheckKey]
	virtualMachineMetadataTransport := vm.Spec.VmMetadata.Transport

	if vmoperatorImageSupportedCheck == constants.VMOperatorImageSupportedCheckDisable {
		return validationErrs
	}
	if virtualMachineMetadataTransport == vmopv1.VirtualMachineMetadataCloudInitTransport {
		return validationErrs
	}

	image := vmopv1.VirtualMachineImage{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: vm.Spec.ImageName}, &image); err != nil {
		validationErrs = append(validationErrs, fmt.Sprintf("error validating image: %v", err))
		return validationErrs
	}
	if image.Status.ImageSupported != nil && !*image.Status.ImageSupported {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.VirtualMachineImageNotSupported))
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
		validationErrs = append(validationErrs, fmt.Sprintf(messages.NoResourceQuotaFmt, namespace))
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
		case network.NsxtNetworkType:
			// Empty NetworkName is allowed to let NCP pick the namespace default.
		case network.VdsNetworkType:
			if nif.NetworkName == "" {
				validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkNameNotSpecifiedFmt, i))
			}
		case "":
			// Must unfortunately allow for testing.
		default:
			validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkTypeNotSupportedFmt, i, network.NsxtNetworkType, network.VdsNetworkType))
		}

		if _, ok := networkNames[nif.NetworkName]; ok {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.MultipleNetworkInterfacesNotSupportedFmt, i))
		} else {
			networkNames[nif.NetworkName] = struct{}{}
		}

		if nif.ProviderRef != nil {
			// We only support ProviderRef with NetOP types.
			gvk := netopv1alpha1.SchemeGroupVersion.WithKind(reflect.TypeOf(netopv1alpha1.NetworkInterface{}).Name())
			if gvk.Group != nif.ProviderRef.APIGroup || gvk.Kind != nif.ProviderRef.Kind {
				validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkTypeProviderRefNotSupportedFmt, i))
			}
		}

		switch nif.EthernetCardType {
		case "", "pcnet32", "e1000", "e1000e", "vmxnet2", "vmxnet3":
		default:
			validationErrs = append(validationErrs, fmt.Sprintf(messages.NetworkTypeEthCardTypeNotSupportedFmt, i))
		}
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
				validationErrs = append(validationErrs, v.validateVolumeWithPVC(ctx, vm, vol, i)...)
			} else { // vol.VsphereVolume != nil
				validationErrs = append(validationErrs, v.validateVsphereVolume(vol.VsphereVolume, i)...)
			}
		}
	}

	return validationErrs
}

func (v validator) validateVolumeWithPVC(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine, vol vmopv1.VirtualMachineVolume, idx int) []string {
	var validationErrs []string

	image := vmopv1.VirtualMachineImage{}
	err := v.client.Get(ctx, types.NamespacedName{Name: vm.Spec.ImageName}, &image)
	if err != nil {
		validationErrs = append(validationErrs, fmt.Sprintf("error validating image for PVC: %v", err))
	}

	// Check that the VirtualMachineImage's hardware version is at least the minimum supported virtual hardware version
	if image.Spec.HardwareVersion != 0 && image.Spec.HardwareVersion < constants.MinSupportedHWVersionForPVC {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.PersistentVolumeClaimHardwareVersionNotSupportedFmt,
			image.Name, image.Spec.HardwareVersion, constants.MinSupportedHWVersionForPVC))
	}

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

func (v validator) validateReadinessProbe(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) []string {
	probe := vm.Spec.ReadinessProbe
	if probe == nil {
		return nil
	}

	var validationErrs []string

	if probe.TCPSocket == nil && probe.GuestHeartbeat == nil {
		validationErrs = append(validationErrs, messages.ReadinessProbeNoActions)
	} else if probe.TCPSocket != nil && probe.GuestHeartbeat != nil {
		validationErrs = append(validationErrs, messages.ReadinessProbeOnlyOneAction)
	}

	// Validate the TCP probe if set and environment is a restricted network environment between CP VMs and Workload VMs e.g. VMC
	if probe.TCPSocket != nil {
		isRestrictedEnv, err := v.isNetworkRestrictedForReadinessProbe(ctx)
		if err != nil {
			validationErrs = append(validationErrs, err.Error())
		} else if isRestrictedEnv && probe.TCPSocket.Port.IntValue() != allowedRestrictedNetworkTCPProbePort {
			validationErrs = append(validationErrs, fmt.Sprintf(messages.ReadinessProbePortNotSupportedFmt, allowedRestrictedNetworkTCPProbePort))
		}
	}
	return validationErrs
}

func (v validator) validateUpdatesWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) []string {
	var fieldNames []string

	if !equality.Semantic.DeepEqual(vm.Spec.Ports, oldVM.Spec.Ports) {
		fieldNames = append(fieldNames, "spec.ports")
	}
	if !equality.Semantic.DeepEqual(vm.Spec.VmMetadata, oldVM.Spec.VmMetadata) {
		fieldNames = append(fieldNames, "spec.vmMetadata")
	}
	// AKP: This would fail if the API server shuffles the Spec up before passing it to the webhook. Should this be a sorted compare? Same for Volumes.
	if !equality.Semantic.DeepEqual(vm.Spec.NetworkInterfaces, oldVM.Spec.NetworkInterfaces) {
		fieldNames = append(fieldNames, "spec.networkInterfaces")
	}

	if vm.Spec.AdvancedOptions != nil {
		fieldNames = append(fieldNames, v.validateAdvancedOptionsUpdateWhenPoweredOn(ctx, vm, oldVM)...)
	}

	if vm.Spec.Volumes != nil {
		fieldNames = append(fieldNames, v.validateVsphereVolumesUpdateWhenPoweredOn(ctx, vm, oldVM)...)
	}

	return fieldNames
}

// validateAdvancedOptionsUpdateWhenPoweredOn validates that AdvancedOptions update request is valid when the VM is powered on.
// We do not reconcile DefaultVolumeProvisioningOptions, so ANY updates to those are denied.
func (v validator) validateAdvancedOptionsUpdateWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) []string {
	var fieldNames []string
	defaultVolumeProvisioningOptionsKey := "spec.advancedOptions.defaultVolumeProvisioningOptions"

	if vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
		if oldVM.Spec.AdvancedOptions == nil || oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions == nil {
			// Newly added default provisioning options.
			fieldNames = append(fieldNames, defaultVolumeProvisioningOptionsKey)
		} else if oldVM.Spec.AdvancedOptions != nil && oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
			// Updated default provisioning options.
			if !equality.Semantic.DeepEqual(vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions, oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions) {
				fieldNames = append(fieldNames, defaultVolumeProvisioningOptionsKey)
			}
		}
	}

	return fieldNames
}

// validateVsphereVolumesUpdateWhenPoweredOn validates that Volume update request is valid when the VM is powered on.
// We do not support any modifications to vSphere volumes while the VM is powered on.
func (v validator) validateVsphereVolumesUpdateWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) []string {
	volumesKey := "spec.volumes[VsphereVolume]"
	var fieldNames []string

	// Do not allow modifications to vSphere Volumes while the VM is powered on.
	oldvSphereVolumes := make(map[string]vmopv1.VirtualMachineVolume)
	for _, vol := range oldVM.Spec.Volumes {
		if vol.VsphereVolume != nil {
			oldvSphereVolumes[vol.Name] = vol
		}
	}

	newvSphereVolumes := make(map[string]vmopv1.VirtualMachineVolume)
	for _, vol := range vm.Spec.Volumes {
		if vol.VsphereVolume != nil {
			newvSphereVolumes[vol.Name] = vol
		}
	}

	if !reflect.DeepEqual(oldvSphereVolumes, newvSphereVolumes) {
		fieldNames = append(fieldNames, volumesKey)
	}

	return fieldNames
}

func (v validator) validateImmutableFields(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) []string {
	var validationErrs, fieldNames []string

	if vm.Spec.ImageName != oldVM.Spec.ImageName {
		fieldNames = append(fieldNames, "spec.imageName")
	}
	if vm.Spec.ClassName != oldVM.Spec.ClassName {
		fieldNames = append(fieldNames, "spec.className")
	}
	if vm.Spec.StorageClass != oldVM.Spec.StorageClass {
		fieldNames = append(fieldNames, "spec.storageClass")
	}
	if vm.Spec.ResourcePolicyName != oldVM.Spec.ResourcePolicyName {
		fieldNames = append(fieldNames, "spec.resourcePolicyName")
	}

	if len(fieldNames) > 0 {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.UpdatingImmutableFieldsNotAllowedFmt, fieldNames))
	}

	return validationErrs
}

func (v validator) validateAvailabilityZone(
	ctx *context.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) []string {

	// If there is an oldVM in play then make sure the field is immutable.
	if oldVM != nil {
		if vm.Labels[topology.KubernetesTopologyZoneLabelKey] != oldVM.Labels[topology.KubernetesTopologyZoneLabelKey] {
			return []string{
				fmt.Sprintf(
					messages.UpdatingImmutableFieldsNotAllowedFmt,
					[]string{"metadata.labels." + topology.KubernetesTopologyZoneLabelKey}),
			}
		}
	}

	// Validate the name of the provided availability zone.
	if zone := vm.Labels[topology.KubernetesTopologyZoneLabelKey]; zone != "" {
		if _, err := topology.GetAvailabilityZone(
			ctx.Context, v.client, zone); err != nil {
			return []string{
				fmt.Sprintf(messages.MetadataInvalidAvailabilityZone, zone),
			}
		}
	}

	return nil
}

// vmFromUnstructured returns the VirtualMachine from the unstructured object.
func (v validator) vmFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachine, error) {
	vm := &vmopv1.VirtualMachine{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vm); err != nil {
		return nil, err
	}
	return vm, nil
}

func (v validator) isNetworkRestrictedForReadinessProbe(ctx *context.WebhookRequestContext) (bool, error) {
	vmopNamespace, err := lib.GetVmOpNamespaceFromEnv()
	if err != nil {
		return false, fmt.Errorf("error fetching VMOpNamespace while validating TCP readiness probe port: %v", err)
	}
	configMap := &v1.ConfigMap{}
	configMapKey := types.NamespacedName{Name: config.ProviderConfigMapName, Namespace: vmopNamespace}
	err = v.client.Get(ctx, configMapKey, configMap)
	if err != nil {
		return false, fmt.Errorf("error fetching config map: %s while validating TCP readiness probe port: %v", config.ProviderConfigMapName, err)
	}
	restrictedNetworkEnv := configMap.Data[isRestrictedNetworkKey]
	return restrictedNetworkEnv == "true", nil
}
