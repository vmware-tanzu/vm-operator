// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

// bootDiskVMOptions describes a VM that is deployed from an image and that
// optionally requests a spec.advanced.bootDiskCapacity.
type bootDiskVMOptions struct {
	Namespace        string
	Name             string
	ImageName        string
	ClassName        string
	StorageClass     string
	PowerState       vmopv1.VirtualMachinePowerState
	PromoteDisksMode vmopv1.VirtualMachinePromoteDisksMode
	BootDiskCapacity *resource.Quantity
	Annotations      map[string]string
}

// newBootDiskVMOptions returns the options of a VM that is powered on and that
// uses the default promoteDisksMode and no spec.advanced.bootDiskCapacity.
func newBootDiskVMOptions(
	namespace, name, imageName string,
	resources *e2eConfig.Resources) bootDiskVMOptions {

	return bootDiskVMOptions{
		Namespace:    namespace,
		Name:         name,
		ImageName:    imageName,
		ClassName:    resources.VMClassName,
		StorageClass: resources.StorageClassName,
		PowerState:   vmopv1.VirtualMachinePowerStateOn,
	}
}

// imageBootDiskCapacity returns the capacity of the boot disk of the image.
func imageBootDiskCapacity(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, imageName string) resource.Quantity {

	GinkgoHelper()

	vmoperator.WaitForOVFVirtualMachineImageReady(ctx, &config.Config, client, namespace, imageName)

	vmi := &vmopv1.VirtualMachineImage{}
	Expect(client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      imageName,
	}, vmi)).To(Succeed(), "failed to get VirtualMachineImage %s", imageName)
	Expect(vmi.Status.Disks).ToNot(BeEmpty())
	Expect(vmi.Status.Disks[0].Limit).ToNot(BeNil(), "image %s has no boot disk limit", imageName)

	return vmi.Status.Disks[0].Limit.DeepCopy()
}

// createBootDiskVM creates the VM and registers its deletion for the end of the
// spec unless skipCleanup is true.
func createBootDiskVM(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	opts bootDiskVMOptions,
	skipCleanup bool) {

	GinkgoHelper()

	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Namespace:   opts.Namespace,
			Annotations: opts.Annotations,
		},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName:        opts.ClassName,
			ImageName:        opts.ImageName,
			StorageClass:     opts.StorageClass,
			PowerState:       opts.PowerState,
			PromoteDisksMode: opts.PromoteDisksMode,
		},
	}
	if opts.BootDiskCapacity != nil {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			BootDiskCapacity: opts.BootDiskCapacity,
		}
	}

	By("Creating the Virtual Machine")
	Expect(client.Create(ctx, vm)).To(Succeed(), "failed to create VM %s", opts.Name)
	DeferCleanup(func() {
		if !skipCleanup {
			vmoperator.DeleteVirtualMachineAndWait(ctx, config, client, opts.Namespace, opts.Name)
		}
	})
}

// updateBootDiskVM gets the VM, applies mutate to it, and updates it, retrying
// on conflicts.
func updateBootDiskVM(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string,
	mutate func(vm *vmopv1.VirtualMachine)) {

	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, client, namespace, name)
		g.Expect(err).NotTo(HaveOccurred())
		mutate(vm)
		g.Expect(client.Update(ctx, vm)).To(Succeed())
	}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(Succeed(),
		"Timed out updating VirtualMachine %s/%s", namespace, name)
}

// setBootDiskVMPowerState sets spec.powerState of the VM and waits until the VM
// has that power state.
func setBootDiskVMPowerState(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string,
	powerState vmopv1.VirtualMachinePowerState) {

	GinkgoHelper()

	By("Setting the power state of the VM to " + string(powerState))
	vmoperator.UpdateVirtualMachinePowerState(ctx, config, client, namespace, name, string(powerState))
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, client, namespace, name, string(powerState))
}

// setBootDiskCapacity sets spec.advanced.bootDiskCapacity of the VM.
func setBootDiskCapacity(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string,
	capacity resource.Quantity) {

	GinkgoHelper()

	By("Setting spec.advanced.bootDiskCapacity to " + capacity.String())
	updateBootDiskVM(ctx, config, client, namespace, name, func(vm *vmopv1.VirtualMachine) {
		if vm.Spec.Advanced == nil {
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
		}
		vm.Spec.Advanced.BootDiskCapacity = &capacity
	})
}

