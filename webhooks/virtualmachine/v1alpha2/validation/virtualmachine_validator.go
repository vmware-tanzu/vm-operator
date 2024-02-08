// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	goctx "context"
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

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsstoragev1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/storagepolicy/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	cloudinitvalidate "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/validate"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
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
	storagePolicyQuotaNotFoundFmt            = "StoragePolicyQuota not found in namespace %s: error: %s"
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
	invalidMinHardwareVersionDowngrade       = "cannot downgrade hardware version"
	invalidMinHardwareVersionPowerState      = "cannot upgrade hardware version unless powered off"
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
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, nil)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
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
//   - Minimum VM Hardware Version
//
// Following fields can only be changed when the VM is powered off.
//   - Bootstrap
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
	fieldErrs = append(fieldErrs, v.validateNextRestartTimeOnUpdate(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vm, oldVM)...)
	fieldErrs = append(fieldErrs, v.validateMinHardwareVersion(ctx, vm, oldVM)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateBootstrap(
	ctx *context.WebhookRequestContext,
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

		if pkgconfig.FromContext(ctx).Features.WindowsSysprep {
			if sysPrep.Sysprep != nil && sysPrep.RawSysprep != nil {
				allErrs = append(allErrs, field.Invalid(p, "sysPrep",
					"sysprep and rawSysprep are mutually exclusive"))
			} else if sysPrep.Sysprep == nil && sysPrep.RawSysprep == nil {
				allErrs = append(allErrs, field.Invalid(p, "sysPrep",
					"either sysprep or rawSysprep must be provided"))
			}

			if sysPrep.Sysprep != nil {
				allErrs = append(allErrs, v.validateInlineSysprep(p, sysPrep.Sysprep)...)
			}
		} else {
			allErrs = append(allErrs, field.Invalid(p, "Sysprep", fmt.Sprintf(featureNotEnabled, "Sysprep")))
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

func (v validator) validateInlineSysprep(p *field.Path, sysprep *sysprep.Sysprep) field.ErrorList {
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
		if identification.JoinDomain != "" && identification.JoinWorkgroup != "" {
			allErrs = append(allErrs, field.Invalid(s, "identification",
				"joinDomain and joinWorkgroup are mutually exclusive"))
		}

		if identification.JoinDomain != "" {
			if identification.DomainAdmin == "" ||
				identification.DomainAdminPassword == nil ||
				identification.DomainAdminPassword.Name == "" {
				allErrs = append(allErrs, field.Invalid(s, "identification",
					"joinDomain requires domainAdmin and domainAdminPassword selector to be set"))
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

	var zones []topologyv1.AvailabilityZone
	if pkgconfig.FromContext(ctx).Features.PodVMOnStretchedSupervisor {
		azs, err := topology.GetAvailabilityZones(ctx, v.client)
		if err != nil {
			return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storagePolicyQuotaNotFoundFmt, vm.Namespace, err.Error())))
		}
		zones = azs
	}

	if pkgconfig.FromContext(ctx).Features.PodVMOnStretchedSupervisor && len(zones) > 1 {
		storagePolicyQuota := &cnsstoragev1.StoragePolicyQuota{}
		if err := v.client.Get(ctx, client.ObjectKey{
			Namespace: vm.Namespace,
			Name:      util.CNSStoragePolicyQuotaName(scName),
		}, storagePolicyQuota); err != nil {
			return append(allErrs, field.Invalid(scPath, scName, fmt.Sprintf(storagePolicyQuotaNotFoundFmt, vm.Namespace, err.Error())))
		}
	} else {
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

	return nil
}

func (v validator) validateNetwork(ctx *context.WebhookRequestContext, vm *vmopv1.VirtualMachine) field.ErrorList {
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
	networkName := interfaceSpec.Network.Name

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
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	networkSpec := vm.Spec.Network
	if networkSpec == nil {
		return allErrs
	}

	var (
		linuxPrep *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
		sysPrep   *vmopv1.VirtualMachineBootstrapSysprepSpec
	)

	if vm.Spec.Bootstrap != nil {
		linuxPrep = vm.Spec.Bootstrap.LinuxPrep
		sysPrep = vm.Spec.Bootstrap.Sysprep
	}

	if len(networkSpec.Nameservers) > 0 {
		if linuxPrep == nil && sysPrep == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "network", "nameservers"),
				strings.Join(networkSpec.Nameservers, ","),
				"nameservers is available only with the following bootstrap providers: LinuxPrep and Sysprep",
			))
		}
	}

	if len(networkSpec.SearchDomains) > 0 {
		if linuxPrep == nil && sysPrep == nil {
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
	ctx goctx.Context,
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
			errs := validation.NameIsDNSSubdomain(util.CNSAttachmentNameForVolume(vm.Name, vol.Name), false)
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
		oldVMInstanceStorageVolumes = instancestorage.FilterVolumes(oldVM)
	}

	if !equality.Semantic.DeepEqual(instancestorage.FilterVolumes(vm), oldVMInstanceStorageVolumes) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "volumes"), addingModifyingInstanceVolumesNotAllowed))
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

