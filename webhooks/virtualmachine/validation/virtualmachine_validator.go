// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	cloudinitvalidate "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/validate"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/anno2extraconfig"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName                          = "default"
	isRestrictedNetworkKey               = "IsRestrictedNetwork"
	allowedRestrictedNetworkTCPProbePort = 6443

	vmiKind        = "VirtualMachineImage"
	cvmiKind       = "ClusterVirtualMachineImage"
	vmclassKind    = "VirtualMachineClass"
	vmSnapshotKind = "VirtualMachineSnapshot"

	readinessProbeOnlyOneAction                = "only one action can be specified"
	tcpReadinessProbeNotAllowedVPC             = "VPC networking doesn't allow TCP readiness probe to be specified"
	updatesNotAllowedWhenPowerOn               = "updates to this field is not allowed when VM power is on"
	addingNewCdromNotAllowedWhenPowerOn        = "adding new CD-ROMs is not allowed when VM is powered on"
	removingCdromNotAllowedWhenPowerOn         = "removing CD-ROMs is not allowed when VM is powered on"
	storageClassNotFoundFmt                    = "Storage policy %s does not exist"
	storageClassNotAssignedFmt                 = "Storage policy is not associated with the namespace %s"
	vSphereVolumeSizeNotMBMultiple             = "value must be a multiple of MB"
	addingModifyingInstanceVolumesNotAllowed   = "adding or modifying instance storage volume claim(s) is not allowed"
	featureNotEnabled                          = "the %s feature is not enabled"
	invalidPowerStateOnCreateFmt               = "cannot set a new VM's power state to %s"
	invalidPowerStateOnUpdateFmt               = "cannot %s a VM that is %s"
	invalidPowerStateOnUpdateEmptyString       = "cannot set power state to empty string"
	invalidNextRestartTimeOnCreate             = "cannot restart VM on create"
	invalidRFC3339NanoTimeFormat               = "must be formatted as RFC3339Nano"
	invalidNextRestartTimeOnUpdateNow          = "mutation webhooks are required to restart VM"
	modifyAnnotationNotAllowedForNonAdmin      = "modifying this annotation is not allowed for non-admin users"
	modifyLabelNotAllowedForNonAdmin           = "modifying this label is not allowed for non-admin users"
	invalidMinHardwareVersionNotSupported      = "should be less than or equal to %d"
	invalidMinHardwareVersionDowngrade         = "cannot downgrade hardware version"
	invalidMinHardwareVersionPowerState        = "cannot upgrade hardware version unless powered off"
	invalidImageKind                           = "supported: " + vmiKind + "; " + cvmiKind
	invalidZone                                = "cannot use zone that is being deleted"
	restrictedToPrivUsers                      = "restricted to privileged users"
	controllerBusNumberRangeFmt                = "%s controllerBusNumber must be in the range of 0 to %d"
	unitNumberRangeFmt                         = "%s unitNumber must be in the range of 0 to %d"
	addRestrictedAnnotation                    = "adding this annotation is restricted to privileged users"
	delRestrictedAnnotation                    = "removing this annotation is restricted to privileged users"
	modRestrictedAnnotation                    = "modifying this annotation is restricted to privileged users"
	notUpgraded                                = "modifying this VM is not allowed until it is upgraded"
	invalidClassInstanceReference              = "must specify a valid reference to a VirtualMachineClassInstance object"
	invalidClassInstanceReferenceNotActive     = "must specify a reference to a VirtualMachineClassInstance object that is active"
	invalidClassInstanceReferenceOwnerMismatch = "VirtualMachineClassInstance must be an instance of the VM Class specified by spec.class"
	labelSelectorCanNotContainVMOperatorLabels = "label selector can not contain VM Operator managed labels (vmoperator.vmware.com)"
	guestCustomizationVCDParityNotEnabled      = "VC guest customization VCD parity capability is not enabled"
	bootstrapProviderTypeCannotBeChanged       = "bootstrap provider type cannot be changed"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha5-virtualmachine,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachines,versions=v1alpha5,name=default.validating.virtualmachine.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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

