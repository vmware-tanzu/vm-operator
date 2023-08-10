// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
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

	"github.com/pkg/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"

	invalidCPUReqMsg    = "CPU request must not be larger than the CPU limit"
	invalidMemoryReqMsg = "memory request must not be larger than the memory limit"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha2-virtualmachineclass,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineclasses,versions=v1alpha2,name=default.validating.virtualmachineclass.v1alpha2.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachineClass validation webhook")
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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineClass{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	vmClass, err := v.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validatePolicies(ctx, vmClass, field.NewPath("spec", "policies"))...)

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
	var fieldErrs field.ErrorList
	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validatePolicies(ctx *context.WebhookRequestContext, vmClass *vmopv1.VirtualMachineClass,
	polPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	request, limits := vmClass.Spec.Policies.Resources.Requests, vmClass.Spec.Policies.Resources.Limits
	reqPath := polPath.Child("resources", "requests")

	// Validate the CPU request.
	if !isRequestLimitValid(request.Cpu, limits.Cpu) {
		allErrs = append(allErrs, field.Invalid(reqPath.Child("cpu"), request.Cpu.String(), invalidCPUReqMsg))
	}

	// Validate the memory request.
	if !isRequestLimitValid(request.Memory, limits.Memory) {
		allErrs = append(allErrs, field.Invalid(reqPath.Child("memory"), request.Memory.String(), invalidMemoryReqMsg))
	}

	// TODO: Validate req and limit against hardware configuration of the class

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
	return request.IsZero() || limit.IsZero() || request.Value() <= limit.Value()
}
