// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providerconfigmap_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/providerconfigmap"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Provider ConfigMap controller tests", unitTestsCM)
}

func unitTestsCM() {
	Context("Create ContentSource for a content library", func() {
		var (
			initObjects []client.Object
			ctx         *builder.UnitTestContextForController

			reconciler *providerconfigmap.ConfigMapReconciler
			cm         *corev1.ConfigMap
			clUUID     string
		)

		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-cs",
				Namespace: "dummy-ns",
			},
			Data: make(map[string]string),
		}
		clUUID = "dummy-cl"

		JustBeforeEach(func() {
			ctx = suite.NewUnitTestContextForController(initObjects...)
			reconciler = providerconfigmap.NewReconciler(
				ctx.Client,
				ctx.Logger,
				ctx.VMProvider,
			)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			initObjects = nil
			reconciler = nil
		})

		Context("CreateOrUpdateContentSourceResources", func() {
			BeforeEach(func() {
				cm.Data[config.ContentSourceKey] = clUUID
				initObjects = append(initObjects, cm)
			})

			When("called with a CL UUID", func() {
				It("creates ContentSource and ContentLibraryProvider resources", func() {
					err := reconciler.CreateOrUpdateContentSourceResources(ctx, clUUID)
					Expect(err).NotTo(HaveOccurred())

					objKey := client.ObjectKey{Name: clUUID}

					By("ContentSource should be created and have a ProviderRef for the CL")
					cs := &vmopv1.ContentSource{}
					err = ctx.Client.Get(ctx, objKey, cs)
					Expect(err).NotTo(HaveOccurred())

					By("ContentSource should have label set")
					Expect(cs.Labels).To(HaveKeyWithValue(providerconfigmap.TKGContentSourceLabelKey, providerconfigmap.TKGContentSourceLabelValue))

					By("ContentLibraryProvider should be created for the CL")
					cl := &vmopv1.ContentLibraryProvider{}
					err = ctx.Client.Get(ctx, objKey, cl)
					Expect(err).NotTo(HaveOccurred())
					Expect(cl.Spec.UUID).To(Equal(clUUID))

					Expect(cs.Spec.ProviderRef.Name).To(Equal(clUUID))
					Expect(cs.Spec.ProviderRef.Kind).To(Equal(cl.Kind))
					Expect(cs.Spec.ProviderRef.APIVersion).To(Equal(cl.APIVersion))
				})
			})
		})

		Context("CreateContentSourceBindings", func() {
			var (
				workloadNS *corev1.Namespace
			)
			BeforeEach(func() {
				cm.Data[config.ContentSourceKey] = clUUID

				workloadNS = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							providerconfigmap.UserWorkloadNamespaceLabel: "cluster-moid",
						},
					},
				}
				initObjects = append(initObjects, cm, workloadNS)
			})

			When("called with a CL UUID", func() {
				It("creates ContentSource and ContentLibraryProvider resources", func() {
					// So the ContentSource and the ContentLibraryProvider resources are created.
					err := reconciler.CreateOrUpdateContentSourceResources(ctx, clUUID)
					Expect(err).NotTo(HaveOccurred())

					err = reconciler.CreateContentSourceBindings(ctx, clUUID)
					Expect(err).NotTo(HaveOccurred())

					binding := &vmopv1.ContentSourceBinding{}
					err = ctx.Client.Get(ctx, client.ObjectKey{Name: clUUID, Namespace: workloadNS.Name}, binding)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	})
}
