// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func getVMTests() {
	Describe("GetVirtualMachine", getVM)
}

func getVM() {

	var (
		ctx    *builder.TestContextForVCSim
		nsInfo builder.WorkloadNamespaceInfo

		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		nsInfo = ctx.CreateWorkloadNamespace()

		vm := builder.DummyVirtualMachine()
		vm.Name = "getvm-test"
		vm.Namespace = nsInfo.Namespace

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	Context("Gets VM by path", func() {
		BeforeEach(func() {
			// TODO: Create VM instead of using one that vcsim creates for free.
			vm, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())

			task, err := vm.Clone(ctx, nsInfo.Folder, vmCtx.VM.Name, vimtypes.VirtualMachineCloneSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.Finder, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})

		It("returns success when provided Folder", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.Finder, nsInfo.Folder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})
	})

	Context("Gets VM when MoID is set", func() {
		BeforeEach(func() {
			// TODO: Create VM instead of using one that vcsim creates for free.
			vm, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())
			vmCtx.VM.Status.UniqueID = vm.Reference().Value
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.Finder, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
			Expect(vm.UniqueID(ctx)).To(Equal(vmCtx.VM.Status.UniqueID))
		})
	})

	Context("Gets VM with ResourcePolicy", func() {
		BeforeEach(func() {
			resourcePolicy, folder := ctx.CreateVirtualMachineSetResourcePolicy("getvm-test", nsInfo)
			vmCtx.VM.Spec.ResourcePolicyName = resourcePolicy.Name

			// TODO: Create VM instead of using one that vcsim creates for free.
			vm, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())

			task, err := vm.Clone(ctx, folder, vmCtx.VM.Name, vimtypes.VirtualMachineCloneSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.Finder, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})
	})
}
