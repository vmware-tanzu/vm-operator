// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"

	invalidCPUReqMsg    = "CPU request must not be larger than the CPU limit"
	invalidMemoryReqMsg = "memory request must not be larger than the memory limit"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha6-virtualmachineclass,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineclasses,versions=v1alpha6,name=default.validating.virtualmachineclass.v1alpha6.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineClass validation webhook: %w", err)
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

type validator struct {
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineClass{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmClass, err := v.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	return buildResponse(ctx, v.validatePolicies(ctx, vmClass))
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	// Updates are validated like creates so an existing class cannot be edited
	// into an invalid request/limit combination.
	vmClass, err := v.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	return buildResponse(ctx, v.validatePolicies(ctx, vmClass))
}

func buildResponse(ctx *pkgctx.WebhookRequestContext, fieldErrs field.ErrorList) admission.Response {
	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validatePolicies(_ *pkgctx.WebhookRequestContext, vmClass *vmopv1.VirtualMachineClass) field.ErrorList {
	var allErrs field.ErrorList

	request, limits := vmClass.Spec.Resources.Requests, vmClass.Spec.Resources.Limits
	reqPath := field.NewPath("spec", "resources", "requests")

	allErrs = append(allErrs, validateRequestLimit(
		reqPath.Child("cpu"), request.CPU, limits.CPU, invalidCPUReqMsg)...)
	allErrs = append(allErrs, validateRequestLimit(
		reqPath.Child("memory"), request.Memory, limits.Memory, invalidMemoryReqMsg)...)

	// TODO: Validate req and limit against hardware configuration of the class

	return allErrs
}

// validateRequestLimit validates that a class request range can be satisfied
// by its limit range, per "Size, requests and limits" in the spec: the
// request default must not exceed the limit default, and the smallest request
// (requests.min) must not exceed the largest limit (limits.max). A bound is
// only compared when both sides are set and non-zero; an unset or zero limit
// means unlimited.
func validateRequestLimit(
	path *field.Path,
	request, limit *vmopv1.ResourceQuantityRange,
	msg string) field.ErrorList {

	var allErrs field.ErrorList

	reqDefault := vmopv1.ClassResourceDefault(request)
	limDefault := vmopv1.ClassResourceDefault(limit)
	if !isRequestLimitValid(reqDefault, limDefault) {
		allErrs = append(allErrs, field.Invalid(path, reqDefault.String(), msg))
	}

	if request != nil && request.Min != nil && limit != nil && limit.Max != nil &&
		!isRequestLimitValid(*request.Min, *limit.Max) {

		allErrs = append(allErrs, field.Invalid(
			path.Child("min"), request.Min.String(), msg))
	}

	return allErrs
}

// vmClassFromUnstructured returns the VirtualMachineClass from the unstructured object.
func (v validator) vmClassFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineClass, error) {
	vmClass := &vmopv1.VirtualMachineClass{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmClass); err != nil {
		return nil, err
	}
	return vmClass, nil
}

func isRequestLimitValid(request, limit resource.Quantity) bool {
	return request.IsZero() || limit.IsZero() || request.Cmp(limit) <= 0
}
