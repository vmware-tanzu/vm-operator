// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"reflect"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("DevicesFromConfigSpec", func() {
	var (
		devOut     []vimtypes.BaseVirtualDevice
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configSpec = &vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualSriovEthernetCard{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualVmxnet3{},
				},
			},
		}
	})

	JustBeforeEach(func() {
		devOut = pkgutil.DevicesFromConfigSpec(configSpec)
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
			configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{}
		})
		It("will not panic", func() {
			Expect(devOut).To(HaveLen(0))
		})
	})

	When("a ConfigSpec has a VirtualPCIPassthrough device, SR-IOV NIC, and Vmxnet3 NIC", func() {
		It("returns the devices in insertion order", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimtypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimtypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimtypes.VirtualVmxnet3{}))
		})
	})

	When("a ConfigSpec has one or more DeviceChanges with a nil Device", func() {
		BeforeEach(func() {
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{},
					&vimtypes.VirtualDeviceConfigSpec{
						Device: &vimtypes.VirtualPCIPassthrough{},
					},
					&vimtypes.VirtualDeviceConfigSpec{},
					&vimtypes.VirtualDeviceConfigSpec{
						Device: &vimtypes.VirtualSriovEthernetCard{},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Device: &vimtypes.VirtualVmxnet3{},
					},
					&vimtypes.VirtualDeviceConfigSpec{},
					&vimtypes.VirtualDeviceConfigSpec{},
				},
			}
		})

		It("will still return only the expected device(s)", func() {
			Expect(devOut).To(HaveLen(3))
			Expect(devOut[0]).To(BeEquivalentTo(&vimtypes.VirtualPCIPassthrough{}))
			Expect(devOut[1]).To(BeEquivalentTo(&vimtypes.VirtualSriovEthernetCard{}))
			Expect(devOut[2]).To(BeEquivalentTo(&vimtypes.VirtualVmxnet3{}))
		})
	})
})

var _ = Describe("ConfigSpec Util", func() {
	Context("MarshalConfigSpecToXML", func() {
		It("marshals and unmarshal to the same spec", func() {
			inputSpec := vimtypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := pkgutil.MarshalConfigSpecToXML(inputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			var outputSpec vimtypes.VirtualMachineConfigSpec
			err = xml.Unmarshal(bytes, &outputSpec)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reflect.DeepEqual(inputSpec, outputSpec)).To(BeTrue())
		})

		It("marshals spec correctly to expected base64 encoded XML", func() {
			inputSpec := vimtypes.VirtualMachineConfigSpec{Name: "dummy-VM"}
			bytes, err := pkgutil.MarshalConfigSpecToXML(inputSpec)
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

			dec1 := vimtypes.NewJSONDecoder(f)

			var ci1 vimtypes.VirtualMachineConfigInfo
			Expect(dec1.Decode(&ci1)).To(Succeed())

			cs1 := ci1.ToConfigSpec()
			cs2 := virtualMachineConfigInfoForTests.ToConfigSpec()

			Expect(cmp.Diff(cs1, cs2)).To(BeEmpty())

			var w bytes.Buffer
			enc1 := vimtypes.NewJSONEncoder(&w)
			Expect(enc1.Encode(cs1)).To(Succeed())

			dec2 := vimtypes.NewJSONDecoder(&w)

			var cs3 vimtypes.VirtualMachineConfigSpec
			Expect(dec2.Decode(&cs3)).To(Succeed())

			Expect(cmp.Diff(cs1, cs3)).To(BeEmpty())
			Expect(cmp.Diff(cs2, cs3)).To(BeEmpty())
		})
	})

	Context("EnsureMinHardwareVersionInConfigSpec", func() {
		When("minimum hardware version is unset", func() {
			It("does not change the existing value of the configSpec's version", func() {
				configSpec := &vimtypes.VirtualMachineConfigSpec{Version: "vmx-15"}
				pkgutil.EnsureMinHardwareVersionInConfigSpec(configSpec, 0)

				Expect(configSpec.Version).To(Equal("vmx-15"))
			})

			It("does not set the configSpec's version", func() {
				configSpec := &vimtypes.VirtualMachineConfigSpec{}
				pkgutil.EnsureMinHardwareVersionInConfigSpec(configSpec, 0)

				Expect(configSpec.Version).To(BeEmpty())
			})
		})

		It("overrides the hardware version if the existing version is lesser", func() {
			configSpec := &vimtypes.VirtualMachineConfigSpec{Version: "vmx-15"}
			pkgutil.EnsureMinHardwareVersionInConfigSpec(configSpec, 17)

			Expect(configSpec.Version).To(Equal("vmx-17"))
		})

		It("sets the hardware version if the existing version is unset", func() {
			configSpec := &vimtypes.VirtualMachineConfigSpec{}
			pkgutil.EnsureMinHardwareVersionInConfigSpec(configSpec, 16)

			Expect(configSpec.Version).To(Equal("vmx-16"))
		})

		It("overrides the hardware version if the existing version is set incorrectly", func() {
			configSpec := &vimtypes.VirtualMachineConfigSpec{Version: "foo"}
			pkgutil.EnsureMinHardwareVersionInConfigSpec(configSpec, 17)

			Expect(configSpec.Version).To(Equal("vmx-17"))
		})
	})
})

