// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vmdk"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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

// UpdateVmiWithOvfEnvelope updates the given VMI object from an OVF envelope.
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

	if ovfEnvelope.Disk != nil && len(ovfEnvelope.Disk.Disks) > 0 {
		populateImageStatusFromOVFDiskSection(status, ovfEnvelope.Disk)
	} else {
		status.Disks = nil
	}
}

// UpdateVmiWithVirtualMachine updates the given VMI object from a
// mo.VirtualMachine data structure.
func UpdateVmiWithVirtualMachine(
	ctx context.Context,
	obj client.Object,
	vm mo.VirtualMachine) {

	var status *vmopv1.VirtualMachineImageStatus

	switch vmi := obj.(type) {
	case *vmopv1.VirtualMachineImage:
		status = &vmi.Status
	case *vmopv1.ClusterVirtualMachineImage:
		status = &vmi.Status
	default:
		return
	}

	if vm.Config == nil {
		return
	}

	// Populate the firmware.
	status.Firmware = vm.Config.Firmware

	// Populate the guest info.
	status.OSInfo.ID = vm.Config.GuestId
	if vm.Guest != nil {
		status.OSInfo.Type = vm.Guest.GuestFamily
	}

	// Populate the hardware version.
	if version := ParseVirtualHardwareVersion(vm.Config.Version); version > 0 {
		status.HardwareVersion = &version
	}

	if vac := vm.Config.VAppConfig; vac != nil {
		if vmCI := vac.GetVmConfigInfo(); vmCI != nil {

			// Populate the product information.
			if product := vmCI.Product; len(product) > 0 {
				p := product[0]
				status.ProductInfo = vmopv1.VirtualMachineImageProductInfo{
					Product:     p.Name,
					Vendor:      p.Vendor,
					Version:     p.Version,
					FullVersion: p.FullVersion,
				}
			}

			// Populate the vApp properties.
			status.OVFProperties = nil
			status.VMwareSystemProperties = nil
			for _, prop := range vmCI.Property {

				if strings.HasPrefix(prop.Id, "vmware-system") {

					// Handle vmware-system properties.
					p := common.KeyValuePair{
						Key:   prop.Id,
						Value: prop.DefaultValue,
					}
					status.VMwareSystemProperties = append(
						status.VMwareSystemProperties,
						p)

				} else if prop.UserConfigurable != nil && *prop.UserConfigurable {

					// Handle non-vmware-system properties that are
					// user-configurable.
					p := vmopv1.OVFProperty{
						Key:     prop.Id,
						Type:    prop.Type,
						Default: &prop.DefaultValue,
					}
					status.OVFProperties = append(status.OVFProperties, p)

				}
			}
		}
	}

	// Populate the disk information.
	status.Disks = nil
	for _, bd := range vm.Config.Hardware.Device {
		if disk, ok := bd.(*vimtypes.VirtualDisk); ok {

			var (
				capacity *resource.Quantity
				size     *resource.Quantity
			)

			var uuid string
			switch tb := disk.Backing.(type) {
			case *vimtypes.VirtualDiskFlatVer2BackingInfo:
				uuid = tb.Uuid
			case *vimtypes.VirtualDiskSeSparseBackingInfo:
				uuid = tb.Uuid
			case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
				uuid = tb.Uuid
			case *vimtypes.VirtualDiskSparseVer2BackingInfo:
				uuid = tb.Uuid
			case *vimtypes.VirtualDiskRawDiskVer2BackingInfo:
				uuid = tb.Uuid
			default:
				capacity = kubeutil.BytesToResource(disk.CapacityInBytes)
			}

			if uuid != "" {
				di, _ := vmdk.GetVirtualDiskInfoByUUID(
					ctx,
					nil,   /* the client is not needed since props aren't refetched */
					vm,    /* use props from this object */
					false, /* do not refetch props */
					false, /* include disks related to snapshots */
					uuid)
				capacity = kubeutil.BytesToResource(di.CapacityInBytes)
				size = kubeutil.BytesToResource(di.Size)
			}

			status.Disks = append(
				status.Disks,
				vmopv1.VirtualMachineImageDiskInfo{
					Capacity: capacity,
					Size:     size,
				})
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
	if os := ovfVirtualSystem.OperatingSystem; os != nil {
		osInfo := &imageStatus.OSInfo
		osInfo.ID = strconv.Itoa(int(os.ID))
		if os.Version != nil {
			osInfo.Version = *os.Version
		}
		if os.OSType != nil {
			osInfo.Type = *os.OSType
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

	imageStatus.OVFProperties = nil
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

	imageStatus.VMwareSystemProperties = nil
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

func populateImageStatusFromOVFDiskSection(imageStatus *vmopv1.VirtualMachineImageStatus, diskSection *ovf.DiskSection) {
	imageStatus.Disks = make([]vmopv1.VirtualMachineImageDiskInfo, len(diskSection.Disks))

	for i, disk := range diskSection.Disks {
		diskDetail := vmopv1.VirtualMachineImageDiskInfo{}
		if disk.PopulatedSize != nil {
			populatedSize := int64(*disk.PopulatedSize)
			diskDetail.Size = resource.NewQuantity(populatedSize, getQuantityFormat(populatedSize))
		}

		capacity, _ := strconv.ParseInt(disk.Capacity, 10, 64)
		if capacityAllocationUnits := disk.CapacityAllocationUnits; capacityAllocationUnits != nil && capacity != 0 {
			bytesMultiplier := ovf.ParseCapacityAllocationUnits(*capacityAllocationUnits)
			capacity *= bytesMultiplier
		}
		diskDetail.Capacity = resource.NewQuantity(capacity, getQuantityFormat(capacity))

		imageStatus.Disks[i] = diskDetail
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

// getQuantityFormat returns the appropriate resource.Format based upon the divisibility of the input val.
func getQuantityFormat(val int64) resource.Format {
	format := resource.DecimalSI

	if val%1024 == 0 {
		format = resource.BinarySI
	}
	return format
}
