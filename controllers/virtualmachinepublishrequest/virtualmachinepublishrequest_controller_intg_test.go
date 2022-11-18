// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking VirtualMachinePublishRequest controller tests", virtualMachinePublishRequestReconcile)
}

func virtualMachinePublishRequestReconcile() {
	var (
		ctx   *builder.IntegrationTestContext
		vmpub *vmopv1alpha1.VirtualMachinePublishRequest
		vm    *vmopv1alpha1.VirtualMachine
		cl    *imgregv1a1.ContentLibrary
	)

	getVirtualMachinePublishRequest := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1alpha1.VirtualMachinePublishRequest {
		vmpub := &vmopv1alpha1.VirtualMachinePublishRequest{}
		if err := ctx.Client.Get(ctx, objKey, vmpub); err != nil {
			return nil
		}
		return vmpub
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				ClassName:  "dummy-class",
				PowerState: vmopv1alpha1.VirtualMachinePoweredOn,
			},
		}

		vmpub = &vmopv1alpha1.VirtualMachinePublishRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vmpub",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachinePublishRequestSpec{
				Source: vmopv1alpha1.VirtualMachinePublishRequestSource{
					Name:       vm.Name,
					Kind:       vm.Kind,
					APIVersion: vm.APIVersion,
				},
				Target: vmopv1alpha1.VirtualMachinePublishRequestTarget{
					Item: vmopv1alpha1.VirtualMachinePublishRequestTargetItem{
						Name: "dummy-item",
					},
					Location: vmopv1alpha1.VirtualMachinePublishRequestTargetLocation{
						Name: "dummy-cl",
					},
				},
			},
		}

		cl = &imgregv1a1.ContentLibrary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-cl",
				Namespace: ctx.Namespace,
			},
			Spec: imgregv1a1.ContentLibrarySpec{
				UUID: "dummy-cl",
			},
		}

		intgFakeVMProvider.Reset()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			Expect(ctx.Client.Create(ctx, cl)).To(Succeed())
			Expect(ctx.Client.Create(ctx, vmpub)).To(Succeed())
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmpub)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, cl)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("resource successfully created", func() {
			Eventually(func() bool {
				obj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
				if obj != nil && obj.IsTargetValid() && obj.IsSourceValid() {
					return true
				}
				return false
			}).Should(BeTrue())
		})
	})
}
