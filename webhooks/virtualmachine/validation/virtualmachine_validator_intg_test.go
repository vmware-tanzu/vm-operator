// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

func intgTests() {
	Describe("Invoking Create", intgTestsValidateCreate)
	Describe("Invoking Update", intgTestsValidateUpdate)
	Describe("Invoking Delete", intgTestsValidateDelete)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vm      *vmopv1.VirtualMachine
	vmImage *vmopv1.VirtualMachineImage
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vm = builder.DummyVirtualMachine()
	ctx.vmImage = builder.DummyVirtualMachineImage(ctx.vm.Spec.ImageName)
	ctx.vm.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		invalidImageName                bool
		imageNotFound                   bool
		validGuestOSType                bool
		invalidGuestOSType              bool
		imageSupportCheckSkipAnnotation bool
		invalidMetadataConfigMap        bool
		invalidStorageClass             bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		vm := ctx.vm.DeepCopy()

		if args.invalidImageName {
			vm.Spec.ImageName = ""
		}

		// Delete image before VM create
		if args.imageNotFound {
			err := ctx.Client.Delete(ctx, ctx.vmImage)
			Expect(err).NotTo(HaveOccurred())
		}

		// Setting the annotation skips Image compatibility validation
		// works with validGuestOSType or invalidGuestOSType or v1alpha1 non-compatible images
		if args.imageSupportCheckSkipAnnotation {
			vm.Annotations = make(map[string]string)
			vm.Annotations[vsphere.VMOperatorImageSupportedCheckKey] = vsphere.VMOperatorImageSupportedCheckDisable
		}

		if args.validGuestOSType {
			ctx.vmImage.Status.ImageSupported = &[]bool{true}[0]
			err := ctx.Client.Status().Update(ctx, ctx.vmImage)
			Expect(err).ToNot(HaveOccurred())
		}
		if args.invalidGuestOSType {
			ctx.vmImage.Status.ImageSupported = &[]bool{false}[0]
			err := ctx.Client.Status().Update(ctx, ctx.vmImage)
			Expect(err).ToNot(HaveOccurred())
		}
		if args.invalidMetadataConfigMap {
			vm.Spec.VmMetadata.ConfigMapName = ""
		}
		// StorageClass specifies but not assigned to ResourceQuota
		if args.invalidStorageClass {
			vm.Spec.StorageClass = builder.DummyStorageClassName
			storageClass := builder.DummyStorageClass()
			rlName := "blah.storageclass.storage.k8s.io/persistentvolumeclaims"
			resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
		}

		err := ctx.Client.Create(ctx, vm)
		if expectedAllowed {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
		if expectedReason != "" {
			Expect(err.Error()).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
	})
	AfterEach(func() {
		_ = ctx.Client.Delete(ctx, ctx.vmImage)
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("should work", createArgs{}, true, "", nil),
		Entry("should work for image with valid osType", createArgs{validGuestOSType: true}, true, "", nil),
		Entry("should work despite osType when VMOperatorImageSupportedCheckKey is disabled", createArgs{imageSupportCheckSkipAnnotation: true, invalidGuestOSType: true}, true, "", nil),
		Entry("should not work for invalid image name", createArgs{invalidImageName: true}, false, "spec.imageName must be specified", nil),
		Entry("should not work for image with an invalid osType or invalid image", createArgs{invalidGuestOSType: true}, false, fmt.Sprintf(messages.VMImageNotSupported, builder.DummyOSType, builder.DummyImageName), nil),
		Entry("should not work for invalid metadata configmapname", createArgs{invalidMetadataConfigMap: true}, false, "spec.vmMetadata.configMapName must be specified", nil),
		Entry("should not work for invalid storage class", createArgs{invalidStorageClass: true}, false, fmt.Sprintf(messages.StorageClassNotAssigned, builder.DummyStorageClassName, ""), nil),
	)
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		//Create the VM
		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vm)
	})
	AfterEach(func() {
		err := ctx.Client.Delete(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		ctx = nil
	})

	When("update is performed with changed image name", func() {
		BeforeEach(func() {
			ctx.vm.Spec.ImageName += "-2"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updates to immutable fields are not allowed: [spec.imageName]"))
		})
	})
	When("update is performed with changed storageClass name", func() {
		BeforeEach(func() {
			ctx.vm.Spec.StorageClass += "-2"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updates to immutable fields are not allowed: [spec.storageClass]"))
		})
	})

	Context("VirtualMachine update while VM is powered on", func() {
		BeforeEach(func() {
			ctx.vm.Spec.PowerState = "poweredOn"
		})

		When("Ports are updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Ports = []vmopv1.VirtualMachinePort{{
					Name: "updated-port",
				}}
			})
			It("rejects the request", func() {
				fields := []string{"spec.ports"}
				expectedReason := fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerState, fields, ctx.vm.Spec.PowerState)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("VmMetadata is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					ConfigMapName: "updated-configmap",
				}
			})

			It("rejects the request", func() {
				fields := []string{"spec.vmMetadata"}
				expectedReason := fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerState, fields, ctx.vm.Spec.PowerState)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("NetworkInterfaces are updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{{
					NetworkName: "updated-network",
				}}
			})

			It("rejects the request", func() {
				fields := []string{"spec.networkInterfaces"}
				expectedReason := fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerState, fields, ctx.vm.Spec.PowerState)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("Volumes are updated", func() {
			When("a vSphere volume is added", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name:          "updated-vsphere-volume",
							VsphereVolume: &vmopv1.VsphereVolumeSource{},
						},
					)
				})

				It("rejects the request", func() {
					fields := []string{"spec.volumes[VsphereVolume]"}
					expectedReason := fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerState, fields, ctx.vm.Spec.PowerState)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedReason))
				})
			})

			When("a PV is added", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name: "dummy-new-pv-volume",
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "dummy-new-claim-name",
							},
						},
					)
				})

				It("does not reject the request", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		When("AdvancedOptions VolumeProvisioningOptions are updated", func() {
			BeforeEach(func() {
				thinProvisioning := true
				ctx.vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
						ThinProvisioned: &thinProvisioning,
					},
				}
			})

			It("rejects the request", func() {
				fields := []string{"spec.advancedOptions.defaultVolumeProvisioningOptions"}
				expectedReason := fmt.Sprintf(messages.UpdatingFieldsNotAllowedInPowerState, fields, ctx.vm.Spec.PowerState)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})
	})
}

func intgTestsValidateDelete() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		// Create the VM
		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vm)
	})
	AfterEach(func() {
		err := ctx.Client.Delete(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
