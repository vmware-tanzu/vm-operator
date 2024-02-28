// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CreateConfigSpec", func() {
	const vmName = "dummy-vm"

	var (
		vm              *vmopv1.VirtualMachine
		vmCtx           context.VirtualMachineContextA2
		vmClassSpec     *vmopv1.VirtualMachineClassSpec
		vmImageStatus   *vmopv1.VirtualMachineImageStatus
		minCPUFreq      uint64
		configSpec      *vimTypes.VirtualMachineConfigSpec
		classConfigSpec *vimTypes.VirtualMachineConfigSpec
		pvcVolume       = vmopv1.VirtualMachineVolume{
			Name: "vmware",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "my-pvc",
					},
				},
			},
		}
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClassA2()
		vmClassSpec = &vmClass.Spec
		vmImageStatus = &vmopv1.VirtualMachineImageStatus{Firmware: "efi"}
		minCPUFreq = 2500

		vm = builder.DummyVirtualMachineA2()
		vm.Name = vmName
		// Explicitly set these in the tests.
		vm.Spec.Volumes = nil
		vm.Spec.MinHardwareVersion = 0

		vmCtx = context.VirtualMachineContextA2{
			Context: goctx.Background(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		configSpec = virtualmachine.CreateConfigSpec(
			vmCtx,
			classConfigSpec,
			vmClassSpec,
			vmImageStatus,
			minCPUFreq)
		Expect(configSpec).ToNot(BeNil())
	})

	Context("No VM Class ConfigSpec", func() {
		BeforeEach(func() {
			classConfigSpec = nil
		})

		When("Basic ConfigSpec assertions", func() {
			It("returns expected config spec", func() {
				Expect(configSpec).ToNot(BeNil())

				Expect(configSpec.Name).To(Equal(vmName))
				Expect(configSpec.Version).To(BeEmpty())
				Expect(configSpec.Annotation).ToNot(BeEmpty())
				Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
				Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
				Expect(configSpec.CpuAllocation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation).ToNot(BeNil())
				Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
			})
		})

		When("VM has min version", func() {
			BeforeEach(func() {
				vmImageStatus = nil
				vm.Spec.MinHardwareVersion = int32(vimTypes.VMX10)
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimTypes.VMX10.String()))
			})
		})

		When("VM has PVCs", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{pvcVolume}
				vm.Spec.MinHardwareVersion = int32(vimTypes.VMX10)
			})

			It("config spec has expected version for PVCs", func() {
				Expect(configSpec.Version).To(Equal(vimTypes.VMX15.String()))
			})
		})
	})

	Context("VM Class ConfigSpec", func() {
		BeforeEach(func() {
			classConfigSpec = &vimTypes.VirtualMachineConfigSpec{
				Name:       "dont-use-this-dummy-VM",
				Annotation: "test-annotation",
				DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
					&vimTypes.VirtualDeviceConfigSpec{
						Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimTypes.VirtualE1000{
							VirtualEthernetCard: vimTypes.VirtualEthernetCard{
								VirtualDevice: vimTypes.VirtualDevice{
									Key: 4000,
								},
							},
						},
					},
				},
				Firmware: "bios",
			}
		})

		It("Returns expected config spec", func() {
			Expect(configSpec.Name).To(Equal(vmName))
			Expect(configSpec.Version).To(BeEmpty())
			Expect(configSpec.Annotation).To(Equal("test-annotation"))
			Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
			Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
			Expect(configSpec.CpuAllocation).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation).ToNot(BeNil())
			Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimTypes.VirtualE1000)
			Expect(ok).To(BeTrue())
		})

		When("Image firmware is empty", func() {
			BeforeEach(func() {
				vmImageStatus = &vmopv1.VirtualMachineImageStatus{}
			})

			It("config spec has the firmware from the class", func() {
				Expect(configSpec.Firmware).ToNot(Equal(vmImageStatus.Firmware))
				Expect(configSpec.Firmware).To(Equal(classConfigSpec.Firmware))
			})
		})

		When("VM has a valid firmware override annotation", func() {
			BeforeEach(func() {
				vm.Annotations[constants.FirmwareOverrideAnnotation] = "efi"
			})

			It("config spec has overridden firmware annotation", func() {
				Expect(configSpec.Firmware).To(Equal(vm.Annotations[constants.FirmwareOverrideAnnotation]))
			})
		})

		When("VM has an invalid firmware override annotation", func() {
			BeforeEach(func() {
				vm.Annotations[constants.FirmwareOverrideAnnotation] = "foo"
			})

			It("config spec doesn't have the invalid value", func() {
				Expect(configSpec.Firmware).ToNot(Equal(vm.Annotations[constants.FirmwareOverrideAnnotation]))
			})
		})

		When("VM Image hardware version is set", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{pvcVolume}
				vmImageStatus.HardwareVersion = pointer.Int32(int32(vimTypes.VMX21))
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimTypes.VMX21.String()))
			})
		})

		When("VM Class config spec has devices", func() {

			Context("VM Class has vGPU", func() {
				BeforeEach(func() {
					classConfigSpec.DeviceChange = append(classConfigSpec.DeviceChange,
						&vimTypes.VirtualDeviceConfigSpec{
							Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimTypes.VirtualPCIPassthrough{
								VirtualDevice: vimTypes.VirtualDevice{
									Backing: &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "profile-from-configspec",
									},
								},
							},
						},
					)
				})

				It("config spec has expected version", func() {
					Expect(configSpec.Version).To(Equal(vimTypes.VMX17.String()))
				})

				Context("VM MinHardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimTypes.MaxValidHardwareVersion)
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
					})
				})
			})

			Context("VM Class has DDPIO", func() {
				BeforeEach(func() {
					classConfigSpec.DeviceChange = append(classConfigSpec.DeviceChange,
						&vimTypes.VirtualDeviceConfigSpec{
							Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimTypes.VirtualPCIPassthrough{
								VirtualDevice: vimTypes.VirtualDevice{
									Backing: &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
										AllowedDevice: []vimTypes.VirtualPCIPassthroughAllowedDevice{
											{
												VendorId: 52,
												DeviceId: 53,
											},
										},
										CustomLabel: "label-from-configspec",
									},
								},
							},
						},
					)
				})

				It("config spec has expected version", func() {
					Expect(configSpec.Version).To(Equal("vmx-17"))
				})

				Context("VM MinHardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimTypes.MaxValidHardwareVersion)
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
					})
				})
			})
		})

		When("VM Class config spec specifies version", func() {
			BeforeEach(func() {
				classConfigSpec.Version = vimTypes.MaxValidHardwareVersion.String()
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
			})

			When("VM and/or VM Image have version set", func() {
				Context("VM and VM Image are less than class", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimTypes.MaxValidHardwareVersion - 2)
						vmImageStatus.HardwareVersion = pointer.Int32(int32(vimTypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
					})
				})

				Context("VM MinHardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimTypes.MaxValidHardwareVersion)
						vmImageStatus.HardwareVersion = pointer.Int32(int32(vimTypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
					})
				})

				Context("VM Image HardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimTypes.MaxValidHardwareVersion - 3)
						vmImageStatus.HardwareVersion = pointer.Int32(int32(vimTypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						// When the ConfigSpec.Version is set the image is ignored so it won't
						// be the expected value.
						Expect(configSpec.Version).To(Equal(vimTypes.MaxValidHardwareVersion.String()))
					})
				})
			})
		})
	})
})

