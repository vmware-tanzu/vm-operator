// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1/mutation"
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

	Describe("Handling Spec.VmMetadata.SecretName", func() {

		var (
			secretNameInWebhookContext string
			response                   admission.Response
		)

		BeforeEach(func() {
			ctx.vm.Spec.VmMetadata.ConfigMapName = ""
		})
		JustBeforeEach(func() {
			ctx.WebhookRequestContext.Op = admissionv1.Update
			Expect(ctx.vm.Name).To(BeEmpty())
			ctx.vm.CreationTimestamp = metav1.Now()
			Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())

			// Assert the name was generated.
			Expect(ctx.vm.Name).ToNot(BeEmpty())

			// Update the webhook context with the object.
			obj, err := builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).ToNot(BeNil())
			ctx.WebhookRequestContext.Obj = obj
			ctx.WebhookRequestContext.OldObj = obj.DeepCopy()

			Expect(unstructured.SetNestedField(
				ctx.WebhookRequestContext.Obj.Object,
				secretNameInWebhookContext,
				"spec", "vmMetadata", "secretName",
			)).To(Succeed())

			response = ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
		When("it is empty", func() {
			BeforeEach(func() {
				secretNameInWebhookContext = "my-secret"
			})
			Specify("a request to set it to non-zero value should be allowed", func() {
				Expect(response.PatchType).To(BeNil())
				Expect(response.Patches).To(BeEmpty())
			})
		})
		When("it is non-empty", func() {
			BeforeEach(func() {
				secretNameInWebhookContext = ""
				ctx.vm.Spec.VmMetadata.SecretName = "my-secret"
			})
			Specify("a request to set it to zero-length string should be mutated with the current value", func() {
				Expect(response.PatchType).ToNot(BeNil())
				Expect(string(*response.PatchType)).To(Equal("JSONPatch"))
				Expect(response.Patches).To(Equal([]jsonpatch.Operation{
					{
						Operation: "add",
						Path:      "/spec/vmMetadata/secretName",
						Value:     "my-secret",
					},
				}))
			})
		})
	})

	Describe("AddDefaultNetworkInterface", func() {
		BeforeEach(func() {
			Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeVDS)).Should(Succeed())
		})

		AfterEach(func() {
			Expect(os.Unsetenv(lib.NetworkProviderType)).Should(Succeed())
		})

		Context("When VM NetworkInterface is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{}
			})

			AfterEach(func() {
				Expect(os.Unsetenv(lib.NetworkProviderType)).To(Succeed())
			})

			When("VDS network", func() {
				It("Should add default network interface with type vsphere-distributed", func() {
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkType).Should(Equal("vsphere-distributed"))
				})
			})

			When("NSX-T network", func() {
				It("Should add default network interface with type NSX-T", func() {
					Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeNSXT)).Should(Succeed())

					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkType).Should(Equal("nsx-t"))
				})
			})

			When("Named network", func() {
				var networkName string

				BeforeEach(func() {
					networkName = "VM Network"
					Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeNamed)).Should(Succeed())
				})

				AfterEach(func() {
					networkName = ""
					Expect(os.Unsetenv(lib.NetworkProviderType)).Should(Succeed())
				})

				It("Should add default network interface with name set in the configMap Network", func() {
					configMap := corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "vmware-system-vmop",
							Name:      config.ProviderConfigMapName,
						},
						Data: map[string]string{"Network": networkName},
					}

					Expect(ctx.Client.Create(ctx, &configMap)).To(Succeed())
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(HaveLen(1))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkType).Should(Equal(""))
					Expect(ctx.vm.Spec.NetworkInterfaces[0].NetworkName).Should(Equal(networkName))
				})
			})

			When("NoNetwork annotation is set", func() {
				It("Should not add default network interface", func() {
					ctx.vm.Annotations[vmopv1.NoDefaultNicAnnotation] = "true"
					oldVM := ctx.vm.DeepCopy()
					Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
					Expect(ctx.vm.Spec.NetworkInterfaces).Should(Equal(oldVM.Spec.NetworkInterfaces))
				})
			})
		})

		Context("VM NetworkInterface is not empty", func() {
			It("Should not add default network interface", func() {
				oldVM := ctx.vm.DeepCopy()
				Expect(mutation.AddDefaultNetworkInterface(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
				Expect(ctx.vm.Spec.NetworkInterfaces).Should(Equal(oldVM.Spec.NetworkInterfaces))
			})
		})
	})

	Describe("SetDefaultPowerState", func() {

		Context("When VM PowerState is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = ""
			})

			It("Should set PowerState to PoweredOn", func() {
				Expect(mutation.SetDefaultPowerState(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeTrue())
				Expect(ctx.vm.Spec.PowerState).Should(Equal(vmopv1.VirtualMachinePoweredOn))
			})

		})

		Context("When VM PowerState is not empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
			})

			It("Should not mutate PowerState", func() {
				oldVM := ctx.vm.DeepCopy()
				Expect(mutation.SetDefaultPowerState(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)).To(BeFalse())
				Expect(ctx.vm.Spec.PowerState).Should(Equal(oldVM.Spec.PowerState))
			})
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
					"status.imageName",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.VirtualMachineImage)
						return []string{image.Status.ImageName}
					}).
				WithIndex(&vmopv1.ClusterVirtualMachineImage{},
					"status.imageName",
					func(rawObj client.Object) []string {
						image := rawObj.(*vmopv1.ClusterVirtualMachineImage)
						return []string{image.Status.ImageName}
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
				vmi1 := builder.DummyVirtualMachineImage("vmi-1")
				vmi1.Status.ImageName = dupImageStatusName
				vmi2 := builder.DummyVirtualMachineImage("vmi-2")
				vmi2.Status.ImageName = dupImageStatusName
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
				cvmi1 := builder.DummyClusterVirtualMachineImage("cvmi-1")
				cvmi1.Status.ImageName = dupImageStatusName
				cvmi2 := builder.DummyClusterVirtualMachineImage("cvmi-2")
				cvmi2.Status.ImageName = dupImageStatusName
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
				vmi := builder.DummyVirtualMachineImage("vmi-123")
				vmi.Status.ImageName = dupImageStatusName
				cvmi := builder.DummyClusterVirtualMachineImage("cvmi-123")
				cvmi.Status.ImageName = dupImageStatusName
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
				vmi := builder.DummyVirtualMachineImage("vmi-123")
				vmi.Status.ImageName = uniqueImageStatusName
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
				cvmi := builder.DummyClusterVirtualMachineImage("vmi-123")
				cvmi.Status.ImageName = uniqueImageStatusName
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
						ctx.Client,
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
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
					})
					It("should mutate the field to a valid UTC timestamp", func() {
						for _, s := range []string{"now", "Now", "NOW"} {
							ctx.vm.Spec.NextRestartTime = s
							ok, err := mutation.SetNextRestartTime(
								&ctx.WebhookRequestContext,
								ctx.Client,
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
						oldVM.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.Client,
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
						oldVM.Spec.PowerState = vmopv1.VirtualMachineSuspended
					})
					It("should return an error", func() {
						ctx.vm.Spec.NextRestartTime = "now"
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.Client,
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
				append([]any{
					func(nextRestartTime string) {
						ctx.vm.Spec.NextRestartTime = nextRestartTime
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.Client,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							nextRestartTime,
							`may only be set to "now"`).Error()))
						Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
					}},
					newInvalidNextRestartTimeTableEntries("should return an invalid field error"))...,
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
						ctx.Client,
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
							ctx.Client,
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
				append([]any{
					func(nextRestartTime string) {
						ctx.vm.Spec.NextRestartTime = nextRestartTime
						ok, err := mutation.SetNextRestartTime(
							&ctx.WebhookRequestContext,
							ctx.Client,
							ctx.vm,
							oldVM)
						Expect(ok).To(BeFalse())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal(field.Invalid(
							field.NewPath("spec", "nextRestartTime"),
							nextRestartTime,
							`may only be set to "now"`).Error()))
						Expect(ctx.vm.Spec.NextRestartTime).To(Equal(nextRestartTime))
					}},
					newInvalidNextRestartTimeTableEntries("should return an invalid field error"))...,
			)
		})
	})
}
