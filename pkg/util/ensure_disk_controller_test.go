// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	vmx20 = "vmx-20"
)

type nilVirtualDeviceConfigSpec struct{}

func (n nilVirtualDeviceConfigSpec) GetVirtualDeviceConfigSpec() *vimtypes.VirtualDeviceConfigSpec {
	return nil
}

type nilVirtualDevice struct{}

func (n nilVirtualDevice) GetVirtualDevice() *vimtypes.VirtualDevice {
	return nil
}

var _ = DescribeTable(
	"EnsureDisksHaveControllers",
	func(
		configSpec *vimtypes.VirtualMachineConfigSpec,
		existingDevices []vimtypes.BaseVirtualDevice,
		expectedErr error,
		expectedConfigSpec *vimtypes.VirtualMachineConfigSpec) {

		err := pkgutil.EnsureDisksHaveControllers(configSpec, existingDevices...)
		if expectedErr != nil {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expectedErr))
		} else {
			Expect(configSpec).To(BeComparableTo(expectedConfigSpec))
		}
	},

	Entry(
		"return an error when configSpec arg is nil",
		nil,
		nil,
		fmt.Errorf("configSpec is nil"),
		nil,
	),

	Entry(
		"do nothing if configSpec has no disks",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
		},
		nil,
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
		},
	),

	Entry(
		"do nothing if configSpec has a disk, but it is being removed",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		nil,
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
	),

	Entry(
		"do nothing if configSpec has a disk, but it is already attached to a controller from existing devices",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
						},
					},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and there is a nil base device change in ConfigSpec and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				vimtypes.BaseVirtualDeviceConfigSpec(nil),
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				vimtypes.BaseVirtualDeviceConfigSpec(nil),
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disks to multiple new PVSCSI controllers when adding disks in ConfigSpec and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version:      vmx20,
			DeviceChange: generateDeviceChangesForDefaultVirtualDisk(17), // maxDisksPerSCSIController+1
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: joinSlices(
				generateDeviceChangesForVirtualDisk(16, -1),
				generateDeviceChangesForVirtualDisk(1, -2),
				[]vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: 100,
										Key:           -1,
									},
									BusNumber: 0,
								},
								HotAddRemove: ptr.To(true),
								SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.ParaVirtualSCSIController{
							VirtualSCSIController: vimtypes.VirtualSCSIController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: 100,
										Key:           -2,
									},
									BusNumber: 1,
								},
								HotAddRemove: ptr.To(true),
								SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
							},
						},
					},
				},
			),
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and there is a nil device change in ConfigSpec and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				nilVirtualDeviceConfigSpec{},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				nilVirtualDeviceConfigSpec{},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and there is a nil base device in ConfigSpec and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: vimtypes.BaseVirtualDevice(nil),
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: vimtypes.BaseVirtualDevice(nil),
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and there is a nil device in ConfigSpec and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: nilVirtualDevice{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: nilVirtualDevice{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and existing device includes PCI controller and nil base device",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			vimtypes.BaseVirtualDevice(nil),
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk and existing device includes PCI controller and nil device",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			nilVirtualDevice{},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk while removing SATA controller and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk that references SATA controller being removed and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 15000,
						},
					},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk that references non-existent controller and existing device includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1000,
						},
					},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk sans controller and no existing devices",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		nil,
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -2,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualPCIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -1,
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
									ControllerKey: -1,
									Key:           -2,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding PCI controller and disk sans controller and no existing devices",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualPCIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -50,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		nil,
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualPCIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -50,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -51,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: -50,
									Key:           -51,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to new PVSCSI controller when adding disk sans controller and existing devices only includes PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	//
	// SCSI (in existing devices)
	//
	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and PVSCSI controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk that specifies unit and existing controller when adding disk and existing devices includes PCI and PVSCSI controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](4),
						},
					},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](4),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach multiple disks, with one disk specifying unit number and existing controller when adding disk and existing devices includes PCI and PVSCSI controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 42, // Disk with this size should keep its reserved unit number.
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](1),
						},
					},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](2),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						CapacityInBytes: 42,
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](1),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and Bus Logic controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualBusLogicController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and LSI Logic controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualLsiLogicController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and LSI Logic SAS controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualLsiLogicSASController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and SCSI controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: 100,
						Key:           1000,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 1000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// SATA (in existing devices)
	//
	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and SATA controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualAHCIController{
				VirtualSATAController: vimtypes.VirtualSATAController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           15000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 15000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and AHCI controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualAHCIController{
				VirtualSATAController: vimtypes.VirtualSATAController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           15000,
						},
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 15000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// NVME (in existing devices)
	//
	Entry(
		"attach disk to existing controller when adding disk and existing devices includes PCI and NVME controllers",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.VirtualNVMEController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: 100,
						Key:           31000,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 31000,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// SCSI (in ConfigSpec)
	//
	Entry(
		"attach disk to PVSCSI controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to Bus Logic controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to LSI Logic controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to LSI Logic SAS controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to SCSI controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           -10,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           -10,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// SATA (in ConfigSpec)
	//
	Entry(
		"attach disk to SATA controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	Entry(
		"attach disk to AHCI controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -10,
								},
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// NVME (in ConfigSpec)
	//
	Entry(
		"attach disk to NVME controller in ConfigSpec when adding disk and existing devices includes only PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           -10,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
		},
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           -10,
							},
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -10,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
			},
		},
	),

	//
	// First SCSI HBA is full
	//
	Entry(
		"attach disk to new PVSCSI controller when adding disk and existing devices includes PCI controller and LSI Logic controller with no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
								BusNumber: 1,
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	//
	// First and second SCSI HBAs are full
	//
	Entry(
		"attach disk to new PVSCSI controller when adding disk and existing devices includes PCI controller and two existing SCSI controllers have no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
								BusNumber: 0,
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),

			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
								BusNumber: 3,
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 1,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	//
	// First, second, and third SCSI HBAs are full
	//
	Entry(
		"attach disk to new PVSCSI controller when adding disk and existing devices includes PCI controller and three existing SCSI controllers have no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
								BusNumber: 3,
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),

			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
								BusNumber: 0,
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),

			//
			// SCSI HBA 3 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1002,
								},
								BusNumber: 1,
							},
						},
					},
				},
				generateVirtualDisks(16, 1002)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 2,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	//
	// First, second, and third SCSI HBAs are full (different bus number)
	//
	Entry(
		"attach disk to new PVSCSI controller (bus number three) when adding disk and existing devices includes PCI controller and three existing SCSI controllers have no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
								BusNumber: 2,
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),

			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
								BusNumber: 0,
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),

			//
			// SCSI HBA 3 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1002,
								},
								BusNumber: 1,
							},
						},
					},
				},
				generateVirtualDisks(16, 1002)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 3,
							},
							HotAddRemove: ptr.To(true),
							SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
						},
					},
				},
			},
		},
	),

	//
	// All SCSI HBAs are full
	//
	Entry(
		"attach disk to new SATA controller when adding disk and existing devices includes PCI controller and there are already four SCSI controllers with no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),
			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),
			//
			// SCSI HBA 3 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           1002,
							},
						},
					},
				},
				generateVirtualDisks(16, 1002)...,
			),
			//
			// SCSI HBA 4 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1003,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1003)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           -1,
								},
								BusNumber: 0,
							},
						},
					},
				},
			},
		},
	),

	//
	// All SCSI and SATA HBAs are full
	//
	Entry(
		"attach disk to new NVME controller when adding disk and existing devices includes PCI controller and there are already four SCSI and SATA controllers with no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),
			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),
			//
			// SCSI HBA 3 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           1002,
							},
						},
					},
				},
				generateVirtualDisks(16, 1002)...,
			),
			//
			// SCSI HBA 4 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1003,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1003)...,
			),

			//
			// SATA HBA 1 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15000)...,
			),
			//
			// SATA HBA 2 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15001,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15001)...,
			),
			//
			// SATA HBA 3 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15002,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15002)...,
			),
			//
			// SATA HBA 4 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15003,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15003)...,
			),
		),
		nil,
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: -1,
							UnitNumber:    ptr.To[int32](0),
						},
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           -1,
							},
							BusNumber: 0,
						},
						SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
					},
				},
			},
		},
	),

	//
	// All SCSI, SATA, and NVME HBAs are full
	//
	Entry(
		"return an error that there are no available controllers when adding a disk and existing devices includes PCI controller and there are already four SCSI, SATA, and NVME controllers with no free slots",
		&vimtypes.VirtualMachineConfigSpec{
			Version: vmx20,
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    &vimtypes.VirtualDisk{},
				},
			},
		},
		joinSlices(
			[]vimtypes.BaseVirtualDevice{
				&vimtypes.VirtualPCIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 100,
						},
					},
				},
			},

			//
			// SCSI HBA 1 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1000,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1000)...,
			),
			//
			// SCSI HBA 2 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1001,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1001)...,
			),
			//
			// SCSI HBA 3 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualSCSIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           1002,
							},
						},
					},
				},
				generateVirtualDisks(16, 1002)...,
			),
			//
			// SCSI HBA 4 / 16 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           1003,
								},
							},
						},
					},
				},
				generateVirtualDisks(16, 1003)...,
			),

			//
			// SATA HBA 1 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15000,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15000)...,
			),
			//
			// SATA HBA 2 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15001,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15001)...,
			),
			//
			// SATA HBA 3 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualSATAController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           15002,
							},
						},
					},
				},
				generateVirtualDisks(30, 15002)...,
			),
			//
			// SATA HBA 4 / 30 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualAHCIController{
						VirtualSATAController: vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: 100,
									Key:           15003,
								},
							},
						},
					},
				},
				generateVirtualDisks(30, 15003)...,
			),

			//
			// NVME HBA 1 / 15 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           31000,
							},
						},
					},
				},
				generateVirtualDisks(15, 31000)...,
			),
			//
			// NVME HBA 2 / 15 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           31001,
							},
						},
					},
				},
				generateVirtualDisks(15, 31001)...,
			),
			//
			// NVME HBA 3 / 15 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           31002,
							},
						},
					},
				},
				generateVirtualDisks(15, 31002)...,
			),
			//
			// NVME HBA 4 / 15 disks
			//
			append(
				[]vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualNVMEController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								ControllerKey: 100,
								Key:           31003,
							},
						},
					},
				},
				generateVirtualDisks(15, 31003)...,
			),
		),
		fmt.Errorf("no controllers available"),
		nil,
	),

	Entry(
		"return an error that there are no available unit numbers when adding disks and existing devices includes PVSCSI and PCI controller",
		&vimtypes.VirtualMachineConfigSpec{
			Version:      vmx20,
			DeviceChange: generateDeviceChangesForVirtualDiskWithJustController(17, 1000), // maxDisksPerSCSIController+1
		},
		[]vimtypes.BaseVirtualDevice{
			&vimtypes.VirtualPCIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
					},
				},
			},
			&vimtypes.ParaVirtualSCSIController{
				VirtualSCSIController: vimtypes.VirtualSCSIController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							ControllerKey: 100,
							Key:           1000,
						},
					},
				},
			},
		},
		fmt.Errorf("no available unit number for controller key 1000"),
		nil,
	),
)

