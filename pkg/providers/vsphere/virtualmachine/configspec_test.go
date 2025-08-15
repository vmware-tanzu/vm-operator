// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CreateConfigSpec", func() {
	const (
		vmName      = "dummy-vm"
		fakeGuestID = "fake-guest-id"
	)

	var (
		vm              *vmopv1.VirtualMachine
		vmCtx           pkgctx.VirtualMachineContext
		vmClassSpec     vmopv1.VirtualMachineClassSpec
		vmImageStatus   vmopv1.VirtualMachineImageStatus
		minCPUFreq      uint64
		configSpec      vimtypes.VirtualMachineConfigSpec
		classConfigSpec vimtypes.VirtualMachineConfigSpec
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
		vmClass := builder.DummyVirtualMachineClass("my-class")
		vmClassSpec = vmClass.Spec
		vmImageStatus = vmopv1.VirtualMachineImageStatus{Firmware: "efi"}
		minCPUFreq = 2500

		vm = builder.DummyVirtualMachine()
		vm.Name = vmName
		vm.Spec.InstanceUUID = uuid.NewString()
		vm.Spec.BiosUUID = uuid.NewString()
		// Explicitly set these in the tests.
		vm.Spec.Volumes = nil
		vm.Spec.MinHardwareVersion = 0

		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.Background(),
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
			classConfigSpec = vimtypes.VirtualMachineConfigSpec{}
		})

		When("Basic ConfigSpec assertions", func() {
			It("returns expected config spec", func() {
				Expect(configSpec).ToNot(BeNil())

				Expect(configSpec.Name).To(Equal(vmName))
				Expect(configSpec.Version).To(BeEmpty())
				Expect(configSpec.Annotation).ToNot(BeEmpty())
				Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
				Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
				Expect(configSpec.CpuAllocation.Shares).ToNot(BeNil())
				Expect(configSpec.CpuAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
				Expect(configSpec.CpuAllocation.Limit).To(HaveValue(BeEquivalentTo(5368709120000)))
				Expect(configSpec.CpuAllocation.Reservation).To(HaveValue(BeEquivalentTo(2684354560000)))
				Expect(configSpec.MemoryAllocation.Shares).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
				Expect(configSpec.MemoryAllocation.Limit).To(HaveValue(BeEquivalentTo(4096)))
				Expect(configSpec.MemoryAllocation.Reservation).To(HaveValue(BeEquivalentTo(2048)))
				Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
			})

			Context("VM Class has no requests/limits (best effort)", func() {
				BeforeEach(func() {
					vmClassSpec.Policies = vmopv1.VirtualMachineClassPolicies{}
				})

				It("returns expected config spec", func() {
					Expect(configSpec.CpuAllocation.Shares).ToNot(BeNil())
					Expect(configSpec.CpuAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
					Expect(configSpec.CpuAllocation.Limit).To(HaveValue(BeEquivalentTo(-1)))
					Expect(configSpec.CpuAllocation.Reservation).To(HaveValue(BeZero()))
					Expect(configSpec.MemoryAllocation.Shares).ToNot(BeNil())
					Expect(configSpec.MemoryAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
					Expect(configSpec.MemoryAllocation.Limit).To(HaveValue(BeEquivalentTo(-1)))
					Expect(configSpec.MemoryAllocation.Reservation).To(HaveValue(BeZero()))
				})
			})

			Context("VM Class has request but no limits", func() {
				BeforeEach(func() {
					vmClassSpec.Policies.Resources.Limits = vmopv1.VirtualMachineResourceSpec{}
				})

				It("returns expected config spec", func() {
					Expect(configSpec.CpuAllocation.Shares).ToNot(BeNil())
					Expect(configSpec.CpuAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
					Expect(configSpec.CpuAllocation.Limit).To(HaveValue(BeEquivalentTo(-1)))
					Expect(configSpec.CpuAllocation.Reservation).To(HaveValue(BeEquivalentTo(2684354560000)))
					Expect(configSpec.MemoryAllocation.Shares).ToNot(BeNil())
					Expect(configSpec.MemoryAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
					Expect(configSpec.MemoryAllocation.Limit).To(HaveValue(BeEquivalentTo(-1)))
					Expect(configSpec.MemoryAllocation.Reservation).To(HaveValue(BeEquivalentTo(2048)))
				})
			})
		})

		When("VM has no bios or instance uuid", func() {
			BeforeEach(func() {
				vm.Spec.InstanceUUID = ""
				vm.Spec.BiosUUID = ""
			})

			It("config spec has empty uuids", func() {
				Expect(configSpec.Uuid).To(BeEmpty())
				Expect(configSpec.InstanceUuid).To(BeEmpty())
			})
		})

		When("VM has bios and instance uuid", func() {
			It("config spec has expected uuids", func() {
				Expect(configSpec.Uuid).ToNot(BeEmpty())
				Expect(configSpec.Uuid).To(Equal(vm.Spec.BiosUUID))
				Expect(configSpec.InstanceUuid).To(Equal(vm.Spec.InstanceUUID))
			})
		})

		When("VM has min version", func() {
			BeforeEach(func() {
				vmImageStatus = vmopv1.VirtualMachineImageStatus{}
				vm.Spec.MinHardwareVersion = int32(vimtypes.VMX10)
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimtypes.VMX10.String()))
			})
		})

		When("VM has PVCs", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{pvcVolume}
				vm.Spec.MinHardwareVersion = int32(vimtypes.VMX10)
			})

			It("config spec has expected version for PVCs", func() {
				Expect(configSpec.Version).To(Equal(vimtypes.VMX15.String()))
			})
		})

		When("VM spec has guestID set", func() {
			BeforeEach(func() {
				vm.Spec.GuestID = fakeGuestID
			})

			It("config spec has the expected guestID set", func() {
				Expect(configSpec.GuestId).To(Equal(fakeGuestID))
			})
		})
	})

	Context("VM Class ConfigSpec", func() {
		BeforeEach(func() {
			classConfigSpec = vimtypes.VirtualMachineConfigSpec{
				Name:       "dont-use-this-dummy-VM",
				Annotation: "test-annotation",
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](42),
				},
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](42),
				},
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualE1000{
							VirtualEthernetCard: vimtypes.VirtualEthernetCard{
								VirtualDevice: vimtypes.VirtualDevice{
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
			Expect(configSpec.CpuAllocation.Shares).ToNot(BeNil())
			Expect(configSpec.CpuAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
			Expect(configSpec.CpuAllocation.Reservation).To(HaveValue(BeEquivalentTo(2684354560000)))
			Expect(configSpec.CpuAllocation.Limit).To(HaveValue(BeEquivalentTo(5368709120000)))
			Expect(configSpec.MemoryAllocation).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation.Shares).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
			Expect(configSpec.MemoryAllocation.Reservation).To(HaveValue(BeEquivalentTo(2048)))
			Expect(configSpec.MemoryAllocation.Limit).To(HaveValue(BeEquivalentTo(4096)))
			Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())
		})

		When("Image firmware is empty", func() {
			BeforeEach(func() {
				vmImageStatus = vmopv1.VirtualMachineImageStatus{}
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
				vmImageStatus.HardwareVersion = ptr.To(int32(vimtypes.VMX21))
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimtypes.VMX21.String()))
			})
		})

		When("VM Class config spec has devices", func() {

			Context("VM Class has vGPU", func() {
				BeforeEach(func() {
					classConfigSpec.DeviceChange = append(classConfigSpec.DeviceChange,
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "profile-from-configspec",
									},
								},
							},
						},
					)
				})

				It("config spec has expected version", func() {
					Expect(configSpec.Version).To(Equal(vimtypes.VMX17.String()))
				})

				Context("VM MinHardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion)
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
					})
				})
			})

			Context("VM Class has DDPIO", func() {
				BeforeEach(func() {
					classConfigSpec.DeviceChange = append(classConfigSpec.DeviceChange,
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
						vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion)
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
					})
				})
			})
		})

		When("VM Class config spec specifies version", func() {
			BeforeEach(func() {
				classConfigSpec.Version = vimtypes.MaxValidHardwareVersion.String()
			})

			It("config spec has expected version", func() {
				Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
			})

			When("VM and/or VM Image have version set", func() {
				Context("VM and VM Image are less than class", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion - 2)
						vmImageStatus.HardwareVersion = ptr.To(int32(vimtypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
					})
				})

				Context("VM MinHardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion)
						vmImageStatus.HardwareVersion = ptr.To(int32(vimtypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
					})
				})

				Context("VM Image HardwareVersion is greatest", func() {
					BeforeEach(func() {
						vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion - 3)
						vmImageStatus.HardwareVersion = ptr.To(int32(vimtypes.MaxValidHardwareVersion - 1))
					})

					It("config spec has expected version", func() {
						// When the ConfigSpec.Version is set the image is ignored so it won't
						// be the expected value.
						Expect(configSpec.Version).To(Equal(vimtypes.MaxValidHardwareVersion.String()))
					})
				})
			})
		})

		When("VM spec has guestID set", func() {
			BeforeEach(func() {
				vm.Spec.GuestID = fakeGuestID
			})

			It("config spec has the expected guestID set", func() {
				Expect(configSpec.GuestId).To(Equal(fakeGuestID))
			})
		})

		When("VM Class has reserved profile ID", func() {
			BeforeEach(func() {
				vmClassSpec.ReservedProfileID = "my-profile-id"
			})
			Specify("configSpec should have the expected ExtraConfig value", func() {
				profileID, _ := object.OptionValueList(
					configSpec.ExtraConfig).GetString(
					constants.ExtraConfigReservedProfileID)
				Expect(profileID).To(Equal(vmClassSpec.ReservedProfileID))

				namespacedName, _ := object.OptionValueList(
					configSpec.ExtraConfig).GetString(
					constants.ExtraConfigVMServiceNamespacedName)
				Expect(namespacedName).To(Equal(vm.NamespacedName()))
			})
		})

		When("VM Class has reservations", func() {
			BeforeEach(func() {
				vmClassSpec.Policies = vmopv1.VirtualMachineClassPolicies{}
				classConfigSpec.CpuAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](200),
				}
				classConfigSpec.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](200),
				}
			})
			Specify("configSpec should have the allocation defaults", func() {
				Expect(configSpec.CpuAllocation).ToNot(BeNil())
				Expect(configSpec.CpuAllocation.Reservation).ToNot(BeNil())
				Expect(configSpec.CpuAllocation.Limit).ToNot(BeNil())
				Expect(*configSpec.CpuAllocation.Reservation).To(Equal(int64(200)))
				Expect(*configSpec.CpuAllocation.Limit).To(Equal(int64(-1)))
				Expect(configSpec.CpuAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))

				Expect(configSpec.MemoryAllocation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Reservation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Limit).ToNot(BeNil())
				Expect(*configSpec.MemoryAllocation.Reservation).To(Equal(int64(200)))
				Expect(*configSpec.MemoryAllocation.Limit).To(Equal(int64(-1)))
				Expect(configSpec.MemoryAllocation.Shares).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation.Shares.Level).To(Equal(vimtypes.SharesLevelNormal))
			})
		})
	})
})

