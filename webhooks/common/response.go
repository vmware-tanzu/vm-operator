// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"net/http"
	"strings"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// BuildValidationResponse creates the response from one or more validation errors and any
// errors returned attempting to validate the ingress data.
func BuildValidationResponse(
	ctx *context.WebhookRequestContext,
	validationErrs []string,
	err error,
	additionalValidationErrors ...string) (response admission.Response) {

	// Log the response on the way out.
	defer func() {
		if response.Allowed {
			ctx.Logger.V(4).Info("validation allowed")
		} else {
			var keysAndValues []interface{}
			if result := response.Result; result != nil {
				if reason := string(result.Reason); reason != "" {
					keysAndValues = append(keysAndValues, "reason", reason)
				}
				if result.Message != "" {
					keysAndValues = append(keysAndValues, "message", result.Message)
				}
				if result.Code > 0 {
					keysAndValues = append(keysAndValues, "code", result.Code)
				}
			}
			ctx.Logger.Info("validation denied", keysAndValues...)
		}
	}()

	// Check for well-defined errors.
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return admission.Errored(http.StatusNotFound, err)
		case apierrors.IsGone(err) || apierrors.IsResourceExpired(err):
			return admission.Errored(http.StatusGone, err)
		case apierrors.IsServiceUnavailable(err):
			return admission.Errored(http.StatusServiceUnavailable, err)
		case apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err):
			return admission.Errored(http.StatusGatewayTimeout, err)
		default:
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	// Append any of the additional validation errors to the primary list.
	validationErrs = append(validationErrs, additionalValidationErrors...)

	// The existence of validation errors means the request is denied.
	if len(validationErrs) > 0 {
		reason := strings.Join(validationErrs, ", ")
		return admission.Response{
			AdmissionResponse: admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Reason: metav1.StatusReason(reason),
					Code:   http.StatusUnprocessableEntity,
				},
			},
		}
	}

	return admission.Allowed("")
}
