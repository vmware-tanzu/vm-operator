// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking Create", intgTestsValidateCreate)
	Describe("Invoking Update", intgTestsValidateUpdate)
	Describe("Invoking Delete", intgTestsValidateDelete)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vmPub *vmopv1.VirtualMachinePublishRequest
	vm    *vmopv1.VirtualMachine
	cl    *imgregv1a1.ContentLibrary

	oldIsWCPVMImageRegistryEnabledFunc func() bool
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmPub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", ctx.Namespace, "dummy-vm",
		"dummy-item", "dummy-cl")
	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm"
	vm.Namespace = ctx.Namespace
	ctx.vm = vm
	ctx.cl = builder.DummyContentLibrary("dummy-cl", ctx.Namespace, "dummy-id")

	ctx.oldIsWCPVMImageRegistryEnabledFunc = lib.IsWCPVMImageRegistryEnabled

	return ctx
}

func intgTestsValidateCreate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		lib.IsWCPVMImageRegistryEnabled = func() bool {
			return true
		}
		ctx = newIntgValidatingWebhookContext()

		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		err = ctx.Client.Create(ctx, ctx.cl)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		lib.IsWCPVMImageRegistryEnabled = ctx.oldIsWCPVMImageRegistryEnabledFunc
		err = nil
		ctx = nil
	})

	When("WCP_VM_Image_Registry is enabled: create is performed", func() {
		BeforeEach(func() {
			err = ctx.Client.Create(ctx, ctx.vmPub)
		})
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("WCP_VM_Image_Registry is not enabled", func() {
		BeforeEach(func() {
			lib.IsWCPVMImageRegistryEnabled = func() bool {
				return false
			}
			err = ctx.Client.Create(ctx, ctx.vmPub)
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()

		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		err = ctx.Client.Create(ctx, ctx.cl)
		Expect(err).ToNot(HaveOccurred())

		err = ctx.Client.Create(ctx, ctx.vmPub)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmPub)
	})

	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("update is performed with changed source name", func() {
		BeforeEach(func() {
			ctx.vmPub.Spec.Source.Name = "alternate-vm-name"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
		})
	})

	When("update is performed with changed target info", func() {
		BeforeEach(func() {
			ctx.vmPub.Spec.Target.Location.Name = "alternate-cl"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
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

		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		err = ctx.Client.Create(ctx, ctx.cl)
		Expect(err).ToNot(HaveOccurred())

		err = ctx.Client.Create(ctx, ctx.vmPub)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vmPub)
	})

	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
