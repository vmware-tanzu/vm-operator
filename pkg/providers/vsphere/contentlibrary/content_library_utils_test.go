// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"context"
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func assertImage(image *vmopv1.VirtualMachineImage, diskName string) {
	ExpectWithOffset(1, image).ToNot(BeNil())
	ExpectWithOffset(1, image.Name).Should(Equal("dummy-image"))

	ExpectWithOffset(1, image.Status.ProductInfo.Vendor).Should(Equal("LinuxVendor"))
	ExpectWithOffset(1, image.Status.ProductInfo.Product).Should(Equal("Linux"))
	ExpectWithOffset(1, image.Status.ProductInfo.Version).Should(Equal("v1"))
	ExpectWithOffset(1, image.Status.ProductInfo.FullVersion).Should(Equal("v1.0.0"))

	ExpectWithOffset(1, image.Status.OSInfo.Type).Should(Equal("otherLinuxGuest"))
	Expect(image.Status.OSInfo.ID).Should(Equal("36"))

	ExpectWithOffset(1, image.Status.HardwareVersion).Should(Equal(ptr.To[int32](9)))
	ExpectWithOffset(1, image.Status.Firmware).Should(Equal("efi"))

	ExpectWithOffset(1, image.Status.OVFProperties).Should(HaveLen(1))
	ExpectWithOffset(1, image.Status.OVFProperties[0].Key).Should(Equal("dummy-key-configurable"))
	ExpectWithOffset(1, image.Status.OVFProperties[0].Type).Should(Equal("string"))
	ExpectWithOffset(1, image.Status.OVFProperties[0].Default).Should(Equal(ptr.To("dummy-value")))

	ExpectWithOffset(1, image.Status.VMwareSystemProperties).Should(HaveLen(1))
	ExpectWithOffset(1, image.Status.VMwareSystemProperties[0].Key).Should(Equal("vmware-system.tkr.os-version"))
	ExpectWithOffset(1, image.Status.VMwareSystemProperties[0].Value).Should(Equal("1.15"))

	ExpectWithOffset(1, conditions.Has(image, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)).To(BeFalse())

	ExpectWithOffset(1, image.Status.Disks).To(HaveLen(1))
	ExpectWithOffset(1, image.Status.Disks[0].Name).To(Equal(diskName))
	ExpectWithOffset(1, image.Status.Disks[0].Requested.String()).To(Equal("18743296"))
	ExpectWithOffset(1, image.Status.Disks[0].Limit.String()).To(Equal("30Mi"))

	ExpectWithOffset(1, image.Labels).To(HaveKeyWithValue("example.com/hello", "world"))
	ExpectWithOffset(1, image.Labels).To(HaveKeyWithValue("fu.bar", ""))
	ExpectWithOffset(1, image.Labels).To(HaveKeyWithValue("another.example.com", "true"))
}

var _ = Describe("UpdateVmiWithOvfEnvelope", func() {
	var (
		ovfEnvelope *ovf.Envelope
		image       *vmopv1.VirtualMachineImage
	)

	BeforeEach(func() {
		image = builder.DummyVirtualMachineImage("dummy-image")

		f, err := os.Open(path.Join(
			testutil.GetRootDirOrDie(),
			"test", "builder", "testdata",
			"images", "ttylinux-pc_i486-16.1.ovf"))
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			_ = f.Close()
		}()
		ovfEnvelope, err = ovf.Unmarshal(f)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		contentlibrary.UpdateVmiWithOvfEnvelope(image, *ovfEnvelope)
	})

	It("Image should have expected properties", func() {
		assertImage(image, "disk0")
	})

	It("Repeated calls should not duplicate items", func() {
		Expect(image.Status.Disks).ToNot(BeEmpty())
		Expect(image.Status.OVFProperties).ToNot(BeEmpty())
		Expect(image.Status.VMwareSystemProperties).ToNot(BeEmpty())

		savedImage := image.DeepCopy()
		contentlibrary.UpdateVmiWithOvfEnvelope(image, *ovfEnvelope)
		Expect(image).To(Equal(savedImage))
	})

	Context("Image is V1Alpha1Compatible", func() {
		BeforeEach(func() {
			ovfEnvelope.VirtualSystem.VirtualHardware[0].ExtraConfig = append(ovfEnvelope.VirtualSystem.VirtualHardware[0].ExtraConfig,
				ovf.Config{
					Key:   constants.VMOperatorV1Alpha1ExtraConfigKey,
					Value: constants.VMOperatorV1Alpha1ConfigReady,
				},
			)
		})

		It("V1Alpha1Compatible condition is true", func() {
			Expect(conditions.IsTrue(image, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)).To(BeTrue())
		})
	})
})