var _ = Describe("RemoveDevicesFromConfigSpec", func() {
	var (
		configSpec *vimtypes.VirtualMachineConfigSpec
		fn         func(vimtypes.BaseVirtualDevice) bool
	)

	BeforeEach(func() {
		configSpec = &vimtypes.VirtualMachineConfigSpec{
			Name:         "dummy-VM",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{},
		}
	})

	When("provided a config spec with a disk and func to remove VirtualDisk type", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 1024,
						VirtualDevice: vimtypes.VirtualDevice{
							Key: -42,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: ptr.To(true),
							},
						},
					},
				},
			}

			fn = func(dev vimtypes.BaseVirtualDevice) bool {
				switch dev.(type) {
				case *vimtypes.VirtualDisk:
					return true
				default:
					return false
				}
			}
		})

		It("config spec deviceChanges empty", func() {
			pkgutil.RemoveDevicesFromConfigSpec(configSpec, fn)
			Expect(configSpec.DeviceChange).To(BeEmpty())
		})
	})
})

var _ = Describe("SanitizeVMClassConfigSpec", func() {
	var (
		ctx        context.Context
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()

		configSpec = &vimtypes.VirtualMachineConfigSpec{
			Name:         "dummy-VM",
			Annotation:   "test-annotation",
			Uuid:         "uuid",
			GuestId:      "dummy-guestID",
			InstanceUuid: "instanceUUID",
			Files:        &vimtypes.VirtualMachineFileInfo{},
			VmProfile: []vimtypes.BaseVirtualMachineProfileSpec{
				&vimtypes.VirtualMachineDefinedProfileSpec{
					ProfileId: "dummy-id",
				},
			},
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{Key: "my-key", Value: "my-value"},
				&vimtypes.OptionValue{Key: constants.MMPowerOffVMExtraConfigKey, Value: "deprecated"},
			},
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualSATAController{
						VirtualController: vimtypes.VirtualController{},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{},
					},
				},
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
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimtypes.VirtualDevice{
							Key: -42,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								ThinProvisioned: ptr.To(true),
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 1024 * 1024,
						VirtualDevice: vimtypes.VirtualDevice{
							Key: -32,
							Backing: &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
								LunUuid: "dummy-uuid",
							},
						},
					},
				},
			},
		}
	})

	It("returns expected sanitized ConfigSpec", func() {
		pkgutil.SanitizeVMClassConfigSpec(ctx, configSpec)

		Expect(configSpec.Name).To(Equal("dummy-VM"))
		Expect(configSpec.Annotation).ToNot(BeEmpty())
		Expect(configSpec.Annotation).To(Equal("test-annotation"))
		Expect(configSpec.Uuid).To(BeEmpty())
		Expect(configSpec.InstanceUuid).To(BeEmpty())
		Expect(configSpec.GuestId).To(BeEmpty())
		Expect(configSpec.Files).To(BeNil())
		Expect(configSpec.VmProfile).To(BeEmpty())

		ecMap := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
		Expect(ecMap).To(HaveKeyWithValue("my-key", "my-value"))
		Expect(ecMap).ToNot(HaveKey(constants.MMPowerOffVMExtraConfigKey))

		Expect(configSpec.DeviceChange).To(HaveLen(6))
		dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
		_, ok := dSpec.Device.(*vimtypes.VirtualSATAController)
		Expect(ok).To(BeTrue())
		dSpec = configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
		_, ok = dSpec.Device.(*vimtypes.VirtualIDEController)
		Expect(ok).To(BeTrue())
		dSpec = configSpec.DeviceChange[2].GetVirtualDeviceConfigSpec()
		_, ok = dSpec.Device.(*vimtypes.VirtualSCSIController)
		Expect(ok).To(BeTrue())
		dSpec = configSpec.DeviceChange[3].GetVirtualDeviceConfigSpec()
		_, ok = dSpec.Device.(*vimtypes.VirtualNVMEController)
		Expect(ok).To(BeTrue())
		dSpec = configSpec.DeviceChange[4].GetVirtualDeviceConfigSpec()
		_, ok = dSpec.Device.(*vimtypes.VirtualE1000)
		Expect(ok).To(BeTrue())
		dSpec = configSpec.DeviceChange[5].GetVirtualDeviceConfigSpec()
		_, ok = dSpec.Device.(*vimtypes.VirtualDisk)
		Expect(ok).To(BeTrue())
		dev := dSpec.Device.GetVirtualDevice()
		backing, ok := dev.Backing.(*vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		Expect(ok).To(BeTrue())
		Expect(backing.LunUuid).To(Equal("dummy-uuid"))
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

var virtualMachineConfigInfoForTests vimtypes.VirtualMachineConfigInfo = vimtypes.VirtualMachineConfigInfo{
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
	Files: vimtypes.VirtualMachineFileInfo{
		VmPathName:        "[datastore1] test/test.vmx",
		SnapshotDirectory: "[datastore1] test/",
		SuspendDirectory:  "[datastore1] test/",
		LogDirectory:      "[datastore1] test/",
	},
	Tools: &vimtypes.ToolsConfigInfo{
		ToolsVersion:            0,
		AfterPowerOn:            addrOfBool(true),
		AfterResume:             addrOfBool(true),
		BeforeGuestStandby:      addrOfBool(true),
		BeforeGuestShutdown:     addrOfBool(true),
		BeforeGuestReboot:       nil,
		ToolsUpgradePolicy:      "manual",
		SyncTimeWithHostAllowed: addrOfBool(true),
		SyncTimeWithHost:        addrOfBool(false),
		LastInstallInfo: &vimtypes.ToolsConfigInfoToolsLastInstallInfo{
			Counter: 0,
		},
	},
	Flags: vimtypes.VirtualMachineFlagInfo{
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
	DefaultPowerOps: vimtypes.VirtualMachineDefaultPowerOpInfo{
		PowerOffType:        "soft",
		SuspendType:         "hard",
		ResetType:           "soft",
		DefaultPowerOffType: "soft",
		DefaultSuspendType:  "hard",
		DefaultResetType:    "soft",
		StandbyAction:       "checkpoint",
	},
	RebootPowerOff: addrOfBool(false),
	Hardware: vimtypes.VirtualHardware{
		NumCPU:              1,
		NumCoresPerSocket:   ptr.To[int32](1),
		AutoCoresPerSocket:  addrOfBool(true),
		MemoryMB:            2048,
		VirtualICH7MPresent: addrOfBool(false),
		VirtualSMCPresent:   addrOfBool(false),
		Device: []vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualIDEController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 200,
						DeviceInfo: &vimtypes.Description{
							Label:   "IDE 0",
							Summary: "IDE 0",
						},
					},
					BusNumber: 0,
				},
			},
			&vimtypes.VirtualIDEController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 201,
						DeviceInfo: &vimtypes.Description{
							Label:   "IDE 1",
							Summary: "IDE 1",
						},
					},
					BusNumber: 1,
				},
			},
			&vimtypes.VirtualPS2Controller{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 300,
						DeviceInfo: &vimtypes.Description{
							Label:   "PS2 controller 0",
							Summary: "PS2 controller 0",
						},
					},
					BusNumber: 0,
					Device:    []int32{600, 700},
				},
			},
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
						DeviceInfo: &vimtypes.Description{
							Label:   "PCI controller 0",
							Summary: "PCI controller 0",
						},
					},
					BusNumber: 0,
					Device:    []int32{500, 12000, 14000, 1000, 15000, 4000},
				},
			},
			&vimtypes.VirtualSIOController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 400,
						DeviceInfo: &vimtypes.Description{
							Label:   "SIO controller 0",
							Summary: "SIO controller 0",
						},
					},
					BusNumber: 0,
				},
			},
			&vimtypes.VirtualKeyboard{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: 600,
					DeviceInfo: &vimtypes.Description{
						Label:   "Keyboard",
						Summary: "Keyboard",
					},
					ControllerKey: 300,
					UnitNumber:    addrOfInt32(0),
				},
			},
			&vimtypes.VirtualPointingDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Key:        700,
					DeviceInfo: &vimtypes.Description{Label: "Pointing device", Summary: "Pointing device; Device"},
					Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							UseAutoDetect: addrOfBool(false),
						},
						HostPointingDevice: "autodetect",
					},
					ControllerKey: 300,
					UnitNumber:    addrOfInt32(1),
				},
			},
			&vimtypes.VirtualMachineVideoCard{
				VirtualDevice: vimtypes.VirtualDevice{
					Key:           500,
					DeviceInfo:    &vimtypes.Description{Label: "Video card ", Summary: "Video card"},
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
			&vimtypes.VirtualMachineVMCIDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: 12000,
					DeviceInfo: &vimtypes.Description{
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
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 1000,
							DeviceInfo: &vimtypes.Description{
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
			&vimtypes.VirtualAHCIController{
				VirtualSATAController: vimtypes.VirtualSATAController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 15000,
							DeviceInfo: &vimtypes.Description{
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
			&vimtypes.VirtualCdrom{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: 16000,
					DeviceInfo: &vimtypes.Description{
						Label:   "CD/DVD drive 1",
						Summary: "Remote device",
					},
					Backing: &vimtypes.VirtualCdromRemotePassthroughBackingInfo{
						VirtualDeviceRemoteDeviceBackingInfo: vimtypes.VirtualDeviceRemoteDeviceBackingInfo{
							UseAutoDetect: addrOfBool(false),
						},
					},
					Connectable:   &vimtypes.VirtualDeviceConnectInfo{AllowGuestControl: true, Status: "untried"},
					ControllerKey: 15000,
					UnitNumber:    addrOfInt32(0),
				},
			},
			&vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: 2000,
					DeviceInfo: &vimtypes.Description{
						Label:   "Hard disk 1",
						Summary: "4,194,304 KB",
					},
					Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "[datastore1] test/test.vmdk",
							Datastore: &vimtypes.ManagedObjectReference{
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
				Shares:          &vimtypes.SharesInfo{Shares: 1000, Level: "normal"},
				StorageIOAllocation: &vimtypes.StorageIOAllocationInfo{
					Limit:       addrOfInt64(-1),
					Shares:      &vimtypes.SharesInfo{Shares: 1000, Level: "normal"},
					Reservation: addrOfInt32(0),
				},
				DiskObjectId:               "1-2000",
				NativeUnmanagedLinkedClone: addrOfBool(false),
			},
			&vimtypes.VirtualVmxnet3{
				VirtualVmxnet: vimtypes.VirtualVmxnet{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 4000,
							DeviceInfo: &vimtypes.Description{
								Label:   "Network adapter 1",
								Summary: "VM Network",
							},
							Backing: &vimtypes.VirtualEthernetCardNetworkBackingInfo{
								VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
									DeviceName:    "VM Network",
									UseAutoDetect: addrOfBool(false),
								},
								Network: &vimtypes.ManagedObjectReference{
									Type:  "Network",
									Value: "network-27",
								},
							},
							Connectable: &vimtypes.VirtualDeviceConnectInfo{
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
						ResourceAllocation: &vimtypes.VirtualEthernetCardResourceAllocation{
							Reservation: addrOfInt64(0),
							Share: vimtypes.SharesInfo{
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
			&vimtypes.VirtualUSBXHCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 14000,
						DeviceInfo: &vimtypes.Description{
							Label:   "USB xHCI controller ",
							Summary: "USB xHCI controller",
						},
						SlotInfo: &vimtypes.VirtualDevicePciBusSlotInfo{
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
	CpuAllocation: &vimtypes.ResourceAllocationInfo{
		Reservation:           addrOfInt64(0),
		ExpandableReservation: addrOfBool(false),
		Limit:                 addrOfInt64(-1),
		Shares: &vimtypes.SharesInfo{
			Shares: 1000,
			Level:  vimtypes.SharesLevelNormal,
		},
	},
	MemoryAllocation: &vimtypes.ResourceAllocationInfo{
		Reservation:           addrOfInt64(0),
		ExpandableReservation: addrOfBool(false),
		Limit:                 addrOfInt64(-1),
		Shares: &vimtypes.SharesInfo{
			Shares: 20480,
			Level:  vimtypes.SharesLevelNormal,
		},
	},
	LatencySensitivity: &vimtypes.LatencySensitivity{
		Level: vimtypes.LatencySensitivitySensitivityLevelNormal,
	},
	MemoryHotAddEnabled: addrOfBool(false),
	CpuHotAddEnabled:    addrOfBool(false),
	CpuHotRemoveEnabled: addrOfBool(false),
	ExtraConfig: []vimtypes.BaseOptionValue{
		&vimtypes.OptionValue{Key: "nvram", Value: "test.nvram"},
		&vimtypes.OptionValue{Key: "svga.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge0.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge4.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge4.virtualDev", Value: "pcieRootPort"},
		&vimtypes.OptionValue{Key: "pciBridge4.functions", Value: "8"},
		&vimtypes.OptionValue{Key: "pciBridge5.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge5.virtualDev", Value: "pcieRootPort"},
		&vimtypes.OptionValue{Key: "pciBridge5.functions", Value: "8"},
		&vimtypes.OptionValue{Key: "pciBridge6.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge6.virtualDev", Value: "pcieRootPort"},
		&vimtypes.OptionValue{Key: "pciBridge6.functions", Value: "8"},
		&vimtypes.OptionValue{Key: "pciBridge7.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "pciBridge7.virtualDev", Value: "pcieRootPort"},
		&vimtypes.OptionValue{Key: "pciBridge7.functions", Value: "8"},
		&vimtypes.OptionValue{Key: "hpet0.present", Value: "TRUE"},
		&vimtypes.OptionValue{Key: "RemoteDisplay.maxConnections", Value: "-1"},
		&vimtypes.OptionValue{Key: "sched.cpu.latencySensitivity", Value: "normal"},
		&vimtypes.OptionValue{Key: "vmware.tools.internalversion", Value: "0"},
		&vimtypes.OptionValue{Key: "vmware.tools.requiredversion", Value: "12352"},
		&vimtypes.OptionValue{Key: "migrate.hostLogState", Value: "none"},
		&vimtypes.OptionValue{Key: "migrate.migrationId", Value: "0"},
		&vimtypes.OptionValue{Key: "migrate.hostLog", Value: "test-36f94569.hlog"},
		&vimtypes.OptionValue{
			Key:   "viv.moid",
			Value: "c5b34aa9-d962-4a74-b7d2-b83ec683ba1b:vm-28:lIgQ2t7v24n2nl3N7K3m6IHW2OoPF4CFrJd5N+Tdfio=",
		},
	},
	DatastoreUrl: []vimtypes.VirtualMachineConfigInfoDatastoreUrlPair{
		{
			Name: "datastore1",
			Url:  "/vmfs/volumes/63970ed8-4abddd2a-62d7-02003f49c37d",
		},
	},
	SwapPlacement: "inherit",
	BootOptions: &vimtypes.VirtualMachineBootOptions{
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
	InitialOverhead: &vimtypes.VirtualMachineConfigInfoOverheadInfo{
		InitialMemoryReservation: 214446080,
		InitialSwapReservation:   2541883392,
	},
	NestedHVEnabled: addrOfBool(false),
	VPMCEnabled:     addrOfBool(false),
	ScheduledHardwareUpgradeInfo: &vimtypes.ScheduledHardwareUpgradeInfo{
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
	GuestIntegrityInfo: &vimtypes.VirtualMachineGuestIntegrityInfo{
		Enabled: addrOfBool(false),
	},
	MigrateEncryption: "opportunistic",
	SgxInfo: &vimtypes.VirtualMachineSgxInfo{
		FlcMode:            "unlocked",
		RequireAttestation: addrOfBool(false),
	},
	ContentLibItemInfo:      nil,
	FtEncryptionMode:        "ftEncryptionOpportunistic",
	GuestMonitoringModeInfo: &vimtypes.VirtualMachineGuestMonitoringModeInfo{},
	SevEnabled:              addrOfBool(false),
	NumaInfo: &vimtypes.VirtualMachineVirtualNumaInfo{
		AutoCoresPerNumaNode:    addrOfBool(true),
		VnumaOnCpuHotaddExposed: addrOfBool(false),
	},
	PmemFailoverEnabled:          addrOfBool(false),
	VmxStatsCollectionEnabled:    addrOfBool(true),
	VmOpNotificationToAppEnabled: addrOfBool(false),
	VmOpNotificationTimeout:      -1,
	DeviceSwap: &vimtypes.VirtualMachineVirtualDeviceSwap{
		LsiToPvscsi: &vimtypes.VirtualMachineVirtualDeviceSwapDeviceSwapInfo{
			Enabled:    addrOfBool(true),
			Applicable: addrOfBool(false),
			Status:     "none",
		},
	},
	Pmem:         nil,
	DeviceGroups: &vimtypes.VirtualMachineVirtualDeviceGroups{},
}

var _ = DescribeTable(
	"SafeConfigSpecToString",
	func(in *vimtypes.VirtualMachineConfigSpec, expected string) {
		Expect(pkgutil.SafeConfigSpecToString(in)).To(Equal(expected))
	},
	Entry(
		"nil ConfigSpec",
		nil,
		`null`,
	),
	Entry(
		"empty ConfigSpec",
		&vimtypes.VirtualMachineConfigSpec{},
		`{"_typeName":"VirtualMachineConfigSpec"}`,
	),
	Entry(
		"w interface field set to nil pointer",
		&vimtypes.VirtualMachineConfigSpec{
			VAppConfig: (*vimtypes.VmConfigSpec)(nil),
		},
		`{"_typeName":"VirtualMachineConfigSpec","vAppConfig":null}`,
	),
)

var _ = DescribeTable(
	"DatastoreNameFromStorageURI",
	func(in, expected string) {
		Expect(pkgutil.DatastoreNameFromStorageURI(in)).To(Equal(expected))
	},
	Entry(
		"empty",
		"",
		"",
	),
	Entry(
		"empty",
		"invalid",
		"",
	),
	Entry(
		"just the datastore",
		"[my-datastore-1]",
		"my-datastore-1",
	),
	Entry(
		"a full path",
		"[my-datastore-1] my-vm/my-vm.vmx",
		"my-datastore-1",
	),
)

var _ = DescribeTable(
	"CopyStorageControllersAndDisks",
	func(
		src, dst vimtypes.VirtualMachineConfigSpec,
		storagePolicy string,
		expected vimtypes.VirtualMachineConfigSpec) {

		pkgutil.CopyStorageControllersAndDisks(&dst, src, storagePolicy)

		Expect(reflect.DeepEqual(expected, dst)).To(BeTrue(), cmp.Diff(expected, dst))
	},
	Entry(
		"empty",
		vimtypes.VirtualMachineConfigSpec{},
		vimtypes.VirtualMachineConfigSpec{},
		"",
		vimtypes.VirtualMachineConfigSpec{},
	),
	Entry(
		"src is empty",
		vimtypes.VirtualMachineConfigSpec{},
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
		"",
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
	),
	Entry(
		"src has a disk with no controller",
		vimtypes.VirtualMachineConfigSpec{
			Name: "hello",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
		"fake-storage-policy",
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
					Profile: []vimtypes.BaseVirtualMachineProfileSpec{
						&vimtypes.VirtualMachineDefinedProfileSpec{
							ProfileId: "fake-storage-policy",
						},
					},
				},
			},
		},
	),

	Entry(
		"src has a disk with controller",
		vimtypes.VirtualMachineConfigSpec{
			Name: "hello",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
		"fake-storage-policy",
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
					Profile: []vimtypes.BaseVirtualMachineProfileSpec{
						&vimtypes.VirtualMachineDefinedProfileSpec{
							ProfileId: "fake-storage-policy",
						},
					},
				},
			},
		},
	),

	Entry(
		"all supported controllers",
		vimtypes.VirtualMachineConfigSpec{
			Name: "hello",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -101,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -102,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -103,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -104,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -105,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -106,
							},
						},
					},
				},
			},
		},
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
		"fake-storage-policy",
		vimtypes.VirtualMachineConfigSpec{
			Name: "world",
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
							},
						},
					},
				},
			},
		},
	),
)

