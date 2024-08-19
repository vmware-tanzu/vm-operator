// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/capability"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.V1Alpha3,
		),
		unitTestsReconcile,
	)
}

func unitTestsReconcile() {
	var (
		ctx *builder.UnitTestContextForController

		reconciler *capability.Reconciler
		configMap  *corev1.ConfigMap
	)

	const (
		dummyWCPClusterCapabilitiesConfigMapName = "dummy-wcp-cluster-capabilities"
		dummyWCPClusterCapabilitiesNamespace     = "dummy-ns"
	)

	BeforeEach(func() {
		ctx = suite.NewUnitTestContextForController()
		reconciler = capability.NewReconciler(
			ctx,
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
		)

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      capability.WCPClusterCapabilitiesConfigMapName,
				Namespace: capability.WCPClusterCapabilitiesNamespace,
			},
			Data: map[string]string{},
		}
		Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		When("configmap with different name enters the reconcile", func() {
			It("should not reconcile wcp cluster capabilities config and return back", func() {
				obj := client.ObjectKey{Namespace: dummyWCPClusterCapabilitiesNamespace, Name: dummyWCPClusterCapabilitiesConfigMapName}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: obj})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("configmap does not exists", func() {
			It("should return false", func() {
				Expect(ctx.Client.Delete(ctx, configMap)).To(Succeed())
				obj := client.ObjectKey{Namespace: configMap.Namespace, Name: configMap.Name}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: obj})
				Expect(err).NotTo(HaveOccurred())
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
			})
		})

		When("configmap Data is empty", func() {
			It("should return false", func() {
				obj := client.ObjectKey{Namespace: configMap.Namespace, Name: configMap.Name}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: obj})
				Expect(err).NotTo(HaveOccurred())
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
			})
		})

		When("configmap Data is invalid", func() {
			BeforeEach(func() {
				data := map[string]string{
					capability.TKGMultipleCLCapabilityKey: "not-valid",
				}
				configMap.Data = data
				Expect(ctx.Client.Update(ctx, configMap)).To(Succeed())
			})

			It("should return false", func() {
				obj := client.ObjectKey{Namespace: configMap.Namespace, Name: configMap.Name}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: obj})
				Expect(err).NotTo(HaveOccurred())
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeFalse())
			})
		})

		When("configmap data is valid", func() {
			BeforeEach(func() {
				data := map[string]string{
					capability.TKGMultipleCLCapabilityKey: "true",
				}
				configMap.Data = data
				Expect(ctx.Client.Update(ctx, configMap)).To(Succeed())
			})

			It("should return true", func() {
				obj := client.ObjectKey{Namespace: configMap.Namespace, Name: configMap.Name}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: obj})
				Expect(err).NotTo(HaveOccurred())
				Expect(pkgcfg.FromContext(ctx).Features.TKGMultipleCL).To(BeTrue())
			})
		})
	})
}
