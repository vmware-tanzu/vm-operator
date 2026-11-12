// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// garbageCollectorUsername is the identity of the Kubernetes garbage
// collector. It prunes a dangling owner reference from a Tag with an UPDATE
// when an owning VM is deleted, and the Tag would otherwise keep a stale
// owner.
const garbageCollectorUsername = "system:serviceaccount:kube-system:generic-garbage-collector"

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

// ValidateCreate admits only privileged requesters, and requires a
// consistent concrete identity and derived resource name.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validatePrivileged(ctx, admissionv1.Create)...)
	fieldErrs = append(fieldErrs, validateIdentity(tag)...)

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateUpdate admits only privileged requesters, or the garbage collector
// pruning owner references. It validates the complete identity. Immutability
// of spec is enforced by the CEL transition rule on the CRD.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	tag, err := v.tagFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validatePrivileged(ctx, admissionv1.Update)...)
	fieldErrs = append(fieldErrs, validateIdentity(tag)...)

	return common.BuildValidationResponse(ctx, nil, fieldErrsToStrings(fieldErrs), nil)
}

// ValidateDelete allows deletion. DELETE is not registered with this webhook.
func (v validator) ValidateDelete(_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// validatePrivileged permits only a privileged account to create or update a
// Tag. On UPDATE the garbage collector is also admitted so it can prune a
// dangling owner reference. A DevOps user is never admitted: the Tag API is
// internal bookkeeping and is not exposed to namespace users.
func (v validator) validatePrivileged(
	ctx *pkgctx.WebhookRequestContext,
	op admissionv1.Operation) field.ErrorList {

	if ctx.IsPrivilegedAccount {
		return nil
	}

	if op == admissionv1.Update &&
		ctx.UserInfo.Username == garbageCollectorUsername {

		return nil
	}

	return field.ErrorList{
		field.Forbidden(field.NewPath("metadata"),
			fmt.Sprintf("only privileged users may %s a Tag", op)),
	}
}

func validateIdentity(tag *vspherepolv1.Tag) field.ErrorList {
	if err := virtualmachine.ValidateTagIdentity(tag); err != nil {
		return field.ErrorList{field.Invalid(field.NewPath("spec"), tag.Spec, err.Error())}
	}
	return nil
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
