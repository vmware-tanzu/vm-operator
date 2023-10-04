// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"
	"encoding/json"

	"sigs.k8s.io/yaml"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

type VMDiskData struct {
	// ID of the virtual disk object (only set for FCDs).
	VDiskID string
	// Filename contains the datastore path to the virtual disk.
	FileName string
}

// BackupVirtualMachine backs up the required data of a VM into its ExtraConfig.
// Currently, the following data is backed up:
// - Kubernetes VirtualMachine object in YAML format (without its .status field).
// - VM bootstrap data in JSON (if provided).
// - List of VM disk data in JSON (including FCDs attached to the VM).
func BackupVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	bootstrapData map[string]string) error {

	resVM := res.NewVMFromObject(vcVM)
	moVM, err := resVM.GetProperties(vmCtx, []string{"config", "runtime"})
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM properties for backup")
		return err
	}
	curEcMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)

	var ecToUpdate []types.BaseOptionValue

	vmKubeDataBackup, err := getVMKubeDataBackup(vmCtx.VM, curEcMap)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM kube data for backup")
		return err
	}
	if vmKubeDataBackup == "" {
		vmCtx.Logger.V(4).Info("Skipping VM kube data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMKubeDataExtraConfigKey,
			Value: vmKubeDataBackup,
		})
	}

	instanceIDBackup, err := getInstanceIDBackup(vmCtx.VM, curEcMap)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM instance ID for backup")
		return err
	}
	if instanceIDBackup == "" {
		vmCtx.Logger.V(4).Info("Skipping VM instance ID backup as it exists")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMInstanceIDExtraConfigKey,
			Value: instanceIDBackup,
		})
	}

	bootstrapDataBackup, err := getBootstrapDataBackup(bootstrapData, curEcMap)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM bootstrap data for backup")
		return err
	}
	if bootstrapDataBackup == "" {
		vmCtx.Logger.V(4).Info("Skipping VM bootstrap data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMBootstrapDataExtraConfigKey,
			Value: bootstrapDataBackup,
		})
	}

	diskDataBackup, err := getDiskDataBackup(vmCtx, resVM, curEcMap)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM disk data for backup")
		return err
	}
	if diskDataBackup == "" {
		vmCtx.Logger.V(4).Info("Skipping VM disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   constants.BackupVMDiskDataExtraConfigKey,
			Value: diskDataBackup,
		})
	}

	if len(ecToUpdate) != 0 {
		vmCtx.Logger.V(4).Info("Updating VM ExtraConfig with backup data")
		if _, err := vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}); err != nil {
			vmCtx.Logger.Error(err, "Failed to update VM ExtraConfig for backup")
			return err
		}
	}

	return nil
}

func getVMKubeDataBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// If the ExtraConfig already contains the latest VM spec, determined by
	// 'metadata.generation', return an empty string to skip the backup.
	if ecKubeData, ok := ecMap[constants.BackupVMKubeDataExtraConfigKey]; ok {
		curBackupVM, err := constructVMObj(ecKubeData)
		if err != nil {
			return "", err
		}
		if curBackupVM.ObjectMeta.Generation >= vm.ObjectMeta.Generation {
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

func getInstanceIDBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	// Instance ID should not be changed once persisted in VM's ExtraConfig.
	// Return an empty string to skip the backup if it already exists.
	if _, ok := ecMap[constants.BackupVMInstanceIDExtraConfigKey]; ok {
		return "", nil
	}

	instanceID := vm.Annotations[vmopv1.InstanceIDAnnotation]
	if instanceID == "" {
		instanceID = string(vm.UID)
	}

	return util.EncodeGzipBase64(instanceID)
}

func getBootstrapDataBackup(
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

func getDiskDataBackup(
	ctx goctx.Context,
	resVM *res.VirtualMachine,
	ecMap map[string]string) (string, error) {
	disks, err := resVM.GetVirtualDisks(ctx)
	if err != nil {
		return "", err
	}

	diskData := []VMDiskData{}
	for _, device := range disks {
		if disk, ok := device.(*types.VirtualDisk); ok {
			vmDiskData := VMDiskData{}
			if disk.VDiskId != nil {
				vmDiskData.VDiskID = disk.VDiskId.Id
			}
			if b, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				vmDiskData.FileName = b.FileName
			}
			// Only add the disk data if it's not empty.
			if vmDiskData != (VMDiskData{}) {
				diskData = append(diskData, vmDiskData)
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
