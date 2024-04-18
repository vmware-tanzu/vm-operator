// Copyright (c) 2021-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("UpdateVM selected MO Properties", func() {

	It("Covers VM Status properties", func() {
		for _, p := range vmlifecycle.VMStatusPropertiesSelector {
			match := false
			for _, pp := range session.VMUpdatePropertiesSelector {
				if p == pp || strings.HasPrefix(p, pp+".") {
					match = true
					break
				}
			}
			Expect(match).To(BeTrue(), "Status prop %q not found in update props", p)
		}
	})
})

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

	Context("CPU Allocation", func() {
		var vmClassSpec *vmopv1.VirtualMachineClassSpec
		var minCPUFreq uint64 = 1

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}
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
				config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(virtualmachine.CPUQuantityToMhz(r, minCPUFreq)),
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
				config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Limit: ptr.To(virtualmachine.CPUQuantityToMhz(r, minCPUFreq)),
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
				config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Limit: ptr.To(10 * virtualmachine.CPUQuantityToMhz(r, minCPUFreq)),
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
				config.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(10 * virtualmachine.CPUQuantityToMhz(r, minCPUFreq)),
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
		var vmClassSpec *vmopv1.VirtualMachineClassSpec

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}
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
				config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(virtualmachine.MemoryQuantityToMb(r)),
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
				config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Limit: ptr.To(virtualmachine.MemoryQuantityToMb(r)),
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
				config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Limit: ptr.To(10 * virtualmachine.MemoryQuantityToMb(r)),
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
				config.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(10 * virtualmachine.MemoryQuantityToMb(r)),
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

			ecMap = util.ExtraConfigToMap(configSpec.ExtraConfig)
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

		It("configSpec cbt set to true OMG", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: false,
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to true OMG w Advanced set to nil", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = nil

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeFalse())
		})

		It("configSpec cbt set to false", func() {
			config.ChangeTrackingEnabled = ptr.To(false)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: true,
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).ToNot(BeNil())
			Expect(*configSpec.ChangeTrackingEnabled).To(BeTrue())
		})

		It("configSpec cbt matches", func() {
			config.ChangeTrackingEnabled = ptr.To(true)
			vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
				ChangeBlockTracking: true,
			}

			session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
			Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
		})

		Context("configSpec.cbt is false and spec.cbt is true", func() {
			BeforeEach(func() {
				config.ChangeTrackingEnabled = ptr.To(false)
				vmSpec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ChangeBlockTracking: true,
				}
			})

			It("classConfigSpec not nil and same as configInfo", func() {
				classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
					ChangeTrackingEnabled: ptr.To(false),
				}

				session.UpdateConfigSpecChangeBlockTracking(ctx, config, configSpec, classConfigSpec, vmSpec)
				Expect(configSpec.ChangeTrackingEnabled).To(BeNil())
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
})
