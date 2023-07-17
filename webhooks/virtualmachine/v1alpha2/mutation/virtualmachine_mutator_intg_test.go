// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
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

	ctx.vm = builder.DummyVirtualMachineA2()
	ctx.vm.Namespace = ctx.Namespace

	return ctx
}

func intgTestsMutating() {
	var (
		ctx *intgMutatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
	})

	AfterEach(func() {
		ctx = nil
	})

	Context("SetNextRestartTime", func() {
		When("create a VM", func() {
			When("spec.nextRestartTime is empty", func() {
				It("should not mutate", func() {
					ctx.vm.Spec.NextRestartTime = ""
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
					Expect(modified.Spec.NextRestartTime).Should(BeEmpty())
				})
			})
			When("spec.nextRestartTime is not empty", func() {
				It("should not mutate", func() {
					ctx.vm.Spec.NextRestartTime = "hello"
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
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
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
			})
			When("spec.nextRestartTime is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.NextRestartTime = ""
				})
				When("spec.nextRestartTime is empty", func() {
					It("should not mutate", func() {
						modified.Spec.NextRestartTime = ""
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
						Expect(modified.Spec.NextRestartTime).Should(BeEmpty())
					})
				})
				When("spec.nextRestartTime is now", func() {
					It("should mutate", func() {
						modified.Spec.NextRestartTime = "Now"
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
						Expect(modified.Spec.NextRestartTime).ShouldNot(BeEmpty())
						_, err := time.Parse(time.RFC3339Nano, modified.Spec.NextRestartTime)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})
			When("existing spec.nextRestartTime is not empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.NextRestartTime = lastRestartTime.Format(time.RFC3339Nano)
				})
				When("spec.nextRestartTime is empty", func() {
					It("should mutate to original value", func() {
						modified.Spec.NextRestartTime = ""
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
						Expect(modified.Spec.NextRestartTime).Should(Equal(ctx.vm.Spec.NextRestartTime))
					})
				})
				When("spec.nextRestartTime is now", func() {
					It("should mutate", func() {
						modified.Spec.NextRestartTime = "Now"
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).Should(Succeed())
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
}
