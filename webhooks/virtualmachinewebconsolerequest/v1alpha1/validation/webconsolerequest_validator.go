// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	webconsolerequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha1-webconsolerequest,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=webconsolerequests,versions=v1alpha1,name=default.validating.webconsolerequest.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=webconsolerequests,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=webconsolerequests/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return errors.Wrapf(err, "failed to create webconsolerequest validation webhook")
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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.WebConsoleRequest{}).Name())
}

func (v validator) ValidateCreate(ctx *context.WebhookRequestContext) admission.Response {

	wcr, err := v.webConsoleRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList
	fieldErrs = append(fieldErrs, v.validateMetadata(ctx, wcr)...)
	fieldErrs = append(fieldErrs, v.validateSpec(ctx, wcr)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) ValidateDelete(*context.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *context.WebhookRequestContext) admission.Response {
	wcr, err := v.webConsoleRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldwcr, err := v.webConsoleRequestFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList
	fieldErrs = append(fieldErrs, v.validateImmutableFields(wcr, oldwcr)...)
	fieldErrs = append(fieldErrs, v.validateUUIDLabel(wcr, oldwcr)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}
	return common.BuildValidationResponse(ctx, validationErrs, nil)
}

func (v validator) validateMetadata(ctx *context.WebhookRequestContext, wcr *vmopv1.WebConsoleRequest) field.ErrorList {
	var fieldErrs field.ErrorList
	return fieldErrs
}

func (v validator) validateSpec(ctx *context.WebhookRequestContext, wcr *vmopv1.WebConsoleRequest) field.ErrorList {
	var fieldErrs field.ErrorList
	specPath := field.NewPath("spec")

	fieldErrs = append(fieldErrs, v.validateVirtualMachineName(specPath.Child("virtualMachineName"), wcr)...)
	fieldErrs = append(fieldErrs, v.validatePublicKey(specPath.Child("publicKey"), wcr.Spec.PublicKey)...)

	return fieldErrs
}

func (v validator) validateVirtualMachineName(path *field.Path, wcr *vmopv1.WebConsoleRequest) field.ErrorList {
	var allErrs field.ErrorList

	if wcr.Spec.VirtualMachineName == "" {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}

	// Not checking existence of wcr.Spec.VirtualMachineName because the webhooks are meant for internal consistency
	// checking. Also, the kubectl-vsphere plugin validates the existence of the VM at that level as well.

	return allErrs
}

func (v validator) validatePublicKey(path *field.Path, publicKey string) field.ErrorList {
	var allErrs field.ErrorList

	if publicKey == "" {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}

	block, _ := pem.Decode([]byte(publicKey))
	if block == nil || block.Type != "PUBLIC KEY" {
		allErrs = append(allErrs, field.Invalid(path, "", "invalid public key format"))
		return allErrs
	}
	_, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(path, "", "invalid public key"))
	}

	return allErrs
}

func (v validator) validateImmutableFields(wcr, oldwcr *vmopv1.WebConsoleRequest) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	allErrs = append(allErrs, validation.ValidateImmutableField(wcr.Spec.VirtualMachineName, oldwcr.Spec.VirtualMachineName, specPath.Child("virtualMachineName"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(wcr.Spec.PublicKey, oldwcr.Spec.PublicKey, specPath.Child("publicKey"))...)

	return allErrs
}

// webConsoleRequestFromUnstructured returns the wcr from the unstructured object.
func (v validator) webConsoleRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.WebConsoleRequest, error) {
	wcr := &vmopv1.WebConsoleRequest{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), wcr); err != nil {
		return nil, err
	}
	return wcr, nil
}

func (v validator) validateUUIDLabel(wcr, oldwcr *vmopv1.WebConsoleRequest) field.ErrorList {
	var allErrs field.ErrorList

	oldUUIDLabelVal := oldwcr.Labels[webconsolerequest.UUIDLabelKey]
	if oldUUIDLabelVal == "" {
		return allErrs
	}

	newUUIDLabelVal := wcr.Labels[webconsolerequest.UUIDLabelKey]
	labelsPath := field.NewPath("metadata", "labels")
	allErrs = append(allErrs, validation.ValidateImmutableField(newUUIDLabelVal, oldUUIDLabelVal, labelsPath.Key(webconsolerequest.UUIDLabelKey))...)

	return allErrs
}
