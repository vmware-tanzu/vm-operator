// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"reflect"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func init() {
	// Register InvokeFSR_TaskRequest so that vcsim's UnmarshalBody can parse
	// the SOAP body for InvokeFSR_Task calls. Without this registration
	// UnmarshalBody fails with "no vmomi type defined for 'InvokeFSR_Task'"
	// before call() is reached, which means Map.Handler is never invoked and
	// fsrInvoked is never set. vcsim does not natively implement InvokeFSR_Task,
	// so the handler intercepts it and returns MethodNotFound.
	vimtypes.Add("InvokeFSR_Task",
		reflect.TypeOf((*internal.InvokeFSR_TaskRequest)(nil)).Elem())
}

func vmCBTTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
	})

	Context("powered-on VM", func() {
		When("ChangeBlockTracking is updated", func() {
			var fsrInvoked atomic.Bool

			JustBeforeEach(func() {
				// Deploy and power on the VM without CBT before installing
				// the intercept handler.
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

				// vcsim does not natively implement InvokeFSR_Task, so the
				// Map.Handler intercepts the call, records that FSR was attempted,
				// and returns MethodNotFound. reconcilePoweredOnChangeBlockTracking
				// treats FSR failure as best-effort: it logs the error but does not
				// propagate it, so the overall reconcile still succeeds.
				fsrInvoked.Store(false)
				ctx.SimulatorContext().Map.Handler = func(
					_ *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "InvokeFSR_Task" {
						fsrInvoked.Store(true)
						return nil, &vimtypes.MethodNotFound{
							Receiver: m.This,
							Method:   m.Name,
						}
					}
					return nil, nil
				}
			})

			AfterEach(func() {
				ctx.SimulatorContext().Map.Handler = nil
			})

			It("invokes FSR and updates the ConfigSpec when CBT changes", func() {
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				var moVM mo.VirtualMachine

				By("enabling CBT on the powered-on VM", func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						ChangeBlockTracking: ptr.To(true),
					}
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVM)).To(Succeed())
				})

				By("verifying CBT is enabled in the VM config", func() {
					Expect(moVM.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))
				})

				By("verifying the status reflects CBT enabled", func() {
					Expect(vm.Status.ChangeBlockTracking).To(HaveValue(BeTrue()))
				})

				By("verifying FSR was attempted to apply CBT on the live VM", func() {
					Expect(fsrInvoked.Load()).To(BeTrue())
				})

				By("disabling CBT on the powered-on VM", func() {
					fsrInvoked.Store(false)
					vm.Spec.Advanced.ChangeBlockTracking = ptr.To(false)
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVM)).To(Succeed())
				})

				By("verifying CBT is disabled in the VM config", func() {
					Expect(moVM.Config.ChangeTrackingEnabled).To(HaveValue(BeFalse()))
				})

				By("verifying the status reflects CBT disabled", func() {
					Expect(vm.Status.ChangeBlockTracking).To(HaveValue(BeFalse()))
				})

				By("verifying FSR was attempted again to apply the CBT change", func() {
					Expect(fsrInvoked.Load()).To(BeTrue())
				})
			})

			It("does not invoke FSR when CBT did not change", func() {
				// Enable CBT on the first reconcile. The configSpec includes
				// ChangeTrackingEnabled, doReconfigure returns ErrReconfigure,
				// and reconcilePoweredOnChangeBlockTracking invokes FSR.
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ChangeBlockTracking: ptr.To(true),
				}
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(fsrInvoked.Load()).To(BeTrue(),
					"FSR should be attempted when CBT is first enabled on a powered-on VM")

				fsrInvoked.Store(false)
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

				By("verifying FSR is not re-attempted when CBT did not change", func() {
					Expect(fsrInvoked.Load()).To(BeFalse())
				})
			})
		})
	})
}
