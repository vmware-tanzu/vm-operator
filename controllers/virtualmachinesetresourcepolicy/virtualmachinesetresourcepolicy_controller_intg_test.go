// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {

	var (
		ctx *builder.IntegrationTestContext

		resourcePolicy    *vmopv1alpha1.VirtualMachineSetResourcePolicy
		resourcePolicyKey client.ObjectKey
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		resourcePolicy = &vmopv1alpha1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ctx.Namespace,
				Name:      "dummy-vm-policy",
			},
			Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{},
		}

		resourcePolicyKey = client.ObjectKey{Namespace: resourcePolicy.Namespace, Name: resourcePolicy.Name}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		intgFakeVmProvider.Reset()
	})

	getResourcePolicy := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) *vmopv1alpha1.VirtualMachineSetResourcePolicy {
		rp := &vmopv1alpha1.VirtualMachineSetResourcePolicy{}
		if err := ctx.Client.Get(ctx, objKey, rp); err != nil {
			return nil
		}
		return rp
	}

	waitForResourcePolicyFinalizer := func(ctx *builder.IntegrationTestContext, objKey types.NamespacedName) {
		Eventually(func() []string {
			if rp := getResourcePolicy(ctx, objKey); rp != nil {
				return rp.GetFinalizers()
			}
			return nil
		}).Should(ContainElement(finalizer), "waiting for VirtualMachineSetResourcePolicy finalizer")
	}

	Context("Reconcile", func() {

		It("Reconciles after VirtualMachineSetResourcePolicy creation", func() {
			Expect(ctx.Client.Create(ctx, resourcePolicy)).To(Succeed())

			By("VirtualMachineSetResourcePolicy should have a finalizer added", func() {
				waitForResourcePolicyFinalizer(ctx, resourcePolicyKey)
			})

			By("Provider should be able to find the Resource Policy", func() {
				Eventually(func() bool {
					exists, err := intgFakeVmProvider.DoesVirtualMachineSetResourcePolicyExist(ctx, resourcePolicy)
					if err != nil {
						return false
					}
					return exists
				}).Should(BeTrue())
			})

			By("Deleting the ResourcePolicy", func() {
				err := ctx.Client.Delete(ctx, resourcePolicy)
				Expect(err == nil || apiErrors.IsNotFound(err)).To(BeTrue())
			})

		})
	})
}
