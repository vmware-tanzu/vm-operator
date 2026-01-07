// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storageclass_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
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
		ctx       *builder.IntegrationTestContext
		obj       storagev1.StorageClass
		profileID string
		polKey    ctrlclient.ObjectKey
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
		obj = storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "my-storage-class-",
			},
			Provisioner: "fake",
			Parameters:  map[string]string{},
		}
	})

	JustBeforeEach(func() {
		obj.Parameters["storagePolicyID"] = profileID
		polKey = ctrlclient.ObjectKey{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      kubeutil.GetStoragePolicyObjectName(profileID),
		}
		Expect(ctx.Client.Create(ctx, &obj)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("reconciling a StorageClass", func() {

		Context("sans policy ID", func() {
			BeforeEach(func() {
				profileID = ""
			})
			It("should consistently not create a StoragePolicy object", func() {
				Consistently(func(g Gomega) {
					g.Expect(ctx.Client.Get(
						ctx,
						polKey,
						&infrav1.StoragePolicy{})).ToNot(Succeed())
				})
			})
		})
		Context("with invalid policy ID", func() {
			BeforeEach(func() {
				profileID = "invalid"
			})
			It("should eventually create a StoragePolicy object", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(
						ctx,
						polKey,
						&infrav1.StoragePolicy{})).To(Succeed())
				})
			})
		})
		Context("with valid policy ID", func() {
			BeforeEach(func() {
				profileID = uuid.NewString()
			})
			It("should eventually create a StoragePolicy object", func() {
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(
						ctx,
						polKey,
						&infrav1.StoragePolicy{})).To(Succeed())
				})
			})
		})
	})
}
