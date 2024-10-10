// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
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
			ctx    *builder.IntegrationTestContext
			obj    *capv1.Capabilities
			status capv1.CapabilitiesStatus
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()
			status = capv1.CapabilitiesStatus{}
			obj = &capv1.Capabilities{
				ObjectMeta: metav1.ObjectMeta{
					Name: capabilities.CapabilitiesName,
				},
			}
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
			obj.Status = status
			Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
		})

		AfterEach(func() {
			atomic.StoreInt32(&numExits, 0)
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
							config.Features.BringYourOwnEncryptionKey = false
							config.Features.TKGMultipleCL = false
							config.Features.WorkloadDomainIsolation = false
						},
					)

					status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
						capabilities.CapabilityKeyBringYourOwnKeyProvider: {
							Activated: true,
						},
						capabilities.CapabilityKeyTKGMultipleContentLibraries: {
							Activated: true,
						},
						capabilities.CapabilityKeyWorkloadIsolation: {
							Activated: true,
						},
					}
				})
				Specify("the pod was exited", func() {
					Eventually(func(g Gomega) {
						g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(1)))
					}, time.Second*5).Should(Succeed())
				})
				Specify("feature states should be updated", func() {
					Eventually(func(g Gomega) {
						g.Expect(pkgcfg.FromContext(suite.Context).Features.BringYourOwnEncryptionKey).To(BeTrue())
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
							config.Features.BringYourOwnEncryptionKey = true
							config.Features.TKGMultipleCL = true
							config.Features.WorkloadDomainIsolation = true
						},
					)

					status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
						capabilities.CapabilityKeyBringYourOwnKeyProvider: {
							Activated: false,
						},
						capabilities.CapabilityKeyTKGMultipleContentLibraries: {
							Activated: false,
						},
						capabilities.CapabilityKeyWorkloadIsolation: {
							Activated: false,
						},
					}
				})
				Specify("the pod was exited", func() {
					Eventually(func(g Gomega) {
						g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(1)))
					}, time.Second*5).Should(Succeed())
				})
				Specify("feature states should be updated", func() {
					Eventually(func(g Gomega) {
						g.Expect(pkgcfg.FromContext(suite.Context).Features.BringYourOwnEncryptionKey).To(BeFalse())
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
						config.Features.BringYourOwnEncryptionKey = false
						config.Features.TKGMultipleCL = false
						config.Features.WorkloadDomainIsolation = false
					},
				)

				status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
					capabilities.CapabilityKeyBringYourOwnKeyProvider: {
						Activated: true,
					},
					capabilities.CapabilityKeyTKGMultipleContentLibraries: {
						Activated: true,
					},
					capabilities.CapabilityKeyWorkloadIsolation: {
						Activated: true,
					},
				}
			})

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(pkgcfg.FromContext(suite.Context).Features.BringYourOwnEncryptionKey).To(BeTrue())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeTrue())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeTrue())
				}, time.Second*5).Should(Succeed())

				obj.Status.Supervisor[capabilities.CapabilityKeyBringYourOwnKeyProvider] = capv1.CapabilityStatus{
					Activated: false,
				}
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			})

			Specify("the pod was exited once on create and once on update", func() {
				Eventually(func(g Gomega) {
					g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(2)))
				}, time.Second*5).Should(Succeed())
			})

			Specify("feature states should be updated", func() {
				Eventually(func(g Gomega) {
					g.Expect(pkgcfg.FromContext(suite.Context).Features.BringYourOwnEncryptionKey).To(BeFalse())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeTrue())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeTrue())
				}, time.Second*5).Should(Succeed())
			})
		})

		When("the capabilities have not changed", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(
					suite.Context,
					func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = false
						config.Features.TKGMultipleCL = false
						config.Features.WorkloadDomainIsolation = false
					},
				)

				status.Supervisor = map[capv1.CapabilityName]capv1.CapabilityStatus{
					capabilities.CapabilityKeyBringYourOwnKeyProvider: {
						Activated: false,
					},
					capabilities.CapabilityKeyTKGMultipleContentLibraries: {
						Activated: false,
					},
					capabilities.CapabilityKeyWorkloadIsolation: {
						Activated: false,
					},
				}
			})
			Specify("the pod was not exited and features were not updated", func() {
				Consistently(func(g Gomega) {
					g.Expect(atomic.LoadInt32(&numExits)).To(Equal(int32(0)))
					g.Expect(pkgcfg.FromContext(suite.Context).Features.BringYourOwnEncryptionKey).To(BeFalse())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.TKGMultipleCL).To(BeFalse())
					g.Expect(pkgcfg.FromContext(suite.Context).Features.WorkloadDomainIsolation).To(BeFalse())
				}, time.Second*3).Should(Succeed())
			})
		})
	})
