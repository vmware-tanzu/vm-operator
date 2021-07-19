// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	goctx "context"
	"fmt"
	"sync/atomic"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("Update ConfigSpec", func() {

	var config *vimTypes.VirtualMachineConfigInfo
	var configSpec *vimTypes.VirtualMachineConfigSpec

	// represents the VM Service FSS. This should be manipulated atomically to avoid races where
	// the controller is trying to read this _while_ the tests are updating it.
	var vmServiceFSS uint32

	BeforeEach(func() {
		config = &vimTypes.VirtualMachineConfigInfo{}
		configSpec = &vimTypes.VirtualMachineConfigSpec{}
	})

	// Just a few examples for testing these things here. Need to think more about whether this
	// is a good way or not. Probably better to do this via UpdateVirtualMachine when we have
	// better integration tests.

	Context("Basic Hardware", func() {
		var vmClassSpec *vmopv1alpha1.VirtualMachineClassSpec

		BeforeEach(func() {
			vmClassSpec = &vmopv1alpha1.VirtualMachineClassSpec{}
		})

		JustBeforeEach(func() {
			session.UpdateHardwareConfigSpec(config, configSpec, vmClassSpec)
		})

		It("config spec is empty", func() {
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.ManagedBy).ToNot(BeNil())
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

	Context("CPU Allocation", func() {
		var vmClassSpec *vmopv1alpha1.VirtualMachineClassSpec
		var minCPUFreq uint64 = 1

		BeforeEach(func() {
			vmClassSpec = &vmopv1alpha1.VirtualMachineClassSpec{}
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecCPUAllocation(config, configSpec, vmClassSpec, minCPUFreq)
		})

		It("config spec is empty", func() {
			Expect(configSpec.CpuAllocation).To(BeNil())
		})

		Context("config matches class policy request", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.CpuAllocation = &vimTypes.ResourceAllocationInfo{
					Reservation: pointer.Int64Ptr(session.CpuQuantityToMhz(r, minCPUFreq)),
				}
				vmClassSpec.Policies.Resources.Requests.Cpu = r
			})

			It("config spec is empty", func() {
				Expect(configSpec.CpuAllocation).To(BeNil())
			})
		})

		Context("config matches class policy limit", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.CpuAllocation = &vimTypes.ResourceAllocationInfo{
					Limit: pointer.Int64Ptr(session.CpuQuantityToMhz(r, minCPUFreq)),
				}
				vmClassSpec.Policies.Resources.Limits.Cpu = r
			})

			It("config spec is empty", func() {
				Expect(configSpec.CpuAllocation).To(BeNil())
			})
		})

		Context("config matches is different from policy limit", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.CpuAllocation = &vimTypes.ResourceAllocationInfo{
					Limit: pointer.Int64Ptr(10 * session.CpuQuantityToMhz(r, minCPUFreq)),
				}
				vmClassSpec.Policies.Resources.Limits.Cpu = r
			})

			It("config spec is not empty", func() {
				Expect(configSpec.CpuAllocation).ToNot(BeNil())
				Expect(configSpec.CpuAllocation.Reservation).To(BeNil())
				Expect(configSpec.CpuAllocation.Limit).ToNot(BeNil())
				Expect(*configSpec.CpuAllocation.Limit).To(BeNumerically("==", 100*1024*1024))
			})
		})

		Context("config matches is different from policy request", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.CpuAllocation = &vimTypes.ResourceAllocationInfo{
					Reservation: pointer.Int64Ptr(10 * session.CpuQuantityToMhz(r, minCPUFreq)),
				}
				vmClassSpec.Policies.Resources.Requests.Cpu = r
			})

			It("config spec is not empty", func() {
				Expect(configSpec.CpuAllocation).ToNot(BeNil())
				Expect(configSpec.CpuAllocation.Limit).To(BeNil())
				Expect(configSpec.CpuAllocation.Reservation).ToNot(BeNil())
				Expect(*configSpec.CpuAllocation.Reservation).To(BeNumerically("==", 100*1024*1024))
			})
		})
	})

	Context("Memory Allocation", func() {
		var vmClassSpec *vmopv1alpha1.VirtualMachineClassSpec

		BeforeEach(func() {
			vmClassSpec = &vmopv1alpha1.VirtualMachineClassSpec{}
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecMemoryAllocation(config, configSpec, vmClassSpec)
		})

		It("config spec is empty", func() {
			Expect(configSpec.MemoryAllocation).To(BeNil())
		})

		Context("config matches class policy request", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
					Reservation: pointer.Int64Ptr(session.MemoryQuantityToMb(r)),
				}
				vmClassSpec.Policies.Resources.Requests.Memory = r
			})

			It("config spec is empty", func() {
				Expect(configSpec.MemoryAllocation).To(BeNil())
			})
		})

		Context("config matches class policy limit", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
					Limit: pointer.Int64Ptr(session.MemoryQuantityToMb(r)),
				}
				vmClassSpec.Policies.Resources.Limits.Memory = r
			})

			It("config spec is empty", func() {
				Expect(configSpec.MemoryAllocation).To(BeNil())
			})
		})

		Context("config matches is different from policy limit", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
					Limit: pointer.Int64Ptr(10 * session.MemoryQuantityToMb(r)),
				}
				vmClassSpec.Policies.Resources.Limits.Memory = r
			})

			It("config spec is not empty", func() {
				Expect(configSpec.MemoryAllocation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Reservation).To(BeNil())
				Expect(configSpec.MemoryAllocation.Limit).ToNot(BeNil())
				Expect(*configSpec.MemoryAllocation.Limit).To(BeNumerically("==", 100))
			})
		})

		Context("config matches is different from policy request", func() {
			BeforeEach(func() {
				r := resource.MustParse("100Mi")
				config.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
					Reservation: pointer.Int64Ptr(10 * session.MemoryQuantityToMb(r)),
				}
				vmClassSpec.Policies.Resources.Requests.Memory = r
			})

			It("config spec is not empty", func() {
				Expect(configSpec.MemoryAllocation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Limit).To(BeNil())
				Expect(configSpec.MemoryAllocation.Reservation).ToNot(BeNil())
				Expect(*configSpec.MemoryAllocation.Reservation).To(BeNumerically("==", 100))
			})
		})
	})

	Context("ExtraConfig", func() {
		var vmImage *vmopv1alpha1.VirtualMachineImage
		var vmClassSpec *vmopv1alpha1.VirtualMachineClassSpec
		var vm *vmopv1alpha1.VirtualMachine
		var vmMetadata *vmprovider.VmMetadata
		var globalExtraConfig map[string]string
		var ecMap map[string]string

		BeforeEach(func() {
			vmImage = &vmopv1alpha1.VirtualMachineImage{}
			vmClassSpec = &vmopv1alpha1.VirtualMachineClassSpec{}
			vm = &vmopv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: make(map[string]string),
				},
			}
			vmMetadata = &vmprovider.VmMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
			}
			globalExtraConfig = make(map[string]string)
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecExtraConfig(
				config,
				configSpec,
				vmImage,
				vmClassSpec,
				vm,
				vmMetadata,
				globalExtraConfig)

			ecMap = make(map[string]string)
			for _, ec := range configSpec.ExtraConfig {
				if optionValue := ec.GetOptionValue(); optionValue != nil {
					ecMap[optionValue.Key] = optionValue.Value.(string)
				}
			}
		})

		Context("Empty input", func() {
			It("No changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		Context("Updates configSpec.ExtraConfig", func() {
			BeforeEach(func() {
				conditions.MarkTrue(vmImage, vmopv1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
				config.ExtraConfig = append(config.ExtraConfig, &vimTypes.OptionValue{
					Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigReady})
				vmMetadata.Data["guestinfo.test"] = "test"
				vmMetadata.Data["nvram"] = "this should ignored"
				globalExtraConfig["global"] = "test"
			})

			It("Expected configSpec.ExtraConfig", func() {
				By("VM Metadata", func() {
					Expect(ecMap).To(HaveKeyWithValue("guestinfo.test", "test"))
					Expect(ecMap).ToNot(HaveKey("nvram"))
				})

				By("VM Image compatible", func() {
					Expect(ecMap).To(HaveKeyWithValue("guestinfo.vmservice.defer-cloud-init", "enabled"))
				})

				By("Global map", func() {
					Expect(ecMap).To(HaveKeyWithValue("global", "test"))
				})
			})
		})

		Context("ExtraConfig value already exists", func() {
			BeforeEach(func() {
				config.ExtraConfig = append(config.ExtraConfig, &vimTypes.OptionValue{Key: "foo", Value: "bar"})
				vmMetadata.Data["foo"] = "bar"
			})

			It("No changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		Context("when ThunderPciDevicesFSS is enabled", func() {
			var oldThunderPciDevicesFSSEnableFunc func() bool
			BeforeEach(func() {
				oldThunderPciDevicesFSSEnableFunc = lib.IsThunderPciDevicesFSSEnabled
				lib.IsThunderPciDevicesFSSEnabled = func() bool {
					return true
				}
			})
			AfterEach(func() {
				lib.IsThunderPciDevicesFSSEnabled = oldThunderPciDevicesFSSEnableFunc
			})

			Context("when virtual devices are not present", func() {
				It("No Changes", func() {
					Expect(ecMap).To(BeEmpty())
				})
			})

			Context("when vGPU device is available", func() {
				BeforeEach(func() {
					vmClassSpec.Hardware.Devices = vmopv1alpha1.VirtualDevices{VGPUDevices: []vmopv1alpha1.VGPUDevice{
						{
							ProfileName: "test-vgpu-profile",
						},
					}}
				})

				It("maintenance mode powerOff extraConfig should be added", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.MMPowerOffVMExtraConfigKey, constants.ExtraConfigTrue))
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
					vmClassSpec.Hardware.Devices = vmopv1alpha1.VirtualDevices{DynamicDirectPathIODevices: []vmopv1alpha1.DynamicDirectPathIODevice{
						{
							VendorID:    123,
							DeviceID:    24,
							CustomLabel: "",
						},
					}}
				})

				It("maintenance mode powerOff extraConfig should be added", func() {
					Expect(ecMap).To(HaveKeyWithValue(constants.MMPowerOffVMExtraConfigKey, constants.ExtraConfigTrue))
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
	})

	Context("VAppConfig", func() {
		var vmMetadata *vmprovider.VmMetadata

		BeforeEach(func() {
			vmMetadata = &vmprovider.VmMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1alpha1.VirtualMachineMetadataOvfEnvTransport,
			}
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecVAppConfig(
				config,
				configSpec,
				vmMetadata)
		})

		Context("Empty input", func() {
			It("No changes", func() {
				Expect(configSpec.VAppConfig).To(BeNil())
			})
		})

		Context("update to user configurable field", func() {
			BeforeEach(func() {
				vmMetadata.Data["foo"] = "bar"
				config.VAppConfig = &vimTypes.VmConfigInfo{
					Property: []vimTypes.VAppPropertyInfo{
						{
							Id:               "foo",
							Value:            "should-change",
							UserConfigurable: pointer.BoolPtr(true),
						},
					},
				}
			})

			It("Updates configSpec.VAppConfig", func() {
				Expect(configSpec.VAppConfig).ToNot(BeNil())
				vmCs := configSpec.VAppConfig.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Value).To(Equal("bar"))
			})
		})
	})

	Context("ChangeBlockTracking", func() {
		var vmSpec vmopv1alpha1.VirtualMachineSpec

		BeforeEach(func() {
			vmSpec = vmopv1alpha1.VirtualMachineSpec{
				AdvancedOptions: &vmopv1alpha1.VirtualMachineAdvancedOptions{},
			}
			config.ChangeTrackingEnabled = nil
		})

		It("cbt and status cbt unset", func() {
			session.UpdateConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to true", func() {
			config.ChangeTrackingEnabled = pointer.BoolPtr(true)
			vmSpec.AdvancedOptions.ChangeBlockTracking = pointer.BoolPtr(false)

			session.UpdateConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = pointer.BoolPtr(false)
			vmSpec.AdvancedOptions.ChangeBlockTracking = pointer.BoolPtr(true)

			session.UpdateConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = pointer.BoolPtr(true)
			vmSpec.AdvancedOptions.ChangeBlockTracking = pointer.BoolPtr(true)

			session.UpdateConfigSpecChangeBlockTracking(config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})
	})

	Context("Firmware", func() {
		var vm *vmopv1alpha1.VirtualMachine

		BeforeEach(func() {
			vm = &vmopv1alpha1.VirtualMachine{
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
		var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
		var dvpg1 *vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var dvpg2 *vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var err error

		BeforeEach(func() {
			dvpg1 = &vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimTypes.DistributedVirtualSwitchPortConnection{
					PortgroupKey: "key1",
					SwitchUuid:   "uuid1",
				},
			}

			dvpg2 = &vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: vimTypes.DistributedVirtualSwitchPortConnection{
					PortgroupKey: "key2",
					SwitchUuid:   "uuid2",
				},
			}
		})

		JustBeforeEach(func() {
			deviceChanges, err = session.UpdateEthCardDeviceChanges(expectedList, currentList)
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

		Context("Add device", func() {
			var card1 vimTypes.BaseVirtualDevice
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
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when backing change", func() {
			var card1 vimTypes.BaseVirtualDevice
			var card2 vimTypes.BaseVirtualDevice

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
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when MAC address is different", func() {
			var card1 vimTypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimTypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				card1.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimTypes.VirtualEthernetCardMacTypeManual)
				card1.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = "mac1"
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				card2.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimTypes.VirtualEthernetCardMacTypeManual)
				card2.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = "mac2"
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Add and remove device when ExternalID is different", func() {
			var card1 vimTypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimTypes.BaseVirtualDevice
			var key2 int32 = 200

			BeforeEach(func() {
				card1, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card1.GetVirtualDevice().Key = key1
				card1.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId = "ext1"
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				card2.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId = "ext2"
				currentList = append(currentList, card2)
			})

			It("returns remove and add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(HaveLen(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		Context("Keeps existing device with same backing", func() {
			var card1 vimTypes.BaseVirtualDevice
			var key1 int32 = 100
			var card2 vimTypes.BaseVirtualDevice
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
		var vgpuDevices = []vmopv1alpha1.VGPUDevice{
			{
				ProfileName: "SampleProfile",
			},
		}
		var ddpioDevices = []vmopv1alpha1.DynamicDirectPathIODevice{
			{
				VendorID:    42,
				DeviceID:    43,
				CustomLabel: "SampleLabel",
			},
		}
		var pciDevices vmopv1alpha1.VirtualDevices
		Context("For vGPU device", func() {
			BeforeEach(func() {
				pciDevices = vmopv1alpha1.VirtualDevices{
					VGPUDevices: vgpuDevices,
				}
			})
			It("should create vSphere device with VmiopBackingInfo", func() {
				vSphereDevices := session.CreatePCIDevices(pciDevices)
				Expect(len(vSphereDevices)).To(Equal(1))
				virtualDevice := vSphereDevices[0].GetVirtualDevice()
				backing := virtualDevice.Backing.(*vimTypes.VirtualPCIPassthroughVmiopBackingInfo)
				Expect(backing.Vgpu).To(Equal(pciDevices.VGPUDevices[0].ProfileName))
			})
		})
		Context("For Dynamic DirectPath I/O device", func() {
			BeforeEach(func() {
				pciDevices = vmopv1alpha1.VirtualDevices{
					DynamicDirectPathIODevices: ddpioDevices,
				}
			})
			It("should create vSphere device with DynamicBackingInfo", func() {
				vSphereDevices := session.CreatePCIDevices(pciDevices)
				Expect(len(vSphereDevices)).To(Equal(1))
				virtualDevice := vSphereDevices[0].GetVirtualDevice()
				backing := virtualDevice.Backing.(*vimTypes.VirtualPCIPassthroughDynamicBackingInfo)
				Expect(backing.AllowedDevice[0].DeviceId).To(Equal(int32(pciDevices.DynamicDirectPathIODevices[0].DeviceID)))
				Expect(backing.AllowedDevice[0].VendorId).To(Equal(int32(pciDevices.DynamicDirectPathIODevices[0].VendorID)))
				Expect(backing.CustomLabel).To(Equal(pciDevices.DynamicDirectPathIODevices[0].CustomLabel))
			})
		})
	})

	Context("PCI Device Changes", func() {
		var (
			currentList, expectedList object.VirtualDeviceList
			deviceChanges             []vimTypes.BaseVirtualDeviceConfigSpec
			err                       error

			// Variables related to vGPU devices.
			backingInfo1, backingInfo2 *vimTypes.VirtualPCIPassthroughVmiopBackingInfo
			deviceKey1, deviceKey2     int32
			vGPUDevice1, vGPUDevice2   vimTypes.BaseVirtualDevice

			// Variables related to dynamicDirectPathIO devices.
			allowedDev1, allowedDev2                         vimTypes.VirtualPCIPassthroughAllowedDevice
			backingInfo3, backingInfo4                       *vimTypes.VirtualPCIPassthroughDynamicBackingInfo
			deviceKey3, deviceKey4                           int32
			dynamicDirectPathIODev1, dynamicDirectPathIODev2 vimTypes.BaseVirtualDevice

			oldVMServiceFSSState uint32
		)

		BeforeEach(func() {
			oldVMServiceFSSState = vmServiceFSS
			atomic.StoreUint32(&vmServiceFSS, 1)

			backingInfo1 = &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop1"}
			backingInfo2 = &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop2"}
			deviceKey1 = int32(-200)
			deviceKey2 = int32(-201)
			vGPUDevice1 = session.CreatePCIPassThroughDevice(deviceKey1, backingInfo1)
			vGPUDevice2 = session.CreatePCIPassThroughDevice(deviceKey2, backingInfo2)

			allowedDev1 = vimTypes.VirtualPCIPassthroughAllowedDevice{
				VendorId: 1000,
				DeviceId: 100,
			}
			allowedDev2 = vimTypes.VirtualPCIPassthroughAllowedDevice{
				VendorId: 2000,
				DeviceId: 200,
			}
			backingInfo3 = &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
				AllowedDevice: []vimTypes.VirtualPCIPassthroughAllowedDevice{allowedDev1},
				CustomLabel:   "sampleLabel3",
			}
			backingInfo4 = &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
				AllowedDevice: []vimTypes.VirtualPCIPassthroughAllowedDevice{allowedDev2},
				CustomLabel:   "sampleLabel4",
			}
			deviceKey3 = int32(-202)
			deviceKey4 = int32(-203)
			dynamicDirectPathIODev1 = session.CreatePCIPassThroughDevice(deviceKey3, backingInfo3)
			dynamicDirectPathIODev2 = session.CreatePCIPassThroughDevice(deviceKey4, backingInfo4)
		})

		JustBeforeEach(func() {
			deviceChanges, err = session.UpdatePCIDeviceChanges(expectedList, currentList)
		})

		AfterEach(func() {
			currentList = nil
			expectedList = nil

			atomic.StoreUint32(&vmServiceFSS, oldVMServiceFSSState)
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
					Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
				}
			})
		})

		Context("Adding vGPU and dynamicDirectPathIO devices with same backing info", func() {
			BeforeEach(func() {
				expectedList = append(expectedList, vGPUDevice1)
				// Creating a vGPUDevice with same backingInfo1 but different deviceKey.
				vGPUDevice2 = session.CreatePCIPassThroughDevice(deviceKey2, backingInfo1)
				expectedList = append(expectedList, vGPUDevice2)
				expectedList = append(expectedList, dynamicDirectPathIODev1)
				// Creating a dynamicDirectPathIO device with same backingInfo3 but different deviceKey.
				dynamicDirectPathIODev2 = session.CreatePCIPassThroughDevice(deviceKey4, backingInfo3)
				expectedList = append(expectedList, dynamicDirectPathIODev2)
			})

			It("Should return add device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(len(expectedList)))

				for idx, dev := range deviceChanges {
					configSpec := dev.GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
				}
			})
		})

		Context("When the expected and current lists have DDPIO devices with different custom labels", func() {
			BeforeEach(func() {
				expectedList = []vimTypes.BaseVirtualDevice{dynamicDirectPathIODev1}
				// Creating a dynamicDirectPathIO device with same backing info except for the custom label.
				backingInfoDiffCustomLabel := &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
					AllowedDevice: backingInfo3.AllowedDevice,
					CustomLabel:   "DifferentLabel",
				}
				dynamicDirectPathIODev2 = session.CreatePCIPassThroughDevice(deviceKey4, backingInfoDiffCustomLabel)
				currentList = []vimTypes.BaseVirtualDevice{dynamicDirectPathIODev2}
			})

			It("should return add and remove device changes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(len(deviceChanges)).To(Equal(2))

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
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
					Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationRemove))
				}

				for i := 2; i < 4; i++ {
					configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
					Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[i-2].GetVirtualDevice().Key))
					Expect(configSpec.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
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
})