var _ = Describe("SafeDeviceChangesToString", func() {
	When("deviceChanges is empty", func() {
		It("returns empty array string", func() {
			result := pkgutil.SafeDeviceChangesToString([]vimtypes.BaseVirtualDeviceConfigSpec{})
			Expect(result).To(Equal("[]"))
		})
	})

	When("deviceChanges is nil", func() {
		It("returns empty array string", func() {
			result := pkgutil.SafeDeviceChangesToString(nil)
			Expect(result).To(Equal("[]"))
		})
	})

	When("deviceChanges has controllers", func() {
		It("returns string representation of device changes", func() {
			deviceChanges := []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -2,
							},
							BusNumber: 0,
						},
					},
				},
			}
			result := pkgutil.SafeDeviceChangesToString(deviceChanges)
			Expect(result).To(Equal(`{"_typeName":"VirtualMachineConfigSpec","deviceChange":[{"_typeName":"VirtualDeviceConfigSpec","operation":"add","device":{"_typeName":"ParaVirtualSCSIController","key":-1,"busNumber":0,"sharedBus":"noSharing"}},{"_typeName":"VirtualDeviceConfigSpec","operation":"add","device":{"_typeName":"VirtualIDEController","key":-2,"busNumber":0}}]}`))
		})
	})
})

