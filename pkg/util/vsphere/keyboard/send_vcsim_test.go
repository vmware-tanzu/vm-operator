// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/keyboard"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// vcsim's upstream simulator does not implement PutUsbScanCodes. Rather than
// settle for only asserting the resulting MethodNotFound fault, this test
// file adds an externally-introduced PutUsbScanCodes implementation to
// vcsim for the duration of these specs -- the same idea used in
// pkg/vmconfig/volumes/unmanaged/register/unmanagedvolumes_register_test.go
// (there: PbmQueryAssociatedProfiles on a fake ProfileManager, added via
// Registry.Map.Handler).
//
// That file's approach (a Map.Handler override) does not work for
// VirtualMachine methods specifically: govmomi's Session.Get falls back to
// a plain Map.Get for any object type it doesn't specially case (see
// simulator/session_manager.go's Session.Get), which discards a
// Map.Handler override for any call made through an authenticated session
// -- and PutUsbScanCodes always is, since it targets a VirtualMachine.
// Instead, installUsbScanCodesSupport replaces the VM's entry in the
// simulator's registry outright (via Registry.Put, keyed by the VM's
// existing reference), so every lookup path -- including Session.Get's
// fallback -- resolves to the same, augmented object.
//
// fakeVirtualMachine embeds the real, already-registered *simulator.VirtualMachine
// so state such as Runtime.PowerState reflects the actual object being
// operated on, not a blank stand-in.
type fakeVirtualMachine struct {
	*simulator.VirtualMachine
}

// PutUsbScanCodes mirrors real vSphere's documented behavior: the VM must be
// powered on, otherwise the call faults with InvalidPowerState. On success,
// it returns the number of key events accepted.
func (vm *fakeVirtualMachine) PutUsbScanCodes(req *vimtypes.PutUsbScanCodes) soap.HasFault {
	r := &methods.PutUsbScanCodesBody{}

	if vm.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOn {
		r.Fault_ = simulator.Fault("", &vimtypes.InvalidPowerState{
			RequestedState: vimtypes.VirtualMachinePowerStatePoweredOn,
			ExistingState:  vm.Runtime.PowerState,
		})
		return r
	}

	r.Res = &vimtypes.PutUsbScanCodesResponse{
		Returnval: int32(len(req.Spec.KeyEvents)),
	}

	return r
}

// installUsbScanCodesSupport augments ref's registered simulator object
// with PutUsbScanCodes support, in place, for the lifetime of ctx.
func installUsbScanCodesSupport(ctx *builder.TestContextForVCSim, ref vimtypes.ManagedObjectReference) {
	registry := ctx.SimulatorContext().For(vim25.Path).Map

	vm, ok := registry.Get(ref).(*simulator.VirtualMachine)
	Expect(ok).To(BeTrue(), "expected %v to already be registered as a *simulator.VirtualMachine", ref)

	registry.Put(&fakeVirtualMachine{VirtualMachine: vm})
}

func sendVCSimTests() {
	Describe("SendCommands against a real object.VirtualMachine", func() {
		var (
			ctx *builder.TestContextForVCSim
			obj *object.VirtualMachine
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

			vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(vmList)).To(BeNumerically(">", 0))
			obj = object.NewVirtualMachine(ctx.VCClient.Client, vmList[0].Reference())

			installUsbScanCodesSupport(ctx, obj.Reference())

			state, err := obj.PowerState(ctx)
			Expect(err).ToNot(HaveOccurred())
			if state != vimtypes.VirtualMachinePowerStatePoweredOn {
				task, err := obj.PowerOn(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())
			}
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		It("sends boot commands to a powered-on VM without error", func() {
			tokens, err := keyboard.ParseCommands([]string{"<esc>boot<enter>"}, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(keyboard.SendCommands(ctx, obj, tokens)).To(Succeed())
		})

		It("fails with the real InvalidPowerState fault when the VM is powered off", func() {
			task, err := obj.PowerOff(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			tokens, err := keyboard.ParseCommands([]string{"<esc>"}, nil)
			Expect(err).ToNot(HaveOccurred())

			err = keyboard.SendCommands(ctx, obj, tokens)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("InvalidPowerState"))
		})
	})
}
