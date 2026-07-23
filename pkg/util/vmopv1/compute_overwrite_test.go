// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("OverwriteSpecComputeConfig", func() {

	var (
		vm        vmopv1.VirtualMachine
		liveCI    vimtypes.VirtualMachineConfigInfo
		poweredOn bool
		cs        vimtypes.VirtualMachineConfigSpec
		blocked []string
		blockedPO []string
	)

	BeforeEach(func() {
		vm = vmopv1.VirtualMachine{}
		liveCI = vimtypes.VirtualMachineConfigInfo{
			Version: vimtypes.VMX20.String(),
			Hardware: vimtypes.VirtualHardware{
				NumCPU:   4,
				MemoryMB: 4096,
				// AutoCoresPerSocket=true and NumCoresPerSocket=1 simulate a
				// vmx-20+ VM already in automatic socket-sizing mode. Tests that
				// cover explicit topology scenarios override these as needed.
				NumCoresPerSocket:  ptr.To(int32(1)),
				AutoCoresPerSocket: ptr.To(true),
			},
			// AutoCoresPerNumaNode=true simulates vNUMA already in auto mode.
			// Tests that cover explicit vNUMA scenarios override this.
			NumaInfo: &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(true),
			},
		}
		poweredOn = false
		cs = vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		blocked, blockedPO = vmopv1util.OverwriteSpecComputeConfig(vm, liveCI, poweredOn, &cs)
	})

	// ───────────────────── resources.size.cpu ─────────────────────

	Context("resources.size.cpu — powered-off", func() {
		BeforeEach(func() {
			poweredOn = false
			q := resource.MustParse("8")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
		})
		It("writes NumCPUs to cs", func() {
			Expect(cs.NumCPUs).To(Equal(int32(8)))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("resources.size.cpu — powered-on, no CpuHotAddEnabled", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.CpuHotAddEnabled = ptr.To(false)
			q := resource.MustParse("8")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
		})
		It("does not write NumCPUs", func() {
			Expect(cs.NumCPUs).To(Equal(int32(0)))
		})
		It("adds resources.size.cpu to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("resources.size.cpu"))
		})
	})

	Context("resources.size.cpu — powered-on, CpuHotAddEnabled, increasing", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.NumCPU = 4
			liveCI.CpuHotAddEnabled = ptr.To(true)
			q := resource.MustParse("8") // 8 > 4 → increase
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
		})
		It("writes NumCPUs (hot-add allowed)", func() {
			Expect(cs.NumCPUs).To(Equal(int32(8)))
		})
		It("does not add resources.size.cpu to blockedPowerOff", func() {
			Expect(blockedPO).NotTo(ContainElement("resources.size.cpu"))
		})
	})

	Context("resources.size.cpu — powered-on, CpuHotAddEnabled, decreasing", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.NumCPU = 8
			liveCI.CpuHotAddEnabled = ptr.To(true)
			q := resource.MustParse("4") // 4 < 8 → decrease, not hot-pluggable
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
		})
		It("does not write NumCPUs", func() {
			Expect(cs.NumCPUs).To(Equal(int32(0)))
		})
		It("adds resources.size.cpu to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("resources.size.cpu"))
		})
	})

	Context("resources.size.cpu — nil spec, powered-off", func() {
		BeforeEach(func() {
			poweredOn = false
			// spec.resources.size is nil → don't manage
		})
		It("does not write NumCPUs (class/zero preserved)", func() {
			Expect(cs.NumCPUs).To(Equal(int32(0)))
		})
	})

	Context("resources.size.cpu — same as live, powered-on", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.NumCPU = 4
			liveCI.CpuHotAddEnabled = ptr.To(true)
			q := resource.MustParse("4") // same as live
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
		})
		It("does not write NumCPUs (no diff)", func() {
			Expect(cs.NumCPUs).To(Equal(int32(0)))
		})
		It("does not add resources.size.cpu to blockedPowerOff (not differing)", func() {
			Expect(blockedPO).NotTo(ContainElement("resources.size.cpu"))
		})
	})

	// ───────────────────── resources.size.memory ─────────────────────

	Context("resources.size.memory — powered-off", func() {
		BeforeEach(func() {
			poweredOn = false
			q := resource.MustParse("8Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
		})
		It("writes MemoryMB to cs", func() {
			Expect(cs.MemoryMB).To(Equal(int64(8192)))
		})
	})

	Context("resources.size.memory — powered-on, no MemoryHotAddEnabled", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.MemoryHotAddEnabled = ptr.To(false)
			q := resource.MustParse("8Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
		})
		It("does not write MemoryMB", func() {
			Expect(cs.MemoryMB).To(Equal(int64(0)))
		})
		It("adds resources.size.memory to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("resources.size.memory"))
		})
	})

	Context("resources.size.memory — powered-on, MemoryHotAddEnabled, increasing", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.MemoryMB = 4096
			liveCI.MemoryHotAddEnabled = ptr.To(true)
			q := resource.MustParse("8Gi") // 8192 MB > 4096 MB
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
		})
		It("writes MemoryMB (hot-add allowed)", func() {
			Expect(cs.MemoryMB).To(Equal(int64(8192)))
		})
		It("does not add resources.size.memory to blockedPowerOff", func() {
			Expect(blockedPO).NotTo(ContainElement("resources.size.memory"))
		})
	})

	// ───────────────────── CPU allocation (hot-pluggable) ─────────────────────

	Context("resources.requests.cpu — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.CpuAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(100)),
			}
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{
					CPU: resourceMHz(500),
				},
			}
		})
		It("writes CpuAllocation.Reservation", func() {
			Expect(cs.CpuAllocation).NotTo(BeNil())
			Expect(cs.CpuAllocation.Reservation).To(Equal(ptr.To(int64(500))))
		})
		It("does not add to blockedPowerOff (hot-pluggable)", func() {
			Expect(blockedPO).NotTo(ContainElement(ContainSubstring("cpu")))
		})
	})

	Context("resources — powered-on, no spec set, live already at defaults", func() {
		BeforeEach(func() {
			poweredOn = true
			// No spec.resources set → desired: reservation=0, limit=-1 (same as live)
			liveCI.CpuAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(0)),
				Limit:       ptr.To(int64(-1)),
			}
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(0)),
				Limit:       ptr.To(int64(-1)),
			}
		})
		It("does not write allocation (desired==live, no-op)", func() {
			Expect(cs.CpuAllocation).To(BeNil())
			Expect(cs.MemoryAllocation).To(BeNil())
		})
	})

	Context("resources — powered-on, no spec set, live has non-default allocation (class-set)", func() {
		BeforeEach(func() {
			poweredOn = true
			// No spec.resources set → desired defaults: reservation=0, limit=-1
			// Live has class-set non-default allocation → reset to defaults
			liveCI.CpuAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(500)),
				Limit:       ptr.To(int64(1000)),
			}
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(1024)),
				Limit:       ptr.To(int64(2048)),
			}
		})
		It("writes allocation to reset to defaults (reservation=0, limit=-1)", func() {
			Expect(cs.CpuAllocation).NotTo(BeNil())
			Expect(cs.CpuAllocation.Reservation).To(Equal(ptr.To(int64(0))))
			Expect(cs.CpuAllocation.Limit).To(Equal(ptr.To(int64(-1))))
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(Equal(ptr.To(int64(0))))
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(-1))))
		})
	})

	// ───────────────────── latency sensitivity (hot-pluggable) ─────────────────────

	Context("cpuAdvanced.latencySensitivity — powered-on, set to High", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
			}
			ls := vmopv1.VirtualMachineLatencySensitivityHigh
			cpuReq := resource.MustParse("4000")
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				LatencySensitivity: &ls,
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{CPU: &cpuReq},
			}
		})
		It("writes LatencySensitivity High", func() {
			Expect(cs.LatencySensitivity).NotTo(BeNil())
			Expect(cs.LatencySensitivity.Level).To(Equal(vimtypes.LatencySensitivitySensitivityLevelHigh))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ───────────────────── cpuAdvanced.topology.coresPerSocket ─────────────────────

	Context("cpuAdvanced.topology.coresPerSocket — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(2))
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
		})
		It("does not write NumCoresPerSocket (power-off required)", func() {
			Expect(cs.NumCoresPerSocket).To(BeNil())
		})
		It("adds coresPerSocket to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.topology.coresPerSocket"))
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — powered-off, differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(2))
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
		})
		It("writes NumCoresPerSocket", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(4))))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — powered-on, same as live", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
		})
		It("does not add to blockedPowerOff (no diff)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ─── coresPerSocket nil=reset: vmx-20+ ────────────────────────────────────────────

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, vmx-20+, auto=false (not yet auto)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
		})
		It("writes NumCoresPerSocket=0 to request auto sizing", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(0))))
		})
		It("does not add to blockedPowerOff (powered-off reconfigure)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, vmx-20+, auto=true (already auto)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(1))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(true)
		})
		It("does not write NumCoresPerSocket (already in auto mode)", func() {
			Expect(cs.NumCoresPerSocket).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, vmx-20+, powered-on, auto=false", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = true
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
		})
		It("does not write NumCoresPerSocket (power-off required)", func() {
			Expect(cs.NumCoresPerSocket).To(BeNil())
		})
		It("adds coresPerSocket to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.topology.coresPerSocket"))
		})
	})

	// ─── coresPerSocket nil=reset: pre-vmx-20 ─────────────────────────────────────────

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, pre-vmx-20, live=4 (non-default)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX17.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false) // always false on pre-vmx-20
		})
		It("writes NumCoresPerSocket=1 (reset to minimum)", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(1))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, pre-vmx-20, live=1 (already default)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX17.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(1))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
		})
		It("does not write NumCoresPerSocket (already at default)", func() {
			Expect(cs.NumCoresPerSocket).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ─── coresPerSocket sentinel 0 = auto ────────────────────────────────────────────

	Context("cpuAdvanced.topology.coresPerSocket — spec=0 (sentinel auto), vmx-20+, live=4 (not auto)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
			cps := int32(0)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{CoresPerSocket: &cps},
			}
		})
		It("writes NumCoresPerSocket=0 (same as nil spec: reset to auto)", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(0))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — spec=0 (sentinel auto), vmx-20+, live already in auto", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(1))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(true)
			cps := int32(0)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{CoresPerSocket: &cps},
			}
		})
		It("does not write NumCoresPerSocket (already in auto mode)", func() {
			Expect(cs.NumCoresPerSocket).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — spec=0 (sentinel auto), pre-vmx-20, live=4", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX17.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
			cps := int32(0)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{CoresPerSocket: &cps},
			}
		})
		It("writes NumCoresPerSocket=1 (same as nil spec: reset to minimum for pre-vmx-20)", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(1))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ─── vnumaNodeCount nil=reset (vmx-20+) ──────────────────────────────────────────

	Context("cpuAdvanced.topology.vnumaNodeCount — nil spec, vmx-20+, auto=false (not yet auto)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(false),
				CoresPerNumaNode:     ptr.To(int32(4)),
			}
		})
		It("writes VirtualNuma.CoresPerNumaNode=0 to reset to auto", func() {
			Expect(cs.VirtualNuma).NotTo(BeNil())
			Expect(cs.VirtualNuma.CoresPerNumaNode).To(Equal(ptr.To(int32(0))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — nil spec, vmx-20+, auto=true (already auto)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(true),
			}
		})
		It("does not write VirtualNuma (already in auto mode)", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — nil spec, vmx-20+, no NumaInfo", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = nil
		})
		It("does not write VirtualNuma (no NumaInfo to check)", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ─── vnumaNodeCount sentinel 0 = auto ────────────────────────────────────────────

	Context("cpuAdvanced.topology.vnumaNodeCount — spec=0 (sentinel auto), vmx-20+, live not in auto", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(false),
				CoresPerNumaNode:     ptr.To(int32(4)),
			}
			n := int32(0)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{VNUMANodeCount: &n},
			}
		})
		It("writes VirtualNuma.CoresPerNumaNode=0 (same as nil: reset to auto)", func() {
			Expect(cs.VirtualNuma).NotTo(BeNil())
			Expect(cs.VirtualNuma.CoresPerNumaNode).To(Equal(ptr.To(int32(0))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — spec=0 (sentinel auto), vmx-20+, live already auto", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(true),
			}
			n := int32(0)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{VNUMANodeCount: &n},
			}
		})
		It("does not write VirtualNuma (already in auto mode)", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ───────────────────── vNUMA fields — hw version gate (vmx-20) ─────────────────────

	Context("cpuAdvanced.topology.vnumaNodeCount — hwVer < vmx-20", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX19.String()
			n := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
				},
			}
		})
		It("does not write VirtualNuma", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("adds vnumaNodeCount to blocked with version suffix", func() {
			Expect(blocked).To(ContainElement("cpuAdvanced.topology.vnumaNodeCount (requires hwVer >= 20)"))
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — hwVer >= vmx-20, powered-off, cs.NumCPUs pre-set (class or cpuSizeField)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			cs.NumCPUs = 8 // simulates class/cpuSizeField having set this
			n := int32(2)
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
					CoresPerSocket: &cps,
				},
			}
		})
		It("writes VirtualNuma.CoresPerNumaNode = cs.NumCPUs / nodeCount (8/2=4)", func() {
			Expect(cs.VirtualNuma).NotTo(BeNil())
			Expect(cs.VirtualNuma.CoresPerNumaNode).To(Equal(ptr.To(int32(4))))
		})
		It("returns no blocked", func() {
			Expect(blocked).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — class changes CPU, spec doesn't, spec has explicit VNUMANodeCount", func() {
		// Gap scenario: class changes CPU (cs.NumCPUs=16) but doesn't write VirtualNuma.
		// differs must use cs.NumCPUs (not live) to detect the stale CoresPerNumaNode.
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			cs.NumCPUs = 16 // class changed CPU from 8 to 16
			liveCI.Hardware.NumCPU = 8
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				CoresPerNumaNode: ptr.To(int32(4)), // correct for 8 CPUs / 2 nodes
			}
			n := int32(2)
			cps := int32(8)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
					CoresPerSocket: &cps,
				},
			}
		})
		It("writes updated CoresPerNumaNode based on new CPU count (16/2=8)", func() {
			Expect(cs.VirtualNuma).NotTo(BeNil())
			Expect(cs.VirtualNuma.CoresPerNumaNode).To(Equal(ptr.To(int32(8))))
		})
	})

	// ───────────────────── cpuAdvanced.hotAddEnabled — hw gate (vmx-11) ─────────────────────

	Context("cpuAdvanced.hotAddEnabled — hwVer < vmx-11", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX10.String()
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{HotAddEnabled: &v}
		})
		It("does not write CpuHotAddEnabled (vSphere ignores incompatible fields)", func() {
			Expect(cs.CpuHotAddEnabled).To(BeNil())
		})
		It("adds cpuAdvanced.hotAddEnabled to blocked", func() {
			Expect(blocked).To(ContainElement("cpuAdvanced.hotAddEnabled (requires hwVer >= 11)"))
		})
	})

	Context("cpuAdvanced.hotAddEnabled — hwVer >= vmx-11, powered-on, differs", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX11.String()
			poweredOn = true
			liveCI.CpuHotAddEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{HotAddEnabled: &v}
		})
		It("does not write CpuHotAddEnabled (power-off required)", func() {
			// cpuHotAddEnabled flag is a power-off-required operation
			Expect(cs.CpuHotAddEnabled).To(BeNil())
		})
		It("adds cpuAdvanced.hotAddEnabled to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.hotAddEnabled"))
		})
	})

	Context("cpuAdvanced.hotAddEnabled — hwVer >= vmx-11, powered-off, differs", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX11.String()
			poweredOn = false
			liveCI.CpuHotAddEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{HotAddEnabled: &v}
		})
		It("writes CpuHotAddEnabled=true", func() {
			Expect(cs.CpuHotAddEnabled).To(Equal(ptr.To(true)))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.hotAddEnabled — nil spec, live already at default (false)", func() {
		It("does not write CpuHotAddEnabled (desired==live, no-op)", func() {
			Expect(cs.CpuHotAddEnabled).To(BeNil())
		})
	})

	Context("cpuAdvanced.hotAddEnabled — nil spec, powered-off, live=true (class enabled it)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.CpuHotAddEnabled = ptr.To(true) // live is true (class set it)
		})
		It("writes CpuHotAddEnabled=false (nil spec = default=false, overrides class)", func() {
			Expect(cs.CpuHotAddEnabled).To(Equal(ptr.To(false)))
		})
	})

	// ───────────────────── cpuAdvanced.iommuEnabled (power-off required) ─────────────────────

	Context("cpuAdvanced.iommuEnabled — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Flags = vimtypes.VirtualMachineFlagInfo{
				VvtdEnabled: ptr.To(false),
			}
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{IOMMUEnabled: &v}
		})
		It("does not write Flags.VvtdEnabled", func() {
			Expect(cs.Flags).To(BeNil())
		})
		It("adds iommuEnabled to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.iommuEnabled"))
		})
	})

	Context("cpuAdvanced.iommuEnabled — powered-off, differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Flags = vimtypes.VirtualMachineFlagInfo{
				VvtdEnabled: ptr.To(false),
			}
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{IOMMUEnabled: &v}
		})
		It("writes Flags.VvtdEnabled=true", func() {
			Expect(cs.Flags).NotTo(BeNil())
			Expect(cs.Flags.VvtdEnabled).To(Equal(ptr.To(true)))
		})
	})

	// ───────────────────── memoryAdvanced.hotAddEnabled — hw gate (vmx-7) ─────────────────────

	Context("memoryAdvanced.hotAddEnabled — hwVer < vmx-7", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX6.String()
			v := true
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{HotAddEnabled: &v}
		})
		It("does not write MemoryHotAddEnabled (vSphere ignores incompatible fields)", func() {
			Expect(cs.MemoryHotAddEnabled).To(BeNil())
		})
		It("adds memoryAdvanced.hotAddEnabled to blocked", func() {
			Expect(blocked).To(ContainElement("memoryAdvanced.hotAddEnabled (requires hwVer >= 7)"))
		})
	})

	Context("memoryAdvanced.hotAddEnabled — hwVer >= vmx-7, powered-on, differs", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX7.String()
			poweredOn = true
			liveCI.MemoryHotAddEnabled = ptr.To(false)
			v := true
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{HotAddEnabled: &v}
		})
		It("does not write MemoryHotAddEnabled (power-off required)", func() {
			Expect(cs.MemoryHotAddEnabled).To(BeNil())
		})
		It("adds memoryAdvanced.hotAddEnabled to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("memoryAdvanced.hotAddEnabled"))
		})
	})

	// ───────────────────── memoryAdvanced.reservationLockedToMax (hot-pluggable) ─────────────────────

	Context("memoryAdvanced.reservationLockedToMax — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.MemoryReservationLockedToMax = ptr.To(false)
			v := true
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: &v,
			}
		})
		It("writes MemoryReservationLockedToMax=true (hot-pluggable)", func() {
			Expect(cs.MemoryReservationLockedToMax).To(Equal(ptr.To(true)))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).NotTo(ContainElement(ContainSubstring("reservation")))
		})
	})

	// When the lock is desired to stay active, vSphere owns the reservation. A
	// Reservation-only ConfigSpec (without the lock) is validated by vSphere
	// and rejected when the value does not equal the locked memory size.
	// memoryAllocationField must skip the reservation in that case. The gate
	// is on the desired lock state, not the live one: when the lock is being
	// removed this cycle (nil/false spec, live still locked), the reservation
	// must be written in the same ConfigSpec as the unlock, or it would lag a
	// full reconcile behind (see memReservationLockedField, which unlocks
	// unconditionally whenever the desired state is false).

	Context("resources.allocation — powered-on, lock desired active, no spec reservation", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 16384
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(16384)),
				Limit:       ptr.To(int64(-1)),
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			// Spec has no requests.memory (required by webhook when lock is set).
		})
		It("does not write MemoryAllocation (vSphere owns the reservation)", func() {
			Expect(cs.MemoryAllocation).To(BeNil())
		})
	})

	Context("resources.allocation — powered-on, lock desired active, limit differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 16384
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(16384)),
				Limit:       ptr.To(int64(-1)),
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			// Spec requests a limit; no requests.memory.
			lim := resource.MustParse("32Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Limits: &vmopv1.VirtualMachineResourceQuantity{Memory: &lim},
			}
		})
		It("writes only Limit, not Reservation", func() {
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(BeNil())
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(32768))))
		})
	})

	Context("resources.allocation — powered-on, lock being unlocked this cycle, no explicit reservation", func() {
		BeforeEach(func() {
			poweredOn = true
			// Lock is live but spec no longer wants it (nil/false): the lock
			// field unlocks unconditionally, so the reservation must reset to
			// its own desired value (0, since requests.memory is unset) in the
			// same cycle instead of leaving the locked value (16384) stale.
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 16384
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(16384)),
				Limit:       ptr.To(int64(-1)),
			}
		})
		It("writes an explicit Reservation instead of leaving the locked value stale", func() {
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(Equal(ptr.To(int64(0))))
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(-1))))
		})
	})

	Context("resources.allocation — powered-on, unlocking and setting a new reservation in the same reconcile", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 16384
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(16384)),
				Limit:       ptr.To(int64(-1)),
			}
			v := false
			req := resource.MustParse("4Gi")
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: &v,
			}
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{Memory: &req},
			}
		})
		It("writes the new Reservation in the same ConfigSpec as the unlock", func() {
			Expect(cs.MemoryReservationLockedToMax).To(Equal(ptr.To(false)))
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(Equal(ptr.To(int64(4096))))
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(-1))))
		})
	})

	// ───────────────────── class ConfigSpec override (differsCS) ─────────────────────

	Context("resources.size.cpu — spec=live=4, class wrote 8 to CS (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCPU = 4
			q := resource.MustParse("4")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			cs.NumCPUs = 8 // class wrote 8
		})
		It("overrides class value: writes spec's 4 to CS", func() {
			Expect(cs.NumCPUs).To(Equal(int32(4)))
		})
	})

	Context("resources.size.cpu — nil spec, class wrote 8 to CS (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCPU = 4
			// spec.resources.size.cpu is nil — no opinion
			cs.NumCPUs = 8 // class wrote 8
		})
		It("does not override class: CS keeps 8", func() {
			Expect(cs.NumCPUs).To(Equal(int32(8)))
		})
	})

	Context("resources.size.cpu — spec=4, live=4, class matches spec (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCPU = 4
			q := resource.MustParse("4")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			cs.NumCPUs = 4 // class already agrees with spec
		})
		It("does not re-write CS (spec==live==class, no-op)", func() {
			Expect(cs.NumCPUs).To(Equal(int32(4)))
		})
	})

	Context("resources.size.cpu — spec=4, live=4, no class change (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCPU = 4
			q := resource.MustParse("4")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{CPU: &q},
			}
			// cs.NumCPUs=0 (class did not change CPU)
		})
		It("does not write NumCPUs (spec==live, class uninvolved — no-op, no spurious reconfigure)", func() {
			Expect(cs.NumCPUs).To(Equal(int32(0)))
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — spec=4, live=4, class wrote 8 to CS (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
			cs.NumCoresPerSocket = ptr.To(int32(8)) // class wrote 8
		})
		It("overrides class value: writes spec's 4 to CS", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(4))))
		})
		It("does not add to blockedPowerOff (powered-off)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, vmx-20+, class wrote 4 to CS (powered-off)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(4))
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(false)
			cs.NumCoresPerSocket = ptr.To(int32(4)) // class wrote 4
		})
		It("writes 0 to CS: nil spec overrides class to request auto sizing", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(0))))
		})
	})

	// ───────────────────── resources.size.memory — nil spec / class override ─────────────────────

	Context("resources.size.memory — nil spec, powered-off", func() {
		BeforeEach(func() {
			poweredOn = false
			// spec.resources.size.memory is nil → don't manage
		})
		It("does not write MemoryMB (class/zero preserved)", func() {
			Expect(cs.MemoryMB).To(Equal(int64(0)))
		})
	})

	Context("resources.size.memory — spec=live=4096MB, class wrote 8192MB to CS (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.MemoryMB = 4096
			q := resource.MustParse("4Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Size: &vmopv1.VirtualMachineResourceQuantity{Memory: &q},
			}
			cs.MemoryMB = 8192
		})
		It("overrides class value: writes spec's 4096MB to CS", func() {
			Expect(cs.MemoryMB).To(Equal(int64(4096)))
		})
	})

	Context("resources.size.memory — nil spec, class wrote 8192MB to CS (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Hardware.MemoryMB = 4096
			cs.MemoryMB = 8192
		})
		It("does not override class: CS keeps 8192MB", func() {
			Expect(cs.MemoryMB).To(Equal(int64(8192)))
		})
	})

	// ───────────────────── latencySensitivity — nil spec / HighWithHyperthreading ─────────────────────

	Context("cpuAdvanced.latencySensitivity — nil spec, live=Normal (default)", func() {
		BeforeEach(func() {
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
			}
		})
		It("does not write LatencySensitivity (desired==live, no-op)", func() {
			Expect(cs.LatencySensitivity).To(BeNil())
		})
	})

	Context("cpuAdvanced.latencySensitivity — nil spec, live=High (reset to default)", func() {
		BeforeEach(func() {
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelHigh,
			}
		})
		It("writes LatencySensitivity Normal (nil spec=default, overrides live High)", func() {
			Expect(cs.LatencySensitivity).NotTo(BeNil())
			Expect(cs.LatencySensitivity.Level).To(Equal(vimtypes.LatencySensitivitySensitivityLevelNormal))
		})
	})

	Context("cpuAdvanced.latencySensitivity — powered-off, HighWithHyperthreading", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
			}
			ls := vmopv1.VirtualMachineLatencySensitivityHighWithHyperthreading
			cpuReq := resource.MustParse("4000")
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{LatencySensitivity: &ls}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{CPU: &cpuReq},
			}
		})
		It("writes LatencySensitivity High and SimultaneousThreads=2", func() {
			Expect(cs.LatencySensitivity).NotTo(BeNil())
			Expect(cs.LatencySensitivity.Level).To(Equal(vimtypes.LatencySensitivitySensitivityLevelHigh))
			Expect(cs.SimultaneousThreads).To(Equal(int32(2)))
		})
	})

	// ─── latencySensitivity — prerequisite defense-in-depth ─────────────────────────────────────

	Context("cpuAdvanced.latencySensitivity=High — no memory reservation (prereq not met)", func() {
		BeforeEach(func() {
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
			}
			cpuReq := resource.MustParse("4000")
			ls := vmopv1.VirtualMachineLatencySensitivityHigh
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{LatencySensitivity: &ls}
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Requests: &vmopv1.VirtualMachineResourceQuantity{CPU: &cpuReq},
			}
			// No memory reservation set.
		})
		It("does not write LatencySensitivity", func() {
			Expect(cs.LatencySensitivity).To(BeNil())
		})
		It("adds latencySensitivity to blocked with memory reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("cpuAdvanced.latencySensitivity")))
			Expect(blocked).To(ContainElement(ContainSubstring("full memory reservation required")))
		})
	})

	Context("cpuAdvanced.latencySensitivity=High — no CPU reservation (prereq not met)", func() {
		BeforeEach(func() {
			liveCI.LatencySensitivity = &vimtypes.LatencySensitivity{
				Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
			}
			ls := vmopv1.VirtualMachineLatencySensitivityHigh
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{LatencySensitivity: &ls}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			// No CPU reservation set.
		})
		It("does not write LatencySensitivity", func() {
			Expect(cs.LatencySensitivity).To(BeNil())
		})
		It("adds latencySensitivity to blocked with CPU reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("cpuAdvanced.latencySensitivity")))
			Expect(blocked).To(ContainElement(ContainSubstring("full CPU reservation required")))
		})
	})

	// ─── vnumaNodeCount — prerequisite defense-in-depth ──────────────────────────────────────

	Context("cpuAdvanced.topology.vnumaNodeCount > 0 — no coresPerSocket (prereq not met)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			n := int32(2)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{VNUMANodeCount: &n},
				// CoresPerSocket deliberately omitted.
			}
		})
		It("does not write VirtualNuma", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("adds vnumaNodeCount to blocked with coresPerSocket reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("cpuAdvanced.topology.vnumaNodeCount")))
			Expect(blocked).To(ContainElement(ContainSubstring("coresPerSocket")))
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — numCPUs not evenly divisible by vnumaNodeCount, size.cpu deferred to class", func() {
		// Gap scenario: spec.resources.size.cpu is left nil (deferring to the
		// class), so the webhook's validateComputeTopology skips its
		// divisibility check entirely. numCPUs is only known via cs.NumCPUs
		// (class-derived) or the live count — 6 CPUs / 4 nodes doesn't divide
		// evenly, which the webhook never gets to see.
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			cs.NumCPUs = 6 // simulates class/cpuSizeField having set this
			n := int32(4)
			cps := int32(4)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
					CoresPerSocket: &cps,
				},
			}
		})
		It("does not write VirtualNuma", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("adds vnumaNodeCount to blocked with a divisibility reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("cpuAdvanced.topology.vnumaNodeCount")))
			Expect(blocked).To(ContainElement(ContainSubstring("evenly divisible")))
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — derived coresPerNumaNode not a multiple/divisor of coresPerSocket", func() {
		// 12 CPUs / 4 nodes = 3 coresPerNumaNode, which is neither a multiple
		// nor a divisor of coresPerSocket=8.
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			cs.NumCPUs = 12
			n := int32(4)
			cps := int32(8)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
					CoresPerSocket: &cps,
				},
			}
		})
		It("does not write VirtualNuma", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
		It("adds vnumaNodeCount to blocked with a multiple-or-divisor reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("cpuAdvanced.topology.vnumaNodeCount")))
			Expect(blocked).To(ContainElement(ContainSubstring("multiple or divisor")))
		})
	})

	// ───────────────────── coresPerSocket — live=auto, class wrote non-zero ─────────────────────

	Context("cpuAdvanced.topology.coresPerSocket — nil spec, vmx-20+, live=auto, class wrote non-zero", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.Hardware.AutoCoresPerSocket = ptr.To(true) // live already in auto
			cs.NumCoresPerSocket = ptr.To(int32(4))           // class wrote non-auto
		})
		It("writes 0 to override class and maintain auto mode", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(0))))
		})
	})

	// ───────────────────── vnumaNodeCount — powered-on blocked ─────────────────────

	Context("cpuAdvanced.topology.vnumaNodeCount — hwVer >= vmx-20, powered-on, explicit spec differs", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = true
			liveCI.Hardware.NumCPU = 8
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				CoresPerNumaNode: ptr.To(int32(4)), // 8 CPUs / 4 nodes = 2 cores/node
			}
			n := int32(4) // 4 nodes → with 8 CPUs → desired 2 cores/node ≠ live 4
			cps := int32(2)
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					VNUMANodeCount: &n,
					CoresPerSocket: &cps,
				},
			}
		})
		It("adds vnumaNodeCount to blockedPowerOff (power-off required)", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.topology.vnumaNodeCount"))
		})
		It("does not write VirtualNuma", func() {
			Expect(cs.VirtualNuma).To(BeNil())
		})
	})

	Context("cpuAdvanced.topology.vnumaNodeCount — nil spec, class wrote non-zero CoresPerNumaNode", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX20.String()
			poweredOn = false
			liveCI.NumaInfo = &vimtypes.VirtualMachineVirtualNumaInfo{
				AutoCoresPerNumaNode: ptr.To(true),
			}
			cs.VirtualNuma = &vimtypes.VirtualMachineVirtualNuma{
				CoresPerNumaNode: ptr.To(int32(4)),
			}
		})
		It("writes CoresPerNumaNode=0 to override class and restore auto mode", func() {
			Expect(cs.VirtualNuma).NotTo(BeNil())
			Expect(cs.VirtualNuma.CoresPerNumaNode).To(Equal(ptr.To(int32(0))))
		})
	})

	// ───────────────────── iommuEnabled — nil spec ─────────────────────

	Context("cpuAdvanced.iommuEnabled — nil spec, live=false (default)", func() {
		It("does not write Flags.VvtdEnabled (desired==live, no-op)", func() {
			Expect(cs.Flags).To(BeNil())
		})
	})

	Context("cpuAdvanced.iommuEnabled — nil spec, live=true (reset to default)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.Flags = vimtypes.VirtualMachineFlagInfo{VvtdEnabled: ptr.To(true)}
		})
		It("writes Flags.VvtdEnabled=false (nil spec=default=false)", func() {
			Expect(cs.Flags).NotTo(BeNil())
			Expect(cs.Flags.VvtdEnabled).To(Equal(ptr.To(false)))
		})
	})

	Context("cpuAdvanced.iommuEnabled — class enabled it, nil spec, live=false (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			cs.Flags = &vimtypes.VirtualMachineFlagInfo{VvtdEnabled: ptr.To(true)}
		})
		It("writes Flags.VvtdEnabled=false (nil spec=default=false, overrides class)", func() {
			Expect(cs.Flags.VvtdEnabled).To(Equal(ptr.To(false)))
		})
	})

	// ───────────────────── nestedHardwareVirtualizationEnabled ─────────────────────

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — hwVer < vmx-9", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX8.String()
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				NestedHardwareVirtualizationEnabled: &v,
			}
		})
		It("does not write NestedHVEnabled (vSphere ignores incompatible fields)", func() {
			Expect(cs.NestedHVEnabled).To(BeNil())
		})
		It("adds nestedHardwareVirtualizationEnabled to blocked", func() {
			Expect(blocked).To(ContainElement("cpuAdvanced.nestedHardwareVirtualizationEnabled (requires hwVer >= 9)"))
		})
	})

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — powered-off, differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.NestedHVEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				NestedHardwareVirtualizationEnabled: &v,
			}
		})
		It("writes NestedHVEnabled=true", func() {
			Expect(cs.NestedHVEnabled).To(Equal(ptr.To(true)))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.NestedHVEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				NestedHardwareVirtualizationEnabled: &v,
			}
		})
		It("does not write NestedHVEnabled (power-off required)", func() {
			Expect(cs.NestedHVEnabled).To(BeNil())
		})
		It("adds nestedHardwareVirtualizationEnabled to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.nestedHardwareVirtualizationEnabled"))
		})
	})

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — nil spec, live=false (default)", func() {
		It("does not write NestedHVEnabled (desired==live, no-op)", func() {
			Expect(cs.NestedHVEnabled).To(BeNil())
		})
	})

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — nil spec, live=true (reset to default)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.NestedHVEnabled = ptr.To(true)
		})
		It("writes NestedHVEnabled=false (nil spec=default=false)", func() {
			Expect(cs.NestedHVEnabled).To(Equal(ptr.To(false)))
		})
	})

	Context("cpuAdvanced.nestedHardwareVirtualizationEnabled — class enabled it, nil spec, live=false (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			cs.NestedHVEnabled = ptr.To(true)
		})
		It("writes NestedHVEnabled=false (nil spec=default=false, overrides class)", func() {
			Expect(cs.NestedHVEnabled).To(Equal(ptr.To(false)))
		})
	})

	// ───────────────────── performanceCountersEnabled ─────────────────────

	Context("cpuAdvanced.performanceCountersEnabled — hwVer < vmx-9", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX8.String()
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				PerformanceCountersEnabled: &v,
			}
		})
		It("does not write VPMCEnabled (vSphere ignores incompatible fields)", func() {
			Expect(cs.VPMCEnabled).To(BeNil())
		})
		It("adds performanceCountersEnabled to blocked", func() {
			Expect(blocked).To(ContainElement("cpuAdvanced.performanceCountersEnabled (requires hwVer >= 9)"))
		})
	})

	Context("cpuAdvanced.performanceCountersEnabled — powered-off, differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.VPMCEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				PerformanceCountersEnabled: &v,
			}
		})
		It("writes VPMCEnabled=true", func() {
			Expect(cs.VPMCEnabled).To(Equal(ptr.To(true)))
		})
	})

	Context("cpuAdvanced.performanceCountersEnabled — powered-on, differs", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.VPMCEnabled = ptr.To(false)
			v := true
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				PerformanceCountersEnabled: &v,
			}
		})
		It("does not write VPMCEnabled (power-off required)", func() {
			Expect(cs.VPMCEnabled).To(BeNil())
		})
		It("adds performanceCountersEnabled to blockedPowerOff", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.performanceCountersEnabled"))
		})
	})

	Context("cpuAdvanced.performanceCountersEnabled — nil spec, live=false (default)", func() {
		It("does not write VPMCEnabled (desired==live, no-op)", func() {
			Expect(cs.VPMCEnabled).To(BeNil())
		})
	})

	Context("cpuAdvanced.performanceCountersEnabled — nil spec, live=true (reset to default)", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.VPMCEnabled = ptr.To(true)
		})
		It("writes VPMCEnabled=false (nil spec=default=false)", func() {
			Expect(cs.VPMCEnabled).To(Equal(ptr.To(false)))
		})
	})

	Context("cpuAdvanced.performanceCountersEnabled — class enabled it, nil spec, live=false (powered-off)", func() {
		BeforeEach(func() {
			poweredOn = false
			cs.VPMCEnabled = ptr.To(true)
		})
		It("writes VPMCEnabled=false (nil spec=default=false, overrides class)", func() {
			Expect(cs.VPMCEnabled).To(Equal(ptr.To(false)))
		})
	})

	// ───────────────────── memoryAdvanced.hotAddEnabled — powered-off ─────────────────────

	Context("memoryAdvanced.hotAddEnabled — hwVer >= vmx-7, powered-off, differs", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX7.String()
			poweredOn = false
			liveCI.MemoryHotAddEnabled = ptr.To(false)
			v := true
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{HotAddEnabled: &v}
		})
		It("writes MemoryHotAddEnabled=true", func() {
			Expect(cs.MemoryHotAddEnabled).To(Equal(ptr.To(true)))
		})
		It("does not add to blockedPowerOff", func() {
			Expect(blockedPO).To(BeEmpty())
		})
	})

	// ───────────────────── memoryAdvanced.reservationLockedToMax ─────────────────────

	Context("memoryAdvanced.reservationLockedToMax — powered-off, differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.MemoryReservationLockedToMax = ptr.To(false)
			v := true
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: &v,
			}
		})
		It("writes MemoryReservationLockedToMax=true", func() {
			Expect(cs.MemoryReservationLockedToMax).To(Equal(ptr.To(true)))
		})
	})

	Context("memoryAdvanced.reservationLockedToMax — nil spec, live=false (default)", func() {
		It("does not write MemoryReservationLockedToMax (desired==live, no-op)", func() {
			Expect(cs.MemoryReservationLockedToMax).To(BeNil())
		})
	})

	// ── memoryAdvanced.reservationLockedToMax — PCI passthrough/SR-IOV prerequisite ──

	Context("memoryAdvanced.reservationLockedToMax — existing PCI passthrough device, spec unset", func() {
		BeforeEach(func() {
			liveCI.Hardware.Device = []vimtypes.BaseVirtualDevice{&vimtypes.VirtualPCIPassthrough{}}
		})
		It("blocks with a prerequisite reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("memoryAdvanced.reservationLockedToMax")))
			Expect(blocked).To(ContainElement(ContainSubstring("PCI passthrough or SR-IOV")))
		})
		It("does not write MemoryReservationLockedToMax", func() {
			Expect(cs.MemoryReservationLockedToMax).To(BeNil())
		})
	})

	Context("memoryAdvanced.reservationLockedToMax — existing SR-IOV device, spec unset", func() {
		BeforeEach(func() {
			liveCI.Hardware.Device = []vimtypes.BaseVirtualDevice{&vimtypes.VirtualSriovEthernetCard{}}
		})
		It("blocks with a prerequisite reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("memoryAdvanced.reservationLockedToMax")))
		})
	})

	Context("memoryAdvanced.reservationLockedToMax — existing PCI passthrough device, spec explicitly true", func() {
		BeforeEach(func() {
			liveCI.Hardware.Device = []vimtypes.BaseVirtualDevice{&vimtypes.VirtualPCIPassthrough{}}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
		})
		It("does not block", func() {
			Expect(blocked).To(BeEmpty())
		})
		It("writes MemoryReservationLockedToMax=true", func() {
			Expect(cs.MemoryReservationLockedToMax).To(Equal(ptr.To(true)))
		})
	})

	Context("memoryAdvanced.reservationLockedToMax — PCI passthrough device being added this reconfigure, spec unset", func() {
		BeforeEach(func() {
			cs.DeviceChange = append(cs.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    &vimtypes.VirtualPCIPassthrough{},
			})
		})
		It("blocks with a prerequisite reason", func() {
			Expect(blocked).To(ContainElement(ContainSubstring("memoryAdvanced.reservationLockedToMax")))
		})
	})

	Context("memoryAdvanced.reservationLockedToMax — existing PCI passthrough device being removed this reconfigure, spec unset", func() {
		BeforeEach(func() {
			dev := &vimtypes.VirtualPCIPassthrough{}
			dev.Key = 4000
			liveCI.Hardware.Device = []vimtypes.BaseVirtualDevice{dev}

			removedDev := &vimtypes.VirtualPCIPassthrough{}
			removedDev.Key = 4000
			cs.DeviceChange = append(cs.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
				Device:    removedDev,
			})
		})
		It("does not block", func() {
			Expect(blocked).To(BeEmpty())
		})
	})

	Context("resources.allocation — powered-off, lock desired active, no spec reservation", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 8192
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(8192)),
				Limit:       ptr.To(int64(-1)),
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
		})
		It("does not write MemoryAllocation (vSphere owns the reservation)", func() {
			Expect(cs.MemoryAllocation).To(BeNil())
		})
	})

	Context("resources.allocation — powered-off, lock desired active, limit differs", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 8192
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(8192)),
				Limit:       ptr.To(int64(-1)),
			}
			vm.Spec.MemoryAdvanced = &vmopv1.VirtualMachineMemoryAdvancedSpec{
				ReservationLockedToMax: ptr.To(true),
			}
			lim := resource.MustParse("16Gi")
			vm.Spec.Resources = &vmopv1.VirtualMachineResourcesSpec{
				Limits: &vmopv1.VirtualMachineResourceQuantity{Memory: &lim},
			}
		})
		It("writes only Limit, not Reservation", func() {
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(BeNil())
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(16384))))
		})
	})

	Context("resources.allocation — powered-off, lock being unlocked this cycle, no explicit reservation", func() {
		BeforeEach(func() {
			poweredOn = false
			liveCI.MemoryReservationLockedToMax = ptr.To(true)
			liveCI.Hardware.MemoryMB = 8192
			liveCI.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
				Reservation: ptr.To(int64(8192)),
				Limit:       ptr.To(int64(-1)),
			}
		})
		It("writes an explicit Reservation instead of leaving the locked value stale", func() {
			Expect(cs.MemoryAllocation).NotTo(BeNil())
			Expect(cs.MemoryAllocation.Reservation).To(Equal(ptr.To(int64(0))))
			Expect(cs.MemoryAllocation.Limit).To(Equal(ptr.To(int64(-1))))
		})
	})

	// ───────────────────── cpuAdvanced.hotAddEnabled — class override ─────────────────────

	Context("cpuAdvanced.hotAddEnabled — class enabled it, nil spec, live=false (powered-off)", func() {
		BeforeEach(func() {
			liveCI.Version = vimtypes.VMX11.String()
			poweredOn = false
			cs.CpuHotAddEnabled = ptr.To(true)
		})
		It("writes CpuHotAddEnabled=false (nil spec=default=false, overrides class)", func() {
			Expect(cs.CpuHotAddEnabled).To(Equal(ptr.To(false)))
		})
	})

	// ───────────────────── multiple blocks ─────────────────────

	Context("multiple hw-incompatible and power-off fields — powered-on", func() {
		BeforeEach(func() {
			poweredOn = true
			liveCI.Version = vimtypes.VMX10.String() // below vmx-11

			// CPU hot-add flag (vmx-11 gate)
			cpuHotAdd := true
			// IOMMU (no hw gate, power-off required)
			iommu := true
			liveCI.Flags = vimtypes.VirtualMachineFlagInfo{VvtdEnabled: ptr.To(false)}
			// Cores per socket (power-off required)
			cps := int32(4)
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(2))

			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				HotAddEnabled: &cpuHotAdd,
				IOMMUEnabled:  &iommu,
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
		})
		It("puts hotAddEnabled in blocked (hw gate)", func() {
			Expect(blocked).To(ContainElement("cpuAdvanced.hotAddEnabled (requires hwVer >= 11)"))
		})
		It("puts iommuEnabled in blockedPowerOff (power-off required)", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.iommuEnabled"))
		})
		It("puts coresPerSocket in blockedPowerOff (power-off required)", func() {
			Expect(blockedPO).To(ContainElement("cpuAdvanced.topology.coresPerSocket"))
		})
	})

	// ───────────────────── powered-off: no blockedPowerOff ─────────────────────

	Context("powered-off — power-off-required fields are applied, not blocked", func() {
		BeforeEach(func() {
			poweredOn = false
			// All power-off-required fields differ from live
			iommu := true
			liveCI.Flags = vimtypes.VirtualMachineFlagInfo{VvtdEnabled: ptr.To(false)}
			cps := int32(4)
			liveCI.Hardware.NumCoresPerSocket = ptr.To(int32(2))
			vm.Spec.CPUAdvanced = &vmopv1.VirtualMachineCPUAdvancedSpec{
				IOMMUEnabled: &iommu,
				Topology: &vmopv1.VirtualMachineCPUTopologySpec{
					CoresPerSocket: &cps,
				},
			}
		})
		It("blockedPowerOff is empty (all applied)", func() {
			Expect(blockedPO).To(BeEmpty())
		})
		It("applies iommuEnabled", func() {
			Expect(cs.Flags).NotTo(BeNil())
			Expect(cs.Flags.VvtdEnabled).To(Equal(ptr.To(true)))
		})
		It("applies coresPerSocket", func() {
			Expect(cs.NumCoresPerSocket).To(Equal(ptr.To(int32(4))))
		})
	})
})

// resourceMHz creates a resource.Quantity representing the given integer value (used for
// MHz-based CPU allocation in tests).
func resourceMHz(mhz int64) *resource.Quantity {
	q := resource.NewQuantity(mhz, resource.DecimalSI)
	return q
}
