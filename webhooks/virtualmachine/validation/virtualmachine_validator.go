// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/annotations"
	cloudinitvalidate "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/validate"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName                          = "default"
	storageResourceQuotaStrPattern       = ".storageclass.storage.k8s.io/"
	isRestrictedNetworkKey               = "IsRestrictedNetwork"
	allowedRestrictedNetworkTCPProbePort = 6443

	vmiKind  = "VirtualMachineImage"
	cvmiKind = "ClusterVirtualMachineImage"

	readinessProbeOnlyOneAction              = "only one action can be specified"
	tcpReadinessProbeNotAllowedVPC           = "VPC networking doesn't allow TCP readiness probe to be specified"
	updatesNotAllowedWhenPowerOn             = "updates to this field is not allowed when VM power is on"
	storageClassNotFoundFmt                  = "Storage policy %s does not exist"
	storageClassNotAssignedFmt               = "Storage policy is not associated with the namespace %s"
	vSphereVolumeSizeNotMBMultiple           = "value must be a multiple of MB"
	addingModifyingInstanceVolumesNotAllowed = "adding or modifying instance storage volume claim(s) is not allowed"
	featureNotEnabled                        = "the %s feature is not enabled"
	invalidPowerStateOnCreateFmt             = "cannot set a new VM's power state to %s"
	invalidPowerStateOnUpdateFmt             = "cannot %s a VM that is %s"
	invalidPowerStateOnUpdateEmptyString     = "cannot set power state to empty string"
	invalidNextRestartTimeOnCreate           = "cannot restart VM on create"
	invalidNextRestartTimeOnUpdate           = "must be formatted as RFC3339Nano"
	invalidNextRestartTimeOnUpdateNow        = "mutation webhooks are required to restart VM"
	modifyAnnotationNotAllowedForNonAdmin    = "modifying this annotation is not allowed for non-admin users"
	modifyLabelNotAllowedForNonAdmin         = "modifying this label is not allowed for non-admin users"
	invalidMinHardwareVersionNotSupported    = "should be less than or equal to %d"
	invalidMinHardwareVersionDowngrade       = "cannot downgrade hardware version"
	invalidMinHardwareVersionPowerState      = "cannot upgrade hardware version unless powered off"
	invalidImageKind                         = "supported: " + vmiKind + "; " + cvmiKind
	invalidZone                              = "cannot use zone that is being deleted"
	restrictedToPrivUsers                    = "restricted to privileged users"
	invalidPVCBYOKFmt                        = "cannot attach volume to vm with spec.crypto.encryptionClassName=%q"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha3-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha3,name=default.validating.virtualmachine.v1alpha3.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachine validation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client ctrlclient.Client) builder.Validator {
	return validator{
		client: client,
		// TODO BMV Use the Context.scheme instead
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    ctrlclient.Client
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachine{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateImageOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateClassOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateStorageClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateCrypto(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validatePowerStateOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateLabel(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateNetworkHostAndDomainName(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateMinHardwareVersion(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateCdrom(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// Updates to VM's image are only allowed if it is a Registered VM (used for failover
// in disaster recovery).
func (v validator) validateImageOnUpdate(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vmopv1util.IsImagelessVM(*vm) && pkgcfg.FromContext(ctx).Features.VMIncrementalRestore {
		if annotations.HasRegisterVM(vm) {
			if !vmopv1util.ImageRefsEqual(vm.Spec.Image, oldVM.Spec.Image) {
				if !ctx.IsPrivilegedAccount {
					allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "image"), restrictedToPrivUsers))
				}
			}

			if oldVM.Spec.ImageName != vm.Spec.ImageName {
				if !ctx.IsPrivilegedAccount {
					allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "imageName"), restrictedToPrivUsers))
				}
			}

			return allErrs
		}
	}

	allErrs = append(allErrs,
		validation.ValidateImmutableField(vm.Spec.ImageName, oldVM.Spec.ImageName, field.NewPath("spec", "imageName"))...)
	allErrs = append(allErrs,
		validation.ValidateImmutableField(vm.Spec.Image, oldVM.Spec.Image, field.NewPath("spec", "image"))...)

	return allErrs
}

// ValidateUpdate validates if the given VirtualMachineSpec update is valid.
// Changes to following fields are not allowed:
//   - Image
//   - ImageName
//   - StorageClass
//   - ResourcePolicyName
//   - Minimum VM Hardware Version
//
// Following fields can only be changed when the VM is powered off.
//   - Bootstrap
//   - GuestID
//   - CD-ROM (updating connection state is allowed regardless of power state)
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
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
	fieldErrs = append(fieldErrs, v.validateCrypto(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnUpdate(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateMinHardwareVersion(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateLabel(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNetworkHostAndDomainName(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateCdrom(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateBootstrap(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	bootstrapPath := field.NewPath("spec", "bootstrap")

	var (
		cloudInit  *vmopv1.VirtualMachineBootstrapCloudInitSpec
		linuxPrep  *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
		sysPrep    *vmopv1.VirtualMachineBootstrapSysprepSpec
		vAppConfig *vmopv1.VirtualMachineBootstrapVAppConfigSpec
	)

	if vm.Spec.Bootstrap != nil {
		cloudInit = vm.Spec.Bootstrap.CloudInit
		linuxPrep = vm.Spec.Bootstrap.LinuxPrep
		sysPrep = vm.Spec.Bootstrap.Sysprep
		vAppConfig = vm.Spec.Bootstrap.VAppConfig
	}

	if cloudInit != nil {
		p := bootstrapPath.Child("cloudInit")

		if linuxPrep != nil || sysPrep != nil || vAppConfig != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"CloudInit may not be used with any other bootstrap provider"))
		}

		if v := cloudInit.CloudConfig; v != nil {
			if cloudInit.RawCloudConfig != nil {
				allErrs = append(allErrs, field.Invalid(p, "cloudInit",
					"cloudConfig and rawCloudConfig are mutually exclusive"))
			}
			allErrs = append(allErrs, cloudinitvalidate.CloudConfigJSONRawMessage(p, *v)...)
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

		if sysPrep.Sysprep != nil && sysPrep.RawSysprep != nil {
			allErrs = append(allErrs, field.Invalid(p, "sysPrep",
				"sysprep and rawSysprep are mutually exclusive"))
		} else if sysPrep.Sysprep == nil && sysPrep.RawSysprep == nil {
			allErrs = append(allErrs, field.Invalid(p, "sysPrep",
				"either sysprep or rawSysprep must be provided"))
		}

		if sysPrep.Sysprep != nil {
			allErrs = append(allErrs, v.validateInlineSysprep(p, vm, sysPrep.Sysprep)...)
		}
	}

	if vAppConfig != nil {
		p := bootstrapPath.Child("vAppConfig")

		if cloudInit != nil {
			allErrs = append(allErrs, field.Forbidden(p,
				"vAppConfig may not be used in conjunction with CloudInit bootstrap provider"))
		}

		if len(vAppConfig.Properties) != 0 && len(vAppConfig.RawProperties) != 0 {
			allErrs = append(allErrs, field.TypeInvalid(p, "vAppConfig",
				"properties and rawProperties are mutually exclusive"))
		}

		for _, property := range vAppConfig.Properties {
			if key := property.Key; key == "" {
				allErrs = append(allErrs, field.Invalid(p.Child("properties").Child("key"), "key",
					"key is a required field in vAppConfig Properties"))
			}
			if value := property.Value; value.From != nil && value.Value != nil {
				allErrs = append(allErrs, field.Invalid(p.Child("properties").Child("value"), "value",
					"from and value is mutually exclusive"))
			}
		}

	}

	return allErrs
}

func (v validator) validateInlineSysprep(
	p *field.Path,
	vm *vmopv1.VirtualMachine,
	sysprep *sysprep.Sysprep) field.ErrorList {

	var allErrs field.ErrorList

	s := p.Child("sysprep")
	if guiUnattended := sysprep.GUIUnattended; guiUnattended != nil {
		if guiUnattended.AutoLogon && guiUnattended.AutoLogonCount == 0 {
			allErrs = append(allErrs, field.Invalid(s, "guiUnattended",
				"autoLogon requires autoLogonCount to be specified"))
		}

		if guiUnattended.AutoLogon && (guiUnattended.Password == nil || guiUnattended.Password.Name == "") {
			allErrs = append(allErrs, field.Invalid(s, "guiUnattended",
				"autoLogon requires password selector to be set"))
		}
	}

	if identification := sysprep.Identification; identification != nil {
		var domainName string
		if network := vm.Spec.Network; network != nil {
			domainName = network.DomainName
		}
		if domainName != "" && identification.JoinWorkgroup != "" {
			allErrs = append(allErrs, field.Invalid(s, "identification",
				"spec.network.domainName and joinWorkgroup are mutually exclusive"))
		}

		if domainName != "" {
			if identification.DomainAdmin == "" ||
				identification.DomainAdminPassword == nil ||
				identification.DomainAdminPassword.Name == "" {
				allErrs = append(allErrs, field.Invalid(s, "identification",
					"spec.network.domainName requires domainAdmin and domainAdminPassword selector to be set"))
			}
		}

		if identification.JoinWorkgroup != "" {
			if identification.DomainAdmin != "" || identification.DomainAdminPassword != nil {
				allErrs = append(allErrs, field.Invalid(s, "identification",
					"joinWorkgroup and domainAdmin/domainAdminPassword are mutually exclusive"))
			}
		}
	}

	return allErrs
}

func (v validator) validateImageOnCreate(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var (
		allErrs field.ErrorList
		f       = field.NewPath("spec", "image")
	)

	switch {
	// Imageless VMs are only allowed if the Mobility Import VM, or the Incremental restore FSS is supported.
	case vmopv1util.IsImagelessVM(*vm) &&
		(pkgcfg.FromContext(ctx).Features.VMImportNewNet ||
			pkgcfg.FromContext(ctx).Features.VMIncrementalRestore):
		// TODO: Simplify this once mobility operator starts creating VMs with correct annotation.
		// Skip validations on images if it is a VM created using ImportVM, or RegisterVM.
		if annotations.HasImportVM(vm) || annotations.HasRegisterVM(vm) {
			// Restrict creating imageless VM resources to privileged users.
			if !ctx.IsPrivilegedAccount {
				allErrs = append(allErrs, field.Forbidden(f, restrictedToPrivUsers))
			}

			return allErrs
		}

		// Restrict creating imageless VM resources to privileged users.
		// TODO: This should be removed once all consumers have migrated to using annotations.
		if !ctx.IsPrivilegedAccount {
			allErrs = append(allErrs, field.Forbidden(f, restrictedToPrivUsers))
		}

	case vm.Spec.Image == nil:
		allErrs = append(allErrs, field.Required(f, ""))
	case vm.Spec.Image.Kind == "":
		allErrs = append(allErrs, field.Required(f.Child("kind"), invalidImageKind))
	case vm.Spec.Image.Kind != vmiKind && vm.Spec.Image.Kind != cvmiKind:
		allErrs = append(allErrs, field.Invalid(f.Child("kind"), vm.Spec.Image.Kind, invalidImageKind))
	}

	return allErrs
}

func (v validator) validateClassOnCreate(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	f := field.NewPath("spec", "className")

	switch {
	case vmopv1util.IsClasslessVM(*vm) &&
		pkgcfg.FromContext(ctx).Features.VMImportNewNet:

		// Restrict creating classless VM resources to privileged users.
		if !ctx.IsPrivilegedAccount {
			allErrs = append(allErrs, field.Forbidden(f, restrictedToPrivUsers))
		}

	case vm.Spec.ClassName == "":

		allErrs = append(allErrs, field.Required(f, ""))

	}

	return allErrs
}

func (v validator) validateClassOnUpdate(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if f := pkgcfg.FromContext(ctx).Features; !f.VMResize && !f.VMResizeCPUMemory {
		return append(allErrs,
			validation.ValidateImmutableField(vm.Spec.ClassName, oldVM.Spec.ClassName, field.NewPath("spec", "className"))...)
	}

	if vm.Spec.ClassName == "" {
		if pkgcfg.FromContext(ctx).Features.VMImportNewNet {
			if !ctx.IsPrivilegedAccount {
				// Restrict creating classless VM resources to privileged users.
				allErrs = append(
					allErrs,
					field.Required(field.NewPath("spec", "className"), ""))
			}
		} else {
			allErrs = append(
				allErrs,
				field.Required(field.NewPath("spec", "className"), ""))
		}
	}

	return allErrs
}

func (v validator) validateStorageClass(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if vm.Spec.StorageClass == "" {
		return allErrs
	}

	scPath := field.NewPath("spec", "storageClass")
	scName := vm.Spec.StorageClass

	sc := &storagev1.StorageClass{}
	if err := v.client.Get(ctx, ctrlclient.ObjectKey{Name: scName}, sc); err != nil {
		if apierrors.IsNotFound(err) {
			return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storageClassNotFoundFmt, scName)))
		}
		return append(allErrs, field.Invalid(scPath, scName, err.Error()))
	}

	// This is what enforces that the storage policy has been associated with this namespace.
	if pkgcfg.FromContext(ctx).Features.PodVMOnStretchedSupervisor {
		storagePolicyQuotas := &spqv1.StoragePolicyQuotaList{}
		if err := v.client.List(ctx, storagePolicyQuotas, ctrlclient.InNamespace(vm.Namespace)); err != nil {
			return append(allErrs, field.Invalid(scPath, scName, err.Error()))
		}

		for _, q := range storagePolicyQuotas.Items {
			for i := range q.Status.SCLevelQuotaStatuses {
				if q.Status.SCLevelQuotaStatuses[i].StorageClassName == scName {
					return nil
				}
			}
		}

	} else {
		resourceQuotas := &corev1.ResourceQuotaList{}
		if err := v.client.List(ctx, resourceQuotas, ctrlclient.InNamespace(vm.Namespace)); err != nil {
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
	}

	return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storageClassNotAssignedFmt, vm.Namespace)))
}

func (v validator) validateCrypto(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var (
		encClassName string
		cryptoPath   = field.NewPath("spec", "crypto")
	)

	if vm.Spec.Crypto != nil {
		encClassName = vm.Spec.Crypto.EncryptionClassName
		if !pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			return field.ErrorList{
				field.Invalid(
					cryptoPath,
					vm.Spec.Crypto,
					fmt.Sprintf(featureNotEnabled, "Bring Your Own Key (Provider)")),
			}
		}
	}

	if encClassName == "" {
		return nil
	}

	var (
		allErrs          field.ErrorList
		encClassNamePath = cryptoPath.Child("encryptionClassName")
	)

	if ok, _, err := kubeutil.IsEncryptedStorageClass(
		ctx,
		v.client,
		vm.Spec.StorageClass); err != nil {

		allErrs = append(allErrs, field.InternalError(encClassNamePath, err))

	} else if !ok {
		// Return an error on the "vm.Spec.Crypto.EncryptionClassName" path
		// instead of "vm.Spec.StorageClass" because the storage class is
		// invalid due to the user's choice of encryption class name.
		allErrs = append(allErrs, field.Invalid(
			encClassNamePath,
			vm.Spec.Crypto.EncryptionClassName,
			"requires spec.storageClass specify an encryption storage class"))
	}

	return allErrs
}

func (v validator) validateNetwork(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	networkSpec := vm.Spec.Network
	if networkSpec == nil {
		return allErrs
	}

	networkPath := field.NewPath("spec", "network")

	allErrs = append(allErrs, v.validateNetworkSpecWithBootStrap(ctx, vm)...)

	if len(networkSpec.Nameservers) > 0 {
		for i, n := range networkSpec.Nameservers {
			if net.ParseIP(n) == nil {
				allErrs = append(allErrs,
					field.Invalid(networkPath.Child("nameservers").Index(i), n, "must be an IPv4 or IPv6 address"))
			}
		}
	}

	if len(networkSpec.Interfaces) > 0 {
		p := networkPath.Child("interfaces")

		for i, interfaceSpec := range networkSpec.Interfaces {
			allErrs = append(allErrs, v.validateNetworkInterfaceSpec(p.Index(i), interfaceSpec, vm.Name)...)
			allErrs = append(allErrs, v.validateNetworkInterfaceSpecWithBootstrap(ctx, p.Index(i), interfaceSpec, vm)...)
		}
	}

	return allErrs
}

func (v validator) validateNetworkInterfaceSpec(
	interfacePath *field.Path,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	vmName string) field.ErrorList {

	var allErrs field.ErrorList
	var networkIfCRName string
	var networkName string

	if interfaceSpec.Network != nil {
		networkName = interfaceSpec.Network.Name
	}

	// The networkInterface CR name ("vmName-networkName-interfaceName" or "vmName-interfaceName") needs to be a DNS1123 Label
	if networkName != "" {
		networkIfCRName = fmt.Sprintf("%s-%s-%s", vmName, networkName, interfaceSpec.Name)
	} else {
		networkIfCRName = fmt.Sprintf("%s-%s", vmName, interfaceSpec.Name)
	}

	for _, msg := range validation.NameIsDNSSubdomain(networkIfCRName, false) {
		allErrs = append(allErrs, field.Invalid(interfacePath.Child("name"), networkIfCRName, "is the resulting network interface name: "+msg))
	}

	var ipv4Addrs, ipv6Addrs []string
	for i, ipCIDR := range interfaceSpec.Addresses {
		ip, _, err := net.ParseCIDR(ipCIDR)
		if err != nil {
			p := interfacePath.Child("addresses").Index(i)
			allErrs = append(allErrs, field.Invalid(p, ipCIDR, err.Error()))
			continue
		}

		if ip.To4() != nil {
			ipv4Addrs = append(ipv4Addrs, ipCIDR)
		} else {
			ipv6Addrs = append(ipv6Addrs, ipCIDR)
		}
	}

	if ipv4 := interfaceSpec.Gateway4; ipv4 != "" {
		p := interfacePath.Child("gateway4")

		if len(ipv4Addrs) == 0 {
			allErrs = append(allErrs, field.Invalid(p, ipv4, "gateway4 must have an IPv4 address in the addresses field"))
		}

		if ip := net.ParseIP(ipv4); ip == nil || ip.To4() == nil {
			allErrs = append(allErrs, field.Invalid(p, ipv4, "must be a valid IPv4 address"))
		}
	}

	if ipv6 := interfaceSpec.Gateway6; ipv6 != "" {
		p := interfacePath.Child("gateway6")

		if len(ipv6Addrs) == 0 {
			allErrs = append(allErrs, field.Invalid(p, ipv6, "gateway6 must have an IPv6 address in the addresses field"))
		}

		if ip := net.ParseIP(ipv6); ip == nil || ip.To16() == nil || ip.To4() != nil {
			allErrs = append(allErrs, field.Invalid(p, ipv6, "must be a valid IPv6 address"))
		}
	}

	if interfaceSpec.DHCP4 {
		if len(ipv4Addrs) > 0 {
			p := interfacePath.Child("dhcp4")
			allErrs = append(allErrs, field.Invalid(p, strings.Join(ipv4Addrs, ","),
				"dhcp4 cannot be used with IPv4 addresses in addresses field"))
		}

		if gw := interfaceSpec.Gateway4; gw != "" {
			p := interfacePath.Child("gateway4")
			allErrs = append(allErrs, field.Invalid(p, gw, "gateway4 is mutually exclusive with dhcp4"))
		}
	}

	if interfaceSpec.DHCP6 {
		if len(ipv6Addrs) > 0 {
			p := interfacePath.Child("dhcp6")
			allErrs = append(allErrs, field.Invalid(p, strings.Join(ipv6Addrs, ","),
				"dhcp6 cannot be used with IPv6 addresses in addresses field"))
		}

		if gw := interfaceSpec.Gateway6; gw != "" {
			p := interfacePath.Child("gateway6")
			allErrs = append(allErrs, field.Invalid(p, gw, "gateway6 is mutually exclusive with dhcp6"))
		}
	}

	for i, n := range interfaceSpec.Nameservers {
		if net.ParseIP(n) == nil {
			allErrs = append(allErrs,
				field.Invalid(interfacePath.Child("nameservers").Index(i), n, "must be an IPv4 or IPv6 address"))
		}
	}

	if len(interfaceSpec.Routes) > 0 {
		p := interfacePath.Child("routes")

		for i, r := range interfaceSpec.Routes {
			ip, _, err := net.ParseCIDR(r.To)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(p.Index(i).Child("to"), r.To, err.Error()))
			}

			viaIP := net.ParseIP(r.Via)
			if viaIP == nil {
				allErrs = append(allErrs,
					field.Invalid(p.Index(i).Child("via"), r.Via, "must be an IPv4 or IPv6 address"))
			}

			if (ip.To4() != nil) != (viaIP.To4() != nil) {
				allErrs = append(allErrs,
					field.Invalid(p.Index(i), "", "cannot mix IP address families"))
			}
		}
	}

	return allErrs
}

