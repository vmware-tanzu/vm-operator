// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmInstanceStorageTests() {
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
		// Fast Deploy does not support instance storage.
		disableFastDeploy(parentCtx)

		testConfig = newVMTestConfig()
		testConfig.WithInstanceStorage = true

		vmClass, vm = newVMTestObjects("test-vm-instance-storage")
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

	expectInstanceStorageVolumes := func(
		vm *vmopv1.VirtualMachine,
		isStorage vmopv1.InstanceStorage) {

		ExpectWithOffset(1, isStorage.Volumes).ToNot(BeEmpty())
		isVolumes := vmopv1util.FilterInstanceStorageVolumes(vm)
		ExpectWithOffset(1, isVolumes).To(HaveLen(len(isStorage.Volumes)))

		for _, isVol := range isStorage.Volumes {
			found := false

			for idx, vol := range isVolumes {
				claim := vol.PersistentVolumeClaim.InstanceVolumeClaim
				if claim.StorageClass == isStorage.StorageClass && claim.Size == isVol.Size {
					isVolumes = append(isVolumes[:idx], isVolumes[idx+1:]...)
					found = true
					break
				}
			}

			ExpectWithOffset(1, found).To(BeTrue(), "failed to find instance storage volume for %v", isVol)
		}
	}

	It("creates VM without instance storage", func() {
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
	})

	It("create VM with instance storage", func() {
		Expect(vm.Spec.Volumes).To(BeEmpty())

		vmClass.Spec.Hardware.InstanceStorage = vmopv1.InstanceStorage{
			StorageClass: vm.Spec.StorageClass,
			Volumes: []vmopv1.InstanceStorageVolume{
				{
					Size: resource.MustParse("256Gi"),
				},
				{
					Size: resource.MustParse("512Gi"),
				},
			},
		}
		Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())

		Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeFalse())

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError(vsphere.ErrAddedInstanceStorageVols))

		By("Instance storage volumes should be added to VM", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
			expectInstanceStorageVolumes(vm, vmClass.Spec.Hardware.InstanceStorage)
		})

		_, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError("instance storage PVCs are not bound yet"))
		Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
		Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())

		By("Placement should have been done", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
			Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
			Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
		})

		isVol0 := vm.Spec.Volumes[0]
		Expect(isVol0.PersistentVolumeClaim.InstanceVolumeClaim).ToNot(BeNil())

		By("simulate volume controller workflow", func() {
			// Simulate what would be set by volume controller.
			vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = ""

			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("one or more persistent volumes is pending"))
			Expect(err.Error()).To(ContainSubstring(isVol0.Name))

			// Simulate what would be set by the volume controller.
			for _, vol := range vm.Spec.Volumes {
				vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
					Name:     vol.Name,
					Attached: true,
				})
			}
		})

		By("VM is now created", func() {
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
		})
	})
}
