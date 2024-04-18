// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"reflect"

	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
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

	// Empty Files as they usually ref files in disk
	configSpec.Files = nil
	// Empty VmProfiles as storage profiles are disk specific
	configSpec.VmProfile = []vimtypes.BaseVirtualMachineProfileSpec{}

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

// AppendNewExtraConfigValues add the new extra config values if not already present in the extra config.
func AppendNewExtraConfigValues(
	extraConfig []vimtypes.BaseOptionValue,
	newECMap map[string]string) []vimtypes.BaseOptionValue {

	ecMap := make(map[string]vimtypes.AnyType)
	for _, opt := range extraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			ecMap[optValue.Key] = optValue.Value
		}
	}

	// Only add fields that aren't already in the ExtraConfig.
	var newExtraConfig []vimtypes.BaseOptionValue
	for k, v := range newECMap {
		if _, exists := ecMap[k]; !exists {
			newExtraConfig = append(newExtraConfig, &vimtypes.OptionValue{Key: k, Value: v})
		}
	}

	return append(extraConfig, newExtraConfig...)
}

// ExtraConfigToMap converts the ExtraConfig to a map with string values.
func ExtraConfigToMap(input []vimtypes.BaseOptionValue) (output map[string]string) {
	output = make(map[string]string)
	for _, opt := range input {
		if optValue := opt.GetOptionValue(); optValue != nil {
			// Only set string type values
			if val, ok := optValue.Value.(string); ok {
				output[optValue.Key] = val
			}
		}
	}
	return
}

// MergeExtraConfig adds the key/value to the ExtraConfig if the key is not
// present or the new value is different than the existing value.
// It returns the newly added ExtraConfig.
// Please note the result *may* include keys with empty values. This indicates
// to vSphere to remove the key/value pair.
func MergeExtraConfig(
	existingExtraConfig []vimtypes.BaseOptionValue,
	newKeyValuePairs map[string]string) []vimtypes.BaseOptionValue {

	var mergedExtraConfig []vimtypes.BaseOptionValue
	existingExtraConfigKeyValuePairs := ExtraConfigToMap(existingExtraConfig)

	for nk, nv := range newKeyValuePairs {
		if ev, ok := existingExtraConfigKeyValuePairs[nk]; !ok || nv != ev {
			mergedExtraConfig = append(
				mergedExtraConfig,
				&vimtypes.OptionValue{
					Key:   nk,
					Value: nv,
				})
		}
	}

	return mergedExtraConfig
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
