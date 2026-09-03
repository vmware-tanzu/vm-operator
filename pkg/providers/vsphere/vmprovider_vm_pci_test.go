// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmPCITests() {
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

		// For old behavior, we'll fallback to these standalone fields when the
		// class does not have a ConfigSpec.
		vmClass.Spec.Hardware.Devices = vmopv1.VirtualDevices{
			VGPUDevices: []vmopv1.VGPUDevice{
				{
					ProfileName: "profile-from-class-without-class-as-config-fss",
				},
			},
			DynamicDirectPathIODevices: []vmopv1.DynamicDirectPathIODevice{
				{
					VendorID:    59,
					DeviceID:    60,
					CustomLabel: "label-from-class-without-class-as-config-fss",
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		// Explicitly place the VM into one of the zones that the test context
		// will create.
		pinVMToFirstZone(ctx, vm)
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	It("VM should have PCI devices from VM Class", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

		devList := object.VirtualDeviceList(o.Config.Hardware.Device)
		p := devList.SelectByType(&vimtypes.VirtualPCIPassthrough{})
		Expect(p).To(HaveLen(2))
	})
}