var _ = Describe("CreateConfigSpecForPlacement", func() {

	var (
		vmCtx               context.VirtualMachineContextA2
		storageClassesToIDs map[string]string
		baseConfigSpec      *vimTypes.VirtualMachineConfigSpec
		configSpec          *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		baseConfigSpec = &vimTypes.VirtualMachineConfigSpec{}
		storageClassesToIDs = map[string]string{}

		vm := builder.DummyVirtualMachineA2()
		vmCtx = context.VirtualMachineContextA2{
			Context: pkgconfig.NewContext(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		configSpec = virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			baseConfigSpec,
			storageClassesToIDs)
		Expect(configSpec).ToNot(BeNil())
	})

	Context("Returns expected ConfigSpec", func() {
		BeforeEach(func() {
			baseConfigSpec = &vimTypes.VirtualMachineConfigSpec{
				Name:       "dummy-VM",
				Annotation: "test-annotation",
				NumCPUs:    42,
				MemoryMB:   4096,
				Firmware:   "secret-sauce",
				DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
					&vimTypes.VirtualDeviceConfigSpec{
						Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimTypes.VirtualPCIPassthrough{
							VirtualDevice: vimTypes.VirtualDevice{
								Backing: &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "SampleProfile2",
								},
							},
						},
					},
					&vimTypes.VirtualDeviceConfigSpec{
						Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimTypes.VirtualVmxnet3{
							VirtualVmxnet: vimTypes.VirtualVmxnet{
								VirtualEthernetCard: vimTypes.VirtualEthernetCard{
									VirtualDevice: vimTypes.VirtualDevice{
										Backing: &vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{},
									},
								},
							},
						},
					},
				},
			}
		})

		It("Placement ConfigSpec contains expected field set sans ethernet device from class config spec", func() {
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal(baseConfigSpec.Annotation))
			Expect(configSpec.NumCPUs).To(Equal(baseConfigSpec.NumCPUs))
			Expect(configSpec.MemoryMB).To(Equal(baseConfigSpec.MemoryMB))
			Expect(configSpec.CpuAllocation).To(Equal(baseConfigSpec.CpuAllocation))
			Expect(configSpec.MemoryAllocation).To(Equal(baseConfigSpec.MemoryAllocation))
			Expect(configSpec.Firmware).To(Equal(baseConfigSpec.Firmware))

			Expect(configSpec.DeviceChange).To(HaveLen(2))
			dSpec0 := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec0.Device.(*vimTypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			/* Removed until PlaceVmsXCluster() bugs get fixed.
			dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			_, ok = dSpec1.Device.(*vimTypes.VirtualVmxnet3)
			Expect(ok).To(BeTrue())
			*/
			dSpec2 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			_, ok = dSpec2.Device.(*vimTypes.VirtualDisk)
			Expect(ok).To(BeTrue())
		})
	})

	Context("When InstanceStorage is configured", func() {
		const storagePolicyID = "storage-id-42"

		BeforeEach(func() {
			pkgconfig.SetContext(vmCtx, func(config *pkgconfig.Config) {
				config.Features.InstanceStorage = true
			})

			builder.AddDummyInstanceStorageVolumeA2(vmCtx.VM)
			storageClassesToIDs[builder.DummyStorageClassName] = storagePolicyID
		})
		It("ConfigSpec contains expected InstanceStorage devices", func() {
			Expect(configSpec.DeviceChange).To(HaveLen(3))
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[1], 256, storagePolicyID)
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[2], 512, storagePolicyID)
		})
	})

	Context("Removes VirtualEthernetCards without a backing", func() {
		BeforeEach(func() {
			baseConfigSpec = &vimTypes.VirtualMachineConfigSpec{
				Name: "dummy-VM",
				DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
					&vimTypes.VirtualDeviceConfigSpec{
						Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
						Device:    &vimTypes.VirtualVmxnet3{},
					},
				},
			}
		})

		It("Returns expected ConfigSpec DeviceChanges", func() {
			// Just the dummy disk entry: vmxnet3 device should not be there.
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimTypes.VirtualDisk)
			Expect(ok).To(BeTrue())
		})
	})
})