func (v validator) validateNetworkSpecWithBootStrap(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	networkSpec := vm.Spec.Network
	if networkSpec == nil {
		return allErrs
	}

	var (
		cloudInit *vmopv1.VirtualMachineBootstrapCloudInitSpec
		linuxPrep *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
		sysPrep   *vmopv1.VirtualMachineBootstrapSysprepSpec
	)

	if vm.Spec.Bootstrap != nil {
		cloudInit = vm.Spec.Bootstrap.CloudInit
		linuxPrep = vm.Spec.Bootstrap.LinuxPrep
		sysPrep = vm.Spec.Bootstrap.Sysprep
	}

	if len(networkSpec.Nameservers) > 0 {
		if cloudInit != nil {
			if !ptr.DerefWithDefault(cloudInit.UseGlobalNameserversAsDefault, true) {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec", "network", "nameservers"),
					strings.Join(networkSpec.Nameservers, ","),
					"nameservers is only available for CloudInit when UseGlobalNameserversAsDefault is true",
				))
			}
		} else if linuxPrep == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "network", "nameservers"),
				strings.Join(networkSpec.Nameservers, ","),
				"nameservers is available only with the following bootstrap providers: LinuxPrep and Sysprep",
			))
		}
	}

	if len(networkSpec.SearchDomains) > 0 {
		if cloudInit != nil {
			if !ptr.DerefWithDefault(cloudInit.UseGlobalSearchDomainsAsDefault, true) {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec", "network", "searchDomains"),
					strings.Join(networkSpec.SearchDomains, ","),
					"searchDomains is only available for CloudInit when UseGlobalSearchDomainsAsDefault is true",
				))
			}
		} else if linuxPrep == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "network", "searchDomains"),
				strings.Join(networkSpec.SearchDomains, ","),
				"searchDomains is available only with the following bootstrap providers: LinuxPrep and Sysprep",
			))
		}
	}

	return allErrs
}

