// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.V1Alpha3,
	),
	func() {

		var (
			ctx *builder.IntegrationTestContext
			obj *corev1.ConfigMap
		)

		BeforeEach(func() {
			atomic.StoreInt32(&numExits, 0)
			ctx = suite.NewIntegrationTestContext()
			obj = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      capabilities.ConfigMapName,
					Namespace: capabilities.ConfigMapNamespace,
				},
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			Expect(ctx.Client.Delete(ctx, obj)).To(Succeed())
			Eventually(apierrors.IsNotFound(
				ctx.Client.Get(ctx, capabilities.ConfigMapKey, obj),
			), time.Second*5).Should(BeTrue())

			ctx.AfterEach()
			ctx = nil
		})

		When("the capabilities have changed", func() {
			When("some capabilities are enabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(
						suite.Context,
						func(config *pkgcfg.Config) {
							config.Features.TKGMultipleCL = false
							config.Features.WorkloadDomainIsolation = false
						},
					)

					obj.Data = map[string]string{
						capabilities.CapabilityKeyTKGMultipleContentLibraries: "true",
						capabilities.CapabilityKeyWorkloadIsolation:           "true",
					}
				})
				Specify("the pod was exited", func() {
					Eventually(func(g Gomega) {
						g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(1)))
					}, time.Second*5).Should(Succeed())
				})
				Specify("feature states should be updated", func() {
					Eventually(func(g Gomega) {
						g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeTrue())
						g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeTrue())
					}, time.Second*5).Should(Succeed())
				})
			})

			When("some capabilities are disabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(
						suite.Context,
						func(config *pkgcfg.Config) {
							config.Features.TKGMultipleCL = true
							config.Features.WorkloadDomainIsolation = true
						},
					)

					obj.Data = map[string]string{
						capabilities.CapabilityKeyTKGMultipleContentLibraries: "false",
						capabilities.CapabilityKeyWorkloadIsolation:           "false",
					}
				})
				Specify("the pod was exited", func() {
					Eventually(func(g Gomega) {
						g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(1)))
					}, time.Second*5).Should(Succeed())
				})
				Specify("feature states should be updated", func() {
					Eventually(func(g Gomega) {
						g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeFalse())
						g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeFalse())
					}, time.Second*5).Should(Succeed())
				})
			})
		})

		When("capabilities have changed twice", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(
					suite.Context,
					func(config *pkgcfg.Config) {
						config.Features.TKGMultipleCL = false
						config.Features.WorkloadDomainIsolation = false
					},
				)

				obj.Data = map[string]string{
					capabilities.CapabilityKeyTKGMultipleContentLibraries: "true",
					capabilities.CapabilityKeyWorkloadIsolation:           "true",
				}
			})

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeTrue())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeTrue())
				}, time.Second*5).Should(Succeed())

				obj.Data[capabilities.CapabilityKeyTKGMultipleContentLibraries] = "false"
				Expect(ctx.Client.Update(ctx, obj)).To(Succeed())
			})

			Specify("the pod was exited once on create and once on update", func() {
				Eventually(func(g Gomega) {
					g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(2)))
				}, time.Second*5).Should(Succeed())
			})

			Specify("feature states should be updated", func() {
				Eventually(func(g Gomega) {
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeFalse())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeTrue())
				}, time.Second*5).Should(Succeed())
			})
		})

		When("the capabilities have not changed", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(
					suite.Context,
					func(config *pkgcfg.Config) {
						config.Features.TKGMultipleCL = false
						config.Features.WorkloadDomainIsolation = false
					},
				)

				obj.Data = map[string]string{
					capabilities.CapabilityKeyTKGMultipleContentLibraries: "false",
					capabilities.CapabilityKeyWorkloadIsolation:           "false",
				}
			})
			Specify("the pod was not exited and features were not updated", func() {
				Consistently(func(g Gomega) {
					g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(0)))
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeFalse())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeFalse())
				}, time.Second*3).Should(Succeed())
			})
		})
	})
