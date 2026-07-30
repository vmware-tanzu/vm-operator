// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

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
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const webHookName = "default"

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vim-vmware-com-v1alpha1-virtualmachineguestoptions,mutating=false,failurePolicy=fail,groups=vim.vmware.com,resources=virtualmachineguestoptions,versions=v1alpha1,name=default.validating.virtualmachineguestoptions.v1alpha1.vim.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineguestoptions,verbs=get;list
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineguestoptions/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineGuestOptions validation webhook: %w", err)
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
	return vimv1.GroupVersion.WithKind(reflect.TypeFor[vimv1.VirtualMachineGuestOptions]().Name())
}

// ValidateCreate validates a new VirtualMachineGuestOptions.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	obj, err := v.vmGuestOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := v.validateID(string(obj.Spec.ID))
	fieldErrs = append(fieldErrs, v.validateNameMatchesID(obj.Name, string(obj.Spec.ID))...)

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

// ValidateUpdate re-runs the base validations so a VirtualMachineGuestOptions
// that predates this webhook is still caught if it is otherwise invalid.
// spec.id immutability is enforced by the CRD's CEL transition rule.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	obj, err := v.vmGuestOptionsFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := v.validateID(string(obj.Spec.ID))
	fieldErrs = append(fieldErrs, v.validateNameMatchesID(obj.Name, string(obj.Spec.ID))...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// validateID returns an error if spec.id is empty.
func (v validator) validateID(id string) field.ErrorList {
	if id == "" {
		return field.ErrorList{field.Required(field.NewPath("spec", "id"), "id must be provided")}
	}

	return nil
}

// validateNameMatchesID returns an error if metadata.name does not equal the
// DNS-safe transform of spec.id.
func (v validator) validateNameMatchesID(name, id string) field.ErrorList {
	if id == "" {
		return nil
	}

	if want := pkgutil.VimGuestOptionsName(id); name != want {
		return field.ErrorList{field.Invalid(
			field.NewPath("metadata", "name"),
			name,
			fmt.Sprintf("metadata.name must equal the DNS-safe transform of spec.id (%q)", want),
		)}
	}

	return nil
}

// vmGuestOptionsFromUnstructured returns the VirtualMachineGuestOptions from
// the unstructured object.
func (v validator) vmGuestOptionsFromUnstructured(obj runtime.Unstructured) (*vimv1.VirtualMachineGuestOptions, error) {
	vmgo := &vimv1.VirtualMachineGuestOptions{}

	err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmgo)
	if err != nil {
		return nil, err
	}

	return vmgo, nil
}