var _ = Describe("Customization", func() {

	Context("IsPending", func() {
		var extraConfig []vimTypes.BaseOptionValue
		var pending bool

		BeforeEach(func() {
			extraConfig = nil
		})

		JustBeforeEach(func() {
			pending = session.IsCustomizationPendingExtraConfig(extraConfig)
		})

		Context("Empty ExtraConfig", func() {
			It("not pending", func() {
				Expect(pending).To(BeFalse())
			})
		})

		Context("ExtraConfig with pending key", func() {
			BeforeEach(func() {
				extraConfig = append(extraConfig, &vimTypes.OptionValue{
					Key:   constants.GOSCPendingExtraConfigKey,
					Value: "/foo/bar",
				})
			})

			It("is pending", func() {
				Expect(pending).To(BeTrue())
			})
		})
	})

	Context("getLinuxCustomizationSpec", func() {
		var (
			updateArgs session.VmUpdateArgs
			macaddress = "01-23-45-67-89-AB-CD-EF"
			nameserver = "8.8.8.8"
			vmName     = "dummy-vm"
		)

		customizationAdaptorMapping := &vimTypes.CustomizationAdapterMapping{
			MacAddress: macaddress,
		}

		BeforeEach(func() {
			updateArgs.DNSServers = []string{nameserver}
			updateArgs.NetIfList = []network.NetworkInterfaceInfo{
				{
					Customization: customizationAdaptorMapping,
				},
			}
		})

		It("should return linux customization spec", func() {
			spec := session.GetLinuxCustomizationSpec(vmName, updateArgs)
			Expect(spec.GlobalIPSettings.DnsServerList).To(Equal(updateArgs.DNSServers))
			Expect(spec.NicSettingMap).To(Equal([]vimTypes.CustomizationAdapterMapping{*customizationAdaptorMapping}))
			linuxSpec := spec.Identity.(*vimTypes.CustomizationLinuxPrep)
			hostName := linuxSpec.HostName.(*vimTypes.CustomizationFixedName).Name
			Expect(hostName).To(Equal(vmName))
		})
	})

	Context("getCloudInitPrepCustomizationSpec", func() {
		var (
			currentEthCards  object.VirtualDeviceList
			updateArgs       session.VmUpdateArgs
			expectedMetadata session.CloudInitMetadata
			userdata         = "dummy-cloudInit-userdata"
			addrs            = []string{"192.168.1.37"}
			gateway          = "192.168.1.1"
			nameserver       = "8.8.8.8"
			macaddress       = "01-23-45-67-89-AB-CD-EF"
			vmName           = "dummy-vm"
		)

		BeforeEach(func() {
			netPlanEth := network.NetplanEthernet{
				Dhcp4:       false,
				Addresses:   addrs,
				Gateway4:    gateway,
				Nameservers: network.NetplanEthernetNameserver{},
			}

			updateArgs.DNSServers = []string{nameserver}
			updateArgs.NetIfList = []network.NetworkInterfaceInfo{
				{
					NetplanEthernet: netPlanEth,
				},
			}
			updateArgs.VmMetadata = &vmprovider.VmMetadata{
				Data: map[string]string{
					"user-data": userdata,
				},
			}

			expectedNetPlanEth := netPlanEth

			// Irrespective of the networkType, macAddress should be assigned.
			expectedNetPlanEth.Match.MacAddress = macaddress

			// metadata.Network.Ethernets is a map and GetNetplanEthernets() set key as "nic" + (index in NetIfList)
			// In this case, len(updateArgs.NetIfList) = 1. Hence, key = nic0
			ethName := "nic0"

			// Update nameserver settings in accordance to the expected value.
			// getCloudInitPrepCustomizationSpec() injects nameserver settings for each ethernet
			// as updateArgs.DNSServers.
			expectedNetPlanEth.Nameservers.Addresses = updateArgs.DNSServers
			expectedMetadata = session.CloudInitMetadata{
				InstanceId:    vmName,
				Hostname:      vmName,
				LocalHostname: vmName,
				Network: session.Netplan{
					Version: constants.NetPlanVersion,
					Ethernets: map[string]network.NetplanEthernet{
						ethName: expectedNetPlanEth,
					},
				},
			}
		})

		AfterEach(func() {
			currentEthCards = nil
		})

		Context("NetOp - when MacAddress is not assigned to NetworkInterface", func() {
			BeforeEach(func() {
				ethCard, err := object.EthernetCardTypes().CreateEthernetCard("vmxnet3", nil)
				Expect(err).ToNot(HaveOccurred())
				ethCard.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimTypes.VirtualEthernetCardMacTypeGenerated)
				ethCard.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = macaddress
				currentEthCards = append(currentEthCards, ethCard)

				// There is only one element in NetIfList.
				updateArgs.NetIfList[0].Device = ethCard
			})

			It("should return cloudinitPrep customization spec", func() {
				spec, err := session.GetCloudInitPrepCustomizationSpec(vmName, currentEthCards, updateArgs)
				Expect(err).ToNot(HaveOccurred())
				cloudinitPrepSpec := spec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Userdata).To(Equal(userdata))
				metadataBytes := []byte(cloudinitPrepSpec.Metadata)
				metadata := session.CloudInitMetadata{}
				err = yaml.Unmarshal(metadataBytes, &metadata)
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(Equal(expectedMetadata))
			})
		})

		Context("NCP - when MacAddress is assigned to NetworkInterface", func() {
			BeforeEach(func() {
				ethCard, err := object.EthernetCardTypes().CreateEthernetCard("vmxnet3", nil)
				Expect(err).ToNot(HaveOccurred())
				ethCard.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimTypes.VirtualEthernetCardMacTypeManual)
				ethCard.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = macaddress
				currentEthCards = append(currentEthCards, ethCard)

				// There is only one element in NetIfList.
				updateArgs.NetIfList[0].Device = ethCard
				updateArgs.NetIfList[0].NetplanEthernet.Match = network.NetplanEthernetMatch{
					MacAddress: macaddress,
				}
			})

			It("should return cloudinitPrep customization spec", func() {
				spec, err := session.GetCloudInitPrepCustomizationSpec(vmName, currentEthCards, updateArgs)
				Expect(err).ToNot(HaveOccurred())
				cloudinitPrepSpec := spec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Userdata).To(Equal(userdata))
				metadataBytes := []byte(cloudinitPrepSpec.Metadata)
				metadata := session.CloudInitMetadata{}
				err = yaml.Unmarshal(metadataBytes, &metadata)
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata).To(Equal(expectedMetadata))
			})
		})
	})
})

