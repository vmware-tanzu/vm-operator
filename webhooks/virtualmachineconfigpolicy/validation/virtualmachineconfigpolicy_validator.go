// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/http"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-validate-vim-vmware-com-v1alpha1-virtualmachineconfigpolicy,mutating=false,failurePolicy=fail,groups=vim.vmware.com,resources=virtualmachineconfigpolicies,versions=v1alpha1,name=default.validating.virtualmachineconfigpolicy.v1alpha1.vim.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vim.vmware.com,resources=virtualmachineconfigpolicies,verbs=get;list

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewValidatingWebhook(ctx, mgr, webHookName, NewValidator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create VirtualMachineConfigPolicy validation webhook: %w", err)
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
	return vimv1.GroupVersion.WithKind(reflect.TypeFor[vimv1.VirtualMachineConfigPolicy]().Name())
}

func (v validator) ValidateCreate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	policy, err := v.configPolicyFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	fieldErrs := v.validateSpec(ctx, policy)

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
	policy, err := v.configPolicyFromUnstructured(ctx.Obj)
	if err != nil {
		return webhook.Errored(http.StatusBadRequest, err)
	}

	// Re-run the same rules validated on create: spec.zone is not immutable
	// (a policy may be repointed at a different zone), so an update must
	// validate the new zone reference exists just as create does.
	fieldErrs := v.validateSpec(ctx, policy)

	validationErrs := make([]string, 0, len(fieldErrs))
	for _, fieldErr := range fieldErrs {
		validationErrs = append(validationErrs, fieldErr.Error())
	}

	return common.BuildValidationResponse(ctx, nil, validationErrs, nil)
}

// validateSpec returns an error if spec.zone does not reference an existing
// Zone in the policy's namespace. A non-empty extraConfig allowed/denied key
// is a simple structural rule enforced by the CRD's OpenAPI schema
// (+kubebuilder:validation:MinLength=1 on
// VirtualMachineConfigPolicyExtraConfigKey.Key) rather than here, per the
// constitution's preference for CEL/OpenAPI validation over Go code for
// rules that do not need cross-object or vSphere data.
func (v validator) validateSpec(
	ctx *pkgctx.WebhookRequestContext,
	policy *vimv1.VirtualMachineConfigPolicy) field.ErrorList {
	return v.validateZone(ctx, policy)
}

// validateZone returns an error if spec.zone does not reference an existing
// Zone in the policy's namespace.
//
// This is skipped for the VM Operator service account: the Zone controller
// itself creates/patches a policy in the same reconcile that just created
// its Zone, using the same cache-backed client this webhook does, so a
// cache lag would otherwise cause a spurious rejection of VM Operator's own
// write. A user-driven create/update still goes through this check.
func (v validator) validateZone(
	ctx *pkgctx.WebhookRequestContext,
	policy *vimv1.VirtualMachineConfigPolicy) field.ErrorList {
	if ctx.IsVMOperatorAccount {
		return nil
	}

	f := field.NewPath("spec", "zone")

	_, err := topology.GetZone(ctx, v.client, policy.Spec.Zone, policy.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return field.ErrorList{
				field.NotFound(f, policy.Spec.Zone),
			}
		}

		return field.ErrorList{
			field.InternalError(f, err),
		}
	}

	return nil
}

// configPolicyFromUnstructured returns the VirtualMachineConfigPolicy from
// the unstructured object.
func (v validator) configPolicyFromUnstructured(
	obj runtime.Unstructured) (*vimv1.VirtualMachineConfigPolicy, error) {
	policy := &vimv1.VirtualMachineConfigPolicy{}

	err := v.converter.FromUnstructured(obj.UnstructuredContent(), policy)
	if err != nil {
		return nil, err
	}

	return policy, nil
}
