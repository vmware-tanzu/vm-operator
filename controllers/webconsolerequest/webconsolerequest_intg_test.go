// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolerequest_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/webconsolerequest"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking WebConsoleRequest controller tests", webConsoleRequestReconcile)
}

func webConsoleRequestReconcile() {
	var (
		ctx *builder.IntegrationTestContext
		wcr *v1alpha1.WebConsoleRequest
		vm  *v1alpha1.VirtualMachine

		oldIsUnifiedTKGBYOIFSSEnabled func() bool
	)

	getWebConsoleRequest := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *v1alpha1.WebConsoleRequest {
		wcr := &v1alpha1.WebConsoleRequest{}
		if err := ctx.Client.Get(ctx, objKey, vm); err != nil {
			return nil
		}
		return wcr
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &v1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
		}

		wcr = &v1alpha1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-wcr",
				Namespace: ctx.Namespace,
			},
			Spec: v1alpha1.WebConsoleRequestSpec{
				VirtualMachineName: vm.Name,
				PublicKey:          "",
			},
		}

		oldIsUnifiedTKGBYOIFSSEnabled = lib.IsUnifiedTKGBYOIFSSEnabled
		lib.IsUnifiedTKGBYOIFSSEnabled = func() bool {
			return true
		}

		fakeVMProvider.Lock()
		defer fakeVMProvider.Unlock()
		fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error) {
			return "some-fake-webmksticket", nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil

		fakeVMProvider.Reset()

		lib.IsUnifiedTKGBYOIFSSEnabled = oldIsUnifiedTKGBYOIFSSEnabled
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, wcr)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, wcr)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("resource successfully created", func() {
			wcr := getWebConsoleRequest(ctx, types.NamespacedName{Name: wcr.Name, Namespace: wcr.Namespace})
			Expect(wcr).ToNot(BeNil())
			Expect(wcr.Status.Response).ToNot(BeEmpty())
			Expect(wcr.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
		})
	})
}