var _ = Describe("CreateConfigSpecForPlacement", func() {

	var (
		vmCtx               pkgctx.VirtualMachineContext
		storageClassesToIDs map[string]string
		baseConfigSpec      vimtypes.VirtualMachineConfigSpec
		configSpec          vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		baseConfigSpec = vimtypes.VirtualMachineConfigSpec{}
		storageClassesToIDs = map[string]string{}

		vm := builder.DummyVirtualMachine()
		vmCtx = pkgctx.VirtualMachineContext{
			Context: pkgcfg.NewContext(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		var err error
		configSpec, err = virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			baseConfigSpec,
			storageClassesToIDs)
		Expect(err).ToNot(HaveOccurred())
		Expect(configSpec).ToNot(BeNil())
	})

	Context("Returns expected ConfigSpec", func() {
		BeforeEach(func() {
			baseConfigSpec = vimtypes.VirtualMachineConfigSpec{
				Name:       "dummy-VM",
				Annotation: "test-annotation",
				NumCPUs:    42,
				MemoryMB:   4096,
				Firmware:   "secret-sauce",
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
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
						Device: &vimtypes.VirtualVmxnet3{
							VirtualVmxnet: vimtypes.VirtualVmxnet{
								VirtualEthernetCard: vimtypes.VirtualEthernetCard{
									VirtualDevice: vimtypes.VirtualDevice{
										Backing: &vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo{},
									},
								},
							},
						},
					},
				},
			}
		})

		It("Placement ConfigSpec contains expected fields set from class config spec", func() {
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal(baseConfigSpec.Annotation))
			Expect(configSpec.NumCPUs).To(Equal(baseConfigSpec.NumCPUs))
			Expect(configSpec.MemoryMB).To(Equal(baseConfigSpec.MemoryMB))
			Expect(configSpec.CpuAllocation).To(Equal(baseConfigSpec.CpuAllocation))
			Expect(configSpec.MemoryAllocation).To(Equal(baseConfigSpec.MemoryAllocation))
			Expect(configSpec.Firmware).To(Equal(baseConfigSpec.Firmware))

			Expect(configSpec.DeviceChange).To(HaveLen(5))

			dSpec0 := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec0.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			Expect(dSpec0.Device.GetVirtualDevice().Key).ToNot(BeZero())

			dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			_, ok = dSpec1.Device.(*vimtypes.VirtualVmxnet3)
			Expect(ok).To(BeTrue())

			dSpec2 := configSpec.DeviceChange[2].GetVirtualDeviceConfigSpec()
			disk, ok := dSpec2.Device.(*vimtypes.VirtualDisk)
			Expect(ok).To(BeTrue())
			Expect(disk.CapacityInBytes).To(BeEquivalentTo(1024 * 1024)) // Dummy disk size
			Expect(disk.UnitNumber).To(HaveValue(BeEquivalentTo(0)))

			dSpec3 := configSpec.DeviceChange[3].GetVirtualDeviceConfigSpec()
			_, ok = dSpec3.Device.(*vimtypes.VirtualPCIController)
			Expect(ok).To(BeTrue())

			dSpec4 := configSpec.DeviceChange[4].GetVirtualDeviceConfigSpec()
			_, ok = dSpec4.Device.(*vimtypes.ParaVirtualSCSIController)
			Expect(ok).To(BeTrue())
		})

		Context("VirtualDisk is already present", func() {
			BeforeEach(func() {
				baseConfigSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualDisk{
							CapacityInBytes: 42,
						},
					},
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
				}
			})

			It("Returns expected ConfigSpec.DeviceChanges", func() {
				Expect(configSpec.DeviceChange).To(HaveLen(4))

				dSpec0 := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
				disk, ok := dSpec0.Device.(*vimtypes.VirtualDisk)
				Expect(ok).To(BeTrue())
				Expect(disk.CapacityInBytes).To(BeEquivalentTo(42))
				Expect(disk.UnitNumber).To(HaveValue(BeEquivalentTo(0)))

				dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
				_, ok = dSpec1.Device.(*vimtypes.VirtualPCIPassthrough)
				Expect(ok).To(BeTrue())

				dSpec2 := configSpec.DeviceChange[2].GetVirtualDeviceConfigSpec()
				_, ok = dSpec2.Device.(*vimtypes.VirtualPCIController)
				Expect(ok).To(BeTrue())

				dSpec3 := configSpec.DeviceChange[3].GetVirtualDeviceConfigSpec()
				_, ok = dSpec3.Device.(*vimtypes.ParaVirtualSCSIController)
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("When InstanceStorage is configured", func() {
		const storagePolicyID = "storage-id-42"

		BeforeEach(func() {
			pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
				config.Features.InstanceStorage = true
			})

			builder.AddDummyInstanceStorageVolume(vmCtx.VM)
			storageClassesToIDs[builder.DummyStorageClassName] = storagePolicyID
		})
		It("ConfigSpec contains expected InstanceStorage devices", func() {
			Expect(configSpec.DeviceChange).To(HaveLen(5))
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[1], 1, 256, storagePolicyID)
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[2], 2, 512, storagePolicyID)
		})
	})
})

