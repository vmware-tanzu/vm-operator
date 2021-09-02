// +build !integration

// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
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

var _ = Describe("LibItemToVirtualMachineImage", func() {
	const (
		versionKey = "vmware-system-version"
		versionVal = "1.15"
	)

	Context("Expose ovfEnv properties", func() {
		const (
			ovfStringType            = "string"
			userConfigurableKey      = "dummy-key-configurable"
			notUserConfigurableKey   = "dummy-key-not-configurable"
			defaultValue             = "dummy-value"
		)

		It("returns a VirtualMachineImage with expected annotations and ovfEnv", func() {
			ts := time.Now()
			item := &library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}

			ovfEnvelope := &ovf.Envelope{
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
									UserConfigurable: pointer.BoolPtr(true),
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
				},
			}

			image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			Expect(image.Annotations).To(HaveLen(2))
			Expect(image.Annotations).To(HaveKey(constants.VMImageCLVersionAnnotation))
			Expect(image.Annotations).Should(HaveKeyWithValue(versionKey, versionVal))
			Expect(image.CreationTimestamp).To(BeEquivalentTo(metav1.NewTime(ts)))

			Expect(image.Spec.ProductInfo.Vendor).Should(Equal("vendor"))
			Expect(image.Spec.ProductInfo.Product).Should(Equal("product"))
			Expect(image.Spec.ProductInfo.Version).Should(Equal("version"))
			Expect(image.Spec.ProductInfo.FullVersion).Should(Equal("fullVersion"))

			Expect(image.Spec.OVFEnv).Should(HaveLen(1))
			Expect(image.Spec.OVFEnv).Should(HaveKey(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Key).Should(Equal(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Type).Should(Equal(ovfStringType))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Default).Should(Equal(pointer.String(defaultValue)))
		})
	})

	Context("LibItemToVirtualMachineImage, ImageCompatibility and SupportedGuestOS", func() {
		var item *library.Item

		BeforeEach(func() {
			ts := time.Now()
			item = &library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}
		})

		It("with vmtx type", func() {
			item.Type = "vmtx"
			image := contentlibrary.LibItemToVirtualMachineImage(item, nil)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(HaveKey(constants.VMImageCLVersionAnnotation))

			// ImageSupported in Status is unset as the image type is not OVF type
			Expect(image.Status.ImageSupported).Should(BeNil())
			Expect(image.Status.Conditions).Should(BeEmpty())
		})

		It("ImageSupported should be set to true when it is a TKG image and valid OS Type is set and OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig", func() {
			tkgKey := "vmware-system.guest.kubernetes.distribution.image.version"

			ovfEnvelope := &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: pointer.String("dummy_valid_os_type"),
						},
					},
					Product: []ovf.ProductSection{
						{
							Property: []ovf.Property{
								{
									Key:     tkgKey,
									Default: pointer.StringPtr("someRandom"),
								},
							},
						},
					},
				},
			}

			image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
			Expect(image).ToNot(BeNil())

			Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(true)))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ImageSupported should be set to false when OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig and is not a TKG image and has a valid OS type set", func() {
			ovfEnvelope := &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					Product: []ovf.ProductSection{
						{
							Property: []ovf.Property{
								{
									Key:     "someKey",
									Default: pointer.StringPtr("someRandom"),
								},
							},
						},
					},
				},
			}

			image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
			Expect(image).ToNot(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(false)))

			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					"VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1"),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ImageSupported should be set to true when OVF Envelope has VMOperatorV1Alpha1ExtraConfigKey set to VMOperatorV1Alpha1ConfigReady in extraConfig and has a valid OS type set", func() {
			ovfEnvelope := &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					VirtualHardware: []ovf.VirtualHardwareSection{
						{
							ExtraConfig: []ovf.Config{
								{
									Key:   constants.VMOperatorV1Alpha1ExtraConfigKey,
									Value: constants.VMOperatorV1Alpha1ConfigReady,
								},
							},
						},
					},
				},
			}

			image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
			Expect(image).ToNot(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(true)))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})
	})
})