var _ = Describe("Template", func() {
	Context("update VmConfigArgs", func() {
		var (
			updateArgs session.VmUpdateArgs

			ip         = "192.168.1.37"
			subnetMask = "255.255.255.0"
			gateway    = "192.168.1.1"
			nameserver = "8.8.8.8"
		)

		vm := &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
		}
		vmCtx := context.VMContext{
			Context: goctx.Background(),
			Logger:  logf.Log.WithValues("vmName", vm.NamespacedName()),
			VM:      vm,
		}

		BeforeEach(func() {
			updateArgs.DNSServers = []string{nameserver}
			updateArgs.NetIfList = []network.NetworkInterfaceInfo{
				{
					IPConfiguration: network.IPConfig{
						IP:         ip,
						SubnetMask: subnetMask,
						Gateway:    gateway,
					},
				},
			}
			updateArgs.VmMetadata = &vmprovider.VmMetadata{
				Data: make(map[string]string),
			}
		})

		It("should resolve them correctly while specifying valid templates", func() {
			updateArgs.VmMetadata.Data["ip"] = "{{ (index .NetworkInterfaces 0).IP }}"
			updateArgs.VmMetadata.Data["subMask"] = "{{ (index .NetworkInterfaces 0).SubnetMask }}"
			updateArgs.VmMetadata.Data["gateway"] = "{{ (index .NetworkInterfaces 0).Gateway }}"
			updateArgs.VmMetadata.Data["nameserver"] = "{{ (index .NameServers 0) }}"

			session.UpdateVmConfigArgsTemplates(vmCtx, updateArgs)

			Expect(updateArgs.VmMetadata.Data["ip"]).To(Equal(ip))
			Expect(updateArgs.VmMetadata.Data["subMask"]).To(Equal(subnetMask))
			Expect(updateArgs.VmMetadata.Data["gateway"]).To(Equal(gateway))
			Expect(updateArgs.VmMetadata.Data["nameserver"]).To(Equal(nameserver))
		})

		It("should use the original text if resolving template failed", func() {
			updateArgs.VmMetadata.Data["ip"] = "{{ (index .NetworkInterfaces 100).IP }}"
			updateArgs.VmMetadata.Data["subMask"] = "{{ invalidTemplate }}"
			updateArgs.VmMetadata.Data["gateway"] = "{{ (index .NetworkInterfaces ).Gateway }}"
			updateArgs.VmMetadata.Data["nameserver"] = "{{ (index .NameServers 0) }}"

			session.UpdateVmConfigArgsTemplates(vmCtx, updateArgs)

			Expect(updateArgs.VmMetadata.Data["ip"]).To(Equal("{{ (index .NetworkInterfaces 100).IP }}"))
			Expect(updateArgs.VmMetadata.Data["subMask"]).To(Equal("{{ invalidTemplate }}"))
			Expect(updateArgs.VmMetadata.Data["gateway"]).To(Equal("{{ (index .NetworkInterfaces ).Gateway }}"))
			Expect(updateArgs.VmMetadata.Data["nameserver"]).To(Equal(nameserver))
		})
	})
})

