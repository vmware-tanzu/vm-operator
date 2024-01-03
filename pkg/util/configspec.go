// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/vmware/govmomi/vim25"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// MarshalConfigSpecToXML returns a byte slice of the provided ConfigSpec
// marshalled to an XML string.
func MarshalConfigSpecToXML(
	configSpec *vimTypes.VirtualMachineConfigSpec) ([]byte, error) {

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
				Value: vim25.Namespace + ":" + reflect.TypeOf(vimTypes.VirtualMachineConfigSpec{}).Name(),
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
	data []byte) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}

	// Instantiate a new XML decoder in order to specify the lookup table used
	// by GoVmomi to transform XML types to Golang types.
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.TypeFunc = vimTypes.TypeFunc()

	if err := dec.Decode(&configSpec); err != nil {
		return nil, err
	}

	return configSpec, nil
}

// UnmarshalConfigSpecFromBase64XML returns a ConfigSpec object from a
// byte-slice of the ConfigSpec marshaled as a base64-encoded, XML string.
func UnmarshalConfigSpecFromBase64XML(
	src []byte) (*vimTypes.VirtualMachineConfigSpec, error) {

	data, err := Base64Decode(src)
	if err != nil {
		return nil, err
	}
	return UnmarshalConfigSpecFromXML(data)
}

// MarshalConfigSpecToJSON returns a byte slice of the provided ConfigSpec
// marshaled to a JSON string.
func MarshalConfigSpecToJSON(
	configSpec *vimTypes.VirtualMachineConfigSpec) ([]byte, error) {

	var w bytes.Buffer
	enc := vimTypes.NewJSONEncoder(&w)
	if err := enc.Encode(configSpec); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// UnmarshalConfigSpecFromJSON returns a ConfigSpec object from a byte-slice of
// the ConfigSpec marshaled as a JSON string.
func UnmarshalConfigSpecFromJSON(
	data []byte) (*vimTypes.VirtualMachineConfigSpec, error) {

	var configSpec vimTypes.VirtualMachineConfigSpec

	dec := vimTypes.NewJSONDecoder(bytes.NewReader(data))
	if err := dec.Decode(&configSpec); err != nil {
		return nil, err
	}
	return &configSpec, nil
}

// DevicesFromConfigSpec returns a slice of devices from the ConfigSpec's
// DeviceChange property.
func DevicesFromConfigSpec(
	configSpec *vimTypes.VirtualMachineConfigSpec,
) []vimTypes.BaseVirtualDevice {
	if configSpec == nil {
		return nil
	}

	var devices []vimTypes.BaseVirtualDevice
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
	configSpec *vimTypes.VirtualMachineConfigSpec) {

	// These are unique for each VM.
	configSpec.Uuid = ""
	configSpec.InstanceUuid = ""

	// Empty Files as they usually ref files in disk
	configSpec.Files = nil
	// Empty VmProfiles as storage profiles are disk specific
	configSpec.VmProfile = []vimTypes.BaseVirtualMachineProfileSpec{}

	if pkgconfig.FromContext(ctx).Features.VMClassAsConfig {
		// Remove all virtual disks except disks with raw device mapping backings.
		RemoveDevicesFromConfigSpec(configSpec, isNonRDMDisk)
	} else {
		// Remove all virtual disks and disk controllers
		RemoveDevicesFromConfigSpec(configSpec, isDiskOrDiskController)
	}
}

// RemoveDevicesFromConfigSpec removes devices from config spec device changes based on the matcher function.
func RemoveDevicesFromConfigSpec(configSpec *vimTypes.VirtualMachineConfigSpec, fn func(vimTypes.BaseVirtualDevice) bool) {
	if configSpec == nil {
		return
	}

	var targetDevChanges []vimTypes.BaseVirtualDeviceConfigSpec
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
	extraConfig []vimTypes.BaseOptionValue,
	newECMap map[string]string) []vimTypes.BaseOptionValue {

	ecMap := make(map[string]vimTypes.AnyType)
	for _, opt := range extraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			ecMap[optValue.Key] = optValue.Value
		}
	}

	// Only add fields that aren't already in the ExtraConfig.
	var newExtraConfig []vimTypes.BaseOptionValue
	for k, v := range newECMap {
		if _, exists := ecMap[k]; !exists {
			newExtraConfig = append(newExtraConfig, &vimTypes.OptionValue{Key: k, Value: v})
		}
	}

	return append(extraConfig, newExtraConfig...)
}

// ExtraConfigToMap converts the ExtraConfig to a map with string values.
func ExtraConfigToMap(input []vimTypes.BaseOptionValue) (output map[string]string) {
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

// MergeExtraConfig adds the key/value to the ExtraConfig if the key is not present.
// It returns the newly added ExtraConfig.
func MergeExtraConfig(extraConfig []vimTypes.BaseOptionValue, newMap map[string]string) []vimTypes.BaseOptionValue {
	merged := make([]vimTypes.BaseOptionValue, 0)
	ecMap := ExtraConfigToMap(extraConfig)
	for k, v := range newMap {
		if _, exists := ecMap[k]; !exists {
			merged = append(merged, &vimTypes.OptionValue{Key: k, Value: v})
		}
	}
	return merged
}

// EnsureMinHardwareVersionInConfigSpec ensures that the hardware version in the ConfigSpec
// is at least equal to the passed minimum hardware version value.
func EnsureMinHardwareVersionInConfigSpec(configSpec *vimTypes.VirtualMachineConfigSpec, minVersion int32) {
	if minVersion == 0 {
		return
	}

	configSpecHwVersion := int32(0)
	if configSpec.Version != "" {
		configSpecHwVersion = ParseVirtualHardwareVersion(configSpec.Version)
	}
	if minVersion > configSpecHwVersion {
		configSpecHwVersion = minVersion
	}
	configSpec.Version = fmt.Sprintf("vmx-%d", configSpecHwVersion)
}