// MTU, routes, and searchDomains are available only with CloudInit.
// Nameservers is available only with CloudInit and Sysprep.
func (v validator) validateNetworkInterfaceSpecWithBootstrap(
	ctx context.Context,
	interfacePath *field.Path,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	var (
		cloudInit *vmopv1.VirtualMachineBootstrapCloudInitSpec
		sysPrep   *vmopv1.VirtualMachineBootstrapSysprepSpec
	)

	if vm.Spec.Bootstrap != nil {
		cloudInit = vm.Spec.Bootstrap.CloudInit
		sysPrep = vm.Spec.Bootstrap.Sysprep
	}

	if guestDeviceName := interfaceSpec.GuestDeviceName; guestDeviceName != "" {
		if cloudInit == nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("guestDeviceName"),
				guestDeviceName,
				"guestDeviceName is available only with the following bootstrap providers: CloudInit",
			))
		}
	}

	if mtu := interfaceSpec.MTU; mtu != nil {
		if cloudInit == nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("mtu"),
				mtu,
				"mtu is available only with the following bootstrap providers: CloudInit",
			))
		}
	}

	if routes := interfaceSpec.Routes; len(routes) > 0 {
		if cloudInit == nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("routes"),
				// Not exposing routes here in error message
				"routes",
				"routes is available only with the following bootstrap providers: CloudInit",
			))
		}
	}

	if nameservers := interfaceSpec.Nameservers; len(nameservers) > 0 {
		if cloudInit == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("nameservers"),
				strings.Join(nameservers, ","),
				"nameservers is available only with the following bootstrap providers: CloudInit and Sysprep",
			))
		}
	}

	if searchDomains := interfaceSpec.SearchDomains; len(searchDomains) > 0 {
		if cloudInit == nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("searchDomains"),
				strings.Join(searchDomains, ","),
				"searchDomains is available only with the following bootstrap providers: CloudInit",
			))
		}
	}

	return allErrs
}