// vmFromUnstructured returns the VirtualMachine from the unstructured object.
func (v validator) vmFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachine, error) {
	vm := &vmopv1.VirtualMachine{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vm); err != nil {
		return nil, err
	}
	return vm, nil
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachine{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vm, err := v.vmFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	ctx.Context = vmopv1util.GetContextWithWorkloadDomainIsolation(ctx.Context, *vm)
	if !pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		ctx.Logger.Info("Disabled WorkloadDomainIsolation capability for this VM")
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateImageOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateClassOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateStorageClass(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateCrypto(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validatePowerStateOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnCreate(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateLabel(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateNetworkHostAndDomainName(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateMinHardwareVersion(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateHardware(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateChecks(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateNextPowerStateChangeTimeFormat(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBootOptions(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateSnapshot(ctx, vm, nil)...)
	fieldErrs = append(fieldErrs, v.validateGroupName(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateVMAffinity(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBiosUUID(ctx, vm)...)

	if pkgcfg.FromContext(ctx).Features.AllDisksArePVCs {
		fieldErrs = append(fieldErrs, v.validatePVCUnmanagedVolumeClaimInfo(ctx, vm)...)
	}

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// Updates to VM's image are only allowed if it is a failed over VM.
func (v validator) validateImageOnUpdate(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if pkgcfg.FromContext(ctx).Features.VMIncrementalRestore {
		// Allow resetting of image if this is a failover operation.
		if vmopv1util.IsImagelessVM(*vm) && metav1.HasAnnotation(vm.ObjectMeta, vmopv1.FailedOverVMAnnotation) {
			if !ctx.IsPrivilegedAccount {
				if !vmopv1util.ImageRefsEqual(vm.Spec.Image, oldVM.Spec.Image) {
					allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "image"), restrictedToPrivUsers))
				}
				if oldVM.Spec.ImageName != vm.Spec.ImageName {
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

	ctx.Context = vmopv1util.GetContextWithWorkloadDomainIsolation(ctx.Context, *vm)
	if !pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		ctx.Logger.Info("Disabled WorkloadDomainIsolation capability for this VM")
	}

	var fieldErrs field.ErrorList

	// Check if the VM has been upgraded.
	fieldErrs = append(fieldErrs, v.validateSchemaUpgrade(ctx, vm, oldVM)...)

	// Check if an immutable field has been modified.
	fieldErrs = append(fieldErrs, v.validateImmutableFields(ctx, vm, oldVM)...)

	// First validate any updates to the desired state based on the current
	// power state of the VM.
	fieldErrs = append(fieldErrs, v.validatePowerStateOnUpdate(ctx, vm, oldVM)...)

	// Validations for allowed updates. Return validation responses here for conditional updates regardless
	// of whether the update is allowed or not.
	fieldErrs = append(fieldErrs, v.validateCrypto(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAvailabilityZone(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateBootstrapProviderImmutable(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateBootstrap(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNetwork(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateInstanceStorageVolumes(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateReadinessProbe(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateAdvanced(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnUpdate(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateMinHardwareVersion(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateLabel(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNetworkHostAndDomainName(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateHardware(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateChecks(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateNextPowerStateChangeTimeFormat(ctx, vm)...)
	fieldErrs = append(fieldErrs, v.validateBootOptions(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateSnapshot(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateGroupName(ctx, vm)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateBootstrapProviderImmutable(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if ctx.IsPrivilegedAccount {
		return nil
	}

	var fieldErrs field.ErrorList
	p := field.NewPath("spec", "bootstrap")

	oldBS := oldVM.Spec.Bootstrap
	if oldBS == nil {
		return fieldErrs
	}

	bs := vm.Spec.Bootstrap
	if bs == nil {
		return append(fieldErrs, field.Forbidden(p, bootstrapProviderTypeCannotBeChanged))
	}

	// vApp can be combined with LinuxPrep and Sysprep, or used standalone, so don't
	// worry about that here.
	switch {
	case oldBS.CloudInit != nil && bs.CloudInit == nil:
		return append(fieldErrs, field.Forbidden(p.Child("cloudInit"), bootstrapProviderTypeCannotBeChanged))
	case oldBS.LinuxPrep != nil && bs.LinuxPrep == nil:
		return append(fieldErrs, field.Forbidden(p.Child("linuxPrep"), bootstrapProviderTypeCannotBeChanged))
	case oldBS.Sysprep != nil && bs.Sysprep == nil:
		return append(fieldErrs, field.Forbidden(p.Child("sysprep"), bootstrapProviderTypeCannotBeChanged))
	}

	return nil
}

//nolint:gocyclo
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

		if pkgcfg.FromContext(ctx).Features.GuestCustomizationVCDParity {
			if linuxPrep.ScriptText != nil {
				if sc := linuxPrep.ScriptText; sc.From != nil && sc.Value != nil {
					allErrs = append(allErrs, field.Invalid(p.Child("scriptText").Child("value"), "value",
						"from and value are mutually exclusive"))
				}
			}
		} else {
			if linuxPrep.ExpirePasswordAfterNextLogin {
				allErrs = append(allErrs, field.Forbidden(p.Child("expirePasswordAfterNextLogin"),
					guestCustomizationVCDParityNotEnabled))
			}

			if linuxPrep.Password != nil {
				allErrs = append(allErrs, field.Forbidden(p.Child("password"),
					guestCustomizationVCDParityNotEnabled))
			}

			if linuxPrep.ScriptText != nil {
				allErrs = append(allErrs, field.Forbidden(p.Child("scriptText"),
					guestCustomizationVCDParityNotEnabled))
			}
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
			allErrs = append(allErrs, v.validateInlineSysprep(ctx, p, vm, sysPrep.Sysprep)...)
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
					"from and value are mutually exclusive"))
			}
		}

	}

	return allErrs
}

func (v validator) validateInlineSysprep(
	ctx *pkgctx.WebhookRequestContext,
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
			if identification.DomainAdmin != "" || identification.DomainAdminPassword != nil || identification.DomainOU != "" {
				allErrs = append(allErrs, field.Invalid(s, "identification",
					"joinWorkgroup and domainAdmin/domainAdminPassword/domainOU are mutually exclusive"))
			}
		}
	}

	if pkgcfg.FromContext(ctx).Features.GuestCustomizationVCDParity {
		if scriptText := sysprep.ScriptText; scriptText != nil {
			if scriptText.From != nil && scriptText.Value != nil {
				allErrs = append(allErrs, field.Invalid(s.Child("scriptText").Child("value"), "value",
					"from and value are mutually exclusive"))
			}
		}
	} else {
		if sysprep.ExpirePasswordAfterNextLogin {
			allErrs = append(allErrs, field.Forbidden(p.Child("expirePasswordAfterNextLogin"),
				guestCustomizationVCDParityNotEnabled))
		}

		if sysprep.ScriptText != nil {
			allErrs = append(allErrs, field.Forbidden(p.Child("scriptText"),
				guestCustomizationVCDParityNotEnabled))
		}
	}

	return allErrs
}

func (v validator) validateImageOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

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
		// Skip validations on images if it is a VM that is imported, registered, or failed over.
		if metav1.HasAnnotation(vm.ObjectMeta, vmopv1.ImportedVMAnnotation) ||
			metav1.HasAnnotation(vm.ObjectMeta, vmopv1.RestoredVMAnnotation) ||
			metav1.HasAnnotation(vm.ObjectMeta, vmopv1.FailedOverVMAnnotation) {

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

func (v validator) validateClassOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if vmopv1util.IsClasslessVM(*vm) {
		f := field.NewPath("spec", "className")

		if pkgcfg.FromContext(ctx).Features.VMImportNewNet {
			if !ctx.IsPrivilegedAccount {
				allErrs = append(allErrs, field.Forbidden(f, restrictedToPrivUsers))
			}
		} else {
			allErrs = append(allErrs, field.Required(f, ""))
		}
	}

	if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
		allErrs = append(allErrs, v.validateClassInstance(ctx, vm)...)
	}

	return allErrs
}

func (v validator) validateClassInstance(ctx *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	// The mutating webhook ensures that the spec.className and
	// spec.class are both set. Verify if they are valid. However, for
	// pre-v1a4 versions, the class instance will be unset. Return
	// early in that case.

	if vm.Spec.Class == nil {
		return allErrs
	}

	f := field.NewPath("spec", "class")
	classInstance := &vmopv1.VirtualMachineClassInstance{}
	if err := v.client.Get(
		ctx,
		ctrlclient.ObjectKey{Name: vm.Spec.Class.Name, Namespace: vm.Namespace},
		classInstance); err != nil {
		return append(
			allErrs,
			field.Invalid(f.Child("name"), vm.Spec.Class.Name, invalidClassInstanceReference),
		)
	}

	// Specifying an inactive instance is disallowed.
	if _, ok := classInstance.Labels[vmopv1.VMClassInstanceActiveLabelKey]; !ok {
		return append(
			allErrs,
			field.Invalid(f.Child("name"), vm.Spec.Class.Name, invalidClassInstanceReferenceNotActive),
		)
	}

	for _, owner := range classInstance.OwnerReferences {
		if owner.Kind == vmclassKind && owner.Name == vm.Spec.ClassName {
			// The instance is owned by the class specified in spec.className.
			return allErrs
		}
	}

	// The instance does not have an OwnerReference that points to spec.className.
	return append(
		allErrs,
		field.Invalid(f.Child("name"), vm.Spec.Class.Name, invalidClassInstanceReferenceOwnerMismatch))
}

func (v validator) validateClassOnUpdate(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	if oldVM.Spec.ClassName == vm.Spec.ClassName {
		return allErrs
	}

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

	if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
		allErrs = append(allErrs, v.validateClassInstance(ctx, vm)...)
	}

	return allErrs
}

func (v validator) validateStorageClass(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

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

	ok, err := spqutil.IsStorageClassInNamespace(ctx, v.client, sc, vm.Namespace)
	if err != nil {
		return append(allErrs, field.Invalid(scPath, scName, err.Error()))
	}

	if !ok {
		allErrs = append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storageClassNotAssignedFmt, vm.Namespace)))
	}

	return allErrs
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

func (v validator) validateNetwork(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	networkSpec := vm.Spec.Network
	if networkSpec == nil {
		return allErrs
	}

	networkPath := field.NewPath("spec", "network")
	allErrs = append(allErrs, v.validateNetworkSpecWithBootStrap(ctx, networkPath, vm)...)

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

	if oldVM != nil {
		if pkgcfg.FromContext(ctx).Features.MutableNetworks {
			allErrs = append(allErrs, v.validateNetworkInterfaceMacAddressNotChanged(ctx, vm, oldVM)...)
		}
	}

	return allErrs
}

// validateNetworkInterfaceMacAddressNotChanged tries to check that the MAC address
// for an interface is not changed. From the webhook this is best-effort: one could
// always just remove and then quickly add back the interface with just different
// MAC address.
func (v validator) validateNetworkInterfaceMacAddressNotChanged(
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	networkSpec := vm.Spec.Network
	if networkSpec == nil || len(networkSpec.Interfaces) == 0 {
		return nil
	}

	oldVMNetworkSpec := oldVM.Spec.Network
	if oldVMNetworkSpec == nil || len(oldVMNetworkSpec.Interfaces) == 0 {
		return nil
	}

	interfaceSpecToKey := func(s vmopv1.VirtualMachineNetworkInterfaceSpec) string {
		return fmt.Sprintf("%s::%s::%s", s.Name, s.Network.Name, s.Network.Kind)
	}

	interfacesToMACs := make(map[string]string, len(oldVMNetworkSpec.Interfaces))
	for _, interfaceSpec := range oldVMNetworkSpec.Interfaces {
		if interfaceSpec.Network == nil {
			// The VM mutation webhook will always set this.
			continue
		}
		interfacesToMACs[interfaceSpecToKey(interfaceSpec)] = interfaceSpec.MACAddr
	}

	var allErrs field.ErrorList

	for i, interfaceSpec := range networkSpec.Interfaces {
		if interfaceSpec.Network == nil {
			continue
		}

		key := interfaceSpecToKey(interfaceSpec)
		if oldMAC, ok := interfacesToMACs[key]; ok && interfaceSpec.MACAddr != oldMAC {
			p := field.NewPath("spec", "network", "interfaces").Index(i).Child("macAddr")
			allErrs = append(allErrs, field.Invalid(p, interfaceSpec.MACAddr, validation.FieldImmutableErrorMsg))
		}
	}

	return allErrs
}

// Note the code for VDS is basically done, but only support this for VPC right
// now since that is what matters.
var macAddressSupportNetworkGroups = []string{
	// netopv1alpha1.GroupName,
	vpcv1alpha1.GroupVersion.Group,
}

//nolint:gocyclo
func (v validator) validateNetworkInterfaceSpec(
	interfacePath *field.Path,
	interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec,
	vmName string) field.ErrorList {

	var allErrs field.ErrorList
	var networkIfCRName string
	var networkAPIVersion string
	var networkName string

	if interfaceSpec.Network != nil {
		networkAPIVersion = interfaceSpec.Network.APIVersion
		networkName = interfaceSpec.Network.Name
	}

	var networkGV schema.GroupVersion
	if networkAPIVersion != "" {
		if gv, err := schema.ParseGroupVersion(networkAPIVersion); err != nil {
			allErrs = append(allErrs, field.Invalid(
				interfacePath.Child("network", "apiVersion"), networkAPIVersion, err.Error()))
		} else {
			networkGV = gv
		}
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

	if interfaceSpec.MACAddr != "" {
		if !slices.Contains(macAddressSupportNetworkGroups, networkGV.Group) {
			allErrs = append(allErrs, field.Invalid(interfacePath.Child("macAddr"), interfaceSpec.MACAddr,
				fmt.Sprintf("macAddr is available only with the following network providers: %s",
					strings.Join(macAddressSupportNetworkGroups, ","))))
		}
	}

	var ipv4Addrs, ipv6Addrs []string
	for i, ipCIDR := range interfaceSpec.Addresses {
		// NOTE: VPC SubnetPort only takes the IP address so we might want to make this more flexible.
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

	if ipv4 := interfaceSpec.Gateway4; ipv4 != "" && ipv4 != "None" {
		p := interfacePath.Child("gateway4")

		if len(ipv4Addrs) == 0 {
			allErrs = append(allErrs, field.Invalid(p, ipv4, "gateway4 must have an IPv4 address in the addresses field"))
		}

		if ip := net.ParseIP(ipv4); ip == nil || ip.To4() == nil {
			allErrs = append(allErrs, field.Invalid(p, ipv4, "must be a valid IPv4 address"))
		}
	}

	if ipv6 := interfaceSpec.Gateway6; ipv6 != "" && ipv6 != "None" {
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
			var toIP net.IP
			if r.To != "default" {
				ip, _, err := net.ParseCIDR(r.To)
				if err != nil {
					allErrs = append(allErrs, field.Invalid(p.Index(i).Child("to"), r.To, err.Error()))
				}
				toIP = ip
			}

			viaIP := net.ParseIP(r.Via)
			if viaIP == nil {
				allErrs = append(allErrs,
					field.Invalid(p.Index(i).Child("via"), r.Via, "must be an IPv4 or IPv6 address"))
			}

			if toIP != nil {
				if (toIP.To4() != nil) != (viaIP.To4() != nil) {
					allErrs = append(allErrs,
						field.Invalid(p.Index(i), "", "cannot mix IP address families"))
				}
			}
		}
	}

	return allErrs
}

func (v validator) validateNetworkSpecWithBootStrap(
	_ *pkgctx.WebhookRequestContext,
	networkPath *field.Path,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var (
		networkSpec = vm.Spec.Network
		cloudInit   *vmopv1.VirtualMachineBootstrapCloudInitSpec
		linuxPrep   *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
		sysPrep     *vmopv1.VirtualMachineBootstrapSysprepSpec
		allErrs     field.ErrorList
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
					networkPath.Child("nameservers"),
					strings.Join(networkSpec.Nameservers, ","),
					"nameservers is available only for CloudInit when UseGlobalNameserversAsDefault is true",
				))
			}
		} else if linuxPrep == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				networkPath.Child("nameservers"),
				strings.Join(networkSpec.Nameservers, ","),
				"nameservers is available only with the following bootstrap providers: LinuxPrep and Sysprep",
			))
		}
	}

	if len(networkSpec.SearchDomains) > 0 {
		if cloudInit != nil {
			if !ptr.DerefWithDefault(cloudInit.UseGlobalSearchDomainsAsDefault, true) {
				allErrs = append(allErrs, field.Invalid(
					networkPath.Child("searchDomains"),
					strings.Join(networkSpec.SearchDomains, ","),
					"searchDomains is available only for CloudInit when UseGlobalSearchDomainsAsDefault is true",
				))
			}
		} else if linuxPrep == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				networkPath.Child("searchDomains"),
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
	_ *pkgctx.WebhookRequestContext,
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

func (v validator) validateVolumes(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs       field.ErrorList
		volumesPath   = field.NewPath("spec", "volumes")
		volumeSpecMap = sets.New[string]()
		oldVolumesMap = map[string]*vmopv1.VirtualMachineVolume{}
	)

	if oldVM != nil {
		for _, oldVol := range oldVM.Spec.Volumes {
			if oldVol.Name != "" {
				oldVolumesMap[oldVol.Name] = &oldVol
			}
		}
	}

	for i, vol := range vm.Spec.Volumes {
		volPath := volumesPath.Index(i)

		if vol.Name != "" {
			if _, found := volumeSpecMap[vol.Name]; found {
				allErrs = append(allErrs, field.Duplicate(
					volPath.Child("name"), vol.Name))
			} else {
				volumeSpecMap.Insert(vol.Name)
			}
		} else {
			allErrs = append(allErrs, field.Required(volPath.Child("name"), ""))
		}

		if vol.Name != "" {
			errs := validation.NameIsDNSSubdomain(
				pkgutil.CNSAttachmentNameForVolume(vm.Name, vol.Name), false)
			for _, msg := range errs {
				allErrs = append(allErrs, field.Invalid(volPath.Child("name"),
					vol.Name, msg))
			}
		}

		if vol.PersistentVolumeClaim == nil {
			allErrs = append(allErrs, field.Required(
				volPath.Child("persistentVolumeClaim"), ""))
		} else {
			allErrs = append(allErrs,
				v.validateVolumeWithPVC(ctx, oldVM, vm, oldVolumesMap[vol.Name],
					vol, volPath)...)

			if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
				allErrs = append(allErrs,
					v.validateControllerFields(vol, oldVM, *volPath)...,
				)
			}
		}
	}

	return allErrs
}

func (v validator) validateControllerFields(
	vol vmopv1.VirtualMachineVolume,
	oldVM *vmopv1.VirtualMachine,
	volPath field.Path,
) field.ErrorList {
	var allErrs field.ErrorList

	pvc := vol.PersistentVolumeClaim

	unitNumberRequired := oldVM != nil
	if unitNumberRequired && pvc.UnitNumber == nil {
		allErrs = append(allErrs, field.Required(
			volPath.Child("persistentVolumeClaim",
				"unitNumber"),
			"",
		))
	}

	controllerTypeRequired := oldVM != nil ||
		pvc.UnitNumber != nil ||
		pvc.ControllerBusNumber != nil
	if controllerTypeRequired && pvc.ControllerType == "" {
		allErrs = append(allErrs, field.Required(
			volPath.Child("persistentVolumeClaim",
				"controllerType"),
			"",
		))
	}

	controllerBusNumberRequired := oldVM != nil ||
		pvc.UnitNumber != nil ||
		pvc.ControllerType != ""
	if controllerBusNumberRequired && pvc.ControllerBusNumber == nil {
		allErrs = append(allErrs, field.Required(
			volPath.Child("persistentVolumeClaim",
				"controllerBusNumber"),
			"",
		))
	}

	return allErrs
}

func (v validator) validateVolumeWithPVC(
	ctx *pkgctx.WebhookRequestContext,
	oldVM, vm *vmopv1.VirtualMachine,
	oldVol *vmopv1.VirtualMachineVolume,
	vol vmopv1.VirtualMachineVolume,
	volPath *field.Path) field.ErrorList {

	var (
		allErrs field.ErrorList
		pvcPath = volPath.Child("persistentVolumeClaim")
	)

	if vol.PersistentVolumeClaim.ReadOnly {
		allErrs = append(allErrs, field.NotSupported(pvcPath.Child("readOnly"), true, []string{"false"}))
	}

	if vol.PersistentVolumeClaim.ClaimName == "" {
		allErrs = append(allErrs, field.Required(pvcPath.Child("claimName"), ""))
	}

	if oldVol != nil && oldVol.PersistentVolumeClaim != nil &&
		oldVol.PersistentVolumeClaim.ClaimName == vol.PersistentVolumeClaim.ClaimName {

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.ApplicationType,
			oldVol.PersistentVolumeClaim.ApplicationType,
			pvcPath.Child("applicationType"))...)

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.ControllerBusNumber,
			oldVol.PersistentVolumeClaim.ControllerBusNumber,
			pvcPath.Child("controllerBusNumber"))...)

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.ControllerType,
			oldVol.PersistentVolumeClaim.ControllerType,
			pvcPath.Child("controllerType"))...)

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.DiskMode,
			oldVol.PersistentVolumeClaim.DiskMode,
			pvcPath.Child("diskMode"))...)

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.SharingMode,
			oldVol.PersistentVolumeClaim.SharingMode,
			pvcPath.Child("sharingMode"))...)

		allErrs = append(allErrs, validation.ValidateImmutableField(
			vol.PersistentVolumeClaim.UnitNumber,
			oldVol.PersistentVolumeClaim.UnitNumber,
			pvcPath.Child("unitNumber"))...)
	}

	if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		// Validate PVC access mode, sharing mode, and controller combinations
		// if VM is being created or the PVC is being updated.
		// We skip the validation if the PVC is not being updated because
		// we want to avoid rejecting an update because of an external state change.
		volumeChanged := oldVol == nil || !equality.Semantic.DeepEqual(
			*oldVol,
			vol,
		)
		hardwareChanged := oldVM == nil || vm == nil || !equality.Semantic.DeepEqual(
			oldVM.Spec.Hardware,
			vm.Spec.Hardware,
		)

		if !ctx.IsPrivilegedAccount && (volumeChanged || hardwareChanged) {
			allErrs = append(allErrs,
				v.validatePVCAccessModeAndSharingModeCombinations(
					ctx,
					vm,
					vol,
					volPath,
				)...,
			)

		} else {
			ctx.Logger.V(4).Info(
				"Skipping PVC access mode and sharing mode combinations validation",
				"volume", vol.Name,
				"volumeChanged", volumeChanged,
				"hardwareChanged", hardwareChanged,
				"isPrivilegedAccount", ctx.IsPrivilegedAccount,
			)
		}
	}

	return allErrs
}

// validatePVCAccessModeAndSharingModeCombinations validates the combinations
// of PVC access mode, volume sharing mode, and controller sharing mode
// according to the business rules.
func (v validator) validatePVCAccessModeAndSharingModeCombinations(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	volPath *field.Path) field.ErrorList {

	var allErrs field.ErrorList
	pvcPath := volPath.Child("persistentVolumeClaim")

	// Skip validation if claim name is empty (handled by required validation)
	if vol.PersistentVolumeClaim.ClaimName == "" {
		return allErrs
	}

	// Fetch the PVC to get its access modes
	pvc := &corev1.PersistentVolumeClaim{}
	if err := v.client.Get(ctx, ctrlclient.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      vol.PersistentVolumeClaim.ClaimName,
	}, pvc); err != nil {
		// If the PVC doesn't exist, skip validation
		// The PVC existence will be validated elsewhere or at runtime
		if apierrors.IsNotFound(err) {
			return allErrs
		}
		// For other errors, return the error
		allErrs = append(allErrs, field.Invalid(pvcPath.Child("claimName"),
			vol.PersistentVolumeClaim.ClaimName, err.Error()))
		return allErrs
	}

	// Get the controller sharing mode for the specified controller
	controllerSharingMode := v.getControllerSharingMode(
		ctx,
		vm,
		vol.PersistentVolumeClaim.ControllerType,
		vol.PersistentVolumeClaim.ControllerBusNumber,
	)

	// Rule 1: If volume is ReadWriteOnce, the disk's sharing mode cannot be
	// MultiWriter and the controller's sharing mode cannot be Physical.
	if slices.Contains(pvc.Spec.AccessModes, corev1.ReadWriteOnce) {
		if vol.PersistentVolumeClaim.SharingMode == vmopv1.VolumeSharingModeMultiWriter {
			allErrs = append(allErrs,
				field.Invalid(
					pvcPath.Child("sharingMode"),
					vol.PersistentVolumeClaim.SharingMode,
					fmt.Sprintf("Disk MultiWriter sharing mode is not allowed "+
						"for ReadWriteOnce volumes for PVC %s",
						vol.PersistentVolumeClaim.ClaimName),
				),
			)
		}

		if controllerSharingMode == vmopv1.VirtualControllerSharingModePhysical {
			allErrs = append(allErrs,
				field.Invalid(
					pvcPath.Child("controllerType"),
					vol.PersistentVolumeClaim.ControllerType,
					fmt.Sprintf("Physical controller sharing mode is not "+
						"allowed for ReadWriteOnce volume for PVC %s",
						vol.PersistentVolumeClaim.ClaimName),
				),
			)
		}
	}

	// Rule 2: If volume is ReadWriteMany, either the disk's sharing mode
	// is MultiWriter or the controller's sharing mode should be Physical
	if slices.Contains(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		// Check if neither condition is met
		hasMultiWriterSharing := vol.PersistentVolumeClaim.SharingMode == vmopv1.VolumeSharingModeMultiWriter
		hasPhysicalController := controllerSharingMode == vmopv1.VirtualControllerSharingModePhysical

		if !hasMultiWriterSharing && !hasPhysicalController {
			allErrs = append(allErrs,
				field.Invalid(
					pvcPath.Child("accessModes"),
					pvc.Spec.AccessModes,
					fmt.Sprintf(
						"Either disk sharing mode must be MultiWriter or "+
							"controller sharing mode must be Physical for "+
							"ReadWriteMany volume for PVC %s",
						vol.PersistentVolumeClaim.ClaimName,
					),
				),
			)
		}
	}

	return allErrs
}

// getControllerSharingMode gets the sharing mode of the specified controller
// from the VM hardware spec.
// Returns the sharing mode if the controller was found and it supports
// sharing mode, otherwise returns None.
func (v validator) getControllerSharingMode(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	controllerType vmopv1.VirtualControllerType,
	controllerBusNumber *int32) vmopv1.VirtualControllerSharingMode {

	if vm.Spec.Hardware == nil ||
		controllerType == "" ||
		controllerBusNumber == nil {
		return vmopv1.VirtualControllerSharingModeNone
	}

	// Find the controller with the specified type and bus number
	switch controllerType {
	case vmopv1.VirtualControllerTypeSCSI:
		for _, controller := range vm.Spec.Hardware.SCSIControllers {
			if controller.BusNumber == *controllerBusNumber {
				return controller.SharingMode
			}
		}
	case vmopv1.VirtualControllerTypeNVME:
		for _, controller := range vm.Spec.Hardware.NVMEControllers {
			if controller.BusNumber == *controllerBusNumber {
				return controller.SharingMode
			}
		}
	}

	return vmopv1.VirtualControllerSharingModeNone
}

// validateInstanceStorageVolumes validates if instance storage volumes are added/modified.
func (v validator) validateInstanceStorageVolumes(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

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

func (v validator) isNetworkRestrictedForReadinessProbe(ctx *pkgctx.WebhookRequestContext) (bool, error) {
	configMap := &corev1.ConfigMap{}
	configMapKey := ctrlclient.ObjectKey{Name: config.ProviderConfigMapName, Namespace: ctx.Namespace}
	if err := v.client.Get(ctx, configMapKey, configMap); err != nil {
		return false, fmt.Errorf("error get ConfigMap: %s while validating TCP readiness probe port: %w", configMapKey, err)
	}

	return configMap.Data[isRestrictedNetworkKey] == "true", nil
}

func (v validator) validateReadinessProbe(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

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

func (v validator) validateAdvanced(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

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
	_ *pkgctx.WebhookRequestContext,
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
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if vm.Spec.NextRestartTime == oldVM.Spec.NextRestartTime {
		return nil
	}

	var allErrs field.ErrorList

	nextRestartTimePath := field.NewPath("spec").Child("nextRestartTime")

	if strings.EqualFold(vm.Spec.NextRestartTime, "now") {
		allErrs = append(
			allErrs,
			field.Invalid(
				nextRestartTimePath,
				vm.Spec.NextRestartTime,
				invalidNextRestartTimeOnUpdateNow))
	} else if _, err := time.Parse(time.RFC3339Nano, vm.Spec.NextRestartTime); err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				nextRestartTimePath,
				vm.Spec.NextRestartTime,
				invalidRFC3339NanoTimeFormat))
	}

	return allErrs
}

func (v validator) validatePowerStateOnCreate(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs field.ErrorList
	)

	if powerState := vm.Spec.PowerState; powerState == vmopv1.VirtualMachinePowerStateSuspended {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec").Child("powerState"),
				powerState,
				fmt.Sprintf(invalidPowerStateOnCreateFmt, powerState)))
	}

	return allErrs
}

