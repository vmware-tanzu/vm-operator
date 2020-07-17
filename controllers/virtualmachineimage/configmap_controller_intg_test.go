// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineimage_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTestsCM() {
	var (
		ctx *builder.IntegrationTestContext

		cm     *corev1.ConfigMap
		clUUID = "dummy-cl"
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vsphere.ProviderConfigMapName,
				Namespace: ctx.PodNamespace,
			},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {

		clExists := func(clName string) bool {
			clList := &vmopv1alpha1.ContentLibraryProviderList{}
			Expect(ctx.Client.List(ctx, clList)).To(Succeed())

			for _, cl := range clList.Items {
				if cl.Name == clName {
					return true
				}
			}

			return false
		}

		// Verifies that a ContentSource exists and has a ProviderRef set to the content library.
		csExists := func(csName, clName string) bool {
			csList := &vmopv1alpha1.ContentSourceList{}
			Expect(ctx.Client.List(ctx, csList)).To(Succeed())

			for _, cs := range csList.Items {
				if cs.Name == csName && cs.Spec.ProviderRef.Name == clName {
					return true
				}
			}

			return false
		}

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, cm)).To(Succeed())
		})

		JustAfterEach(func() {
			err := ctx.Client.Delete(ctx, cm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		When("ConfigMap is created without ContentSource key", func() {
			It("no ContentSource is created", func() {
				csList := &vmopv1alpha1.ContentSourceList{}
				err := ctx.Client.List(ctx, csList)
				Expect(err).NotTo(HaveOccurred())
				Expect(csList.Items).To(HaveLen(0))
			})
		})

		When("ConfigMap is created with ContentSource key", func() {
			BeforeEach(func() {
				cm.Data = make(map[string]string)
				cm.Data[vsphere.ContentSourceKey] = clUUID
			})

			It("a ContentSource is created", func() {
				// For now, only verify that the custom resources exist. Ideally, we should also check that no additional resources are present.
				// However, it is not possible to do that because the OwnerRef is maintained by the ContentSource controller that is not running
				// in this suite.
				Eventually(func() bool {
					return csExists(clUUID, clUUID) && clExists(clUUID)
				}).Should(BeTrue())
			})
		})

		When("ConfigMap's ContentSource key is updated", func() {
			BeforeEach(func() {
				cm.Data = make(map[string]string)
				cm.Data[vsphere.ContentSourceKey] = clUUID
			})

			It("ContentSource is updated to point to the new CL UUID from ConfigMap", func() {
				newCLUUID := "new-cl"
				cm.Data[vsphere.ContentSourceKey] = newCLUUID
				Expect(ctx.Client.Update(ctx, cm)).NotTo(HaveOccurred())

				Eventually(func() bool {
					return csExists(newCLUUID, newCLUUID) && clExists(newCLUUID)
				}).Should(BeTrue())
			})
		})
	})
}
