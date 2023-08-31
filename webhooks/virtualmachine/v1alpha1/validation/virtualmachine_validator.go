// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/pkg/errors"

	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/volume/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName                          = "default"
	storageResourceQuotaStrPattern       = ".storageclass.storage.k8s.io/"
	isRestrictedNetworkKey               = "IsRestrictedNetwork"
	allowedRestrictedNetworkTCPProbePort = 6443

	readinessProbeNoActions                   = "must specify an action"
	readinessProbeOnlyOneAction               = "only one action can be specified"
	updatesNotAllowedWhenPowerOn              = "updates to this field is not allowed when VM power is on"
	storageClassNotAssignedFmt                = "Storage policy is not associated with the namespace %s"
	storageClassNotFoundFmt                   = "Storage policy is not associated with the namespace %s"
	invalidVolumeSpecified                    = "only one of persistentVolumeClaim or vsphereVolume must be specified"
	vSphereVolumeSizeNotMBMultiple            = "value must be a multiple of MB"
	eagerZeroedAndThinProvisionedNotSupported = "Volume provisioning cannot have EagerZeroed and ThinProvisioning set. Eager zeroing requires thick provisioning"
	addingModifyingInstanceVolumesNotAllowed  = "adding or modifying instance storage volume claim(s) is not allowed"
	metadataTransportResourcesInvalid         = "%s and %s cannot be specified simultaneously"
	featureNotEnabled                         = "the %s feature is not enabled"
	invalidPowerStateOnCreateFmt              = "cannot set a new VM's power state to %s"
	invalidPowerStateOnUpdateFmt              = "cannot %s a VM that is %s"
	invalidPowerStateOnUpdateEmptyString      = "cannot set power state to empty string"
	invalidNextRestartTimeOnCreate            = "cannot restart VM on create"
	invalidNextRestartTimeOnUpdate            = "must be formatted as RFC3339Nano"
	invalidNextRestartTimeOnUpdateNow         = "mutation webhooks are required to restart VM"
	settingAnnotationNotAllowed               = "adding this annotation is not allowed"
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
	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateMetadata(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateImage(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateStorageClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVMVolumeProvisioningOptions(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validatePowerStateOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

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
	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVM, err := v.vmFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	// Check if an immutable field has been modified.
	fieldErrs = append(fieldErrs, v.validateImmutableFields(ctx, vm, oldVM)...)

	// First validate any updates to the desired state based on the current
	// power state of the VM.
	fieldErrs = append(fieldErrs, v.validatePowerStateOnUpdate(ctx, vm, oldVM)...)

	// Validations for allowed updates. Return validation responses here for
	// conditional updates regardless of whether the update is allowed or not.
	fieldErrs = append(fieldErrs, v.validateMetadata(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVMVolumeProvisioningOptions(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnUpdate(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.VmMetadata == nil {
		return allErrs
	}

	mdPath := field.NewPath("spec", "vmMetadata")

	if vm.Spec.VmMetadata.ConfigMapName != "" && vm.Spec.VmMetadata.SecretName != "" {
		allErrs = append(allErrs, field.Invalid(mdPath.Child("configMapName"), vm.Spec.VmMetadata.ConfigMapName,
			fmt.Sprintf(metadataTransportResourcesInvalid, mdPath.Child("configMapName"), mdPath.Child("secretName"))))
	}

	// Do not allow the Sysprep transport unless the FSS is enabled.
	if !lib.IsWindowsSysprepFSSEnabled() {
		if v := vmopv1.VirtualMachineMetadataSysprepTransport; v == vm.Spec.VmMetadata.Transport {
			allErrs = append(allErrs,
				field.Invalid(mdPath.Child("transport"), v,
					fmt.Sprintf(featureNotEnabled, "Sysprep")))
		}
	}

	return allErrs
}

func (v validator) validateImage(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	imageNamePath := field.NewPath("spec", "imageName")
	imageName := vm.Spec.ImageName

	if imageName == "" {
		allErrs = append(allErrs, field.Required(imageNamePath, ""))
	}

	return allErrs
}

func (v validator) validateClass(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.ClassName == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "className"), ""))
	}

	return allErrs
}

func (v validator) validateStorageClass(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.StorageClass == "" {
		return allErrs
	}

	scPath := field.NewPath("spec", "storageClass")
	scName := vm.Spec.StorageClass
	namespace := vm.Namespace

	sc := &storagev1.StorageClass{}
	if err := v.client.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		return append(allErrs, field.Invalid(scPath, scName,
			fmt.Sprintf(storageClassNotFoundFmt, namespace)))
	}

	resourceQuotas := &corev1.ResourceQuotaList{}
	if err := v.client.List(ctx, resourceQuotas, client.InNamespace(namespace)); err != nil {
		return append(allErrs, field.Invalid(scPath, scName, err.Error()))
	}

	prefix := scName + storageResourceQuotaStrPattern
	for _, resourceQuota := range resourceQuotas.Items {
		for resourceName := range resourceQuota.Spec.Hard {
			if strings.HasPrefix(resourceName.String(), prefix) {
				return nil
			}
		}
	}

	return append(allErrs, field.Invalid(scPath, scName,
		fmt.Sprintf(storageClassNotAssignedFmt, namespace)))
}

func (v validator) validateNetwork(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	networkInterfacePath := field.NewPath("spec", "networkInterfaces")
	var networkNames = map[string]struct{}{}

	for i, nif := range vm.Spec.NetworkInterfaces {
		curPath := networkInterfacePath.Index(i)
		switch nif.NetworkType {
		case network.NsxtNetworkType:
			// Empty NetworkName is allowed to let NCP pick the namespace default.
		case network.VdsNetworkType:
			// Empty NetworkName is allowed to let net-operator pick the namespace default.
		case "":
			if lib.IsNamedNetworkProviderEnabled() {
				// Allowed for testing.
			} else {
				// Disallowed in production.
				allErrs = append(
					allErrs,
					field.Invalid(
						curPath.Child("networkType"),
						"",
						"not supported in production",
					),
				)
			}
		default:
			allErrs = append(allErrs, field.NotSupported(curPath.Child("networkType"), nif.NetworkType,
				[]string{network.NsxtNetworkType, network.VdsNetworkType}))
		}

		if _, ok := networkNames[nif.NetworkName]; ok {
			allErrs = append(allErrs, field.Duplicate(curPath.Child("networkName"), nif.NetworkName))
		} else {
			networkNames[nif.NetworkName] = struct{}{}
		}

		if nif.ProviderRef != nil {
			// We only support ProviderRef with NetOP types.
			gvk := netopv1alpha1.SchemeGroupVersion.WithKind(reflect.TypeOf(netopv1alpha1.NetworkInterface{}).Name())
			if gvk.Group != nif.ProviderRef.APIGroup || gvk.Kind != nif.ProviderRef.Kind {
				allErrs = append(allErrs, field.NotSupported(curPath.Child("providerRef"), nif.ProviderRef,
					[]string{gvk.String()}))
			}
		}

		supportedEthernetCardTypes := []string{"", "pcnet32", "e1000", "e1000e", "vmxnet2", "vmxnet3"}
		supportedMap := make(map[string]bool, len(supportedEthernetCardTypes))
		for _, cardType := range supportedEthernetCardTypes {
			supportedMap[cardType] = true
		}
		if _, ok := supportedMap[nif.EthernetCardType]; !ok {
			allErrs = append(allErrs, field.NotSupported(curPath.Child("ethernetCardType"), nif.EthernetCardType,
				supportedEthernetCardTypes))
		}

	}

	return allErrs
}

// validateInstanceStorageVolumes validates if instance storage volumes are added/modified.
// The instance storage volumes should to be validated irrespective of WCP_Instance_Storage FSS status,
// as we don't allow any users other than privileged users to add/modify instance storage volumes even if FSS is disabled.
func (v validator) validateInstanceStorageVolumes(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	var oldVMInstanceStorageVolumes []vmopv1.VirtualMachineVolume
	if oldVM != nil {
		oldVMInstanceStorageVolumes = instancestorage.FilterVolumes(oldVM)
	}

	if !equality.Semantic.DeepEqual(instancestorage.FilterVolumes(vm), oldVMInstanceStorageVolumes) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "volumes"), addingModifyingInstanceVolumesNotAllowed))
	}

	return allErrs
}