var _ = Describe("MergeStorageControllersAndDisks", func() {
	var (
		dst *vimtypes.VirtualMachineConfigSpec
		src vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		dst = &vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{},
		}
		src = vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{},
		}
	})

	When("both src and dst are empty", func() {
		It("succeeds with no changes", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")
			Expect(dst.DeviceChange).To(BeEmpty())
		})
	})

	When("src has controllers and disks", func() {
		BeforeEach(func() {
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			}
		})

		It("copies controllers and disks to dst", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")
			Expect(dst.DeviceChange).To(HaveLen(2))
		})

		It("applies storage policy to disks", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "test-policy-id")
			Expect(err).ToNot(HaveOccurred())
			Expect(dst.DeviceChange).To(HaveLen(2))

			diskSpec := dst.DeviceChange[1].GetVirtualDeviceConfigSpec()
			Expect(diskSpec).ToNot(BeNil())
			Expect(diskSpec.Profile).To(HaveLen(1))
			profileSpec := diskSpec.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
			Expect(profileSpec.ProfileId).To(Equal("test-policy-id"))
		})
	})

	When("dst has a controller with same bus number and type as src", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			}
		})

		It("replaces dst controller with src controller", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")
			// Should have 2 items: controller and disk from src
			Expect(dst.DeviceChange).To(HaveLen(2))
			// Find the controller
			var controllerSpec *vimtypes.VirtualDeviceConfigSpec
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if _, ok := spec.Device.(vimtypes.BaseVirtualSCSIController); ok {
						controllerSpec = spec
						break
					}
				}
			}
			Expect(controllerSpec).ToNot(BeNil())
			Expect(controllerSpec.Device.GetVirtualDevice().Key).To(Equal(int32(-100)))
		})
	})

	When("dst has SCSI controller with different SharedBus than src", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "physicalSharing",
						},
					},
				},
			}
		})

		It("returns error for SharedBus mismatch", func() {
			_, _, err := pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("SCSI controller at bus 0: sharing mode conflict (\"noSharing\" vs \"physicalSharing\")"))
		})
	})

	When("dst has SCSI controller with different subtype than src", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("returns error for type conflict", func() {
			_, _, err := pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("SCSI controller at bus 0: type conflict (\"ParaVirtual\" vs \"BusLogic\")"))
		})
	})

	When("dst has NVME controller with different SharedBus than src", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -1,
							},
							BusNumber: 0,
						},
						SharedBus: "noSharing",
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -100,
							},
							BusNumber: 0,
						},
						SharedBus: "physicalSharing",
					},
				},
			}
		})

		It("returns error for SharedBus mismatch", func() {
			_, _, err := pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("NVME controller at bus 0: sharing mode conflict (\"noSharing\" vs \"physicalSharing\")"))
		})
	})

	When("dst has disk with controller key that needs remapping", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
								ThinProvisioned:              ptr.To(true),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("remaps disk controller key to src controller key", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			// Find the disk in dst
			var disk *vimtypes.VirtualDisk
			var diskSpec *vimtypes.VirtualDeviceConfigSpec
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if d, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
						disk = d
						diskSpec = spec
						break
					}
				}
			}
			Expect(disk).ToNot(BeNil())
			Expect(disk.ControllerKey).To(Equal(int32(-100)))
			Expect(diskSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			Expect(diskSpec.FileOperation).To(Equal(vimtypes.VirtualDeviceConfigSpecFileOperationCreate))
		})
	})

	When("src has all controller types", func() {
		BeforeEach(func() {
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -101,
								},
								BusNumber: 0,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -101,
							Key:           -201,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 20 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -102,
							},
							BusNumber: 0,
						},
						SharedBus: "noSharing",
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -102,
							Key:           -202,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 30 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualIDEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -103,
							},
							BusNumber: 0,
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -103,
							Key:           -203,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 40 * 1024 * 1024 * 1024,
					},
				},
			}
		})

		It("copies all controller types", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")
			// Should have 8 items: 4 controllers + 4 disks
			Expect(dst.DeviceChange).To(HaveLen(8))
			// Verify all controller types are present
			var controllers []vimtypes.BaseVirtualDevice
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					switch spec.Device.(type) {
					case vimtypes.BaseVirtualSCSIController,
						vimtypes.BaseVirtualSATAController,
						*vimtypes.VirtualIDEController,
						*vimtypes.VirtualNVMEController:
						controllers = append(controllers, spec.Device)
					}
				}
			}
			Expect(controllers).To(HaveLen(4))
		})
	})

	When("dst has multiple disks on different controllers that need remapping", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -2,
								},
								BusNumber: 1,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -2,
							Key:           -201,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 20 * 1024 * 1024 * 1024,
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -200,
								},
								BusNumber: 1,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("remaps all disk controller keys correctly", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			var disks []*vimtypes.VirtualDisk
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if d, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
						disks = append(disks, d)
					}
				}
			}
			Expect(disks).To(HaveLen(2))
			// First disk should be remapped to -100 (bus 0 controller)
			Expect(disks[0].ControllerKey).To(Equal(int32(-100)))
			// Second disk should be remapped to -200 (bus 1 controller)
			Expect(disks[1].ControllerKey).To(Equal(int32(-200)))
		})
	})

	When("dst has disk with non-Add operation", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationEdit,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("does not remap disk controller key for non-Add operations", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			var disk *vimtypes.VirtualDisk
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if d, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
						disk = d
						break
					}
				}
			}
			Expect(disk).ToNot(BeNil())
			// ControllerKey should remain -1 because operation is Edit, not Add
			Expect(disk.ControllerKey).To(Equal(int32(-1)))
		})
	})

	When("dst has controllers not present in src and src has no disks", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -2,
								},
								BusNumber: 1,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("preserves controllers in dst that are not duplicated in src, even when src has no disks", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			var controllers []vimtypes.BaseVirtualSCSIController
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if ctrl, ok := spec.Device.(vimtypes.BaseVirtualSCSIController); ok {
						controllers = append(controllers, ctrl)
					}
				}
			}
			// Bus 0 controller from dst is removed (duplicated in src).
			// Bus 0 controller from src is added then removed (no disks in src).
			// Bus 1 controller from dst is preserved (not duplicated in src).
			Expect(controllers).To(HaveLen(1))
			ctrl := controllers[0]
			Expect(ctrl.GetVirtualSCSIController().BusNumber).To(Equal(int32(1)))
			Expect(ctrl.GetVirtualSCSIController().GetVirtualController().GetVirtualDevice().Key).To(Equal(int32(-2)))
		})
	})

	When("dst has multiple disks on the same controller", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -201,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 20 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -202,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 30 * 1024 * 1024 * 1024,
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
			}
		})

		It("remaps all disks to the new controller key", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			var disks []*vimtypes.VirtualDisk
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if d, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
						disks = append(disks, d)
					}
				}
			}
			Expect(disks).To(HaveLen(3))
			// All disks should be remapped to -100
			for _, disk := range disks {
				Expect(disk.ControllerKey).To(Equal(int32(-100)))
			}
		})
	})

	When("complex scenario with multiple controllers and disks", func() {
		BeforeEach(func() {
			dst.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -1,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -2,
								},
								BusNumber: 1,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -200,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 10 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							Key:           -201,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 20 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -2,
							Key:           -202,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 30 * 1024 * 1024 * 1024,
					},
				},
			}
			src.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -100,
								},
								BusNumber: 0,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: -200,
								},
								BusNumber: 1,
							},
							SharedBus: "noSharing",
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -100,
							Key:           -300,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 40 * 1024 * 1024 * 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -200,
							Key:           -301,
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{},
								DiskMode:                     string(vimtypes.VirtualDiskModePersistent),
							},
						},
						CapacityInBytes: 50 * 1024 * 1024 * 1024,
					},
				},
			}
		})

		It("merges multiple controllers and disks from both dst and src, remaps disk controller keys, and preserves all disks", func() {
			dstKeyToSrcKey, controllersToRemove, err :=
				pkgutil.ValidateStorageControllerCompatibility(*dst, src)
			Expect(err).ToNot(HaveOccurred())
			pkgutil.MergeStorageControllersAndDisks(
				dst, src, dstKeyToSrcKey, controllersToRemove, "")

			var controllers []vimtypes.BaseVirtualSCSIController
			var disks []*vimtypes.VirtualDisk
			for _, devChange := range dst.DeviceChange {
				spec := devChange.GetVirtualDeviceConfigSpec()
				if spec != nil && spec.Device != nil {
					if ctrl, ok := spec.Device.(vimtypes.BaseVirtualSCSIController); ok {
						controllers = append(controllers, ctrl)
					}
					if d, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
						disks = append(disks, d)
					}
				}
			}

			// Should have 2 controllers (both from src, replacing dst controllers)
			// Both controllers have disks in src, so both should be kept
			Expect(controllers).To(HaveLen(2), "Should have 2 controllers from src")
			// Should have 5 disks total (3 from dst + 2 from src)
			Expect(disks).To(HaveLen(5), "Should have 5 disks total")

			// Find controllers by bus number
			var bus0Ctrl, bus1Ctrl vimtypes.BaseVirtualSCSIController
			for _, ctrl := range controllers {
				if ctrl.GetVirtualSCSIController().BusNumber == 0 {
					bus0Ctrl = ctrl
				} else if ctrl.GetVirtualSCSIController().BusNumber == 1 {
					bus1Ctrl = ctrl
				}
			}
			Expect(bus0Ctrl).ToNot(BeNil(), "Should have bus 0 controller")
			Expect(bus1Ctrl).ToNot(BeNil(), "Should have bus 1 controller")
			Expect(bus0Ctrl.GetVirtualSCSIController().GetVirtualController().GetVirtualDevice().Key).To(Equal(int32(-100)))
			Expect(bus1Ctrl.GetVirtualSCSIController().GetVirtualController().GetVirtualDevice().Key).To(Equal(int32(-200)))

			// Verify disk remapping: disks on bus 0 should point to -100, disks on bus 1 should point to -200
			for _, disk := range disks {
				switch disk.Key {
				case -200, -201:
					Expect(disk.ControllerKey).To(Equal(int32(-100)), "Disks on bus 0 should be remapped to controller -100")
				case -202:
					Expect(disk.ControllerKey).To(Equal(int32(-200)), "Disk on bus 1 should be remapped to controller -200")
				case -300:
					Expect(disk.ControllerKey).To(Equal(int32(-100)), "Disk from src should have controller -100")
				case -301:
					Expect(disk.ControllerKey).To(Equal(int32(-200)), "Disk from src should have controller -200")
				}
			}
		})
	})
})