// waitForFirstBootDone waits until VM Operator has powered on the VM at least
// once.
func waitForFirstBootDone(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string) {

	GinkgoHelper()

	By("Waiting for the VM to have booted")
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, client, namespace, name)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Annotations).To(HaveKey(vmopv1.FirstBootDoneAnnotation))
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"Timed out waiting for VirtualMachine %s/%s to have booted", namespace, name)
}

// waitForBootDiskPromoted waits until the disk promotion of the VM completes.
func waitForBootDiskPromoted(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	client ctrlclient.Client,
	namespace, name string) {

	GinkgoHelper()

	By("Waiting for the disks of the VM to be promoted")
	vmoperator.WaitOnVirtualMachineCondition(ctx, config, client, namespace, name, metav1.Condition{
		Type:   vmopv1.VirtualMachineDiskPromotionSynced,
		Status: metav1.ConditionTrue,
	})
}

// bootDiskCapacityInVCenter returns the capacity in bytes of the first disk,
// which like the product code is assumed to be the boot disk, of the VM in
// vCenter.
func bootDiskCapacityInVCenter(
	ctx context.Context,
	g Gomega,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine) int64 {

	g.Expect(vm.Status.UniqueID).NotTo(BeEmpty(),
		"VirtualMachine %s/%s has no status.uniqueID yet", vm.Namespace, vm.Name)

	var moVM mo.VirtualMachine
	vcVM := object.NewVirtualMachine(vimClient, vimtypes.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: vm.Status.UniqueID,
	})
	g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device"}, &moVM)).To(Succeed())
	g.Expect(moVM.Config).NotTo(BeNil())

	for _, d := range moVM.Config.Hardware.Device {
		if disk, ok := d.(*vimtypes.VirtualDisk); ok {
			return disk.CapacityInBytes
		}
	}

	g.Expect(false).To(BeTrue(), "no disk found on the vCenter VM")
	return 0
}

// eventuallyVCenterBootDiskCapacity polls until the first (boot) disk of the VM
// in vCenter has the expected capacity. It does not look at the VM's power
// state or at its boot disk PVC, if any.
func eventuallyVCenterBootDiskCapacity(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	vimClient *vim25.Client,
	k8sClient ctrlclient.Client,
	namespace, name string,
	expected resource.Quantity,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, k8sClient, namespace, name)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(bootDiskCapacityInVCenter(ctx, g, vimClient, vm)).To(Equal(expected.Value()),
			"vCenter boot disk capacity of VirtualMachine %s/%s is not %s", namespace, name, expected.String())
	}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed(),
		"Timed out waiting for the vCenter boot disk of VirtualMachine %s/%s to be %s", namespace, name, expected.String())
}

// consistentlyBootDiskCapacity waits until the VM exists in vCenter. Then it
// makes sure that, for the duration of the "consistent-virtual-machine-condition"
// interval, the first (boot) disk in vCenter keeps the expected capacity.
//
// Use it for a VM whose boot disk must not be resized, ex. a smaller capacity
// than the image, or spec.promoteDisksMode set to Disabled.
func consistentlyBootDiskCapacity(
	ctx context.Context,
	config *e2eConfig.E2EConfig,
	vimClient *vim25.Client,
	k8sClient ctrlclient.Client,
	namespace, name string,
	expected resource.Quantity,
) {
	GinkgoHelper()

	check := func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, k8sClient, namespace, name)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(bootDiskCapacityInVCenter(ctx, g, vimClient, vm)).To(Equal(expected.Value()),
			"vCenter boot disk capacity of VirtualMachine %s/%s is not %s", namespace, name, expected.String())
	}

	Eventually(check, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"Timed out waiting for VirtualMachine %s/%s to exist in vCenter", namespace, name)
	Consistently(check, config.GetIntervals("default", "consistent-virtual-machine-condition")...).Should(Succeed(),
		"Boot disk capacity of VirtualMachine %s/%s changed", namespace, name)
}
