// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
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
		invalidImageName         bool
		imageNotFound            bool
		validGuestOSType         bool
		invalidGuestOSType       bool
		gOSCSkipAnnotation       bool
		invalidMetadataTransport bool
		invalidMetadataConfigMap bool
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

		// Setting the annotation skips GuestOSType validation
		// works with validGuestOSType or invalidGuestOSType
		if args.gOSCSkipAnnotation {
			vm.Annotations = make(map[string]string)
			vm.Annotations[vsphere.VMOperatorVMGOSCustomizeCheckKey] = vsphere.VMOperatorVMGOSCustomizeDisable
		}

		if args.validGuestOSType {
			ctx.vmImage.Status.GuestOSCustomizable = &[]bool{true}[0]
			err := ctx.Client.Status().Update(ctx, ctx.vmImage)
			Expect(err).ToNot(HaveOccurred())
		}
		if args.invalidGuestOSType {
			ctx.vmImage.Status.GuestOSCustomizable = &[]bool{false}[0]
			err := ctx.Client.Status().Update(ctx, ctx.vmImage)
			Expect(err).ToNot(HaveOccurred())
		}
		if args.invalidMetadataTransport {
			vm.Spec.VmMetadata.Transport = "blah"
		}
		if args.invalidMetadataConfigMap {
			vm.Spec.VmMetadata.ConfigMapName = ""
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
		Entry("should work despite osType when VMGOSCustomizeCheckKey is disabled", createArgs{gOSCSkipAnnotation: true, invalidGuestOSType: true}, true, "", nil),
		Entry("should not work for invalid image name", createArgs{invalidImageName: true}, false, "spec.imageName must be specified", nil),
		Entry("should not work for image with an invalid osType", createArgs{invalidGuestOSType: true}, false, fmt.Sprintf(messages.GuestOSCustomizationNotSupported, builder.DummyOSType, builder.DummyImageName), nil),
		Entry("should not work for invalid metadata transport", createArgs{invalidMetadataTransport: true}, false, "spec.vmmetadata.transport is not supported", nil),
		Entry("should not work for invalid metadata configmapname", createArgs{invalidMetadataConfigMap: true}, false, "spec.vmmetadata.configmapname must be specified", nil),
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
			Expect(err.Error()).To(ContainSubstring("updates to immutable fields are not allowed"))
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
