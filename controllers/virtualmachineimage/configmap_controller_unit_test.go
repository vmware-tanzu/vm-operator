// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTestsCM() {
	Context("Create ContentSource for a content library", func() {
		var (
			initObjects []runtime.Object
			ctx         *builder.UnitTestContextForController

			reconciler *virtualmachineimage.ConfigMapReconciler
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
			reconciler = virtualmachineimage.NewCMReconciler(
				ctx.Client,
				ctx.Logger,
				ctx.Namespace,
				ctx.VmProvider,
			)
		})

		Context("CreateContentSourceResources", func() {
			BeforeEach(func() {
				cm.Data[vsphere.ContentSourceKey] = clUUID
				initObjects = append(initObjects, cm)
			})

			When("called with a CL UUID", func() {
				It("creates ContentSource and ContentLibraryProvider resources", func() {
					err := reconciler.CreateContentSourceResources(ctx, clUUID)
					Expect(err).NotTo(HaveOccurred())

					objKey := client.ObjectKey{Name: clUUID}

					By("ContentSource should be created and have a ProviderRef for the CL")
					cs := &vmopv1alpha1.ContentSource{}
					err = ctx.Client.Get(ctx, objKey, cs)
					Expect(err).NotTo(HaveOccurred())
					Expect(cs.Spec.ProviderRef.Name).To(Equal(clUUID))

					By("ContentLibraryProvider should be created for the CL")
					cl := &vmopv1alpha1.ContentLibraryProvider{}
					err = ctx.Client.Get(ctx, objKey, cl)
					Expect(err).NotTo(HaveOccurred())
					Expect(cl.Spec.UUID).To(Equal(clUUID))
				})
			})
		})
	})
}