func (v validator) validateVolumes(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
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
			errs := validation.NameIsDNSSubdomain(util.CNSAttachmentNameForVolume(vm.Name, vol.Name), false)
			for _, msg := range errs {
				allErrs = append(allErrs, field.Invalid(volPath.Child("name"), vol.Name, msg))
			}
		}

		if vol.PersistentVolumeClaim == nil {
			allErrs = append(allErrs, field.Required(volPath.Child("persistentVolumeClaim"), ""))
		} else {
			allErrs = append(allErrs, v.validateVolumeWithPVC(ctx, vm, vol, volPath)...)
		}
	}

	return allErrs
}

func (v validator) validateVolumeWithPVC(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	volPath *field.Path) field.ErrorList {

	var (
		allErrs   field.ErrorList
		pvcPath   = volPath.Child("persistentVolumeClaim")
		claimName = vol.PersistentVolumeClaim.ClaimName
	)

	if vol.PersistentVolumeClaim.ReadOnly {
		allErrs = append(allErrs, field.NotSupported(pvcPath.Child("readOnly"), true, []string{"false"}))
	}

	if claimName == "" {
		allErrs = append(allErrs, field.Required(pvcPath.Child("claimName"), ""))
		return allErrs
	}

	if vol.PersistentVolumeClaim.InstanceVolumeClaim != nil {
		return allErrs
	}

	// For now, make this check best effort. There's an ask to provide immediate feedback but
	// ideally the validation webhooks should just deal with internal consistency.
	pvc := &corev1.PersistentVolumeClaim{}
	if err := v.client.Get(ctx, ctrlclient.ObjectKey{Name: claimName, Namespace: vm.Namespace}, pvc); err != nil {
		return allErrs
	}

	if scName := pvc.Spec.StorageClassName; scName != nil && *scName != "" {
		// Or just check for "-wffc" suffix instead?
		sc := &storagev1.StorageClass{}
		if err := v.client.Get(ctx, ctrlclient.ObjectKey{Name: *scName}, sc); err == nil {
			if mode := sc.VolumeBindingMode; mode != nil && *mode == storagev1.VolumeBindingWaitForFirstConsumer {
				allErrs = append(allErrs,
					field.Forbidden(pvcPath,
						"PVC with WaitForFirstConsumer StorageClass is not supported for VirtualMachines"))
			}
		}
	}

	return allErrs
}