var _ = Describe("ConfigSpecFromVMClassDevices", func() {

	var (
		vmClassSpec *vmopv1.VirtualMachineClassSpec
		configSpec  *vimTypes.VirtualMachineConfigSpec
	)

	Context("when Class specifies GPU/DDPIO in Hardware", func() {

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}

			vmClassSpec.Hardware.Devices.VGPUDevices = []vmopv1.VGPUDevice{{
				ProfileName: "createplacementspec-profile",
			}}

			vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices = []vmopv1.DynamicDirectPathIODevice{{
				VendorID:    20,
				DeviceID:    30,
				CustomLabel: "createplacementspec-label",
			}}
		})

		JustBeforeEach(func() {
			configSpec = virtualmachine.ConfigSpecFromVMClassDevices(vmClassSpec)
		})

		It("Returns expected ConfigSpec", func() {
			Expect(configSpec).ToNot(BeNil())
			Expect(configSpec.DeviceChange).To(HaveLen(2)) // One each for GPU an DDPIO above

			dSpec1 := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			dev1, ok := dSpec1.Device.(*vimTypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			pciDev1 := dev1.GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimTypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).To(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal(vmClassSpec.Hardware.Devices.VGPUDevices[0].ProfileName))

			dSpec2 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			dev2, ok2 := dSpec2.Device.(*vimTypes.VirtualPCIPassthrough)
			Expect(ok2).To(BeTrue())
			pciDev2 := dev2.GetVirtualDevice()
			pciBacking2, ok2 := pciDev2.Backing.(*vimTypes.VirtualPCIPassthroughDynamicBackingInfo)
			Expect(ok2).To(BeTrue())
			Expect(pciBacking2.AllowedDevice[0].DeviceId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].DeviceID))
			Expect(pciBacking2.AllowedDevice[0].VendorId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].VendorID))
			Expect(pciBacking2.CustomLabel).To(Equal(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].CustomLabel))
		})
	})
})

func assertInstanceStorageDeviceChange(
	deviceChange vimTypes.BaseVirtualDeviceConfigSpec,
	expectedSizeGB int,
	expectedStoragePolicyID string) {

	dc := deviceChange.GetVirtualDeviceConfigSpec()
	Expect(dc.Operation).To(Equal(vimTypes.VirtualDeviceConfigSpecOperationAdd))
	Expect(dc.FileOperation).To(Equal(vimTypes.VirtualDeviceConfigSpecFileOperationCreate))

	dev, ok := dc.Device.(*vimTypes.VirtualDisk)
	Expect(ok).To(BeTrue())
	Expect(dev.CapacityInBytes).To(BeEquivalentTo(expectedSizeGB * 1024 * 1024 * 1024))

	Expect(dc.Profile).To(HaveLen(1))
	profile, ok := dc.Profile[0].(*vimTypes.VirtualMachineDefinedProfileSpec)
	Expect(ok).To(BeTrue())
	Expect(profile.ProfileId).To(Equal(expectedStoragePolicyID))
}