func (v validator) validatePowerStateOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	powerStatePath := field.NewPath("spec").Child("powerState")

	// If a VM is powered off, all config changes are allowed except for setting
	// the power state to shutdown or suspend.
	// If a VM is requesting a power off, we can Reconfigure the VM _after_
	// we power it off - all changes are allowed.
	// If a VM is requesting a power on, we can Reconfigure the VM _before_
	// we power it on - all changes are allowed.
	newDesPS, oldDesPS := vm.Spec.PowerState, oldVM.Spec.PowerState

	if newDesPS == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				powerStatePath,
				newDesPS,
				invalidPowerStateOnUpdateEmptyString))
		return allErrs
	}

	switch oldDesPS {

	case vmopv1.VirtualMachinePowerStateOn:
		// The VM's previous, desired power state is "on."

		switch newDesPS { //nolint:gocritic

		case vmopv1.VirtualMachinePowerStateOn:
			// The VM's new, desired power state is "on."

			// Check if the VM is currently halted from being powered on due to
			// the check annotation.
			isHalted := false
			for k := range oldVM.Annotations {
				if strings.HasPrefix(k, vmopv1.CheckAnnotationPowerOn) {
					isHalted = true
					break
				}
			}

			if !isHalted {
				allErrs = append(
					allErrs,
					v.validateUpdatesWhenPoweredOn(ctx, vm, oldVM)...)
			}

		}

	case vmopv1.VirtualMachinePowerStateOff:
		// The VM's previous, desired power state is "off."

		switch newDesPS { //nolint:gocritic

		case vmopv1.VirtualMachinePowerStateSuspended:
			// The VM's new, desired power state is "suspended."

			allErrs = append(
				allErrs,
				field.Invalid(
					powerStatePath,
					newDesPS,
					fmt.Sprintf(
						invalidPowerStateOnUpdateFmt,
						"suspend",
						"powered off")))
		}
	}

	return allErrs
}

