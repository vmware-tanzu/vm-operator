// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineservice/validation/messages"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-virtualmachineservice,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachineservices,versions=v1alpha1,name=default.validating.virtualmachineservice.vmoperator.vmware.com,sideEffects=None
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

type validator struct {
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineService{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vmService, err := v.vmServiceFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateMetadata(ctx, vmService)...)
	validationErrs = append(validationErrs, v.validateSpec(ctx, vmService)...)

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	var validationErrs []string

	vmService, err := v.vmServiceFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMService, err := v.vmServiceFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	validationErrs = append(validationErrs, v.validateAllowedChanges(ctx, vmService, oldVMService)...)
	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, vmService *vmopv1.VirtualMachineService) []string {
	var validationErrs []string

	if len(validation.IsDNS1123Label(vmService.Name)) != 0 {
		validationErrs = append(validationErrs, fmt.Sprintf(messages.NameNotDNSComplaint, vmService.Name))
	}

	return validationErrs
}

func (v validator) validateSpec(ctx *context.WebhookRequestContext, vmService *vmopv1.VirtualMachineService) []string {
	var validationErrs []string

	if vmService.Spec.Type == "" {
		validationErrs = append(validationErrs, messages.TypeNotSpecified)
	}
	if len(vmService.Spec.Ports) == 0 {
		validationErrs = append(validationErrs, messages.PortsNotSpecified)
	}
	if len(vmService.Spec.Selector) == 0 {
		validationErrs = append(validationErrs, messages.SelectorNotSpecified)
	}

	return validationErrs
}

// validateAllowedChanges returns true only if immutable fields have not been modified.
func (v validator) validateAllowedChanges(ctx *context.WebhookRequestContext, vmService, oldVMService *vmopv1.VirtualMachineService) []string {
	var validationErrs []string
	return validationErrs
}

// vmServiceFromUnstructured returns the VirtualMachineService from the unstructured object.
func (v validator) vmServiceFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineService, error) {
	vmService := &vmopv1.VirtualMachineService{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmService); err != nil {
		return nil, err
	}
	return vmService, nil
}
