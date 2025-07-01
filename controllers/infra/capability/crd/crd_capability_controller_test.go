// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capability_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/config/capabilities"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"Reconcile",
	Label(
		testlabels.Controller,
		testlabels.EnvTest,
		testlabels.API,
	),
	Ordered,
	func() {

		var (
			ctx    *builder.IntegrationTestContext
			obj    *capv1.Capabilities
			status capv1.CapabilitiesStatus
			dep    appsv1.Deployment
			depKey ctrlclient.ObjectKey
		)

		BeforeEach(func() {
			ctx = suite.NewIntegrationTestContext()
			status = capv1.CapabilitiesStatus{}
			obj = &capv1.Capabilities{
				ObjectMeta: metav1.ObjectMeta{
					Name: capabilities.CapabilitiesName,
				},
			}

			dep = appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "vmop-deployment-",
					Namespace:    ctx.PodNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "controller-manager",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-manager",
							Namespace: ctx.PodNamespace,
							Labels: map[string]string{
								"app": "controller-manager",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "controller-manager",
									Image: "vmop:latest",
								},
							},
						},
					},
				},
			}

			Expect(ctx.Client.Create(ctx, &dep)).To(Succeed())

			depKey = ctrlclient.ObjectKeyFromObject(&dep)

			pkgcfg.SetContext(
				suite.Context,
				func(config *pkgcfg.Config) {
					config.DeploymentName = dep.Name
					config.PodNamespace = ctx.PodNamespace
				},
			)
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
			obj.Status = status
			Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
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
						var dep appsv1.Deployment
						g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
						lastExitTimeStr := dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
						g.Expect(lastExitTimeStr).ToNot(BeEmpty())
						lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
						g.Expect(lastExitReason).To(Equal("capabilities have changed: BringYourOwnEncryptionKey=true,TKGMultipleCL=true,WorkloadDomainIsolation=true"))
					}, time.Second*5).Should(Succeed())
				})

				When("there is an error getting the deployment", func() {
					BeforeEach(func() {
						Expect(ctx.Client.Delete(ctx, &dep)).To(Succeed())
					})
					Specify("coverage", FlakeAttempts(5), func() {
						// No test, just for coverage
					})
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
						var dep appsv1.Deployment
						g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
						lastExitTimeStr := dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
						g.Expect(lastExitTimeStr).ToNot(BeEmpty())
						lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
						g.Expect(lastExitReason).To(Equal("capabilities have changed: BringYourOwnEncryptionKey=false,TKGMultipleCL=false,WorkloadDomainIsolation=false"))
					}, time.Second*5).Should(Succeed())
				})
			})
		})

		When("capabilities have changed twice", func() {
			var lastExitTimeStr1 string

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
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					lastExitTimeStr1 = dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
					g.Expect(lastExitTimeStr1).ToNot(BeEmpty())
					lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
					g.Expect(lastExitReason).To(Equal("capabilities have changed: BringYourOwnEncryptionKey=true,TKGMultipleCL=true,WorkloadDomainIsolation=true"))
				}, time.Second*5).Should(Succeed())

				pkgcfg.SetContext(suite.Context, func(config *pkgcfg.Config) {
					config.Features.BringYourOwnEncryptionKey = true
					config.Features.TKGMultipleCL = true
					config.Features.WorkloadDomainIsolation = true
				})

				obj.Status.Supervisor[capabilities.CapabilityKeyBringYourOwnKeyProvider] = capv1.CapabilityStatus{
					Activated: false,
				}
				Expect(ctx.Client.Status().Update(ctx, obj)).To(Succeed())
			})

			Specify("the pod was exited once on create and once on update", func() {
				Eventually(func(g Gomega) {
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					lastExitTimeStr2 := dep.Spec.Template.Annotations[pkgconst.LastRestartTimeAnnotationKey]
					g.Expect(lastExitTimeStr2).ToNot(BeEmpty())
					g.Expect(lastExitTimeStr2).ToNot(Equal(lastExitTimeStr1))
					lastExitReason := dep.Spec.Template.Annotations[pkgconst.LastRestartReasonAnnotationKey]
					g.Expect(lastExitReason).To(Equal("capabilities have changed: BringYourOwnEncryptionKey=false"))
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
					var dep appsv1.Deployment
					g.Expect(ctx.Client.Get(ctx, depKey, &dep)).To(Succeed())
					g.Expect(dep.Spec.Template.Annotations).ToNot(HaveKey(pkgconst.LastRestartTimeAnnotationKey))
					g.Expect(dep.Spec.Template.Annotations).ToNot(HaveKey(pkgconst.LastRestartReasonAnnotationKey))
				}, time.Second*3).Should(Succeed())
			})
		})
	})