var _ = Describe("ConfigSpecFromVMClassDevices", func() {

	var (
		vmClassSpec *vmopv1.VirtualMachineClassSpec
		configSpec  vimtypes.VirtualMachineConfigSpec
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
			dev1, ok := dSpec1.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			pciDev1 := dev1.GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).To(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal(vmClassSpec.Hardware.Devices.VGPUDevices[0].ProfileName))

			dSpec2 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			dev2, ok2 := dSpec2.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok2).To(BeTrue())
			pciDev2 := dev2.GetVirtualDevice()
			pciBacking2, ok2 := pciDev2.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
			Expect(ok2).To(BeTrue())
			Expect(pciBacking2.AllowedDevice[0].DeviceId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].DeviceID))
			Expect(pciBacking2.AllowedDevice[0].VendorId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].VendorID))
			Expect(pciBacking2.CustomLabel).To(Equal(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].CustomLabel))
		})
	})
})

func assertInstanceStorageDeviceChange(
	deviceChange vimtypes.BaseVirtualDeviceConfigSpec,
	expectedUnitNumber int32,
	expectedSizeGB int,
	expectedStoragePolicyID string) {

	dc := deviceChange.GetVirtualDeviceConfigSpec()
	Expect(dc.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
	Expect(dc.FileOperation).To(Equal(vimtypes.VirtualDeviceConfigSpecFileOperationCreate))

	dev, ok := dc.Device.(*vimtypes.VirtualDisk)
	Expect(ok).To(BeTrue())
	Expect(dev.UnitNumber).ToNot(BeNil())
	Expect(*dev.UnitNumber).To(Equal(expectedUnitNumber))
	Expect(dev.CapacityInBytes).To(BeEquivalentTo(expectedSizeGB * 1024 * 1024 * 1024))

	Expect(dc.Profile).To(HaveLen(1))
	profile, ok := dc.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
	Expect(ok).To(BeTrue())
	Expect(profile.ProfileId).To(Equal(expectedStoragePolicyID))
}
