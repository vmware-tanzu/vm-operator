// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinewebconsolerequest_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	proxyaddr "github.com/vmware-tanzu/vm-operator/pkg/util/kube/proxyaddr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
			testlabels.API,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	const ticket = "some-fake-webmksticket"

	var (
		ctx      *builder.IntegrationTestContext
		wcr      *vmopv1.VirtualMachineWebConsoleRequest
		vm       *vmopv1.VirtualMachine
		proxySvc *corev1.Service
	)

	getWebConsoleRequest := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1.VirtualMachineWebConsoleRequest {
		wcr := &vmopv1.VirtualMachineWebConsoleRequest{}
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
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
		}

		_, publicKeyPem := builder.WebConsoleRequestKeyPair()

		wcr = &vmopv1.VirtualMachineWebConsoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-wcr",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1.VirtualMachineWebConsoleRequestSpec{
				Name:      vm.Name,
				PublicKey: publicKeyPem,
			},
		}

		proxySvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxyaddr.ProxyAddrServiceName,
				Namespace: proxyaddr.ProxyAddrServiceNamespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					{
						Name: "dummy-proxy-port",
						Port: 443,
					},
				},
			},
		}

		intgFakeVMProvider.Lock()
		defer intgFakeVMProvider.Unlock()
		intgFakeVMProvider.GetVirtualMachineWebMKSTicketFn = func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
			return ticket, nil
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, wcr)).To(Succeed())
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
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, wcr)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("resource successfully created", func() {
			objKey := types.NamespacedName{Name: wcr.Name, Namespace: wcr.Namespace}

			Eventually(func(g Gomega) {
				wcr = getWebConsoleRequest(ctx, objKey)
				g.Expect(wcr).ToNot(BeNil())
				g.Expect(wcr.Status.Response).ToNot(BeEmpty())
			}).Should(Succeed(), "waiting response to be set")

			Expect(wcr.Status.ProxyAddr).To(Equal("192.168.0.1"))
			Expect(wcr.Status.Response).To(Equal(ticket))
			Expect(wcr.Status.ExpiryTime.Time).To(BeTemporally("~", time.Now(), virtualmachinewebconsolerequest.DefaultExpiryTime))
			Expect(wcr.Labels).To(HaveKeyWithValue(virtualmachinewebconsolerequest.UUIDLabelKey, string(wcr.UID)))
		})
	})
}
