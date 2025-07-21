// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"

	virtualMachineKind = "VirtualMachine"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha4-virtualmachinesnapshot,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,versions=v1alpha4,name=default.validating.virtualmachinesnapshot.v1alpha4.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinesnapshots/status,verbs=get

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineSnapshot validation webhook: %w", err)
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

func (v validator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSnapshot{}).Name())
}

// ValidateCreate makes sure the VirtualMachineSnapshot create request is valid.
func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmSnapshot, err := v.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	vmRefField := field.NewPath("spec", "vmRef")

	if vmSnapshot.Spec.VMRef != nil {
		if vmSnapshot.Spec.VMRef.Kind != virtualMachineKind {
			fieldErrs = append(fieldErrs, field.Invalid(vmRefField.Child("kind"),
				vmSnapshot.Spec.VMRef.Kind, fmt.Sprintf("must be %q", virtualMachineKind)))
		}

		if vmSnapshot.Spec.VMRef.APIVersion != "" {
			gv, err := schema.ParseGroupVersion(vmSnapshot.Spec.VMRef.APIVersion)
			if err != nil {
				fieldErrs = append(fieldErrs, field.Invalid(vmRefField.Child("apiVersion"),
					vmSnapshot.Spec.VMRef.APIVersion, "must be valid group version"))
			} else if gv.Group != vmopv1.GroupName {
				fieldErrs = append(fieldErrs, field.Invalid(vmRefField.Child("apiVersion"),
					vmSnapshot.Spec.VMRef.APIVersion, fmt.Sprintf("group must be %q", vmopv1.GroupName)))
			}
		}

		if vmSnapshot.Spec.VMRef.Name == "" {
			fieldErrs = append(fieldErrs, field.Required(vmRefField.Child("name"), "name must be provided"))
		}
	} else {
		fieldErrs = append(fieldErrs, field.Required(vmRefField, "vmRef must be provided"))
	}

	validationErrs := make([]string, 0)
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

// ValidateUpdate validates if the VirtualMachineSnapshot update is valid
// - Fields other than Description are not allowed to be changed.
func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmSnapshot, err := v.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMSnapshot, err := v.vmSnapshotFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.Memory, oldVMSnapshot.Spec.Memory, field.NewPath("spec", "memory"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.Quiesce, oldVMSnapshot.Spec.Quiesce, field.NewPath("spec", "quiesce"))...)
	fieldErrs = append(fieldErrs, validation.ValidateImmutableField(vmSnapshot.Spec.VMRef, oldVMSnapshot.Spec.VMRef, field.NewPath("spec", "vmRef"))...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// vmSnapshotFromUnstructured returns the VirtualMachineSnapshot from the unstructured object.
func (v validator) vmSnapshotFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSnapshot, error) {
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmSnapshot); err != nil {
		return nil, err
	}
	return vmSnapshot, nil
}
