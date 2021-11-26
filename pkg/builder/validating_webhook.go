// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	goctx "context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
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
	ValidateCreate(*context.WebhookRequestContext) admission.Response

	// ValidateDelete returns nil if the request is valid.
	ValidateDelete(*context.WebhookRequestContext) admission.Response

	// ValidateUpdate returns nil if the request is valid.
	ValidateUpdate(*context.WebhookRequestContext) admission.Response
}

type ValidatorFunc func(client client.Client) Validator

// NewValidatingWebhook returns a new admissions webhook for validating requests.
func NewValidatingWebhook(
	ctx *context.ControllerManagerContext,
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
	webhookContext := &context.WebhookContext{
		Context:  ctx,
		Name:     webhookNameShort,
		Recorder: record.New(mgr.GetEventRecorderFor(webhookNameLong)),
		Logger:   ctx.Logger.WithName(webhookNameShort),
	}

	// Initialize the webhook's decoder.
	decoder, err := admission.NewDecoder(mgr.GetScheme())
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize decoder")
	}

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
		},
	}, nil
}

var _ admission.Handler = &validatingWebhookHandler{}

type validatingWebhookHandler struct {
	*context.WebhookContext
	*admission.Decoder
	Validator
}

func (h *validatingWebhookHandler) Handle(_ goctx.Context, req admission.Request) admission.Response {
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

	// Create the webhook request context.
	webhookRequestContext := &context.WebhookRequestContext{
		WebhookContext: h.WebhookContext,
		Obj:            obj,
		OldObj:         oldObj,
		Logger:         h.WebhookContext.Logger.WithName(obj.GetNamespace()).WithName(obj.GetName()),
		UserInfo:       &req.UserInfo,
	}

	return h.HandleValidate(req, webhookRequestContext)
}

func (h *validatingWebhookHandler) HandleValidate(req admission.Request, ctx *context.WebhookRequestContext) admission.Response {
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
