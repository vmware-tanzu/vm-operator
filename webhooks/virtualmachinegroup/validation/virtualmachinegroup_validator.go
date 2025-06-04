// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName                           = "default"
	modifyAnnotationNotAllowedForNonAdmin = "modifying this annotation is not allowed for non-admin users"
	emptyPowerStateNotAllowedAfterSet     = "cannot set powerState to empty once it's been set"
	invalidTimeFormat                     = "time must be in RFC3339Nano format"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha4-virtualmachinegroup,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegroups,versions=v1alpha4,name=default.validating.virtualmachinegroup.v1alpha4.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineGroup validation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client client.Client) builder.Validator {
	return validator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    client.Client
	converter runtime.UnstructuredConverter
}

// vmGroupFromUnstructured returns the VirtualMachineGroup from the unstructured object.
func (v validator) vmGroupFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineGroup, error) {
	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmGroup); err != nil {
		return nil, err
	}
	return vmGroup, nil
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineGroup{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmGroup, err := v.vmGroupFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vmGroup, nil)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmGroup, err := v.vmGroupFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMGroup, err := v.vmGroupFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateAnnotation(ctx, vmGroup, oldVMGroup)...)
	fieldErrs = append(fieldErrs, v.validatePowerState(ctx, vmGroup, oldVMGroup)...)
	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateAnnotation(ctx *pkgctx.WebhookRequestContext, vmGroup, oldVMGroup *vmopv1.VirtualMachineGroup) field.ErrorList {
	var allErrs field.ErrorList

	// Use an empty VMGroup to validate annotation on creation requests.
	if oldVMGroup == nil {
		oldVMGroup = &vmopv1.VirtualMachineGroup{}
	}

	annotationPath := field.NewPath("metadata", "annotations")

	// Disallow if last-updated-power-state annotation was modified by a non-admin user.
	oldVal := oldVMGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
	newVal := vmGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
	if !ctx.IsPrivilegedAccount && oldVal != newVal {
		allErrs = append(allErrs, field.Forbidden(
			annotationPath.Key(constants.LastUpdatedPowerStateTimeAnnotation),
			modifyAnnotationNotAllowedForNonAdmin))
	}

	// Disallow if the last-updated-power-state annotation value is not in RFC3339Nano format.
	if newVal != "" {
		if _, err := time.Parse(time.RFC3339Nano, newVal); err != nil {
			allErrs = append(allErrs, field.Invalid(
				annotationPath.Key(constants.LastUpdatedPowerStateTimeAnnotation),
				newVal,
				invalidTimeFormat))
		}
	}

	return allErrs
}

func (v validator) validatePowerState(_ *pkgctx.WebhookRequestContext, vmGroup, oldVMGroup *vmopv1.VirtualMachineGroup) field.ErrorList {
	var allErrs field.ErrorList

	powerStatePath := field.NewPath("spec", "powerState")

	// Disallow setting powerState to empty after it's been set.
	if oldVMGroup.Spec.PowerState != "" && vmGroup.Spec.PowerState == "" {
		allErrs = append(allErrs, field.Forbidden(powerStatePath, emptyPowerStateNotAllowedAfterSet))
	}

	return allErrs
}
