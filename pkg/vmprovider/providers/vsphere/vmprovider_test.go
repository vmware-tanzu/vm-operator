// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var _ = Describe("VirtualMachineImages", func() {

	var (
		versionKey = "vmware-system-version"
		versionVal = "1.15"
	)

	Context("when ovf info is present", func() {
		It("returns a VirtualMachineImage object from a content library with annotations and ovf info", func() {
			ts := time.Now()
			item := library.Item{
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
							Version:     "version",
							FullVersion: "fullVersion",
							Property: []ovf.Property{
								{
									Key:     versionKey,
									Default: &versionVal,
								},
							},
						}},
				},
			}

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, nil)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(HaveLen(2))
			Expect(image.Annotations).To(HaveKey(vsphere.VMImageCLVersionAnnotation))
			Expect(image.Annotations).Should(HaveKeyWithValue("vmware-system-version", "1.15"))
			Expect(image.CreationTimestamp).To(BeEquivalentTo(metav1.NewTime(ts)))

			Expect(image.Spec.ProductInfo.Vendor).Should(Equal("vendor"))
			Expect(image.Spec.ProductInfo.Product).Should(Equal("product"))
			Expect(image.Spec.ProductInfo.Version).Should(Equal("version"))
			Expect(image.Spec.ProductInfo.FullVersion).Should(Equal("fullVersion"))
		})

		It("returns a VirtualMachineImage object from an inventory VM with annotations", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				// TODO: Need to convert this VM to a vApp (and back).
				annotations := map[string]string{}
				annotations[versionKey] = versionVal

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), resVm)
				Expect(err).ToNot(HaveOccurred())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				//Expect(image.Annotations).ToNot(BeEmpty())
				//Expect(image.Annotations).To(HaveKeyWithValue(versionKey, versionVal))
			})
		})

		Context("when ovf info is absent", func() {
			It("returns a VirtualMachineImage object from a content library with annotations and no ovf info", func() {
				ts := time.Now()
				item := library.Item{
					Name:         "fakeItem",
					Type:         "ovf",
					LibraryID:    "fakeID",
					CreationTime: &ts,
				}

				ovfEnvelope := &ovf.Envelope{}

				image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, nil)
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal("fakeItem"))
				Expect(image.Annotations).To(HaveKey(vsphere.VMImageCLVersionAnnotation))
				Expect(image.CreationTimestamp).To(BeEquivalentTo(metav1.NewTime(ts)))
				Expect(image.Spec.ProductInfo.Version).Should(BeEmpty())
			})
		})
	})

	Context("LibItemToVirtualMachineImage, ImageCompatibility and SupportedGuestOS", func() {
		var (
			item                        library.Item
			ovfEnvelope                 *ovf.Envelope
			dummyValidOsType            = "dummy_valid_os_type"
			dummyEmptyOsType            = ""
			dummyWindowsOSType          = "dummy_win_os"
			dummyLinuxFamily            = string(types.VirtualMachineGuestOsFamilyLinuxGuest)
			dummyWindowsFamily          = string(types.VirtualMachineGuestOsFamilyWindowsGuest)
			supportedGuestOsIdsToFamily map[string]string
			supportedFalse              = new(bool)
			supportedTrue               = new(bool)
			notCompatibleMsg            = "VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1"
		)

		BeforeEach(func() {
			ts := time.Now()

			item = library.Item{
				Name:         "fakeItem",
				Type:         "ovf",
				LibraryID:    "fakeID",
				CreationTime: &ts,
			}

			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyValidOsType,
						},
					},
				},
			}

			supportedGuestOsIdsToFamily = make(map[string]string)
			// supported guestOSIds fetched from the cluster
			supportedGuestOsIdsToFamily[dummyValidOsType] = dummyLinuxFamily
			supportedGuestOsIdsToFamily[dummyWindowsOSType] = dummyWindowsFamily

			supportedTrue = pointer.BoolPtr(true)
			supportedFalse = pointer.BoolPtr(false)
		})

		It("ovfEnvelope has a valid GuestOSType and OVF is not a TKG Image and is not v1alpha1 compatible", func() {
			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does not
			// contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ovfEnvelope has a empty string GuestOSType and OVF is not TKG image and is not v1alpha1 compatible", func() {
			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyEmptyOsType,
						}},
				},
			}

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does not contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					vmopv1alpha1.VirtualMachineImageOSTypeNotSupportedReason,
					vmopv1alpha1.ConditionSeverityError,
					fmt.Sprintf("VirtualMachineImage image type %s is not supported by VMService", "")),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ovfEnvelope has an invalid windows GuestOSType and OVF is not TKG image type and is not v1alpha1 compatible", func() {
			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyWindowsOSType,
						},
					},
				},
			}

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does
			// not contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					vmopv1alpha1.VirtualMachineImageOSTypeNotSupportedReason,
					vmopv1alpha1.ConditionSeverityError,
					fmt.Sprintf("VirtualMachineImage image type %s is not supported by VMService", dummyWindowsOSType)),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("with vmtx type", func() {
			item.Type = "vmtx"
			image := vsphere.LibItemToVirtualMachineImage(&item, nil, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(HaveKey(vsphere.VMImageCLVersionAnnotation))

			// ImageSupported in Status is unset as the image type is not OVF type
			Expect(image.Status.ImageSupported).Should(BeNil())
			Expect(image.Status.Conditions).Should(BeEmpty())
		})

		It("ImageSupported should be set to true when it is a TKG image and valid OS Type is set and OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig", func() {
			tkgKey := "vmware-system.guest.kubernetes.distribution.image.version"

			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyValidOsType,
						}},
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

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())

			Expect(image.Status.ImageSupported).Should(Equal(supportedTrue))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ImageSupported should be set to false when OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig and is not a TKG image and has a valid OS type set", func() {
			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyValidOsType,
						},
					},
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

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))

			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ImageSupported should be set to true when OVF Envelope has vsphere.VMOperatorV1Alpha1ExtraConfigKey set to vsphere.VMOperatorV1Alpha1ConfigReady in extraConfig and has a valid OS type set", func() {
			ovfEnvelope = &ovf.Envelope{
				VirtualSystem: &ovf.VirtualSystem{
					OperatingSystem: []ovf.OperatingSystemSection{
						{
							OSType: &dummyValidOsType,
						},
					},
					VirtualHardware: []ovf.VirtualHardwareSection{
						{
							ExtraConfig: []ovf.Config{
								{
									Key:   vsphere.VMOperatorV1Alpha1ExtraConfigKey,
									Value: vsphere.VMOperatorV1Alpha1ConfigReady,
								},
							},
						},
					},
				},
			}

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, supportedGuestOsIdsToFamily)
			Expect(image).ToNot(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(supportedTrue))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})
	})

	It("GetValidGuestOSDescriptorIDs from cluster", func() {
		res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
			finder := find.NewFinder(c)
			cluster, err := finder.DefaultClusterComputeResource(ctx)
			Expect(err).ToNot(HaveOccurred())
			ids, err := vsphere.GetValidGuestOSDescriptorIDs(ctx, cluster, c)
			Expect(err).To(BeNil())
			Expect(ids).ToNot(BeNil())
			return nil
		})
		Expect(res).To(BeNil())
	})

	Context("Expose ovfEnv properties", func() {
		var (
			ovfUserConfigurableTrue  = true
			ovfUserConfigurableFalse = false
			ovfStringType            = "string"
			userConfigurableKey      = "dummy-key-configurable"
			notUserConfigurableKey   = "dummy-key-not-configurable"
			defaultValue             = "dummy-value"
		)

		It("returns a VirtualMachineImage object from a content library with ovfEnv", func() {
			ts := time.Now()
			item := library.Item{
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
							FullVersion: "fullversion",
							Version:     "version",
							Property: []ovf.Property{
								{
									Key:     versionKey,
									Type:    ovfStringType,
									Default: &versionVal,
								},
								{
									Key:              userConfigurableKey,
									Type:             ovfStringType,
									Default:          &defaultValue,
									UserConfigurable: &ovfUserConfigurableTrue,
								},
								{
									Key:              notUserConfigurableKey,
									Type:             ovfStringType,
									Default:          &defaultValue,
									UserConfigurable: &ovfUserConfigurableFalse,
								},
							},
						},
					},
				},
			}

			image := vsphere.LibItemToVirtualMachineImage(&item, ovfEnvelope, nil)
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			Expect(image.Spec.OVFEnv).Should(HaveLen(1))
			Expect(image.Spec.OVFEnv).Should(HaveKey(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Key).Should(Equal(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Type).Should(Equal(ovfStringType))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Default).Should(Equal(&defaultValue))
		})
	})
})
