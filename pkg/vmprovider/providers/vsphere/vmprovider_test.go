// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package vsphere_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/mocks"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var _ = Describe("virtualmachine images", func() {

	var mockVmProviderInterface *mocks.MockOvfPropertyRetriever
	var mockController *gomock.Controller
	var (
		versionKey    = "vmware-system-version"
		versionVal    = "1.15"
		imgKey        = "img-foo-1"
		imgVal        = "bar-1"
		imgValUpdated = imgVal + "-updated"
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockVmProviderInterface = mocks.NewMockOvfPropertyRetriever(mockController)
	})

	AfterEach(func() {
		mockController.Finish()

	})

	Context("when adding VM annotation", func() {
		It("should fetch annotations from OVF properties", func() {

			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				expectedAnnotations := map[string]string{
					versionKey:                        versionVal,
					imgKey:                            imgVal,
					vsphere.VmOperatorVMImagePropsKey: "false",
				}

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())
				vmAnnotations := map[string]string{}
				vmAnnotations[versionKey] = versionVal

				imgAnnotations := map[string]string{}
				imgAnnotations[imgKey] = imgVal
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(imgAnnotations, nil).
					Times(1)

				// Test we fetch them the first time
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("not fetching them again if already there")
				// Test we don't fetch them the second time
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("not fetching them again even if there are changed OVF properties on image")
				// Test that we don't fetch if we have updated image annotations
				imgAnnotations[imgKey] = imgValUpdated
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(gomock.Any(), gomock.Any()).
					Return(imgAnnotations, nil).
					Times(0)

				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))

				By("re-fetch image properties when we reset the cache key")
				vmAnnotations[vsphere.VmOperatorVMImagePropsKey] = "true"
				expectedAnnotations[imgKey] = imgValUpdated
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(imgAnnotations, nil).
					Times(1)
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(BeNil())
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))
			})
		})
		It("handles errors from VC", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				expectedAnnotations := map[string]string{
					versionKey: versionVal,
				}
				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())
				vmAnnotations := map[string]string{}
				vmAnnotations[versionKey] = versionVal

				ovfFetchErr := errors.New("VC error foo bar is closed")
				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromVM(context.Background(), resVm).
					Return(map[string]string{}, ovfFetchErr).
					Times(1)
				err = vsphere.AddVmImageAnnotations(vmAnnotations, ctx, mockVmProviderInterface, resVm)
				Expect(err).To(Equal(ovfFetchErr))
				Expect(vmAnnotations).Should(Not(BeEmpty()))
				Expect(vmAnnotations).Should(Equal(expectedAnnotations))
			})
		})
	})

	Context("when annotate flag is set to false", func() {

		It("returns a virtualmachineimage object from an inventory VM without annotations", func() {
			simulator.Test(func(ctx context.Context, c *vim25.Client) {
				svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
				obj := object.NewVirtualMachine(c, svm.Reference())

				resVm, err := res.NewVMFromObject(obj)
				Expect(err).To(BeNil())

				image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), resVm, vsphere.DoNotAnnotateVmImage, nil)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal(obj.Name()))
				Expect(image.Annotations).To(BeEmpty())
			})
		})

		It("returns a virtualmachineimage object from a content library without annotations but with ovf info", func() {
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
							Property: []ovf.Property{{
								Key:     versionKey,
								Default: &versionVal,
							},
							},
						}},
				},
			}

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, nil)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			Expect(image.Spec.ProductInfo.Vendor).Should(Equal("vendor"))
			Expect(image.Spec.ProductInfo.Product).Should(Equal("product"))
			Expect(image.Spec.ProductInfo.FullVersion).Should(Equal("fullversion"))
			Expect(image.Spec.ProductInfo.Version).Should(Equal("version"))
		})
	})

	Context("when annotate flag is set to true", func() {

		Context("when ovf info is present", func() {
			It("returns a virtualmachineimage object from a content library with annotations and ovf info", func() {
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
								Property: []ovf.Property{{
									Key:     versionKey,
									Default: &versionVal,
								},
								},
							}},
					},
				}

				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(ovfEnvelope, nil).
					AnyTimes()

				image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface, nil)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal("fakeItem"))
				Expect(image.Annotations).NotTo(BeEmpty())
				Expect(len(image.Annotations)).To(BeEquivalentTo(1))
				Expect(image.Annotations).Should(HaveKey("vmware-system-version"))
				Expect(image.Annotations["vmware-system-version"]).Should(Equal("1.15"))
				Expect(image.CreationTimestamp).To(BeEquivalentTo(v1.NewTime(ts)))

				Expect(image.Spec.ProductInfo.Vendor).Should(Equal("vendor"))
				Expect(image.Spec.ProductInfo.Product).Should(Equal("product"))
				Expect(image.Spec.ProductInfo.FullVersion).Should(Equal("fullversion"))
				Expect(image.Spec.ProductInfo.Version).Should(Equal("version"))
			})

			It("returns a virtualmachineimage object from an inventory VM with annotations", func() {
				simulator.Test(func(ctx context.Context, c *vim25.Client) {
					svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
					obj := object.NewVirtualMachine(c, svm.Reference())

					resVm, err := res.NewVMFromObject(obj)
					Expect(err).To(BeNil())

					annotations := map[string]string{}
					annotations[versionKey] = versionVal
					mockVmProviderInterface.EXPECT().
						GetOvfInfoFromVM(gomock.Any(), gomock.Any()).
						Return(annotations, nil).
						AnyTimes()

					image, err := vsphere.ResVmToVirtualMachineImage(context.TODO(), resVm, vsphere.AnnotateVmImage, mockVmProviderInterface)
					Expect(err).To(BeNil())
					Expect(image).ToNot(BeNil())
					Expect(image.Name).Should(Equal(obj.Name()))
					Expect(image.Annotations).ToNot(BeEmpty())
					Expect(len(image.Annotations)).To(BeEquivalentTo(1))
					Expect(image.Annotations).To(HaveKey(versionKey))
					Expect(image.Annotations[versionKey]).To(Equal(versionVal))
				})
			})
		})

		Context("when ovf info is absent", func() {
			It("returns a virtualmachineimage object from a content library with annotations and no ovf info", func() {
				ts := time.Now()
				item := library.Item{
					Name:         "fakeItem",
					Type:         "ovf",
					LibraryID:    "fakeID",
					CreationTime: &ts,
				}

				ovfEnvelope := &ovf.Envelope{}

				mockVmProviderInterface.EXPECT().
					GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(ovfEnvelope, nil).
					AnyTimes()

				image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface, nil)
				Expect(err).To(BeNil())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).Should(Equal("fakeItem"))
				Expect(image.Annotations).To(BeEmpty())
				Expect(image.CreationTimestamp).To(BeEquivalentTo(v1.NewTime(ts)))

				Expect(image.Spec.ProductInfo).ShouldNot(BeNil())
				Expect(image.Spec.ProductInfo.Version).Should(BeEmpty())
			})
		})
	})

	Context("when annotate flag is set to true and error occurs in fetching ovf properties", func() {
		It("returns an err", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, errors.New("error occurred when downloading library content")).
				AnyTimes()
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface, nil)
			Expect(err).NotTo(BeNil())
			Expect(image).To(BeNil())
			Expect(err).Should(MatchError("error occurred when downloading library content"))
		})
	})

	Context("LibItemToVirtualMachineImage, ImageCompatibility and SupportedGuestOS", func() {
		var (
			item                        library.Item
			ovfEnvelope                 *ovf.Envelope
			dummyValidOsType            = "dummy_valid_os_type"
			dummyEmptyOsType            = ""
			dummyWindowsOSType          = "dummy_win_os"
			dummylinuxFamily            = string(types.VirtualMachineGuestOsFamilyLinuxGuest)
			dummyWindowsFamily          = string(types.VirtualMachineGuestOsFamilyWindowsGuest)
			supportedGuestOsIdsToFamily map[string]string
			supportedFalse              = new(bool)
			supportedTrue               = new(bool)
			trueVar                     = true
			falseVar                    = false
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
						}},
				},
			}

			supportedGuestOsIdsToFamily = make(map[string]string)
			// supported guestOSIds fetched from the cluster
			supportedGuestOsIdsToFamily[dummyValidOsType] = dummylinuxFamily
			supportedGuestOsIdsToFamily[dummyWindowsOSType] = dummyWindowsFamily

			supportedTrue = &trueVar
			supportedFalse = &falseVar
		})

		It("ovfEnvelope has a valid GuestOSType and OVF is not a TKG Image and is not v1alpha1 compatible", func() {
			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does not contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
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

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does not contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))
			msg := fmt.Sprintf("VirtualMachineImage image type %s is not supported by VM Svc", "")
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					vmopv1alpha1.VirtualMachineImageOSTypeNotSupportedReason,
					vmopv1alpha1.ConditionSeverityError,
					msg),
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

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			// ImageSupported in Status is to false as OS type is windows, OVF is not a TKG image and does not contain VMOperatorV1Alpha1ExtraConfigKey in extraConfig
			Expect(image.Status.ImageSupported).Should(Equal(supportedFalse))
			msg := fmt.Sprintf("VirtualMachineImage image type %s is not supported by VM Svc", dummyWindowsOSType)
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					vmopv1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
					vmopv1alpha1.ConditionSeverityError,
					notCompatibleMsg),
				*conditions.FalseCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition,
					vmopv1alpha1.VirtualMachineImageOSTypeNotSupportedReason,
					vmopv1alpha1.ConditionSeverityError,
					msg),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("with vmtx type ImageSupported flag is not set", func() {
			item.Type = "vmtx"
			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, nil, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))
			Expect(image.Annotations).To(BeEmpty())

			// ImageSupported in Status is unset as the image type is not OVF type
			Expect(image.Status.ImageSupported).Should(BeNil())
			Expect(image.Status.Conditions).Should(BeEmpty())
		})

		It("ImageSupported should be set to true when it is a TKG image and valid OS Type is set and OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig", func() {
			tkgKey := "vmware-system.guest.kubernetes.distribution.image.version"
			tkgValue := "somerandomvalue"
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
									Default: &tkgValue,
								},
							},
						},
					},
				},
			}

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(supportedTrue))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(
					vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
				*conditions.TrueCondition(vmopv1alpha1.VirtualMachineImageOSTypeSupportedCondition),
			}
			Expect(image.Status.Conditions).Should(conditions.MatchConditions(expectedCondition))
		})

		It("ImageSupported should be set to false when OVF Envelope does not have vsphere.VMOperatorV1Alpha1ExtraConfigKey in extraConfig and is not a TKG image and has a valid OS type set", func() {
			someRandomValue := "someRandom"
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
									Key:     "somekey",
									Default: &someRandomValue,
								},
							},
						},
					},
				},
			}

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
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
			//magicKeyValue := "v1alpha1"
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

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.DoNotAnnotateVmImage, mockVmProviderInterface, supportedGuestOsIdsToFamily)
			Expect(err).To(BeNil())
			Expect(image.Status.ImageSupported).Should(Equal(supportedTrue))
			expectedCondition := vmopv1alpha1.Conditions{
				*conditions.TrueCondition(
					vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition),
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

		It("returns a virtualmachineimage object from a content library with ovfEnv", func() {
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

			mockVmProviderInterface.EXPECT().
				GetOvfInfoFromLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(ovfEnvelope, nil).
				AnyTimes()

			image, err := vsphere.LibItemToVirtualMachineImage(context.TODO(), nil, &item, vsphere.AnnotateVmImage, mockVmProviderInterface, nil)
			Expect(err).To(BeNil())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal("fakeItem"))

			Expect(len(image.Spec.OVFEnv)).Should(Equal(1))
			Expect(image.Spec.OVFEnv).Should(HaveKey(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Key).Should(Equal(userConfigurableKey))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Type).Should(Equal(ovfStringType))
			Expect(image.Spec.OVFEnv[userConfigurableKey].Default).Should(Equal(&defaultValue))
		})
	})
})