// validateInstanceStorageVolumes validates if instance storage volumes are added/modified.
func (v validator) validateInstanceStorageVolumes(
	ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	var oldVMInstanceStorageVolumes []vmopv1.VirtualMachineVolume
	if oldVM != nil {
		oldVMInstanceStorageVolumes = vmopv1util.FilterInstanceStorageVolumes(oldVM)
	}

	if !equality.Semantic.DeepEqual(vmopv1util.FilterInstanceStorageVolumes(vm), oldVMInstanceStorageVolumes) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "volumes"), addingModifyingInstanceVolumesNotAllowed))
	}

	return allErrs
}

func (v validator) validateReadinessProbe(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	probe := vm.Spec.ReadinessProbe
	if probe == nil {
		return allErrs
	}

	readinessProbePath := field.NewPath("spec", "readinessProbe")

	actionsCnt := 0
	if probe.TCPSocket != nil {
		actionsCnt++
	}
	if probe.GuestHeartbeat != nil {
		actionsCnt++
	}
	if len(probe.GuestInfo) != 0 {
		actionsCnt++
	}
	if actionsCnt > 1 {
		allErrs = append(allErrs, field.Forbidden(readinessProbePath, readinessProbeOnlyOneAction))
	}

	if probe.TCPSocket != nil {
		tcpSocketPath := readinessProbePath.Child("tcpSocket")

		// TCP readiness probe is not allowed under VPC Networking
		if pkgcfg.FromContext(ctx).NetworkProviderType == pkgcfg.NetworkProviderTypeVPC {
			allErrs = append(allErrs, field.Forbidden(tcpSocketPath, tcpReadinessProbeNotAllowedVPC))
		} else if probe.TCPSocket.Port.IntValue() != allowedRestrictedNetworkTCPProbePort {
			// Validate port if environment is a restricted network environment between SV CP VMs and Workload VMs e.g. VMC.
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

func (v validator) validateAdvanced(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	advanced := vm.Spec.Advanced
	if advanced == nil {
		return allErrs
	}

	advancedPath := field.NewPath("spec", "advanced")

	if capacity := advanced.BootDiskCapacity; capacity != nil && !capacity.IsZero() {
		if capacity.Value()%megaByte.Value() != 0 {
			allErrs = append(allErrs, field.Invalid(advancedPath.Child("bootDiskCapacity"),
				capacity.Value(), vSphereVolumeSizeNotMBMultiple))
		}
	}

	return allErrs
}

func (v validator) validateNextRestartTimeOnCreate(
	ctx *pkgctx.WebhookRequestContext,
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
	ctx *pkgctx.WebhookRequestContext,
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
	ctx *pkgctx.WebhookRequestContext,
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
	ctx *pkgctx.WebhookRequestContext,
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
			allErrs = append(allErrs, v.validateUpdatesWhenPoweredOn(ctx, newVM, oldVM)...)
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

func isBootstrapCloudInit(vm *vmopv1.VirtualMachine) bool {
	if vm.Spec.Bootstrap == nil {
		return false
	}
	return vm.Spec.Bootstrap.CloudInit != nil
}

func (v validator) validateUpdatesWhenPoweredOn(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Permit upgrade of existing VM's instanceID, regardless of powerState
	if isBootstrapCloudInit(oldVM) && isBootstrapCloudInit(vm) {
		if oldVM.Spec.Bootstrap.CloudInit.InstanceID == "" {
			oldVM.Spec.Bootstrap.CloudInit.InstanceID = vm.Spec.Bootstrap.CloudInit.InstanceID
		}
	}

	if !equality.Semantic.DeepEqual(vm.Spec.Bootstrap, oldVM.Spec.Bootstrap) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("bootstrap"), updatesNotAllowedWhenPowerOn))
	}

	if vm.Spec.GuestID != oldVM.Spec.GuestID {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("guestID"), updatesNotAllowedWhenPowerOn))
	}

	allErrs = append(allErrs, validateCdromWhenPoweredOn(vm.Spec.Cdrom, oldVM.Spec.Cdrom)...)

	// TODO: More checks.

	return allErrs
}

