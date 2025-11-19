// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("GetVirtualMachineReconcilePriority", func() {
	var (
		ctx             context.Context
		vm              *vmopv1.VirtualMachine
		defaultPriority int
	)

	BeforeEach(func() {
		ctx = context.Background()
		defaultPriority = handler.LowPriority
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test-ns",
			},
			Spec: vmopv1.VirtualMachineSpec{
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
			Status: vmopv1.VirtualMachineStatus{
				PowerState: vmopv1.VirtualMachinePowerStateOn,
			},
		}
	})

	Context("When annotation with priority is set", func() {
		It("should return the priority from annotation with valid integer", func() {
			vm.Annotations = map[string]string{
				pkgconst.ReconcilePriorityAnnotationKey: "42",
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(42))
		})

		It("should ignore annotation with invalid integer and proceed with other checks", func() {
			vm.Annotations = map[string]string{
				pkgconst.ReconcilePriorityAnnotationKey: "invalid",
			}
			// VM is created with matching power state, should return default
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should handle empty annotation value", func() {
			vm.Annotations = map[string]string{
				pkgconst.ReconcilePriorityAnnotationKey: "",
			}
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})
	})

	Context("When object is not a VirtualMachine", func() {
		It("should return default priority for non-VM object", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, pod, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should handle object with nil annotations gracefully", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Namespace:   "test-ns",
					Annotations: nil,
				},
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, pod, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})
	})

	Context("When VM is being deleted", func() {
		It("should return priorityDeleting", func() {
			now := metav1.Now()
			vm.DeletionTimestamp = &now
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventDelete, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineDeleting))
		})

		It("should prioritize deletion over creation", func() {
			now := metav1.Now()
			vm.DeletionTimestamp = &now
			// VM is not created yet
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineConditionCreated, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventDelete, vm, nil, defaultPriority)
			// Deleting should be returned instead of creating
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineDeleting))
		})
	})

	Context("When VM is not created", func() {
		It("should return priorityCreating when Created condition is false", func() {
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineConditionCreated, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineCreating))
		})

		It("should return priorityCreating when Created condition is not set", func() {
			// No conditions set
			vm.Status.Conditions = nil
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineCreating))
		})

		It("should return priorityCreating when Created condition is unknown", func() {
			pkgcond.MarkUnknown(vm, vmopv1.VirtualMachineConditionCreated, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineCreating))
		})
	})

	Context("When VM power state needs change", func() {
		BeforeEach(func() {
			// Mark VM as created
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
		})

		It("should return priorityPowerStateChange when powering on", func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachinePowerStateChange))
		})

		It("should return priorityPowerStateChange when powering off", func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachinePowerStateChange))
		})

		It("should return priorityPowerStateChange when suspending", func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachinePowerStateChange))
		})
	})

	Context("When VM is powered on and waiting for IP", func() {
		BeforeEach(func() {
			// Mark VM as created
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
		})

		It("should return priorityWaitingForIP when network is enabled and no IP is set", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForIP))
		})

		It("should return priorityWaitingForIP when network status is nil", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForIP))
		})

		It("should return priorityWaitingForIP when both IPv4 and IPv6 are empty", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "",
				PrimaryIP6: "",
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForIP))
		})

		It("should return default priority when IPv4 is set", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "192.168.1.1",
				PrimaryIP6: "",
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return default priority when IPv6 is set", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "",
				PrimaryIP6: "2001:db8::1",
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return default priority when both IPv4 and IPv6 are set", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "192.168.1.1",
				PrimaryIP6: "2001:db8::1",
			}
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return default priority when network is disabled", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: true,
			}
			vm.Status.Network = nil
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return default priority when network spec is nil", func() {
			vm.Spec.Network = nil
			vm.Status.Network = nil
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})
	})

	Context("When VM is powered off and waiting for IP", func() {
		BeforeEach(func() {
			// Mark VM as created
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
		})

		It("should not wait for IP when VM is powered off and disk promotion disabled", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Should not return priorityWaitingForIP since VM is off
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should check disk promotion when VM is powered off", func() {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOffline
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Should return disk promotion priority since VM is powered off
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForDiskPromo))
		})
	})

	Context("When VM is waiting for disk promotion", func() {
		BeforeEach(func() {
			// Mark VM as created
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
		})

		It("should return priorityWaitingForDiskPromo when disk promotion is online and not synced", func() {
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForDiskPromo))
		})

		It("should return priorityWaitingForDiskPromo when disk promotion is offline and not synced", func() {
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOffline
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForDiskPromo))
		})

		It("should return priorityWaitingForDiskPromo when condition is unknown", func() {
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
			pkgcond.MarkUnknown(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForDiskPromo))
		})

		It("should return default priority when disk promotion is disabled", func() {
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return default priority when disk promotion is synced", func() {
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineDiskPromotionSynced)
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should return priorityWaitingForDiskPromo when PromoteDisksMode is empty (not disabled)", func() {
			vm.Spec.PromoteDisksMode = ""
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Empty string is not equal to "Disabled", so disk promotion check applies
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForDiskPromo))
		})
	})

	Context("Priority ordering", func() {
		It("should have correct priority ordering", func() {
			// Test that deleting takes precedence over power state change
			now := metav1.Now()
			vm.DeletionTimestamp = &now
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff

			deletingPriority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventDelete, vm, nil, defaultPriority)
			Expect(deletingPriority).To(Equal(kubeutil.PriorityVirtualMachineDeleting))

			// Reset deletion
			vm.DeletionTimestamp = nil
			powerChangePriority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(powerChangePriority).To(Equal(kubeutil.PriorityVirtualMachinePowerStateChange))

			// Ensure creating has highest priority (lowest number for higher priority queue)
			vm.Status.PowerState = vm.Spec.PowerState
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineConditionCreated, "", "")
			creatingPriority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventCreate, vm, nil, defaultPriority)
			Expect(creatingPriority).To(Equal(kubeutil.PriorityVirtualMachineCreating))

			// Verify order: creating > powerChange > deleting > waitingForIP > waitingForDiskPromo
			Expect(creatingPriority).To(BeNumerically(">", powerChangePriority))
			Expect(powerChangePriority).To(BeNumerically(">", deletingPriority))
		})
	})

	Context("Complex scenarios", func() {
		It("should handle VM with network enabled, powered on, no IP, and disk promotion", func() {
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")

			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Should return priorityWaitingForIP since it's checked before disk promotion
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachineWaitingForIP))
		})

		It("should not check disk promotion when VM is powered on with IP", func() {
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				PrimaryIP4: "192.168.1.1",
			}
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
			pkgcond.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")

			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Should return default since disk promo check doesn't run after PoweredOn case succeeds
			Expect(priority).To(Equal(defaultPriority))
		})

		It("should prioritize power state change over waiting for IP", func() {
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
				Disabled: false,
			}
			vm.Status.Network = nil

			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			// Should return priorityPowerStateChange since it's checked before waiting for IP
			Expect(priority).To(Equal(kubeutil.PriorityVirtualMachinePowerStateChange))
		})
	})

	Context("With different default priorities", func() {
		It("should respect custom default priority", func() {
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn

			customDefault := 42
			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, customDefault)
			Expect(priority).To(Equal(customDefault))
		})

		It("should use default priority when no special conditions are met", func() {
			pkgcond.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled

			priority := kubeutil.GetVirtualMachineReconcilePriority(ctx, kubeutil.EventUpdate, vm, nil, defaultPriority)
			Expect(priority).To(Equal(defaultPriority))
		})
	})
})
