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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// MutatingWebhook is an admissions webhook that mutates VM Operator
// resources.
type MutatingWebhook struct {
	admission.Webhook

	// Name is the name of the webhook.
	Name string

	// Path is the path of the webhook.
	Path string
}

// Mutator is used to create a new admissions webhook for mutating requests.
type Mutator interface {
	// For returns the GroupVersionKind for which this webhook mutates requests.
	For() schema.GroupVersionKind

	// Mutate will try modify invalid value.
	Mutate(*pkgctx.WebhookRequestContext) admission.Response
}

type MutatorFunc func(client.Client) Mutator

// NewMutatingWebhook returns a new admissions webhook for mutating requests.
func NewMutatingWebhook(
	ctx *pkgctx.ControllerManagerContext,
	mgr ctrlmgr.Manager,
	webhookName string,
	mutator Mutator) (*MutatingWebhook, error) {

	if webhookName == "" {
		return nil, errors.New("webhookName arg is empty")
	}
	if mutator == nil {
		return nil, errors.New("mutator arg is nil")
	}

	webhookNameShort := generateMutateName(webhookName, mutator.For())
	webhookPath := "/" + webhookNameShort
	webhookNameLong := fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, webhookNameShort)

	// Build the WebhookContext.
	webhookContext := &pkgctx.WebhookContext{
		Context:            ctx,
		Name:               webhookNameShort,
		Namespace:          ctx.Namespace,
		ServiceAccountName: ctx.ServiceAccountName,
		Recorder:           record.New(mgr.GetEventRecorderFor(webhookNameLong)),
		Logger:             ctx.Logger.WithName(webhookNameShort),
	}

	// Initialize the webhook's decoder.
	decoder := admission.NewDecoder(mgr.GetScheme())

	// Create the webhook.
	return &MutatingWebhook{
		Name: webhookNameShort,
		Path: webhookPath,
		Webhook: webhook.Admission{
			Handler: &mutatingWebhookHandler{
				Decoder:        decoder,
				WebhookContext: webhookContext,
				Mutator:        mutator,
			},
		},
	}, nil
}

var _ admission.Handler = &mutatingWebhookHandler{}

type mutatingWebhookHandler struct {
	*pkgctx.WebhookContext
	admission.Decoder
	Mutator
}

func (h *mutatingWebhookHandler) Handle(_ context.Context, req admission.Request) admission.Response {
	if h.Mutator == nil {
		panic("mutator should never be nil")
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
		oldObj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.OldObject, oldObj); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	}

	if req.Operation == admissionv1.Update && !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed(AdmitMesgUpdateOnDeleting)
	}

	// Get the old Object for Update operations.
	if req.Operation == admissionv1.Update {
		oldObj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.OldObject, oldObj); err != nil {
			return webhook.Errored(http.StatusBadRequest, err)
		}
	}

	var (
		objName      string
		objNamespace string
	)
	if obj != nil {
		objName = obj.GetName()
		objNamespace = obj.GetNamespace()
	} else {
		objName = oldObj.GetName()
		objNamespace = oldObj.GetNamespace()
	}

	webhookRequestContext := &pkgctx.WebhookRequestContext{
		WebhookContext:      h.WebhookContext,
		Op:                  req.Operation,
		Obj:                 obj,
		RawObj:              req.Object.Raw,
		OldObj:              oldObj,
		UserInfo:            req.UserInfo,
		IsPrivilegedAccount: IsPrivilegedAccount(h.WebhookContext, req.UserInfo),
		Logger:              h.WebhookContext.Logger.WithName(objNamespace).WithName(objName),
	}

	return h.Mutate(webhookRequestContext)
}

func generateMutateName(webhookName string, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s-mutate-", webhookName) +
		strings.ReplaceAll(gvk.Group, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}
