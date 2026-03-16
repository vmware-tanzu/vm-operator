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

	configSpec, err := ovfEnvelope.ToConfigSpec()
	if err != nil {
		return fmt.Errorf("failed to transform ovf to configSpec: %w", err)
	}

	var status *vmopv1.VirtualMachineImageStatus

	switch vmi := obj.(type) {
	case *vmopv1.VirtualMachineImage:
		status = &vmi.Status
	case *vmopv1.ClusterVirtualMachineImage:
		status = &vmi.Status
	default:
		return nil
	}

	// Populate the firmware.
	status.Firmware = configSpec.Firmware

	// Populate the guest info.
	status.OSInfo.Type = configSpec.GuestId
	status.OSInfo.ID = strconv.Itoa(int(ovf.GuestIDToCIMOSType(
		configSpec.GuestId)))

	// Populate the hardware version.
	version, err := vimtypes.ParseHardwareVersion(configSpec.Version)
	if err != nil {
		version = 0
	}
	if version > 0 {
		status.HardwareVersion = ptr.To(int32(version))
	}

	if bvac := configSpec.VAppConfig; bvac != nil {
		if vac := bvac.GetVmConfigSpec(); vac != nil {

			// Use info from the first product section in the VM image,
			// if one exists.
			if product := vac.Product; len(product) > 0 {
				if info := product[0].Info; info != nil {
					status.ProductInfo.Vendor = info.Vendor
					status.ProductInfo.Product = info.Name
					status.ProductInfo.Version = info.Version
					status.ProductInfo.FullVersion = info.FullVersion
				}
			}

			status.OVFProperties = nil
			status.VMwareSystemProperties = nil

			for _, prop := range vac.Property {
				if info := prop.Info; info != nil {

					if strings.HasPrefix(info.Id, "vmware-system") {
						status.VMwareSystemProperties = append(
							status.VMwareSystemProperties,
							common.KeyValuePair{
								Key:   info.Id,
								Value: info.DefaultValue,
							})

					} else if info.UserConfigurable != nil &&
						*info.UserConfigurable {

						var defVal *string
						if info.DefaultValue != "" {
							defVal = &info.DefaultValue
						}

						status.OVFProperties = append(
							status.OVFProperties,
							vmopv1.OVFProperty{
								Key:     info.Id,
								Type:    info.Type,
								Default: defVal,
							})
					}
				}
			}
		}
	}

	// Populate the disk information.
	usedDiskNames := make(map[string]int)
	status.Disks = nil
	devices := pkgutil.DevicesFromConfigSpec(&configSpec)
	controllers := getControllerMapFromVM(devices...)
	for _, bdc := range configSpec.DeviceChange {
		if bdc != nil {
			if dc := bdc.GetVirtualDeviceConfigSpec(); dc != nil {
				if d, ok := dc.Device.(*vimtypes.VirtualDisk); ok {
					diskName := d.DeviceInfo.GetDescription().Label
					// Ensure disk name is unique in status.Disks; append an
					// incremented suffix if the name is already present.
					suffix := 0
					if currSuffix, exists := usedDiskNames[diskName]; exists {
						suffix = currSuffix + 1
						diskName = fmt.Sprintf("%s_%d", diskName, suffix)
					}
					usedDiskNames[diskName] = suffix

					sdi := vmopv1.VirtualMachineImageDiskInfo{
						Name:       diskName,
						Limit:      kubeutil.BytesToResource(d.CapacityInBytes),
						Requested:  kubeutil.BytesToResource(d.CapacityInBytes),
						UnitNumber: d.UnitNumber,
					}

					if c, ok := controllers[d.ControllerKey]; ok {
						sdi.ControllerType = c.typ3
						sdi.ControllerBusNumber = c.bus
					}

					status.Disks = append(status.Disks, sdi)
				}
			}
		}
	}

	initVSpherePolicyLabelsFromVMExtraConfig(obj, configSpec.ExtraConfig)

	if setter, ok := obj.(conditions.Setter); ok {
		if isVMV1Alpha1Compatible(configSpec.ExtraConfig) {
			conditions.MarkTrue(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
		} else {
			conditions.Delete(setter, vmopv1.VirtualMachineImageV1Alpha1CompatibleCondition)
		}
	}

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
					Requested:  kubeutil.BytesToResource(di.CapacityInBytes),
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

// isVMV1Alpha1Compatible checks if the image has VMOperatorV1Alpha1ExtraConfigKey
// set to VMOperatorV1Alpha1ConfigReady in its ExtraConfig.
func isVMV1Alpha1Compatible(extraConfig object.OptionValueList) bool {
	v, _ := extraConfig.GetString(constants.VMOperatorV1Alpha1ExtraConfigKey)
	return v == constants.VMOperatorV1Alpha1ConfigReady
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
