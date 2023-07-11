// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func publishTests() {

	var (
		ctx      *builder.TestContextForVCSim
		vcVM     *object.VirtualMachine
		vm       *vmopv1.VirtualMachine
		cl       *imgregv1a1.ContentLibrary
		vmPub    *vmopv1.VirtualMachinePublishRequest
		vmCtx    context.VirtualMachineContext
		vmPubCtx context.VirtualMachinePublishRequestContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{WithContentLibrary: true})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vm = builder.DummyVirtualMachine()
		vm.Status.UniqueID = vcVM.Reference().Value
		cl = builder.DummyContentLibrary("dummy-cl", "dummy-ns", ctx.ContentLibraryID)
		vmPub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", "dummy-ns",
			vcVM.Name(), "dummy-item-name", "dummy-cl")
		vmPub.Status.SourceRef = &vmPub.Spec.Source
		vmPub.Status.TargetRef = &vmPub.Spec.Target
		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
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
		Expect(state).To(Equal(types.VirtualMachinePowerStatePoweredOn))

		itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(itemID).NotTo(BeNil())
	})

	// TODO: update after vcsim bug is resolved.
	// Currently if cl doesn't exist, vcsim set notFound http code
	// but doesn't return immediately, which cause a panic error.
	XIt("returns error if target content library does not exist", func() {
		vmPubCtx.ContentLibrary.Spec.UUID = "12345"

		itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
		Expect(err).To(HaveOccurred())
		Expect(itemID).To(BeEmpty())
	})

	// TODO: vcsim currently doesn't check if an item already exists in the cl.
	XIt("returns error if target content library item already exists", func() {
		vmPubCtx.VMPublishRequest.Spec.Target.Item.Name = ctx.ContentLibraryImageName

		itemID, err := virtualmachine.CreateOVF(vmCtx, ctx.RestClient, vmPub, cl, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(itemID).NotTo(BeNil())
	})
}
