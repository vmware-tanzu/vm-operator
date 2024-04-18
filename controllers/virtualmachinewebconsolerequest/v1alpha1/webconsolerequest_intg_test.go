// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	webconsolerequest "github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.V1Alpha1,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	const v1a2Ticket = "some-fake-webmksticket-v1a2"

	var (
		ctx                *builder.IntegrationTestContext
		wcr                *vmopv1a1.WebConsoleRequest
		vm                 *vmopv1a1.VirtualMachine
		proxySvc           *corev1.Service
		v1a2ProviderCalled bool
	)

	getWebConsoleRequest := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1a1.WebConsoleRequest {
		wcr := &vmopv1a1.WebConsoleRequest{}
		if err := ctx.Client.Get(ctx, objKey, wcr); err != nil {
			return nil
		}
		return wcr
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1a1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1a1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				PowerState: vmopv1a1.VirtualMachinePoweredOn,
			},
		}

		_, publicKeyPem := builder.WebConsoleRequestKeyPair()

		wcr = &vmopv1a1.WebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-wcr",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1a1.WebConsoleRequestSpec{
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

		intgFakeVMProvider.Lock()
		intgFakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
			v1a2ProviderCalled = true
			return v1a2Ticket, nil
		}
		intgFakeVMProvider.Unlock()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
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
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, proxySvc)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("resource successfully created", func() {
			objKey := types.NamespacedName{Name: wcr.Name, Namespace: wcr.Namespace}

			Eventually(func(g Gomega) {
				wcr = getWebConsoleRequest(ctx, objKey)
				g.Expect(wcr).ToNot(BeNil())
				g.Expect(wcr.Status.Response).ToNot(BeEmpty())
			}).Should(Succeed(), "waiting response to be set")

			Expect(v1a2ProviderCalled).Should(BeTrue())
			Expect(wcr.Status.ProxyAddr).To(Equal("192.168.0.1"))
			Expect(wcr.Status.Response).To(Equal(v1a2Ticket))
			Expect(wcr.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), webconsolerequest.DefaultExpiryTime))
			Expect(wcr.Labels).To(HaveKeyWithValue(webconsolerequest.UUIDLabelKey, string(wcr.UID)))
		})
	})
}
