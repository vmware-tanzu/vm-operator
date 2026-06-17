// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"regexp"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vim-vmware-com-v1alpha1-virtualmachineconfigoptions,mutating=false,failurePolicy=fail,groups=vim.vmware.com,resources=virtualmachineconfigoptions,versions=v1alpha1,name=default.validating.virtualmachineconfigoptions.v1alpha1.vim.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineConfigOptions validation webhook: %w", err)
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
	return schema.GroupVersionKind{
		Group:   "vim.vmware.com",
		Version: "v1alpha1",
		Kind:    "VirtualMachineConfigOptions",
	}
}

// ValidateCreate validates that the VirtualMachineConfigOptions create request is valid.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	obj, err := v.vmconfigOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	hardwareVersionField := field.NewPath("spec", "hardwareVersion")

	if obj.Spec.HardwareVersion == "" {
		fieldErrs = append(fieldErrs, field.Required(hardwareVersionField, "hardwareVersion must be provided"))
	} else {
		re := regexp.MustCompile(`^vmx-\d+$`)
		if !re.MatchString(obj.Spec.HardwareVersion) {
			fieldErrs = append(fieldErrs, field.Invalid(
				hardwareVersionField,
				obj.Spec.HardwareVersion,
				"hardwareVersion must match pattern ^vmx-\\d+$"))
		}
	}

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// ValidateDelete allows all delete requests for VirtualMachineConfigOptions.
func (v validator) ValidateDelete(_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate validates that the VirtualMachineConfigOptions update is valid.
// The spec.hardwareVersion field is immutable after creation.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	newObj, err := v.vmconfigOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldObj, err := v.vmconfigOptionsFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs,
		validation.ValidateImmutableField(
			newObj.Spec.HardwareVersion,
			oldObj.Spec.HardwareVersion,
			field.NewPath("spec", "hardwareVersion"))...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// vmconfigOptionsFromUnstructured returns the VirtualMachineConfigOptions from
// the unstructured object.
func (v validator) vmconfigOptionsFromUnstructured(obj runtime.Unstructured) (*vimv1.VirtualMachineConfigOptions, error) {
	vmConfigOptions := &vimv1.VirtualMachineConfigOptions{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmConfigOptions); err != nil {
		return nil, err
	}
	return vmConfigOptions, nil
}