var _ = Describe("Network Interfaces VM Status", func() {
	Context("nicInfoToNetworkIfStatus", func() {
		dummyMacAddress := "00:50:56:8c:7b:34"
		dummyIpAddress1 := vimTypes.NetIpConfigInfoIpAddress{
			IpAddress:    "192.168.128.5",
			PrefixLength: 16,
		}
		dummyIpAddress2 := vimTypes.NetIpConfigInfoIpAddress{
			IpAddress:    "fe80::250:56ff:fe8c:7b34",
			PrefixLength: 64,
		}
		dummyIpConfig := &vimTypes.NetIpConfigInfo{
			IpAddress: []vimTypes.NetIpConfigInfoIpAddress{
				dummyIpAddress1,
				dummyIpAddress2,
			},
		}
		guestNicInfo := vimTypes.GuestNicInfo{
			Connected:  true,
			MacAddress: dummyMacAddress,
			IpConfig:   dummyIpConfig,
		}

		It("returns populated NetworkInterfaceStatus", func() {
			networkIfStatus := session.NicInfoToNetworkIfStatus(guestNicInfo)
			Expect(networkIfStatus.MacAddress).To(Equal(dummyMacAddress))
			Expect(networkIfStatus.Connected).To(BeTrue())
			Expect(networkIfStatus.IpAddresses[0]).To(Equal("192.168.128.5/16"))
			Expect(networkIfStatus.IpAddresses[1]).To(Equal("fe80::250:56ff:fe8c:7b34/64"))
		})
	})
})

