// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolerequest_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/webconsolerequest"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking WebConsoleRequest Reconcile", unitTestsReconcile)
}

func unitTestsReconcile() {

	var (
		initObjects []client.Object
		ctx         *builder.UnitTestContextForController

		reconciler *webconsolerequest.Reconciler
		wcrCtx     *vmopContext.WebConsoleRequestContext
		wcr        *v1alpha1.WebConsoleRequest
		vm         *v1alpha1.VirtualMachine

		oldIsPublicCloudBYOIFSSEnabledFunc func() bool
	)

	BeforeEach(func() {
		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vm",
			},
		}

		wcr = &v1alpha1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-wcr",
			},
			Spec: v1alpha1.WebConsoleRequestSpec{
				VirtualMachineName: vm.Name,
				PublicKey:          "",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = webconsolerequest.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)

		wcrCtx = &vmopContext.WebConsoleRequestContext{
			Context:           ctx,
			Logger:            ctx.Logger.WithName(wcr.Name),
			WebConsoleRequest: wcr,
			VM:                vm,
		}

		oldIsPublicCloudBYOIFSSEnabledFunc = lib.IsVMServicePublicCloudBYOIFSSEnabled
		lib.IsVMServicePublicCloudBYOIFSSEnabled = func() bool {
			return true
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVMProvider.Reset()

		lib.IsVMServicePublicCloudBYOIFSSEnabled = oldIsPublicCloudBYOIFSSEnabledFunc
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, wcr, vm)
		})

		JustBeforeEach(func() {
			fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error) {
				return "some-fake-webmksticket", nil
			}
		})

		When("NoOp", func() {
			It("returns success", func() {
				err := reconciler.ReconcileNormal(wcrCtx)
				Expect(err).ToNot(HaveOccurred())

				Expect(wcrCtx.WebConsoleRequest.Status.Response).ToNot(BeEmpty())
				Expect(wcrCtx.WebConsoleRequest.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
			})
		})
	})
}
