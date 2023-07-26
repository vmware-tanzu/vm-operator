// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	webconsolerequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
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
		wcr        *vmopv1.WebConsoleRequest
		vm         *vmopv1.VirtualMachine
		proxySvc   *corev1.Service
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vm",
			},
		}

		wcr = &vmopv1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-wcr",
			},
			Spec: vmopv1.WebConsoleRequestSpec{
				VirtualMachineName: vm.Name,
				PublicKey:          "",
			},
		}

		proxySvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      webconsolerequest.ProxyAddrServiceName,
				Namespace: webconsolerequest.ProxyAddrServiceNamespace,
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: "dummy-proxy-ip",
						},
					},
				},
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
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		reconciler = nil
		fakeVMProvider.Reset()
	})

	Context("ReconcileNormal", func() {
		BeforeEach(func() {
			initObjects = append(initObjects, wcr, vm, proxySvc)
		})

		JustBeforeEach(func() {
			fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
				return "some-fake-webmksticket", nil
			}
		})

		When("NoOp", func() {
			It("returns success", func() {
				err := reconciler.ReconcileNormal(wcrCtx)
				Expect(err).ToNot(HaveOccurred())

				Expect(wcrCtx.WebConsoleRequest.Status.ProxyAddr).To(Equal("dummy-proxy-ip"))
				Expect(wcrCtx.WebConsoleRequest.Status.Response).ToNot(BeEmpty())
				Expect(wcrCtx.WebConsoleRequest.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
				// Checking the label key only because UID will not be set to a resource during unit test.
				Expect(wcrCtx.WebConsoleRequest.Labels).To(HaveKey(webconsolerequest.UUIDLabelKey))
			})
		})
	})
}
