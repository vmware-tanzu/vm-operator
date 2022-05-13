// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

func uniTests() {
	Describe("Invoking Mutate", unitTestsMutating)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vm *vmopv1.VirtualMachine
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vm := builder.DummyVirtualMachine()
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vm:                                vm,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})

	AfterEach(func() {
		ctx = nil
	})

	Describe("VirtualMachineMutator should admit updates when object is under deletion", func() {
		Context("when update request comes in while deletion in progress ", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Describe("AddDefaultNetworkInterface", func() {
		BeforeEach(func() {
			Expect(os.Setenv(mutation.NetworkProviderKey, mutation.VDSTYPE)).Should(Succeed())
		})

		AfterEach(func() {
			Expect(os.Unsetenv(mutation.NetworkProviderKey)).Should(Succeed())
		})

		Context("When VM NetworkInterface is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{}
			})

			When("VDS network", func() {
				It("Should add default network interface with type vsphere-distributed", func() {
					Expect(mutation.AddDefaultNetworkInterface(ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkType).Should(Equal("vsphere-distributed"))
				})
			})

			When("NSX-T network", func() {
				It("Should add default network interface with type NSX-T", func() {
					Expect(os.Setenv(mutation.NetworkProviderKey, mutation.NSXTYPE)).Should(Succeed())

					Expect(mutation.AddDefaultNetworkInterface(ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkType).Should(Equal("nsx-t"))
				})
			})

			When("NoNetwork annotation is set", func() {
				It("Should not add default network interface", func() {
					ctx.vm.Annotations[vmopv1.NoDefaultNicAnnotation] = "true"
					oldVM := ctx.vm.DeepCopy()
					Expect(mutation.AddDefaultNetworkInterface(ctx.vm)).To(BeFalse())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(Equal(oldVM.Spec.NetworkInterfaces))
				})
			})
		})

		Context("VM NetworkInterface is not empty", func() {
			It("Should not add default network interface", func() {
				oldVM := ctx.vm.DeepCopy()
				Expect(mutation.AddDefaultNetworkInterface(ctx.vm)).To(BeFalse())
				Expect(ctx.vm.Spec.NetworkInterfaces).Should(Equal(oldVM.Spec.NetworkInterfaces))
			})
		})
	})
}
