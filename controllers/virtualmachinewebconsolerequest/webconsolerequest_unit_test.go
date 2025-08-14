// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinewebconsolerequest_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest"
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
		Label(testlabels.Controller, testlabels.V1Alpha1),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {

	var (
		initObjects    []client.Object
		ctx            *builder.UnitTestContextForController
		fakeVMProvider *providerfake.VMProvider

		reconciler  *virtualmachinewebconsolerequest.Reconciler
		wcrCtx      *pkgctx.WebConsoleRequestContextV1
		wcr         *vmopv1.VirtualMachineWebConsoleRequest
		vm          *vmopv1.VirtualMachine
		proxySvc    *corev1.Service
		proxySvcDNS *appv1a1.SupervisorProperties
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vm",
			},
		}

		wcr = &vmopv1.VirtualMachineWebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-wcr",
			},
			Spec: vmopv1.VirtualMachineWebConsoleRequestSpec{
				Name:      vm.Name,
				PublicKey: "",
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
		reconciler = virtualmachinewebconsolerequest.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VMProvider,
		)
		fakeVMProvider = ctx.VMProvider.(*providerfake.VMProvider)

		wcrCtx = &pkgctx.WebConsoleRequestContextV1{
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
		const ticket = "my-fake-webmksticket"

		BeforeEach(func() {
			initObjects = append(initObjects, wcr, vm, proxySvc)
		})

		When("GetVirtualMachineWebMKSTicket returns success", func() {

			JustBeforeEach(func() {
				fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
					return ticket, nil
				}
			})

			It("returns success", func() {
				err := reconciler.ReconcileNormal(wcrCtx)
				Expect(err).ToNot(HaveOccurred())

				Expect(wcrCtx.WebConsoleRequest.Status.ProxyAddr).To(Equal("dummy-proxy-ip"))
				Expect(wcrCtx.WebConsoleRequest.Status.Response).To(Equal(ticket))
				Expect(wcrCtx.WebConsoleRequest.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), virtualmachinewebconsolerequest.DefaultExpiryTime))
				// Checking the label key only because UID will not be set to a resource during unit test.
				Expect(wcrCtx.WebConsoleRequest.Labels).To(HaveKey(virtualmachinewebconsolerequest.UUIDLabelKey))
			})
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
