// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmMiscTests() {
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

	It("Powers VM off", func() {
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
		state, err := vcVM.PowerState(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
	})

	It("returns error when StorageClass is required but none specified", func() {
		vm.Spec.StorageClass = ""
		err := createOrUpdateVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError("StorageClass is required but not specified"))

		c := conditions.Get(vm, vmopv1.VirtualMachineConditionStorageReady)
		Expect(c).ToNot(BeNil())
		expectedCondition := conditions.FalseCondition(
			vmopv1.VirtualMachineConditionStorageReady,
			"StorageClassRequired",
			"StorageClass is required but not specified")
		Expect(*c).To(conditions.MatchCondition(*expectedCondition))
	})

	It("returns NoRequeueError when a PVC does not exist", func() {
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
			Name: "volume1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "missing-pvc",
					},
				},
			},
		})

		err := createOrUpdateVM(ctx, vmProvider, vm)
		Expect(err).To(HaveOccurred())
		Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		c := conditions.Get(vm, vmopv1.VirtualMachineConditionStorageReady)
		Expect(c).ToNot(BeNil())
		Expect(c.Status).To(Equal(metav1.ConditionFalse))
		Expect(c.Reason).To(Equal("NotFound"))
	})

	It("Can be called multiple times", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		modified := o.Config.Modified

		_, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

		// Try to assert nothing changed.
		Expect(o.Config.Modified).To(Equal(modified))
	})
}