func (v validator) validateUpdatesWhenPoweredOn(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if vm.Spec.GuestID != oldVM.Spec.GuestID {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("guestID"), updatesNotAllowedWhenPowerOn))
	}

	allErrs = append(allErrs, v.validateHardwareWhenPoweredOn(ctx, vm, oldVM)...)

	// TODO: More checks.

	return allErrs
}

func (v validator) validateSchemaUpgrade(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if builder.IsVMOperatorServiceAccount(ctx.WebhookContext, ctx.UserInfo) ||
		builder.IsSystemMasters(ctx.WebhookContext, ctx.UserInfo) {

		// The VM Operator service account and cluster-admin may bypass this
		// check.
		return nil
	}

	var allErrs field.ErrorList
	fieldPath := field.NewPath("metadata", "annotations")

	var (
		newUpBuildVer = newVM.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey]
		newUpSchemVer = newVM.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey]

		oldUpBuildVer = oldVM.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey]
		oldUpSchemVer = oldVM.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey]
	)

	switch {
	case newUpBuildVer != "" && newUpBuildVer != oldUpBuildVer:
		// Prevent most users from modifying these annotations.
		allErrs = append(allErrs, field.Forbidden(
			fieldPath.Key(pkgconst.UpgradedToBuildVersionAnnotationKey),
			modRestrictedAnnotation))

	case oldUpBuildVer != "" && newUpBuildVer == "":
		ctx.Logger.V(4).Info("Deleted annotation",
			"key", pkgconst.UpgradedToBuildVersionAnnotationKey)
	}

	switch {
	case newUpSchemVer != "" && newUpSchemVer != oldUpSchemVer:
		// Prevent most users from modifying these annotations.
		allErrs = append(allErrs, field.Forbidden(
			fieldPath.Key(pkgconst.UpgradedToSchemaVersionAnnotationKey),
			modRestrictedAnnotation))

	case oldUpSchemVer != "" && newUpSchemVer == "":
		// Allow anyone to delete the annotation.
		ctx.Logger.V(4).Info("Deleted annotation",
			"key", pkgconst.UpgradedToSchemaVersionAnnotationKey)
	}

	if oldUpBuildVer == "" || oldUpSchemVer == "" ||
		oldUpBuildVer != pkgcfg.FromContext(ctx).BuildVersion ||
		oldUpSchemVer != vmopv1.GroupVersion.Version {
		// Prevent most users from modifying the VM spec fields,
		// that are backfilled by the schema upgrade and mutable,
		// before the schema upgrade is completed.
		allErrs = append(allErrs,
			v.validateFieldsDuringSchemaUpgrade(newVM, oldVM)...)
	}

	return allErrs
}