func (v validator) validateImmutableFields(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	allErrs = append(allErrs, v.validateImageOnUpdate(ctx, vm, oldVM)...)
	allErrs = append(allErrs, v.validateClassOnUpdate(ctx, vm, oldVM)...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.StorageClass, oldVM.Spec.StorageClass, specPath.Child("storageClass"))...)
	// New VMs always have non-empty biosUUID. Existing VMs being upgraded may have an empty biosUUID.
	if oldVM.Spec.BiosUUID != "" {
		allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.BiosUUID, oldVM.Spec.BiosUUID, specPath.Child("biosUUID"))...)
	}
	// New VMs always have non-empty instanceUUID. Existing VMs being upgraded may have an empty instanceUUID.
	if oldVM.Spec.InstanceUUID != "" {
		allErrs = append(allErrs, validation.ValidateImmutableField(vm.Spec.InstanceUUID, oldVM.Spec.InstanceUUID, specPath.Child("instanceUUID"))...)
	}
	allErrs = append(allErrs, v.validateImmutableReserved(ctx, vm, oldVM)...)
	allErrs = append(allErrs, v.validateImmutableNetwork(ctx, vm, oldVM)...)

	return allErrs
}

func (v validator) validateImmutableReserved(_ *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	var newResourcePolicyName, oldResourcePolicyName string
	if reserved := vm.Spec.Reserved; reserved != nil {
		newResourcePolicyName = reserved.ResourcePolicyName
	}
	if reserved := oldVM.Spec.Reserved; reserved != nil {
		oldResourcePolicyName = reserved.ResourcePolicyName
	}

	p := field.NewPath("spec", "reserved")
	return append(allErrs, validation.ValidateImmutableField(newResourcePolicyName, oldResourcePolicyName, p.Child("resourcePolicyName"))...)
}

