// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		intgTestsMutating,
	)
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
		img *vmopv1.VirtualMachineImage
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()

		img = builder.DummyVirtualMachineImage(builder.DummyVMIName)
		img.Namespace = ctx.vm.Namespace
		Expect(ctx.Client.Create(ctx, img)).To(Succeed())
		img.Status.Name = ctx.vm.Spec.ImageName
		Expect(ctx.Client.Status().Update(ctx, img)).To(Succeed())
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, img)).To(Succeed())
		img = nil
		ctx = nil
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			AfterEach(func() {
				Expect(ctx.Client.Delete(ctx, ctx.vm)).To(Succeed())
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Default network interface", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
					config.NetworkProviderType = pkgcfg.NetworkProviderTypeVDS
				})
				ctx.vm.Spec.Network.Interfaces = nil
			})
			AfterEach(func() {
				pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
					config.NetworkProviderType = ""
				})
			})

			When("Creating VirtualMachine", func() {
				It("Add default network interface if NetworkInterface is empty and no Annotation", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())

					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.Network.Interfaces).To(HaveLen(1))
					Expect(modified.Spec.Network.Interfaces[0].Name).To(Equal("eth0"))
					Expect(modified.Spec.Network.Interfaces[0].Network.Kind).To(Equal("Network"))
				})
			})
		})
	})

	Context("SetDefaultInstanceUUID", func() {
		When("Creating VirtualMachine", func() {
			When("When VM InstanceUUID is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.InstanceUUID = ""
				})

				It("Should set InstanceUUID", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())

					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					Expect(vm.Spec.InstanceUUID).ToNot(BeEmpty())
				})
			})
			When("When VM InstanceUUID is not empty", func() {
				var id = uuid.NewString()

				BeforeEach(func() {
					ctx.vm.Spec.InstanceUUID = id
				})

				It("Should not mutate InstanceUUID", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					Expect(vm.Spec.InstanceUUID).To(Equal(id))
				})
			})
		})

		When("Updating VirtualMachine", func() {
			var id = uuid.NewString()

			BeforeEach(func() {
				ctx.vm.Spec.InstanceUUID = id
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("Should not mutate InstanceUUID", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Spec.InstanceUUID).To(Equal(id))
			})
		})
	})

	Context("SetDefaultBiosUUID", func() {
		When("Creating VirtualMachine", func() {
			When("When VM BiosUUID is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.BiosUUID = ""
				})

				It("Should set BiosUUID", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())

					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					Expect(vm.Spec.BiosUUID).ToNot(BeEmpty())
				})
			})
			When("When VM BiosUUID is not empty", func() {
				var id = uuid.NewString()

				BeforeEach(func() {
					ctx.vm.Spec.BiosUUID = id
				})

				It("Should not mutate BiosUUID", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					Expect(vm.Spec.BiosUUID).To(Equal(id))
				})
			})
		})

		When("Updating VirtualMachine", func() {
			var id = uuid.NewString()

			BeforeEach(func() {
				ctx.vm.Spec.BiosUUID = id
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("Should not mutate BiosUUID", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Spec.BiosUUID).To(Equal(id))
			})
		})
	})

	Context("SetDefaultPowerState", func() {
		When("Creating VirtualMachine", func() {
			When("When VM PowerState is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.PowerState = ""
				})

				It("Should set PowerState to PoweredOn", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())

					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				})
			})
			When("When VM PowerState is not empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				})

				It("Should not mutate PowerState", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				})
			})
		})

		When("Updating VirtualMachine", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			// This state is not technically possible in production. However,
			// it is used to validate that the power state is not auto-set
			// to poweredOn if empty during an empty. Since the logic for
			// defaulting to poweredOn only works if empty (and on create),
			// it's necessary to replicate the empty state here.
			When("When VM PowerState is empty", func() {
				It("Should not mutate PowerState", func() {
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					modified.Spec.PowerState = ""
					Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.PowerState).To(BeEmpty())
				})
			})

			When("When VM PowerState is not empty", func() {
				It("Should not mutate PowerState", func() {
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					modified.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				})
			})
		})
	})

	Context("ResolveImageName", func() {

		const (
			vmiKind  = "VirtualMachineImage"
			cvmiKind = "Cluster" + vmiKind
		)

		shouldMutateImageButNotImageName := func() {
			ExpectWithOffset(1, ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			modified := &vmopv1.VirtualMachine{}
			ExpectWithOffset(1, ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
			ExpectWithOffset(1, modified.Spec.Image).ToNot(BeNil())
			ExpectWithOffset(1, modified.Spec.Image.Kind).To(Equal(vmiKind))
			ExpectWithOffset(1, modified.Spec.Image.Name).To(Equal(builder.DummyVMIName))
			ExpectWithOffset(1, modified.Spec.ImageName).To(Equal(ctx.vm.Spec.ImageName))
		}

		shouldNotMutateImageOrImageName := func() {
			ExpectWithOffset(1, ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			modified := &vmopv1.VirtualMachine{}
			ExpectWithOffset(1, ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
			ExpectWithOffset(1, modified.Spec.Image).To(Equal(ctx.vm.Spec.Image))
			ExpectWithOffset(1, modified.Spec.ImageName).To(Equal(ctx.vm.Spec.ImageName))
		}

		When("Creating VirtualMachine", func() {
			When("spec.image is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Image = nil
				})
				When("spec.imageName is vmi", func() {
					BeforeEach(func() {
						ctx.vm.Spec.ImageName = builder.DummyVMIName
					})
					It("Should mutate Image but not ImageName", func() {
						shouldMutateImageButNotImageName()
					})
				})
				When("spec.imageName is display name", func() {
					BeforeEach(func() {
						ctx.vm.Spec.ImageName = img.Status.Name
					})
					It("Should mutate Image but not ImageName", func() {
						shouldMutateImageButNotImageName()
					})
				})
				When("spec.imageName is empty", func() {
					BeforeEach(func() {
						ctx.vm.Spec.ImageName = ""
					})
					It("Should not mutate Image or ImageName", func() {
						shouldNotMutateImageOrImageName()
					})
				})
			})

			When("spec.image is non-empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Name: builder.DummyVMIName,
						Kind: vmiKind,
					}
				})
				It("Should mutate Image but not ImageName", func() {
					shouldNotMutateImageOrImageName()
				})
			})

		})
	})

	Context("SetNextRestartTime", func() {
		When("create a VM", func() {
			When("spec.nextRestartTime is empty", func() {
				It("should not mutate", func() {
					ctx.vm.Spec.NextRestartTime = ""
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.NextRestartTime).To(BeEmpty())
				})
			})
			When("spec.nextRestartTime is not empty", func() {
				It("should not mutate", func() {
					ctx.vm.Spec.NextRestartTime = "hello"
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					modified := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
					Expect(modified.Spec.NextRestartTime).To(Equal("hello"))
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
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
			})
			When("spec.nextRestartTime is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.NextRestartTime = ""
				})
				When("spec.nextRestartTime is empty", func() {
					It("should not mutate", func() {
						modified.Spec.NextRestartTime = ""
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
						Expect(modified.Spec.NextRestartTime).To(BeEmpty())
					})
				})
				When("spec.nextRestartTime is now", func() {
					It("should mutate", func() {
						modified.Spec.NextRestartTime = "Now"
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
						Expect(modified.Spec.NextRestartTime).ToNot(BeEmpty())
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
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
						Expect(modified.Spec.NextRestartTime).To(Equal(ctx.vm.Spec.NextRestartTime))
					})
				})
				When("spec.nextRestartTime is now", func() {
					It("should mutate", func() {
						modified.Spec.NextRestartTime = "Now"
						Expect(ctx.Client.Update(ctx, modified)).To(Succeed())
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), modified)).To(Succeed())
						Expect(modified.Spec.NextRestartTime).ToNot(BeEmpty())
						nextRestartTime, err := time.Parse(time.RFC3339Nano, modified.Spec.NextRestartTime)
						Expect(err).ToNot(HaveOccurred())
						Expect(lastRestartTime.Before(nextRestartTime)).To(BeTrue())
					})
				})
				DescribeTable(
					`spec.nextRestartTime is a non-empty value that is not "now"`,
					append([]any{
						func(nextRestartTime string) {
							modified.Spec.NextRestartTime = nextRestartTime
							err := ctx.Client.Update(ctx, modified)
							Expect(err).To(HaveOccurred())
							expectedErr := field.Invalid(
								field.NewPath("spec", "nextRestartTime"),
								nextRestartTime,
								`may only be set to "now"`)
							Expect(err.Error()).To(ContainSubstring(expectedErr.Error()))
						}},
						newInvalidNextRestartTimeTableEntries("should return an invalid field error"))...,
				)
			})
		})
	})

	Context("SetCreatedAtAnnotations", func() {
		When("creating a VM", func() {
			It("should add the created-at annotations", func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))
			})
		})

		When("updating a VM", func() {
			var oldBuildVersion string
			BeforeEach(func() {
				oldBuildVersion = pkg.BuildVersion
			})
			AfterEach(func() {
				pkg.BuildVersion = oldBuildVersion
			})
			It("should not update the created-at annotations", func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))

				pkg.BuildVersion = "v2"

				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtBuildVersionAnnotationKey, "v1"))
				Expect(vm.Annotations).To(HaveKeyWithValue(constants.CreatedAtSchemaVersionAnnotationKey, vmopv1.GroupVersion.Version))
			})
		})
	})

	Context("CD-ROM", func() {

		When("creating a VM", func() {

			When("spec.cdrom.image.kind is empty", func() {

				BeforeEach(func() {
					for i, c := range ctx.vm.Spec.Cdrom {
						c.Image.Kind = ""
						ctx.vm.Spec.Cdrom[i] = c
					}
				})

				It("should set default kind to VirtualMachineImage", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					for _, c := range vm.Spec.Cdrom {
						Expect(c.Image.Kind).To(Equal("VirtualMachineImage"))
					}
				})
			})

			When("spec.imageName is empty", func() {

				BeforeEach(func() {
					ctx.vm.Spec.ImageName = ""
				})

				It("should set the spec.imageName to the first connected CD-ROM image name", func() {
					Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					Expect(vm.Spec.ImageName).To(Equal(ctx.vm.Spec.Cdrom[0].Image.Name))
				})
			})
		})

		When("updating a VM", func() {

			var imgNameToOldKind map[string]string

			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
				imgNameToOldKind = make(map[string]string, len(ctx.vm.Spec.Cdrom))
			})

			When("spec.cdrom.image.kind is reset", func() {

				BeforeEach(func() {
					for i, c := range ctx.vm.Spec.Cdrom {
						imgNameToOldKind[c.Image.Name] = c.Image.Kind
						ctx.vm.Spec.Cdrom[i].Image.Kind = ""
					}
				})

				It("should repopulate the default kind only if it was previously set to default", func() {
					Expect(ctx.Client.Update(ctx, ctx.vm)).To(Succeed())
					vm := &vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
					defaultKind := "VirtualMachineImage"
					for _, c := range vm.Spec.Cdrom {
						if imgNameToOldKind[c.Image.Name] == defaultKind {
							Expect(c.Image.Kind).To(Equal(defaultKind))
						} else {
							Expect(c.Image.Kind).To(BeEmpty())
						}
					}
				})
			})
		})
	})

	Context("Crypto", func() {
		const (
			fakeString    = "fake"
			encClassName  = "my-encryption-class"
			keyProviderID = "my-key-provider-id"
			keyID         = "my-key-id"
			storClassName = "my-storage-class"
		)

		var (
			encClass  byokv1.EncryptionClass
			storClass storagev1.StorageClass
		)

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.PodNamespace = pkgcfg.FromContext(suite.Context).PodNamespace
			})

			encClass = byokv1.EncryptionClass{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ctx.Namespace,
					Name:      encClassName,
					Labels: map[string]string{
						kubeutil.DefaultEncryptionClassLabelName: kubeutil.DefaultEncryptionClassLabelValue,
					},
				},
				Spec: byokv1.EncryptionClassSpec{
					KeyProvider: keyProviderID,
					KeyID:       keyID,
				},
			}

			storClass = storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: storClassName,
				},
				Provisioner: fakeString,
			}
			kubeutil.SetStoragePolicyID(&storClass, fakeString)

			ctx.vm.Spec.StorageClass = storClass.Name

			Expect(ctx.Client.Create(ctx, &encClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, &storClass)).To(Succeed())
			Expect(kubeutil.MarkEncryptedStorageClass(
				ctx,
				ctx.Client,
				storClass,
				true)).To(Succeed())
		})

		AfterEach(func() {
			Expect(kubeutil.MarkEncryptedStorageClass(
				ctx,
				ctx.Client,
				storClass,
				false)).To(Succeed())
			Expect(ctx.Client.Delete(ctx, &storClass)).To(Succeed())
		})

		When("creating a VM", func() {
			JustBeforeEach(func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			When("spec.crypto is nil", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = nil
				})
				When("spec.storageClass is empty", func() {
					BeforeEach(func() {
						ctx.vm.Spec.StorageClass = ""
					})
					It("should not modify spec.crypto", func() {
						Expect(ctx.vm.Spec.Crypto).To(BeNil())
					})
				})
				When("spec.storageClass does not support encryption", func() {
					BeforeEach(func() {
						Expect(kubeutil.MarkEncryptedStorageClass(
							ctx,
							ctx.Client,
							storClass,
							false)).To(Succeed())
					})
					It("should not modify spec.crypto", func() {
						Expect(ctx.vm.Spec.Crypto).To(BeNil())
					})
				})
				When("there is no default EncryptionClass", func() {
					BeforeEach(func() {
						encClass.Labels = map[string]string{}
						Expect(ctx.Client.Update(ctx, &encClass)).To(Succeed())
					})
					It("should not modify spec.crypto", func() {
						Expect(ctx.vm.Spec.Crypto).To(BeNil())
					})
				})
				When("there is a default EncryptionClass", func() {
					It("should set spec.crypto.encryptionClassName", func() {
						Expect(ctx.vm.Spec.Crypto).ToNot(BeNil())
						Expect(ctx.vm.Spec.Crypto.EncryptionClassName).To(Equal(encClass.Name))
					})
				})
			})
			When("spec.crypto.encryptionClassName is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}
				})
				When("there is a default EncryptionClass", func() {
					It("should set spec.crypto.encryptionClassName", func() {
						Expect(ctx.vm.Spec.Crypto).ToNot(BeNil())
						Expect(ctx.vm.Spec.Crypto.EncryptionClassName).To(Equal(encClass.Name))
					})
				})
			})
			When("spec.crypto.encryptionClassName is not empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
						EncryptionClassName: fakeString,
					}
				})
				When("there is a default EncryptionClass", func() {
					It("should not modify spec.crypto.encryptionClassName", func() {
						Expect(ctx.vm.Spec.Crypto).ToNot(BeNil())
						Expect(ctx.vm.Spec.Crypto.EncryptionClassName).To(Equal(fakeString))
					})
				})
			})
		})
		When("updating a VM", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})
			JustBeforeEach(func() {
				Expect(ctx.Client.Update(ctx, ctx.vm)).To(Succeed())
			})

			When("spec.crypto is nil", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = nil
				})
				When("there is a default EncryptionClass", func() {
					It("should not modify spec.crypto", func() {
						Expect(ctx.vm.Spec.Crypto).To(BeNil())
					})
				})
			})
			When("spec.crypto.encryptionClassName is empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}
				})
				When("there is a default EncryptionClass", func() {
					It("should not modify spec.crypto.encryptionClassName", func() {
						Expect(ctx.vm.Spec.Crypto).ToNot(BeNil())
						Expect(ctx.vm.Spec.Crypto.EncryptionClassName).To(BeEmpty())
					})
				})
			})
			When("spec.crypto.encryptionClassName is not empty", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
						EncryptionClassName: fakeString,
					}
				})
				When("there is a default EncryptionClass", func() {
					It("should not modify spec.crypto.encryptionClassName", func() {
						Expect(ctx.vm.Spec.Crypto).ToNot(BeNil())
						Expect(ctx.vm.Spec.Crypto.EncryptionClassName).To(Equal(fakeString))
					})
				})
			})
		})
	})

	Context("CleanupApplyPowerStateChangeTimeAnno", func() {

		When("VM has a apply power state change time annotation set", func() {
			BeforeEach(func() {
				timestamp := time.Now().UTC().Format(time.RFC3339Nano)
				ctx.vm.Annotations[constants.ApplyPowerStateTimeAnnotation] = timestamp
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
			})

			It("should remove the annotation if the VM's power state is changed", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), vm)).To(Succeed())
				Expect(vm.Annotations).To(HaveKey(constants.ApplyPowerStateTimeAnnotation))
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
				updated := &vmopv1.VirtualMachine{}
				Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(ctx.vm), updated)).To(Succeed())
				Expect(updated.Annotations).ToNot(HaveKey(constants.ApplyPowerStateTimeAnnotation))
			})
		})
	})
}