func (v validator) validateVolumes(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	volumesPath := field.NewPath("spec", "volumes")
	volumeNames := map[string]bool{}

	for i, vol := range vm.Spec.Volumes {
		curVolPath := volumesPath.Index(i)
		curVolNamePath := curVolPath.Child("name")
		if vol.Name != "" {
			if volumeNames[vol.Name] {
				allErrs = append(allErrs, field.Duplicate(curVolNamePath, vol.Name))
			} else {
				volumeNames[vol.Name] = true
			}
		} else {
			allErrs = append(allErrs, field.Required(curVolNamePath, ""))
		}

		if (vol.PersistentVolumeClaim == nil && vol.VsphereVolume == nil) ||
			(vol.PersistentVolumeClaim != nil && vol.VsphereVolume != nil) {
			allErrs = append(allErrs, field.Forbidden(curVolPath, invalidVolumeSpecified))
			continue
		}

		if vol.PersistentVolumeClaim != nil {
			allErrs = append(allErrs, v.validateVolumeWithPVC(ctx, vm, vol, curVolPath)...)
		} else { // vol.VsphereVolume != nil
			allErrs = append(allErrs, v.validateVsphereVolume(vol.VsphereVolume, curVolPath)...)
		}
	}

	return allErrs
}

func (v validator) validateVolumeWithPVC(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine,
	vol vmopv1.VirtualMachineVolume, volPath *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	// Check that the name used for the CnsNodeVmAttachment will be valid. Don't double up errors if name is missing.
	if vol.Name != "" {
		errs := validation.NameIsDNSSubdomain(v1alpha1.CNSAttachmentNameForVolume(vm, vol.Name), false)
		for _, msg := range errs {
			allErrs = append(allErrs, field.Invalid(volPath.Child("name"), vol.Name, msg))
		}
	}

	pvcPath := volPath.Child("persistentVolumeClaim")
	pvcSource := vol.PersistentVolumeClaim
	if pvcSource.ClaimName == "" {
		allErrs = append(allErrs, field.Required(pvcPath.Child("claimName"), ""))
	}
	if pvcSource.ReadOnly {
		allErrs = append(allErrs, field.NotSupported(pvcPath.Child("readOnly"), pvcSource.ReadOnly,
			[]string{"false"}))
	}

	return allErrs
}