func (v validator) validateHardware(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	allErrs = append(allErrs, v.validateCdrom(ctx, newVM, oldVM)...)
	allErrs = append(allErrs, v.validateControllers(ctx, newVM, oldVM)...)

	return allErrs
}

func (v validator) validateHardwareWhenPoweredOn(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	allErrs = append(allErrs, v.validateCdromWhenPoweredOn(ctx, newVM, oldVM)...)
	allErrs = append(allErrs, v.validateBootOptionsWhenPoweredOn(ctx, newVM, oldVM)...)

	return allErrs
}

func (v validator) validateImmutableFields(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

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
	allErrs = append(allErrs, v.validateImmutableVMAffinity(ctx, vm, oldVM)...)

	if pkgcfg.FromContext(ctx).Features.AllDisksArePVCs {
		allErrs = append(allErrs, v.validatePVCUnmanagedVolumeClaimImmutability(ctx, vm, oldVM)...)
	}

	return allErrs
}

func (v validator) validateImmutableReserved(
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

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

func (v validator) validateImmutableNetwork(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	oldNetwork := oldVM.Spec.Network
	newNetwork := vm.Spec.Network

	if oldNetwork == nil && newNetwork == nil {
		return allErrs
	}

	if pkgcfg.FromContext(ctx).Features.MutableNetworks {
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

	// Skip comparing interfaces if this is a fail-over to allow vendors to
	// connect network each interface to the test, or the production network.
	if _, ok := vm.Annotations[vmopv1.FailedOverVMAnnotation]; ok {
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

		allErrs = append(allErrs,
			validation.ValidateImmutableField(newInterface.MACAddr, oldInterface.MACAddr, pi.Child("macAddr"))...)
	}

	return allErrs
}

func (v validator) validateAvailabilityZone(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	zoneLabelPath := field.NewPath("metadata", "labels").Key(corev1.LabelTopologyZone)

	if oldVM != nil {
		// Once the zone has been set then make sure the field is immutable.
		if oldVal := oldVM.Labels[corev1.LabelTopologyZone]; oldVal != "" {
			newVal := vm.Labels[corev1.LabelTopologyZone]

			// Privileged accounts are allowed to update the
			// availability zone label on the VM. This is used during
			// restore, or a fail-over where the restored environment
			// may not have access to the zone from backup.
			//
			// All other modifications are rejected.
			if ctx.IsPrivilegedAccount {
				return allErrs
			}

			return append(allErrs, validation.ValidateImmutableField(newVal, oldVal, zoneLabelPath)...)
		}
	}

	// Validate the name of the provided availability zone.
	if zoneName := vm.Labels[corev1.LabelTopologyZone]; zoneName != "" {
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

// protectedAnnotationRegex matches annotations with keys matching the pattern:
// ^.+\.protected(/.+)?$
//
// Examples that match:
//   - fu.bar.protected
//   - hello.world.protected/sub-key
//   - vmoperator.vmware.com.protected/reconcile-priority
//
// Examples that do NOT match:
//   - protected.fu.bar
//   - hello.world.protected.against/sub-key
var protectedAnnotationRegex = regexp.MustCompile(`^.+\.protected(/.*)?$`)

// validateProtectedAnnotations validates that annotations matching the
// protected annotation pattern can only be modified by privileged users.
func (v validator) validateProtectedAnnotations(vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	annotationPath := field.NewPath("metadata", "annotations")

	// Collect all protected annotation keys from both old and new VMs
	protectedKeys := make(map[string]struct{})

	for k := range vm.Annotations {
		if protectedAnnotationRegex.MatchString(k) {
			protectedKeys[k] = struct{}{}
		}
	}

	for k := range oldVM.Annotations {
		if protectedAnnotationRegex.MatchString(k) {
			protectedKeys[k] = struct{}{}
		}
	}

	// Check if any protected annotations have been modified
	for k := range protectedKeys {
		if vm.Annotations[k] != oldVM.Annotations[k] {
			allErrs = append(allErrs, field.Forbidden(
				annotationPath.Key(k),
				modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	return allErrs
}

func (v validator) validateAnnotation(ctx *pkgctx.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	annotationPath := field.NewPath("metadata", "annotations")

	clusterModuleName := vm.Annotations[pkgconst.ClusterModuleNameAnnotationKey]
	if clusterModuleName != "" {
		if vm.Spec.Reserved == nil || vm.Spec.Reserved.ResourcePolicyName == "" {
			allErrs = append(allErrs, field.Forbidden(
				annotationPath.Key(pkgconst.ClusterModuleNameAnnotationKey),
				"cluster module assignment requires spec.reserved.resourcePolicyName to specify a VirtualMachineSetResourcePolicy"))
		}
	}

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	// Use an empty VM if the oldVM is nil to validate a creation request.
	create := oldVM == nil
	if create {
		oldVM = &vmopv1.VirtualMachine{}
	}

	allErrs = append(allErrs, v.validateProtectedAnnotations(vm, oldVM)...)

	if vm.Annotations[vmopv1.InstanceIDAnnotation] != oldVM.Annotations[vmopv1.InstanceIDAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.InstanceIDAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.FirstBootDoneAnnotation] != oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.FirstBootDoneAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.RestoredVMAnnotation] != oldVM.Annotations[vmopv1.RestoredVMAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.RestoredVMAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.FailedOverVMAnnotation] != oldVM.Annotations[vmopv1.FailedOverVMAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.FailedOverVMAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.ImportedVMAnnotation] != oldVM.Annotations[vmopv1.ImportedVMAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Key(vmopv1.ImportedVMAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	for k := range anno2extraconfig.AnnotationsToExtraConfigKeys {
		if vm.Annotations[k] != oldVM.Annotations[k] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(k), modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	// The following annotations will be added by the mutation webhook upon VM creation.
	if !reflect.DeepEqual(oldVM, &vmopv1.VirtualMachine{}) {
		if vm.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] != oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(pkgconst.CreatedAtBuildVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}

		if vm.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] != oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(pkgconst.CreatedAtSchemaVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	if !create {
		if clusterModuleName != oldVM.Annotations[pkgconst.ClusterModuleNameAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Key(pkgconst.ClusterModuleNameAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	return allErrs
}

func (v validator) validateMinHardwareVersion(
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "minHardwareVersion")

	// kubebuilder validations handles check for minHardwareVersion < 13.
	// This is necessary here to handle brownfield api versions.
	if vimtypes.HardwareVersion(vm.Spec.MinHardwareVersion) > vimtypes.MaxValidHardwareVersion { //nolint:gosec // disable G115
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

func (v validator) validateLabel(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

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
	_ *pkgctx.WebhookRequestContext,
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

func (v validator) validateCdrom(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs field.ErrorList
		newCD   []vmopv1.VirtualMachineCdromSpec
		newHW   = newVM.Spec.Hardware
		f       = field.NewPath("spec", "hardware", "cdrom")
	)

	if newHW != nil {
		newCD = newHW.Cdrom
	}

	if len(newCD) == 0 {
		return allErrs
	}

	// GuestID must be set when deploying an ISO VM with CD-ROMs.
	if newVM.Spec.GuestID == "" {
		allErrs = append(
			allErrs,
			field.Required(
				field.NewPath("spec", "guestID"),
				"when deploying a VM with CD-ROMs"),
		)
	}

	// Validate image kind and uniqueness for each CD-ROM.
	// Namespace and cluster scope images with the same name are considered
	// duplicates since they come from the same content library item file.
	imgNames := make(map[string]struct{}, len(newCD))
	for i, c := range newCD {
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

	if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		if oldVM == nil {
			if newVM.Spec.Hardware != nil {
				allErrs = append(allErrs, v.validateCdromControllerSpecsOnCreate(newCD, f)...)
			}
		} else {
			allErrs = append(allErrs, v.validateCdromControllerSpecsOnUpdate(newCD, f)...)
		}
	}

	return allErrs
}

func (v validator) validateCdromWhenPoweredOn(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs field.ErrorList
		newCD   []vmopv1.VirtualMachineCdromSpec
		oldCD   []vmopv1.VirtualMachineCdromSpec
		newHW   = newVM.Spec.Hardware
		oldHW   = oldVM.Spec.Hardware
		f       = field.NewPath("spec", "hardware", "cdrom")
	)

	if newHW != nil {
		newCD = newHW.Cdrom
	}
	if oldHW != nil {
		oldCD = oldHW.Cdrom
	}

	if len(newCD) != len(oldCD) {
		// Adding or removing CD-ROMs is not allowed when VM is powered on.
		allErrs = append(allErrs, field.Forbidden(f, updatesNotAllowedWhenPowerOn))
		return allErrs
	}

	oldCdromNameToImage := make(map[string]vmopv1.VirtualMachineImageRef, len(oldCD))
	for _, c := range oldCD {
		oldCdromNameToImage[c.Name] = c.Image
	}

	newCdromNames := make(map[string]bool, len(newCD))
	for _, c := range newCD {
		newCdromNames[c.Name] = true
	}
	for _, oldCdrom := range oldCD {
		if !newCdromNames[oldCdrom.Name] {
			// Removing CD-ROMs is not allowed when VM is powered on.
			allErrs = append(allErrs, field.Forbidden(f.Child("name"), removingCdromNotAllowedWhenPowerOn))
		}
	}

	for i, c := range newCD {
		if oldImage, ok := oldCdromNameToImage[c.Name]; !ok {
			// Adding new CD-ROMs is not allowed when VM is powered on.
			allErrs = append(allErrs, field.Forbidden(f.Index(i).Child("name"), addingNewCdromNotAllowedWhenPowerOn))
		} else if !reflect.DeepEqual(c.Image, oldImage) {
			// CD-ROM image is changed.
			allErrs = append(allErrs, field.Forbidden(f.Index(i).Child("image"), updatesNotAllowedWhenPowerOn))
		}
	}

	if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		// Only validate when the VM power state is known which indicates that
		// the VM exists on the vSphere and we have gone through at least
		// one loop of reconciling the VM.
		if oldVM.Status.PowerState != "" {
			allErrs = append(allErrs, v.validateCdromControllerSpecWhenPoweredOn(newCD, oldCD, f)...)
		}
	}

	return allErrs
}

var capvDefaultServiceAccount = regexp.MustCompile("^system:serviceaccount:svc-tkg-domain-[^:]+:default$")

// isCAPVServiceAccount checks if the username matches that of the CAPV service account.
func isCAPVServiceAccount(username string) bool {
	return capvDefaultServiceAccount.Match([]byte(username))
}

func (v validator) validateChecks(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if errs := v.validateChecksAnnotation(
		ctx,
		newVM,
		oldVM,
		vmopv1.CheckAnnotationPowerOn+"/",
		true); len(errs) > 0 {

		allErrs = append(allErrs, errs...)
	}

	if errs := v.validateChecksAnnotation(
		ctx,
		newVM,
		oldVM,
		vmopv1.CheckAnnotationDelete+"/",
		false); len(errs) > 0 {

		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

func (v validator) validateChecksAnnotation(
	ctx *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine,
	prefix string,
	allowNonPrivilegedAddOnCreate bool) field.ErrorList {

	var allErrs field.ErrorList

	if ctx.IsPrivilegedAccount {
		return allErrs
	}

	if oldVM != nil {
		for k := range oldVM.Annotations {
			if strings.HasPrefix(k, prefix) {
				f := field.NewPath("metadata", "annotations", k)
				if _, onNewVM := newVM.Annotations[k]; !onNewVM {
					allErrs = append(allErrs, field.Forbidden(f, delRestrictedAnnotation))
				}
			}
		}
	}

	for k := range newVM.Annotations {
		if strings.HasPrefix(k, prefix) {
			var (
				oldVal    string
				onOldVM   bool
				f         = field.NewPath("metadata", "annotations", k)
				newVal, _ = newVM.Annotations[k]
				create    = oldVM == nil
			)

			if oldVM != nil {
				oldVal, onOldVM = oldVM.Annotations[k]
			}

			if create {
				if !allowNonPrivilegedAddOnCreate {
					allErrs = append(allErrs, field.Forbidden(f, addRestrictedAnnotation))
				}
			} else {
				switch {
				case !onOldVM:
					allErrs = append(allErrs, field.Forbidden(f, addRestrictedAnnotation))
				case oldVal != newVal:
					allErrs = append(allErrs, field.Forbidden(f, modRestrictedAnnotation))
				}
			}
		}
	}

	return allErrs
}

func (v validator) validateNextPowerStateChangeTimeFormat(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if val, ok := vm.Annotations[pkgconst.ApplyPowerStateTimeAnnotation]; ok {
		if _, err := time.Parse(time.RFC3339Nano, val); err != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("metadata").Child("annotations").Key(pkgconst.ApplyPowerStateTimeAnnotation),
					val,
					invalidRFC3339NanoTimeFormat))
		}
	}

	return allErrs
}

func (v validator) validateBootOptionsWhenPoweredOn(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "bootOptions")

	if !reflect.DeepEqual(oldVM.Spec.BootOptions, vm.Spec.BootOptions) {
		allErrs = append(allErrs, field.Forbidden(fieldPath, updatesNotAllowedWhenPowerOn))
	}

	return allErrs
}

func (v validator) validateBootOptions(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "bootOptions")

	bootOptions := vm.Spec.BootOptions
	if bootOptions != nil {
		if bootOptions.BootOrder != nil {
			create := oldVM == nil
			allErrs = append(allErrs, v.validateBootOrder(ctx, vm, create)...)
		}

		// Both fields are required.
		if bootOptions.BootRetryDelay != nil && bootOptions.BootRetry != vmopv1.VirtualMachineBootOptionsBootRetryEnabled {
			allErrs = append(
				allErrs,
				field.Required(
					fieldPath.Child("bootRetry"),
					"bootRetry must be set when setting bootRetryDelay",
				))
		}

		// EFISecureBoot can only be used in conjunction with EFI firmware
		if bootOptions.EFISecureBoot == vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled &&
			bootOptions.Firmware != vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI {

			allErrs = append(
				allErrs,
				field.Forbidden(
					fieldPath.Child("efiSecureBoot"),
					"cannot set efiSecureBoot when image firmware is not 'efi'",
				))
		}
	}

	return allErrs
}

func (v validator) validateBootOrder(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	create bool) field.ErrorList {

	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "bootOptions", "bootOrder")

	if create {
		// We do not allow for specifying bootOrder on create.
		allErrs = append(allErrs, field.Forbidden(fieldPath, "when creating a VM"))
	} else {
		// Validate that the devices listed in bootOrder are valid. That is,
		// validate that the device type is one of the valid options, that
		// the device exists, and that there are not duplicate devices listed.
		bootableDeviceNames := make(map[string]string)
		for i, bd := range vm.Spec.BootOptions.BootOrder {
			// Name is required for BootableNetworkDevices and BootableDiskDevices
			if bd.Type == vmopv1.VirtualMachineBootOptionsBootableNetworkDevice ||
				bd.Type == vmopv1.VirtualMachineBootOptionsBootableDiskDevice && bd.Name == "" {

				if bd.Name == "" {
					allErrs = append(
						allErrs,
						field.Required(
							fieldPath.Index(i).Child("name"),
							fmt.Sprintf("name must not be empty when specifying a bootable device of type %q", bd.Type),
						))

					continue
				}
			}

			// Ensure duplicate devices are not specified
			if _, ok := bootableDeviceNames[bd.Name]; ok {
				allErrs = append(allErrs, field.Duplicate(fieldPath.Index(i), bd.Name))

				continue
			}
			bootableDeviceNames[bd.Name] = ""

			switch bd.Type {
			case vmopv1.VirtualMachineBootOptionsBootableNetworkDevice:
				if vm.Spec.Network == nil {
					allErrs = append(
						allErrs,
						field.Invalid(
							fieldPath.Index(i),
							bd,
							fmt.Sprintf("cannot specify network device %q when vm.spec.network is empty", bd.Name),
						))

					continue
				}
				// Validate that the named network interface is present.
				found := slices.ContainsFunc(vm.Spec.Network.Interfaces, func(iface vmopv1.VirtualMachineNetworkInterfaceSpec) bool {
					return iface.Name == bd.Name
				})

				if !found {
					allErrs = append(
						allErrs,
						field.Invalid(
							fieldPath.Index(i),
							bd,
							fmt.Sprintf("bootable network device %q was not found in vm.Spec.Network.Interfaces", bd.Name),
						))
				}
			case vmopv1.VirtualMachineBootOptionsBootableDiskDevice:
				// Validate that the named disk is present.
				found := slices.ContainsFunc(vm.Status.Volumes, func(disk vmopv1.VirtualMachineVolumeStatus) bool {
					return disk.Name == bd.Name
				})

				if !found {
					allErrs = append(
						allErrs,
						field.Invalid(
							fieldPath.Index(i),
							bd,
							fmt.Sprintf("bootable disk device %q was not found in vm.Status.Volumes", bd.Name),
						))
				}
			case vmopv1.VirtualMachineBootOptionsBootableCDRomDevice:
				// Validate that a CD-ROM device has been defined.
				if len(vm.Spec.Hardware.Cdrom) == 0 {
					allErrs = append(
						allErrs,
						field.Invalid(
							fieldPath.Index(i),
							bd,
							"no CD-ROM device(s) defined for this VM",
						))
				}
			default:
				allErrs = append(
					allErrs,
					field.Invalid(
						fieldPath.Index(i).Child("type"),
						bd,
						fmt.Sprintf("unsupported bootable device type: %q", bd.Type),
					))
			}
		}
	}

	return allErrs
}

func (v validator) validateSnapshot(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	snapshotPath := field.NewPath("spec", "currentSnapshotName")

	// validate a create request
	if oldVM == nil && vm.Spec.CurrentSnapshotName != "" {
		allErrs = append(allErrs, field.Forbidden(snapshotPath, "creating VM with current snapshot is not allowed"))

		return allErrs
	}

	// the Update request has no currentSnapshotName. nothing to validate here
	if vm.Spec.CurrentSnapshotName == "" {
		return allErrs
	}

	// Validate that currentSnapshotName is not empty string if provided
	if strings.TrimSpace(vm.Spec.CurrentSnapshotName) == "" {
		allErrs = append(allErrs, field.Invalid(snapshotPath, vm.Spec.CurrentSnapshotName, "currentSnapshotName cannot be empty"))
		return allErrs
	}

	// If a revert is in progress, we don't allow a new revert.
	if oldVM != nil && oldVM.Spec.CurrentSnapshotName != "" {
		if oldVM.Spec.CurrentSnapshotName != vm.Spec.CurrentSnapshotName {
			allErrs = append(allErrs, field.Forbidden(snapshotPath, "a snapshot revert is already in progress"))
		}
	}

	// Check if the VM is a VKS node and prevent snapshot revert
	if kubeutil.HasCAPILabels(vm.Labels) {
		allErrs = append(allErrs, field.Forbidden(snapshotPath, "snapshot revert is not allowed for VKS nodes"))
	}

	return allErrs
}

func (v validator) validateGroupName(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if vm.Spec.GroupName != "" && !pkgcfg.FromContext(ctx).Features.VMGroups {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "groupName"),
			vm.Spec.GroupName,
			fmt.Sprintf(featureNotEnabled, "VM Groups")),
		)
	}

	return allErrs
}

//nolint:gocyclo
func (v validator) validateVMAffinity(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	affinity := vm.Spec.Affinity
	if affinity == nil {
		return nil
	}

	var allErrs field.ErrorList

	if vm.Spec.GroupName == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "groupName"), "when setting affinity"))
	}

	path := field.NewPath("spec", "affinity")

	if a := affinity.VMAffinity; a != nil {
		p := path.Child("vmAffinity")

		if len(a.RequiredDuringSchedulingPreferredDuringExecution) > 0 {
			p := p.Child("requiredDuringSchedulingPreferredDuringExecution")

			for idx, rs := range a.RequiredDuringSchedulingPreferredDuringExecution {
				p := p.Index(idx)

				if rs.LabelSelector != nil {
					p := p.Child("labelSelector")

					if kubeutil.HasVMOperatorLabels(rs.LabelSelector.MatchLabels) {
						allErrs = append(allErrs, field.Forbidden(p.Child("matchLabels"), labelSelectorCanNotContainVMOperatorLabels))
					}

					for exprIdx, expr := range rs.LabelSelector.MatchExpressions {
						p := p.Child("matchExpressions").Index(exprIdx)

						if expr.Operator != metav1.LabelSelectorOpIn {
							allErrs = append(allErrs, field.NotSupported(
								p.Child("operator"),
								expr.Operator,
								[]metav1.LabelSelectorOperator{metav1.LabelSelectorOpIn}))
						} else if kubeutil.HasVMOperatorLabels(map[string]string{expr.Key: ""}) {
							allErrs = append(allErrs, field.Forbidden(p.Child("key"), labelSelectorCanNotContainVMOperatorLabels))
						}
					}
				}

				if rs.TopologyKey != corev1.LabelTopologyZone {
					allErrs = append(allErrs, field.NotSupported(
						p.Child("topologyKey"), rs.TopologyKey, []string{corev1.LabelTopologyZone}))
				}
			}
		}

		if len(a.PreferredDuringSchedulingPreferredDuringExecution) > 0 {
			p := p.Child("preferredDuringSchedulingPreferredDuringExecution")

			for idx, rs := range a.PreferredDuringSchedulingPreferredDuringExecution {
				p := p.Index(idx)

				if rs.LabelSelector != nil {
					p := p.Child("labelSelector")

					if kubeutil.HasVMOperatorLabels(rs.LabelSelector.MatchLabels) {
						allErrs = append(allErrs, field.Forbidden(p.Child("matchLabels"), labelSelectorCanNotContainVMOperatorLabels))
					}

					for exprIdx, expr := range rs.LabelSelector.MatchExpressions {
						p := p.Child("matchExpressions").Index(exprIdx)

						if expr.Operator != metav1.LabelSelectorOpIn {
							allErrs = append(allErrs, field.NotSupported(
								p.Child("operator"),
								expr.Operator,
								[]metav1.LabelSelectorOperator{metav1.LabelSelectorOpIn}))
						} else if kubeutil.HasVMOperatorLabels(map[string]string{expr.Key: ""}) {
							allErrs = append(allErrs, field.Forbidden(p.Child("key"), labelSelectorCanNotContainVMOperatorLabels))
						}
					}
				}

				if rs.TopologyKey != corev1.LabelTopologyZone {
					allErrs = append(allErrs, field.NotSupported(
						p.Child("topologyKey"), rs.TopologyKey, []string{corev1.LabelTopologyZone}))
				}
			}
		}
	}

	if a := affinity.VMAntiAffinity; a != nil {
		p := path.Child("vmAntiAffinity")

		if len(a.RequiredDuringSchedulingPreferredDuringExecution) > 0 {
			p := p.Child("requiredDuringSchedulingPreferredDuringExecution")

			for idx, rs := range a.RequiredDuringSchedulingPreferredDuringExecution {
				p := p.Index(idx)

				if rs.LabelSelector != nil {
					p := p.Child("labelSelector")

					if kubeutil.HasVMOperatorLabels(rs.LabelSelector.MatchLabels) {
						allErrs = append(allErrs, field.Forbidden(p.Child("matchLabels"), labelSelectorCanNotContainVMOperatorLabels))
					}

					for exprIdx, expr := range rs.LabelSelector.MatchExpressions {
						p := p.Child("matchExpressions").Index(exprIdx)

						if expr.Operator != metav1.LabelSelectorOpIn {
							allErrs = append(allErrs, field.NotSupported(
								p.Child("operator"),
								expr.Operator,
								[]metav1.LabelSelectorOperator{metav1.LabelSelectorOpIn}))
						} else if kubeutil.HasVMOperatorLabels(map[string]string{expr.Key: ""}) {
							allErrs = append(allErrs, field.Forbidden(p.Child("key"), labelSelectorCanNotContainVMOperatorLabels))
						}
					}
				}

				// For non-privileged users, only zone topology key is allowed for anti-affinity required terms.
				if !ctx.IsPrivilegedAccount && rs.TopologyKey != corev1.LabelTopologyZone {
					allErrs = append(allErrs, field.NotSupported(
						p.Child("topologyKey"), rs.TopologyKey, []string{corev1.LabelTopologyZone}))
				}

				// For privileged users (mobility operator), either zone or host topology key is allowed for anti-affinity required terms.
				if ctx.IsPrivilegedAccount && rs.TopologyKey != corev1.LabelTopologyZone && rs.TopologyKey != corev1.LabelHostname {
					allErrs = append(allErrs, field.NotSupported(
						p.Child("topologyKey"), rs.TopologyKey, []string{corev1.LabelTopologyZone, corev1.LabelHostname}))
				}
			}
		}

		if len(a.PreferredDuringSchedulingPreferredDuringExecution) > 0 {
			p := p.Child("preferredDuringSchedulingPreferredDuringExecution")

			for idx, rs := range a.PreferredDuringSchedulingPreferredDuringExecution {
				p := p.Index(idx)

				if rs.LabelSelector != nil {
					p := p.Child("labelSelector")

					if kubeutil.HasVMOperatorLabels(rs.LabelSelector.MatchLabels) {
						allErrs = append(allErrs, field.Forbidden(p.Child("matchLabels"), labelSelectorCanNotContainVMOperatorLabels))
					}

					for exprIdx, expr := range rs.LabelSelector.MatchExpressions {
						p := p.Child("matchExpressions").Index(exprIdx)

						if expr.Operator != metav1.LabelSelectorOpIn {
							allErrs = append(allErrs, field.NotSupported(
								p.Child("operator"),
								expr.Operator,
								[]metav1.LabelSelectorOperator{metav1.LabelSelectorOpIn}))
						} else if kubeutil.HasVMOperatorLabels(map[string]string{expr.Key: ""}) {
							allErrs = append(allErrs, field.Forbidden(p.Child("key"), labelSelectorCanNotContainVMOperatorLabels))
						}
					}
				}

				// Either zone or host topology key is allowed for anti-affinity preferred terms.
				if rs.TopologyKey != corev1.LabelTopologyZone && rs.TopologyKey != corev1.LabelHostname {
					allErrs = append(allErrs, field.NotSupported(
						p.Child("topologyKey"), rs.TopologyKey, []string{corev1.LabelTopologyZone, corev1.LabelHostname}))
				}
			}
		}
	}

	return allErrs
}

func (v validator) validateImmutableVMAffinity(
	_ *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if !equality.Semantic.DeepEqual(vm.Spec.Affinity, oldVM.Spec.Affinity) {
		p := field.NewPath("spec", "affinity")
		allErrs = append(allErrs, field.Forbidden(p, "updating Affinity is not allowed"))
	}

	return allErrs
}

// validateImmutableFieldsDuringSchemaUpgrade checks the fields tht are
// backfilled by the schema upgrade are not been modified before the schema
// upgrade has completed. Currently, the schema upgrade backfills fields below:
// - vm.Spec.BiosUUID
// - vm.Spec.InstanceUUID (immutable if set)
// - vm.Spec.Bootstrap.CloudInit.InstanceID (immutable if set)
// - vm.Spec.Hardware.*Controllers
// Fields that are immutable will not be validated.
func (v validator) validateFieldsDuringSchemaUpgrade(
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs          field.ErrorList
		specPath         = field.NewPath("spec")
		specHardwarePath = specPath.Child("hardware")
	)

	if !equality.Semantic.DeepEqual(vm.Spec.BiosUUID, oldVM.Spec.BiosUUID) {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("biosUUID"), notUpgraded))
	}

	if !equality.Semantic.DeepEqual(oldVM.Spec.Hardware.IDEControllers,
		vm.Spec.Hardware.IDEControllers) {
		allErrs = append(allErrs, field.Forbidden(
			specHardwarePath.Child("ideControllers"), notUpgraded))
	}
	if !equality.Semantic.DeepEqual(oldVM.Spec.Hardware.NVMEControllers,
		vm.Spec.Hardware.NVMEControllers) {
		allErrs = append(allErrs, field.Forbidden(
			specHardwarePath.Child("nvmeControllers"), notUpgraded))
	}
	if !equality.Semantic.DeepEqual(oldVM.Spec.Hardware.SATAControllers,
		vm.Spec.Hardware.SATAControllers) {
		allErrs = append(allErrs, field.Forbidden(
			specHardwarePath.Child("sataControllers"), notUpgraded))
	}
	if !equality.Semantic.DeepEqual(oldVM.Spec.Hardware.SCSIControllers,
		vm.Spec.Hardware.SCSIControllers) {
		allErrs = append(allErrs, field.Forbidden(
			specHardwarePath.Child("scsiControllers"), notUpgraded))
	}

	return allErrs
}

