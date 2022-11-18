// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

func intgTests() {
	Describe("Invoking Mutation", intgTestsMutating)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vm *vmopv1.VirtualMachine
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vm = builder.DummyVirtualMachine()
	ctx.vm.Namespace = ctx.Namespace

	return ctx
}

func intgTestsMutating() {
	var (
		ctx *intgMutatingWebhookContext
		vm  *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		vm = ctx.vm.DeepCopy()
		vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{}
		Expect(os.Setenv(lib.NetworkProviderType, mutation.VDSTYPE)).Should(Succeed())
	})
	AfterEach(func() {
		ctx = nil
		Expect(os.Unsetenv(lib.NetworkProviderType)).Should(Succeed())
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			BeforeEach(func() {
			})

			AfterEach(func() {
				Expect(ctx.Client.Delete(ctx, vm)).Should(Succeed())
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Default network interface", func() {
			When("Creating VirtualMachine", func() {
				It("Add default network interface if NetworkInterface is empty and no Annotation", func() {
					err := ctx.Client.Create(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
					Expect(modified.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(modified.Spec.NetworkInterfaces[0].NetworkType).Should(Equal(network.VdsNetworkType))
					Expect(modified.Spec.NetworkInterfaces[0].NetworkName).Should(Equal(""))
				})
			})

			When("Updating VirtualMachine", func() {
				BeforeEach(func() {
					vm.Annotations[vmopv1.NoDefaultNicAnnotation] = ""
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				})

				AfterEach(func() {
					Expect(ctx.Client.Delete(ctx, vm)).Should(Succeed())
				})

				It("should not add default network interface", func() {
					modified := &vmopv1.VirtualMachine{}
					vmKey := client.ObjectKeyFromObject(vm)
					Expect(ctx.Client.Get(ctx, vmKey, modified)).Should(Succeed())

					delete(modified.Annotations, vmopv1.NoDefaultNicAnnotation)
					Expect(ctx.Client.Update(ctx, modified)).Should(Succeed())
					Expect(ctx.Client.Get(ctx, vmKey, modified)).Should(Succeed())
					Expect(modified.Spec.NetworkInterfaces).Should(BeEmpty())
				})
			})

		})

	})
}
