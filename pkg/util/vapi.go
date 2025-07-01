// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/http"

	"github.com/vmware/govmomi/vapi/rest"
)

// vapiActivationIDHeader is the HTTP header to pass to a VAPI API in order
// to influence the activationID of the vim.Task spawned by the VAPI API.
const vapiActivationIDHeader = "vapi-ctx-actid"

// WithVAPIActivationID adds the specified id to the context to be used as the
// VAPI REST client's activation ID -- the value assigned to the activationId
// field to the vim.Task the VAPI API may spawn.
func WithVAPIActivationID(
	ctx context.Context,
	client *rest.Client,
	id string) context.Context {

	return client.WithHeader(
		ctx,
		http.Header{vapiActivationIDHeader: []string{id}},
	)
}
