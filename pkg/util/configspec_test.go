// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"reflect"
	"time"

	"k8s.io/utils/pointer"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("DevicesFromConfigSpec", func() {
	var (
		devOut     []vimTypes.BaseVirtualDevice
		configSpec *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualPCIPassthrough{},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualSriovEthernetCard{},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Device: &vimTypes.VirtualVmxnet3{},
				},
			},
		}
	})

	JustBeforeEach(func() {
		devOut = util.DevicesFromConfigSpec(configSpec)
	})

	When("a ConfigSpec has a nil DeviceChange property", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = nil
		})
		It("will not panic", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	When("a ConfigSpec has an empty DeviceChange property", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimTypes.BaseVirtualDeviceConfigSpec{}
		})
		It("will not panic", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	When("a ConfigSpec has a VirtualPCIPassthrough device, SR-IOV NIC, and Vmxnet3 NIC", func() {
		It("returns the devices in insertion order", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimTypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimTypes.VirtualVmxnet3{}))
		})
	})

	When("a ConfigSpec has one or more DeviceChanges with a nil Device", func() {
		BeforeEach(func() {
			configSpec = &vimTypes.VirtualMachineConfigSpec{
				DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualPCIPassthrough{},
					},
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualSriovEthernetCard{},
					},
					&vimTypes.VirtualDeviceConfigSpec{
						Device: &vimTypes.VirtualVmxnet3{},
					},
					&vimTypes.VirtualDeviceConfigSpec{},
					&vimTypes.VirtualDeviceConfigSpec{},
				},
			}
		})

		It("will still return only the expected device(s)", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimTypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimTypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimTypes.VirtualVmxnet3{}))
		})
	})
})

var _ = Describe("ConfigSpec Util", func() {
	Context("MarshalConfigSpecToXML", func() {
		It("marshals and unmarshal to the same spec", func() {
			inputSpec := &vimTypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := util.MarshalConfigSpecToXML(inputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			var outputSpec vimTypes.VirtualMachineConfigSpec
			err = xml.Unmarshal(bytes, &outputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reflect.DeepEqual(inputSpec, &outputSpec)).To(BeTrue())
		})

		It("marshals spec correctly to expected base64 encoded XML", func() {
			inputSpec := &vimTypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := util.MarshalConfigSpecToXML(inputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(base64.StdEncoding.EncodeToString(bytes)).To(Equal("PG9iaiB4bWxuczp2aW0yNT0idXJuOnZpbTI1I" +
				"iB4bWxuczp4c2k9Imh0dHA6Ly93d3cudzMub3JnLzIwMDEvWE1MU2NoZW1hLWluc3RhbmNlIiB4c2k6dHlwZT0idmltMjU6Vmlyd" +
				"HVhbE1hY2hpbmVDb25maWdTcGVjIj48bmFtZT5kdW1teS1WTTwvbmFtZT48L29iaj4="))
		})
	})

	Context("MarshalConfigSpecFromAndToAndFromJSON", func() {
		It("marshals and unmarshal to the same spec", func() {

			f, err := os.Open("./testdata/virtualMachineConfigInfo.json")
			Expect(err).ToNot(HaveOccurred())
			Expect(f).ToNot(BeNil())
			defer func() {
				Expect(f.Close()).To(Succeed())
			}()

			dec1 := vimTypes.NewJSONDecoder(f)

			var ci1 vimTypes.VirtualMachineConfigInfo
			Expect(dec1.Decode(&ci1)).To(Succeed())

			cs1 := ci1.ToConfigSpec()
			cs2 := virtualMachineConfigInfoForTests.ToConfigSpec()

			Expect(cmp.Diff(cs1, cs2)).To(BeEmpty())

			var w bytes.Buffer
			enc1 := vimTypes.NewJSONEncoder(&w)
			Expect(enc1.Encode(cs1)).To(Succeed())

			dec2 := vimTypes.NewJSONDecoder(&w)

			var cs3 vimTypes.VirtualMachineConfigSpec
			Expect(dec2.Decode(&cs3)).To(Succeed())

			// This is a quirk of the ToConfigSpec() function. When converting
			// between a ConfigInfo and ConfigSpec, nillable fields are nil'd
			// if there is no data present.
			cs3.CpuFeatureMask = []vimTypes.VirtualMachineCpuIdInfoSpec{}

			Expect(cmp.Diff(cs1, cs3)).To(BeEmpty())
			Expect(cmp.Diff(cs2, cs3)).To(BeEmpty())
		})
	})
})

var _ = Describe("RemoveDevicesFromConfigSpec", func() {
	var (
		configSpec *vimTypes.VirtualMachineConfigSpec
		fn         func(vimTypes.BaseVirtualDevice) bool
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
			Name:         "dummy-VM",
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{},
		}
	})

	When("provided a config spec with a disk and func to remove VirtualDisk type", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualDisk{
						CapacityInBytes: 1024,
						VirtualDevice: vimTypes.VirtualDevice{
							Key: -42,
							Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: pointer.Bool(true),
							},
						},
					},
				},
			}

			fn = func(dev vimTypes.BaseVirtualDevice) bool {
				switch dev.(type) {
				case *vimTypes.VirtualDisk:
					return true
				default:
					return false
				}
			}
		})

		It("config spec deviceChanges empty", func() {
			util.RemoveDevicesFromConfigSpec(configSpec, fn)
			Expect(configSpec.DeviceChange).To(BeEmpty())
		})
	})
})

