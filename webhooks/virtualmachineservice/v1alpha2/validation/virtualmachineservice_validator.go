// Copyright 2014 The Kubernetes Authors.
// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilnet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

var (
	supportedServiceType = sets.NewString(
		string(vmopv1.VirtualMachineServiceTypeLoadBalancer),
		string(vmopv1.VirtualMachineServiceTypeClusterIP),
		string(vmopv1.VirtualMachineServiceTypeExternalName),
	)

	supportedPortProtocols = sets.NewString(
		string(corev1.ProtocolTCP),
		string(corev1.ProtocolUDP),
		string(corev1.ProtocolSCTP),
	)
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha2-virtualmachineservice,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineservices,versions=v1alpha2,name=default.validating.virtualmachineservice.v1alpha2.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachineService validation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(_ client.Client) builder.Validator {
	return validator{
		converter: runtime.DefaultUnstructuredConverter,
	}
}

// NOTE: Much of this is based on the ValidateService() from k8s's pkg/apis/core/validation/validation.go
// since we should do our best at not allowing the create or update of a VirtualMachineService that cannot
// be transformed into a valid Service.

type validator struct {
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineService{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	vmService, err := v.vmServiceFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList
	fieldErrs = append(fieldErrs, v.validateMetadata(ctx, vmService)...)
	fieldErrs = append(fieldErrs, v.validateSpec(ctx, vmService)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	vmService, err := v.vmServiceFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMService, err := v.vmServiceFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList
	fieldErrs = append(fieldErrs, v.validateAllowedChanges(ctx, vmService, oldVMService)...)
	fieldErrs = append(fieldErrs, v.validateSpec(ctx, vmService)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vmService *vmopv1.VirtualMachineService) field.ErrorList {
	mdPath := field.NewPath("metadata")

	var allErrs field.ErrorList
	allErrs = append(allErrs, ValidateDNS1123Label(vmService.Name, mdPath.Child("name"))...)

	return allErrs
}

func (v validator) validateSpec(ctx *context.WebhookRequestContext, vmService *vmopv1.VirtualMachineService) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	switch vmService.Spec.Type {
	case vmopv1.VirtualMachineServiceTypeLoadBalancer:
		if isHeadlessVMService(vmService) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("clusterIP"), vmService.Spec.ClusterIP,
				"may not be set to 'None' for LoadBalancer services"))
		}

	case vmopv1.VirtualMachineServiceTypeExternalName:
		if vmService.Spec.ClusterIP != "" {
			allErrs = append(allErrs, field.Forbidden(specPath.Child("clusterIP"), "may not be set for ExternalName services"))
		}

		// The value (a CNAME) may have a trailing dot to denote it as fully qualified.
		cname := strings.TrimSuffix(vmService.Spec.ExternalName, ".")
		if len(cname) > 0 {
			for _, msg := range validation.IsDNS1123Subdomain(cname) {
				allErrs = append(allErrs, field.Invalid(specPath.Child("externalName"), cname, msg))
			}
		} else {
			allErrs = append(allErrs, field.Required(specPath.Child("externalName"), ""))
		}
	}

	allErrs = append(allErrs, validatePorts(vmService, specPath)...)

	if vmService.Spec.Selector != nil {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabels(vmService.Spec.Selector, specPath.Child("selector"))...)
	}

	if clusterIP := vmService.Spec.ClusterIP; clusterIP != "" && clusterIP != corev1.ClusterIPNone {
		for _, msg := range validation.IsValidIP(clusterIP) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("clusterIP"), clusterIP, msg))
		}
	}

	if vmService.Spec.Type == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("type"), ""))
	} else if !supportedServiceType.Has(string(vmService.Spec.Type)) {
		allErrs = append(allErrs, field.NotSupported(specPath.Child("type"), vmService.Spec.Type, supportedServiceType.List()))
	}

	if len(vmService.Spec.LoadBalancerSourceRanges) > 0 {
		fldPath := specPath.Child("loadBalancerSourceRanges")

		if vmService.Spec.Type != vmopv1.VirtualMachineServiceTypeLoadBalancer {
			allErrs = append(allErrs, field.Forbidden(fldPath, "may only be used when `type` is 'LoadBalancer'"))
		}

		_, err := utilnet.ParseIPNets(vmService.Spec.LoadBalancerSourceRanges...)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath, fmt.Sprintf("%v", vmService.Spec.LoadBalancerSourceRanges),
				"must be a list of IP ranges. For example, 10.240.0.0/24,10.250.0.0/24"))
		}
	}

	return allErrs
}

