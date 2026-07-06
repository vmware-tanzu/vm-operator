// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// clusterMoIDPattern matches the managed object ID vCenter assigns to a
// ClusterComputeResource, e.g. domain-c21.
var clusterMoIDPattern = regexp.MustCompile(`^domain-c[0-9]+$`)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vim-vmware-com-v1alpha1-configtarget,mutating=false,failurePolicy=fail,groups=vim.vmware.com,resources=configtargets,versions=v1alpha1,name=default.validating.configtarget.v1alpha1.vim.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vim.vmware.com,resources=configtargets,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create ConfigTarget validation webhook: %w", err)
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
	return vimv1.GroupVersion.WithKind(reflect.TypeFor[vimv1.ConfigTarget]().Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	configTarget, err := v.configTargetFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := append(v.validateName(configTarget), v.validateSpec(configTarget)...)

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
	configTarget, err := v.configTargetFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldConfigTarget, err := v.configTargetFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := v.validateAllowedChanges(configTarget, oldConfigTarget)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// validateName returns an error if metadata.name is not a valid vSphere
// cluster managed object ID.
func (v validator) validateName(configTarget *vimv1.ConfigTarget) field.ErrorList {
	if clusterMoIDPattern.MatchString(configTarget.Name) {
		return nil
	}

	return field.ErrorList{
		field.Invalid(
			field.NewPath("metadata", "name"),
			configTarget.Name,
			"must be a valid vSphere cluster managed object ID, e.g. domain-c21"),
	}
}

// validateSpec returns an error if spec.id is empty.
func (v validator) validateSpec(configTarget *vimv1.ConfigTarget) field.ErrorList {
	if configTarget.Spec.ID.ID != "" {
		return nil
	}

	return field.ErrorList{
		field.Required(field.NewPath("spec", "id", "id"), ""),
	}
}

// validateAllowedChanges returns an error if any immutable fields have been
// modified.
func (v validator) validateAllowedChanges(configTarget, oldConfigTarget *vimv1.ConfigTarget) field.ErrorList {
	return validation.ValidateImmutableField(
		configTarget.Spec.ID, oldConfigTarget.Spec.ID, field.NewPath("spec", "id"))
}

// configTargetFromUnstructured returns the ConfigTarget from the unstructured object.
func (v validator) configTargetFromUnstructured(obj runtime.Unstructured) (*vimv1.ConfigTarget, error) {
	configTarget := &vimv1.ConfigTarget{}

	err := v.converter.FromUnstructured(obj.UnstructuredContent(), configTarget)
	if err != nil {
		return nil, err
	}

	return configTarget, nil
}
