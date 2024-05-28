// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha3-virtualmachinereplicaset,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinereplicasets,versions=v1alpha3,name=default.validating.virtualmachinereplicaset.v1alpha3.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineReplicaset validation webhook: %w", err)
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
	return vmopv1.SchemeGroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineReplicaSet{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	rs, err := v.rsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateLabelSelectorLabelMatch(ctx, rs, nil)...)

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
	rs, err := v.rsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList
	fieldErrs = append(fieldErrs, v.validateLabelSelectorLabelMatch(ctx, rs, nil)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateLabelSelectorLabelMatch(
	_ *pkgctx.WebhookRequestContext,
	rs *vmopv1.VirtualMachineReplicaSet,
	_ *vmopv1.VirtualMachineReplicaSet) field.ErrorList {

	var allErrs field.ErrorList

	specPath := field.NewPath("spec")

	selector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("selector"),
				rs.Spec.Selector,
				err.Error(),
			),
		)
	} else if !selector.Matches(labels.Set(rs.Spec.Template.ObjectMeta.Labels)) {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("template", "metadata", "labels"),
				rs.Spec.Template.ObjectMeta.Labels,
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	allErrs = append(allErrs, rs.Spec.Template.ObjectMeta.Validate(specPath.Child("template", "metadata"))...)

	return allErrs
}

// rsFromUnstructured returns the VirtualMachineClass from the unstructured object.
func (v validator) rsFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineReplicaSet, error) {
	rs := &vmopv1.VirtualMachineReplicaSet{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), rs); err != nil {
		return nil, err
	}
	return rs, nil
}