func (v validator) validateUpdatesWhenPoweredOn(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if !equality.Semantic.DeepEqual(vm.Spec.Bootstrap, oldVM.Spec.Bootstrap) {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("bootstrap"), updatesNotAllowedWhenPowerOn))
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
	allErrs = append(allErrs, v.validateImmutableReserved(ctx, vm, oldVM)...)
	allErrs = append(allErrs, v.validateImmutableNetwork(ctx, vm, oldVM)...)

	return allErrs
}

func (v validator) validateImmutableReserved(_ *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
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

func (v validator) validateImmutableNetwork(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList

	oldNetwork := oldVM.Spec.Network
	newNetwork := vm.Spec.Network

	if oldNetwork == nil && newNetwork == nil {
		return allErrs
	}

	p := field.NewPath("spec", "network")

	oldInterfaces := oldVM.Spec.Network.Interfaces
	newInterfaces := vm.Spec.Network.Interfaces

	if len(oldInterfaces) != len(newInterfaces) {
		return append(allErrs, field.Forbidden(p.Child("interfaces"), "network interfaces cannot be added or removed"))
	}

	for i := range newInterfaces {
		newInterface := &newInterfaces[i]
		oldInterface := &oldInterfaces[i]
		pi := p.Child("interfaces").Index(i)

		if newInterface.Name != oldInterface.Name {
			allErrs = append(allErrs, field.Forbidden(pi.Child("name"), validation.FieldImmutableErrorMsg))
		}
		if !reflect.DeepEqual(&newInterface.Network, &oldInterface.Network) {
			allErrs = append(allErrs, field.Forbidden(pi.Child("network"), validation.FieldImmutableErrorMsg))
		}
	}

	return allErrs
}

func (v validator) validateAvailabilityZone(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
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

func (v validator) validateAnnotation(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
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
		allErrs = append(allErrs, field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	if vm.Annotations[vmopv1.FirstBootDoneAnnotation] != oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] {
		allErrs = append(allErrs, field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), modifyAnnotationNotAllowedForNonAdmin))
	}

	// The following annotations will be added by the mutation webhook upon VM creation.
	if !reflect.DeepEqual(oldVM, &vmopv1.VirtualMachine{}) {
		if vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] != oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Child(constants.CreatedAtBuildVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}

		if vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] != oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] {
			allErrs = append(allErrs, field.Forbidden(annotationPath.Child(constants.CreatedAtSchemaVersionAnnotationKey), modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	return allErrs
}

func (v validator) validateMinHardwareVersion(ctx *context.WebhookRequestContext, vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {
	var allErrs field.ErrorList
	fieldPath := field.NewPath("spec", "minHardwareVersion")

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

	return allErrs
}
