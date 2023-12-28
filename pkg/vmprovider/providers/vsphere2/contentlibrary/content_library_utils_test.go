// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/ovf"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("ParseVirtualHardwareVersion", func() {
	It("empty hardware string", func() {
		vmxHwVersionString := ""
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("invalid hardware string", func() {
		vmxHwVersionString := "blah"
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(BeZero())
	})

	It("valid hardware version string eg. vmx-15", func() {
		vmxHwVersionString := "vmx-15"
		Expect(contentlibrary.ParseVirtualHardwareVersion(vmxHwVersionString)).To(Equal(int32(15)))
	})
})

var _ = Describe("UpdateVmiWithOvfEnvelope", func() {
	const (
		ovfStringType          = "string"
		userConfigurableKey    = "dummy-key-configurable"
		notUserConfigurableKey = "dummy-key-not-configurable"
		defaultValue           = "dummy-value"
		versionKey             = "vmware-system.tkr.os-version"
		versionVal             = "1.15"
	)

	var (
		ovfEnvelope ovf.Envelope
		image       *v1alpha2.VirtualMachineImage
	)

	BeforeEach(func() {
		ovfEnvelope = ovf.Envelope{
			VirtualSystem: &ovf.VirtualSystem{
				Product: []ovf.ProductSection{
					{
						Vendor:      "vendor",
						Product:     "product",
						FullVersion: "fullVersion",
						Version:     "version",
						Property: []ovf.Property{
							{
								Key:     versionKey,
								Type:    ovfStringType,
								Default: pointer.String(versionVal),
							},
							{
								Key:              userConfigurableKey,
								Type:             ovfStringType,
								Default:          pointer.String(defaultValue),
								UserConfigurable: pointer.Bool(true),
							},
							{
								Key:              notUserConfigurableKey,
								Type:             ovfStringType,
								Default:          pointer.String(defaultValue),
								UserConfigurable: pointer.Bool(false),
							},
							{
								Key:              notUserConfigurableKey,
								Type:             ovfStringType,
								Default:          pointer.String(defaultValue),
								UserConfigurable: pointer.Bool(false),
							},
						},
					},
				},
				OperatingSystem: []ovf.OperatingSystemSection{
					{
						OSType:  pointer.String("dummy_os_type"),
						ID:      int16(100),
						Version: pointer.String("dummy_version"),
					},
				},
				VirtualHardware: []ovf.VirtualHardwareSection{
					{
						Config: []ovf.Config{
							{
								Key:   "firmware",
								Value: "efi",
							},
						},

						System: &ovf.VirtualSystemSettingData{
							CIMVirtualSystemSettingData: ovf.CIMVirtualSystemSettingData{
								VirtualSystemType: pointer.String("vmx-10"),
							},
						},
					},
				},
			},
		}

		image = builder.DummyVirtualMachineImageA2("dummy-image")
	})

	AfterEach(func() {
		ovfEnvelope = ovf.Envelope{}
		image = nil
	})

	JustBeforeEach(func() {
		contentlibrary.UpdateVmiWithOvfEnvelope(image, ovfEnvelope)
	})

	It("Image status should have expected ProductInfo, OSInfo, Firmware, User configurable and System Properties", func() {
		Expect(image).ToNot(BeNil())
		Expect(image.Name).Should(Equal("dummy-image"))

		Expect(image.Status.ProductInfo.Vendor).Should(Equal("vendor"))
		Expect(image.Status.ProductInfo.Product).Should(Equal("product"))
		Expect(image.Status.ProductInfo.Version).Should(Equal("version"))
		Expect(image.Status.ProductInfo.FullVersion).Should(Equal("fullVersion"))

		Expect(image.Status.OSInfo.Type).Should(Equal("dummy_os_type"))
		Expect(image.Status.OSInfo.Version).Should(Equal("dummy_version"))
		Expect(image.Status.OSInfo.ID).Should(Equal("100"))

		Expect(image.Status.HardwareVersion).Should(Equal(pointer.Int32(10)))
		Expect(image.Status.Firmware).Should(Equal("efi"))

		Expect(image.Status.OVFProperties).Should(HaveLen(1))
		Expect(image.Status.OVFProperties[0].Key).Should(Equal(userConfigurableKey))
		Expect(image.Status.OVFProperties[0].Type).Should(Equal(ovfStringType))
		Expect(image.Status.OVFProperties[0].Default).Should(Equal(pointer.String(defaultValue)))

		Expect(image.Status.VMwareSystemProperties).Should(HaveLen(1))
		Expect(image.Status.VMwareSystemProperties[0].Key).Should(Equal(versionKey))
		Expect(image.Status.VMwareSystemProperties[0].Value).Should(Equal(versionVal))
	})
})