func (v validator) validateVsphereVolume(vsphereVolume *vmopv1.VsphereVolumeSource,
	volPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	fieldPath := volPath.Child("vsphereVolume", "capacity", "ephemeral-storage")

	// Validate that the desired size is a multiple of a megabyte
	megaByte := resource.MustParse("1Mi")
	if vsphereVolume.Capacity.StorageEphemeral().Value()%megaByte.Value() != 0 {
		allErrs = append(allErrs, field.Invalid(fieldPath, vsphereVolume.Capacity.StorageEphemeral(),
			vSphereVolumeSizeNotMBMultiple))
	}

	return allErrs
}

func (v validator) validateVMVolumeProvisioningOptions(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.AdvancedOptions != nil && vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
		provOpts := vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions
		if provOpts.ThinProvisioned != nil && *provOpts.ThinProvisioned && provOpts.EagerZeroed != nil && *provOpts.EagerZeroed {
			fieldPath := field.NewPath("spec", "advancedOptions", "defaultVolumeProvisioningOptions")
			allErrs = append(allErrs, field.Forbidden(fieldPath, eagerZeroedAndThinProvisionedNotSupported))
		}
	}

	return allErrs
}

func (v validator) validateReadinessProbe(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	probe := vm.Spec.ReadinessProbe
	if probe == nil {
		return allErrs
	}

	readinessProbePath := field.NewPath("spec", "readinessProbe")

	if probe.TCPSocket == nil && probe.GuestHeartbeat == nil {
		allErrs = append(allErrs, field.Forbidden(readinessProbePath, readinessProbeNoActions))
	} else if probe.TCPSocket != nil && probe.GuestHeartbeat != nil {
		allErrs = append(allErrs, field.Forbidden(readinessProbePath, readinessProbeOnlyOneAction))
	}

	// Validate the TCP probe if set and environment is a restricted network environment between CP VMs and Workload VMs e.g. VMC
	if probe.TCPSocket != nil {
		tcpSocketPath := readinessProbePath.Child("tcpSocket")
		isRestrictedEnv, err := v.isNetworkRestrictedForReadinessProbe(ctx)
		if err != nil {
			allErrs = append(allErrs, field.Forbidden(tcpSocketPath, err.Error()))
		} else if isRestrictedEnv && probe.TCPSocket.Port.IntValue() != allowedRestrictedNetworkTCPProbePort {
			allErrs = append(allErrs, field.NotSupported(tcpSocketPath.Child("port"), probe.TCPSocket.Port.IntValue(),
				[]string{strconv.Itoa(allowedRestrictedNetworkTCPProbePort)}))
		}
	}

	return allErrs
}