func joinSlices[T any](a []T, b ...[]T) []T {
	for i := range b {
		a = append(a, b[i]...)
	}
	return a
}

func generateVirtualDisks(numDisks int, controllerKey int32) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, numDisks)
	for i := range devices {
		devices[i] = &vimtypes.VirtualDisk{
			VirtualDevice: vimtypes.VirtualDevice{
				ControllerKey: controllerKey,
				UnitNumber:    ptr.To(int32(i)),
			},
		}
	}
	return devices
}

func generateDeviceChangesForDefaultVirtualDisk(numDisks int) []vimtypes.BaseVirtualDeviceConfigSpec {
	devChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, numDisks)
	for i := range numDisks {
		devChanges[i] = &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    &vimtypes.VirtualDisk{},
		}
	}
	return devChanges
}

func generateDeviceChangesForVirtualDiskWithJustController(numDisks int, controllerKey int32) []vimtypes.BaseVirtualDeviceConfigSpec {
	devChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, numDisks)
	for i := range numDisks {
		devChanges[i] = &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device: &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					ControllerKey: controllerKey,
				},
			},
		}
	}
	return devChanges
}

func generateDeviceChangesForVirtualDisk(numDisks int, controllerKey int32) []vimtypes.BaseVirtualDeviceConfigSpec {
	devChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, numDisks)
	for i := range numDisks {
		devChanges[i] = &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device: &vimtypes.VirtualDisk{
				VirtualDevice: vimtypes.VirtualDevice{
					ControllerKey: controllerKey,
					UnitNumber:    ptr.To(int32(i)),
				},
			},
		}
	}
	return devChanges
}
