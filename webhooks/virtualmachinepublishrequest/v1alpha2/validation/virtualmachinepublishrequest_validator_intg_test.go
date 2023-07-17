// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
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

	oldIsWCPVMImageRegistryEnabledFunc func() bool
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmPub = builder.DummyVirtualMachinePublishRequestA2("dummy-vmpub", ctx.Namespace, "dummy-vm",
		"dummy-item", "dummy-cl")
	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm"
	vm.Namespace = ctx.Namespace

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
	})

	AfterEach(func() {
		lib.IsWCPVMImageRegistryEnabled = ctx.oldIsWCPVMImageRegistryEnabledFunc
		err = nil
		ctx = nil
	})

	When("WCP_VM_Image_Registry is enabled: create is performed", func() {
		It("should allow the request", func() {
			Eventually(func() error {
				return ctx.Client.Create(ctx, ctx.vmPub)
			}).Should(Succeed())
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
			Eventually(func() string {
				if err = ctx.Client.Create(ctx, ctx.vmPub); err != nil {
					return err.Error()
				}
				return ""
			}).Should(ContainSubstring("WCP_VM_Image_Registry feature not enabled"))
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

		Expect(ctx.Client.Create(ctx, ctx.vmPub)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmPub)
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmPub)).To(Succeed())

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

		Expect(ctx.Client.Create(ctx, ctx.vmPub)).To(Succeed())
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
