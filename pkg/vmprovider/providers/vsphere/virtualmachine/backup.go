// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"sigs.k8s.io/yaml"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

// BackupVirtualMachine backs up the VM's kube data and bootstrap data into the
// vSphere VM's ExtraConfig. The status fields will not be backed up, as we expect
// to recreate and reconcile these resources during the restore process.
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

	bootstrapDataYaml, err := yaml.Marshal(bootstrapData)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(bootstrapDataYaml))
}