func (v validator) validatePVCUnmanagedVolumeClaimInfo(
	_ *pkgctx.WebhookRequestContext,
	newVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs field.ErrorList
		p       = field.NewPath("spec", "volumes")
	)

	for i, v := range newVM.Spec.Volumes {

		pp := p.Index(i)
		if pvc := v.PersistentVolumeClaim; pvc != nil {

			pp = pp.Child("persistentVolumeClaim")
			if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {

				pp = pp.Child("unmanagedVolumeClaim")

				switch uvc.Type {
				case vmopv1.UnmanagedVolumeClaimVolumeTypeFromImage:
					if uvc.UUID == "" {
						allErrs = append(allErrs, field.Required(
							pp.Child("uuid"),
							"uuid is required when type=FromImage"))
					} else if _, err := uuid.Parse(uvc.UUID); err != nil {
						allErrs = append(allErrs, field.Invalid(
							pp.Child("uuid"),
							uvc.UUID,
							err.Error()))
					}
				case vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM:
					if _, err := uuid.Parse(uvc.Name); err != nil {
						allErrs = append(allErrs, field.Invalid(
							pp.Child("name"),
							uvc.UUID,
							err.Error()))
					}
					if _, err := uuid.Parse(uvc.UUID); err != nil {
						allErrs = append(allErrs, field.Invalid(
							pp.Child("uuid"),
							uvc.UUID,
							err.Error()))
					}
				}
			}
		}
	}

	return allErrs
}