func (v validator) validateImmutableNetwork(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	oldNetwork := oldVM.Spec.Network
	newNetwork := vm.Spec.Network

	if oldNetwork == nil && newNetwork == nil {
		return allErrs
	}

	p := field.NewPath("spec", "network")

	var oldInterfaces, newInterfaces []vmopv1.VirtualMachineNetworkInterfaceSpec
	if oldNetwork != nil {
		oldInterfaces = oldNetwork.Interfaces
	}
	if newNetwork != nil {
		newInterfaces = newNetwork.Interfaces
	}

	if len(oldInterfaces) != len(newInterfaces) {
		return append(allErrs, field.Forbidden(p.Child("interfaces"), "network interfaces cannot be added or removed"))
	}

	// Skip comparing interfaces if this is a test fail-over to allow vendors to
	// connect network each interface to test network.
	if _, ok := vm.Labels[backupapi.TestFailoverLabelKey]; ok {
		return allErrs
	}

	for i := range newInterfaces {
		newInterface := &newInterfaces[i]
		oldInterface := &oldInterfaces[i]
		pi := p.Child("interfaces").Index(i)

		if newInterface.Name != oldInterface.Name {
			allErrs = append(allErrs, field.Forbidden(pi.Child("name"), validation.FieldImmutableErrorMsg))
		}
		if !reflect.DeepEqual(newInterface.Network, oldInterface.Network) {
			allErrs = append(allErrs, field.Forbidden(pi.Child("network"), validation.FieldImmutableErrorMsg))
		}
	}

	return allErrs
}

