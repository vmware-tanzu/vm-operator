// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha5-virtualmachinegrouppublishrequest,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,versions=v1alpha5,name=default.validating.virtualmachinegrouppublishrequest.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create validation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewValidator returns the package's Validator.
func NewValidator(client ctrlclient.Client) builder.Validator {
	return validator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	client    ctrlclient.Client
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineGroupPublishRequest{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	groupPubReq, err := v.vmGroupPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	return common.BuildValidationResponse(ctx, nil,
		common.ConvertFieldErrorsToStrings(v.validateCreateSpec(ctx, groupPubReq)), nil)
}

// validateCreateSpec ensures spec.source, spec.target, and spec.virtualMachines are set.
// Those are optional fields but will be set by the mutating webhook if they are omitted.
func (v validator) validateCreateSpec(ctx *pkgctx.WebhookRequestContext,
	groupPubReq *vmopv1.VirtualMachineGroupPublishRequest) field.ErrorList {
	var fieldErrs field.ErrorList
	specPath := field.NewPath("spec")

	vmGroupSource := &vmopv1.VirtualMachineGroup{ObjectMeta: metav1.ObjectMeta{Namespace: groupPubReq.Namespace,
		Name: groupPubReq.Spec.Source}}
	clTarget := &imgregv1a1.ContentLibrary{ObjectMeta: metav1.ObjectMeta{Namespace: groupPubReq.Namespace,
		Name: groupPubReq.Spec.Target}}

	fieldErrs = append(fieldErrs, validateResource(ctx, v.client, specPath.Child("source"), vmGroupSource))
	fieldErrs = append(fieldErrs, validateResource(ctx, v.client, specPath.Child("target"), clTarget))
	fieldErrs = append(fieldErrs, v.validateSpecVMs(specPath, groupPubReq, ctx))

	return fieldErrs
}

func (v validator) ValidateDelete(_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	groupPubReq, err := v.vmGroupPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}
	oldGroupPubReq, err := v.vmGroupPublishRequestFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	return common.BuildValidationResponse(ctx, nil,
		common.ConvertFieldErrorsToStrings(v.validateUpdateSpec(groupPubReq, oldGroupPubReq)), nil)
}

// validateUpdateSpec ensures no updates to spce's immutable fields.
func (v validator) validateUpdateSpec(groupPubReq, oldGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest) field.ErrorList {
	var fieldErrs field.ErrorList
	specPath := field.NewPath("spec")

	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.Source, oldGroupPubReq.Spec.Source, specPath.Child("source"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.Target, oldGroupPubReq.Spec.Target, specPath.Child("target"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.VirtualMachines, oldGroupPubReq.Spec.VirtualMachines, specPath.Child("virtualMachines"))...)

	return fieldErrs
}

// validateSpecVMs ensures spec.VirtualMachines is defined either via the user or the mutation webhook
// and the spec.virtualMachines is a subset of all members in the group defined in spec.source.
func (v validator) validateSpecVMs(specPath *field.Path,
	groupPubReq *vmopv1.VirtualMachineGroupPublishRequest,
	ctx *pkgctx.WebhookRequestContext) *field.Error {
	if len(groupPubReq.Spec.VirtualMachines) == 0 {
		return field.Required(specPath.Child("virtualMachines"), "")
	}

	vmSet := sets.New(groupPubReq.Spec.VirtualMachines...)
	if len(groupPubReq.Spec.VirtualMachines) != vmSet.Len() {
		return field.Duplicate(specPath.Child("virtualMachines"), "")
	}

	visitedGroups := &sets.Set[string]{}
	vmGroupSet, err := vmopv1util.RetrieveVMGroupMembers(ctx, v.client,
		ctrlclient.ObjectKey{Namespace: groupPubReq.Namespace, Name: groupPubReq.Spec.Source}, visitedGroups)
	if err != nil {
		return field.InternalError(specPath.Child("virtualMachines"), err)
	}
	if !vmGroupSet.IsSuperset(sets.New(groupPubReq.Spec.VirtualMachines...)) {
		return field.Invalid(specPath.Child("virtualMachines"), "",
			"virtual machines must be a direct or indirect member of the source group")
	}

	return nil
}

// vmGroupPublishRequestFromUnstructured returns the VirtualMachineGroupPublishRequest from the unstructured object.
func (v validator) vmGroupPublishRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineGroupPublishRequest, error) {
	vmGroupPublishRequest := &vmopv1.VirtualMachineGroupPublishRequest{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmGroupPublishRequest); err != nil {
		return nil, err
	}
	return vmGroupPublishRequest, nil
}

// validateResource ensures an object's name is set and exist.
func validateResource[T ctrlclient.Object](ctx context.Context, c ctrlclient.Client, path *field.Path, obj T) *field.Error {
	if obj.GetName() == "" {
		return field.Required(path, "")
	}

	if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return field.NotFound(path, obj.GetName())
		}
		return field.InternalError(path, err)
	}

	return nil
}
