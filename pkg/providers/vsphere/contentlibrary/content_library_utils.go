// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vmdk"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// UpdateVmiWithOvfEnvelope updates the given VMI object from an OVF envelope.
func UpdateVmiWithOvfEnvelope(
	obj client.Object,
	ovfEnvelope ovf.Envelope) error {

	var status *vmopv1.VirtualMachineImageStatus

	switch vmi := obj.(type) {
	case *vmopv1.VirtualMachineImage:
		status = &vmi.Status
	case *vmopv1.ClusterVirtualMachineImage:
		status = &vmi.Status
	default:
		return nil
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

	if ovfEnvelope.Disk != nil &&
		ovfEnvelope.VirtualSystem != nil &&
		len(ovfEnvelope.VirtualSystem.VirtualHardware) > 0 {

		if err := populateImageStatusFromOVFDiskSection(
			status,
			ovfEnvelope.VirtualSystem.VirtualHardware[0],
			ovfEnvelope.Disk.Disks); err != nil {

			return fmt.Errorf(
				"failed to populate image status from ovf disk section: %w",
				err)
		}
	} else {
		status.Disks = nil
	}

	initVSpherePolicyLabelsFromOVFExtraConfig(obj, ovfEnvelope.VirtualSystem)

	return nil
}

// UpdateVmiWithVirtualMachine updates the given VMI object from a
// mo.VirtualMachine data structure.
//
//nolint:gocyclo
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
	status.OSInfo.Type = vm.Config.GuestId
	status.OSInfo.ID = strconv.Itoa(int(ovf.GuestIDToCIMOSType(
		vm.Config.GuestId)))

	// Populate the hardware version.
	version, err := vimtypes.ParseHardwareVersion(vm.Config.Version)
	if err != nil {
		version = 0
	}
	if version > 0 {
		status.HardwareVersion = ptr.To(int32(version))
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

	controllers := getControllerMapFromVM(vm.Config.Hardware.Device...)

	// Populate the disk information.
	status.Disks = nil
	for _, bd := range vm.Config.Hardware.Device {
		if disk, ok := bd.(*vimtypes.VirtualDisk); ok {

			if info := pkgutil.GetVirtualDiskInfo(disk); info.UUID != "" {
				di, _ := vmdk.GetVirtualDiskInfoByUUID(
					ctx,
					nil,   /* client is nil since props are not re-fetched */
					vm,    /* use props from this object */
					false, /* do not refetch props */
					false, /* include disks related to snapshots */
					info.UUID)

				sdi := vmopv1.VirtualMachineImageDiskInfo{
					Name:       info.UUID,
					Limit:      kubeutil.BytesToResource(di.CapacityInBytes),
					Requested:  kubeutil.BytesToResource(di.Size),
					UnitNumber: info.UnitNumber,
				}

				if c, ok := controllers[info.ControllerKey]; ok {
					sdi.ControllerType = c.typ3
					sdi.ControllerBusNumber = c.bus
				}

				status.Disks = append(status.Disks, sdi)
			}
		}
	}

	initVSpherePolicyLabelsFromVMExtraConfig(obj, vm.Config.ExtraConfig)

	if setter, ok := obj.(conditions.Setter); ok {
		if isVMV1Alpha1Compatible(vm.Config.ExtraConfig) {
			conditions.MarkTrue(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
		} else {
			conditions.Delete(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
		}
	}
}

type vmControllerSpec struct {
	bus  *int32
	typ3 vmopv1.VirtualControllerType
}

func getControllerMapFromVM(
	devices ...vimtypes.BaseVirtualDevice) map[int32]vmControllerSpec {

	controllers := map[int32]vmControllerSpec{}

	for _, bd := range devices {
		if bc, ok := bd.(vimtypes.BaseVirtualController); ok {
			var c *vmControllerSpec

			switch bd.(type) {
			case *vimtypes.VirtualIDEController:
				c = &vmControllerSpec{
					typ3: vmopv1.VirtualControllerTypeIDE,
				}
			case vimtypes.BaseVirtualSCSIController:
				c = &vmControllerSpec{
					typ3: vmopv1.VirtualControllerTypeSCSI,
				}
			case vimtypes.BaseVirtualSATAController:
				c = &vmControllerSpec{
					typ3: vmopv1.VirtualControllerTypeSATA,
				}
			case *vimtypes.VirtualNVMEController:
				c = &vmControllerSpec{
					typ3: vmopv1.VirtualControllerTypeNVME,
				}
			}

			if c != nil {
				vc := bc.GetVirtualController()
				c.bus = &vc.BusNumber
				controllers[vc.Key] = *c
			}
		}
	}

	return controllers
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
			version, err := vimtypes.ParseHardwareVersion(*sys.VirtualSystemType)
			if err != nil {
				version = 0
			}
			if version > 0 {
				imageStatus.HardwareVersion = ptr.To(int32(version))
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

type ovfControllerSpec struct {
	bus  *int32
	typ3 vmopv1.VirtualControllerType
}

func getControllerMapFromOVF(
	hardware ovf.VirtualHardwareSection) (map[string]ovfControllerSpec, error) {

	controllers := map[string]ovfControllerSpec{}

	for _, i := range hardware.Item {
		var c *ovfControllerSpec
		if rt := i.ResourceType; rt != nil {
			switch *rt {
			case ovf.IdeController:
				c = &ovfControllerSpec{
					typ3: vmopv1.VirtualControllerTypeIDE,
				}
			case ovf.ParallelScsiHba:
				c = &ovfControllerSpec{
					typ3: vmopv1.VirtualControllerTypeSCSI,
				}
			case ovf.OtherStorage:
				if rst := i.ResourceSubType; rst != nil {
					switch *rst {
					case ovf.ResourceSubTypeSATAAHCI,
						ovf.ResourceSubTypeSATAAHCIAlter:
						c = &ovfControllerSpec{
							typ3: vmopv1.VirtualControllerTypeSATA,
						}
					case ovf.ResourceSubTypeNVMEController:
						c = &ovfControllerSpec{
							typ3: vmopv1.VirtualControllerTypeNVME,
						}
					}
				}
			}
		}
		if c != nil {
			if a := i.Address; a != nil && *a != "" {
				v, err := strconv.ParseInt(*a, 10, 32)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to parse bus number: %s", *a)
				}
				c.bus = ptr.To(int32(v))
			}
			controllers[i.InstanceID] = *c
		}
	}

	return controllers, nil
}

func populateImageStatusFromOVFDiskSection(
	status *vmopv1.VirtualMachineImageStatus,
	hardware ovf.VirtualHardwareSection,
	disks []ovf.VirtualDiskDesc) error {

	status.Disks = nil

	controllers, err := getControllerMapFromOVF(hardware)
	if err != nil {
		return fmt.Errorf(
			"failed to build controller map from ovf hardware: %w", err)
	}

	diskIDToDisk := map[string]ovf.VirtualDiskDesc{}
	for _, d := range disks {
		diskIDToDisk[d.DiskID] = d
	}

	for _, i := range hardware.Item {
		if i.ResourceType != nil && *i.ResourceType == ovf.DiskDrive {

			diskName := i.ElementName

			if len(i.HostResource) > 0 {

				diskID := strings.TrimPrefix(i.HostResource[0], "ovf:/disk/")

				if d, ok := diskIDToDisk[diskID]; ok {
					di := vmopv1.VirtualMachineImageDiskInfo{
						// Set the name of the disk in the VMI's status.diskInfo
						// section to ElementName. This is the same value that
						// is placed in the VirtualDisk's Description.Label
						// field when the OVF is converted into a ConfigSpec.
						// This allows users to map the disk from the ConfigSpec
						// to the disk in the VMI.
						Name: diskName,
					}

					if a := i.AddressOnParent; a != nil {
						v, err := strconv.ParseInt(*a, 10, 32)
						if err != nil {
							return fmt.Errorf(
								"failed to parse unit number: %s", *a)
						}
						di.UnitNumber = ptr.To(int32(v))
					}

					if p := i.Parent; p != nil {
						if c, ok := controllers[*p]; ok {
							di.ControllerType = c.typ3
							di.ControllerBusNumber = c.bus
						}
					}

					if d.PopulatedSize != nil {
						populatedSize := int64(*d.PopulatedSize)
						di.Requested = kubeutil.BytesToResource(populatedSize)
					}

					// Parse the disk capacity.
					if c, _ := strconv.ParseInt(d.Capacity, 10, 64); c != 0 {
						if u := d.CapacityAllocationUnits; u != nil {
							m := ovf.ParseCapacityAllocationUnits(*u)
							c *= m
						}
						di.Limit = kubeutil.BytesToResource(c)
					}

					status.Disks = append(status.Disks, di)
				}
			}
		}
	}

	return nil
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
// set to VMOperatorV1Alpha1ConfigReady in its ExtraConfig.
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

// isVMV1Alpha1Compatible checks if the image has VMOperatorV1Alpha1ExtraConfigKey
// set to VMOperatorV1Alpha1ConfigReady in its ExtraConfig.
func isVMV1Alpha1Compatible(extraConfig object.OptionValueList) bool {
	v, _ := extraConfig.GetString(constants.VMOperatorV1Alpha1ExtraConfigKey)
	return v == constants.VMOperatorV1Alpha1ConfigReady
}

func initVSpherePolicyLabelsFromOVFExtraConfig(
	obj client.Object,
	ovfVirtualSystem *ovf.VirtualSystem) {

	if ovfVirtualSystem != nil {
		for _, vh := range ovfVirtualSystem.VirtualHardware {
			for _, ec := range vh.ExtraConfig {
				if ec.Key == pkgconst.VirtualMachineImageExtraConfigLabelsKey {
					if ec.Value != "" {
						ecLabels := strings.Split(ec.Value, ",")
						objLabels := obj.GetLabels()
						if objLabels == nil {
							objLabels = map[string]string{}
						}
						for _, kv := range ecLabels {
							kvParts := strings.Split(kv, ":")
							switch len(kvParts) {
							case 1:
								objLabels[kvParts[0]] = ""
							case 2:
								objLabels[kvParts[0]] = kvParts[1]
							}
						}
						obj.SetLabels(objLabels)
					}
				}
			}
		}
	}
}

func initVSpherePolicyLabelsFromVMExtraConfig(
	obj client.Object,
	extraConfig object.OptionValueList) {

	if v, _ := extraConfig.GetString(
		pkgconst.VirtualMachineImageExtraConfigLabelsKey); v != "" {

		ecLabels := strings.Split(v, ",")
		objLabels := obj.GetLabels()
		if objLabels == nil {
			objLabels = map[string]string{}
		}
		for _, kv := range ecLabels {
			kvParts := strings.Split(kv, ":")
			switch len(kvParts) {
			case 1:
				objLabels[kvParts[0]] = ""
			case 2:
				objLabels[kvParts[0]] = kvParts[1]
			}
		}
		obj.SetLabels(objLabels)
	}
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