var _ = Describe("CreateUserStorageControllersConfigSpec", func() {
	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		configSpec vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContext()
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Hardware: &vmopv1.VirtualMachineHardwareSpec{},
			},
		}
		configSpec = vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{},
		}
	})

	When("vm is nil", func() {
		It("returns empty config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, nil, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("vm.Spec.Hardware is nil", func() {
		BeforeEach(func() {
			vm.Spec.Hardware = nil
		})

		It("returns empty config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("SCSI controller has Type but no SharingMode", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber: 0,
					Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("SCSI controller has SharingMode but no Type", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("SCSI controller has invalid Type", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        "InvalidType",
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("SCSI controller has invalid SharingMode", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: "InvalidSharingMode",
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("SCSI controller is fully defined", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("creates controller config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec).ToNot(BeNil())
			Expect(spec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			_, ok := spec.Device.(vimtypes.BaseVirtualSCSIController)
			Expect(ok).To(BeTrue())
		})
	})

	When("NVME controller has no SharingMode", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
				{
					BusNumber: 0,
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("NVME controller has invalid SharingMode", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
				{
					BusNumber:   0,
					SharingMode: "InvalidSharingMode",
				},
			}
		})

		It("skips the controller", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(BeEmpty())
		})
	})

	When("NVME controller is fully defined", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
				{
					BusNumber:   0,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("creates controller config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec).ToNot(BeNil())
			Expect(spec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			_, ok := spec.Device.(*vimtypes.VirtualNVMEController)
			Expect(ok).To(BeTrue())
		})
	})

	When("SATA controller is defined", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
				{
					BusNumber: 0,
				},
			}
		})

		It("creates controller config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec).ToNot(BeNil())
			Expect(spec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			_, ok := spec.Device.(*vimtypes.VirtualAHCIController)
			Expect(ok).To(BeTrue())
		})
	})

	When("IDE controller is defined", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
				{
					BusNumber: 0,
				},
			}
		})

		It("creates controller config spec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec).ToNot(BeNil())
			Expect(spec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			_, ok := spec.Device.(*vimtypes.VirtualIDEController)
			Expect(ok).To(BeTrue())
		})
	})

	When("all controller types are defined", func() {
		BeforeEach(func() {
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
			vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
				{
					BusNumber: 0,
				},
			}
			vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
				{
					BusNumber:   0,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
			vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
				{
					BusNumber: 0,
				},
			}
		})

		It("creates config specs for all controllers", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(4))
		})
	})

	When("configSpec has PCI controller", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualPCIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 100,
							},
						},
					},
				},
			}
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("uses PCI controller key from configSpec", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec.Device.GetVirtualDevice().ControllerKey).To(Equal(int32(100)))
		})
	})

	When("configSpec has existing device changes", func() {
		BeforeEach(func() {
			configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualE1000{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -1,
							},
						},
					},
				},
			}
			vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
				{
					BusNumber:   0,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: vmopv1.VirtualControllerSharingModeNone,
				},
			}
		})

		It("initializes device key from existing changes", func() {
			result := pkgutil.CreateUserStorageControllersConfigSpec(ctx, vm, configSpec)
			Expect(result.DeviceChange).To(HaveLen(1))
			spec := result.DeviceChange[0].GetVirtualDeviceConfigSpec()
			Expect(spec.Device.GetVirtualDevice().Key).To(Equal(int32(-2)))
		})
	})
})