var _ = Describe("UpdateVmiWithVirtualMachine", func() {

	const diskUUID = "552e3da8-c1c9-415c-af24-cb60d7c450fa"

	var (
		moVM  mo.VirtualMachine
		image *vmopv1.VirtualMachineImage
	)

	BeforeEach(func() {
		image = builder.DummyVirtualMachineImage("dummy-image")
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{
				GuestId:  "otherLinuxGuest",
				Version:  "vmx-9",
				Firmware: "efi",
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualDisk{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 500,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									Uuid: diskUUID,
								},
							},
							CapacityInBytes: 30 * 1024 * 1024,
						},
					},
				},
				ExtraConfig: []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   pkgconst.VirtualMachineImageExtraConfigLabelsKey,
						Value: "example.com/hello:world,fu.bar:,another.example.com:true",
					},
				},
				VAppConfig: &vimtypes.VAppConfigInfo{
					VmConfigInfo: vimtypes.VmConfigInfo{
						Product: []vimtypes.VAppProductInfo{
							{
								Name:        "Linux",
								Vendor:      "LinuxVendor",
								Version:     "v1",
								FullVersion: "v1.0.0",
							},
						},
						Property: []vimtypes.VAppPropertyInfo{
							{
								Id:           "vmware-system.tkr.os-version",
								Type:         "string",
								DefaultValue: "1.15",
							},
							{
								Id:               "dummy-key-configurable",
								Type:             "string",
								DefaultValue:     "dummy-value",
								UserConfigurable: ptr.To(true),
							},
							{
								Id:               "dummy-key-not-configurable",
								Type:             "string",
								DefaultValue:     "dummy-value",
								UserConfigurable: ptr.To(false),
							},
						},
					},
				},
			},
			Guest: &vimtypes.GuestInfo{
				GuestId:     "otherLinuxGuest",
				GuestFamily: string(vimtypes.VirtualMachineGuestOsFamilyLinuxGuest),
			},
			LayoutEx: &vimtypes.VirtualMachineFileLayoutEx{
				Disk: []vimtypes.VirtualMachineFileLayoutExDiskLayout{
					{
						Key: 500,
						Chain: []vimtypes.VirtualMachineFileLayoutExDiskUnit{
							{
								FileKey: []int32{
									4,
								},
							},
						},
					},
				},
				File: []vimtypes.VirtualMachineFileLayoutExFileInfo{
					{
						Key:        4,
						Size:       18743296,
						UniqueSize: 18743296,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		contentlibrary.UpdateVmiWithVirtualMachine(
			context.Background(), image, moVM)
	})

	It("Image should have expected properties", func() {
		assertImage(image, diskUUID)
	})

	It("Repeated calls should not duplicate items", func() {
		Expect(image.Status.Disks).ToNot(BeEmpty())
		Expect(image.Status.OVFProperties).ToNot(BeEmpty())
		Expect(image.Status.VMwareSystemProperties).ToNot(BeEmpty())

		savedImage := image.DeepCopy()
		contentlibrary.UpdateVmiWithVirtualMachine(
			context.Background(), image, moVM)
		Expect(image).To(Equal(savedImage))
	})

	Context("Image is V1Alpha1Compatible", func() {
		BeforeEach(func() {
			moVM.Config.ExtraConfig = append(moVM.Config.ExtraConfig,
				&vimtypes.OptionValue{
					Key:   constants.VMOperatorV1Alpha1ExtraConfigKey,
					Value: constants.VMOperatorV1Alpha1ConfigReady,
				},
			)
		})

		It("V1Alpha1Compatible condition is true", func() {
			Expect(conditions.IsTrue(image, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)).To(BeTrue())
		})
	})
})
