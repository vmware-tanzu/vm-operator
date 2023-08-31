// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net"
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	volume "github.com/vmware-tanzu/vm-operator/controllers/volume/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName                          = "default"
	storageResourceQuotaStrPattern       = ".storageclass.storage.k8s.io/"
	isRestrictedNetworkKey               = "IsRestrictedNetwork"
	allowedRestrictedNetworkTCPProbePort = 6443

	readinessProbeOnlyOneAction              = "only one action can be specified"
	updatesNotAllowedWhenPowerOn             = "updates to this field is not allowed when VM power is on"
	storageClassNotAssignedFmt               = "Storage policy is not associated with the namespace %s"
	storageClassNotFoundFmt                  = "Storage policy is not associated with the namespace %s"
	vSphereVolumeSizeNotMBMultiple           = "value must be a multiple of MB"
	addingModifyingInstanceVolumesNotAllowed = "adding or modifying instance storage volume claim(s) is not allowed"
	featureNotEnabled                        = "the %s feature is not enabled"
	invalidPowerStateOnCreateFmt             = "cannot set a new VM's power state to %s"
	invalidPowerStateOnUpdateFmt             = "cannot %s a VM that is %s"
	invalidPowerStateOnUpdateEmptyString     = "cannot set power state to empty string"
	invalidNextRestartTimeOnCreate           = "cannot restart VM on create"
	invalidNextRestartTimeOnUpdate           = "must be formatted as RFC3339Nano"
	invalidNextRestartTimeOnUpdateNow        = "mutation webhooks are required to restart VM"
	settingAnnotationNotAllowed              = "adding this annotation is not allowed"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha2-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha2,name=default.validating.virtualmachine.v1alpha2.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrap(err, "failed to create VirtualMachine validation webhook")
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

	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateImage(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateStorageClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
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
// Changes to following fields are not allowed:
//   - ImageName
//   - ClassName
//   - StorageClass
//   - ResourcePolicyName
//
// Following fields can only be changed when the VM is powered off.
//   - Bootstrap
//   - Network
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

	// Validations for allowed updates. Return validation responses here for conditional updates regardless
	// of whether the update is allowed or not.
	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnUpdate(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateBootstrap(
	ctx *context.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	bootstrapPath := field.NewPath("spec", "bootstrap")

	cloudInit := vm.Spec.Bootstrap.CloudInit
	linuxPrep := vm.Spec.Bootstrap.LinuxPrep
	sysPrep := vm.Spec.Bootstrap.Sysprep
	vAppConfig := vm.Spec.Bootstrap.VAppConfig

	if cloudInit != nil {
		p := bootstrapPath.Child("cloudInit")

		if linuxPrep != nil || sysPrep != nil || vAppConfig != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"CloudInit may not be used with any other bootstrap provider"))
		}

		hasCloudConfig := !equality.Semantic.DeepEqual(cloudInit.CloudConfig, cloudinit.CloudConfig{})
		hasRawCloudConfig := !equality.Semantic.DeepEqual(cloudInit.RawCloudConfig, corev1.SecretKeySelector{})
		if hasCloudConfig && hasRawCloudConfig {
			allErrs = append(allErrs, field.Invalid(p, cloudInit,
				"cloudConfig and rawCloudConfig are mutually exclusive"))
		}
	}

	if linuxPrep != nil {
		p := bootstrapPath.Child("linuxPrep")

		if cloudInit != nil || sysPrep != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"LinuxPrep may not be used with either CloudInit or Sysprep bootstrap providers"))
		}
	}

	if sysPrep != nil {
		p := bootstrapPath.Child("sysprep")

		if cloudInit != nil || linuxPrep != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"Sysprep may not be used with either CloudInit or LinuxPrep bootstrap providers"))
		}

		if lib.IsWindowsSysprepFSSEnabled() {
			hasSysPrep := !equality.Semantic.DeepEqual(sysPrep.Sysprep, sysprep.Sysprep{})
			hasRawSysPrep := !equality.Semantic.DeepEqual(sysPrep.RawSysprep, corev1.SecretKeySelector{})
			if hasSysPrep && hasRawSysPrep {
				allErrs = append(allErrs, field.Invalid(p, sysPrep,
					"sysprep and rawSysprep are mutually exclusive"))
			}
		} else {
			allErrs = append(allErrs, field.Invalid(p, "sysprep", fmt.Sprintf(featureNotEnabled, "sysprep")))
		}
	}

	if vAppConfig != nil {
		p := bootstrapPath.Child("vAppConfig")

		if cloudInit != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"vAppConfig may not be used in conjunction with CloudInit bootstrap provider"))
		}

		if len(vAppConfig.Properties) != 0 && len(vAppConfig.RawProperties) != 0 {
			allErrs = append(allErrs, field.TypeInvalid(p, vAppConfig,
				"properties and rawProperties are mutually exclusive"))
		}
	}

	return allErrs
}

