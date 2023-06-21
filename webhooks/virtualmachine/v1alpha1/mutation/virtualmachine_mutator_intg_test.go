// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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
		Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeVDS)).Should(Succeed())
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

		Context("SetDefaultPowerState", func() {
			When("Creating VirtualMachine", func() {
				When("When VM PowerState is empty", func() {
					BeforeEach(func() {
						vm.Spec.PowerState = ""
					})
					It("Should set PowerState to PoweredOn", func() {
						Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.PowerState).Should(Equal(vmopv1.VirtualMachinePoweredOn))
					})
				})
				When("When VM PowerState is not empty", func() {
					BeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
					})
					It("Should not mutate PowerState", func() {
						Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.PowerState).Should(Equal(vmopv1.VirtualMachinePoweredOff))
					})
				})
			})

			When("Updating VirtualMachine", func() {
				BeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
				})
				// This state is not technically possible in production. However,
				// it is used to validate that the power state is not auto-set
				// to poweredOn if empty during an empty. Since the logic for
				// defaulting to poweredOn only works if empty (and on create),
				// it's necessary to replicate the empty state here.
				When("When VM PowerState is empty", func() {
					It("Should not mutate PowerState", func() {
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						modified.Spec.PowerState = ""
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.PowerState).Should(BeEmpty())
					})
				})
				When("When VM PowerState is not empty", func() {
					It("Should not mutate PowerState", func() {
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						modified.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.PowerState).Should(Equal(vmopv1.VirtualMachinePoweredOff))
					})
				})
			})

		})

		Context("SetNextRestartTime", func() {
			When("create a VM", func() {
				When("spec.nextRestartTime is empty", func() {
					It("should not mutate", func() {
						vm.Spec.NextRestartTime = ""
						Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.NextRestartTime).Should(BeEmpty())
					})
				})
				When("spec.nextRestartTime is not empty", func() {
					It("should not mutate", func() {
						vm.Spec.NextRestartTime = "hello"
						Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
						modified := &vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
						Expect(modified.Spec.NextRestartTime).Should(Equal("hello"))
					})
				})
			})

			When("updating VirtualMachine", func() {
				var (
					lastRestartTime time.Time
					modified        *vmopv1.VirtualMachine
				)
				BeforeEach(func() {
					lastRestartTime = time.Now().UTC()
					modified = &vmopv1.VirtualMachine{}
				})
				JustBeforeEach(func() {
					Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
				})
				When("spec.nextRestartTime is empty", func() {
					BeforeEach(func() {
						vm.Spec.NextRestartTime = ""
					})
					When("spec.nextRestartTime is empty", func() {
						It("should not mutate", func() {
							modified.Spec.NextRestartTime = ""
							Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
							Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
							Expect(modified.Spec.NextRestartTime).Should(BeEmpty())
						})
					})
					When("spec.nextRestartTime is now", func() {
						It("should mutate", func() {
							modified.Spec.NextRestartTime = "Now"
							Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
							Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
							Expect(modified.Spec.NextRestartTime).ShouldNot(BeEmpty())
							_, err := time.Parse(time.RFC3339Nano, modified.Spec.NextRestartTime)
							Expect(err).ToNot(HaveOccurred())
						})
					})
				})
				When("existing spec.nextRestartTime is not empty", func() {
					BeforeEach(func() {
						vm.Spec.NextRestartTime = lastRestartTime.Format(time.RFC3339Nano)
					})
					When("spec.nextRestartTime is empty", func() {
						It("should mutate to original value", func() {
							modified.Spec.NextRestartTime = ""
							Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
							Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
							Expect(modified.Spec.NextRestartTime).Should(Equal(vm.Spec.NextRestartTime))
						})
					})
					When("spec.nextRestartTime is now", func() {
						It("should mutate", func() {
							modified.Spec.NextRestartTime = "Now"
							Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
							Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), modified)).Should(Succeed())
							Expect(modified.Spec.NextRestartTime).ShouldNot(BeEmpty())
							nextRestartTime, err := time.Parse(time.RFC3339Nano, modified.Spec.NextRestartTime)
							Expect(err).ToNot(HaveOccurred())
							Expect(lastRestartTime.Before(nextRestartTime)).To(BeTrue())
						})
					})
					DescribeTable(
						`spec.nextRestartTime is a non-empty value that is not "now"`,
						func(nextRestartTime string) {
							modified.Spec.NextRestartTime = nextRestartTime
							err := ctx.Client.Update(ctx, modified)
							Expect(err).To(HaveOccurred())
							expectedErr := field.Invalid(
								field.NewPath("spec", "nextRestartTime"),
								nextRestartTime,
								`may only be set to "now"`)
							Expect(err.Error()).To(ContainSubstring(expectedErr.Error()))
						},
						newInvalidNextRestartTimeTableEntries("should return an invalid field error")...,
					)
				})
			})
		})

	})
}