func (v validator) validateAvailabilityZone(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	zoneLabelPath := field.NewPath("metadata", "labels").Key(topology.KubernetesTopologyZoneLabelKey)

	if oldVM != nil {
		// Once the zone has been set then make sure the field is immutable.
		if oldVal := oldVM.Labels[topology.KubernetesTopologyZoneLabelKey]; oldVal != "" {
			newVal := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
			return append(allErrs, validation.ValidateImmutableField(newVal, oldVal, zoneLabelPath)...)
		}
	}

	// Validate the name of the provided availability zone.
	if zoneName := vm.Labels[topology.KubernetesTopologyZoneLabelKey]; zoneName != "" {
		if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
			// Validate the name of the provided zone. It is the same name as az.
			zone, err := topology.GetZone(ctx.Context, v.client, zoneName, vm.Namespace)
			if err != nil {
				return append(allErrs, field.Invalid(zoneLabelPath, zoneName, err.Error()))
			}
			// Prevent new VirtualMachines created on marked zone by SSO users.
			if !zone.DeletionTimestamp.IsZero() {
				if !ctx.IsPrivilegedAccount && !isCAPVServiceAccount(ctx.UserInfo.Username) {
					return append(allErrs, field.Invalid(zoneLabelPath, zoneName, invalidZone))
				}
			}
		} else {
			if _, err := topology.GetAvailabilityZone(ctx.Context, v.client, zoneName); err != nil {
				return append(allErrs, field.Invalid(zoneLabelPath, zoneName, err.Error()))
			}
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

func (v validator) isNetworkRestrictedForReadinessProbe(ctx *pkgctx.WebhookRequestContext) (bool, error) {
	configMap := &corev1.ConfigMap{}
	configMapKey := ctrlclient.ObjectKey{Name: config.ProviderConfigMapName, Namespace: ctx.Namespace}
	if err := v.client.Get(ctx, configMapKey, configMap); err != nil {
		return false, fmt.Errorf("error get ConfigMap: %s while validating TCP readiness probe port: %v", configMapKey, err)
	}

	return configMap.Data[isRestrictedNetworkKey] == "true", nil
}

func (v validator) validateAnnotation(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	// Use an empty VM if the oldVM is nil to validate a creation request.
	if oldVM == nil {
		oldVM = &vmopv1.VirtualMachine{}
	}

	annotationPath := field.NewPath("metadata", "annotations")

	if vm.Annotations[vmopv1.InstanceIDAnnotation] != oldVM.Annotations[vmopv1.InstanceIDAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.InstanceIDAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.FirstBootDoneAnnotation] != oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.FirstBootDoneAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.RegisteredVMAnnotation] != oldVM.Annotations[vmopv1.RegisteredVMAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.RegisteredVMAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.ImportedVMAnnotation] != oldVM.Annotations[vmopv1.ImportedVMAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.ImportedVMAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	// The following annotations will be added by the mutation webhook upon VM creation.
	if !reflect.DeepEqual(oldVM, &vmopv1.VirtualMachine{}) {
		if vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] != oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(constants.CreatedAtBuildVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}

		if vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] != oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(constants.CreatedAtSchemaVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	return allErrs
}

func (v validator) validateMinHardwareVersion(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "minHardwareVersion")

	// kubebuilder validations handles check for minHardwareVersion < 13.
	// This is necessary here to handle brownfield api versions.
	if vimtypes.HardwareVersion(vm.Spec.MinHardwareVersion) > vimtypes.MaxValidHardwareVersion {
		allErrs = append(allErrs, field.Invalid(
			fieldPath,
			vm.Spec.MinHardwareVersion,
			fmt.Sprintf(invalidMinHardwareVersionNotSupported, vimtypes.MaxValidHardwareVersion)))
	}

	if oldVM != nil {
		// Disallow downgrades.
		oldHV, newHV := oldVM.Spec.MinHardwareVersion, vm.Spec.MinHardwareVersion
		if newHV < oldHV {
			allErrs = append(allErrs, field.Invalid(
				fieldPath,
				vm.Spec.MinHardwareVersion,
				invalidMinHardwareVersionDowngrade))
		}

		// Disallow upgrades unless powered off or will be powered off.
		if newHV > oldHV {
			if oldVM.Spec.PowerState != vmopv1.VirtualMachinePowerStateOff &&
				vm.Spec.PowerState != vmopv1.VirtualMachinePowerStateOff {

				allErrs = append(allErrs, field.Invalid(
					fieldPath,
					vm.Spec.MinHardwareVersion,
					invalidMinHardwareVersionPowerState))
			}
		}
	}

	return allErrs
}

func (v validator) validateLabel(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	// Use an empty VM if the oldVM is nil to validate a creation request.
	if oldVM == nil {
		oldVM = &vmopv1.VirtualMachine{}
	}

	labelPath := field.NewPath("metadata", "labels")

	if vm.Labels[vmopv1.PausedVMLabelKey] != oldVM.Labels[vmopv1.PausedVMLabelKey] {
		allErrs = append(allErrs, field.Forbidden(labelPath.Child(vmopv1.PausedVMLabelKey), modifyLabelNotAllowedForNonAdmin))
	}

	return allErrs
}

func (v validator) validateNetworkHostAndDomainName(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if vm == nil {
		panic("vm is nil")
	}

	if err := vmopv1util.ValidateHostAndDomainName(*vm); err != nil {

		// Do not complain about the host name exceeding 15 characters for a
		// Windows guest when the old host name already exceeded 15 characters.
		// This prevents breaking updates/patches to VMs that existed prior to
		// this restriction.
		if oldVM != nil && err.Detail == vmopv1util.ErrInvalidHostNameWindows {
			if bs := oldVM.Spec.Bootstrap; bs != nil && bs.Sysprep != nil {
				oldHostName := oldVM.Name
				if oldVM.Spec.Network != nil && oldVM.Spec.Network.HostName != "" {
					oldHostName = oldVM.Spec.Network.HostName
				}
				if len(oldHostName) > 15 {
					return nil
				}
			}
		}

		return field.ErrorList{err}
	}

	return nil
}

func (v *validator) validateCdrom(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if len(vm.Spec.Cdrom) == 0 {
		return allErrs
	}

	f := field.NewPath("spec", "cdrom")
	if !pkgcfg.FromContext(ctx).Features.IsoSupport {
		return append(allErrs, field.Forbidden(f, fmt.Sprintf(featureNotEnabled, "CD-ROM (ISO) support")))
	}

	// GuestID must be set when deploying an ISO VM with CD-ROMs.
	if vm.Spec.GuestID == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "guestID"), "when deploying a VM with CD-ROMs"))
	}

	// Validate image kind and uniqueness for each CD-ROM.
	// Namespace and cluster scope images with the same name are considered
	// duplicates since they come from the same content library item file.
	imgNames := make(map[string]struct{}, len(vm.Spec.Cdrom))
	for i, c := range vm.Spec.Cdrom {
		imgPath := f.Index(i).Child("image")
		imgKind := c.Image.Kind
		if imgKind != vmiKind && imgKind != cvmiKind {
			allErrs = append(allErrs, field.NotSupported(imgPath.Child("kind"), imgKind, []string{vmiKind, cvmiKind}))
		}
		imgName := c.Image.Name
		if _, ok := imgNames[imgName]; ok {
			allErrs = append(allErrs, field.Duplicate(imgPath.Child("name"), imgName))
		} else {
			imgNames[imgName] = struct{}{}
		}
	}

	return allErrs
}

func validateCdromWhenPoweredOn(
	cdrom, oldCdrom []vmopv1.VirtualMachineCdromSpec) field.ErrorList {

	var (
		allErrs field.ErrorList
		f       = field.NewPath("spec", "cdrom")
	)

	if len(cdrom) != len(oldCdrom) {
		// Adding or removing CD-ROMs is not allowed when VM is powered on.
		allErrs = append(allErrs, field.Forbidden(f, updatesNotAllowedWhenPowerOn))
		return allErrs
	}

	oldCdromNameToImage := make(map[string]vmopv1.VirtualMachineImageRef, len(oldCdrom))
	for _, c := range oldCdrom {
		oldCdromNameToImage[c.Name] = c.Image
	}

	for i, c := range cdrom {
		if oldImage, ok := oldCdromNameToImage[c.Name]; !ok {
			// CD-ROM name is changed.
			allErrs = append(allErrs, field.Forbidden(f.Index(i).Child("name"), updatesNotAllowedWhenPowerOn))
		} else if !reflect.DeepEqual(c.Image, oldImage) {
			// CD-ROM image is changed.
			allErrs = append(allErrs, field.Forbidden(f.Index(i).Child("image"), updatesNotAllowedWhenPowerOn))
		}
	}

	return allErrs
}

var capvDefaultServiceAccount = regexp.MustCompile("^system:serviceaccount:svc-tkg-domain-[^:]+:default$")

// isCAPVServiceAccount checks if the username matches that of the CAPV service account.
func isCAPVServiceAccount(username string) bool {
	return capvDefaultServiceAccount.Match([]byte(username))
}
