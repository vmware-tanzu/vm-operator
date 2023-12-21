// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8syaml "sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// BackupVirtualMachineOptions contains the options for BackupVirtualMachine.
type BackupVirtualMachineOptions struct {
	VMCtx               context.VirtualMachineContextA2
	VcVM                *object.VirtualMachine
	DiskUUIDToPVC       map[string]corev1.PersistentVolumeClaim
	AdditionalResources []client.Object
}

// PVCDiskData contains the data of a disk attached to VM backed by a PVC.
type PVCDiskData struct {
	// Filename contains the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// BackupVirtualMachine backs up the required data of a VM into its ExtraConfig.
// Currently, the following data is backed up:
// - VM Kubernetes resource in YAMl.
// - Additional VM-relevant Kubernetes resources in YAML, separated by "---".
// - Cloud-Init instance ID (if not already stored in ExtraConfig).
// - PVC disk data in JSON format (if DiskUUIDToPVC is not empty).
func BackupVirtualMachine(opts BackupVirtualMachineOptions) error {
	var moVM mo.VirtualMachine
	if err := opts.VcVM.Properties(opts.VMCtx, opts.VcVM.Reference(),
		[]string{"config.extraConfig"}, &moVM); err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get VM properties for backup")
		return err
	}

	curEcMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)
	ecToUpdate := []types.BaseOptionValue{}

	vmYAML, err := getDesiredResourceYAMLForBackup(
		[]client.Object{opts.VMCtx.VM},
		vmopv1.VMResourceYAMLExtraConfigKey,
		curEcMap,
	)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get VM resource yaml for backup")
		return err
	}
	if vmYAML == "" {
		opts.VMCtx.Logger.Info("Skipping VM resource yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.VMResourceYAMLExtraConfigKey,
			Value: vmYAML,
		})
	}

	additionalYAML, err := getDesiredResourceYAMLForBackup(
		opts.AdditionalResources,
		vmopv1.AdditionalResourcesYAMLExtraConfigKey,
		curEcMap,
	)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get additional resources yaml for backup")
		return err
	}

	if additionalYAML == "" {
		opts.VMCtx.Logger.Info("Skipping additional resources yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
			Value: additionalYAML,
		})
	}

	instanceID, err := getDesiredCloudInitInstanceIDForBackup(opts.VMCtx.VM, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get cloud-init instance ID for backup")
		return err
	}

	if instanceID == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping cloud-init instance ID as already stored")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.CloudInitInstanceIDExtraConfigKey,
			Value: instanceID,
		})
	}

	pvcDiskData, err := getDesiredPVCDiskDataForBackup(opts, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get PVC disk data for backup")
		return err
	}

	if pvcDiskData == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping PVC disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.PVCDiskDataExtraConfigKey,
			Value: pvcDiskData,
		})
	}

	if len(ecToUpdate) != 0 {
		opts.VMCtx.Logger.Info("Updating VM ExtraConfig with latest backup data")
		opts.VMCtx.Logger.V(4).Info("", "ExtraConfigToUpdate", ecToUpdate)
		if _, err := opts.VcVM.Reconfigure(opts.VMCtx, types.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}); err != nil {
			opts.VMCtx.Logger.Error(err, "failed to update VM ExtraConfig with latest backup data")
			return err
		}
	}

	return nil
}

// getDesiredResourceYAMLForBackup returns the encoded and gzipped YAML of the
// given resources, or an empty string if the data is unchanged in ExtraConfig.
func getDesiredResourceYAMLForBackup(
	resources []client.Object,
	ecKey string,
	ecMap map[string]string) (string, error) {
	// Check if the given resources are up-to-date with the latest backup.
	// This is done by comparing the resource versions of each object UID.
	var isLatestBackup bool
	if ecKubeData, ok := ecMap[ecKey]; ok {
		if resourceToVersion := tryGetResourceVersion(ecKubeData); resourceToVersion != nil {
			fmt.Println("resourceToVersion", resourceToVersion)
			isLatestBackup = true
			for _, curObj := range resources {
				if curObj.GetResourceVersion() != resourceToVersion[string(curObj.GetUID())] {
					isLatestBackup = false
					break
				}
			}
		}
	}

	// All resources are up-to-date, return an empty string to skip the backup.
	if isLatestBackup {
		return "", nil
	}

	// Backup the given resources YAML with encoding and compression.
	// Use "---" as the separator if more than one resource is passed.
	marshaledStrs := []string{}
	for _, res := range resources {
		marshaledYaml, err := k8syaml.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %q: %v", res.GetName(), err)
		}
		marshaledStrs = append(marshaledStrs, string(marshaledYaml))
	}

	resourcesYAML := strings.Join(marshaledStrs, "\n---\n")
	return util.EncodeGzipBase64(resourcesYAML)
}

// tryGetResourceVersion tries to get the resource version of each object in
// the encoded and gzipped YAML. Returns a map of resource UID to version.
func tryGetResourceVersion(ecResourcesYAML string) map[string]string {
	decoded, err := util.TryToDecodeBase64Gzip([]byte(ecResourcesYAML))
	if err != nil {
		return nil
	}

	resourceVersions := map[string]string{}
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	resourcesYAML := strings.Split(decoded, "---\n")
	for _, resYAML := range resourcesYAML {
		fmt.Println("resYAML", resYAML)
		if resYAML != "" {
			resYAML = strings.TrimSpace(resYAML)
			res := &unstructured.Unstructured{}
			_, _, err := decUnstructured.Decode([]byte(resYAML), nil, res)
			if err != nil {
				fmt.Println("err", err)
				continue
			}
			resourceVersions[string(res.GetUID())] = res.GetResourceVersion()
		}
	}

	return resourceVersions
}

func getDesiredCloudInitInstanceIDForBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// Cloud-Init instance ID should not be changed once persisted in VM's
	// ExtraConfig. Return an empty string to skip the backup if it exists.
	if _, ok := ecMap[vmopv1.CloudInitInstanceIDExtraConfigKey]; ok {
		return "", nil
	}

	instanceID := vm.Annotations[vmopv1.InstanceIDAnnotation]
	if instanceID == "" {
		instanceID = string(vm.UID)
	}

	return util.EncodeGzipBase64(instanceID)
}

func getDesiredPVCDiskDataForBackup(
	opts BackupVirtualMachineOptions,
	ecMap map[string]string) (string, error) {
	// Return an empty string to skip backup if no disk uuid to PVC is specified.
	if len(opts.DiskUUIDToPVC) == 0 {
		return "", nil
	}

	deviceList, err := opts.VcVM.Device(opts.VMCtx)
	if err != nil {
		return "", err
	}

	var diskData []PVCDiskData
	for _, device := range deviceList.SelectByType((*types.VirtualDisk)(nil)) {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if b, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				if pvc, ok := opts.DiskUUIDToPVC[b.Uuid]; ok {
					diskData = append(diskData, PVCDiskData{
						FileName:    b.FileName,
						PVCName:     pvc.Name,
						AccessModes: pvc.Spec.AccessModes,
					})
				}
			}
		}
	}

	diskDataJSON, err := json.Marshal(diskData)
	if err != nil {
		return "", err
	}
	diskDataBackup, err := util.EncodeGzipBase64(string(diskDataJSON))
	if err != nil {
		return "", err
	}

	// Return an empty string to skip the backup if the data is unchanged.
	if diskDataBackup == ecMap[vmopv1.PVCDiskDataExtraConfigKey] {
		return "", nil
	}

	return diskDataBackup, nil
}
