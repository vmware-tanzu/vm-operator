// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func publishTests() {

	var (
		ctx      *builder.TestContextForVCSim
		vcVM     *object.VirtualMachine
		vm       *vmopv1.VirtualMachine
		cl       *imgregv1a1.ContentLibrary
		clv1a2   *imgregv1.ContentLibrary
		vmPub    *vmopv1.VirtualMachinePublishRequest
		vmCtx    pkgctx.VirtualMachineContext
		vmPubCtx pkgctx.VirtualMachinePublishRequestContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{WithContentLibrary: true})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vm = builder.DummyVirtualMachine()
		vm.Status.UniqueID = vcVM.Reference().Value

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}

		vmPub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", "dummy-ns",
			vcVM.Name(), "dummy-item-name", "dummy-cl")
		vmPub.Status.SourceRef = &vmPub.Spec.Source
		vmPub.Status.TargetRef = &vmPub.Spec.Target
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("CreateOVF is called", func() {
		BeforeEach(func() {
			cl = builder.DummyContentLibrary("dummy-cl", "dummy-ns", ctx.LocalContentLibraryID)
		})
		It("Publishes VM that is off", func() {
			t, err := vcVM.PowerOff(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(t.Wait(ctx)).To(Succeed())

			itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(itemID).NotTo(BeNil())
		})

		It("Publishes VM that is on", func() {
			state, err := vcVM.PowerState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))

			itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(itemID).NotTo(BeNil())
		})

		// TODO: update after vcsim bug is resolved.
		// Currently if cl doesn't exist, vcsim set notFound http code
		// but doesn't return immediately, which cause a panic error.
		It("returns error if target content library does not exist", func() {
			Skip("vcsim doesn't return immediately, which cause a panic")
			vmPubCtx.ContentLibrary.Spec.UUID = "12345"

			itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
			Expect(err).To(HaveOccurred())
			Expect(itemID).To(BeEmpty())
		})

		// TODO: vcsim currently doesn't check if an item already exists in the cl.
		It("returns error if target content library item already exists", func() {
			Skip("vcsim currently doesn't check if an item already exists in the cl")
			vmPubCtx.VMPublishRequest.Spec.Target.Item.Name = ctx.ContentLibraryImageName

			itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(itemID).NotTo(BeNil())
		})
	})

	When("CloneVM is called", func() {
		BeforeEach(func() {
			inventoryCL, err := ctx.Finder.DefaultFolder(ctx)
			Expect(err).NotTo(HaveOccurred())

			clv1a2 = builder.DummyContentLibraryV1A2("dummy-cl", "dummy-ns", inventoryCL.Reference().Value)
		})

		It("Publishes VM that is off", func() {
			t, err := vcVM.PowerOff(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(t.Wait(ctx)).To(Succeed())

			itemID, err := virtualmachine.CloneVM(vmCtx, ctx.VCClient.Client, vmPub, clv1a2, "1234")
			Expect(err).ToNot(HaveOccurred())
			Expect(itemID).NotTo(BeNil())
		})

		It("Publishes VM that is on", func() {
			state, err := vcVM.PowerState(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))

			itemID, err := virtualmachine.CloneVM(vmCtx, ctx.VCClient.Client, vmPub, clv1a2, "1234")
			Expect(err).ToNot(HaveOccurred())
			Expect(itemID).NotTo(BeNil())
		})
	})
}