func validatePorts(vmService *vmopv1.VirtualMachineService, specPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	portsPath := specPath.Child("ports")

	if len(vmService.Spec.Ports) == 0 && !isHeadlessVMService(vmService) && vmService.Spec.Type != vmopv1.VirtualMachineServiceTypeExternalName {
		allErrs = append(allErrs, field.Required(portsPath, ""))
	}

	allPortNames := sets.Set[string]{}
	for i := range vmService.Spec.Ports {
		portPath := portsPath.Index(i)
		allErrs = append(allErrs, validateServicePort(&vmService.Spec.Ports[i], len(vmService.Spec.Ports) > 1, &allPortNames, portPath)...)
	}

	// Check for duplicate Ports, considering (protocol,port) pairs.
	ports := make(map[vmopv1.VirtualMachineServicePort]bool)
	for i, port := range vmService.Spec.Ports {
		portPath := portsPath.Index(i)
		key := vmopv1.VirtualMachineServicePort{Protocol: port.Protocol, Port: port.Port}
		_, found := ports[key]
		if found {
			allErrs = append(allErrs, field.Duplicate(portPath, key))
		}
		ports[key] = true
	}

	return allErrs
}

func validateServicePort(sp *vmopv1.VirtualMachineServicePort, requireName bool, allNames *sets.Set[string], fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if requireName && len(sp.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), ""))
	} else if len(sp.Name) != 0 {
		allErrs = append(allErrs, ValidateDNS1123Label(sp.Name, fldPath.Child("name"))...)
		if allNames.Has(sp.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("name"), sp.Name))
		} else {
			allNames.Insert(sp.Name)
		}
	}

	for _, msg := range validation.IsValidPortNum(int(sp.Port)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("port"), sp.Port, msg))
	}

	if len(sp.Protocol) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("protocol"), ""))
	} else if !supportedPortProtocols.Has(sp.Protocol) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("protocol"), sp.Protocol, supportedPortProtocols.List()))
	}

	for _, msg := range validation.IsValidPortNum(int(sp.TargetPort)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("targetPort"), sp.TargetPort, msg))
	}

	return allErrs
}

// There is much more nuance to this for a Service - like changing the Type - but let this be
// pretty simple for now.
func (v validator) validateAllowedChanges(ctx *context.WebhookRequestContext, vmService, oldVMService *vmopv1.VirtualMachineService) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if vmService.Spec.Type != oldVMService.Spec.Type {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("type"), "field is immutable"))
	}

	// Service's ClusterIP cannot be changed through updates so neither should ours.
	if vmService.Spec.ClusterIP != oldVMService.Spec.ClusterIP {
		allErrs = append(allErrs, field.Forbidden(specPath.Child("clusterIP"), "field is immutable"))
	}

	return allErrs
}

// vmServiceFromUnstructured returns the VirtualMachineService from the unstructured object.
func (v validator) vmServiceFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineService, error) {
	vmService := &vmopv1.VirtualMachineService{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmService); err != nil {
		return nil, err
	}
	return vmService, nil
}

func isHeadlessVMService(vmService *vmopv1.VirtualMachineService) bool {
	return vmService.Spec.ClusterIP == corev1.ClusterIPNone
}

func ValidateDNS1123Label(value string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for _, msg := range validation.IsDNS1123Label(value) {
		allErrs = append(allErrs, field.Invalid(fldPath, value, msg))
	}
	return allErrs
}
