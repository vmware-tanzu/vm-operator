// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/ovf"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
)

var vmxRe = regexp.MustCompile(`vmx-(\d+)`)

// ParseVirtualHardwareVersion parses the virtual hardware version
// For eg. "vmx-15" returns 15.
func ParseVirtualHardwareVersion(vmxVersion string) int32 {
	// obj is the full string and the submatch (\d+) and return a []string with values
	obj := vmxRe.FindStringSubmatch(vmxVersion)
	if len(obj) != 2 {
		return 0
	}

	version, err := strconv.ParseInt(obj[1], 10, 32)
	if err != nil {
		return 0
	}

	return int32(version)
}

// UpdateVmiWithOvfEnvelope updates the given vmi object with the content of given OVF envelope.
func UpdateVmiWithOvfEnvelope(obj client.Object, ovfEnvelope ovf.Envelope) {
	var status *vmopv1.VirtualMachineImageStatus

	switch vmi := obj.(type) {
	case *vmopv1.VirtualMachineImage:
		status = &vmi.Status
	case *vmopv1.ClusterVirtualMachineImage:
		status = &vmi.Status
	default:
		return
	}

	if ovfEnvelope.VirtualSystem != nil {
		initImageStatusFromOVFVirtualSystem(status, ovfEnvelope.VirtualSystem)

		if setter, ok := obj.(conditions.Setter); ok {
			if isOVFV1Alpha1Compatible(ovfEnvelope.VirtualSystem) {
				conditions.MarkTrue(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
			} else {
				conditions.Delete(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
			}
		}
	}
}

func initImageStatusFromOVFVirtualSystem(
	imageStatus *vmopv1.VirtualMachineImageStatus,
	ovfVirtualSystem *ovf.VirtualSystem) {

	// Use info from the first product section in the VM image, if one exists.
	if product := ovfVirtualSystem.Product; len(product) > 0 {
		p := product[0]

		productInfo := &imageStatus.ProductInfo
		productInfo.Vendor = p.Vendor
		productInfo.Product = p.Product
		productInfo.Version = p.Version
		productInfo.FullVersion = p.FullVersion
	}

	// Use operating system info from the first os section in the VM image, if one exists.
	if os := ovfVirtualSystem.OperatingSystem; len(os) > 0 {
		o := os[0]

		osInfo := &imageStatus.OSInfo
		osInfo.ID = strconv.Itoa(int(o.ID))
		if o.Version != nil {
			osInfo.Version = *o.Version
		}
		if o.OSType != nil {
			osInfo.Type = *o.OSType
		}
	}

	// Use hardware section info from the VM image, if one exists.
	if virtualHW := ovfVirtualSystem.VirtualHardware; len(virtualHW) > 0 {
		imageStatus.Firmware = getFirmwareType(virtualHW[0])

		if sys := virtualHW[0].System; sys != nil && sys.VirtualSystemType != nil {
			ver := ParseVirtualHardwareVersion(*sys.VirtualSystemType)
			if ver != 0 {
				imageStatus.HardwareVersion = &ver
			}
		}
	}

	for _, product := range ovfVirtualSystem.Product {
		for _, prop := range product.Property {
			// Only show user configurable properties
			if prop.UserConfigurable != nil && *prop.UserConfigurable {
				property := vmopv1.OVFProperty{
					Key:     prop.Key,
					Type:    prop.Type,
					Default: prop.Default,
				}
				imageStatus.OVFProperties = append(imageStatus.OVFProperties, property)
			}
		}
	}

	ovfSystemProps := getVmwareSystemPropertiesFromOvf(ovfVirtualSystem)
	if len(ovfSystemProps) > 0 {
		for k, v := range ovfSystemProps {
			prop := common.KeyValuePair{
				Key:   k,
				Value: v,
			}
			imageStatus.VMwareSystemProperties = append(imageStatus.VMwareSystemProperties, prop)
		}
	}
}

func getVmwareSystemPropertiesFromOvf(ovfVirtualSystem *ovf.VirtualSystem) map[string]string {
	properties := make(map[string]string)

	if ovfVirtualSystem != nil {
		for _, product := range ovfVirtualSystem.Product {
			for _, prop := range product.Property {
				if strings.HasPrefix(prop.Key, "vmware-system") {
					if prop.Default != nil {
						properties[prop.Key] = *prop.Default
					}
				}
			}
		}
	}

	return properties
}

// isOVFV1Alpha1Compatible checks if the image has VMOperatorV1Alpha1ExtraConfigKey
// set to VMOperatorV1Alpha1ConfigReady in it's the ExtraConfig.
func isOVFV1Alpha1Compatible(ovfVirtualSystem *ovf.VirtualSystem) bool {
	if ovfVirtualSystem != nil {
		for _, virtualHardware := range ovfVirtualSystem.VirtualHardware {
			for _, config := range virtualHardware.ExtraConfig {
				if config.Key == constants.VMOperatorV1Alpha1ExtraConfigKey && config.Value == constants.VMOperatorV1Alpha1ConfigReady {
					return true
				}
			}
		}
	}
	return false
}

// getFirmwareType returns the firmware type (eg: "efi", "bios") present in the virtual hardware section of the OVF.
func getFirmwareType(hardware ovf.VirtualHardwareSection) string {
	for _, cfg := range hardware.Config {
		if cfg.Key == "firmware" {
			return cfg.Value
		}
	}
	return ""
}
