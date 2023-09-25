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
	vmKubeData, err := getEncodedVMKubeData(vmCtx.VM)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get encoded VM kube data")
		return err
	}

	vmBootstrapData, err := getEncodedVMBootstrapData(bootstrapData)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get encoded VM bootstrap data")
		return err
	}
	if len(vmBootstrapData) == 0 {
		vmCtx.Logger.Info("No VM bootstrap data is provided for backup")
	}

	vmDiskData, err := getEncodedVMDiskData(vmCtx, vcVM)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get encoded VM disk data")
		return err
	}

	_, err = vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
		ExtraConfig: []types.BaseOptionValue{
			&types.OptionValue{
				Key:   constants.BackupVMKubeDataExtraConfigKey,
				Value: vmKubeData,
			},
			&types.OptionValue{
				Key:   constants.BackupVMBootstrapDataExtraConfigKey,
				Value: vmBootstrapData,
			},
			&types.OptionValue{
				Key:   constants.BackupVMDiskDataExtraConfigKey,
				Value: vmDiskData,
			},
		}})
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to reconfigure VM ExtraConfig for backup")
		return err
	}

	return nil
}

func getEncodedVMKubeData(vm *vmopv1.VirtualMachine) (string, error) {
	backupVM := vm.DeepCopy()
	backupVM.Status = vmopv1.VirtualMachineStatus{}
	backupVMYaml, err := yaml.Marshal(backupVM)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(backupVMYaml))
}

func getEncodedVMBootstrapData(bootstrapData map[string]string) (string, error) {
	if len(bootstrapData) == 0 {
		return "", nil
	}

	bootstrapDataJSON, err := json.Marshal(bootstrapData)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(bootstrapDataJSON))
}

func getEncodedVMDiskData(
	ctx goctx.Context, vcVM *object.VirtualMachine) (string, error) {
	resVM := res.NewVMFromObject(vcVM)
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

	return util.EncodeGzipBase64(string(diskDataJSON))
}
