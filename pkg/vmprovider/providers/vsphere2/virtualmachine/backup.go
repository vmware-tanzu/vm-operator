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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// BackupVirtualMachineOptions contains the options for BackupVirtualMachine.
type BackupVirtualMachineOptions struct {
	VMCtx         context.VirtualMachineContextA2
	VcVM          *object.VirtualMachine
	KubeObjects   []client.Object
	DiskUUIDToPVC map[string]corev1.PersistentVolumeClaim
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
// - VM relevant Kubernetes objects in YAML separated by "---".
// - Cloud-Init instance ID (if not already stored in ExtraConfig).
// - PVC disk data in JSON format (if DiskUUIDToPVC is not empty).
func BackupVirtualMachine(opts BackupVirtualMachineOptions) error {
	var moVM mo.VirtualMachine
	if err := opts.VcVM.Properties(opts.VMCtx, opts.VcVM.Reference(),
		[]string{"config.extraConfig"}, &moVM); err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get VM properties for backup")
		return err
	}

	curEcMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)
	ecToUpdate := []types.BaseOptionValue{}

	objectsYAML, err := getDesiredKubeObjectsYAMLForBackup(opts.KubeObjects, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get kube objects yaml for backup")
		return err
	}

	if objectsYAML == "" {
		opts.VMCtx.Logger.Info("Skipping kube objects yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey,
			Value: objectsYAML,
		})
	}

	instanceID, err := getDesiredCloudInitInstanceIDForBackup(opts.VMCtx.VM, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get cloud-init instance ID for backup")
		return err
	}

	if instanceID == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping cloud-init instance ID as already stored")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.VMBackupCloudInitInstanceIDExtraConfigKey,
			Value: instanceID,
		})
	}

	pvcDiskData, err := getDesiredPVCDiskDataForBackup(opts, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get PVC disk data for backup")
		return err
	}

	if pvcDiskData == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping PVC disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.VMBackupPVCDiskDataExtraConfigKey,
			Value: pvcDiskData,
		})
	}

	if len(ecToUpdate) != 0 {
		opts.VMCtx.Logger.Info("Updating VM ExtraConfig with latest backup data")
		opts.VMCtx.Logger.V(4).Info("", "ExtraConfigToUpdate", ecToUpdate)
		if _, err := opts.VcVM.Reconfigure(opts.VMCtx, types.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}); err != nil {
			opts.VMCtx.Logger.Error(err, "Failed to update VM ExtraConfig with latest backup data")
			return err
		}
	}

	return nil
}

func getDesiredKubeObjectsYAMLForBackup(
	kubeObjects []client.Object,
	ecMap map[string]string) (string, error) {
	// Check if the VM's ExtraConfig already contains the latest version of all
	// objects, determined by each resource UID to resource version mapping.
	var isLatestBackup bool
	if ecKubeData, ok := ecMap[vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey]; ok {
		if resourceToVersion := tryGetResourceVersion(ecKubeData); resourceToVersion != nil {
			isLatestBackup = true
			for _, curObj := range kubeObjects {
				if curObj.GetResourceVersion() != resourceToVersion[string(curObj.GetUID())] {
					isLatestBackup = false
					break
				}
			}
		}
	}

	// All objects are up-to-date, return an empty string to skip the backup.
	if isLatestBackup {
		return "", nil
	}

	// Backup the latest version of all objects with encoded and gzipped YAML.
	// Use "---" as the separator between objects to allow for easy parsing.
	marshaledStrs := []string{}
	for _, obj := range kubeObjects {
		marshaledYaml, err := yaml.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %s: %v", obj.GetName(), err)
		}
		marshaledStrs = append(marshaledStrs, string(marshaledYaml))
	}

	kubeObjectsYAML := strings.Join(marshaledStrs, "---\n")
	return util.EncodeGzipBase64(kubeObjectsYAML)
}

func tryGetResourceVersion(ecKubeObjectsYAML string) map[string]string {
	decoded, err := util.TryToDecodeBase64Gzip([]byte(ecKubeObjectsYAML))
	if err != nil {
		return nil
	}

	resourceVersions := map[string]string{}
	kubeObjectsYAML := strings.Split(decoded, "---\n")
	for _, objYAML := range kubeObjectsYAML {
		if objYAML != "" {
			objYAML = strings.TrimSpace(objYAML)
			var obj map[string]interface{}
			if err := yaml.Unmarshal([]byte(objYAML), &obj); err != nil {
				continue
			}
			metadata, ok := obj["metadata"].(map[string]interface{})
			if !ok {
				continue
			}
			uid, ok := metadata["uid"].(string)
			if !ok {
				continue
			}
			resourceVersion, ok := metadata["resourceVersion"].(string)
			if !ok {
				continue
			}
			resourceVersions[uid] = resourceVersion
		}
	}

	return resourceVersions
}

func getDesiredCloudInitInstanceIDForBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// Cloud-Init instance ID should not be changed once persisted in VM's
	// ExtraConfig. Return an empty string to skip the backup if it exists.
	if _, ok := ecMap[vmopv1.VMBackupCloudInitInstanceIDExtraConfigKey]; ok {
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
	if diskDataBackup == ecMap[vmopv1.VMBackupPVCDiskDataExtraConfigKey] {
		return "", nil
	}

	return diskDataBackup, nil
}