func (v validator) validateImage(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.ImageName == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "imageName"), ""))
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

	// TODO: This validation shouldn't be done in the webhook.

	sc := &storagev1.StorageClass{}
	if err := v.client.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storageClassNotFoundFmt, vm.Namespace)))
	}

	resourceQuotas := &corev1.ResourceQuotaList{}
	if err := v.client.List(ctx, resourceQuotas, client.InNamespace(vm.Namespace)); err != nil {
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

	return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storageClassNotAssignedFmt, vm.Namespace)))
}

func (v validator) validateNetwork(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	networkSpec := &vm.Spec.Network
	networkPath := field.NewPath("spec", "network")

	var hasIPv4Address, hasIPv6Address bool
	for i, ipCIDR := range networkSpec.Addresses {
		ip, _, err := net.ParseCIDR(ipCIDR)
		if err != nil {
			p := networkPath.Child("addresses").Index(i)
			allErrs = append(allErrs, field.Invalid(p, ipCIDR, err.Error()))
			continue
		}

		hasIPv4Address = hasIPv4Address || ip.To4() != nil
		hasIPv6Address = hasIPv6Address || ip.To16() != nil
	}

	if networkSpec.DHCP4 {
		if hasIPv4Address {
			p := networkPath.Child("dhcp4")
			allErrs = append(allErrs, field.Invalid(p, networkSpec.Addresses,
				"dhcp4 cannot be used with IP4 addresses in network.addresses field"))
		}

		if networkSpec.Gateway4 != "" {
			p := networkPath.Child("gateway4")
			allErrs = append(allErrs, field.Invalid(p, true, "gateway4 is mutually exclusive with dhcp4"))
		}
	}

	if networkSpec.DHCP6 {
		if hasIPv6Address {
			p := networkPath.Child("dhcp6")
			allErrs = append(allErrs, field.Invalid(p, networkSpec.Addresses,
				"dhcp6 cannot be used with IP6 addresses in network.addresses field"))
		}

		if networkSpec.Gateway6 != "" {
			p := networkPath.Child("gateway6")
			allErrs = append(allErrs, field.Invalid(p, true, "gateway6 is mutually exclusive with dhcp6"))
		}
	}

	// TODO: Much more

	return allErrs
}

func (v validator) validateVolumes(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	volumesPath := field.NewPath("spec", "volumes")
	volumeNames := map[string]bool{}

	for i, vol := range vm.Spec.Volumes {
		volPath := volumesPath.Index(i)

		if vol.Name != "" {
			if volumeNames[vol.Name] {
				allErrs = append(allErrs, field.Duplicate(volPath.Child("name"), vol.Name))
			} else {
				volumeNames[vol.Name] = true
			}
		} else {
			allErrs = append(allErrs, field.Required(volPath.Child("name"), ""))
		}

		if vol.Name != "" {
			errs := validation.NameIsDNSSubdomain(volume.CNSAttachmentNameForVolume(vm.Name, vol.Name), false)
			for _, msg := range errs {
				allErrs = append(allErrs, field.Invalid(volPath.Child("name"), vol.Name, msg))
			}
		}

		if vol.PersistentVolumeClaim == nil {
			allErrs = append(allErrs, field.Required(volPath.Child("persistentVolumeClaim"), ""))
		} else {
			allErrs = append(allErrs, v.validateVolumeWithPVC(ctx, vol, volPath)...)
		}
	}

	// TODO: InstanceStorage

	return allErrs
}

func (v validator) validateVolumeWithPVC(
	ctx *context.WebhookRequestContext,
	vol vmopv1.VirtualMachineVolume,
	volPath *field.Path) field.ErrorList {

	var allErrs field.ErrorList
	pvcPath := volPath.Child("persistentVolumeClaim")

	if vol.PersistentVolumeClaim.ClaimName == "" {
		allErrs = append(allErrs, field.Required(pvcPath.Child("claimName"), ""))
	}
	if vol.PersistentVolumeClaim.ReadOnly {
		allErrs = append(allErrs, field.NotSupported(pvcPath.Child("readOnly"), true, []string{"false"}))
	}

	return allErrs
}

