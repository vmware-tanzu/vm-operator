// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
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

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	// TODO: Implement validation for create

	validationErrs := make([]string, 0)

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

func (v validator) ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response {
	return admission.Allowed("")
}

func (v validator) ValidateUpdate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	// TODO: Implement validation for update

	validationErrs := make([]string, 0)

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}
