// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

var _ = Describe("WithVCOpID", func() {

	It("nil object", func() {
		ctx := pkgctx.WithVCOpID(context.Background(), nil, "nil")
		recID, ok := pkgctx.InternalReconcileIDFromContext(ctx)
		Expect(ok).To(BeTrue())
		Expect(recID).To(Not(BeEmpty()))

		id := ctx.Value(vimtypes.ID{})
		Expect(id).ToNot(BeNil())
		Expect(id.(string)).To(Equal("vmoperator-nil-" + recID))
	})

	It("Generates internal reconcile ID", func() {
		vm := &vmopv1.VirtualMachine{}
		vm.Name = "foo"

		ctx := pkgctx.WithVCOpID(context.Background(), vm, "first")
		recID, ok := pkgctx.InternalReconcileIDFromContext(ctx)
		Expect(ok).To(BeTrue())
		Expect(recID).To(Not(BeEmpty()))

		id := ctx.Value(vimtypes.ID{})
		Expect(id).ToNot(BeNil())
		Expect(id.(string)).To(Equal("vmoperator-foo-first-" + recID))

		// Subsequent calls should preserve internal reconcile ID.
		{
			ctx2 := pkgctx.WithVCOpID(ctx, vm, "second")
			recID2, ok := pkgctx.InternalReconcileIDFromContext(ctx2)
			Expect(ok).To(BeTrue())
			Expect(recID2).To(Equal(recID))

			id := ctx2.Value(vimtypes.ID{})
			Expect(id).ToNot(BeNil())
			Expect(id.(string)).To(Equal("vmoperator-foo-second-" + recID))
		}
	})
})
