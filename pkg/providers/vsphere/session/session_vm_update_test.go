// Copyright (c) 2021-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	pkgclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

var _ = Describe("Update ConfigSpec", func() {

	var (
		ctx        context.Context
		config     *vimtypes.VirtualMachineConfigInfo
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		config = &vimtypes.VirtualMachineConfigInfo{}
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	})

	// Just a few examples for testing these things here. Need to think more about whether this
	// is a good way or not. Probably better to do this via UpdateVirtualMachine when we have
	// better integration tests.

	Context("Basic Hardware", func() {
		var vmClassSpec *vmopv1.VirtualMachineClassSpec

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}
		})

		JustBeforeEach(func() {
			session.UpdateHardwareConfigSpec(config, configSpec, vmClassSpec)
		})

		Context("Updates Hardware", func() {
			BeforeEach(func() {
				vmClassSpec.Hardware.Cpus = 42
				vmClassSpec.Hardware.Memory = resource.MustParse("2000Mi")
			})

			It("config spec is not empty", func() {
				Expect(configSpec.NumCPUs).To(BeNumerically("==", 42))
				Expect(configSpec.MemoryMB).To(BeNumerically("==", 2000))
			})
		})

		Context("config already matches", func() {
			BeforeEach(func() {
				config.Hardware.NumCPU = 42
				vmClassSpec.Hardware.Cpus = int64(config.Hardware.NumCPU)
				config.Hardware.MemoryMB = 1500
				vmClassSpec.Hardware.Memory = resource.MustParse(fmt.Sprintf("%dMi", config.Hardware.MemoryMB))
			})

			It("config spec show no changes", func() {
				Expect(configSpec.NumCPUs).To(BeZero())
				Expect(configSpec.MemoryMB).To(BeZero())
			})
		})
	})

	Context("ExtraConfig", func() {
		var vmClassSpec *vmopv1.VirtualMachineClassSpec
		var classConfigSpec *vimtypes.VirtualMachineConfigSpec
		var vm *vmopv1.VirtualMachine
		var globalExtraConfig map[string]string
		var ecMap map[string]string

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: make(map[string]string),
				},
			}
			globalExtraConfig = make(map[string]string)
			classConfigSpec = nil
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecExtraConfig(
				ctx,
				config,
				configSpec,
				classConfigSpec,
				vmClassSpec,
				vm,
				globalExtraConfig)

			ecMap = pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
		})

		Context("Empty input", func() {
			Specify("no changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		When("classConfigSpec, vmClassSpec, vm, and globalExtraConfig params are nil", func() {
			BeforeEach(func() {
				classConfigSpec = nil
				vmClassSpec = nil
				vm = nil
				globalExtraConfig = nil
			})
			Specify("no changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		Context("Updates configSpec.ExtraConfig", func() {
			BeforeEach(func() {
				config.ExtraConfig = append(config.ExtraConfig, &vimtypes.OptionValue{
					Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigReady})
				globalExtraConfig["guestinfo.test"] = "test"
				globalExtraConfig["global"] = "test"
			})

			When("VM uses LinuxPrep with vAppConfig bootstrap", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						LinuxPrep:  &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
					}
				})

				It("Expected configSpec.ExtraConfig", func() {
					By("VM Image compatible", func() {
						Expect(ecMap).To(HaveKeyWithValue(constants.VMOperatorV1Alpha1ExtraConfigKey, constants.VMOperatorV1Alpha1ConfigEnabled))
					})

					By("Global map", func() {
						Expect(ecMap).To(HaveKeyWithValue("guestinfo.test", "test"))
						Expect(ecMap).To(HaveKeyWithValue("global", "test"))
					})
				})
			})

			Context("When VM uses other bootstrap", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
					}
				})

				It("defer cloud-init extra config is still ready", func() {
					By("Should not include an update to the ExtraConfig field", func() {
						Expect(ecMap).ToNot(HaveKey(constants.VMOperatorV1Alpha1ExtraConfigKey))
					})

					By("Global map", func() {
						Expect(ecMap).To(HaveKeyWithValue("guestinfo.test", "test"))
						Expect(ecMap).To(HaveKeyWithValue("global", "test"))
					})
				})
			})
		})

		Context("ExtraConfig has a non-empty MM flag", func() {
			BeforeEach(func() {
				config.ExtraConfig = append(
					config.ExtraConfig,
					&vimtypes.OptionValue{
						Key:   constants.MMPowerOffVMExtraConfigKey,
						Value: "true",
					})
			})
			It("Should be set to an empty value", func() {
				Expect(ecMap).To(HaveKeyWithValue(constants.MMPowerOffVMExtraConfigKey, ""))
			})
			When("classConfigSpec, vmClassSpec, and vm params are nil", func() {
				BeforeEach(func() {
					classConfigSpec = nil
					vmClassSpec = nil
					vm = nil
				})
				It("Should be set to an empty value", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.MMPowerOffVMExtraConfigKey, ""))
				})
				When("globalExtraConfig is nil", func() {
					BeforeEach(func() {
						globalExtraConfig = nil
					})
					It("Should be set to an empty value", func() {
						Expect(ecMap).To(HaveKeyWithValue(constants.MMPowerOffVMExtraConfigKey, ""))
					})
				})
			})
		})

		Context("ExtraConfig value already exists", func() {
			BeforeEach(func() {
				config.ExtraConfig = append(config.ExtraConfig, &vimtypes.OptionValue{Key: "foo", Value: "bar"})
				globalExtraConfig["foo"] = "bar"
			})

			It("No changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		Context("ThunderPciDevices related test", func() {

			Context("when virtual devices are not present", func() {
				It("No Changes", func() {
					Expect(ecMap).To(BeEmpty())
				})
			})

			Context("when vGPU and DDPIO devices are present but classConfigSpec, vmClassSpec, and vm params are nil", func() {
				BeforeEach(func() {
					config.Hardware.Device = []vimtypes.BaseVirtualDevice{
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "SampleProfile",
								},
							},
						},
						&vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{},
							},
						},
					}
					classConfigSpec = nil
					vmClassSpec = nil
					vm = nil
				})

				Specify("No Changes", func() {
					Expect(ecMap).To(BeEmpty())
				})
			})

			Context("when vGPU device is available", func() {
				BeforeEach(func() {
					vmClassSpec.Hardware.Devices = vmopv1.VirtualDevices{VGPUDevices: []vmopv1.VGPUDevice{
						{
							ProfileName: "test-vgpu-profile",
						},
					}}
				})

				It("PCI passthru MMIO extraConfig should be added", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOExtraConfigKey, constants.ExtraConfigTrue))
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOSizeExtraConfigKey, constants.PCIPassthruMMIOSizeDefault))
				})

				Context("when PCI passthru MMIO override annotation is set", func() {
					BeforeEach(func() {
						vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation] = "12345"
					})

					It("PCI passthru MMIO extraConfig should be set to override annotation value", func() {
						Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOExtraConfigKey, constants.ExtraConfigTrue))
						Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOSizeExtraConfigKey, "12345"))
					})
				})
			})

			Context("when DDPIO device is available", func() {
				BeforeEach(func() {
					vmClassSpec.Hardware.Devices = vmopv1.VirtualDevices{DynamicDirectPathIODevices: []vmopv1.DynamicDirectPathIODevice{
						{
							VendorID:    123,
							DeviceID:    24,
							CustomLabel: "",
						},
					}}
				})

				It("PCI passthru MMIO extraConfig should be added", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOExtraConfigKey, constants.ExtraConfigTrue))
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOSizeExtraConfigKey, constants.PCIPassthruMMIOSizeDefault))
				})

				Context("when PCI passthru MMIO override annotation is set", func() {
					BeforeEach(func() {
						vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation] = "12345"
					})

					It("PCI passthru MMIO extraConfig should be set to override annotation value", func() {
						Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOExtraConfigKey, constants.ExtraConfigTrue))
						Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOSizeExtraConfigKey, "12345"))
					})
				})
			})
		})

		const (
			dummyKey = "dummy-key"
			dummyVal = "dummy-val"
		)

		When("classConfigSpec extra config is not nil", func() {
			BeforeEach(func() {
				classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   dummyKey + "-1",
							Value: dummyVal + "-1",
						},
						&vimtypes.OptionValue{
							Key:   dummyKey + "-2",
							Value: dummyVal + "-2",
						},
					},
				}
				config.ExtraConfig = append(config.ExtraConfig, &vimtypes.OptionValue{Key: "hello", Value: "world"})
			})
			It("vm extra config overlaps with global extra config", func() {
				globalExtraConfig["hello"] = "world"
				Expect(ecMap).To(HaveKeyWithValue(dummyKey+"-1", dummyVal+"-1"))
				Expect(ecMap).To(HaveKeyWithValue(dummyKey+"-2", dummyVal+"-2"))
				Expect(ecMap).ToNot(HaveKeyWithValue("hello", "world"))
			})

			It("global extra config overlaps with class config spec - class config spec takes precedence", func() {
				globalExtraConfig[dummyKey+"-1"] = dummyVal + "-3"
				Expect(ecMap).To(HaveKeyWithValue(dummyKey+"-1", dummyVal+"-1"))
				Expect(ecMap).To(HaveKeyWithValue(dummyKey+"-2", dummyVal+"-2"))
			})

			Context("class config spec has vGPU and DDPIO devices", func() {
				BeforeEach(func() {
					classConfigSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "SampleProfile2",
									},
								},
							},
						},
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
										AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
											{
												VendorId: 52,
												DeviceId: 53,
											},
										},
										CustomLabel: "SampleLabel2",
									},
								},
							},
						},
					}

				})

				It("extraConfig Map has MMIO keys added", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOExtraConfigKey, constants.ExtraConfigTrue))
					Expect(ecMap).To(HaveKeyWithValue(constants.PCIPassthruMMIOSizeExtraConfigKey, constants.PCIPassthruMMIOSizeDefault))
				})
			})
		})
	})

	Context("ChangeBlockTracking", func() {
		var vmSpec vmopv1.VirtualMachineSpec
		var classConfigSpec *vimtypes.VirtualMachineConfigSpec

		BeforeEach(func() {
			config.ChangeTrackingEnabled = nil
			classConfigSpec = nil
		})

		AfterEach(func() {
			configSpec.ChangeTrackingEnabled = nil
		})

		It("cbt and status cbt unset", func() {
			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to true", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(false),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to true with Advanced set to nil", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = nil

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = ptr.To(false)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(true),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(true),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		Context("configSpec.cbt is false and spec.cbt is true", func() {
			BeforeEach(func() {
				config.ChangeTrackingEnabled = ptr.To(false)
				vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ChangeBlockTracking: ptr.To(true),
				}
			})

			It("classConfigSpec not nil and same as configInfo", func() {
				classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
					ChangeTrackingEnabled: ptr.To(false),
				}

				session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
				Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
				Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
			})

			It("classConfigSpec not nil, different from configInfo, overrides vm spec cbt", func() {
				classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
					ChangeTrackingEnabled: ptr.To(true),
				}

				session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
				Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
				Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
			})
		})
	})

	Context("Firmware", func() {
		var vm *vmopv1.VirtualMachine

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: make(map[string]string),
				},
			}
			config.Firmware = "bios"
		})

		It("No firmware annotation", func() {
			session.UpdateConfigSpecFirmware(config, configSpec, vm)
			Expect(configSpec.Firmware).To(BeEmpty())
		})

		It("Set firmware annotation equal to current vm firmware", func() {
			vm.Annotations[constants.FirmwareOverrideAnnotation] = config.Firmware
			session.UpdateConfigSpecFirmware(config, configSpec, vm)
			Expect(configSpec.Firmware).To(BeEmpty())
		})

		It("Set firmware annotation differing to current vm firmware", func() {
			vm.Annotations[constants.FirmwareOverrideAnnotation] = "efi"
			session.UpdateConfigSpecFirmware(config, configSpec, vm)
			Expect(configSpec.Firmware).To(Equal("efi"))
		})

		It("Set firmware annotation to an invalid value", func() {
			vm.Annotations[constants.FirmwareOverrideAnnotation] = "invalidfirmware"
			session.UpdateConfigSpecFirmware(config, configSpec, vm)
			Expect(configSpec.Firmware).To(BeEmpty())
		})
	})

	Context("Ethernet Card Changes", func() {
		var expectedList object.VirtualDeviceList
		var currentList object.VirtualDeviceList
		var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
		var dvpg1 *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var dvpg2 *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var err error

		BeforeEach(func() {
			dvpg1 = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					PortgroupKey: "key1",
					SwitchUuid:   "uuid1",
				},
			}

			dvpg2 = &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimtypes.DistributedVirtualSwitchPortConnection{
					PortgroupKey: "key2",
					SwitchUuid:   "uuid2",
				},
			}
		})

		JustBeforeEach(func() {
			deviceChanges, err = session.UpdateEthCardDeviceChanges(ctx, expectedList, currentList)
		})

		AfterEach(func() {
			currentList = nil
			expectedList = nil
		})

		Context("No devices", func() {
			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
			})
		})

		Context("No device change when nothing changes", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimtypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				currentList = append(currentList, card2)
			})

			It("returns no device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(0))
			})
		})

		Context("Add device", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				expectedList = append(expectedList, card1)
			})

			It("returns add device change", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(1))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when backing change", func() {
			var card1 vimtypes.BaseVirtualDevice
			var card2 vimtypes.BaseVirtualDevice

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg2)
				Expect(err).ToNot(HaveOccurred())
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when MAC address is different", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimtypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				card1.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
				card1.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = "mac1"
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				card2.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
				card2.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = "mac2"
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when card type is different", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimtypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet2", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when ExternalID is different", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimtypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				card1.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId = "ext1"
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				card2.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId = "ext2"
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Keeps existing device with same backing", func() {
			var card1 vimtypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimtypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				currentList = append(currentList, card2)
			})

			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
			})
		})
	})

	Context("Create vSphere PCI device", func() {
		var vgpuDevices = []vmopv1.VGPUDevice{
			{
				ProfileName: "SampleProfile",
			},
		}
		var ddpioDevices = []vmopv1.DynamicDirectPathIODevice{
			{
				VendorID:    42,
				DeviceID:    43,
				CustomLabel: "SampleLabel",
			},
		}
		var pciDevices vmopv1.VirtualDevices
		Context("For VM Class Spec vGPU device", func() {
			BeforeEach(func() {
				pciDevices = vmopv1.VirtualDevices{
					VGPUDevices: vgpuDevices,
				}
			})
			It("should create vSphere device with VmiopBackingInfo", func() {
				vSphereDevices := virtualmachine.CreatePCIDevicesFromVMClass(pciDevices)
				Expect(vSphereDevices).To(HaveLen(1))
				virtualDevice := vSphereDevices[0].GetVirtualDevice()
				backing := virtualDevice.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
				Expect(backing.Vgpu).To(Equal(pciDevices.VGPUDevices[0].ProfileName))
			})
		})
		Context("For VM Class Spec Dynamic DirectPath I/O device", func() {
			BeforeEach(func() {
				pciDevices = vmopv1.VirtualDevices{
					DynamicDirectPathIODevices: ddpioDevices,
				}
			})
			It("should create vSphere device with DynamicBackingInfo", func() {
				vSphereDevices := virtualmachine.CreatePCIDevicesFromVMClass(pciDevices)
				Expect(vSphereDevices).To(HaveLen(1))
				virtualDevice := vSphereDevices[0].GetVirtualDevice()
				backing := virtualDevice.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
				Expect(backing.AllowedDevice[0].DeviceId).To(Equal(int32(pciDevices.DynamicDirectPathIODevices[0].DeviceID)))
				Expect(backing.AllowedDevice[0].VendorId).To(Equal(int32(pciDevices.DynamicDirectPathIODevices[0].VendorID)))
				Expect(backing.CustomLabel).To(Equal(pciDevices.DynamicDirectPathIODevices[0].CustomLabel))
			})
		})

		When("PCI devices from ConfigSpec are specified", func() {

			var devIn []*vimtypes.VirtualPCIPassthrough

			Context("For ConfigSpec VGPU device", func() {
				BeforeEach(func() {
					devIn = []*vimtypes.VirtualPCIPassthrough{
						{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "configspec-profile",
								},
							},
						},
					}
				})
				It("should create vSphere device with VmiopBackingInfo", func() {
					devList := virtualmachine.CreatePCIDevicesFromConfigSpec(devIn)
					Expect(devList).To(HaveLen(1))

					Expect(devList[0]).ToNot(BeNil())
					Expect(devList[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthrough{}))
					Expect(devList[0].(*vimtypes.VirtualPCIPassthrough).Backing).ToNot(BeNil())
					Expect(devList[0].(*vimtypes.VirtualPCIPassthrough).Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughVmiopBackingInfo{}))
					Expect(devList[0].(*vimtypes.VirtualPCIPassthrough).Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo).Vgpu).To(Equal("configspec-profile"))
				})
			})

			Context("For ConfigSpec DirectPath I/O device", func() {
				BeforeEach(func() {
					devIn = []*vimtypes.VirtualPCIPassthrough{
						{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
									CustomLabel: "configspec-ddpio-label",
									AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
										{
											VendorId: 456,
											DeviceId: 457,
										},
									},
								},
							},
						},
					}
				})
				It("should create vSphere device with DynamicBackingInfo", func() {
					devList := virtualmachine.CreatePCIDevicesFromConfigSpec(devIn)
					Expect(devList).To(HaveLen(1))

					Expect(devList[0]).ToNot(BeNil())
					Expect(devList[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthrough{}))

					Expect(devList[0].(*vimtypes.VirtualPCIPassthrough).Backing).ToNot(BeNil())
					backing := devList[0].(*vimtypes.VirtualPCIPassthrough).Backing
					Expect(backing).To(BeAssignableToTypeOf(&vimtypes.VirtualPCIPassthroughDynamicBackingInfo{}))

					Expect(backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo).CustomLabel).To(Equal("configspec-ddpio-label"))
					Expect(backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo).AllowedDevice[0].VendorId).To(BeEquivalentTo(456))
					Expect(backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo).AllowedDevice[0].DeviceId).To(BeEquivalentTo(457))
				})
			})
		})
	})

	Context("PCI Device Changes", func() {
		var (
			currentList, expectedList object.VirtualDeviceList
			deviceChanges             []vimtypes.BaseVirtualDeviceConfigSpec
			err                       error

			// Variables related to vGPU devices.
			backingInfo1, backingInfo2 *vimtypes.VirtualPCIPassthroughVmiopBackingInfo
			deviceKey1, deviceKey2     int32
			vGPUDevice1, vGPUDevice2   vimtypes.BaseVirtualDevice

			// Variables related to dynamicDirectPathIO devices.
			allowedDev1, allowedDev2                         vimtypes.VirtualPCIPassthroughAllowedDevice
			backingInfo3, backingInfo4                       *vimtypes.VirtualPCIPassthroughDynamicBackingInfo
			deviceKey3, deviceKey4                           int32
			dynamicDirectPathIODev1, dynamicDirectPathIODev2 vimtypes.BaseVirtualDevice
		)

		BeforeEach(func() {
			backingInfo1 = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop1"}
			backingInfo2 = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop2"}
			deviceKey1 = int32(-200)
			deviceKey2 = int32(-201)
			vGPUDevice1 = virtualmachine.CreatePCIPassThroughDevice(deviceKey1, backingInfo1)
			vGPUDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey2, backingInfo2)

			allowedDev1 = vimtypes.VirtualPCIPassthroughAllowedDevice{
				VendorId: 1000,
				DeviceId: 100,
			}
			allowedDev2 = vimtypes.VirtualPCIPassthroughAllowedDevice{
				VendorId: 2000,
				DeviceId: 200,
			}
			backingInfo3 = &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
				AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev1},
				CustomLabel:   "sampleLabel3",
			}
			backingInfo4 = &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
				AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev2},
				CustomLabel:   "sampleLabel4",
			}
			deviceKey3 = int32(-202)
			deviceKey4 = int32(-203)
			dynamicDirectPathIODev1 = virtualmachine.CreatePCIPassThroughDevice(deviceKey3, backingInfo3)
			dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfo4)
		})

		JustBeforeEach(func() {
			deviceChanges, err = session.UpdatePCIDeviceChanges(expectedList, currentList)
		})

		AfterEach(func() {
			currentList = nil
			expectedList = nil
		})

		Context("No devices", func() {
			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
			})
		})

		Context("Adding vGPU and dynamicDirectPathIO devices with different backing info", func() {
			BeforeEach(func() {
				expectedList = append(expectedList, vGPUDevice1)
				expectedList = append(expectedList, vGPUDevice2)
				expectedList = append(expectedList, dynamicDirectPathIODev1)
				expectedList = append(expectedList, dynamicDirectPathIODev2)
			})

			It("Should return add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(len(expectedList)))

				for idx, dev := range deviceChanges {
					configSpec := dev.GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
				}
			})
		})

		Context("Adding vGPU and dynamicDirectPathIO devices with same backing info", func() {
			BeforeEach(func() {
				expectedList = append(expectedList, vGPUDevice1)
				// Creating a vGPUDevice with same backingInfo1 but different deviceKey.
				vGPUDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey2, backingInfo1)
				expectedList = append(expectedList, vGPUDevice2)
				expectedList = append(expectedList, dynamicDirectPathIODev1)
				// Creating a dynamicDirectPathIO device with same backingInfo3 but different deviceKey.
				dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfo3)
				expectedList = append(expectedList, dynamicDirectPathIODev2)
			})

			It("Should return add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(len(expectedList)))

				for idx, dev := range deviceChanges {
					configSpec := dev.GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
				}
			})
		})

		Context("When the expected and current lists have DDPIO devices with different custom labels", func() {
			BeforeEach(func() {
				expectedList = []vimtypes.BaseVirtualDevice{dynamicDirectPathIODev1}
				// Creating a dynamicDirectPathIO device with same backing info except for the custom label.
				backingInfoDiffCustomLabel := &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
					AllowedDevice: backingInfo3.AllowedDevice,
					CustomLabel:   "DifferentLabel",
				}
				dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfoDiffCustomLabel)
				currentList = []vimtypes.BaseVirtualDevice{dynamicDirectPathIODev2}
			})

			It("should return add and remove device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("When the expected and current list of pciDevices have different Devices", func() {
			BeforeEach(func() {
				currentList = append(currentList, vGPUDevice1)
				expectedList = append(expectedList, vGPUDevice2)
				currentList = append(currentList, dynamicDirectPathIODev1)
				expectedList = append(expectedList, dynamicDirectPathIODev2)
			})

			It("Should return add and remove device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(4))

				for i := 0; i < 2; i++ {
					configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[i].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
				}

				for i := 2; i < 4; i++ {
					configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[i-2].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
				}
			})
		})

		Context("When the expected and current list of pciDevices have same Devices", func() {
			BeforeEach(func() {
				currentList = append(currentList, vGPUDevice1)
				expectedList = append(expectedList, vGPUDevice1)
				currentList = append(currentList, dynamicDirectPathIODev1)
				expectedList = append(expectedList, dynamicDirectPathIODev1)
			})

			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
			})
		})
	})

	Context("UpdateConfigSpecGuestID", func() {
		const fakeGuestID = "fakeGuestID"
		var vmSpecGuestID string

		JustBeforeEach(func() {
			session.UpdateConfigSpecGuestID(config, configSpec, vmSpecGuestID)
		})

		When("VM spec guestID is empty", func() {
			BeforeEach(func() {
				vmSpecGuestID = ""
			})

			It("should not set guestID in configSpec", func() {
				Expect(configSpec.GuestId).To(BeEmpty())
			})
		})

		When("VM spec guestID is not empty and VM ConfigInfo guestID is empty", func() {
			BeforeEach(func() {
				vmSpecGuestID = fakeGuestID
				config.GuestId = ""
			})

			It("should set guestID in configSpec", func() {
				Expect(configSpec.GuestId).To(Equal(vmSpecGuestID))
			})
		})

		When("VM spec guestID is different from the VM ConfigInfo guestID", func() {
			BeforeEach(func() {
				vmSpecGuestID = fakeGuestID
				config.GuestId = "some-other-guestID"
			})

			It("should set guestID in configSpec", func() {
				Expect(configSpec.GuestId).To(Equal(vmSpecGuestID))
			})
		})

		When("VM spec guestID already matches VM ConfigInfo guestID", func() {
			BeforeEach(func() {
				config.GuestId = fakeGuestID
				vmSpecGuestID = fakeGuestID
			})

			It("should not set guestID in configSpec", func() {
				Expect(configSpec.GuestId).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("UpdateVirtualMachine", func() {

	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
		sess       *session.Session
		vm         *vmopv1.VirtualMachine
		vcVM       *object.VirtualMachine
		vmCtx      pkgctx.VirtualMachineContext
		vmProps    = vsphere.VMUpdatePropertiesSelector
	)

	getLastRestartTime := func(moVM mo.VirtualMachine) string {
		for i := range moVM.Config.ExtraConfig {
			ov := moVM.Config.ExtraConfig[i].GetOptionValue()
			if ov.Key == "vmservice.lastRestartTime" {
				return ov.Value.(string)
			}
		}
		return ""
	}

	BeforeEach(func() {
		testConfig.NumFaultDomains = 1
		vm = builder.DummyVirtualMachine()
		vm.Name = "my-vm"
		vm.Namespace = "my-namespace"
		vm.Spec.Network.Interfaces = nil
		vm.Spec.Volumes = nil
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)

		vcClient, err := pkgclient.NewClient(ctx, ctx.VCClientConfig)
		Expect(err).ToNot(HaveOccurred())

		ccr, err := ctx.Finder.ClusterComputeResourceOrDefault(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(ccr).ToNot(BeNil())

		vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(vmList).ToNot(BeNil())
		Expect(vmList).ToNot(BeEmpty())
		vcVM = vmList[0]

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctxop.WithContext(ctx),
			Logger:  suite.GetLogger().WithName(vcVM.Name()),
			VM:      vm,
		}

		pkgcfg.UpdateContext(vmCtx, func(config *pkgcfg.Config) {
			config.NetworkProviderType = pkgcfg.NetworkProviderTypeNamed
		})

		// Update the vSphere VM with the VM namespace/name/managedBy
		t, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
			ManagedBy: &vimtypes.ManagedByInfo{
				ExtensionKey: vmopv1.ManagedByExtensionKey,
				Type:         vmopv1.ManagedByExtensionType,
			},
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   constants.ExtraConfigVMServiceNamespacedName,
					Value: vmCtx.VM.NamespacedName(),
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(t.Wait(ctx)).To(Succeed())

		Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())

		sess = &session.Session{
			Client:       vcClient,
			K8sClient:    ctx.Client,
			Finder:       ctx.Finder,
			ClusterMoRef: ccr.Reference(),
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	assertNoUpdate := func() {
		ExpectWithOffset(1, ctxop.IsUpdate(vmCtx)).To(BeFalse())
	}

	assertUpdate := func() {
		ExpectWithOffset(1, ctxop.IsUpdate(vmCtx)).To(BeTrue())
	}

	When("vcVM is powered on", func() {
		JustBeforeEach(func() {

			// Ensure the VM is powered on.
			switch vmCtx.MoVM.Summary.Runtime.PowerState {
			case vimtypes.VirtualMachinePowerStatePoweredOff,
				vimtypes.VirtualMachinePowerStateSuspended:
				t, err := vcVM.PowerOn(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(t.Wait(ctx)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
			}
			Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
		})

		When("power state is not changed", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})
			It("should not return an error", func() {
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
				assertNoUpdate()
			})
		})

		When("powering off the VM", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})
			When("powerOffMode is hard", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
				})
				It("should power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertUpdate()
				})
			})
			When("powerOffMode is soft", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
				})
				It("should power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertUpdate()
				})
			})
			When("powerOffMode is trySoft", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				})
				It("should power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertUpdate()
				})
			})
		})

		When("restarting the VM", func() {
			var (
				oldLastRestartTime string
			)

			BeforeEach(func() {
				vm.Spec.NextRestartTime = time.Now().UTC().Format(time.RFC3339Nano)
			})
			JustBeforeEach(func() {
				oldLastRestartTime = getLastRestartTime(vmCtx.MoVM)
				vm.Spec.NextRestartTime = time.Now().UTC().Format(time.RFC3339Nano)
			})

			When("restartMode is hard", func() {
				BeforeEach(func() {
					vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeHard
				})
				It("should restart the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					newLastRestartTime := getLastRestartTime(vmCtx.MoVM)
					Expect(newLastRestartTime).ToNot(BeEmpty())
					Expect(newLastRestartTime).ToNot(Equal(oldLastRestartTime))
					assertUpdate()
				})
			})
			When("restartMode is soft", func() {
				BeforeEach(func() {
					vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeSoft
				})
				It("should return an error about lacking tools", func() {
					Expect(testutil.ContainsError(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil), "failed to soft restart vm ServerFaultCode: ToolsUnavailable")).To(BeTrue())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					newLastRestartTime := getLastRestartTime(vmCtx.MoVM)
					Expect(newLastRestartTime).To(Equal(oldLastRestartTime))
					assertUpdate()
				})
			})
			When("restartMode is trySoft", func() {
				BeforeEach(func() {
					vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				})
				It("should restart the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					newLastRestartTime := getLastRestartTime(vmCtx.MoVM)
					Expect(newLastRestartTime).ToNot(BeEmpty())
					Expect(newLastRestartTime).ToNot(Equal(oldLastRestartTime))
					assertUpdate()
				})
			})
		})

		When("suspending the VM", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			})
			When("suspendMode is hard", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
				})
				It("should suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
					assertUpdate()
				})
			})
			When("suspendMode is soft", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeSoft
				})
				It("should suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
					assertUpdate()
				})
			})
			When("suspendMode is trySoft", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				})
				It("should suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
					assertUpdate()
				})
			})
		})
	})

	When("vcVM is powered off", func() {
		JustBeforeEach(func() {

			// Ensure the VM is powered off.
			switch vmCtx.MoVM.Summary.Runtime.PowerState {
			case vimtypes.VirtualMachinePowerStatePoweredOn,
				vimtypes.VirtualMachinePowerStateSuspended:
				t, err := vcVM.PowerOff(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(t.Wait(ctx)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
			}
			Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
		})

		When("power state is not changed", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})
			It("should not return an error", func() {
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
				assertNoUpdate()
			})
		})

		When("powering on the VM", func() {

			var (
				updateArgs *session.VMUpdateArgs
				resizeArgs *session.VMResizeArgs
			)

			BeforeEach(func() {
				resizeArgs = nil
				updateArgs = &session.VMUpdateArgs{
					ResourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{},
				}
			})

			getResizeArgs := func() (*session.VMResizeArgs, error) { //nolint:unparam
				return resizeArgs, nil
			}
			getUpdateArgs := func() (*session.VMUpdateArgs, error) { //nolint:unparam
				return updateArgs, nil
			}

			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			const (
				oldDiskSizeBytes = int64(10 * 1024 * 1024 * 1024)
				newDiskSizeGi    = 20
				newDiskSizeBytes = int64(newDiskSizeGi * 1024 * 1024 * 1024)
			)

			When("the boot disk size is changed for non-ISO VMs", func() {
				JustBeforeEach(func() {
					vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
					disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
					Expect(disks).To(HaveLen(1))
					Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
					diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
					Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))

					q := resource.MustParse(fmt.Sprintf("%dGi", newDiskSizeGi))
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						BootDiskCapacity: &q,
					}
					vm.Spec.Cdrom = nil
				})
				It("should power on the VM with the boot disk resized", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
					disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
					Expect(disks).To(HaveLen(1))
					Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
					diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
					Expect(diskCapacityBytes).To(Equal(newDiskSizeBytes))
					assertUpdate()
				})
			})

			When("the boot disk size is changed for ISO VMs", func() {
				JustBeforeEach(func() {
					vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
					disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
					Expect(disks).To(HaveLen(1))
					Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
					diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
					Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))

					q := resource.MustParse(fmt.Sprintf("%dGi", newDiskSizeGi))
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						BootDiskCapacity: &q,
					}
					vm.Spec.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						{
							Name: "cdrom",
							Image: vmopv1.VirtualMachineImageRef{
								Name: "fake-iso-image",
							},
						},
					}
				})

				It("should power on the VM without the boot disk resized", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
					disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
					Expect(disks).To(HaveLen(1))
					Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
					diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
					Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))
					// VM is powered on.
					assertUpdate()
				})
			})

			When("there are no NICs", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces = nil
				})
				It("should power on the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					assertUpdate()
				})
			})

			When("there is a single NIC", func() {
				BeforeEach(func() {
					vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
							Network: &common.PartialObjectRef{
								Name: "VM Network",
							},
						},
					}
				})
				When("with networking disabled", func() {
					BeforeEach(func() {
						vm.Spec.Network.Disabled = true
					})
					It("should power on the VM", func() {
						Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
						Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
						Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						assertUpdate()
					})
				})
				When("with networking enabled", func() {

					BeforeEach(func() {
						vm.Spec.Network.Disabled = false
					})

					When("class has SR-IOV NIC", func() {
						BeforeEach(func() {
							updateArgs.ConfigSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Device: &vimtypes.VirtualSriovEthernetCard{},
								},
							}
						})
						It("should power on the VM", func() {
							Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
							Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
							Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
							vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
							nics := vmDevs.SelectByType(&vimtypes.VirtualEthernetCard{})
							Expect(nics).To(HaveLen(1))
							Expect(nics[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualSriovEthernetCard{}))
							assertUpdate()
						})
					})

					When("class does not have NIC", func() {
						BeforeEach(func() {
							updateArgs.ConfigSpec.DeviceChange = nil
						})
						It("should power on the VM", func() {
							Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
							Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
							Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
							vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
							nics := vmDevs.SelectByType(&vimtypes.VirtualEthernetCard{})
							Expect(nics).To(HaveLen(1))
							Expect(nics[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualVmxnet3{}))
							assertUpdate()
						})
					})
				})
			})

			When("VM.Spec.GuestID is changed", func() {

				When("the guest ID value is invalid", func() {

					BeforeEach(func() {
						vm.Spec.GuestID = "invalid-guest-id"
					})

					It("should return an error and set the VM's Guest ID condition false", func() {
						errMsg := "reconfigure VM task failed"
						err := sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(errMsg))
						c := conditions.Get(vm, vmopv1.GuestIDReconfiguredCondition)
						Expect(c).ToNot(BeNil())
						expectedCondition := conditions.FalseCondition(
							vmopv1.GuestIDReconfiguredCondition,
							"Invalid",
							"The specified guest ID value is not supported: invalid-guest-id",
						)
						Expect(*c).To(conditions.MatchCondition(*expectedCondition))
						assertUpdate()
					})
				})

				When("the guest ID value is valid", func() {

					BeforeEach(func() {
						vm.Spec.GuestID = "vmwarePhoton64Guest"
					})

					It("should power on the VM with the specified guest ID", func() {
						Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
						Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
						Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(vmCtx.MoVM.Config.GuestId).To(Equal("vmwarePhoton64Guest"))
						assertUpdate()
					})
				})

				When("the guest ID spec is removed", func() {

					BeforeEach(func() {
						vm.Spec.GuestID = ""
					})

					It("should clear the VM guest ID condition if previously set", func() {
						vm.Status.Conditions = []metav1.Condition{
							{
								Type:   vmopv1.GuestIDReconfiguredCondition,
								Status: metav1.ConditionFalse,
							},
						}
						Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
						Expect(conditions.Get(vm, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
						assertUpdate()
					})
				})
			})

			Context("ISO FSS is enabled", func() {

				const (
					vmiName     = "vmi-iso"
					vmiKind     = "VirtualMachineImage"
					vmiFileName = "dummy.iso"
				)

				BeforeEach(func() {
					testConfig.WithContentLibrary = true
					testConfig.WithISOSupport = true
				})

				JustBeforeEach(func() {
					// Add required objects to get CD-ROM backing file name.
					objs := builder.DummyImageAndItemObjectsForCdromBacking(vmiName, vmCtx.VM.Namespace, vmiKind, vmiFileName, ctx.ContentLibraryIsoItemID, true, true, true, "ISO")
					for _, obj := range objs {
						Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
					}
				})

				When("there are CD-ROM device changes", func() {

					BeforeEach(func() {
						vm.Spec.Cdrom = []vmopv1.VirtualMachineCdromSpec{
							{
								Name: "cdrom-1",
								Image: vmopv1.VirtualMachineImageRef{
									Name: vmiName,
									Kind: vmiKind,
								},
								AllowGuestControl: ptr.To(true),
								Connected:         ptr.To(true),
							},
						}
					})

					It("should power on the VM with expected CD-ROM device", func() {
						Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
						Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
						cdromDeviceList := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device).SelectByType(&vimtypes.VirtualCdrom{})
						Expect(cdromDeviceList).To(HaveLen(1))
						cdrom := cdromDeviceList[0].(*vimtypes.VirtualCdrom)
						Expect(cdrom.Connectable.StartConnected).To(BeTrue())
						Expect(cdrom.Connectable.Connected).To(BeTrue())
						Expect(cdrom.Connectable.AllowGuestControl).To(BeTrue())
						Expect(cdrom.ControllerKey).ToNot(BeZero())
						Expect(cdrom.UnitNumber).ToNot(BeNil())
						Expect(cdrom.Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualCdromIsoBackingInfo{}))
						backing := cdrom.Backing.(*vimtypes.VirtualCdromIsoBackingInfo)
						Expect(backing.FileName).To(Equal(vmiFileName))
						assertUpdate()
					})
				})
			})
		})

		When("suspending the VM", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			})
			When("suspendMode is hard", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
				})
				It("should not suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertNoUpdate()
				})
			})
			When("suspendMode is soft", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeSoft
				})
				It("should not suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertNoUpdate()
				})
			})
			When("suspendMode is trySoft", func() {
				BeforeEach(func() {
					vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				})
				It("should not suspend the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertNoUpdate()
				})
			})
		})
	})

	When("vcVM is suspended", func() {
		JustBeforeEach(func() {

			// Ensure the VM is suspended.
			switch vmCtx.MoVM.Summary.Runtime.PowerState {
			case vimtypes.VirtualMachinePowerStatePoweredOn,
				vimtypes.VirtualMachinePowerStatePoweredOff:
				t, err := vcVM.Suspend(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(t.Wait(ctx)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
			}
			Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
		})

		When("power state is not changed", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			})
			It("should not return an error", func() {
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
				assertNoUpdate()
			})
		})

		When("powering on the VM", func() {

			var (
				updateArgs *session.VMUpdateArgs
				resizeArgs *session.VMResizeArgs
			)

			BeforeEach(func() {
				resizeArgs = nil
				updateArgs = &session.VMUpdateArgs{
					ResourcePolicy: &vmopv1.VirtualMachineSetResourcePolicy{},
				}
			})

			getResizeArgs := func() (*session.VMResizeArgs, error) { //nolint:unparam
				return resizeArgs, nil
			}
			getUpdateArgs := func() (*session.VMUpdateArgs, error) { //nolint:unparam
				return updateArgs, nil
			}

			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			It("should power on the VM", func() {
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
				Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
				assertUpdate()
			})
		})

		When("powering off the VM", func() {
			BeforeEach(func() {
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})
			When("powerOffMode is hard", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
				})
				It("should power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertUpdate()
				})
			})
			When("powerOffMode is soft", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
				})
				It("should not power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
					assertNoUpdate()
				})
			})
			When("powerOffMode is trySoft", func() {
				BeforeEach(func() {
					vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
				})
				It("should power off the VM", func() {
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, nil, nil)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					Expect(vmCtx.MoVM.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
					assertUpdate()
				})
			})
		})
	})
})

