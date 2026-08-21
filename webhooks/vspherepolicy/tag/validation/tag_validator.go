// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
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

// allowedSystemAccountsForTag are the control-plane clients that must be
// allowed to write a Tag even though they are not privileged accounts.
// generic-garbage-collector prunes a dangling owner reference on UPDATE and
// deletes a Tag with zero owners on DELETE; namespace-controller deletes a
// Tag as part of namespace teardown on DELETE. Without this allow-list,
// both operations are denied, which wedges owner-reference cleanup and
// namespace termination respectively.
var allowedSystemAccountsForTag = map[string]struct{}{
	"system:serviceaccount:kube-system:generic-garbage-collector": {},
	"system:serviceaccount:kube-system:namespace-controller":      {},
}

var (
	specKeyPath   = field.NewPath("spec", "key")
	specValuePath = field.NewPath("spec", "value")
	metadataPath  = field.NewPath("metadata")
)

// +kubebuilder:webhook:verbs=create;update;delete,path=/default-validate-vsphere-policy-vmware-com-v1alpha1-tag,mutating=false,failurePolicy=fail,groups=vsphere.policy.vmware.com,resources=tags,versions=v1alpha1,name=default.validating.tag.v1alpha1.vsphere.policy.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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

// ValidateCreate admits only privileged requesters, and requires a valid
// label key and value whose derived name matches metadata.name.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	if resp := v.validatePrivileged(ctx, admissionv1.Create); resp != nil {
		return *resp
	}

	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := make(field.ErrorList, 0)

	fieldErrs = append(fieldErrs, validateLabelKey(tag.Spec.Key)...)
	fieldErrs = append(fieldErrs, validateLabelValue(tag.Spec.Value)...)
	fieldErrs = append(fieldErrs, validateDerivedName(tag)...)

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateUpdate admits only privileged requesters, requires a valid label
// key and value, and rejects any change to either — the pair is the
// resource's identity, since the name is derived from it.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	if resp := v.validatePrivileged(ctx, admissionv1.Update); resp != nil {
		return *resp
	}

	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldTag, err := v.tagFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := make(field.ErrorList, 0)

	fieldErrs = append(fieldErrs, validateLabelKey(tag.Spec.Key)...)
	fieldErrs = append(fieldErrs, validateLabelValue(tag.Spec.Value)...)

	if tag.Spec.Key != oldTag.Spec.Key {
		fieldErrs = append(fieldErrs, field.Forbidden(specKeyPath, "field is immutable"))
	}

	if tag.Spec.Value != oldTag.Spec.Value {
		fieldErrs = append(fieldErrs, field.Forbidden(specValuePath, "field is immutable"))
	}

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateDelete admits only privileged requesters.
func (v validator) ValidateDelete(ctx *pkgctx.WebhookRequestContext) admission.Response {
	if resp := v.validatePrivileged(ctx, admissionv1.Delete); resp != nil {
		return *resp
	}

	return common.BuildValidationResponse(ctx, nil, nil, nil)
}

// validatePrivileged permits only a privileged account, or one of the
// allow-listed system service accounts, to create, update or delete a Tag.
// The allow-list is checked before IsPrivilegedAccount, and neither test
// admits a DevOps user: the Tag API is internal bookkeeping and is not
// exposed to namespace users.
func (v validator) validatePrivileged(
	ctx *pkgctx.WebhookRequestContext,
	op admissionv1.Operation) *admission.Response {
	if _, ok := allowedSystemAccountsForTag[ctx.UserInfo.Username]; ok {
		return nil
	}

	if ctx.IsPrivilegedAccount {
		return nil
	}

	resp := common.BuildValidationResponse(ctx, nil, []string{
		field.Forbidden(metadataPath,
			fmt.Sprintf("only privileged users may %s a Tag", op)).Error(),
	}, nil)

	return &resp
}

// validateLabelKey requires spec.key to be non-empty and a valid
// Kubernetes label key.
func validateLabelKey(key string) field.ErrorList {
	return unversionedvalidation.ValidateLabelName(key, specKeyPath)
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
	wantName := virtualmachine.TagResourceName(tag.Spec.Key, tag.Spec.Value)
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
