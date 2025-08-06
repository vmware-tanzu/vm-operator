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
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha4-virtualmachinegrouppublishrequest,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,versions=v1alpha4,name=default.validating.virtualmachinegrouppublishrequest.v1alpha4.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegrouppublishrequests,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinegroups,verbs=get
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get

const (
	webHookName                = "default"
	mutatingWebhookDownWarning = "The mutating webhook should have filled it if empty."
)

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

	fieldErrs := v.validateCreateSpec(ctx, groupPubReq)
	errStrings := common.ConvertFieldErrorsToStrings(fieldErrs)
	return common.BuildValidationResponse(ctx, nil, errStrings, nil)
}

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
	fieldErrs = append(fieldErrs, v.validateSpecVMs(specPath, groupPubReq))

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

func (v validator) validateUpdateSpec(groupPubReq, oldGroupPubReq *vmopv1.VirtualMachineGroupPublishRequest) field.ErrorList {
	var fieldErrs field.ErrorList
	specPath := field.NewPath("spec")

	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.Source, oldGroupPubReq.Spec.Source, specPath.Child("source"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.Target, oldGroupPubReq.Spec.Target, specPath.Child("target"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(groupPubReq.Spec.VirtualMachines, oldGroupPubReq.Spec.VirtualMachines, specPath.Child("virtualMachines"))...)

	return fieldErrs
}

// validateSpecVMs ensures spec.VirtualMachines is defined either via the user or the mutation webhook.
// It does not check the existence of those VMs since it would require multiple calls to the API server.
// Rather, each VM publish request's status is reflected on VM group publish request's status.
func (validator) validateSpecVMs(specPath *field.Path, groupPubReq *vmopv1.VirtualMachineGroupPublishRequest) *field.Error {
	if len(groupPubReq.Spec.VirtualMachines) == 0 {
		return field.Required(specPath.Child("virtualMachines"), mutatingWebhookDownWarning)
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

func validateResource[T ctrlclient.Object](ctx context.Context, c ctrlclient.Client, path *field.Path, obj T) *field.Error {
	if obj.GetName() == "" {
		return field.Required(path, mutatingWebhookDownWarning)
	}

	if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return field.NotFound(path, obj.GetName())
		}
		return field.InternalError(path, err)
	}

	return nil
}
