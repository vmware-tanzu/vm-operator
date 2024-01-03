// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1alpha2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	webconsolerequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking WebConsoleRequest controller tests", webConsoleRequestReconcile)
}

func webConsoleRequestReconcile() {
	var (
		ctx                *builder.IntegrationTestContext
		wcr                *vmopv1.WebConsoleRequest
		vm                 *vmopv1.VirtualMachine
		proxySvc           *corev1.Service
		v1a1ProviderCalled bool
		v1a2ProviderCalled bool
	)

	getWebConsoleRequest := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.WebConsoleRequest {
		wcr := &vmopv1.WebConsoleRequest{}
		if err := ctx.Client.Get(ctx, objKey, wcr); err != nil {
			return nil
		}
		return wcr
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				PowerState: vmopv1.VirtualMachinePoweredOn,
			},
		}

		_, publicKeyPem := builder.WebConsoleRequestKeyPair()

		wcr = &vmopv1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-wcr",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.WebConsoleRequestSpec{
				VirtualMachineName: vm.Name,
				PublicKey:          publicKeyPem,
			},
		}

		proxySvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      webconsolerequest.ProxyAddrServiceName,
				Namespace: webconsolerequest.ProxyAddrServiceNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "dummy-proxy-port",
						Port: 443,
					},
				},
			},
		}

		fakeVMProvider.Lock()
		defer fakeVMProvider.Unlock()

		fakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
			v1a1ProviderCalled = true
			return "some-fake-webmksticket", nil
		}

		fakeVMProviderA2.Lock()
		defer fakeVMProviderA2.Unlock()

		fakeVMProviderA2.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1alpha2.VirtualMachine, pubKey string) (string, error) {
			v1a2ProviderCalled = true
			return "some-fake-webmksticket-1", nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		fakeVMProvider.Reset()
		fakeVMProviderA2.Reset()
		v1a1ProviderCalled = false
		v1a2ProviderCalled = false
	})

	Context("Reconcile", func() {
		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, proxySvc)).To(Succeed())
			proxySvc.Status = corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: "192.168.0.1",
						},
					},
				},
			}
			Expect(ctx.Client.Status().Update(ctx, proxySvc)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, wcr)).To(Succeed())
		})

		JustAfterEach(func() {
			err := ctx.Client.Delete(ctx, wcr)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, proxySvc)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("v1a1 provider", func() {
			It("resource successfully created", func() {
				Eventually(func(g Gomega) {
					wcr = getWebConsoleRequest(ctx, types.NamespacedName{Name: wcr.Name, Namespace: wcr.Namespace})
					g.Expect(wcr).ToNot(BeNil())
					g.Expect(wcr.Status.Response).ToNot(BeEmpty())
				}).Should(Succeed(), "waiting for webconsolerequest to be")

				Expect(v1a1ProviderCalled).Should(BeTrue())
				Expect(wcr.Status.ProxyAddr).To(Equal("192.168.0.1"))
				Expect(wcr.Status.Response).ToNot(BeEmpty())
				Expect(wcr.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
				Expect(wcr.Labels).To(HaveKeyWithValue(webconsolerequest.UUIDLabelKey, string(wcr.UID)))
			})
		})

		When("VM Service v1a2 FSS enabled", func() {
			BeforeEach(func() {
				pkgconfig.SetContext(suite, func(config *pkgconfig.Config) {
					config.Features.VMOpV1Alpha2 = true
				})
			})

			AfterEach(func() {
				pkgconfig.SetContext(suite, func(config *pkgconfig.Config) {
					config.Features.VMOpV1Alpha2 = false
				})
			})

			It("resource successfully created", func() {
				Eventually(func(g Gomega) {
					wcr = getWebConsoleRequest(ctx, types.NamespacedName{Name: wcr.Name, Namespace: wcr.Namespace})
					g.Expect(wcr).ToNot(BeNil())
					g.Expect(wcr.Status.Response).ToNot(BeEmpty())
				}).Should(Succeed(), "waiting for webconsolerequest to be")

				Expect(v1a2ProviderCalled).Should(BeTrue())
				Expect(wcr.Status.ProxyAddr).To(Equal("192.168.0.1"))
				Expect(wcr.Status.Response).ToNot(BeEmpty())
				Expect(wcr.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
				Expect(wcr.Labels).To(HaveKeyWithValue(webconsolerequest.UUIDLabelKey, string(wcr.UID)))
			})
		})
	})
}
