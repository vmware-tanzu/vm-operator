// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentsource_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const finalizer = "contentsource.vmoperator.vmware.com"

func intgTests() {
	var (
		ctx *builder.IntegrationTestContext

		cs    vmopv1alpha1.ContentSource
		cl    vmopv1alpha1.ContentLibraryProvider
		csKey types.NamespacedName
		clKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		cl = vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-cl",
				Namespace: "dummy-ns",
			},
		}
		cs = vmopv1alpha1.ContentSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-cs",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.ContentSourceSpec{
				ProviderRef: vmopv1alpha1.ContentProviderReference{
					APIVersion: "vmoperator.vmware.com/v1alpha1",
					Name:       cl.ObjectMeta.Name,
					Kind:       "ContentLibraryProvider",
				},
			},
		}

		csKey = types.NamespacedName{Name: cs.ObjectMeta.Name}
		clKey = types.NamespacedName{Name: cl.ObjectMeta.Name}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVMProvider.Reset()
	})

	getContentSource := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.ContentSource {
		cs := &vmopv1alpha1.ContentSource{}
		if err := ctx.Client.Get(ctx, objKey, cs); err != nil {
			return nil
		}
		return cs
	}

	waitForContentSourceFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func() []string {
			if cs := getContentSource(ctx, objKey); cs != nil {
				return cs.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for ContentSource finalizer")
	}

	getContentLibraryProvider := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.ContentLibraryProvider {
		cl := &vmopv1alpha1.ContentLibraryProvider{}
		if err := ctx.Client.Get(ctx, objKey, cl); err != nil {
			return nil
		}

		// fmt.Printf("CL Found: %+v\n\n\n", cl)
		return cl
	}

	waitForCLProviderOwnerReference := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName, ownerRef metav1.OwnerReference) {
		Eventually(func() []metav1.OwnerReference {
			if cl := getContentLibraryProvider(ctx, objKey); cl != nil {
				return cl.OwnerReferences
			}
			return []metav1.OwnerReference{}
		}).Should(ContainElement(ownerRef), "waiting for ContentSource OwnerRef on the ContentLibraryProvider resource")
	}

	Context("Reconcile ContentSource", func() {
		When("ContentSource and ContentLibraryProvider exists", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, &cl)).To(Succeed())
				Expect(ctx.Client.Create(ctx, &cs)).To(Succeed())
			})

			AfterEach(func() {
				err := ctx.Client.Delete(ctx, &cl)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

				err = ctx.Client.Delete(ctx, &cs)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("Reconciles after ContentSource creation", func() {
				By("ContentSource should have a finalizer added", func() {
					waitForContentSourceFinalizer(ctx, csKey)
				})

				By("ContentLibraryProvider should have OwnerReference set", func() {
					csObj := getContentSource(ctx, csKey)
					isController := true
					ownerRef := metav1.OwnerReference{
						// Not sure why we have to set these manually. csObj.APIVersion is "".
						APIVersion: "vmoperator.vmware.com/v1alpha1",
						Kind:       "ContentSource",
						Name:       csObj.Name,
						UID:        csObj.UID,
						Controller: &isController,
					}
					waitForCLProviderOwnerReference(ctx, clKey, ownerRef)
				})
			})
		})
	})
}
