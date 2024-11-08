// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package proxyaddr

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Test Proxy Address", func() {
	var (
		ctx            context.Context
		client         ctrlclient.Client
		initialObjects []ctrlclient.Object
		proxySvc       *corev1.Service
		api            *appv1a1.SupervisorProperties
		withFuncs      interceptor.Funcs
	)

	const (
		apiServerDNSName = "domain-1.test"
		virtualIP        = "dummy-proxy-ip"
	)

	BeforeEach(func() {
		proxySvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProxyAddrServiceName,
				Namespace: ProxyAddrServiceNamespace,
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: virtualIP,
						},
					},
				},
			},
		}
		api = nil
		initialObjects = nil
		ctx = pkgcfg.NewContext()
		withFuncs = interceptor.Funcs{}
	})

	JustBeforeEach(func() {
		client = builder.NewFakeClientWithInterceptors(
			withFuncs, initialObjects...)
	})

	When("Simplified Enablement FSS is disabled", func() {
		BeforeEach(func() {
			api = &appv1a1.SupervisorProperties{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "supervisor-env-props",
					Namespace: "vmware-system-supervisor-services",
				},
				Spec: appv1a1.SupervisorPropertiesSpec{
					APIServerDNSNames: []string{apiServerDNSName},
				},
			}
			ctx = pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
				config.Features.SimplifiedEnablement = false
			})

			initialObjects = []ctrlclient.Object{proxySvc, api}
		})

		It("Should always return the virtual IP", func() {
			str, err := ProxyAddress(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(str).To(Equal(virtualIP))
		})

	})

	When("API Server DNS Names is NOT set", func() {
		BeforeEach(func() {
			api = &appv1a1.SupervisorProperties{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "supervisor-env-props",
					Namespace: "vmware-system-supervisor-services",
				},
				Spec: appv1a1.SupervisorPropertiesSpec{
					APIServerDNSNames: []string{},
				},
			}
			ctx = pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
				config.Features.SimplifiedEnablement = true
			})

			initialObjects = []ctrlclient.Object{proxySvc, api}
		})

		It("Should always return the virtual IP", func() {
			str, err := ProxyAddress(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(str).To(Equal(virtualIP))
		})

	})

	When("API Server DNS Names is set", func() {
		BeforeEach(func() {
			api = &appv1a1.SupervisorProperties{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "supervisor-env-props",
					Namespace: "vmware-system-supervisor-services",
				},
				Spec: appv1a1.SupervisorPropertiesSpec{
					APIServerDNSNames: []string{apiServerDNSName},
				},
			}
			ctx = pkgcfg.UpdateContext(ctx, func(config *pkgcfg.Config) {
				config.Features.SimplifiedEnablement = true
			})

			initialObjects = []ctrlclient.Object{proxySvc, api}
		})

		It("Should always return the DNS Name", func() {
			str, err := ProxyAddress(ctx, client)
			Expect(err).ToNot(HaveOccurred())
			Expect(str).To(Equal(apiServerDNSName))
		})

	})

})
