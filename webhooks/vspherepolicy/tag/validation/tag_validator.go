// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

var (
	specKeyPath   = field.NewPath("spec", "key")
	specValuePath = field.NewPath("spec", "value")
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vsphere-policy-vmware-com-v1alpha1-tag,mutating=false,failurePolicy=fail,groups=vsphere.policy.vmware.com,resources=tags,versions=v1alpha1,name=default.validating.tag.v1alpha1.vsphere.policy.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tags,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create Tag validation webhook: %w", err)
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

// For returns the GroupVersionKind that this validator handles.
func (v validator) For() schema.GroupVersionKind {
	return vspherepolv1.GroupVersion.WithKind(reflect.TypeFor[vspherepolv1.Tag]().Name())
}

// ValidateCreate requires a valid label key and value whose derived name
// matches metadata.name. Authorization is enforced by RBAC.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, validateLabelKey(tag.Spec.Key)...)
	fieldErrs = append(fieldErrs, validateLabelValue(tag.Spec.Value)...)
	fieldErrs = append(fieldErrs, validateDerivedName(tag)...)

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateUpdate requires a valid label key and value. Immutability of
// spec.key and spec.value is enforced by CEL rules on the CRD.
// Authorization is enforced by RBAC.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, validateLabelKey(tag.Spec.Key)...)
	fieldErrs = append(fieldErrs, validateLabelValue(tag.Spec.Value)...)

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateDelete allows deletion; authorization is enforced by RBAC.
// DELETE is not registered with this webhook.
func (v validator) ValidateDelete(_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// validateLabelKey requires spec.key to be non-empty and a valid
// Kubernetes label key.
func validateLabelKey(key string) field.ErrorList {
	return metavalidation.ValidateLabelName(key, specKeyPath)
}

// validateLabelValue requires spec.value to be a valid Kubernetes label
// value. An empty value is permitted.
func validateLabelValue(value string) field.ErrorList {
	var fieldErrs field.ErrorList
	for _, msg := range utilvalidation.IsValidLabelValue(value) {
		fieldErrs = append(fieldErrs, field.Invalid(specValuePath, value, msg))
	}

	return fieldErrs
}

// validateDerivedName requires metadata.name to equal the name derived
// from the label key/value pair, so a Tag is always resolvable by the
// keyed Get the VM reconcile path uses.
func validateDerivedName(tag *vspherepolv1.Tag) field.ErrorList {
	wantName := virtualmachine.TagResourceName(tag.Namespace, tag.Spec.Key, tag.Spec.Value)
	if tag.Name == wantName {
		return nil
	}

	return field.ErrorList{
		field.Invalid(
			field.NewPath("metadata", "name"),
			tag.Name,
			fmt.Sprintf("must equal %q, the name derived from spec.key and spec.value", wantName)),
	}
}

// tagFromUnstructured returns the Tag from the unstructured object.
func (v validator) tagFromUnstructured(obj runtime.Unstructured) (*vspherepolv1.Tag, error) {
	tag := &vspherepolv1.Tag{}

	err := v.converter.FromUnstructured(obj.UnstructuredContent(), tag)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Tag from unstructured: %w", err)
	}

	return tag, nil
}

// fieldErrsToStrings converts a field.ErrorList to a slice of strings.
func fieldErrsToStrings(fieldErrs field.ErrorList) []string {
	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return validationErrs
}
