// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
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
		testConfig = builder.VCSimTestConfig{
			WithWorkloadIsolation: true,
		}

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

	Context("Gets VM by inventory", func() {
		BeforeEach(func() {
			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())

			task, err := vm.Clone(ctx, nsInfo.Folder, vmCtx.VM.Name, vimtypes.VirtualMachineCloneSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})

		It("returns nil if VM does not exist", func() {
			vmCtx.VM.Name = "bogus"
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).To(BeNil())
		})

		Context("Namespace Folder does not exist", func() {
			BeforeEach(func() {
				task, err := nsInfo.Folder.Destroy(vmCtx)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(vmCtx)).To(Succeed())
			})

			It("returns error", func() {
				vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("failed to get namespace Folder"))
				Expect(vm).To(BeNil())
			})
		})

		It("returns success when MoID is invalid", func() {
			// Expect fallback to inventory.
			vmCtx.VM.Status.UniqueID = "vm-bogus"

			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})
	})

	Context("Gets VM when MoID is set", func() {
		BeforeEach(func() {
			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())
			vmCtx.VM.Status.UniqueID = vm.Reference().Value
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
			Expect(vm.Reference().Value).To(Equal(vmCtx.VM.Status.UniqueID))
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
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})
	})

	Context("Gets VM with ResourcePolicy by inventory", func() {
		BeforeEach(func() {
			resourcePolicy, folder := ctx.CreateVirtualMachineSetResourcePolicyA2("getvm-test", nsInfo)
			if vmCtx.VM.Spec.Reserved == nil {
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
			}
			vmCtx.VM.Spec.Reserved.ResourcePolicyName = resourcePolicy.Name

			vm, err := ctx.Finder.VirtualMachine(ctx, vcVMName)
			Expect(err).ToNot(HaveOccurred())

			task, err := vm.Clone(ctx, folder, vmCtx.VM.Name, vimtypes.VirtualMachineCloneSpec{})
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		})

		It("returns success", func() {
			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm).ToNot(BeNil())
		})

		It("returns error when ResourcePolicy does not exist", func() {
			if vmCtx.VM.Spec.Reserved == nil {
				vmCtx.VM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
			}
			vmCtx.VM.Spec.Reserved.ResourcePolicyName = "bogus"

			vm, err := vcenter.GetVirtualMachine(vmCtx, ctx.Client, ctx.VCClient.Client, ctx.Datacenter, ctx.Finder)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("failed to get VirtualMachineSetResourcePolicy"))
			Expect(vm).To(BeNil())
		})
	})
}
