// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	"k8s.io/apimachinery/pkg/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func getVMTests() {
	Describe("GetVirtualMachine", getVM)
}

func getVM() {
	// Use a VM that vcsim creates for us.
	const vcVMName = "DC0_C0_RP0_VM0"

	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		testConfig builder.VCSimTestConfig

		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
		nsInfo = ctx.CreateWorkloadNamespace()

		vm := builder.DummyVirtualMachine()
		vm.Name = "getvm-test"
		vm.Namespace = nsInfo.Namespace

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	Context("Gets VM when MoID is set", func() {
		BeforeEach(func() {
			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())
			vmCtx.VM.Status.UniqueID = vm.Reference().Value
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
			Expect(vm.Reference().Value).To(Equal(vmCtx.VM.Status.UniqueID))
		})

		Context("VC client is logged out", func() {
			BeforeEach(func() {
				Expect(ctx.VCClient.Logout(ctx)).To(Succeed())
			})

			It("returns error", func() {
				vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
				Expect(err).To(HaveOccurred())
				Expect(vm).To(BeNil())
			})
		})
	})

	Context("Gets VM by UUID", func() {
		BeforeEach(func() {
			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vm.Properties(ctx, vm.Reference(), nil, &o)).To(Succeed())
			vmCtx.VM.UID = types.UID(o.Config.InstanceUuid)
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})

		Context("VC client is logged out", func() {
			BeforeEach(func() {
				Expect(ctx.VCClient.Logout(ctx)).To(Succeed())
			})

			It("returns error", func() {
				vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
				Expect(err).To(HaveOccurred())
				Expect(vm).To(BeNil())
			})
		})
	})

	Context("Gets VM by BiosUUID", func() {
		BeforeEach(func() {
			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vm.Properties(ctx, vm.Reference(), nil, &o)).To(Succeed())
			vmCtx.VM.Spec.BiosUUID = o.Config.Uuid
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})

		Context("VC client is logged out", func() {
			BeforeEach(func() {
				Expect(ctx.VCClient.Logout(ctx)).To(Succeed())
			})

			It("returns error", func() {
				vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
				Expect(err).To(HaveOccurred())
				Expect(vm).To(BeNil())
			})
		})
	})

	Context("VM does not exist", func() {
		BeforeEach(func() {
			vmCtx.VM.UID = "bogus-uid"
			vmCtx.VM.Spec.BiosUUID = "bogus-bios-uuid"
			vmCtx.VM.Status.UniqueID = "bogus-moid"
		})

		It("returns success with nil vm", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.VCClient.Client, ctx.Datacenter)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).To(BeNil())
		})
	})
}
