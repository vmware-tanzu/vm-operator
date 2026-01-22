// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"math/rand"
	"strings"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

func getOpID(ctx context.Context, obj client.Object, operation string) (string, string) {
	opID := strings.Join([]string{"vmoperator", obj.GetName(), operation}, "-")

	if recID := controller.ReconcileIDFromContext(ctx); len(recID) >= 8 {
		id := string(recID[:8])
		return opID + "-" + id, ""
	}

	// Generate our own internal ID.
	const charset = "0123456789abcdef"
	buf := make([]byte, 8)
	for i := range buf {
		idx := rand.Intn(len(charset)) //nolint:gosec
		buf[i] = charset[idx]
	}
	id := string(buf)

	return opID + "-" + id, id
}

// WithVCOpID sets the vimtypes.ID in the context so that it is passed along to VC
// to correlate VMOP and VC logs.
func WithVCOpID(ctx context.Context, obj client.Object, operation string) context.Context {
	vcOpID, intRecID := getOpID(ctx, obj, operation)
	if intRecID != "" {
		// If the context did not have a controller-runtime reconcile ID, include the one
		// that we generated in the context's logger. The underlying reconcile ID context
		// value type is not exported by controller-runtime so we cannot set that specific
		// reconcile type.
		logger := pkglog.FromContextOrDefault(ctx).WithValues("recID", intRecID)
		ctx = logr.NewContext(ctx, logger)
	}
	return context.WithValue(ctx, vimtypes.ID{}, vcOpID)
}
