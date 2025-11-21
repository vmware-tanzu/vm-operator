// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"errors"
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
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/virtualcontroller"
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

// GetDeviceConfigSpec returns the VirtualDeviceConfigSpec from the given
// BaseVirtualDeviceConfigSpec. Returns nil if devChange is nil, if the
// spec is nil, or if the device is nil.
func GetDeviceConfigSpec(
	devChange vimtypes.BaseVirtualDeviceConfigSpec,
) *vimtypes.VirtualDeviceConfigSpec {

	if devChange == nil {
		return nil
	}

	spec := devChange.GetVirtualDeviceConfigSpec()
	if spec == nil || spec.Device == nil {
		return nil
	}

	return spec
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

// RemoveDevicesFromConfigSpec removes devices from config spec device changes
// based on the matcher function.
func RemoveDevicesFromConfigSpec(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	fn func(vimtypes.BaseVirtualDevice) bool) {

	if configSpec == nil {
		return
	}

	var targetDevChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, devChange := range configSpec.DeviceChange {
		dSpec := devChange.GetVirtualDeviceConfigSpec()
		if dSpec == nil || dSpec.Device == nil {
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

// overrideOrAppendController adds a controller to configSpec, replacing any
// existing controller with the same type and bus number. If a conflict is
// detected (same ControllerType and BusNumber), the entire controller ConfigSpec
// is replaced with newCtrl. This means all controller properties are updated,
// including SCSI subtype (e.g., ParaVirtual vs LSI Logic) and sharing mode.
// The new controller's device key is used, and any disks attached to the old
// controller will be discarded. It returns the overridden controller's device
// key, or 0 if no override occurred.
func overrideOrAppendController(
	dst *vimtypes.VirtualMachineConfigSpec,
	newCtrl vimtypes.BaseVirtualDevice,
	existingCtrlIdx map[ControllerID]int) int32 {

	newCtrlID := GetControllerID(newCtrl)
	if newCtrlID == nil || dst == nil {
		return 0
	}

	if idx, exists := existingCtrlIdx[*newCtrlID]; exists {
		oldSpec := GetDeviceConfigSpec(dst.DeviceChange[idx])
		if oldSpec == nil {
			return 0
		}

		dst.DeviceChange[idx] = &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    newCtrl,
		}
		return oldSpec.Device.GetVirtualDevice().Key
	}

	dst.DeviceChange = append(dst.DeviceChange,
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    newCtrl,
		})
	existingCtrlIdx[*newCtrlID] = len(dst.DeviceChange) - 1
	return 0
}

// CopyStorageControllersAndDisksWithOverride copies storage controllers and
// disks from src to dst with conflict resolution. Controllers from src override
// dst controllers with matching type and bus number. When a dst controller is
// overridden, disks attached to it are discarded.
func CopyStorageControllersAndDisksWithOverride(
	ctx context.Context,
	dst *vimtypes.VirtualMachineConfigSpec,
	src vimtypes.VirtualMachineConfigSpec,
	storagePolicyID string,
	removeUnusedControllersFromSrc bool) {

	if dst == nil {
		return
	}

	logger := pkglog.FromContextOrDefault(ctx)

	existingCtrlIdx := map[ControllerID]int{}
	for idx, devChange := range dst.DeviceChange {
		spec := GetDeviceConfigSpec(devChange)
		if spec == nil ||
			spec.Operation != vimtypes.VirtualDeviceConfigSpecOperationAdd {
			continue
		}
		if ctrlID := GetControllerID(spec.Device); ctrlID != nil {
			existingCtrlIdx[*ctrlID] = idx
		}
	}

	var (
		removedDstCtrlKeys = map[int32]struct{}{}
		srcCtrlKeys        = map[int32]struct{}{}
		ctrlKeysWithDisks  = map[int32]struct{}{}
	)

	for _, devChange := range src.DeviceChange {
		srcSpec := GetDeviceConfigSpec(devChange)
		if srcSpec == nil ||
			srcSpec.Operation != vimtypes.VirtualDeviceConfigSpecOperationAdd {
			continue
		}

		if ctrlID := GetControllerID(srcSpec.Device); ctrlID != nil {
			srcKey := srcSpec.Device.GetVirtualDevice().Key
			removedKey := overrideOrAppendController(
				dst,
				srcSpec.Device,
				existingCtrlIdx)

			if removedKey != 0 {
				removedDstCtrlKeys[removedKey] = struct{}{}
				logger.Info("Controller overridden",
					"type", ctrlID.ControllerType,
					"bus", ctrlID.BusNumber,
					"removedKey", removedKey,
					"newKey", srcKey)
			}
			srcCtrlKeys[srcKey] = struct{}{}
			continue
		}

		if disk, ok := srcSpec.Device.(*vimtypes.VirtualDisk); ok {
			ctrlKeysWithDisks[disk.ControllerKey] = struct{}{}

			dstSpec := &vimtypes.VirtualDeviceConfigSpec{
				Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
				Device:        disk,
			}
			if storagePolicyID != "" {
				dstSpec.Profile = []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: storagePolicyID,
					},
				}
			}
			dst.DeviceChange = append(dst.DeviceChange, dstSpec)
		}
	}

	RemoveDevicesFromConfigSpec(dst, func(dev vimtypes.BaseVirtualDevice) bool {
		switch d := dev.(type) {
		case *vimtypes.VirtualDisk:
			_, removed := removedDstCtrlKeys[d.ControllerKey]
			if removed {
				logger.Info("Disk removed from dst due to controller override",
					"diskKey", d.Key,
					"controllerKey", d.ControllerKey)
			}
			return removed
		case vimtypes.BaseVirtualController:
			if !removeUnusedControllersFromSrc {
				return false
			}
			key := d.GetVirtualController().Key
			_, isFromSrc := srcCtrlKeys[key]
			_, hasDisks := ctrlKeysWithDisks[key]
			shouldRemove := isFromSrc && !hasDisks
			if shouldRemove {
				if ctrlID := GetControllerID(dev); ctrlID != nil {
					logger.Info("Unused controller removed",
						"type", ctrlID.ControllerType,
						"bus", ctrlID.BusNumber,
						"key", key)
				}
			}
			return shouldRemove
		default:
			return false
		}
	})
}

