// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"net/http"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesetresourcepolicy/validation/messages"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachinesetresourcepolicy,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,versions=v1alpha1,name=default.validating.virtualmachinesetresourcepolicy.vmoperator.vmware.com
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesetresourcepolicies/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator())
	if err != nil {
		return errors.Wrapf(err, "failed to create VirtualMachineSetResourcePolicy validation webhook")
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
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSetResourcePolicy{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vmRP, err := v.vmRPFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateMetadata(ctx, vmRP)...)
	validationErrs = append(validationErrs, v.validateMemory(ctx, vmRP)...)
	validationErrs = append(validationErrs, v.validateCPU(ctx, vmRP)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vmRP, err := v.vmRPFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMRP, err := v.vmRPFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateAllowedChanges(ctx, vmRP, oldVMRP)...)
	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vmRP *vmopv1.VirtualMachineSetResourcePolicy) []string {
	var validationErrs []string
	return validationErrs
}

func (v validator) validateMemory(ctx *context.WebhookRequestContext, vmRP *vmopv1.VirtualMachineSetResourcePolicy) []string {
	var validationErrs []string

	reservation, limits := vmRP.Spec.ResourcePool.Reservations, vmRP.Spec.ResourcePool.Limits
	if !isReservationLimitValid(reservation.Memory, limits.Memory) {
		validationErrs = append(validationErrs, messages.InvalidMemoryRequest)
	}

	// TODO: Validate reservation and limit against hardware configuration of all the VMs in the resource pool

	return validationErrs
}

func (v validator) validateCPU(ctx *context.WebhookRequestContext, vmRP *vmopv1.VirtualMachineSetResourcePolicy) []string {
	var validationErrs []string

	reservation, limits := vmRP.Spec.ResourcePool.Reservations, vmRP.Spec.ResourcePool.Limits
	if !isReservationLimitValid(reservation.Cpu, limits.Cpu) {
		validationErrs = append(validationErrs, messages.InvalidCPURequest)
	}

	// TODO: Validate reservation and limit against hardware configuration of all the VMs in the resource pool

	return validationErrs
}

// validateAllowedChanges returns true only if immutable fields have not been modified.
func (v validator) validateAllowedChanges(ctx *context.WebhookRequestContext, vmRP, oldVMRP *vmopv1.VirtualMachineSetResourcePolicy) []string {
	var validationErrs []string

	if !reflect.DeepEqual(vmRP.Spec, oldVMRP.Spec) {
		validationErrs = append(validationErrs, messages.UpdatingImmutableFieldsNotAllowed)
	}

	return validationErrs
}

// vmRPFromUnstructured returns the VirtualMachineSetResourcePolicy from the unstructured object.
func (v validator) vmRPFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSetResourcePolicy, error) {
	vmRP := &vmopv1.VirtualMachineSetResourcePolicy{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmRP); err != nil {
		return nil, err
	}
	return vmRP, nil
}

func isReservationLimitValid(reservation, limit resource.Quantity) bool {
	return reservation.IsZero() || limit.IsZero() || reservation.Value() <= limit.Value()
}
