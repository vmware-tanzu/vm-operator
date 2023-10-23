// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
)

// BackupOptions contains the options for calling BackupVirtualMachine() func.
type BackupOptions struct {
	VMCtx         context.VirtualMachineContextA2
	VcVM          *object.VirtualMachine
	BootstrapData map[string]string
	DiskUUIDToPVC map[string]corev1.PersistentVolumeClaim
}

type VMDiskData struct {
	// Filename contains the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// BackupVirtualMachine backs up the required data of a VM into its ExtraConfig.
// Currently, the following data is backed up:
// - Kubernetes VirtualMachine object in YAML format (without its .status field).
// - VM bootstrap data in JSON (if provided).
// - VM disk data in JSON (if created and attached by PVCs).
func BackupVirtualMachine(opts BackupOptions) error {
	var moVM mo.VirtualMachine
	if err := opts.VcVM.Properties(opts.VMCtx, opts.VcVM.Reference(),
		[]string{"config.extraConfig"}, &moVM); err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get VM properties for backup")
		return err
	}
	curEcMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)

	var ecToUpdate []types.BaseOptionValue

	vmKubeDataBackup, err := getDesiredVMKubeDataForBackup(opts.VMCtx.VM, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get VM kube data for backup")
		return err
	}

	if vmKubeDataBackup == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping VM kube data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMKubeDataExtraConfigKey,
			Value: vmKubeDataBackup,
		})
	}

	instanceIDBackup, err := getDesiredCloudInitInstanceIDForBackup(opts.VMCtx.VM, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get cloud-init instance ID for backup")
		return err
	}

	if instanceIDBackup == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping cloud-init instance ID as already stored")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMCloudInitInstanceIDExtraConfigKey,
			Value: instanceIDBackup,
		})
	}

	bootstrapDataBackup, err := getDesiredBootstrapDataForBackup(opts.BootstrapData, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get VM bootstrap data for backup")
		return err
	}

	if bootstrapDataBackup == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping VM bootstrap data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMBootstrapDataExtraConfigKey,
			Value: bootstrapDataBackup,
		})
	}

	diskDataBackup, err := getDesiredDiskDataForBackup(opts, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "Failed to get VM disk data for backup")
		return err
	}

	if diskDataBackup == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping VM disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMDiskDataExtraConfigKey,
			Value: diskDataBackup,
		})
	}

	if len(ecToUpdate) != 0 {
		opts.VMCtx.Logger.Info("Updating VM ExtraConfig with backup data")
		opts.VMCtx.Logger.V(4).Info("", "ExtraConfig", ecToUpdate)
		if _, err := opts.VcVM.Reconfigure(opts.VMCtx, types.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}); err != nil {
			opts.VMCtx.Logger.Error(err, "Failed to update VM ExtraConfig for backup")
			return err
		}
	}

	return nil
}

func getDesiredVMKubeDataForBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// If the ExtraConfig already contains the latest VM spec, determined by
	// 'metadata.generation', return an empty string to skip the backup.
	if ecKubeData, ok := ecMap[constants.BackupVMKubeDataExtraConfigKey]; ok {
		vmFromBackup, err := constructVMObj(ecKubeData)
		if err != nil {
			return "", err
		}
		if vmFromBackup.ObjectMeta.Generation >= vm.ObjectMeta.Generation {
			return "", nil
		}
	}

	backupVM := vm.DeepCopy()
	backupVM.Status = vmopv1.VirtualMachineStatus{}
	backupVMYaml, err := yaml.Marshal(backupVM)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(backupVMYaml))
}

func constructVMObj(ecKubeData string) (vmopv1.VirtualMachine, error) {
	var vmObj vmopv1.VirtualMachine
	decodedKubeData, err := util.TryToDecodeBase64Gzip([]byte(ecKubeData))
	if err != nil {
		return vmObj, err
	}

	err = yaml.Unmarshal([]byte(decodedKubeData), &vmObj)
	return vmObj, err
}

func getDesiredCloudInitInstanceIDForBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// Cloud-Init instance ID should not be changed once persisted in VM's
	// ExtraConfig. Return an empty string to skip the backup if it exists.
	if _, ok := ecMap[constants.BackupVMCloudInitInstanceIDExtraConfigKey]; ok {
		return "", nil
	}

	instanceID := vm.Annotations[vmopv1.InstanceIDAnnotation]
	if instanceID == "" {
		instanceID = string(vm.UID)
	}

	return util.EncodeGzipBase64(instanceID)
}

func getDesiredBootstrapDataForBackup(
	bootstrapDataRaw map[string]string,
	ecMap map[string]string) (string, error) {
	// No bootstrap data is specified, return an empty string to skip the backup.
	if len(bootstrapDataRaw) == 0 {
		return "", nil
	}

	bootstrapDataJSON, err := json.Marshal(bootstrapDataRaw)
	if err != nil {
		return "", err
	}
	bootstrapDataBackup, err := util.EncodeGzipBase64(string(bootstrapDataJSON))
	if err != nil {
		return "", err
	}

	// Return an empty string to skip the backup if the data is unchanged.
	if bootstrapDataBackup == ecMap[constants.BackupVMBootstrapDataExtraConfigKey] {
		return "", nil
	}

	return bootstrapDataBackup, nil
}

func getDesiredDiskDataForBackup(
	opts BackupOptions,
	ecMap map[string]string) (string, error) {
	// Return an empty string to skip backup if no disk uuid to PVC is specified.
	if len(opts.DiskUUIDToPVC) == 0 {
		return "", nil
	}

	deviceList, err := opts.VcVM.Device(opts.VMCtx)
	if err != nil {
		return "", err
	}

	var diskData []VMDiskData
	for _, device := range deviceList.SelectByType((*types.VirtualDisk)(nil)) {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if b, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				if pvc, ok := opts.DiskUUIDToPVC[b.Uuid]; ok {
					diskData = append(diskData, VMDiskData{
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
	if diskDataBackup == ecMap[constants.BackupVMDiskDataExtraConfigKey] {
		return "", nil
	}

	return diskDataBackup, nil
}
