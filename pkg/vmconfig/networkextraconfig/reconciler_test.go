// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/mo"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/networkextraconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var errFake = errors.New("fake error")

func newVMXNet3(key int32) vimtypes.BaseVirtualDevice {
	d := &vimtypes.VirtualVmxnet3{}
	d.Key = key
	return d
}

// collectEditChanges returns all Edit DeviceChange entries from cs.
func collectEditChanges(cs *vimtypes.VirtualMachineConfigSpec) []*vimtypes.VirtualDeviceConfigSpec {
	var out []*vimtypes.VirtualDeviceConfigSpec
	for _, dc := range cs.DeviceChange {
		if spec, ok := dc.(*vimtypes.VirtualDeviceConfigSpec); ok {
			if spec.Operation == vimtypes.VirtualDeviceConfigSpecOperationEdit {
				out = append(out, spec)
			}
		}
	}
	return out
}

func findCondition(vm *vmopv1.VirtualMachine) *metav1.Condition {
	for i := range vm.Status.Conditions {
		if vm.Status.Conditions[i].Type == vmopv1.VirtualMachineNetworkConfigSynced {
			c := vm.Status.Conditions[i]
			return &c
		}
	}
	return nil
}

var _ = Describe("networkextraconfig.Reconcile", func() {
	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		moVM       mo.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		r          = networkextraconfig.New()
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		ctx = pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
			cfg.Features.TelcoVMServiceAPI = true
		})
		ctx = r.WithContext(ctx)

		vm = &vmopv1.VirtualMachine{}
		vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}

		moVM = mo.VirtualMachine{}
		moVM.Config = &vimtypes.VirtualMachineConfigInfo{
			Version: "vmx-21",
			Hardware: vimtypes.VirtualHardware{
				Device: []vimtypes.BaseVirtualDevice{
					newVMXNet3(4000),
				},
			},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	Context("when TelcoVMServiceAPI is disabled", func() {
		It("returns nil and makes no changes", func() {
			ctx2 := pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
				cfg.Features.TelcoVMServiceAPI = false
			})
			Expect(r.Reconcile(ctx2, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtraConfig).To(BeEmpty())
		})
	})

	Context("when moVM.Config is nil", func() {
		It("returns nil and makes no changes", func() {
			moVM.Config = nil
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtraConfig).To(BeEmpty())
		})
	})

	Context("when no network interfaces are configured", func() {
		It("returns nil and makes no changes", func() {
			vm.Spec.Network = nil
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtraConfig).To(BeEmpty())
		})
	})

	Context("with one VMXNet3 interface", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{Name: "eth0"},
			}
		})

		It("emits no ExtraConfig when spec has no VMXNet3 or advancedProperties", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(configSpec.ExtraConfig).To(BeEmpty())
		})

		It("translates VMXNet3 first-class fields to ExtraConfig when present", func() {
			vm.Spec.Network.Interfaces[0].VMXNet3 = &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
				RSSOffloadEnabled: ptr.To(true),
			}
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			found := false
			for _, ec := range configSpec.ExtraConfig {
				kv := ec.GetOptionValue()
				if kv.Key == "ethernet0.rssoffload" {
					Expect(kv.Value).To(Equal("TRUE"))
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected ethernet0.rssoffload in ExtraConfig")
		})

		It("writes advancedProperties bag keys with the device prefix", func() {
			vm.Spec.Network.Interfaces[0].AdvancedProperties = []vmopv1common.KeyValuePair{
				{Key: "latencySensitivity.level", Value: "high"},
			}
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			found := false
			for _, ec := range configSpec.ExtraConfig {
				kv := ec.GetOptionValue()
				if kv.Key == "ethernet0.latencySensitivity.level" {
					Expect(kv.Value).To(Equal("high"))
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected bag key in ExtraConfig")
		})
	})

	Context("when both UPTv2Enabled and VNUMANodeID differ for the same NIC", func() {
		BeforeEach(func() {
			// EFI firmware and vNUMA topology are prerequisites for VNUMANodeID.
			vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
				Firmware: vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
			}
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: ptr.To(int32(2)),
				},
			}
			// UPTv2 requires full memory reservation.
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			// Powered off so the non-hot-pluggable VNUMANodeID field can apply.
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff

			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name:        "eth0",
					VNUMANodeID: ptr.To(int32(1)),
					VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
						UPTv2Enabled: ptr.To(true),
					},
				},
			}
		})

		It("produces exactly one DeviceChange entry with both values set", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())

			editChanges := collectEditChanges(configSpec)
			Expect(editChanges).To(HaveLen(1), "expected exactly one DeviceChange edit, got %d", len(editChanges))

			dev := editChanges[0].Device.(*vimtypes.VirtualVmxnet3)
			Expect(dev.Uptv2Enabled).NotTo(BeNil())
			Expect(*dev.Uptv2Enabled).To(BeTrue())
			Expect(dev.NumaNode).To(Equal(ptr.To(int32(1))))
		})

		It("augments a pre-existing DeviceChange entry rather than adding a duplicate", func() {
			// Simulate another reconciler (e.g. UpdateEthCardDeviceChanges) having
			// already added an Edit for this NIC before we run.
			existing := &vimtypes.VirtualVmxnet3{}
			existing.Key = 4000
			configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Device:    existing,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
			})

			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())

			editChanges := collectEditChanges(configSpec)
			Expect(editChanges).To(HaveLen(1), "expected exactly one DeviceChange edit after augmenting pre-existing entry")

			dev := editChanges[0].Device.(*vimtypes.VirtualVmxnet3)
			Expect(dev.Uptv2Enabled).NotTo(BeNil())
			Expect(*dev.Uptv2Enabled).To(BeTrue())
			Expect(dev.NumaNode).To(Equal(ptr.To(int32(1))))
		})
	})

	Context("when UPTv2Enabled=true but memory reservation is not locked to max", func() {
		BeforeEach(func() {
			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name: "eth0",
					VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
						UPTv2Enabled: ptr.To(true),
					},
				},
			}
			// No memoryAdvanced set → prerequisite blocked.
		})

		It("sets the condition to PrerequisiteNotMet and emits no DeviceChange", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())

			Expect(collectEditChanges(configSpec)).To(BeEmpty(),
				"expected no DeviceChange when memory reservation prerequisite is unmet")

			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmopv1.VirtualMachinePrerequisiteNotMetReason))
			Expect(cond.Message).To(ContainSubstring("full memory reservation required:"))
			Expect(cond.Message).To(ContainSubstring("reservationLockedToMax=true"))
		})

		It("unblocks when spec.resources.requests.memory equals spec.resources.size.memory", func() {
			mem := resource.MustParse("4Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: &mem},
				Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: &mem},
			}
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOn

			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())

			edits := collectEditChanges(configSpec)
			Expect(edits).To(HaveLen(1))
			Expect(edits[0].Device.(*vimtypes.VirtualVmxnet3).Uptv2Enabled).To(Equal(ptr.To(true)))

			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("still blocks when spec requests.memory != size.memory (partial reservation)", func() {
			req := resource.MustParse("2Gi")
			size := resource.MustParse("4Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: &req},
				Size:     &vmopv1.VirtualMachineResourceQuantity{Memory: &size},
			}

			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())

			Expect(collectEditChanges(configSpec)).To(BeEmpty())
			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(vmopv1.VirtualMachinePrerequisiteNotMetReason))
		})
	})

	Context("when UPTv2Enabled=false and memory reservation is not set", func() {
		BeforeEach(func() {
			// Simulate a NIC that has Uptv2Enabled=true in vSphere being cleared.
			existing := &vimtypes.VirtualVmxnet3{}
			existing.Key = 4000
			existing.Uptv2Enabled = ptr.To(true)
			moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{existing}

			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name: "eth0",
					VMXNet3: &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{
						UPTv2Enabled: ptr.To(false),
					},
				},
			}
		})

		It("applies the DeviceChange without a memory reservation prerequisite", func() {
			Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
			Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())

			edits := collectEditChanges(configSpec)
			Expect(edits).To(HaveLen(1))
			Expect(edits[0].Device.(*vimtypes.VirtualVmxnet3).Uptv2Enabled).To(Equal(ptr.To(false)))

			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("when spec.VNUMANodeID is -1 (explicit clear)", func() {
		BeforeEach(func() {
			// EFI firmware and vNUMA topology are prerequisites for VNUMANodeID.
			vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
				Firmware: vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
			}
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: ptr.To(int32(2)),
				},
			}
			vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff

			vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name:        "eth0",
					VNUMANodeID: ptr.To(int32(-1)),
				},
			}
		})

		When("device has an existing NUMA assignment", func() {
			BeforeEach(func() {
				dev := &vimtypes.VirtualVmxnet3{}
				dev.Key = 4000
				dev.NumaNode = ptr.To(int32(2))
				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{dev}
			})

			It("sends NumaNode = -1 in DeviceChange to clear the NUMA assignment", func() {
				Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())

				editChanges := collectEditChanges(configSpec)
				Expect(editChanges).To(HaveLen(1))
				dev := editChanges[0].Device.(*vimtypes.VirtualVmxnet3)
				Expect(dev.NumaNode).To(Equal(ptr.To(int32(-1))))
			})
		})

		When("device already has no NUMA assignment", func() {
			It("emits no DeviceChange (already no NUMA)", func() {
				Expect(r.Reconcile(ctx, nil, nil, vm, moVM, configSpec)).To(Succeed())
				Expect(collectEditChanges(configSpec)).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("networkextraconfig.OnResult", func() {
	var (
		ctx  context.Context
		vm   *vmopv1.VirtualMachine
		moVM mo.VirtualMachine
		r    = networkextraconfig.New()
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		ctx = pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
			cfg.Features.TelcoVMServiceAPI = true
		})
		ctx = r.WithContext(ctx)

		vm = &vmopv1.VirtualMachine{}
		vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}

		moVM = mo.VirtualMachine{}
	})

	Context("when TelcoVMServiceAPI is disabled", func() {
		It("returns nil without setting condition", func() {
			ctx2 := pkgcfg.UpdateContext(ctx, func(cfg *pkgcfg.Config) {
				cfg.Features.TelcoVMServiceAPI = false
			})
			Expect(r.OnResult(ctx2, vm, moVM, nil)).To(Succeed())
			Expect(vm.Status.Conditions).To(BeEmpty())
		})
	})

	Context("with a task error", func() {
		It("marks condition false with ReasonError", func() {
			Expect(r.OnResult(ctx, vm, moVM, errFake)).To(Succeed())
			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(string(cond.Status)).To(Equal("False"))
			Expect(cond.Reason).To(Equal(vmopv1.VirtualMachineNetworkErrorReason))
		})
	})

	Context("with a NoRequeueNoErr sentinel (e.g. ErrCreate, ErrHasTask)", func() {
		It("does not mark condition false", func() {
			Expect(r.OnResult(ctx, vm, moVM, pkgerr.NoRequeueNoErr("created vm"))).To(Succeed())
			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(string(cond.Status)).To(Equal("True"))
		})
	})

	Context("with no state set (no reconcile call)", func() {
		It("marks condition true", func() {
			Expect(r.OnResult(ctx, vm, moVM, nil)).To(Succeed())
			cond := findCondition(vm)
			Expect(cond).NotTo(BeNil())
			Expect(string(cond.Status)).To(Equal("True"))
		})
	})
})
