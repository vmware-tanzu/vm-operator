// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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
	selfReferenceMemberOrGroupName        = "group cannot have itself as a member or group name"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha5-virtualmachinegroup,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegroups,versions=v1alpha5,name=default.validating.virtualmachinegroup.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(
	ctx *pkgctx.ControllerManagerContext,
	mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(
		ctx,
		mgr,
		webHookName,
		NewValidator(mgr.GetClient()),
	)
	if err != nil {
		return fmt.Errorf(
			"failed to create VirtualMachineGroup validation webhook: %w", err)
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

// vmGroupFromUnstructured returns the VirtualMachineGroup from the unstructured
// object.
func (v validator) vmGroupFromUnstructured(
	obj runtime.Unstructured) (*vmopv1.VirtualMachineGroup, error) {
	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := v.converter.FromUnstructured(
		obj.UnstructuredContent(),
		vmGroup); err != nil {
		return nil, err
	}

	return vmGroup, nil
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(
		reflect.TypeOf(vmopv1.VirtualMachineGroup{}).Name(),
	)
}

func (v validator) ValidateCreate(
	ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmGroup, err := v.vmGroupFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validatePowerState(ctx, vmGroup, nil)...)
	fieldErrs = append(fieldErrs, v.validateBootOrderMembers(ctx, vmGroup)...)
	fieldErrs = append(fieldErrs, v.validateGroupName(ctx, vmGroup)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(
	_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(
	ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmGroup, err := v.vmGroupFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMGroup, err := v.vmGroupFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs,
		v.validatePowerState(ctx, vmGroup, oldVMGroup)...,
	)

	fieldErrs = append(fieldErrs, v.validateBootOrderMembers(ctx, vmGroup)...)
	fieldErrs = append(fieldErrs, v.validateGroupName(ctx, vmGroup)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validatePowerState(
	ctx *pkgctx.WebhookRequestContext,
	newVMGroup, oldVMGroup *vmopv1.VirtualMachineGroup) field.ErrorList {
	// Use an empty VMGroup to validate creation requests.
	if oldVMGroup == nil {
		oldVMGroup = &vmopv1.VirtualMachineGroup{}
	}

	var (
		allErrs           field.ErrorList
		annotationPath    = field.NewPath("metadata", "annotations")
		oldAnnotationTime = oldVMGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
		newAnnotationTime = newVMGroup.Annotations[constants.LastUpdatedPowerStateTimeAnnotation]
	)

	// Disallow if last-updated-power-state annotation was modified directly by
	// non-admin users. It should be updated only by the mutating webhook when
	// the group's power state or next force sync time changes.
	if !ctx.IsPrivilegedAccount {
		powerStateChanged := oldVMGroup.Spec.PowerState !=
			newVMGroup.Spec.PowerState
		syncTimeChanged := oldVMGroup.Spec.NextForcePowerStateSyncTime !=
			newVMGroup.Spec.NextForcePowerStateSyncTime
		annotationChanged := oldAnnotationTime != newAnnotationTime

		if annotationChanged && !powerStateChanged && !syncTimeChanged {
			allErrs = append(allErrs, field.Forbidden(
				annotationPath.Key(constants.LastUpdatedPowerStateTimeAnnotation),
				modifyAnnotationNotAllowedForNonAdmin))
		}
	}

	// Disallow if the last-updated-power-state annotation value is not in the
	// RFC3339Nano format.
	if newAnnotationTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, newAnnotationTime); err != nil {
			allErrs = append(allErrs, field.Invalid(
				annotationPath.Key(constants.LastUpdatedPowerStateTimeAnnotation),
				newAnnotationTime,
				invalidTimeFormat))
		}
	}

	// Disallow setting powerState to empty after it's been set.
	if oldVMGroup.Spec.PowerState != "" && newVMGroup.Spec.PowerState == "" {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec", "powerState"),
			emptyPowerStateNotAllowedAfterSet,
		))
	}

	return allErrs
}

// validateBootOrderMembers validates the members in all boot orders to ensure:
// 1. No duplicate members exist across all boot orders.
// 2. The group does not reference itself as a member.
//
// Note: This function does not check for circular dependencies between groups
// since child group resources may not exist at webhook validation time.
// Cycle detection can be handled by the controller when needed.
func (v validator) validateBootOrderMembers(
	_ *pkgctx.WebhookRequestContext,
	vmGroup *vmopv1.VirtualMachineGroup) field.ErrorList {

	var (
		allErrs     field.ErrorList
		path        = field.NewPath("spec", "bootOrder")
		membersSeen = make(map[string]struct{})
	)

	for bootOrderIdx, bootOrder := range vmGroup.Spec.BootOrder {
		for memberIdx, member := range bootOrder.Members {
			memberKey := fmt.Sprintf("%s/%s", member.Kind, member.Name)
			if _, ok := membersSeen[memberKey]; ok {
				allErrs = append(allErrs, field.Duplicate(
					path.Index(bootOrderIdx).Child("members").Index(memberIdx),
					memberKey,
				))
			} else {
				membersSeen[memberKey] = struct{}{}
			}

			if member.Kind == "VirtualMachineGroup" &&
				member.Name == vmGroup.Name {
				allErrs = append(allErrs, field.Invalid(
					path.Index(bootOrderIdx).Child("members").Index(memberIdx),
					memberKey,
					selfReferenceMemberOrGroupName,
				))
			}
		}
	}

	return allErrs
}

// validateGroupName validates that the group name is not the same as the group.
func (v validator) validateGroupName(
	_ *pkgctx.WebhookRequestContext,
	vmGroup *vmopv1.VirtualMachineGroup) field.ErrorList {

	var allErrs field.ErrorList

	if vmGroup.Spec.GroupName == vmGroup.Name {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "groupName"),
			vmGroup.Spec.GroupName,
			selfReferenceMemberOrGroupName,
		))
	}

	return allErrs
}
