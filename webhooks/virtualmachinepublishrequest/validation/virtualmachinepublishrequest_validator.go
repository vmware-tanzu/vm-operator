// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vmoperator-vmware-com-v1alpha4-virtualmachinepublishrequest,mutating=false,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,versions=v1alpha4,name=default.validating.virtualmachinepublishrequest.v1alpha4.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,verbs=get;list
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests/status,verbs=get
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=contentlibraries,verbs=get;list;

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachinePublishRequest validation webhook: %w", err)
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
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachinePublishRequest{}).Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	vmpub, err := v.vmPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	fieldErrs = append(fieldErrs, v.validateSource(ctx, vmpub)...)
	fieldErrs = append(fieldErrs, v.validateTargetLocation(ctx, vmpub)...)

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
	vmpub, err := v.vmPublishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	oldVMpub, err := v.vmPublishRequestFromUnstructured(ctx.OldObj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	var fieldErrs field.ErrorList

	// Check if an immutable field has been modified.
	fieldErrs = append(fieldErrs, v.validateImmutableFields(vmpub, oldVMpub)...)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) validateSource(ctx *pkgctx.WebhookRequestContext, vmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList

	sourcePath := field.NewPath("spec").Child("source")
	if apiVersion := vmpub.Spec.Source.APIVersion; apiVersion != "" {
		var (
			v1a1GV = vmopv1a1.GroupVersion.String()
			v1a2GV = vmopv1a2.GroupVersion.String()
			v1a3GV = vmopv1a3.GroupVersion.String()
			vmopv1 = vmopv1.GroupVersion.String()
		)

		switch apiVersion {
		case v1a1GV, v1a2GV, v1a3GV, vmopv1:
		default:
			allErrs = append(allErrs, field.NotSupported(sourcePath.Child("apiVersion"),
				apiVersion, []string{v1a1GV, v1a2GV, v1a3GV, vmopv1, ""}))
		}
	}

	if kind := vmpub.Spec.Source.Kind; kind != reflect.TypeOf(vmopv1.VirtualMachine{}).Name() && kind != "" {
		allErrs = append(allErrs, field.NotSupported(sourcePath.Child("kind"),
			vmpub.Spec.Source.Kind, []string{reflect.TypeOf(vmopv1.VirtualMachine{}).Name(), ""}))
	}

	return allErrs
}

func (v validator) validateTargetLocation(ctx *pkgctx.WebhookRequestContext, vmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList

	targetLocationPath := field.NewPath("spec").Child("target").
		Child("location")
	targetLocationName := vmpub.Spec.Target.Location.Name
	targetLocationNamePath := targetLocationPath.Child("name")
	if targetLocationName == "" {
		allErrs = append(allErrs, field.Required(targetLocationNamePath, ""))
	}

	if vmpub.Spec.Target.Location.APIVersion != imgregv1a1.GroupVersion.String() {
		allErrs = append(allErrs, field.NotSupported(targetLocationPath.Child("apiVersion"),
			vmpub.Spec.Target.Location.APIVersion, []string{imgregv1a1.GroupVersion.String(), ""}))
	}

	if vmpub.Spec.Target.Location.Kind != reflect.TypeOf(imgregv1a1.ContentLibrary{}).Name() {
		allErrs = append(allErrs, field.NotSupported(targetLocationPath.Child("kind"),
			vmpub.Spec.Target.Location.Kind, []string{reflect.TypeOf(imgregv1a1.ContentLibrary{}).Name(), ""}))
	}

	return allErrs
}

func (v validator) validateImmutableFields(vmpub, oldvmpub *vmopv1.VirtualMachinePublishRequest) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// all updates to source and target are not allowed.
	// Otherwise, we may end up in a situation where multiple OVFs are published for a single VMPub.
	allErrs = append(allErrs, validation.ValidateImmutableField(vmpub.Spec.Source, oldvmpub.Spec.Source, specPath.Child("source"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(vmpub.Spec.Target, oldvmpub.Spec.Target, specPath.Child("target"))...)

	return allErrs
}

// vmPublishRequestFromUnstructured returns the VirtualMachineService from the unstructured object.
func (v validator) vmPublishRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachinePublishRequest, error) {
	vmPubReq := &vmopv1.VirtualMachinePublishRequest{}
	if err := v.converter.FromUnstructured(obj.UnstructuredContent(), vmPubReq); err != nil {
		return nil, err
	}
	return vmPubReq, nil
}
