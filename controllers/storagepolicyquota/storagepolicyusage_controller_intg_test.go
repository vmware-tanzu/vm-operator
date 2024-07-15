// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storagepolicyquota_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Reconcile",
		Label(
			testlabels.Controller,
			testlabels.EnvTest,
		),
		intgTestsReconcile,
	)
}

func intgTestsReconcile() {
	var (
		ctx    *builder.IntegrationTestContext
		src    *spqv1.StoragePolicyQuota
		srcKey types.NamespacedName
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		src = &spqv1.StoragePolicyQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-quota",
				Namespace: ctx.Namespace,
			},
			Spec: spqv1.StoragePolicyQuotaSpec{
				StoragePolicyId: "my-policy-id",
			},
		}
		srcKey = client.ObjectKeyFromObject(src)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, src)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, src)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should result in the creation of a StoragePolicyUsage resource", func() {
			Eventually(func(g Gomega) {
				var src spqv1.StoragePolicyQuota
				g.Expect(ctx.Client.Get(ctx, srcKey, &src)).To(Succeed())

				var dst spqv1.StoragePolicyUsage
				dstKey := client.ObjectKey{
					Namespace: ctx.Namespace,
					Name:      kubeutil.StoragePolicyUsageNameFromQuotaName(srcKey.Name),
				}
				g.Expect(ctx.Client.Get(ctx, dstKey, &dst)).To(Succeed())

				g.Expect(dst.OwnerReferences).To(Equal([]metav1.OwnerReference{
					{
						APIVersion:         spqv1.GroupVersion.String(),
						Kind:               "StoragePolicyQuota",
						Name:               src.Name,
						UID:                src.UID,
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					},
				}))
				g.Expect(dst.Spec.StoragePolicyId).To(Equal(src.Spec.StoragePolicyId))
				g.Expect(dst.Spec.StorageClassName).To(Equal(src.Name))
				g.Expect(dst.Spec.ResourceAPIgroup).To(Equal(ptr.To(spqv1.GroupVersion.Group)))
				g.Expect(dst.Spec.ResourceKind).To(Equal("StoragePolicyQuota"))
				g.Expect(dst.Spec.ResourceExtensionName).To(Equal("vmservice.cns.vsphere.vmware.com"))
			}).Should(Succeed())
		})
	})
}
