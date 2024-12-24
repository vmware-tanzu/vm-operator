// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"reflect"
	"regexp"

	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

// MarshalConfigSpecToXML returns a byte slice of the provided ConfigSpec
// marshalled to an XML string.
func MarshalConfigSpecToXML(
	configSpec vimtypes.VirtualMachineConfigSpec) ([]byte, error) {

	start := xml.StartElement{
		Name: xml.Name{
			Local: "obj",
		},
		Attr: []xml.Attr{
			{
				Name:  xml.Name{Local: "xmlns:" + vim25.Namespace},
				Value: "urn:" + vim25.Namespace,
			},
			{
				Name:  xml.Name{Local: "xmlns:xsi"},
				Value: XsiNamespace,
			},
			{
				Name:  xml.Name{Local: "xsi:type"},
				Value: vim25.Namespace + ":" + reflect.TypeOf(vimtypes.VirtualMachineConfigSpec{}).Name(),
			},
		},
	}

	var w bytes.Buffer
	enc := xml.NewEncoder(&w)
	err := enc.EncodeElement(configSpec, start)
	if err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// UnmarshalConfigSpecFromXML returns a ConfigSpec object from a byte-slice of
// the ConfigSpec marshaled as an XML string.
func UnmarshalConfigSpecFromXML(
	data []byte) (vimtypes.VirtualMachineConfigSpec, error) {

	var configSpec vimtypes.VirtualMachineConfigSpec

	// Instantiate a new XML decoder in order to specify the lookup table used
	// by GoVmomi to transform XML types to Golang types.
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.TypeFunc = vimtypes.TypeFunc()

	if err := dec.Decode(&configSpec); err != nil {
		return vimtypes.VirtualMachineConfigSpec{}, err
	}

	return configSpec, nil
}

// UnmarshalConfigSpecFromBase64XML returns a ConfigSpec object from a
// byte-slice of the ConfigSpec marshaled as a base64-encoded, XML string.
func UnmarshalConfigSpecFromBase64XML(
	src []byte) (vimtypes.VirtualMachineConfigSpec, error) {

	data, err := Base64Decode(src)
	if err != nil {
		return vimtypes.VirtualMachineConfigSpec{}, err
	}
	return UnmarshalConfigSpecFromXML(data)
}

// MarshalConfigSpecToJSON returns a byte slice of the provided ConfigSpec
// marshaled to a JSON string.
func MarshalConfigSpecToJSON(
	configSpec vimtypes.VirtualMachineConfigSpec) ([]byte, error) {

	var w bytes.Buffer
	enc := vimtypes.NewJSONEncoder(&w)
	if err := enc.Encode(configSpec); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// UnmarshalConfigSpecFromJSON returns a ConfigSpec object from a byte-slice of
// the ConfigSpec marshaled as a JSON string.
func UnmarshalConfigSpecFromJSON(
	data []byte) (vimtypes.VirtualMachineConfigSpec, error) {

	var configSpec vimtypes.VirtualMachineConfigSpec

	dec := vimtypes.NewJSONDecoder(bytes.NewReader(data))
	if err := dec.Decode(&configSpec); err != nil {
		return vimtypes.VirtualMachineConfigSpec{}, err
	}
	return configSpec, nil
}

// DevicesFromConfigSpec returns a slice of devices from the ConfigSpec's
// DeviceChange property.
func DevicesFromConfigSpec(
	configSpec *vimtypes.VirtualMachineConfigSpec,
) []vimtypes.BaseVirtualDevice {
	if configSpec == nil {
		return nil
	}

	var devices []vimtypes.BaseVirtualDevice
	for _, devChange := range configSpec.DeviceChange {
		if spec := devChange.GetVirtualDeviceConfigSpec(); spec != nil {
			if dev := spec.Device; dev != nil {
				devices = append(devices, dev)
			}
		}
	}
	return devices
}

// SanitizeVMClassConfigSpec clears fields in the class ConfigSpec that are
// not allowed or supported.
func SanitizeVMClassConfigSpec(
	ctx context.Context,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	// These are unique for each VM.
	configSpec.Uuid = ""
	configSpec.InstanceUuid = ""
	configSpec.GuestId = ""

	// Empty Files as they usually ref files in disk
	configSpec.Files = nil
	// Empty VmProfiles as storage profiles are disk specific
	configSpec.VmProfile = []vimtypes.BaseVirtualMachineProfileSpec{}

	configSpec.ExtraConfig = OptionValues(configSpec.ExtraConfig).Delete(
		constants.MMPowerOffVMExtraConfigKey,
	)

	// Remove all virtual disks except disks with raw device mapping backings.
	RemoveDevicesFromConfigSpec(configSpec, isNonRDMDisk)
}

// RemoveDevicesFromConfigSpec removes devices from config spec device changes based on the matcher function.
func RemoveDevicesFromConfigSpec(configSpec *vimtypes.VirtualMachineConfigSpec, fn func(vimtypes.BaseVirtualDevice) bool) {
	if configSpec == nil {
		return
	}

	var targetDevChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, devChange := range configSpec.DeviceChange {
		dSpec := devChange.GetVirtualDeviceConfigSpec()
		if !fn(dSpec.Device) {
			targetDevChanges = append(targetDevChanges, devChange)
		}
	}
	configSpec.DeviceChange = targetDevChanges
}

// EnsureMinHardwareVersionInConfigSpec ensures that the hardware version in the
// ConfigSpec is at least equal to the passed minimum hardware version value.
func EnsureMinHardwareVersionInConfigSpec(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	minVersion int32) {

	minHwVersion := vimtypes.HardwareVersion(minVersion)
	if !minHwVersion.IsValid() {
		return
	}

	var configSpecHwVersion vimtypes.HardwareVersion
	if configSpec.Version != "" {
		configSpecHwVersion, _ = vimtypes.ParseHardwareVersion(configSpec.Version)
	}
	if minHwVersion > configSpecHwVersion {
		configSpecHwVersion = minHwVersion
	}
	configSpec.Version = configSpecHwVersion.String()
}

// SafeConfigSpecToString returns the string-ified version of the provided
// ConfigSpec, first trying to use the special JSON encoder, then defaulting to
// the normal JSON encoder.
//
// Please note, this function is not intended to replace marshaling the data
// to JSON using the normal workflows. This function is for when a string-ified
// version of the data is needed for things like logging.
func SafeConfigSpecToString(
	in *vimtypes.VirtualMachineConfigSpec) (s string) {

	return vimtypes.ToString(in)
}

var dsNameRX = regexp.MustCompile(`^\[([^\]].+)\].*$`)

// DatastoreNameFromStorageURI returns the datastore name from a storage URI,
// ex.: [my-datastore-1] vm-name/vm-name.vmx. The previous URI would return the
// value "my-datastore-1".
// An empty string is returned if there is no match.
func DatastoreNameFromStorageURI(s string) string {
	m := dsNameRX.FindStringSubmatch(s)
	if len(m) == 0 {
		return ""
	}
	return m[1]
}

// CopyStorageControllersAndDisks copies the storage controllers and disks from
// the source spec to the destination. This function does not attempt to handle
// any conflicts -- it is a blind copy. If the provided storagePolicyID is
// non-empty, it is assigned to any all the copied disks.
func CopyStorageControllersAndDisks(
	dst *vimtypes.VirtualMachineConfigSpec,
	src vimtypes.VirtualMachineConfigSpec,
	storagePolicyID string) {

	ctrlKeys := map[int32]struct{}{}
	diskCtrlKeys := map[int32]struct{}{}

	for i := range src.DeviceChange {
		srcSpec := src.DeviceChange[i].GetVirtualDeviceConfigSpec()
		if srcSpec.Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd {

			var dstSpec *vimtypes.VirtualDeviceConfigSpec

			switch srcDev := srcSpec.Device.(type) {
			case vimtypes.BaseVirtualSCSIController,
				vimtypes.BaseVirtualSATAController,
				*vimtypes.VirtualIDEController,
				*vimtypes.VirtualNVMEController:

				ctrlKeys[srcDev.GetVirtualDevice().Key] = struct{}{}

				dstSpec = &vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device:    srcDev,
				}

			case *vimtypes.VirtualDisk:

				diskCtrlKeys[srcDev.ControllerKey] = struct{}{}

				dstSpec = &vimtypes.VirtualDeviceConfigSpec{
					Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
					FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
					Device:        srcDev,
				}
				if storagePolicyID != "" {
					dstSpec.Profile = []vimtypes.BaseVirtualMachineProfileSpec{
						&vimtypes.VirtualMachineDefinedProfileSpec{
							ProfileId: storagePolicyID,
						},
					}
				}
			}

			if dstSpec != nil {
				dst.DeviceChange = append(dst.DeviceChange, dstSpec)
			}
		}
	}

	// Remove any controllers that came from the OVF but are not used by disks.
	RemoveDevicesFromConfigSpec(dst, func(bvd vimtypes.BaseVirtualDevice) bool {
		if bvc, ok := bvd.(vimtypes.BaseVirtualController); ok {
			vc := bvc.GetVirtualController()
			if _, ok := ctrlKeys[vc.Key]; ok {
				if _, ok := diskCtrlKeys[vc.Key]; !ok {
					return true
				}
			}
		}
		return false
	})
}
