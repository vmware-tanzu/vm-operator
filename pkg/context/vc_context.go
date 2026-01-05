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

type intRecIDContextKey uint8

const intRecIDContextValue intRecIDContextKey = 0

func getReconcileID(ctx context.Context) (context.Context, string) {
	if recID := controller.ReconcileIDFromContext(ctx); len(recID) >= 8 {
		id := string(recID[:8])
		return ctx, id
	}

	if internalRecID, ok := InternalReconcileIDFromContext(ctx); ok {
		return ctx, internalRecID
	}

	// Generate our own internal reconcile ID. The underlying controller-runtime
	// reconcile ID context value type is not exported so we cannot set that so
	// instead add our internal type to the context.
	const charset = "0123456789abcdef"
	buf := make([]byte, 8)
	for i := range buf {
		idx := rand.Intn(len(charset)) //nolint:gosec
		buf[i] = charset[idx]
	}
	id := string(buf)

	// Add it to the logger so it is present in our logs.
	ctx = logr.NewContext(ctx,
		pkglog.FromContextOrDefault(ctx).WithValues("intReconcileID", id))

	return context.WithValue(ctx, intRecIDContextValue, id), id
}

// InternalReconcileIDFromContext returns the internal reconcile ID value from
// the context.
func InternalReconcileIDFromContext(ctx context.Context) (string, bool) {
	internalRecID := ctx.Value(intRecIDContextValue)
	id, ok := internalRecID.(string)
	return id, ok
}

// WithVCOpID sets the vimtypes.ID in the context so that it is passed along to VC
// to correlate VMOP and VC logs.
func WithVCOpID(ctx context.Context, obj client.Object, operation string) context.Context {
	ctx, recID := getReconcileID(ctx)

	elems := make([]string, 0, 4)
	elems = append(elems, "vmoperator")
	if obj != nil {
		elems = append(elems, obj.GetName())
	}
	elems = append(elems, operation, recID)
	vcOpID := strings.Join(elems, "-")

	return context.WithValue(ctx, vimtypes.ID{}, vcOpID)
}