// validateInstanceStorageVolumes validates if instance storage volumes are added/modified.
func (v validator) validateInstanceStorageVolumes(
	ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	var oldVMInstanceStorageVolumes []vmopv1.VirtualMachineVolume
	if oldVM != nil {
		oldVMInstanceStorageVolumes = filterVolumesA2(oldVM)
	}

	if !equality.Semantic.DeepEqual(filterVolumesA2(vm), oldVMInstanceStorageVolumes) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "volumes"), addingModifyingInstanceVolumesNotAllowed))
	}

	return allErrs
}

func (v validator) validateReadinessProbe(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	readinessProbePath := field.NewPath("spec", "readinessProbe")

	probe := &vm.Spec.ReadinessProbe

	if probe.TCPSocket != nil && probe.GuestHeartbeat != nil {
		allErrs = append(allErrs, field.Forbidden(readinessProbePath, readinessProbeOnlyOneAction))
	}

	if probe.TCPSocket != nil {
		tcpSocketPath := readinessProbePath.Child("tcpSocket")

		// Validate port if environment is a restricted network environment between SV CP VMs and Workload VMs e.g. VMC.
		if probe.TCPSocket.Port.IntValue() != allowedRestrictedNetworkTCPProbePort {
			isRestrictedEnv, err := v.isNetworkRestrictedForReadinessProbe(ctx)
			if err != nil {
				allErrs = append(allErrs, field.Forbidden(tcpSocketPath, err.Error()))
			} else if isRestrictedEnv {
				allErrs = append(allErrs,
					field.NotSupported(tcpSocketPath.Child("port"), probe.TCPSocket.Port.IntValue(),
						[]string{strconv.Itoa(allowedRestrictedNetworkTCPProbePort)}))
			}
		}
	}

	return allErrs
}

var megaByte = resource.MustParse("1Mi")

func (v validator) validateAdvanced(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	advancedPath := field.NewPath("spec", "advanced")
	advanced := &vm.Spec.Advanced

	if diskCap := advanced.BootDiskCapacity; !diskCap.IsZero() {
		if diskCap.Value()%megaByte.Value() != 0 {
			allErrs = append(allErrs, field.Invalid(advancedPath.Child("bootDiskCapacity"),
				diskCap.Value(), vSphereVolumeSizeNotMBMultiple))
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

	if newPowerState == vmopv1.VirtualMachinePowerStateSuspended {
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
	case vmopv1.VirtualMachinePowerStateOn:
		// The request is attempting to power on a VM that is already powered
		// on. Validate fields that may or may not be mutable while a VM is in
		// a powered on state.
		if newPowerState == vmopv1.VirtualMachinePowerStateOn {
			invalidFields := v.validateUpdatesWhenPoweredOn(ctx, newVM, oldVM)
			allErrs = append(allErrs, invalidFields...)
		}

	case vmopv1.VirtualMachinePowerStateOff:
		// Mark the power state as invalid if the request is attempting to
		// suspend a VM that is powered off.
		var msg string
		if newPowerState == vmopv1.VirtualMachinePowerStateSuspended {
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

	if !equality.Semantic.DeepEqual(vm.Spec.Bootstrap, oldVM.Spec.Bootstrap) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("bootstrap"), updatesNotAllowedWhenPowerOn))
	}
	if !equality.Semantic.DeepEqual(vm.Spec.Network, oldVM.Spec.Network) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("network"), updatesNotAllowedWhenPowerOn))
	}

	// TODO: More checks.

	return allErrs
}

func (v validator) validateImmutableFields(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.ImageName, oldVM.Spec.ImageName, specPath.Child("imageName"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.ClassName, oldVM.Spec.ClassName, specPath.Child("className"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.StorageClass, oldVM.Spec.StorageClass, specPath.Child("storageClass"))...)
	// TODO: More checks.

	// TODO: Allow privilege?
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.Reserved.ResourcePolicyName, oldVM.Spec.Reserved.ResourcePolicyName, specPath.Child("reserved", "resourcePolicyName"))...)

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
		return false, fmt.Errorf("error get ConfigMap: %s while validating TCP readiness probe port: %v", configMapKey, err)
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