func (v validator) validatePVCUnmanagedVolumeClaimImmutability(
	_ *pkgctx.WebhookRequestContext,
	newVM, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	var (
		allErrs          field.ErrorList
		p                = field.NewPath("spec", "volumes")
		oldUVCsByVolName = map[string]*vmopv1.UnmanagedVolumeClaimVolumeSource{}
	)

	for _, v := range oldVM.Spec.Volumes {
		if pvc := v.PersistentVolumeClaim; pvc != nil {
			if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {
				oldUVCsByVolName[v.Name] = uvc
			}
		}
	}

	for i, v := range newVM.Spec.Volumes {

		pp := p.Index(i)
		if pvc := v.PersistentVolumeClaim; pvc != nil {

			pp = pp.Child("persistentVolumeClaim", "unmanagedVolumeClaim")
			if uvc := pvc.UnmanagedVolumeClaim; uvc != nil {

				if oldUVC := oldUVCsByVolName[v.Name]; oldUVC != nil {
					if err := validation.ValidateImmutableField(
						uvc.Type, oldUVC.Type, pp.Child("type"),
					); err != nil {
						allErrs = append(allErrs, err...)
					}
					if err := validation.ValidateImmutableField(
						uvc.Name, oldUVC.Name, pp.Child("name"),
					); err != nil {
						allErrs = append(allErrs, err...)
					}
					if err := validation.ValidateImmutableField(
						uvc.UUID, oldUVC.UUID, pp.Child("uuid"),
					); err != nil {
						allErrs = append(allErrs, err...)
					}
				}
			} else if oldUVC := oldUVCsByVolName[v.Name]; oldUVC != nil {
				if err := validation.ValidateImmutableField(
					uvc, oldUVC, pp,
				); err != nil {
					allErrs = append(allErrs, err...)
				}
			}
		}
	}

	return allErrs
}

// validateBiosUUID validates that a non-empty spec.biosUUID is a valid UUID.
func (v validator) validateBiosUUID(_ *pkgctx.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	fieldPath := field.NewPath("spec", "biosUUID")
	if vm.Spec.BiosUUID != "" && uuid.Validate(vm.Spec.BiosUUID) != nil {
		allErrs = append(allErrs, field.Invalid(fieldPath, vm.Spec.BiosUUID, "must provide a valid UUID"))
	}

	return allErrs
}
