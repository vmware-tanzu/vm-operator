// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	pkgclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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
		var vm *vmopv1.VirtualMachine
		var globalExtraConfig map[string]string
		var ecMap map[string]string

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: make(map[string]string),
				},
			}
			globalExtraConfig = make(map[string]string)
		})

		JustBeforeEach(func() {
			session.UpdateConfigSpecExtraConfig(
				ctx,
				config,
				configSpec,
				vm,
				globalExtraConfig)

			ecMap = pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
		})

		Context("Empty input", func() {
			Specify("no changes", func() {
				Expect(ecMap).To(BeEmpty())
			})
		})

		When("vm and globalExtraConfig params are nil", func() {
			BeforeEach(func() {
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
			When("vm params are nil", func() {
				BeforeEach(func() {
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
	})

	Context("ChangeBlockTracking", func() {
		var vmSpec vmopv1.VirtualMachineSpec

		BeforeEach(func() {
			config.ChangeTrackingEnabled = nil
		})

		AfterEach(func() {
			configSpec.ChangeTrackingEnabled = nil
		})

		It("cbt and status cbt unset", func() {
			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to true", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(false),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to true with Advanced set to nil", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = nil

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = ptr.To(false)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(true),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: ptr.To(true),
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})
	})

	XContext("Ethernet Card Changes", func() {
		var expectedList object.VirtualDeviceList
		var currentList object.VirtualDeviceList
		var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
		var dvpg1 *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var dvpg2 *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo
		var results network.NetworkInterfaceResults
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
			for i := range expectedList {
				results.Results = append(results.Results,
					network.NetworkInterfaceResult{
						Device: expectedList[i],
					})
			}
			deviceChanges, err = session.UpdateEthCardDeviceChanges(ctx, &results, currentList)
		})

		AfterEach(func() {
			currentList = nil
			expectedList = nil
			results = network.NetworkInterfaceResults{}
		})

		Context("No devices", func() {
			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
				Expect(results.UpdatedEthCards).To(BeFalse())
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
				Expect(deviceChanges).To(BeEmpty())
				Expect(results.UpdatedEthCards).To(BeFalse())
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
				Expect(results.UpdatedEthCards).To(BeTrue())

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
				Expect(results.UpdatedEthCards).To(BeTrue())

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
				Expect(results.UpdatedEthCards).To(BeTrue())

				configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card2.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

				configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(card1.GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			})
		})

		XContext("Add and remove device when card type is different", func() {
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
				Expect(results.UpdatedEthCards).To(BeTrue())

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
				Expect(results.UpdatedEthCards).To(BeFalse())
				expectedList = append(expectedList, card1)

				card2, err = object.EthernetCardTypes().CreateEthernetCard("vmxnet3", dvpg1)
				Expect(err).ToNot(HaveOccurred())
				card2.GetVirtualDevice().Key = key2
				currentList = append(currentList, card2)
			})

			It("returns empty list", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(deviceChanges).To(BeEmpty())
				Expect(results.UpdatedEthCards).To(BeFalse())
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
		updateArgs *session.VMUpdateArgs
		resizeArgs *session.VMResizeArgs
		vmProps    = vsphere.VMUpdatePropertiesSelector
	)

	BeforeEach(func() {
		testConfig.NumFaultDomains = 1
		vm = builder.DummyVirtualMachine()
		vm.Name = "my-vm"
		vm.Namespace = "my-namespace"
		vm.Spec.Network.Interfaces = nil
		vm.Spec.Volumes = nil
		vm.Spec.Cdrom = nil
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

		configSpec := vimtypes.VirtualMachineConfigSpec{
			Annotation: constants.VCVMAnnotation,
			GuestId:    string(vimtypes.VirtualMachineGuestOsIdentifierCentosGuest),
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
		}

		devList, err := vcVM.Device(ctx)
		Expect(err).ToNot(HaveOccurred())

		if devs := devList.SelectByType(&vimtypes.VirtualCdrom{}); len(devs) > 0 {
			for i := range devs {
				configSpec.DeviceChange = append(configSpec.DeviceChange,
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
						Device:    devs[i],
					},
				)
			}
		}

		if devs := devList.SelectByType(&vimtypes.VirtualEthernetCard{}); len(devs) > 0 {
			for i := range devs {
				configSpec.DeviceChange = append(configSpec.DeviceChange,
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
						Device:    devs[i],
					},
				)
			}
		}

		// Update the vSphere VM with the VM namespace/name/managedBy
		t, err := vcVM.Reconfigure(ctx, configSpec)
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

	assertUpdate := func() {
		ExpectWithOffset(1, ctxop.IsUpdate(vmCtx)).To(BeTrue())
	}

	When("VM is powered off", func() {
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
			It("should resize the boot disk", func() {
				// Reconfigure
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(session.ErrReconfigure))
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
				// Customize
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
				// No-op
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())

				vmDevs := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
				disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
				Expect(disks).To(HaveLen(1))
				Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
				diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
				Expect(diskCapacityBytes).To(Equal(newDiskSizeBytes))
				assertUpdate()
			})
		})

		When("there are no NICs", func() {
			BeforeEach(func() {
				vm.Spec.Network.Interfaces = nil
			})
			It("should customize the VM", func() {
				// Customize
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
				// No-op
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
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
				It("should customize VM", func() {
					// Customize
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					// No-op
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					assertUpdate()
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

				It("should update the VM with the specified guest ID", func() {
					// Reconfigure
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(session.ErrReconfigure))
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					// Customize
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					// No-op
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
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

					// Customize
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					// No-op
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())

					Expect(conditions.Get(vm, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
					assertUpdate()
				})
			})
		})

		When("VM has CD-ROM", func() {

			const (
				vmiName     = "vmi-iso"
				vmiKind     = "VirtualMachineImage"
				vmiFileName = "dummy.iso"
			)

			BeforeEach(func() {
				vm.Spec.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name: "cdrom1",
						Image: vmopv1.VirtualMachineImageRef{
							Name: vmiName,
							Kind: vmiKind,
						},
						AllowGuestControl: ptr.To(true),
						Connected:         ptr.To(true),
					},
				}

				testConfig.WithContentLibrary = true
			})

			JustBeforeEach(func() {
				// Add required objects to get CD-ROM backing file name.
				objs := builder.DummyImageAndItemObjectsForCdromBacking(
					vmiName,
					vmCtx.VM.Namespace,
					vmiKind,
					vmiFileName,
					ctx.ContentLibraryIsoItemID,
					true,
					true,
					resource.MustParse("100Mi"),
					true,
					true,
					"ISO")
				for _, obj := range objs {
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				}
			})

			It("should reconfigure the VM with the expected CD-ROM device", func() {
				// Reconfigure
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(session.ErrReconfigure))
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
				// No-op
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())

				cdromDeviceList := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device).SelectByType(&vimtypes.VirtualCdrom{})
				Expect(cdromDeviceList).To(HaveLen(1))
				cdrom := cdromDeviceList[0].(*vimtypes.VirtualCdrom)
				Expect(cdrom.Connectable.StartConnected).To(BeTrue())
				Expect(cdrom.Connectable.Connected).To(BeFalse())
				Expect(cdrom.Connectable.AllowGuestControl).To(BeTrue())
				Expect(cdrom.ControllerKey).ToNot(BeZero())
				Expect(cdrom.UnitNumber).ToNot(BeNil())
				Expect(cdrom.Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualCdromIsoBackingInfo{}))
				backing := cdrom.Backing.(*vimtypes.VirtualCdromIsoBackingInfo)
				Expect(backing.FileName).To(Equal(vmiFileName))
				assertUpdate()
			})

			When("the boot disk size is changed for VM with CD-ROM", func() {

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
				})

				It("should reconfigure the VM without resizing the boot disk", func() {
					// Reconfigure
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(MatchError(session.ErrReconfigure))
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())
					// No-op
					Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
					Expect(vcVM.Properties(ctx, vcVM.Reference(), vmProps, &vmCtx.MoVM)).To(Succeed())

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
		})
	})

	When("VM's resource police changes", func() {
		var (
			dummyRP    *vmopv1.VirtualMachineSetResourcePolicy
			cmProvider clustermodules.Provider
		)

		BeforeEach(func() {
			dummyRP = builder.DummyVirtualMachineSetResourcePolicy()
			resizeArgs = &session.VMResizeArgs{}
			updateArgs = &session.VMUpdateArgs{}
			resizeArgs.ResourcePolicy = dummyRP
			updateArgs.ResourcePolicy = dummyRP
		})

		JustBeforeEach(func() {
			cmProvider = clustermodules.NewProvider(sess.Client.RestClient())
		})

		AfterEach(func() {
			dummyRP = nil
			cmProvider = nil
		})

		verifyClusterModuleAdd := func() {
			Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
			Expect(vmCtx.VM.Annotations[pkgconst.ClusterModuleUUIDAnnotationKey]).To(
				Equal(dummyRP.Status.ClusterModules[0].ModuleUuid))
			added, err := cmProvider.IsMoRefModuleMember(vmCtx, dummyRP.Status.ClusterModules[0].ModuleUuid, vmCtx.MoVM.Self)
			Expect(err).ToNot(HaveOccurred())
			Expect(added).To(BeTrue())
		}

		It("should updates its cluster module membership accordingly", func() {
			Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())

			By("set up vm's vsphere-cluster-module-group annotation", func() {
				metav1.SetMetaDataAnnotation(&vmCtx.VM.ObjectMeta,
					pkgconst.ClusterModuleNameAnnotationKey,
					builder.DummyClusterModule)
			})

			By("bypass annotation check and return cluster module not found error", func() {
				err := sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf(
					" %q not found in VirtualMachineSetResourcePolicy", builder.DummyClusterModule))))
			})

			By("simulate vsphere-cluster-module-group-uuid is unchanged", func() {
				dummyRP.Status.ClusterModules = []vmopv1.VSphereClusterModuleStatus{
					{
						GroupName:   builder.DummyClusterModule,
						ClusterMoID: sess.ClusterMoRef.Value,
						ModuleUuid:  uuid.NewString(),
					},
				}
				metav1.SetMetaDataAnnotation(&vmCtx.VM.ObjectMeta,
					pkgconst.ClusterModuleUUIDAnnotationKey,
					dummyRP.Status.ClusterModules[0].ModuleUuid)
			})

			By("exit early without adding the vm to the cluster module", func() {
				Expect(sess.UpdateVirtualMachine(vmCtx, vcVM, getUpdateArgs, getResizeArgs)).To(Succeed())
			})

			By("set up a dummy cluster module and update VirtualMachineSetResourcePolicy status", func() {
				cmID, err := cmProvider.CreateModule(vmCtx, sess.ClusterMoRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(cmID).ToNot(BeEmpty())
				exist, err := cmProvider.DoesModuleExist(vmCtx, cmID, sess.ClusterMoRef)
				Expect(err).ToNot(HaveOccurred())
				Expect(exist).To(BeTrue())
				dummyRP.Status.ClusterModules[0].ModuleUuid = cmID
				delete(vmCtx.VM.ObjectMeta.Annotations, pkgconst.ClusterModuleUUIDAnnotationKey)
			})

			By("add vm to the cluster module and add vsphere-cluster-module-group-uuid annotation to vm", verifyClusterModuleAdd)

			By("simulate cluster module changes", func() {
				Expect(cmProvider.DeleteModule(ctx, dummyRP.Status.ClusterModules[0].ModuleUuid)).To(Succeed())
				exist, err := cmProvider.DoesModuleExist(vmCtx, dummyRP.Status.ClusterModules[0].ModuleUuid, sess.ClusterMoRef)
				Expect(err).ToNot(HaveOccurred())
				Expect(exist).To(BeFalse())

				newID, err := cmProvider.CreateModule(ctx, sess.ClusterMoRef)
				Expect(err).NotTo(HaveOccurred())
				Expect(newID).ToNot(BeEmpty())
				dummyRP.Status.ClusterModules[0].ModuleUuid = newID
			})

			By("add vm to the cluster module and add vsphere-cluster-module-group-uuid annotation to vm", verifyClusterModuleAdd)
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