// AddControllersFromVMSpec adds user-defined controllers from vm.Spec.Hardware
// to dst, overriding any existing controllers with the same type and bus number.
func AddControllersFromVMSpec(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	dst *vimtypes.VirtualMachineConfigSpec) {

	if vm == nil || dst == nil || vm.Spec.Hardware == nil {
		return
	}

	var (
		pciCtrl   *vimtypes.VirtualPCIController
		newDevKey int32
	)

	for _, devChange := range dst.DeviceChange {
		spec := GetDeviceConfigSpec(devChange)
		if spec == nil || spec.Operation ==
			vimtypes.VirtualDeviceConfigSpecOperationRemove {
			continue
		}

		if pciCtrl == nil {
			if pci, ok := spec.Device.(*vimtypes.VirtualPCIController); ok {
				pciCtrl = pci
			}
		}

		newDevKey = min(newDevKey, spec.Device.GetVirtualDevice().Key)
	}

	if pciCtrl == nil {
		return
	}

	src := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{},
	}
	newDevKey--

	for _, spec := range vm.Spec.Hardware.IDEControllers {
		virtualcontroller.AddDeviceChangeOp(&src,
			virtualcontroller.NewIDEController(spec, pciCtrl, &newDevKey),
			vimtypes.VirtualDeviceConfigSpecOperationAdd)
	}
	for _, spec := range vm.Spec.Hardware.NVMEControllers {
		virtualcontroller.AddDeviceChangeOp(&src,
			virtualcontroller.NewNVMEController(ctx, spec, pciCtrl, &newDevKey),
			vimtypes.VirtualDeviceConfigSpecOperationAdd)
	}
	for _, spec := range vm.Spec.Hardware.SATAControllers {
		virtualcontroller.AddDeviceChangeOp(&src,
			virtualcontroller.NewSATAController(spec, pciCtrl, &newDevKey),
			vimtypes.VirtualDeviceConfigSpecOperationAdd)
	}
	for _, spec := range vm.Spec.Hardware.SCSIControllers {
		virtualcontroller.AddDeviceChangeOp(&src,
			virtualcontroller.NewSCSIController(
				ctx, spec, pciCtrl, &newDevKey).(vimtypes.BaseVirtualDevice),
			vimtypes.VirtualDeviceConfigSpecOperationAdd)
	}

	CopyStorageControllersAndDisksWithOverride(ctx, dst, src, "", false)
}
