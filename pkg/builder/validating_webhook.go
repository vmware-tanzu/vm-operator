// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// ValidatingWebhook is an admissions webhook that validates resources.
type ValidatingWebhook struct {
	admission.Webhook

	// Name is the name of the webhook.
	Name string

	// Path is the path of the webhook.
	Path string
}

// Validator is used to create a new admissions webhook for validating requests.
type Validator interface {
	// For returns the GroupVersionKind for which this webhook validates requests.
	For() schema.GroupVersionKind

	// ValidateCreate returns nil if the request is valid.
	ValidateCreate(*pkgctx.WebhookRequestContext) admission.Response

	// ValidateDelete returns nil if the request is valid.
	ValidateDelete(*pkgctx.WebhookRequestContext) admission.Response

	// ValidateUpdate returns nil if the request is valid.
	ValidateUpdate(*pkgctx.WebhookRequestContext) admission.Response
}

type ValidatorFunc func(client client.Client) Validator

// NewValidatingWebhook returns a new admissions webhook for validating requests.
func NewValidatingWebhook(
	ctx *pkgctx.ControllerManagerContext,
	mgr ctrlmgr.Manager,
	webhookName string,
	validator Validator) (*ValidatingWebhook, error) {

	if webhookName == "" {
		return nil, errors.New("webhookName arg is empty")
	}
	if validator == nil {
		return nil, errors.New("validator arg is nil")
	}

	var (
		webhookNameShort = generateValidateName(webhookName, validator.For())
		webhookPath      = "/" + webhookNameShort
		webhookNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, webhookNameShort)
	)

	// Build the webhookContext.
	webhookContext := &pkgctx.WebhookContext{
		Context:                         ctx,
		Name:                            webhookNameShort,
		Namespace:                       ctx.Namespace,
		ServiceAccountName:              ctx.ServiceAccountName,
		Recorder:                        record.New(mgr.GetEventRecorderFor(webhookNameLong)),
		Logger:                          ctx.Logger.WithName(webhookNameShort),
		EnableWebhookClientVerification: ctx.EnableWebhookClientVerification,
	}

	// Initialize the webhook's decoder.
	decoder := admission.NewDecoder(mgr.GetScheme())

	// Create the webhook.
	return &ValidatingWebhook{
		Name: webhookNameShort,
		Path: webhookPath,
		Webhook: webhook.Admission{
			Handler: &validatingWebhookHandler{
				WebhookContext: webhookContext,
				Decoder:        decoder,
				Validator:      validator,
			},
			WithContextFunc: contextWithClientCert,
		},
	}, nil
}

var _ admission.Handler = &validatingWebhookHandler{}

type validatingWebhookHandler struct {
	*pkgctx.WebhookContext
	admission.Decoder
	Validator
}

func (h *validatingWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if h.EnableWebhookClientVerification {
		ctx = pkgcfg.JoinContext(ctx, h.WebhookContext)
		if err := VerifyWebhookRequest(ctx); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	}

	if h.Validator == nil {
		panic("validator should never be nil")
	}

	var (
		obj, oldObj *unstructured.Unstructured
	)

	// Get the Object for Create and Update operations.
	//
	// For Delete operations the OldObject contains the Object being
	// deleted. Please see https://github.com/kubernetes/kubernetes/pull/76346
	// for more information.

	if req.Operation == admissionv1.Create || req.Operation == admissionv1.Update {
		obj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.Object, obj); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	} else if req.Operation == admissionv1.Delete {
		obj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.OldObject, obj); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	}

	// Get the old Object for Update operations.
	if req.Operation == admissionv1.Update {
		oldObj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.OldObject, oldObj); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	}

	if obj == nil {
		return admission.Allowed(string(req.Operation))
	}

	// Create the webhook request pkgctx.
	webhookRequestContext := &pkgctx.WebhookRequestContext{
		WebhookContext:      h.WebhookContext,
		Op:                  req.Operation,
		Obj:                 obj,
		OldObj:              oldObj,
		UserInfo:            req.UserInfo,
		IsPrivilegedAccount: IsPrivilegedAccount(h.WebhookContext, req.UserInfo),
		Logger:              h.WebhookContext.Logger.WithName(obj.GetNamespace()).WithName(obj.GetName()),
	}

	return h.HandleValidate(req, webhookRequestContext)
}

func (h *validatingWebhookHandler) HandleValidate(req admission.Request, ctx *pkgctx.WebhookRequestContext) admission.Response {

	if _, ok := ctx.Obj.GetAnnotations()[pkgconst.SkipValidationAnnotationKey]; ok {
		if !ctx.IsPrivilegedAccount {
			// Only privileged users may set the skip validation annotation.
			return webhook.Denied(SkipValidationDenied)
		}

		// The object has the skip validation annotation, so allow the object to
		// bypass validation.
		return webhook.Allowed(SkipValidationAllowed)
	}

	switch req.Operation {
	case admissionv1.Create:
		return h.ValidateCreate(ctx)
	case admissionv1.Update:
		// Allow the Patch/Update requests if the object is under deletion. This eliminates queueing objects
		// due to reconcile failures.
		if !ctx.Obj.GetDeletionTimestamp().IsZero() {
			return admission.Allowed(AdmitMesgUpdateOnDeleting)
		}
		return h.ValidateUpdate(ctx)
	case admissionv1.Delete:
		return h.ValidateDelete(ctx)
	default:
		return admission.Allowed(string(req.Operation))
	}
}

func generateValidateName(webhookName string, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s-validate-", webhookName) +
		strings.ReplaceAll(gvk.Group, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}