var _ = Describe("SanitizeVMClassConfigSpec", func() {
	oldVMClassAsConfigFSSEnabledFunc := lib.IsVMClassAsConfigFSSEnabled
	var (
		configSpec *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = &vimTypes.VirtualMachineConfigSpec{
			Name:         "dummy-VM",
			Annotation:   "test-annotation",
			Uuid:         "uuid",
			InstanceUuid: "instanceUUID",
			Files:        &vimTypes.VirtualMachineFileInfo{},
			VmProfile: []vimTypes.BaseVirtualMachineProfileSpec{
				&vimTypes.VirtualMachineDefinedProfileSpec{
					ProfileId: "dummy-id",
				},
			},
			DeviceChange: []vimTypes.BaseVirtualDeviceConfigSpec{
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualSATAController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualIDEController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualSCSIController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualNVMEController{
						VirtualController: vimTypes.VirtualController{},
					},
				},
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
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimTypes.VirtualDevice{
							Key: -42,
							Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: pointer.Bool(true),
							},
						},
					},
				},
				&vimTypes.VirtualDeviceConfigSpec{
					Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimTypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimTypes.VirtualDevice{
							Key: -32,
							Backing: &vimTypes.VirtualDiskRawDiskMappingVer1BackingInfo{
								LunUuid: "dummy-uuid",
							},
						},
					},
				},
			},
		}

		lib.IsVMClassAsConfigFSSEnabled = func() bool {
			return false
		}
	})

	AfterEach(func() {
		lib.IsVMClassAsConfigFSSEnabled = oldVMClassAsConfigFSSEnabledFunc
	})

	It("returns expected sanitized ConfigSpec", func() {
		util.SanitizeVMClassConfigSpec(configSpec)

		Expect(configSpec.Name).To(Equal("dummy-VM"))
		Expect(configSpec.Annotation).ToNot(BeEmpty())
		Expect(configSpec.Annotation).To(Equal("test-annotation"))
		Expect(configSpec.Uuid).To(BeEmpty())
		Expect(configSpec.InstanceUuid).To(BeEmpty())
		Expect(configSpec.Files).To(BeNil())
		Expect(configSpec.VmProfile).To(BeEmpty())

		Expect(configSpec.DeviceChange).To(HaveLen(1))
		dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		_, ok := dSpec.Device.(*vimTypes.VirtualE1000)
		Expect(ok).To(BeTrue())
	})

	When("VMClassAsConfig is enabled", func() {
		BeforeEach(func() {
			lib.IsVMClassAsConfigFSSEnabled = func() bool {
				return true
			}
		})
		It("returns expected sanitized ConfigSpec", func() {
			util.SanitizeVMClassConfigSpec(configSpec)

			Expect(configSpec.Name).To(Equal("dummy-VM"))
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal("test-annotation"))
			Expect(configSpec.Uuid).To(BeEmpty())
			Expect(configSpec.InstanceUuid).To(BeEmpty())
			Expect(configSpec.Files).To(BeNil())
			Expect(configSpec.VmProfile).To(BeEmpty())

			Expect(configSpec.DeviceChange).To(HaveLen(6))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimTypes.VirtualSATAController)
			Expect(ok).To(BeTrue())
			dSpec = configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			_, ok = dSpec.Device.(*vimTypes.VirtualIDEController)
			Expect(ok).To(BeTrue())
			dSpec = configSpec.DeviceChange[2].GetVirtualDeviceConfigSpec()
			_, ok = dSpec.Device.(*vimTypes.VirtualSCSIController)
			Expect(ok).To(BeTrue())
			dSpec = configSpec.DeviceChange[3].GetVirtualDeviceConfigSpec()
			_, ok = dSpec.Device.(*vimTypes.VirtualNVMEController)
			Expect(ok).To(BeTrue())
			dSpec = configSpec.DeviceChange[4].GetVirtualDeviceConfigSpec()
			_, ok = dSpec.Device.(*vimTypes.VirtualE1000)
			Expect(ok).To(BeTrue())
			dSpec = configSpec.DeviceChange[5].GetVirtualDeviceConfigSpec()
			_, ok = dSpec.Device.(*vimTypes.VirtualDisk)
			Expect(ok).To(BeTrue())
			dev := dSpec.Device.GetVirtualDevice()
			backing, ok := dev.Backing.(*vimTypes.VirtualDiskRawDiskMappingVer1BackingInfo)
			Expect(ok).To(BeTrue())
			Expect(backing.LunUuid).To(Equal("dummy-uuid"))
		})
	})
})