func (v validator) validateNextRestartTimeOnCreate(
	ctx *context.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if vm.Spec.NextRestartTime != "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec").Child("nextRestartTime"),
				vm.Spec.NextRestartTime,
				invalidNextRestartTimeOnCreate))
	}

	return allErrs
}

func (v validator) validateNextRestartTimeOnUpdate(
	ctx *context.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if newVM.Spec.NextRestartTime == oldVM.Spec.NextRestartTime {
		return nil
	}

	var allErrs field.ErrorList

	nextRestartTimePath := field.NewPath("spec").Child("nextRestartTime")

	if strings.EqualFold(newVM.Spec.NextRestartTime, "now") {
		allErrs = append(
			allErrs,
			field.Invalid(
				nextRestartTimePath,
				newVM.Spec.NextRestartTime,
				invalidNextRestartTimeOnUpdateNow))
	} else if _, err := time.Parse(time.RFC3339Nano, newVM.Spec.NextRestartTime); err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				nextRestartTimePath,
				newVM.Spec.NextRestartTime,
				invalidNextRestartTimeOnUpdate))
	}

	return allErrs
}

func (v validator) validatePowerStateOnCreate(
	ctx *context.WebhookRequestContext,
	newVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs       field.ErrorList
		newPowerState = newVM.Spec.PowerState
	)

	if newPowerState == vmopv1.VirtualMachineSuspended {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec").Child("powerState"),
				newPowerState,
				fmt.Sprintf(invalidPowerStateOnCreateFmt, newPowerState)))
	}

	return allErrs
}

func (v validator) validatePowerStateOnUpdate(
	ctx *context.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	powerStatePath := field.NewPath("spec").Child("powerState")

	// If a VM is powered off, all config changes are allowed except for setting
	// the power state to shutdown or suspend.
	// If a VM is requesting a power off, we can Reconfigure the VM _after_
	// we power it off - all changes are allowed.
	// If a VM is requesting a power on, we can Reconfigure the VM _before_
	// we power it on - all changes are allowed.
	newPowerState, oldPowerState := newVM.Spec.PowerState, oldVM.Spec.PowerState

	if newPowerState == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				powerStatePath,
				newPowerState,
				invalidPowerStateOnUpdateEmptyString))
		return allErrs
	}

	switch oldPowerState {
	case vmopv1.VirtualMachinePoweredOn:
		// The request is attempting to power on a VM that is already powered
		// on. Validate fields that may or may not be mutable while a VM is in
		// a powered on state.
		if newPowerState == vmopv1.VirtualMachinePoweredOn {
			invalidFields := v.validateUpdatesWhenPoweredOn(ctx, newVM, oldVM)
			allErrs = append(allErrs, invalidFields...)
		}

	case vmopv1.VirtualMachinePoweredOff:
		// Mark the power state as invalid if the request is attempting to
		// suspend a VM that is powered off.
		var msg string
		if newPowerState == vmopv1.VirtualMachineSuspended {
			msg = "suspend"
		}
		if msg != "" {
			allErrs = append(
				allErrs,
				field.Invalid(
					powerStatePath,
					newPowerState,
					fmt.Sprintf(invalidPowerStateOnUpdateFmt, msg, "powered off")))
		}
	}

	return allErrs
}

