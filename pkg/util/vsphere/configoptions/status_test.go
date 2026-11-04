// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configoptions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/configoptions"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
)

var _ = Describe("Flex class discovery inputs", Label(testlabels.API), func() {
	It("publishes CPU topology, memory, SGX, device groups and configuration capabilities", func() {
		in := &vimtypes.VirtualMachineConfigOption{
			HardwareOptions: vimtypes.VirtualHardwareOption{
				HwVersion:         22,
				NumCPU:            []int32{2, 4, 8},
				NumCoresPerSocket: vimtypes.IntOption{Min: 1, Max: 8, DefaultValue: 2},
				MemoryMB:          vimtypes.LongOption{Min: 128, Max: 8192, DefaultValue: 1024},
				EpcMemoryMB:       &vimtypes.LongOption{Min: 0, Max: 256, DefaultValue: 64},
				DeviceGroupTypes:  []string{"VendorDeviceGroup"},
			},
			Capabilities: vimtypes.VirtualMachineCapability{
				NestedHVSupported: true, VPMCSupported: true, MemoryReservationLockSupported: true,
				BootRetryOptionsSupported: true, ChangeTrackingSupported: true, CpuFeatureMaskSupported: true,
				ToolsAutoUpdateSupported: true, SwapPlacementSupported: true, GuestAutoLockSupported: true,
				SecureBootSupported: ptr.To(false), PmemFailoverSupported: ptr.To(true),
			},
		}
		obj := &vimv1.VirtualMachineConfigOptions{}
		configoptions.PopulateStatus(obj, in)
		hw, caps := obj.Status.HardwareOptions, obj.Status.Capabilities
		Expect(hw.HardwareVersion).To(Equal("vmx-22"))
		Expect(hw.NumCoresPerSocket).To(Equal(&vimv1.IntOption{IntRange: vimv1.IntRange{Min: 1, Max: 8}, Default: 2}))
		Expect(hw.Memory.Max.Cmp(resource.MustParse("8Gi"))).To(BeZero())
		Expect(hw.Memory.Default.Cmp(resource.MustParse("1Gi"))).To(BeZero())
		Expect(hw.EpcMemory.Max.Cmp(resource.MustParse("256Mi"))).To(BeZero())
		Expect(hw.DeviceGroupTypes).To(Equal([]string{"VendorDeviceGroup"}))
		Expect([]bool{caps.NestedHVSupported, caps.VPMCSupported, caps.MemoryReservationLockSupported, caps.BootRetryOptionsSupported, caps.ChangeTrackingSupported, caps.CpuFeatureMaskSupported, caps.ToolsAutoUpdateSupported, caps.SwapPlacementSupported, caps.GuestAutoLockSupported}).To(HaveEach(BeTrue()))
		Expect(caps.SecureBootSupported).To(Equal(ptr.To(false)))
		Expect(caps.PMEMFailoverSupported).To(Equal(ptr.To(true)))
		in.Capabilities.SecureBootSupported = nil
		in.HardwareOptions.EpcMemoryMB = nil
		configoptions.PopulateStatus(obj, in)
		Expect(obj.Status.Capabilities.SecureBootSupported).To(BeNil())
		Expect(obj.Status.HardwareOptions.EpcMemory).To(BeNil())
	})

	It("maps guest firmware, hotplug, IOMMU, SGX and persistent memory with correct units", func() {
		desc := vimtypes.GuestOsDescriptor{
			SupportedMaxCPUs: 64, NumSupportedCoresPerSocket: 32, SupportedMinMemMB: 512, SupportedMaxMemMB: 8192,
			SupportsCpuHotAdd: true, SupportsCpuHotRemove: true, SupportsMemoryHotAdd: true,
			SupportedFirmware: []string{"bios", "efi"}, RecommendedFirmware: "efi",
			SupportsSecureBoot: ptr.To(false), DiskUuidEnabled: true, SmcRequired: true,
			VvtdSupported:             &vimtypes.BoolOption{Supported: true, DefaultValue: false},
			VsgxSupported:             &vimtypes.BoolOption{Supported: true, DefaultValue: true},
			PersistentMemorySupported: ptr.To(true), SupportedMaxPersistentMemoryMB: 1024,
			VRAMSizeInKB:          vimtypes.IntOption{Min: 1024, Max: 4096, DefaultValue: 2048},
			SupportedEthernetCard: []string{"vmxnet3"}, SupportedDiskControllerList: []string{"pvscsi", "VirtualNVMEController"},
		}
		got := configoptions.GuestStatus("vmx-22", desc)
		Expect(got.SupportedMaxCPUs).To(Equal(int32(64)))
		Expect(got.SupportedMaxMem.Value()).To(Equal(int64(8 * 1024 * 1024 * 1024)))
		Expect(got.SupportedFirmware).To(Equal([]vimv1.Firmware{vimv1.FirmwareBIOS, vimv1.FirmwareEFI}))
		Expect(got.RecommendedFirmware).To(Equal(vimv1.FirmwareEFI))
		Expect([]bool{got.SupportsCpuHotAdd, got.SupportsCpuHotRemove, got.SupportsMemoryHotAdd, got.DiskUuidEnabled, got.SmcRequired}).To(HaveEach(BeTrue()))
		Expect(got.VvtdSupported.Supported).To(BeTrue())
		Expect(got.VsgxSupported.Default).To(BeTrue())
		Expect(got.SupportsSecureBoot).To(Equal(ptr.To(false)))
		Expect(got.PersistentMemorySupported).To(Equal(ptr.To(true)))
		Expect(got.SupportedMaxPersistentMemory.Value()).To(Equal(int64(1024 * 1024 * 1024)))
		Expect(got.VRAMSize.Max.Cmp(resource.MustParse("4Mi"))).To(BeZero())
		Expect(got.SupportedEthernetCards).To(Equal([]vimv1.EthernetCardType{vimv1.EthernetCardTypeVirtualVmxnet3}))
		Expect(got.SupportedDiskControllers).To(Equal([]vimv1.VirtualControllerType{vimv1.VirtualControllerTypeParaVirtualSCSI, vimv1.VirtualControllerTypeNVMe}))
		empty := configoptions.GuestStatus("vmx-22", vimtypes.GuestOsDescriptor{})
		Expect(empty.VvtdSupported).To(BeNil())
		Expect(empty.SupportedMaxMem).To(BeNil())
	})

	It("converts the recorded vmx-22 EnvironmentBrowser response", func() {
		data, err := os.ReadFile("../../../../external/vim/api/v1alpha1/testdata/xml/normal/config-option-vmx-22.xml")
		Expect(err).NotTo(HaveOccurred())
		var in vimtypes.VirtualMachineConfigOption
		Expect(xml.Unmarshal(data, &in)).To(Succeed())
		obj := &vimv1.VirtualMachineConfigOptions{}
		configoptions.PopulateStatus(obj, &in)
		Expect(obj.Status.HardwareOptions.NumCPU).NotTo(BeEmpty())
		Expect(obj.Status.Capabilities).NotTo(BeNil())
		Expect(in.GuestOSDescriptor).NotTo(BeEmpty())
		for _, desc := range in.GuestOSDescriptor {
			got := configoptions.GuestStatus(in.Version, desc)
			Expect(got.SupportedMaxCPUs).To(Equal(desc.SupportedMaxCPUs))
			Expect(got.SupportedFirmware).To(HaveLen(len(desc.SupportedFirmware)))
			Expect(got.SupportsCpuHotRemove).To(Equal(desc.SupportsCpuHotRemove))
		}
	})
})
