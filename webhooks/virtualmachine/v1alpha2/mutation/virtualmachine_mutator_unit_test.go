// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha2/mutation"
)

func uniTests() {
	Describe("Invoking Mutate", unitTestsMutating)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vm *vmopv1.VirtualMachine
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vm := builder.DummyVirtualMachineA2()
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

	Describe("SetNextRestartTime", func() {

		var (
			oldVM *vmopv1.VirtualMachine
		)

		BeforeEach(func() {
			oldVM = ctx.vm.DeepCopy()
		})

		When("oldVM has empty spec.nextRestartTime", func() {
			BeforeEach(func() {
				oldVM.Spec.NextRestartTime = ""
			})
			Context("newVM has spec.nextRestartTime set to an empty value", func() {
				It("should not mutate anything", func() {
					ctx.vm.Spec.NextRestartTime = ""
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.vm.Spec.NextRestartTime).To(BeEmpty())
				})
			})
			Context("newVM has spec.nextRestartTime set to 'now' (case-insensitive)", func() {
				Context("vm is powered on", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})
					It("should mutate the field to a valid UTC timestamp", func() {
						for _, s := range []string{"now", "Now", "NOW"} {
							ctx.vm.Spec.NextRestartTime = s
							ok, err := mutation.SetNextRestartTime(
								&ctx.WebhookRequestContext,
								ctx.vm,
								oldVM)
							Expect(ok).To(BeTrue())
							Expect(err).ToNot(HaveOccurred())
							Expect(ctx.vm.Spec.NextRestartTime).ToNot(BeEmpty())
							_, err = time.Parse(time.RFC3339Nano, ctx.vm.Spec.NextRestartTime)
							Expect(err).ShouldNot(HaveOccurred())
						}
					})
				})
				Context("vm is powered off", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							"now",
							"can only restart powered on vm").Error()))
					})
				})
				Context("vm is suspended", func() {
					BeforeEach(func() {
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							"now",
							"can only restart powered on vm").Error()))
					})
				})
			})
			DescribeTable(
				`newVM has spec.nextRestartTime set a non-empty value that is not "now"`,
				func(nextRestartTime string) {
					ctx.vm.Spec.NextRestartTime = nextRestartTime
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeFalse())
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(field.Invalid(
						field.NewPath("spec", "nextRestartTime"),
						nextRestartTime,
						`may only be set to "now"`).Error()))
					Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
				},
				newInvalidNextRestartTimeTableEntries("should return an invalid field error")...,
			)
		})

		When("oldVM has non-empty spec.nextRestartTime", func() {
			var (
				lastRestartTime time.Time
			)
			BeforeEach(func() {
				lastRestartTime = time.Now().UTC()
				oldVM.Spec.NextRestartTime = lastRestartTime.Format(time.RFC3339Nano)
			})
			Context("newVM has spec.nextRestartTime set to an empty value", func() {
				It("should mutate to match oldVM", func() {
					ctx.vm.Spec.NextRestartTime = ""
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeTrue())
					Expect(err).ToNot(HaveOccurred())
					Expect(ctx.vm.Spec.NextRestartTime).To(Equal(oldVM.Spec.NextRestartTime))
				})
			})
			Context("newVM has spec.nextRestartTime set to 'now' (case-insensitive)", func() {
				It("should mutate the field to a valid UTC timestamp", func() {
					for _, s := range []string{"now", "Now", "NOW"} {
						ctx.vm.Spec.NextRestartTime = s
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeTrue())
						Expect(err).ToNot(HaveOccurred())
						Expect(ctx.vm.Spec.NextRestartTime).ToNot(BeEmpty())
						nextRestartTime, err := time.Parse(time.RFC3339Nano, ctx.vm.Spec.NextRestartTime)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(lastRestartTime.Before(nextRestartTime)).To(BeTrue())
					}
				})
			})
			DescribeTable(
				`newVM has spec.nextRestartTime set a non-empty value that is not "now"`,
				func(nextRestartTime string) {
					ctx.vm.Spec.NextRestartTime = nextRestartTime
					ok, err := mutation.SetNextRestartTime(
						&ctx.WebhookRequestContext,
						ctx.vm,
						oldVM)
					Expect(ok).To(BeFalse())
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(field.Invalid(
						field.NewPath("spec", "nextRestartTime"),
						nextRestartTime,
						`may only be set to "now"`).Error()))
					Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
				},
				newInvalidNextRestartTimeTableEntries("should return an invalid field error")...,
			)
		})
	})

	Describe(("ResolveImageName"), func() {
		const (
			dupImageStatusName    = "dup-status-name"
			uniqueImageStatusName = "unique-status-name"
		)

		BeforeEach(func() {
			// Replace the client with a fake client that has the index of VM images.
			ctx.Client = fake.NewClientBuilder().WithScheme(builder.NewScheme()).
				WithIndex(
					&vmopv1.VirtualMachineImage{},
					"status.name",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.VirtualMachineImage)
						return []string{image.Status.Name}
					}).
				WithIndex(&vmopv1.ClusterVirtualMachineImage{},
					"status.name",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.ClusterVirtualMachineImage)
						return []string{image.Status.Name}
					}).Build()
			Expect(os.Setenv(lib.VMImageRegistryFSS, lib.TrueString)).To(Succeed())
		})

		AfterEach(func() {
			Expect(os.Unsetenv(lib.VMImageRegistryFSS)).To(Succeed())
		})

		Context("When VM ImageName is set to vmi resource name", func() {

			BeforeEach(func() {
				ctx.vm.Spec.ImageName = "vmi-xxx"
			})

			It("Should not mutate ImageName", func() {
				mutated, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())
				Expect(ctx.vm.Spec.ImageName).Should(Equal("vmi-xxx"))
			})
		})

		Context("When VM ImageName is set to a status name matching multiple namespace scope images", func() {

			BeforeEach(func() {
				vmi1 := builder.DummyVirtualMachineImageA2("vmi-1")
				vmi1.Status.Name = dupImageStatusName
				vmi2 := builder.DummyVirtualMachineImageA2("vmi-2")
				vmi2.Status.Name = dupImageStatusName
				Expect(ctx.Client.Create(ctx, vmi1)).To(Succeed())
				Expect(ctx.Client.Create(ctx, vmi2)).To(Succeed())
				ctx.vm.Spec.ImageName = dupImageStatusName
			})

			It("Should return an error", func() {
				_, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("multiple VM images exist for \"dup-status-name\" in namespace scope"))
			})
		})

		Context("When VM ImageName is set to a status name matching multiple cluster scope images", func() {

			BeforeEach(func() {
				cvmi1 := builder.DummyClusterVirtualMachineImageA2("cvmi-1")
				cvmi1.Status.Name = dupImageStatusName
				cvmi2 := builder.DummyClusterVirtualMachineImageA2("cvmi-2")
				cvmi2.Status.Name = dupImageStatusName
				Expect(ctx.Client.Create(ctx, cvmi1)).To(Succeed())
				Expect(ctx.Client.Create(ctx, cvmi2)).To(Succeed())
				ctx.vm.Spec.ImageName = dupImageStatusName
			})

			It("Should return an error", func() {
				_, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("multiple VM images exist for \"dup-status-name\" in cluster scope"))
			})
		})

		Context("When VM ImageName is set to a status name matching one namespace and one cluster scope images", func() {

			BeforeEach(func() {
				vmi := builder.DummyVirtualMachineImageA2("vmi-123")
				vmi.Status.Name = dupImageStatusName
				cvmi := builder.DummyClusterVirtualMachineImageA2("cvmi-123")
				cvmi.Status.Name = dupImageStatusName
				Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
				Expect(ctx.Client.Create(ctx, cvmi)).To(Succeed())
				ctx.vm.Spec.ImageName = dupImageStatusName
			})

			It("Should return an error", func() {
				_, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("multiple VM images exist for \"dup-status-name\" in namespace and cluster scope"))
			})
		})

		Context("When VM ImageName is set to a status name matching a single namespace scope image", func() {

			BeforeEach(func() {
				vmi := builder.DummyVirtualMachineImageA2("vmi-123")
				vmi.Status.Name = uniqueImageStatusName
				Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
				ctx.vm.Spec.ImageName = uniqueImageStatusName
			})

			It("Should mutate ImageName to the resource name of the namespace scope image", func() {
				mutated, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())
				Expect(ctx.vm.Spec.ImageName).Should(Equal("vmi-123"))
			})
		})

		Context("When VM ImageName is set to a status name matching a single cluster scope image", func() {

			BeforeEach(func() {
				cvmi := builder.DummyClusterVirtualMachineImageA2("vmi-123")
				cvmi.Status.Name = uniqueImageStatusName
				Expect(ctx.Client.Create(ctx, cvmi)).To(Succeed())
				ctx.vm.Spec.ImageName = uniqueImageStatusName
			})

			It("Should mutate ImageName to the resource name of the cluster scope image", func() {
				mutated, err := mutation.ResolveImageName(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())
				Expect(ctx.vm.Spec.ImageName).Should(Equal("vmi-123"))
			})
		})
	})
}
