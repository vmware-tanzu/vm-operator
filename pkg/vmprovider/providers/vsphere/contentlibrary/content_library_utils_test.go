// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
)

var _ = Describe("LibItemToVirtualMachineImage", func() {
	const (
		versionKey = "vmware-system-version"
		versionVal = "1.15"
	)

	var (
		// FSS related to UnifiedTKG. This FSS should be manipulated atomically to avoid races between tests and
		// provider.
		unifiedTKGFSS uint32
	)

	BeforeEach(func() {
		lib.IsUnifiedTKGFSSEnabled = func() bool {
			return atomic.LoadUint32(&unifiedTKGFSS) != 0
		}
	})

	Context("Expose ovfEnv properties", func() {
		const (
			ovfStringType          = "string"
			userConfigurableKey    = "dummy-key-configurable"
			notUserConfigurableKey = "dummy-key-not-configurable"
			defaultValue           = "dummy-value"
		)

		var (
			ts          time.Time
			item        *library.Item
			ovfEnvelope *ovf.Envelope
			image       *vmopv1.VirtualMachineImage
		)

		BeforeEach(func() {
			ts = time.Now()

			item = &library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}

			ovfEnvelope = &ovf.Envelope{
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
							},
						},
					},
				},
			}
		})

		AfterEach(func() {
			item = nil
			ovfEnvelope = nil
			image = nil
		})

		JustBeforeEach(func() {
			image = contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
		})

		It("returns a VirtualMachineImage with expected annotations and ovfEnv", func() {
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

		When("There is no description or label", func() {
			It("should return an empty Description and Label", func() {
				Expect(image.Spec.OVFEnv[userConfigurableKey].Description).Should(BeEmpty())
				Expect(image.Spec.OVFEnv[userConfigurableKey].Label).Should(BeEmpty())
			})
		})

		When("There is a description", func() {
			BeforeEach(func() {
				ovfEnvelope.VirtualSystem.Product[0].Property[1].Description = pointer.String("description")
			})
			It("should return the Description", func() {
				Expect(image.Spec.OVFEnv[userConfigurableKey].Description).Should(Equal("description"))
			})
		})

		When("There is a label", func() {
			BeforeEach(func() {
				ovfEnvelope.VirtualSystem.Product[0].Property[1].Label = pointer.String("label")
			})
			It("should return the Label", func() {
				Expect(image.Spec.OVFEnv[userConfigurableKey].Label).Should(Equal("label"))
			})
		})

		When("There is a description and a label", func() {
			BeforeEach(func() {
				ovfEnvelope.VirtualSystem.Product[0].Property[1].Description = pointer.String("description")
				ovfEnvelope.VirtualSystem.Product[0].Property[1].Label = pointer.String("label")
			})
			It("should return the Description and the Label", func() {
				Expect(image.Spec.OVFEnv[userConfigurableKey].Description).Should(Equal("description"))
				Expect(image.Spec.OVFEnv[userConfigurableKey].Label).Should(Equal("label"))
			})
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

		When("WCP_Unified_TKG FSS is enabled", func() {
			var (
				oldUnifiedTKGFSSState uint32
			)
			BeforeEach(func() {
				oldUnifiedTKGFSSState = unifiedTKGFSS
				atomic.StoreUint32(&unifiedTKGFSS, 1)
			})

			AfterEach(func() {
				atomic.StoreUint32(&unifiedTKGFSS, oldUnifiedTKGFSSState)
			})

			It("ImageSupported should be set to true when OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig and WCP_Unified_TKG FSS is set ", func() {
				ovfEnvelope := &ovf.Envelope{
					VirtualSystem: &ovf.VirtualSystem{
						Product: []ovf.ProductSection{
							{
								Property: []ovf.Property{
									{
										Key:     "someKey",
										Default: pointer.String("someRandom"),
									},
								},
							},
						},
					},
				}

				image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
				Expect(image).ToNot(BeNil())
				Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(true)))

				Expect(image.Status.Conditions).Should(BeEmpty())
			})

		})

		When("WCP_Unified_TKG FSS is not enabled", func() {
			var (
				oldUnifiedTKGFSSState uint32
			)
			BeforeEach(func() {
				oldUnifiedTKGFSSState = unifiedTKGFSS
				atomic.StoreUint32(&unifiedTKGFSS, 0)
			})

			AfterEach(func() {
				atomic.StoreUint32(&unifiedTKGFSS, oldUnifiedTKGFSSState)
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
										Default: pointer.String("someRandom"),
									},
								},
							},
						},
					},
				}

				image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
				Expect(image).ToNot(BeNil())

				Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(true)))
				expectedCondition := vmopv1.Conditions{
					*conditions.TrueCondition(vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition),
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
										Default: pointer.String("someRandom"),
									},
								},
							},
						},
					},
				}

				image := contentlibrary.LibItemToVirtualMachineImage(item, ovfEnvelope)
				Expect(image).ToNot(BeNil())
				Expect(image.Status.ImageSupported).Should(Equal(pointer.Bool(false)))

				expectedCondition := vmopv1.Conditions{
					*conditions.FalseCondition(vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition,
						vmopv1.VirtualMachineImageV1Alpha1NotCompatibleReason,
						vmopv1.ConditionSeverityError,
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
				expectedCondition := vmopv1.Conditions{
					*conditions.TrueCondition(vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition),
				}
				Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
			})
		})
	})
})
