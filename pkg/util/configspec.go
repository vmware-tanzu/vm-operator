// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/virtualcontroller"
)

const (
	scsiControllerSharedBusMismatchFmt = "SCSI controller at bus %d: sharing mode conflict (%q vs %q)"
	scsiControllerSubtypeMismatchFmt   = "SCSI controller at bus %d: type conflict (%q vs %q)"
	scsiControllerUnsupportedTypeFmt   = "SCSI controller at bus %d: unsupported controller type"
	nvmeControllerSharedBusMismatchFmt = "NVME controller at bus %d: sharing mode conflict (%q vs %q)"
	ControllerConflictVMClassImage     = "Controller conflict between VM Class and VM Image"
	ControllerConflictVMClassImageUser = "Controller conflict between VM Class/Image and user-specified controllers"
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
		if dSpec == nil || dSpec.Device == nil {
			targetDevChanges = append(targetDevChanges, devChange)
			continue
		}

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

	minHwVersion := vimtypes.HardwareVersion(minVersion) //nolint:gosec // disable G115
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

// SafeDeviceChangesToString returns the string-ified version of the provided
// DeviceChange array by creating a minimal ConfigSpec with only the DeviceChange
// field populated. This is useful for logging when you only need to see the
// device changes rather than the entire config spec.
func SafeDeviceChangesToString(
	deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec) (s string) {

	if len(deviceChanges) == 0 {
		return "[]"
	}

	configSpec := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: deviceChanges,
	}
	return SafeConfigSpecToString(&configSpec)
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

// DatastoreFileExists returns nil if name exists in the given Datacenter.
// If the file can be checked and does not exist, os.ErrNotExist is returned.
func DatastoreFileExists(
	ctx context.Context,
	vimClient *vim25.Client,
	name string,
	datacenter *object.Datacenter) error {

	var p object.DatastorePath
	p.FromString(name)

	u := object.NewDatastoreURL(*vimClient.URL(), datacenter.InventoryPath, p.Datastore, p.Path)

	res, err := vimClient.DownloadRequest(ctx, u, &soap.Download{Method: http.MethodHead})
	if err != nil {
		return err
	}

	_ = res.Body.Close() // No Body sent with HEAD request, but still need to close

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return os.ErrNotExist
	default:
		return errors.New(res.Status)
	}
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

// getAddDevice extracts the device from a device config spec if it's an Add
// operation and the device is not nil. Returns the device, or nil if the spec
// is invalid or not an Add operation.
func getAddDevice(
	devChange vimtypes.BaseVirtualDeviceConfigSpec,
) vimtypes.BaseVirtualDevice {
	spec := devChange.GetVirtualDeviceConfigSpec()
	if spec == nil ||
		spec.Operation != vimtypes.VirtualDeviceConfigSpecOperationAdd {
		return nil
	}
	return spec.Device
}

// MergeStorageControllersAndDisks merges storage controllers and disks from
// the source ConfigSpec into the destination ConfigSpec. If a controller
// exists in both source and destination with the same bus number and type,
// the destination controller is removed and replaced with the source
// controller. Disk controller keys are automatically remapped to reference
// the correct controllers after merging. If the provided storagePolicyID is
// non-empty, it is assigned to all copied disks.
//
//   - src is usually the source with controllers with disks while dst is
//     usually one without disks (e.g. VMClass: only has non-RDM VirtualDisk
//     devices when the InstanceStorage feature is enabled).
//   - This method does not check controller conflicts within the same
//     ConfigSpec.
func MergeStorageControllersAndDisks(
	dst *vimtypes.VirtualMachineConfigSpec,
	src vimtypes.VirtualMachineConfigSpec,
	storagePolicyID string) error {

	dstCtrls := make(map[ControllerID]vimtypes.BaseVirtualDevice)
	for _, devChange := range dst.DeviceChange {
		dev := getAddDevice(devChange)
		if dev == nil {
			continue
		}

		if key, isCtrl := GetControllerIDFromDevice(dev); isCtrl {
			dstCtrls[key] = dev
		}
	}

	dstKeyToSrcKey := make(map[int32]int32)
	for _, devChange := range src.DeviceChange {
		dev := getAddDevice(devChange)
		if dev == nil {
			continue
		}

		var ctrlKey *ControllerID

		switch d := dev.(type) {
		case vimtypes.BaseVirtualSCSIController:
			srcCtrl := d.GetVirtualSCSIController()
			ctrlKey = &ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSCSI,
				BusNumber:      srcCtrl.BusNumber,
			}
			if ctrl, exists := dstCtrls[*ctrlKey]; exists {
				dstSCSIController := ctrl.(vimtypes.BaseVirtualSCSIController)
				scsiCtrl := dstSCSIController.GetVirtualSCSIController()

				if scsiCtrl.SharedBus != srcCtrl.SharedBus {
					return fmt.Errorf(
						scsiControllerSharedBusMismatchFmt,
						srcCtrl.BusNumber,
						scsiCtrl.SharedBus,
						srcCtrl.SharedBus)
				}

				// Check if controller types match
				dstCtrlType := scsiControllerTypeToVMOPType(dstSCSIController)
				srcCtrlType := scsiControllerTypeToVMOPType(d)
				if dstCtrlType == "" || srcCtrlType == "" {
					return fmt.Errorf(
						scsiControllerUnsupportedTypeFmt,
						srcCtrl.BusNumber)
				}
				if dstCtrlType != srcCtrlType {
					return fmt.Errorf(
						scsiControllerSubtypeMismatchFmt,
						srcCtrl.BusNumber,
						dstCtrlType,
						srcCtrlType)
				}
			}

		case *vimtypes.VirtualNVMEController:
			ctrlKey = &ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeNVME,
				BusNumber:      d.BusNumber,
			}
			if ctrl, exists := dstCtrls[*ctrlKey]; exists {
				nvmeCtrl := ctrl.(*vimtypes.VirtualNVMEController)
				if nvmeCtrl.SharedBus != d.SharedBus {
					return fmt.Errorf(
						nvmeControllerSharedBusMismatchFmt,
						d.BusNumber,
						nvmeCtrl.SharedBus,
						d.SharedBus)
				}
			}

		case vimtypes.BaseVirtualSATAController:
			ctrlKey = &ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      d.GetVirtualSATAController().BusNumber,
			}

		case *vimtypes.VirtualIDEController:
			ctrlKey = &ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      d.BusNumber,
			}
		}

		if ctrlKey != nil {
			if ctrl, exits := dstCtrls[*ctrlKey]; exits {
				dstKeyToSrcKey[ctrl.GetVirtualDevice().Key] = dev.GetVirtualDevice().Key
				delete(dstCtrls, *ctrlKey)
			}
		}
	}

	// Remove any controllers from dst that have duplicated controllers in src.
	RemoveDevicesFromConfigSpec(dst, func(bvd vimtypes.BaseVirtualDevice) bool {
		if ctrlKey, isCtrl := GetControllerIDFromDevice(bvd); isCtrl {
			if _, exits := dstCtrls[ctrlKey]; !exits {
				return true
			}
		}
		return false
	})

	// Update disk controller keys using dstKeyToSrcKey mapping
	for _, devChange := range dst.DeviceChange {
		spec := devChange.GetVirtualDeviceConfigSpec()
		if spec == nil || spec.Device == nil {
			continue
		}

		if spec.Operation != vimtypes.VirtualDeviceConfigSpecOperationAdd {
			continue
		}

		if disk, ok := spec.Device.(*vimtypes.VirtualDisk); ok {
			if srcCtrlKey, exists := dstKeyToSrcKey[disk.ControllerKey]; exists {
				disk.ControllerKey = srcCtrlKey
			}
		}
	}

	CopyStorageControllersAndDisks(dst, src, storagePolicyID)
	return nil
}

