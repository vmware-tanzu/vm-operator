// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storageclass_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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
		ctx *builder.IntegrationTestContext
		obj storagev1.StorageClass
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		obj = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "my-storage-class-",
			},
			Provisioner: "fake",
			Parameters: map[string]string{
				"storagePolicyID": "my-storage-policy",
			},
		}
	})

	JustBeforeEach(func() {
		Expect(ctx.Client.Create(ctx, &obj)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	Context("reconciling an encryption storage class", func() {
		BeforeEach(func() {
			obj.Parameters["storagePolicyID"] = myEncryptedStoragePolicy
		})
		It("should eventually mark the storage class as encrypted in the context", func() {
			Eventually(func(g Gomega) {
				ok, err := kubeutil.IsEncryptedStorageClass(ctx, ctx.Client, obj.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("reconciling a plain storage class", func() {
		It("should never mark the storage class as encrypted in the context", func() {
			Consistently(func(g Gomega) {
				ok, err := kubeutil.IsEncryptedStorageClass(ctx, ctx.Client, obj.Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
	})
}
