// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass/validation/messages"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachineclass,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineclasses,versions=v1alpha1,name=default.validating.virtualmachineclass.vmoperator.vmware.com,sideEffects=None
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineclasses/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator())
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachineClass validation webhook")
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator() builder.Validator {
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
	var validationErrs []string

	vmClass, err := v.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateMetadata(ctx, vmClass)...)
	validationErrs = append(validationErrs, v.validateMemory(ctx, vmClass)...)
	validationErrs = append(validationErrs, v.validateCPU(ctx, vmClass)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vmClass, err := v.vmClassFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMClass, err := v.vmClassFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateAllowedChanges(ctx, vmClass, oldVMClass)...)
	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vmClass *vmopv1.VirtualMachineClass) []string {
	var validationErrs []string
	return validationErrs
}

func (v validator) validateMemory(ctx *context.WebhookRequestContext, vmClass *vmopv1.VirtualMachineClass) []string {
	var validationErrs []string

	request, limits := vmClass.Spec.Policies.Resources.Requests, vmClass.Spec.Policies.Resources.Limits
	if !isRequestLimitValid(request.Memory, limits.Memory) {
		validationErrs = append(validationErrs, messages.InvalidMemoryRequest)
	}

	// TODO: Validate req and limit against hardware configuration of the class

	return validationErrs
}

func (v validator) validateCPU(ctx *context.WebhookRequestContext, vmClass *vmopv1.VirtualMachineClass) []string {
	var validationErrs []string

	request, limits := vmClass.Spec.Policies.Resources.Requests, vmClass.Spec.Policies.Resources.Limits
	if !isRequestLimitValid(request.Cpu, limits.Cpu) {
		validationErrs = append(validationErrs, messages.InvalidCPURequest)
	}

	// TODO: Validate req and limit against hardware configuration of the class

	return validationErrs
}

// validateAllowedChanges returns true only if immutable fields have not been modified.
func (v validator) validateAllowedChanges(ctx *context.WebhookRequestContext, vmClass, oldVMClass *vmopv1.VirtualMachineClass) []string {
	var validationErrs []string

	if !reflect.DeepEqual(vmClass.Spec, oldVMClass.Spec) {
		validationErrs = append(validationErrs, messages.UpdatingImmutableFieldsNotAllowed)
	}

	return validationErrs
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