func (v validator) validateUpdatesWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	specPath := field.NewPath("spec")

	if !equality.Semantic.DeepEqual(vm.Spec.Ports, oldVM.Spec.Ports) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("ports"), updatesNotAllowedWhenPowerOn))
	}
	if !equality.Semantic.DeepEqual(vm.Spec.VmMetadata, oldVM.Spec.VmMetadata) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("vmMetadata"), updatesNotAllowedWhenPowerOn))
	}
	// AKP: This would fail if the API server shuffles the Spec up before passing it to the webhook. Should this be a sorted compare? Same for Volumes.
	if !equality.Semantic.DeepEqual(vm.Spec.NetworkInterfaces, oldVM.Spec.NetworkInterfaces) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("networkInterfaces"), updatesNotAllowedWhenPowerOn))
	}

	if vm.Spec.AdvancedOptions != nil {
		allErrs = append(allErrs, v.validateAdvancedOptionsUpdateWhenPoweredOn(ctx, vm, oldVM)...)
	}

	if vm.Spec.Volumes != nil {
		allErrs = append(allErrs, v.validateVsphereVolumesUpdateWhenPoweredOn(ctx, vm, oldVM)...)
	}

	return allErrs
}

// validateAdvancedOptionsUpdateWhenPoweredOn validates that AdvancedOptions update request is valid when the VM is powered on.
// We do not reconcile DefaultVolumeProvisioningOptions, so ANY updates to those are denied.
func (v validator) validateAdvancedOptionsUpdateWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	fieldPath := field.NewPath("spec", "advancedOptions", "defaultVolumeProvisioningOptions")

	if vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
		if oldVM.Spec.AdvancedOptions == nil || oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions == nil {
			// Newly added default provisioning options.
			allErrs = append(allErrs, field.Forbidden(fieldPath, updatesNotAllowedWhenPowerOn))
		} else if oldVM.Spec.AdvancedOptions != nil && oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions != nil {
			// Updated default provisioning options.
			if !equality.Semantic.DeepEqual(vm.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions, oldVM.Spec.AdvancedOptions.DefaultVolumeProvisioningOptions) {
				allErrs = append(allErrs, field.Forbidden(fieldPath, updatesNotAllowedWhenPowerOn))
			}
		}
	}

	return allErrs
}

// validateVsphereVolumesUpdateWhenPoweredOn validates that Volume update request is valid when the VM is powered on.
// We do not support any modifications to vSphere volumes while the VM is powered on.
func (v validator) validateVsphereVolumesUpdateWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	fieldPath := field.NewPath("spec", "volumes").Key("VsphereVolume")

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
		allErrs = append(allErrs, field.Forbidden(fieldPath, updatesNotAllowedWhenPowerOn))
	}

	return allErrs
}

func (v validator) validateImmutableFields(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	specPath := field.NewPath("spec")

	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.ImageName, oldVM.Spec.ImageName, specPath.Child("imageName"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.ClassName, oldVM.Spec.ClassName, specPath.Child("className"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.StorageClass, oldVM.Spec.StorageClass, specPath.Child("storageClass"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.ResourcePolicyName, oldVM.Spec.ResourcePolicyName, specPath.Child("resourcePolicyName"))...)

	return allErrs
}

func (v validator) validateAvailabilityZone(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if !lib.IsWcpFaultDomainsFSSEnabled() {
		return allErrs
	}

	zoneLabelPath := field.NewPath("metadata", "labels").Key(topology.KubernetesTopologyZoneLabelKey)

	if oldVM != nil {
		// Once the zone has been set then make sure the field is immutable.
		if oldVal := oldVM.Labels[topology.KubernetesTopologyZoneLabelKey]; oldVal != "" {
			newVal := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
			return append(allErrs, validation.ValidateImmutableField(newVal, oldVal, zoneLabelPath)...)
		}
	}

	// Validate the name of the provided availability zone.
	if zone := vm.Labels[topology.KubernetesTopologyZoneLabelKey]; zone != "" {
		if _, err := topology.GetAvailabilityZone(ctx.Context, v.client, zone); err != nil {
			return append(allErrs, field.Invalid(zoneLabelPath, zone, err.Error()))
		}
	}

	return allErrs
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
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{Name: config.ProviderConfigMapName, Namespace: ctx.Namespace}
	if err := v.client.Get(ctx, configMapKey, configMap); err != nil {
		return false, fmt.Errorf("error fetching config map: %s while validating TCP readiness probe port: %v", configMapKey, err)
	}

	return configMap.Data[isRestrictedNetworkKey] == "true", nil
}

func (v validator) validateAnnotation(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount || vm.Annotations == nil {
		return allErrs
	}

	annotationPath := field.NewPath("metadata", "annotations")

	if _, exists := vm.Annotations[vmopv1.InstanceIDAnnotation]; exists {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), settingAnnotationNotAllowed))
	}

	return allErrs
}
