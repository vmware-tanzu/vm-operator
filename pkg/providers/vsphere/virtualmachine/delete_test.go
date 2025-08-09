// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"errors"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func deleteTests() {

	var (
		ctx    *builder.TestContextForVCSim
		vcVM   *object.VirtualMachine
		vmCtx  pkgctx.VirtualMachineContext
		dcID   string
		dsName string
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{
			WithContentLibrary: true,
		})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		dcID = ctx.Datacenter.Reference().Value
		dsName, err = ctx.Datastore.ObjectName(ctx)
		Expect(err).ToNot(HaveOccurred())

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      builder.DummyVirtualMachine(),
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("Deletes VM that is off", func() {
		moID := vcVM.Reference().Value
		Expect(ctx.GetVMFromMoID(moID)).ToNot(BeNil())

		t, err := vcVM.PowerOff(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(t.Wait(ctx)).To(Succeed())

		err = virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
		Expect(err).ToNot(HaveOccurred())

		Expect(ctx.GetVMFromMoID(moID)).To(BeNil())
	})

	It("Deletes VM that is on", func() {
		moID := vcVM.Reference().Value
		Expect(ctx.GetVMFromMoID(moID)).ToNot(BeNil())

		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))

		err = virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
		Expect(err).ToNot(HaveOccurred())

		Expect(ctx.GetVMFromMoID(moID)).To(BeNil())
	})

	Context("vmHomeDir", func() {
		deleteAndAssert := func() {
			moID := vcVM.Reference().Value
			ExpectWithOffset(1, ctx.GetVMFromMoID(moID)).ToNot(BeNil())
			ExpectWithOffset(1, virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)).To(Succeed())
			ExpectWithOffset(1, ctx.GetVMFromMoID(moID)).To(BeNil())
		}

		var (
			fm          *object.FileManager
			vmDir       string
			vmDirSignal string
		)

		BeforeEach(func() {
			fm = object.NewFileManager(vcVM.Client())
			vmDir = fmt.Sprintf("[%s] vm-dir", dsName)
			vmDirSignal = vmDir + "/signal"
		})

		When("extraConfig is not set", func() {
			When("vmDir exists", func() {
				BeforeEach(func() {
					Expect(fm.MakeDirectory(ctx, vmDir, ctx.Datacenter, true)).To(Succeed())
					task, err := fm.CopyDatastoreFile(
						ctx,
						ctx.ContentLibraryItemNVRAMPath,
						ctx.Datacenter,
						vmDirSignal,
						ctx.Datacenter,
						true)
					Expect(err).ToNot(HaveOccurred())
					Expect(task.Wait(ctx)).To(Succeed())
					Expect(pkgutil.DatastoreFileExists(
						ctx,
						vcVM.Client(),
						vmDirSignal,
						ctx.Datacenter)).To(Succeed())
				})
				It("should delete the vm and not remove vmDir", func() {
					deleteAndAssert()
					Expect(pkgutil.DatastoreFileExists(
						ctx,
						vcVM.Client(),
						vmDirSignal,
						ctx.Datacenter)).To(Succeed())
				})
			})
		})

		When("extraConfig is set", func() {
			BeforeEach(func() {
				task, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   pkgconst.VMHomeDatacenterAndDatastoreIDExtraConfigKey,
							Value: fmt.Sprintf("%s,%s", dcID, dsName),
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())
			})

			When("path does not exist", func() {
				It("should not return an error and still delete the vm", func() {
					deleteAndAssert()
				})
			})

			When("path exists", func() {
				BeforeEach(func() {
					Expect(fm.MakeDirectory(ctx, vmDir, ctx.Datacenter, true)).To(Succeed())
					task, err := fm.CopyDatastoreFile(
						ctx,
						ctx.ContentLibraryItemNVRAMPath,
						ctx.Datacenter,
						vmDirSignal,
						ctx.Datacenter,
						true)
					Expect(err).ToNot(HaveOccurred())
					Expect(task.Wait(ctx)).To(Succeed())
					Expect(pkgutil.DatastoreFileExists(
						ctx,
						vcVM.Client(),
						vmDirSignal,
						ctx.Datacenter)).To(Succeed())
				})
				It("should delete the vm and the specified path", func() {
					deleteAndAssert()
					Expect(pkgutil.DatastoreFileExists(
						ctx,
						vcVM.Client(),
						vmDirSignal,
						ctx.Datacenter)).To(MatchError(os.ErrNotExist))
				})
			})
		})
	})

	Specify("VM is paused by admin", func() {
		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

		sctx := ctx.SimulatorContext()
		sctx.WithLock(
			vcVM.Reference(),
			func() {
				vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
				vm.Config = &vimtypes.VirtualMachineConfigInfo{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.PauseVMExtraConfigKey,
							Value: "True",
						},
					},
				}
			})

		err := virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)
		Expect(err).To(HaveOccurred())
		var noRequeueErr pkgerr.NoRequeueError
		Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
		Expect(noRequeueErr.Message).To(Equal(constants.VMPausedByAdminError))
		Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).ToNot(BeNil())
	})

	DescribeTable("VM is not connected",
		func(state vimtypes.VirtualMachineConnectionState) {
			var moVM mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

			sctx := ctx.SimulatorContext()
			sctx.WithLock(
				vcVM.Reference(),
				func() {
					vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
					vm.Summary.Runtime.ConnectionState = state
				})

			err := virtualmachine.DeleteVirtualMachine(vmCtx, vcVM)

			if state == "" {
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).To(BeNil())
			} else {
				Expect(err).To(HaveOccurred())
				var noRequeueErr pkgerr.NoRequeueError
				Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
				Expect(noRequeueErr.Message).To(Equal(
					fmt.Sprintf("unsupported connection state: %s", state)))
				Expect(ctx.GetVMFromMoID(moVM.Reference().Value)).ToNot(BeNil())
			}
		},
		Entry("empty", vimtypes.VirtualMachineConnectionState("")),
		Entry("disconnected", vimtypes.VirtualMachineConnectionStateDisconnected),
		Entry("inaccessible", vimtypes.VirtualMachineConnectionStateInaccessible),
		Entry("invalid", vimtypes.VirtualMachineConnectionStateInvalid),
		Entry("orphaned", vimtypes.VirtualMachineConnectionStateOrphaned),
	)
}