func mustParseTime(layout, value string) time.Time {
	t, err := time.Parse(layout, value)
	if err != nil {
		panic(err)
	}
	return t
}

func addrOfMustParseTime(layout, value string) *time.Time {
	t := mustParseTime(layout, value)
	return &t
}

func addrOfBool(v bool) *bool {
	return &v
}

func addrOfInt32(v int32) *int32 {
	return &v
}

func addrOfInt64(v int64) *int64 {
	return &v
}

var virtualMachineConfigInfoForTests vimTypes.VirtualMachineConfigInfo = vimTypes.VirtualMachineConfigInfo{
	ChangeVersion:         "2022-12-12T11:48:35.473645Z",
	Modified:              mustParseTime(time.RFC3339, "1970-01-01T00:00:00Z"),
	Name:                  "test",
	GuestFullName:         "VMware Photon OS (64-bit)",
	Version:               "vmx-20",
	Uuid:                  "422ca90b-853b-1101-3350-759f747730cc",
	CreateDate:            addrOfMustParseTime(time.RFC3339, "2022-12-12T11:47:24.685785Z"),
	InstanceUuid:          "502cc2a5-1f06-2890-6d70-ba2c55c5c2b7",
	NpivTemporaryDisabled: addrOfBool(true),
	LocationId:            "",
	Template:              false,
	GuestId:               "vmwarePhoton64Guest",
	AlternateGuestName:    "",
	Annotation:            "",
	Files: vimTypes.VirtualMachineFileInfo{
		VmPathName:        "[datastore1] test/test.vmx",
		SnapshotDirectory: "[datastore1] test/",
		SuspendDirectory:  "[datastore1] test/",
		LogDirectory:      "[datastore1] test/",
	},
	Tools: &vimTypes.ToolsConfigInfo{
		ToolsVersion:            0,
		AfterPowerOn:            addrOfBool(true),
		AfterResume:             addrOfBool(true),
		BeforeGuestStandby:      addrOfBool(true),
		BeforeGuestShutdown:     addrOfBool(true),
		BeforeGuestReboot:       nil,
		ToolsUpgradePolicy:      "manual",
		SyncTimeWithHostAllowed: addrOfBool(true),
		SyncTimeWithHost:        addrOfBool(false),
		LastInstallInfo: &vimTypes.ToolsConfigInfoToolsLastInstallInfo{
			Counter: 0,
		},
	},
	Flags: vimTypes.VirtualMachineFlagInfo{
		EnableLogging:            addrOfBool(true),
		UseToe:                   addrOfBool(false),
		RunWithDebugInfo:         addrOfBool(false),
		MonitorType:              "release",
		HtSharing:                "any",
		SnapshotDisabled:         addrOfBool(false),
		SnapshotLocked:           addrOfBool(false),
		DiskUuidEnabled:          addrOfBool(false),
		SnapshotPowerOffBehavior: "powerOff",
		RecordReplayEnabled:      addrOfBool(false),
		FaultToleranceType:       "unset",
		CbrcCacheEnabled:         addrOfBool(false),
		VvtdEnabled:              addrOfBool(false),
		VbsEnabled:               addrOfBool(false),
	},
	DefaultPowerOps: vimTypes.VirtualMachineDefaultPowerOpInfo{
		PowerOffType:        "soft",
		SuspendType:         "hard",
		ResetType:           "soft",
		DefaultPowerOffType: "soft",
		DefaultSuspendType:  "hard",
		DefaultResetType:    "soft",
		StandbyAction:       "checkpoint",
	},
	RebootPowerOff: addrOfBool(false),
	Hardware: vimTypes.VirtualHardware{
		NumCPU:              1,
		NumCoresPerSocket:   1,
		AutoCoresPerSocket:  addrOfBool(true),
		MemoryMB:            2048,
		VirtualICH7MPresent: addrOfBool(false),
		VirtualSMCPresent:   addrOfBool(false),
		Device: []vimTypes.BaseVirtualDevice{
			&vimTypes.VirtualIDEController{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 200,
						DeviceInfo: &vimTypes.Description{
							Label:   "IDE 0",
							Summary: "IDE 0",
						},
					},
					BusNumber: 0,
				},
			},
			&vimTypes.VirtualIDEController{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 201,
						DeviceInfo: &vimTypes.Description{
							Label:   "IDE 1",
							Summary: "IDE 1",
						},
					},
					BusNumber: 1,
				},
			},
			&vimTypes.VirtualPS2Controller{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 300,
						DeviceInfo: &vimTypes.Description{
							Label:   "PS2 controller 0",
							Summary: "PS2 controller 0",
						},
					},
					BusNumber: 0,
					Device:    []int32{600, 700},
				},
			},
			&vimTypes.VirtualPCIController{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 100,
						DeviceInfo: &vimTypes.Description{
							Label:   "PCI controller 0",
							Summary: "PCI controller 0",
						},
					},
					BusNumber: 0,
					Device:    []int32{500, 12000, 14000, 1000, 15000, 4000},
				},
			},
			&vimTypes.VirtualSIOController{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 400,
						DeviceInfo: &vimTypes.Description{
							Label:   "SIO controller 0",
							Summary: "SIO controller 0",
						},
					},
					BusNumber: 0,
				},
			},
			&vimTypes.VirtualKeyboard{
				VirtualDevice: vimTypes.VirtualDevice{
					Key: 600,
					DeviceInfo: &vimTypes.Description{
						Label:   "Keyboard",
						Summary: "Keyboard",
					},
					ControllerKey: 300,
					UnitNumber:    addrOfInt32(0),
				},
			},
			&vimTypes.VirtualPointingDevice{
				VirtualDevice: vimTypes.VirtualDevice{
					Key:        700,
					DeviceInfo: &vimTypes.Description{Label: "Pointing device", Summary: "Pointing device; Device"},
					Backing: &vimTypes.VirtualPointingDeviceDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimTypes.VirtualDeviceDeviceBackingInfo{
							UseAutoDetect: addrOfBool(false),
						},
						HostPointingDevice: "autodetect",
					},
					ControllerKey: 300,
					UnitNumber:    addrOfInt32(1),
				},
			},
			&vimTypes.VirtualMachineVideoCard{
				VirtualDevice: vimTypes.VirtualDevice{
					Key:           500,
					DeviceInfo:    &vimTypes.Description{Label: "Video card ", Summary: "Video card"},
					ControllerKey: 100,
					UnitNumber:    addrOfInt32(0),
				},
				VideoRamSizeInKB:       4096,
				NumDisplays:            1,
				UseAutoDetect:          addrOfBool(false),
				Enable3DSupport:        addrOfBool(false),
				Use3dRenderer:          "automatic",
				GraphicsMemorySizeInKB: 262144,
			},
			&vimTypes.VirtualMachineVMCIDevice{
				VirtualDevice: vimTypes.VirtualDevice{
					Key: 12000,
					DeviceInfo: &vimTypes.Description{
						Label: "VMCI device",
						Summary: "Device on the virtual machine PCI " +
							"bus that provides support for the " +
							"virtual machine communication interface",
					},
					ControllerKey: 100,
					UnitNumber:    addrOfInt32(17),
				},
				Id:                             -1,
				AllowUnrestrictedCommunication: addrOfBool(false),
				FilterEnable:                   addrOfBool(true),
			},
			&vimTypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimTypes.VirtualSCSIController{
					VirtualController: vimTypes.VirtualController{
						VirtualDevice: vimTypes.VirtualDevice{
							Key: 1000,
							DeviceInfo: &vimTypes.Description{
								Label:   "SCSI controller 0",
								Summary: "VMware paravirtual SCSI",
							},
							ControllerKey: 100,
							UnitNumber:    addrOfInt32(3),
						},
						Device: []int32{2000},
					},
					HotAddRemove:       addrOfBool(true),
					SharedBus:          "noSharing",
					ScsiCtlrUnitNumber: 7,
				},
			},
			&vimTypes.VirtualAHCIController{
				VirtualSATAController: vimTypes.VirtualSATAController{
					VirtualController: vimTypes.VirtualController{
						VirtualDevice: vimTypes.VirtualDevice{
							Key: 15000,
							DeviceInfo: &vimTypes.Description{
								Label:   "SATA controller 0",
								Summary: "AHCI",
							},
							ControllerKey: 100,
							UnitNumber:    addrOfInt32(24),
						},
						Device: []int32{16000},
					},
				},
			},
			&vimTypes.VirtualCdrom{
				VirtualDevice: vimTypes.VirtualDevice{
					Key: 16000,
					DeviceInfo: &vimTypes.Description{
						Label:   "CD/DVD drive 1",
						Summary: "Remote device",
					},
					Backing: &vimTypes.VirtualCdromRemotePassthroughBackingInfo{
						VirtualDeviceRemoteDeviceBackingInfo: vimTypes.VirtualDeviceRemoteDeviceBackingInfo{
							UseAutoDetect: addrOfBool(false),
						},
					},
					Connectable:   &vimTypes.VirtualDeviceConnectInfo{AllowGuestControl: true, Status: "untried"},
					ControllerKey: 15000,
					UnitNumber:    addrOfInt32(0),
				},
			},
			&vimTypes.VirtualDisk{
				VirtualDevice: vimTypes.VirtualDevice{
					Key: 2000,
					DeviceInfo: &vimTypes.Description{
						Label:   "Hard disk 1",
						Summary: "4,194,304 KB",
					},
					Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimTypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] test/test.vmdk",
							Datastore: &vimTypes.ManagedObjectReference{
								Type:  "Datastore",
								Value: "datastore-21",
							},
						},
						DiskMode:               "persistent",
						Split:                  addrOfBool(false),
						WriteThrough:           addrOfBool(false),
						ThinProvisioned:        addrOfBool(false),
						EagerlyScrub:           addrOfBool(false),
						Uuid:                   "6000C298-df15-fe89-ddcb-8ea33329595d",
						ContentId:              "e4e1a794c6307ce7906a3973fffffffe",
						ChangeId:               "",
						Parent:                 nil,
						DeltaDiskFormat:        "",
						DigestEnabled:          addrOfBool(false),
						DeltaGrainSize:         0,
						DeltaDiskFormatVariant: "",
						Sharing:                "sharingNone",
						KeyId:                  nil,
					},
					ControllerKey: 1000,
					UnitNumber:    addrOfInt32(0),
				},
				CapacityInKB:    4194304,
				CapacityInBytes: 4294967296,
				Shares:          &vimTypes.SharesInfo{Shares: 1000, Level: "normal"},
				StorageIOAllocation: &vimTypes.StorageIOAllocationInfo{
					Limit:       addrOfInt64(-1),
					Shares:      &vimTypes.SharesInfo{Shares: 1000, Level: "normal"},
					Reservation: addrOfInt32(0),
				},
				DiskObjectId:               "1-2000",
				NativeUnmanagedLinkedClone: addrOfBool(false),
			},
			&vimTypes.VirtualVmxnet3{
				VirtualVmxnet: vimTypes.VirtualVmxnet{
					VirtualEthernetCard: vimTypes.VirtualEthernetCard{
						VirtualDevice: vimTypes.VirtualDevice{
							Key: 4000,
							DeviceInfo: &vimTypes.Description{
								Label:   "Network adapter 1",
								Summary: "VM Network",
							},
							Backing: &vimTypes.VirtualEthernetCardNetworkBackingInfo{
								VirtualDeviceDeviceBackingInfo: vimTypes.VirtualDeviceDeviceBackingInfo{
									DeviceName:    "VM Network",
									UseAutoDetect: addrOfBool(false),
								},
								Network: &vimTypes.ManagedObjectReference{
									Type:  "Network",
									Value: "network-27",
								},
							},
							Connectable: &vimTypes.VirtualDeviceConnectInfo{
								MigrateConnect: "unset",
								StartConnected: true,
								Status:         "untried",
							},
							ControllerKey: 100,
							UnitNumber:    addrOfInt32(7),
						},
						AddressType:      "assigned",
						MacAddress:       "00:50:56:ac:4d:ed",
						WakeOnLanEnabled: addrOfBool(true),
						ResourceAllocation: &vimTypes.VirtualEthernetCardResourceAllocation{
							Reservation: addrOfInt64(0),
							Share: vimTypes.SharesInfo{
								Shares: 50,
								Level:  "normal",
							},
							Limit: addrOfInt64(-1),
						},
						UptCompatibilityEnabled: addrOfBool(true),
					},
				},
				Uptv2Enabled: addrOfBool(false),
			},
			&vimTypes.VirtualUSBXHCIController{
				VirtualController: vimTypes.VirtualController{
					VirtualDevice: vimTypes.VirtualDevice{
						Key: 14000,
						DeviceInfo: &vimTypes.Description{
							Label:   "USB xHCI controller ",
							Summary: "USB xHCI controller",
						},
						SlotInfo: &vimTypes.VirtualDevicePciBusSlotInfo{
							PciSlotNumber: -1,
						},
						ControllerKey: 100,
						UnitNumber:    addrOfInt32(23),
					},
				},

				AutoConnectDevices: addrOfBool(false),
			},
		},
		MotherboardLayout:   "i440bxHostBridge",
		SimultaneousThreads: 1,
	},
	CpuAllocation: &vimTypes.ResourceAllocationInfo{
		Reservation:           addrOfInt64(0),
		ExpandableReservation: addrOfBool(false),
		Limit:                 addrOfInt64(-1),
		Shares: &vimTypes.SharesInfo{
			Shares: 1000,
			Level:  vimTypes.SharesLevelNormal,
		},
	},
	MemoryAllocation: &vimTypes.ResourceAllocationInfo{
		Reservation:           addrOfInt64(0),
		ExpandableReservation: addrOfBool(false),
		Limit:                 addrOfInt64(-1),
		Shares: &vimTypes.SharesInfo{
			Shares: 20480,
			Level:  vimTypes.SharesLevelNormal,
		},
	},
	LatencySensitivity: &vimTypes.LatencySensitivity{
		Level: vimTypes.LatencySensitivitySensitivityLevelNormal,
	},
	MemoryHotAddEnabled: addrOfBool(false),
	CpuHotAddEnabled:    addrOfBool(false),
	CpuHotRemoveEnabled: addrOfBool(false),
	ExtraConfig: []vimTypes.BaseOptionValue{
		&vimTypes.OptionValue{Key: "nvram", Value: "test.nvram"},
		&vimTypes.OptionValue{Key: "svga.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge0.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge4.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge4.virtualDev", Value: "pcieRootPort"},
		&vimTypes.OptionValue{Key: "pciBridge4.functions", Value: "8"},
		&vimTypes.OptionValue{Key: "pciBridge5.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge5.virtualDev", Value: "pcieRootPort"},
		&vimTypes.OptionValue{Key: "pciBridge5.functions", Value: "8"},
		&vimTypes.OptionValue{Key: "pciBridge6.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge6.virtualDev", Value: "pcieRootPort"},
		&vimTypes.OptionValue{Key: "pciBridge6.functions", Value: "8"},
		&vimTypes.OptionValue{Key: "pciBridge7.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "pciBridge7.virtualDev", Value: "pcieRootPort"},
		&vimTypes.OptionValue{Key: "pciBridge7.functions", Value: "8"},
		&vimTypes.OptionValue{Key: "hpet0.present", Value: "TRUE"},
		&vimTypes.OptionValue{Key: "RemoteDisplay.maxConnections", Value: "-1"},
		&vimTypes.OptionValue{Key: "sched.cpu.latencySensitivity", Value: "normal"},
		&vimTypes.OptionValue{Key: "vmware.tools.internalversion", Value: "0"},
		&vimTypes.OptionValue{Key: "vmware.tools.requiredversion", Value: "12352"},
		&vimTypes.OptionValue{Key: "migrate.hostLogState", Value: "none"},
		&vimTypes.OptionValue{Key: "migrate.migrationId", Value: "0"},
		&vimTypes.OptionValue{Key: "migrate.hostLog", Value: "test-36f94569.hlog"},
		&vimTypes.OptionValue{
			Key:   "viv.moid",
			Value: "c5b34aa9-d962-4a74-b7d2-b83ec683ba1b:vm-28:lIgQ2t7v24n2nl3N7K3m6IHW2OoPF4CFrJd5N+Tdfio=",
		},
	},
	DatastoreUrl: []vimTypes.VirtualMachineConfigInfoDatastoreUrlPair{
		{
			Name: "datastore1",
			Url:  "/vmfs/volumes/63970ed8-4abddd2a-62d7-02003f49c37d",
		},
	},
	SwapPlacement: "inherit",
	BootOptions: &vimTypes.VirtualMachineBootOptions{
		EnterBIOSSetup:       addrOfBool(false),
		EfiSecureBootEnabled: addrOfBool(false),
		BootRetryEnabled:     addrOfBool(false),
		BootRetryDelay:       10000,
		NetworkBootProtocol:  "ipv4",
	},
	FtInfo:                       nil,
	RepConfig:                    nil,
	VAppConfig:                   nil,
	VAssertsEnabled:              addrOfBool(false),
	ChangeTrackingEnabled:        addrOfBool(false),
	Firmware:                     "bios",
	MaxMksConnections:            -1,
	GuestAutoLockEnabled:         addrOfBool(true),
	ManagedBy:                    nil,
	MemoryReservationLockedToMax: addrOfBool(false),
	InitialOverhead: &vimTypes.VirtualMachineConfigInfoOverheadInfo{
		InitialMemoryReservation: 214446080,
		InitialSwapReservation:   2541883392,
	},
	NestedHVEnabled: addrOfBool(false),
	VPMCEnabled:     addrOfBool(false),
	ScheduledHardwareUpgradeInfo: &vimTypes.ScheduledHardwareUpgradeInfo{
		UpgradePolicy:                  "never",
		ScheduledHardwareUpgradeStatus: "none",
	},
	ForkConfigInfo:         nil,
	VFlashCacheReservation: 0,
	VmxConfigChecksum: []uint8{
		0x69, 0xf7, 0xa7, 0x9e,
		0xd1, 0xc2, 0x21, 0x4b,
		0x6c, 0x20, 0x77, 0x0a,
		0x94, 0x94, 0x99, 0xee,
		0x17, 0x5d, 0xdd, 0xa3,
	},
	MessageBusTunnelEnabled: addrOfBool(false),
	GuestIntegrityInfo: &vimTypes.VirtualMachineGuestIntegrityInfo{
		Enabled: addrOfBool(false),
	},
	MigrateEncryption: "opportunistic",
	SgxInfo: &vimTypes.VirtualMachineSgxInfo{
		FlcMode:            "unlocked",
		RequireAttestation: addrOfBool(false),
	},
	ContentLibItemInfo:      nil,
	FtEncryptionMode:        "ftEncryptionOpportunistic",
	GuestMonitoringModeInfo: &vimTypes.VirtualMachineGuestMonitoringModeInfo{},
	SevEnabled:              addrOfBool(false),
	NumaInfo: &vimTypes.VirtualMachineVirtualNumaInfo{
		AutoCoresPerNumaNode:    addrOfBool(true),
		VnumaOnCpuHotaddExposed: addrOfBool(false),
	},
	PmemFailoverEnabled:          addrOfBool(false),
	VmxStatsCollectionEnabled:    addrOfBool(true),
	VmOpNotificationToAppEnabled: addrOfBool(false),
	VmOpNotificationTimeout:      -1,
	DeviceSwap: &vimTypes.VirtualMachineVirtualDeviceSwap{
		LsiToPvscsi: &vimTypes.VirtualMachineVirtualDeviceSwapDeviceSwapInfo{
			Enabled:    addrOfBool(true),
			Applicable: addrOfBool(false),
			Status:     "none",
		},
	},
	Pmem:         nil,
	DeviceGroups: &vimTypes.VirtualMachineVirtualDeviceGroups{},
}