var _ = Describe("VSphere Customization Status to VM Status Condition", func() {
	Context("markCustomizationInfoCondition", func() {
		var (
			vm        *vmopv1alpha1.VirtualMachine
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1alpha1.VirtualMachine{}
			guestInfo = &vimTypes.GuestInfo{
				CustomizationInfo: &vimTypes.GuestInfoCustomizationInfo{},
			}
		})

		JustBeforeEach(func() {
			session.MarkCustomizationInfoCondition(vm, guestInfo)
		})

		Context("guestInfo unset", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo unset", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.GuestCustomizationCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo idle", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo pending", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationPendingReason, vmopv1alpha1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo running", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationRunningReason, vmopv1alpha1.ConditionSeverityInfo, ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo succeeded", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.GuestCustomizationCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo failed", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED)
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, vmopv1alpha1.GuestCustomizationFailedReason, vmopv1alpha1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("customizationInfo invalid", func() {
			BeforeEach(func() {
				guestInfo.CustomizationInfo.CustomizationStatus = "asdf"
				guestInfo.CustomizationInfo.ErrorMsg = "some error message"
			})
			It("sets condition false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.GuestCustomizationCondition, "", vmopv1alpha1.ConditionSeverityError, guestInfo.CustomizationInfo.ErrorMsg),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})

var _ = Describe("VirtualMachineTools Status to VM Status Condition", func() {
	Context("markVMToolsRunningStatusCondition", func() {
		var (
			vm        *vmopv1alpha1.VirtualMachine
			guestInfo *vimTypes.GuestInfo
		)

		BeforeEach(func() {
			vm = &vmopv1alpha1.VirtualMachine{}
			guestInfo = &vimTypes.GuestInfo{
				ToolsRunningStatus: "",
			}
		})

		JustBeforeEach(func() {
			session.MarkVMToolsRunningStatusCondition(vm, guestInfo)
		})

		Context("guestInfo is nil", func() {
			BeforeEach(func() {
				guestInfo = nil
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("ToolsRunningStatus is empty", func() {
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", ""),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is not running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning)
			})
			It("sets condition to false", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.FalseCondition(vmopv1alpha1.VirtualMachineToolsCondition, vmopv1alpha1.VirtualMachineToolsNotRunningReason, vmopv1alpha1.ConditionSeverityError, "VMware Tools is not running"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is running", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("vmtools is starting", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts)
			})
			It("sets condition true", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.TrueCondition(vmopv1alpha1.VirtualMachineToolsCondition),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
		Context("Unexpected vmtools running status", func() {
			BeforeEach(func() {
				guestInfo.ToolsRunningStatus = "blah"
			})
			It("sets condition unknown", func() {
				expectedConditions := vmopv1alpha1.Conditions{
					*conditions.UnknownCondition(vmopv1alpha1.VirtualMachineToolsCondition, "", "Unexpected VMware Tools running status"),
				}
				Expect(vm.Status.Conditions).To(conditions.MatchConditions(expectedConditions))
			})
		})
	})
})
