// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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

	// hardwareVersionPattern is the required format for spec.hardwareVersion.
	hardwareVersionPattern = `^vmx-\d+$`
)

var hardwareVersionRegexp = regexp.MustCompile(hardwareVersionPattern)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vim-vmware-com-v1alpha1-virtualmachineconfigoptions,mutating=false,failurePolicy=fail,groups=vim.vmware.com,resources=virtualmachineconfigoptions,versions=v1alpha1,name=default.validating.virtualmachineconfigoptions.v1alpha1.vim.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions,verbs=get;list
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigoptions/status,verbs=get

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
func NewValidator(_ ctrlclient.Client) builder.Validator {
	return validator{
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type validator struct {
	converter runtime.UnstructuredConverter
}

func (v validator) For() schema.GroupVersionKind {
	return vimv1.GroupVersion.WithKind(reflect.TypeFor[vimv1.VirtualMachineConfigOptions]().Name())
}

// ValidateCreate validates a new VirtualMachineConfigOptions.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	obj, err := v.vmConfigOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := v.validateHardwareVersionFormat(obj.Spec.HardwareVersion)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// ValidateDelete allows all delete requests.
func (v validator) ValidateDelete(_ *pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate validates updates to a VirtualMachineConfigOptions, enforcing
// that spec.hardwareVersion is immutable.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	obj, err := v.vmConfigOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldObj, err := v.vmConfigOptionsFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	hwVersionPath := field.NewPath("spec", "hardwareVersion")
	fieldErrs := validation.ValidateImmutableField(obj.Spec.HardwareVersion, oldObj.Spec.HardwareVersion, hwVersionPath)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// validateHardwareVersionFormat returns an error if the hardware version does
// not match the required pattern (e.g. "vmx-21").
func (v validator) validateHardwareVersionFormat(hardwareVersion string) field.ErrorList {
	hwVersionPath := field.NewPath("spec", "hardwareVersion")

	if hardwareVersion == "" {
		return field.ErrorList{field.Required(hwVersionPath, "hardwareVersion must be provided")}
	}

	if !hardwareVersionRegexp.MatchString(hardwareVersion) {
		return field.ErrorList{field.Invalid(
			hwVersionPath,
			hardwareVersion,
			fmt.Sprintf("hardwareVersion must match %s", hardwareVersionPattern),
		)}
	}

	return nil
}

// vmConfigOptionsFromUnstructured returns the VirtualMachineConfigOptions from
// the unstructured object.
func (v validator) vmConfigOptionsFromUnstructured(obj runtime.Unstructured) (*vimv1.VirtualMachineConfigOptions, error) {
	vmco := &vimv1.VirtualMachineConfigOptions{}

	err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmco)
	if err != nil {
		return nil, err
	}

	return vmco, nil
}