var _ = Describe("UpdateVMGuestIDReconfiguredCondition", func() {

	var (
		vmCtx      pkgctx.VirtualMachineContext
		vmopVM     vmopv1.VirtualMachine
		configSpec vimtypes.VirtualMachineConfigSpec
		taskInfo   *vimtypes.TaskInfo
	)

	BeforeEach(func() {
		// Init VM with the condition set to verify it's actually deleted.
		vmopVM = vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.GuestIDReconfiguredCondition,
						Status: metav1.ConditionFalse,
					},
				},
			},
		}
		vmCtx = pkgctx.VirtualMachineContext{
			VM: &vmopVM,
		}
		configSpec = vimtypes.VirtualMachineConfigSpec{}
		taskInfo = &vimtypes.TaskInfo{}
	})

	JustBeforeEach(func() {
		session.UpdateVMGuestIDReconfiguredCondition(vmCtx.VM, configSpec, taskInfo)
	})

	Context("ConfigSpec doesn't have a guest ID", func() {

		BeforeEach(func() {
			configSpec.GuestId = ""
		})

		It("should delete the existing VM's guest ID condition", func() {
			Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
		})
	})

	Context("ConfigSpec has a guest ID", func() {

		BeforeEach(func() {
			configSpec.GuestId = "test-guest-id-value"
		})

		When("TaskInfo is nil", func() {

			BeforeEach(func() {
				taskInfo = nil
			})

			It("should delete the VM's guest ID condition", func() {
				Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
			})
		})

		When("TaskInfo.Error is nil", func() {

			BeforeEach(func() {
				taskInfo.Error = nil
			})

			It("should delete the VM's guest ID condition", func() {
				Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
			})
		})

		When("TaskInfo.Error.Fault is not an InvalidPropertyFault", func() {

			BeforeEach(func() {
				taskInfo.Error = &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidName{
						Name: "some-invalid-name",
					},
				}
			})

			It("should delete the VM's guest ID condition", func() {
				Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
			})
		})

		When("TaskInfo.Error contains an invalid property error NOT about guestID", func() {

			BeforeEach(func() {
				taskInfo.Error = &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidArgument{
						InvalidProperty: "config.version",
					},
				}
			})

			It("should delete the VM's guest ID condition", func() {
				Expect(conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
			})
		})

		When("TaskInfo.Error contains an invalid property error of guestID", func() {

			BeforeEach(func() {
				taskInfo.Error = &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidArgument{
						InvalidProperty: "configSpec.guestId",
					},
				}
			})

			It("should set the VM's guest ID condition to false with the invalid value in the reason", func() {
				c := conditions.Get(&vmopVM, vmopv1.GuestIDReconfiguredCondition)
				Expect(c).NotTo(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
				Expect(c.Reason).To(Equal("Invalid"))
				Expect(c.Message).To(Equal("The specified guest ID value is not supported: test-guest-id-value"))
			})
		})
	})
})
