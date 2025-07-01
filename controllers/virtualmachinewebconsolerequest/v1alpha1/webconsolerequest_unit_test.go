// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	webconsolerequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/providers/fake"
	proxyaddr "github.com/vmware-tanzu/vm-operator/pkg/util/kube/proxyaddr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.V1Alpha1,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	const v1a2Ticket = "some-fake-webmksticket-v1a2"

	var (
		initObjects    []client.Object
		ctx            *builder.UnitTestContextForController
		fakeVMProvider *providerfake.VMProvider

		reconciler  *webconsolerequest.Reconciler
		wcrCtx      *pkgctx.WebConsoleRequestContext
		wcr         *vmopv1a1.WebConsoleRequest
		vm          *vmopv1a1.VirtualMachine
		proxySvc    *corev1.Service
		proxySvcDNS *appv1a1.SupervisorProperties
	)

	BeforeEach(func() {
		vm = &vmopv1a1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vm",
			},
		}

		wcr = &vmopv1a1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-wcr",
			},
			Spec: vmopv1a1.WebConsoleRequestSpec{
				VirtualMachineName: vm.Name,
				PublicKey:          "",
			},
		}

		proxySvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxyaddr.ProxyAddrServiceName,
				Namespace: proxyaddr.ProxyAddrServiceNamespace,
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
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)

		wcrCtx = &pkgctx.WebConsoleRequestContext{
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
	})

	Context("ReconcileNormal", func() {
		var (
			v1a2ProviderCalled atomic.Bool
		)

		BeforeEach(func() {
			initObjects = append(initObjects, wcr, vm, proxySvc)
		})

		JustBeforeEach(func() {
			fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
				v1a2ProviderCalled.Store(true)
				return v1a2Ticket, nil
			}
		})

		AfterEach(func() {
			fakeVMProvider.Reset()
			v1a2ProviderCalled.Store(false)
		})

		It("returns success", func() {
			err := reconciler.ReconcileNormal(wcrCtx)
			Expect(err).ToNot(HaveOccurred())

			Expect(v1a2ProviderCalled.Load()).Should(BeTrue())
			Expect(wcrCtx.WebConsoleRequest.Status.ProxyAddr).To(Equal("dummy-proxy-ip"))
			Expect(wcrCtx.WebConsoleRequest.Status.Response).To(Equal(v1a2Ticket))
			Expect(wcrCtx.WebConsoleRequest.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
			// Checking the label key only because UID will not be set to a resource during unit test.
			Expect(wcrCtx.WebConsoleRequest.Labels).To(HaveKey(webconsolerequest.UUIDLabelKey))
		})

		When("Web Console returns correct proxy address", func() {

			DescribeTable("DNS Names",
				func(apiServerDNSName []string, expectedProxy string) {
					proxySvcDNS = &appv1a1.SupervisorProperties{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "supervisor-env-props",
							Namespace: "vmware-system-supervisor-services",
						},
						Spec: appv1a1.SupervisorPropertiesSpec{
							APIServerDNSNames: apiServerDNSName,
						},
					}
					Expect(ctx.Client.Create(ctx, proxySvcDNS)).To(Succeed())

					Expect(reconciler.ReconcileNormal(wcrCtx)).To(Succeed())
					Expect(wcrCtx.WebConsoleRequest.Status.ProxyAddr).To(Equal(expectedProxy))
				},
				Entry("API Server DNS Name is set", []string{"domain-1.test"}, "domain-1.test"),
				Entry("API Server DNS Name is not set", []string{}, "dummy-proxy-ip"),
			)
		})
	})

}
