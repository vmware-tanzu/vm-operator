// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmCNSTests() {

	const cnsVolumeName = "cns-volume-1"

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

	It("CSI Volumes workflow", func() {
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		By("Add CNS volume to VM", func() {
			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: cnsVolumeName,
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-volume-1",
							},
						},
					},
				},
			}

			err := createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("one or more persistent volumes is pending"))
			Expect(err.Error()).To(ContainSubstring(cnsVolumeName))
			Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
		})

		By("CNS volume is not attached", func() {
			errMsg := "blah blah blah not attached"

			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     cnsVolumeName,
					Attached: false,
					Error:    errMsg,
				},
			}

			err := createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("one or more persistent volumes is pending"))
			Expect(err.Error()).To(ContainSubstring(cnsVolumeName))

			Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
		})

		By("CNS volume is attached", func() {
			vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
				{
					Name:     cnsVolumeName,
					Attached: true,
				},
			}
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
		})
	})
}