// findPCIController finds the PCI controller from deviceSpecs.
// A VM always has exactly one VirtualPCIController, which is the root
// controller that all other controllers attach to.
func findPCIController(
	deviceSpecs []vimtypes.BaseVirtualDeviceConfigSpec,
) *vimtypes.VirtualPCIController {

	for _, devChange := range deviceSpecs {
		dev := getAddDevice(devChange)
		if dev == nil {
			continue
		}

		if pciCtrl, ok := dev.(*vimtypes.VirtualPCIController); ok {
			return pciCtrl
		}
	}
	return nil
}

// FullyDefinedControllersToConfigSpecs converts user-specified controllers
// from the VirtualMachine's spec.hardware into a VirtualMachineConfigSpec.
// It processes SCSI, SATA, NVME, and IDE controllers, but only includes
// controllers that are fully defined: SCSI controllers require both Type and
// SharingMode to be set, while NVME controllers require SharingMode to be set.
// The function validates controller types and sharing modes, creates device
// config specs for valid controllers, and returns a ConfigSpec with only the
// DeviceChange field populated. Device keys are initialized from the provided
// configSpec. If a PCI controller exists in the configSpec, its key is used;
// otherwise, the PCI controller key defaults to 0 and vSphere will auto-assign
// it during VM creation.
func FullyDefinedControllersToConfigSpecs(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec,
) vimtypes.VirtualMachineConfigSpec {

	if vm == nil || vm.Spec.Hardware == nil {
		return vimtypes.VirtualMachineConfigSpec{}
	}

	hardware := vm.Spec.Hardware
	var newDeviceKey int32
	var userSpecControllers []vimtypes.BaseVirtualDeviceConfigSpec
	var pciKey int32 // default to 0 so it will be auto-assigned by vSphere

	virtualcontroller.InitDeviceKey(&newDeviceKey, configSpec.DeviceChange)
	if pciController := findPCIController(configSpec.DeviceChange); pciController != nil {
		pciKey = pciController.Key
	}

	// Process all controller types
	// For SCSI controllers, only add if both Type and SharingMode
	// are explicitly set.
	for _, spec := range hardware.SCSIControllers {
		if spec.Type == "" || spec.SharingMode == "" {
			continue
		}
		// Validate Type
		switch spec.Type {
		case vmopv1.SCSIControllerTypeParaVirtualSCSI,
			vmopv1.SCSIControllerTypeBusLogic,
			vmopv1.SCSIControllerTypeLsiLogic,
			vmopv1.SCSIControllerTypeLsiLogicSAS:
			// Valid type
		default:
			continue
		}
		// Validate SharingMode
		switch spec.SharingMode {
		case vmopv1.VirtualControllerSharingModeNone,
			vmopv1.VirtualControllerSharingModePhysical,
			vmopv1.VirtualControllerSharingModeVirtual:
			// Valid sharing mode
		default:
			continue
		}
		if ctrl := virtualcontroller.NewSCSIController(
			ctx, spec, pciKey, &newDeviceKey); ctrl != nil {

			userSpecControllers = append(userSpecControllers,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.
						VirtualDeviceConfigSpecOperationAdd,
					Device: ctrl.(vimtypes.BaseVirtualDevice),
				})
		}
	}

	for _, spec := range hardware.SATAControllers {
		if ctrl := virtualcontroller.NewSATAController(
			spec, pciKey, &newDeviceKey); ctrl != nil {

			userSpecControllers = append(userSpecControllers,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.
						VirtualDeviceConfigSpecOperationAdd,
					Device: ctrl,
				})
		}
	}

	// For NVME controllers, only add if SharingMode is explicitly set.
	for _, spec := range hardware.NVMEControllers {
		if spec.SharingMode == "" {
			continue
		}
		// Validate SharingMode
		switch spec.SharingMode {
		case vmopv1.VirtualControllerSharingModeNone,
			vmopv1.VirtualControllerSharingModePhysical:
			// Valid sharing mode
		default:
			continue
		}
		if ctrl := virtualcontroller.NewNVMEController(
			ctx, spec, pciKey, &newDeviceKey); ctrl != nil {

			userSpecControllers = append(userSpecControllers,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.
						VirtualDeviceConfigSpecOperationAdd,
					Device: ctrl,
				})
		}
	}

	for _, spec := range hardware.IDEControllers {
		if ctrl := virtualcontroller.NewIDEController(
			spec, pciKey, &newDeviceKey); ctrl != nil {

			userSpecControllers = append(userSpecControllers,
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.
						VirtualDeviceConfigSpecOperationAdd,
					Device: ctrl,
				})
		}
	}

	return vimtypes.VirtualMachineConfigSpec{DeviceChange: userSpecControllers}
}

// scsiControllerTypeToVMOPType converts a vSphere SCSI controller type to the
// corresponding Kubernetes API SCSIControllerType. Returns an empty string if
// the controller type is not recognized.
func scsiControllerTypeToVMOPType(
	dev vimtypes.BaseVirtualSCSIController) vmopv1.SCSIControllerType {

	switch dev.(type) {
	case *vimtypes.ParaVirtualSCSIController:
		return vmopv1.SCSIControllerTypeParaVirtualSCSI
	case *vimtypes.VirtualBusLogicController:
		return vmopv1.SCSIControllerTypeBusLogic
	case *vimtypes.VirtualLsiLogicController:
		return vmopv1.SCSIControllerTypeLsiLogic
	case *vimtypes.VirtualLsiLogicSASController:
		return vmopv1.SCSIControllerTypeLsiLogicSAS
	default:
		return ""
	}
}
