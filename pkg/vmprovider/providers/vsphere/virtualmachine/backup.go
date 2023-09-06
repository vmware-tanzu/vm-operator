// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	goctx "context"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

// BackupVirtualMachine backs up the VM's kube data and bootstrap data into the VM's ExtraConfig.
func BackupVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	k8sClient ctrlruntime.Client) error {
	vmKubeData, err := getEncodedVMKubeData(vmCtx.VM)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM kube data for backup")
		return err
	}

	vmBootstrapData, err := getEncodedVMBootstrapData(vmCtx.Context, vmCtx.VM, k8sClient)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to get VM bootstrap data for backup")
		return err
	}
	if vmBootstrapData == "" {
		vmCtx.Logger.V(4).Info("No bootstrap data is set to the VM for backup")
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
		vmCtx.Logger.Error(err, "Failed to reconfigure VM's ExtraConfig with backup data")
		return err
	}

	return nil
}

func getEncodedVMKubeData(vm *vmopv1.VirtualMachine) (string, error) {
	backupVM := vm.DeepCopy()
	// No need to back up status fields as we expect the VM to be re-created during restore.
	backupVM.Status = vmopv1.VirtualMachineStatus{}
	backupYaml, err := yaml.Marshal(backupVM)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(backupYaml))
}

func getEncodedVMBootstrapData(
	ctx goctx.Context,
	vm *vmopv1.VirtualMachine,
	k8sClient ctrlruntime.Client) (string, error) {
	// Return early if the VM bootstrap data is not set.
	if vm.Spec.VmMetadata == nil {
		return "", nil
	}

	var bootstrapObj ctrlruntime.Object

	if cmName := vm.Spec.VmMetadata.ConfigMapName; cmName != "" {
		cm := &corev1.ConfigMap{}
		cmKey := ctrlruntime.ObjectKey{Name: cmName, Namespace: vm.Namespace}
		if err := k8sClient.Get(ctx, cmKey, cm); err != nil {
			return "", err
		}
		cm.APIVersion = "v1"
		cm.Kind = "ConfigMap"
		bootstrapObj = cm
	}

	if secretName := vm.Spec.VmMetadata.SecretName; secretName != "" {
		secret := &corev1.Secret{}
		secretKey := ctrlruntime.ObjectKey{Name: secretName, Namespace: vm.Namespace}
		if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
			return "", err
		}
		secret.APIVersion = "v1"
		secret.Kind = "Secret"
		bootstrapObj = secret
	}

	if bootstrapObj == nil {
		return "", nil
	}

	bootstrapYaml, err := yaml.Marshal(bootstrapObj)
	if err != nil {
		return "", err
	}

	return util.EncodeGzipBase64(string(bootstrapYaml))
}
